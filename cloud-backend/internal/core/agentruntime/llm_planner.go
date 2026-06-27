package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type PlannerLLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type LLMPlannerOptions struct {
	MaxTools int
}

type LLMPlanner struct {
	tools    ToolListProvider
	client   PlannerLLMClient
	maxTools int
}

func NewLLMPlanner(tools ToolListProvider, client PlannerLLMClient, opts LLMPlannerOptions) *LLMPlanner {
	maxTools := opts.MaxTools
	if maxTools <= 0 {
		maxTools = 6
	}
	return &LLMPlanner{tools: tools, client: client, maxTools: maxTools}
}

func (p *LLMPlanner) GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error) {
	if p == nil || p.tools == nil || p.client == nil {
		return nil, fmt.Errorf("llm planner is not configured")
	}
	domain := req.Domain
	if domain == "" {
		domain = inferDomain(req.Message)
	}

	// Use HybridToolRetriever for multi-signal scoring instead of brute-force
	// heuristic selection. This selects tools by capability, keyword, tag, cost,
	// and risk relevance rather than a single-domain filter.
	retriever := NewHybridToolRetriever(p.tools.ListManifests())
	candidates, err := retriever.Retrieve(ctx, RetrieveRequest{
		Query:        req.Message,
		Domain:       domain,
		MaxCostLevel: req.MaxCostLevel,
		MaxRiskLevel: req.MaxRiskLevel,
		CoarseTopK:   30,
		PlannerTopK:  p.maxTools,
		IncludeCapabilities: []string{
			"video_planning",
			"script_generation",
			"video_composition",
			"hyperframes",
			"video_render",
			"artifact_package",
			"quality_check",
		},
		ExcludeCapabilities: []string{
			"seedance",
			"tts",
			"asr",
			"platform_publish",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tool retrieval failed: %w", err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no tools matched domain %q", domain)
	}

	raw, err := p.client.Complete(ctx, plannerSystemPrompt(), plannerUserPrompt(req, domain, candidates))
	if err != nil {
		return nil, err
	}
	var plan AgentPlan
	if err := jsonx.ExtractJSON(raw, &plan); err != nil {
		return nil, fmt.Errorf("parse llm agent plan: %w", err)
	}
	normalizeLLMPlan(&plan, req, domain, p.maxTools)
	manifestsByName := manifestMap(p.tools.ListManifests())
	fillRequestRequiredInputs(plan.Steps, manifestsByName, req)
	wireRequiredStepInputs(plan.Steps, manifestsByName)
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("llm planner returned no steps")
	}
	return &plan, nil
}

type HybridPlanner struct {
	primary  Planner
	fallback Planner
}

func NewHybridPlanner(primary Planner, fallback Planner) *HybridPlanner {
	return &HybridPlanner{primary: primary, fallback: fallback}
}

// RepairPlan attempts to fix a Guard-rejected AgentPlan by sending the error
// and the original plan back to the LLM for one repair attempt.
// Returns the repaired plan or an error if repair fails.
func (p *LLMPlanner) RepairPlan(ctx context.Context, originalPlan *AgentPlan, guardError string, manifests []*tool.ToolManifest) (*AgentPlan, error) {
	if p.client == nil {
		return nil, fmt.Errorf("llm client not configured")
	}

	planJSON, err := json.MarshalIndent(originalPlan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal original plan for repair: %w", err)
	}

	userPayload := map[string]interface{}{
		"guardError":     guardError,
		"originalPlan":   json.RawMessage(planJSON),
		"candidateTools": compactToolManifests(manifests),
	}

	encoded, _ := json.MarshalIndent(userPayload, "", "  ")
	userPrompt := "你生成的 AgentPlan 未通过系统校验。请只输出修复后的 AgentPlan JSON。不要解释，不要 Markdown。\n" + string(encoded)

	systemPrompt := strings.TrimSpace(`
你是 AIOS 的 LLM Planner 修复模式。
你之前生成的 AgentPlan 未通过 Guard 校验。请根据错误信息修复 JSON。
禁止输出 DAGRequest、节点类型或执行图。
只输出修复后的 AgentPlan JSON。

必须遵守以下 JSON Schema：
` + AgentPlanJSONSchema + `
`)

	raw, err := p.client.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("repair plan llm call: %w", err)
	}

	var plan AgentPlan
	if err := jsonx.ExtractJSON(raw, &plan); err != nil {
		return nil, fmt.Errorf("parse repaired agent plan: %w", err)
	}
	normalizeLLMPlan(&plan, StartRunRequest{}, "", p.maxTools)
	wireRequiredStepInputs(plan.Steps, manifestMap(manifests))
	return &plan, nil
}

