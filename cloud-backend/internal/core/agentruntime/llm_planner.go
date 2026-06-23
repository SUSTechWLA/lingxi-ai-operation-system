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

	retriever := &HeuristicPlanner{tools: p.tools, maxTools: p.maxTools}
	selected := retriever.selectTools(domain, req.Message)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no tools matched domain %q", domain)
	}

	raw, err := p.client.Complete(ctx, plannerSystemPrompt(), plannerUserPrompt(req, domain, selected))
	if err != nil {
		return nil, err
	}
	var plan AgentPlan
	if err := jsonx.ExtractJSON(raw, &plan); err != nil {
		return nil, fmt.Errorf("parse llm agent plan: %w", err)
	}
	normalizeLLMPlan(&plan, req, domain, p.maxTools)
	return &plan, nil
}

type HybridPlanner struct {
	primary  Planner
	fallback Planner
}

func NewHybridPlanner(primary Planner, fallback Planner) *HybridPlanner {
	return &HybridPlanner{primary: primary, fallback: fallback}
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

func plannerSystemPrompt() string {
	return strings.TrimSpace(`
你是 AIOS 的 LLM Planner。你的职责是根据用户需求和候选工具生成一次性的 AgentPlan。
只输出 JSON，不要输出 Markdown。
禁止输出 DAGRequest、节点类型、ai_node、workflow_template 或执行图细节。
系统会在你输出后通过 PlanGuard 校验工具、参数、成本和副作用，并由 PlanCompiler 根据 ToolManifest 自动插入审核节点。
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
		out = append(out, map[string]interface{}{
			"name":                 manifest.Name,
			"description":          manifest.Description,
			"type":                 manifest.Type,
			"parameters":           manifest.Parameters,
			"output":               manifest.Output,
			"capabilities":         manifest.Capabilities,
			"tags":                 manifest.Tags,
			"costLevel":            manifest.CostLevel,
			"riskLevel":            manifest.RiskLevel,
			"sideEffect":           manifest.SideEffect,
			"approvalPolicy":       manifest.ApprovalPolicy,
			"artifactPolicy":       manifest.ArtifactPolicy,
			"nextRecommendedTools": manifest.NextRecommendedTools,
			"skillPackageId":       manifest.SkillPackageID,
		})
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
