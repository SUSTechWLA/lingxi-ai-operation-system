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
	knowledgePolicy := DefaultKnowledgePolicy(req.Message, domain)
	retriever := NewHybridToolRetriever(p.tools.ListManifests())
	candidates, err := retriever.Retrieve(ctx, RetrieveRequest{
		UserInput:       req.Message,
		Domain:          domain,
		KnowledgePolicy: knowledgePolicy,
		MaxCostLevel:    req.MaxCostLevel,
		MaxRiskLevel:    req.MaxRiskLevel,
		CoarseTopK:      30,
		PlannerTopK:     p.maxTools,
		IncludeCapabilities: []string{
			"video_planning",
			"script_generation",
			"video_composition",
			"hyperframes",
			"video_render",
			"artifact_package",
			"quality_check",
			"knowledge_research",
			"fact_gathering",
			"fresh_knowledge",
			"news_search",
			"web_search",
			"fact_retrieval",
			"current_event_retrieval",
			"video_creation",
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
	allManifests := p.tools.ListManifests()
	normalizeLLMPlan(&plan, req, domain, p.maxTools, allManifests)
	if plan.ToolTrace == nil {
		plan.ToolTrace = &ToolTrace{}
	}
	plan.ToolTrace.CandidateTools = candidateTrace(candidates)
	if plan.KnowledgePolicy == nil {
		plan.KnowledgePolicy = knowledgePolicy
	}
	if err := validatePlanUsesCandidateTools(&plan, candidates); err != nil {
		return nil, err
	}
	manifestsByName := manifestMap(allManifests)
	fillRequestRequiredInputs(plan.Steps, manifestsByName, req)
	wireRequiredStepInputs(plan.Steps, manifestsByName)
	repairInvalidOutputReferences(plan.Steps, manifestsByName)
	catalog := toolManifestCatalog(manifestsByName)
	NewPlanCompiler(catalog).PreparePlan(&plan)
	if err := NewPlanGuard(catalog, nil).Validate(&plan); err != nil {
		return nil, fmt.Errorf("llm planner returned invalid plan: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("llm planner returned no steps")
	}
	return &plan, nil
}

type toolManifestCatalog map[string]*tool.ToolManifest

func (c toolManifestCatalog) GetManifest(name string) *tool.ToolManifest {
	return c[name]
}

func validatePlanUsesCandidateTools(plan *AgentPlan, candidates []ToolCandidate) error {
	if plan == nil {
		return nil
	}
	allowed := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.Name != "" {
			allowed[candidate.Name] = true
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	for _, step := range plan.Steps {
		if step.Tool == "" || allowed[step.Tool] {
			continue
		}
		return fmt.Errorf("llm planner selected tool %q outside retrieved candidates", step.Tool)
	}
	return nil
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
// It implements the PlanRepairer interface, obtaining tool manifests internally.
func (p *LLMPlanner) RepairPlan(ctx context.Context, originalPlan *AgentPlan, guardError string) (*AgentPlan, error) {
	if p == nil || p.tools == nil {
		return nil, fmt.Errorf("llm planner not configured for repair")
	}
	manifests := p.tools.ListManifests()
	return p.repairPlanWithManifests(ctx, originalPlan, guardError, manifests)
}

// repairPlanWithManifests is the core repair logic with explicit manifests.
func (p *LLMPlanner) repairPlanWithManifests(ctx context.Context, originalPlan *AgentPlan, guardError string, manifests []*tool.ToolManifest) (*AgentPlan, error) {
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
	normalizeLLMPlan(&plan, StartRunRequest{}, "", p.maxTools, manifests)
	manifestsByName := manifestMap(manifests)
	wireRequiredStepInputs(plan.Steps, manifestsByName)
	repairInvalidOutputReferences(plan.Steps, manifestsByName)
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

// RepairPlan implements PlanRepairer by delegating to the primary planner
// (if it supports repair), falling back to the fallback planner.
func (p *HybridPlanner) RepairPlan(ctx context.Context, plan *AgentPlan, guardError string) (*AgentPlan, error) {
	if p == nil {
		return nil, fmt.Errorf("hybrid planner is not configured")
	}
	if repairer, ok := p.primary.(PlanRepairer); ok {
		return repairer.RepairPlan(ctx, plan, guardError)
	}
	if repairer, ok := p.fallback.(PlanRepairer); ok {
		return repairer.RepairPlan(ctx, plan, guardError)
	}
	return nil, fmt.Errorf("no planner in hybrid chain supports plan repair")
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
    "knowledgePolicy": {
      "type": "object",
      "properties": {
        "contentType": { "type": "string" },
        "freshnessLevel": { "type": "string", "enum": ["none", "low", "medium", "high"] },
        "retrievalPolicy": { "type": "string", "enum": ["none", "optional", "required", "forbidden"] },
        "knowledgeType": { "type": "string" },
        "allowedTools": { "type": "array", "items": { "type": "string" } },
        "forbiddenTools": { "type": "array", "items": { "type": "string" } },
        "requiredCapabilities": { "type": "array", "items": { "type": "string" } },
        "forbiddenCapabilities": { "type": "array", "items": { "type": "string" } },
        "searchQueries": { "type": "array", "items": { "type": "string" } },
        "freshnessDays": { "type": "integer" },
        "maxSearchResults": { "type": "integer" },
        "mustUseFacts": { "type": "boolean" },
        "mustCiteFacts": { "type": "boolean" },
        "blockOnEmptyFacts": { "type": "boolean" }
      }
    },
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
          "reason": { "type": "string" },
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
11. 必须输出 knowledgePolicy。
12. 检索策略：
    - 用户请求包含最新、最近、今天、昨天、刚刚、实时、现在、出线、夺冠、晋级、比赛结果、世界杯、奥运会、政策、法规、价格、票房、榜单、2026、今年、本届、现任等强时效内容时，freshnessLevel 必须为 high，retrievalPolicy 必须为 required。
    - “发布/上线/开源”只有在需要核验外部当前事实、新闻事件、版本发布时间、榜单或市场数据时才算强时效；如果用户是在宣传自己的产品、开源项目或“本系统”，且事实主要来自用户 brief 或稳定产品资料，retrievalPolicy 应为 none 或 optional。
    - 强时效检索必须从 candidateTools 中选择具备 fresh_knowledge / news_search / web_search / current_event_retrieval / fact_retrieval 等 capability 的工具，不得按固定工具名臆造。
    - 纯观点、创意故事、情感表达、稳定知识口播时，retrievalPolicy 应为 none 或 optional；没有明确理由时不要选择 fresh knowledge/search 类工具。
    - fresh knowledge/search 类工具只用于需要外部事实、新闻、实时结果或当前事件确认的任务。
    - retrievalPolicy=required 时必须设置 searchQueries、mustUseFacts=true、mustCiteFacts=true、blockOnEmptyFacts=true。
    - 如果 required 检索失败，后续脚本生成必须阻断，不得回退到模型旧知识。
13. 每个选择工具的 step 必须填写 reason，说明为什么这个工具适合当前任务。

只输出 JSON，不要输出 Markdown。
禁止输出 DAGRequest、节点类型、ai_node、workflow_template 或执行图细节。
系统会在你输出后通过 PlanGuard 校验并由 PlanCompiler 自动插入审核节点。

必须遵守以下 JSON Schema：
` + AgentPlanJSONSchema + `
`)
}

func plannerUserPrompt(req StartRunRequest, domain string, candidates []ToolCandidate) string {
	payload := map[string]interface{}{
		"userRequest": map[string]interface{}{
			"message": req.Message,
			"domain":  domain,
			"context": req.Context,
			"mode":    req.Mode,
		},
		"candidateTools": compactToolCandidates(candidates),
		"requiredSchema": map[string]interface{}{
			"goal":   "string",
			"domain": "string",
			"mode":   "dynamic_agent",
			"knowledgePolicy": map[string]string{
				"contentType":     "current_event|sports_event|opinion|evergreen_knowledge|creative_story|historical_story",
				"freshnessLevel":  "none|low|medium|high",
				"retrievalPolicy": "none|optional|required|forbidden",
				"knowledgeType":   "latest_news|background_facts|none",
			},
			"steps":      "array<AgentStep>",
			"budget":     "AgentBudget",
			"stopPolicy": "StopPolicy",
		},
		"stepRules": []string{
			"每个 step.tool 必须来自 candidateTools.name",
			"dependsOn 只能引用更早出现的 step.id",
			"arguments 必须补齐工具 required parameters；可使用 {{step_id.output.field}} 引用上游输出",
			"每个使用工具的 step.reason 必须解释选择该工具的任务依据和 capability 依据",
			"需要产物时设置 produceArtifact=true；审核策略不要写入计划，系统会从 ToolManifest 自动处理",
			"默认 plan once，maxReplans 不超过 1",
		},
	}
	encoded, _ := json.MarshalIndent(payload, "", "  ")
	return "请严格返回 AgentPlan JSON，不能返回 DAGRequest。\n" + string(encoded)
}

func compactToolCandidates(candidates []ToolCandidate) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(candidates))
	for _, candidate := range candidates {
		entry := map[string]interface{}{
			"name":                 candidate.Name,
			"description":          candidate.Description,
			"type":                 candidate.Type,
			"inputSchema":          cloneJSONMap(candidate.InputSchema),
			"outputSchema":         cloneJSONMap(candidate.OutputSchema),
			"parameters":           cloneLegacyParamDefs(candidate.LegacyParameters),
			"output":               cloneLegacyParamDefs(candidate.LegacyOutput),
			"providerCapabilities": cloneJSONMap(candidate.ProviderCapabilities),
			"capabilities":         append([]string(nil), candidate.Capabilities...),
			"tags":                 append([]string(nil), candidate.Tags...),
			"costLevel":            candidate.CostLevel,
			"riskLevel":            candidate.RiskLevel,
			"score":                candidate.Score,
			"reason":               candidate.Reason,
		}
		if candidate.Manifest != nil {
			entry["boundary"] = candidate.Manifest.Boundary
			entry["whenToUse"] = candidate.Manifest.WhenToUse
			entry["whenNotToUse"] = candidate.Manifest.WhenNotToUse
			entry["provider"] = candidate.Manifest.Provider
			entry["providerBinding"] = candidate.Manifest.ProviderBinding
			entry["executionPlane"] = candidate.Manifest.ExecutionPlane
			entry["requiresUserDevice"] = candidate.Manifest.RequiresUserDevice
			entry["artifactLocation"] = candidate.Manifest.ArtifactLocation
			entry["sideEffect"] = candidate.Manifest.SideEffect
			entry["approvalPolicy"] = candidate.Manifest.ApprovalPolicy
			entry["artifactPolicy"] = candidate.Manifest.ArtifactPolicy
			entry["qualityPolicy"] = candidate.Manifest.QualityPolicy
			entry["nextRecommendedTools"] = candidate.Manifest.NextRecommendedTools
			entry["skillPackageId"] = candidate.Manifest.SkillPackageID
			if candidate.Manifest.ExecutionPlane == tool.ExecutionPlaneLocal {
				entry["localCommand"] = candidate.Manifest.LocalCommand
				entry["localRequirements"] = candidate.Manifest.LocalRequirements
			}
			if candidate.Manifest.HumanReview != nil {
				entry["humanReview"] = candidate.Manifest.HumanReview
			}
		}
		out = append(out, cloneJSONMap(entry))
	}
	return out
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
			"boundary":             manifest.Boundary,
			"executionPlane":       manifest.ExecutionPlane,
			"requiresUserDevice":   manifest.RequiresUserDevice,
			"artifactLocation":     manifest.ArtifactLocation,
			"inputSchema":          canonicalToolSchema(manifest.InputSchema, manifest.Parameters),
			"outputSchema":         canonicalToolSchema(manifest.OutputSchema, manifest.Output),
			"parameters":           cloneLegacyParamDefs(manifest.Parameters),
			"output":               cloneLegacyParamDefs(manifest.Output),
			"capabilities":         manifest.Capabilities,
			"tags":                 manifest.Tags,
			"whenToUse":            manifest.WhenToUse,
			"whenNotToUse":         manifest.WhenNotToUse,
			"costLevel":            manifest.CostLevel,
			"riskLevel":            manifest.RiskLevel,
			"sideEffect":           manifest.SideEffect,
			"approvalPolicy":       manifest.ApprovalPolicy,
			"artifactPolicy":       manifest.ArtifactPolicy,
			"qualityPolicy":        manifest.QualityPolicy,
			"nextRecommendedTools": manifest.NextRecommendedTools,
			"skillPackageId":       manifest.SkillPackageID,
			"provider":             manifest.Provider,
			"providerBinding":      manifest.ProviderBinding,
			"providerCapabilities": cloneJSONMap(manifest.ProviderCapabilities),
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
		out = append(out, cloneJSONMap(entry))
	}
	return out
}

func normalizeLLMPlan(plan *AgentPlan, req StartRunRequest, domain string, maxTools int, manifests []*tool.ToolManifest) {
	if plan.Goal == "" {
		plan.Goal = req.Message
	}
	if plan.Domain == "" {
		plan.Domain = domain
	}
	if plan.Mode == "" {
		plan.Mode = "dynamic_agent"
	}
	if plan.KnowledgePolicy == nil {
		plan.KnowledgePolicy = defaultKnowledgePolicyForTools(req.Message, plan.Domain, manifests)
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