func (p *HybridPlanner) GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error) {
	if p == nil {
		return nil, fmt.Errorf("hybrid planner is not configured")
	}
	if p.primary != nil {
		plan, err := p.primary.GeneratePlan(ctx, req)
		if err == nil {
			return plan, nil
		}
		if p.fallback == nil {
			return nil, err
		}
	}
	if p.fallback == nil {
		return nil, fmt.Errorf("hybrid planner fallback is not configured")
	}
	return p.fallback.GeneratePlan(ctx, req)
}

type OpenAIPlannerClient struct {
	cfg        config.OpenAIConfig
	httpClient *http.Client
}

func NewOpenAIPlannerClient(cfg config.OpenAIConfig) *OpenAIPlannerClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	return &OpenAIPlannerClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (c *OpenAIPlannerClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("planner llm client is not configured")
	}
	if c.cfg.APIKey == "" {
		return "", fmt.Errorf("planner llm API key is not configured")
	}
	model := c.cfg.Model
	if model == "" {
		model = "gpt-4"
	}
	maxTokens := c.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	body, err := json.Marshal(map[string]interface{}{
		"model":       model,
		"temperature": c.cfg.Temperature,
		"max_tokens":  maxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal planner llm request: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create planner llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call planner llm: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("planner llm returned status %d: %s", resp.StatusCode, string(respBody))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", fmt.Errorf("decode planner llm response: %w", err)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("planner llm returned empty content")
	}
	return decoded.Choices[0].Message.Content, nil
}

// AgentPlanJSONSchema is the JSON Schema for AgentPlan validation.
// It is included in the LLMPlanner prompt to guide structured output.
const AgentPlanJSONSchema = `{
  "type": "object",
  "required": ["goal", "mode", "steps"],
  "properties": {
    "goal": { "type": "string" },
    "domain": { "type": "string" },
    "mode": { "type": "string", "enum": ["dynamic_agent"] },
    "steps": {
      "type": "array",
      "minItems": 1,
      "maxItems": 10,
      "items": {
        "type": "object",
        "required": ["id", "intent", "tool", "arguments"],
        "properties": {
          "id": { "type": "string" },
          "intent": { "type": "string" },
          "tool": { "type": "string" },
          "arguments": { "type": "object" },
          "dependsOn": { "type": "array", "items": { "type": "string" } },
          "expectedOutput": { "type": "array", "items": { "type": "string" } },
          "produceArtifact": { "type": "boolean" }
        }
      }
    },
    "budget": { "type": "object" },
    "stopPolicy": { "type": "object" }
  }
}`

func plannerSystemPrompt() string {
	return strings.TrimSpace(`
你是 Dynamic Guided Video Planner。

你的职责：
1. 根据用户需求和候选工具生成 AgentPlan。
2. 自主决定工具顺序、依赖关系和参数引用。
3. 只使用候选工具，不得发明工具。
4. 尊重每个工具的 approvalPolicy 和 humanReview。
5. 尊重每个工具的 executionPlane：local 工具需要用户本地设备在线。
6. 本地工具 requiresUserDevice=true 时，必须确保前置包含 capability_preflight。
7. 你不需要手写审核节点，系统会根据 ToolManifest 自动插入。
8. 你不得绕过需要人工审核的工具。
9. 第一版只生成图文视频，不使用 Seedance、TTS、ASR、平台发布工具。
10. hyperframes_renderer 只能在 preview 或 composition 已确认后执行。

只输出 JSON，不要输出 Markdown。
禁止输出 DAGRequest、节点类型、ai_node、workflow_template 或执行图细节。
系统会在你输出后通过 PlanGuard 校验并由 PlanCompiler 自动插入审核节点。

必须遵守以下 JSON Schema：
` + AgentPlanJSONSchema + `
`)
}

func plannerUserPrompt(req StartRunRequest, domain string, manifests []*tool.ToolManifest) string {
	payload := map[string]interface{}{
		"userRequest": map[string]interface{}{
			"message": req.Message,
			"domain":  domain,
			"context": req.Context,
			"mode":    req.Mode,
		},
		"candidateTools": compactToolManifests(manifests),
		"requiredSchema": map[string]interface{}{
			"goal":       "string",
			"domain":     "string",
			"mode":       "dynamic_agent",
			"steps":      "array<AgentStep>",
			"budget":     "AgentBudget",
			"stopPolicy": "StopPolicy",
		},
		"stepRules": []string{
			"每个 step.tool 必须来自 candidateTools.name",
			"dependsOn 只能引用更早出现的 step.id",
			"arguments 必须补齐工具 required parameters；可使用 {{step_id.output.field}} 引用上游输出",
			"需要产物时设置 produceArtifact=true；审核策略不要写入计划，系统会从 ToolManifest 自动处理",
			"默认 plan once，maxReplans 不超过 1",
		},
	}
	encoded, _ := json.MarshalIndent(payload, "", "  ")
	return "请严格返回 AgentPlan JSON，不能返回 DAGRequest。\n" + string(encoded)
}

func compactToolManifests(manifests []*tool.ToolManifest) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil {
			continue
		}
		entry := map[string]interface{}{
			"name":                 manifest.Name,
			"description":          manifest.Description,
			"type":                 manifest.Type,
			"executionPlane":       manifest.ExecutionPlane,
			"requiresUserDevice":   manifest.RequiresUserDevice,
			"artifactLocation":     manifest.ArtifactLocation,
			"parameters":           manifest.Parameters,
			"output":               manifest.Output,
			"capabilities":         manifest.Capabilities,
			"tags":                 manifest.Tags,
			"costLevel":            manifest.CostLevel,
			"riskLevel":            manifest.RiskLevel,
			"sideEffect":           manifest.SideEffect,
			"approvalPolicy":       manifest.ApprovalPolicy,
			"artifactPolicy":       manifest.ArtifactPolicy,
			"qualityPolicy":        manifest.QualityPolicy,
			"nextRecommendedTools": manifest.NextRecommendedTools,
			"skillPackageId":       manifest.SkillPackageID,
		}
		// Include local-tool fields when applicable.
		if manifest.ExecutionPlane == tool.ExecutionPlaneLocal {
			entry["localCommand"] = manifest.LocalCommand
			entry["localRequirements"] = manifest.LocalRequirements
		}
		// Include human review UI metadata when present.
		if manifest.HumanReview != nil {
			entry["humanReview"] = manifest.HumanReview
		}
		out = append(out, entry)
	}
	return out
}

func normalizeLLMPlan(plan *AgentPlan, req StartRunRequest, domain string, maxTools int) {
	if plan.Goal == "" {
		plan.Goal = req.Message
	}
	if plan.Domain == "" {
		plan.Domain = domain
	}
	if plan.Mode == "" {
		plan.Mode = "dynamic_agent"
	}
	if plan.Budget.MaxSteps == 0 {
		plan.Budget.MaxSteps = max(maxTools, len(plan.Steps))
	}
	if plan.Budget.MaxToolCalls == 0 {
		plan.Budget.MaxToolCalls = len(plan.Steps)
	}
	if plan.Budget.MaxLLMCalls == 0 {
		plan.Budget.MaxLLMCalls = len(plan.Steps) + 1
	}
	if plan.Budget.MaxReplans == 0 {
		plan.Budget.MaxReplans = 1
	}
	if plan.Budget.MaxCostLevel == "" {
		plan.Budget.MaxCostLevel = tool.CostMedium
	}
	if !plan.StopPolicy.StopWhenEnough {
		plan.StopPolicy.StopWhenEnough = true
	}
}
