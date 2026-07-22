package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/config"
)

type ClientProviderPlanner struct {
	tools    ToolListProvider
	fallback Planner
	maxTools int
}

const clientProviderPlannerTimeoutSeconds = 180

func NewClientProviderPlanner(tools ToolListProvider, fallback Planner, maxTools int) *ClientProviderPlanner {
	if maxTools <= 0 {
		maxTools = 6
	}
	return &ClientProviderPlanner{tools: tools, fallback: fallback, maxTools: maxTools}
}

func (p *ClientProviderPlanner) GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error) {
	if p == nil || p.tools == nil {
		return nil, fmt.Errorf("client provider planner is not configured")
	}
	requestTools := toolProviderForRequest(req, p.tools)
	provider := clientTextProviderFromRequest(req)
	if provider == nil {
		return p.fallbackPlan(ctx, req, fmt.Errorf("client text model provider is not configured"))
	}
	llm := NewLLMPlanner(
		requestTools,
		NewOpenAIPlannerClient(providerConfigFromMap(provider)),
		LLMPlannerOptions{MaxTools: p.maxTools},
	)
	plan, err := llm.GeneratePlan(ctx, req)
	if err == nil {
		return plan, nil
	}
	return p.fallbackPlan(ctx, req, err)
}

func (p *ClientProviderPlanner) RepairPlan(ctx context.Context, plan *AgentPlan, guardError string) (*AgentPlan, error) {
	if p == nil || p.fallback == nil {
		return nil, fmt.Errorf("client provider planner fallback is not configured")
	}
	if repairer, ok := p.fallback.(PlanRepairer); ok {
		return repairer.RepairPlan(ctx, plan, guardError)
	}
	return nil, fmt.Errorf("client provider planner has no repair fallback")
}

func (p *ClientProviderPlanner) RepairPlanForRequest(ctx context.Context, req StartRunRequest, plan *AgentPlan, guardError string) (*AgentPlan, error) {
	if p == nil {
		return nil, fmt.Errorf("client provider planner is not configured")
	}
	if provider := clientTextProviderFromRequest(req); provider != nil {
		llm := NewLLMPlanner(
			toolProviderForRequest(req, p.tools),
			NewOpenAIPlannerClient(providerConfigFromMap(provider)),
			LLMPlannerOptions{MaxTools: p.maxTools},
		)
		return llm.RepairPlanForRequest(ctx, req, plan, guardError)
	}
	if repairer, ok := p.fallback.(RequestScopedPlanRepairer); ok {
		return repairer.RepairPlanForRequest(ctx, req, plan, guardError)
	}
	return nil, fmt.Errorf("client provider planner has no request-scoped repair path")
}

func (p *ClientProviderPlanner) fallbackPlan(ctx context.Context, req StartRunRequest, llmErr error) (*AgentPlan, error) {
	if p.fallback == nil {
		return nil, llmErr
	}
	plan, err := p.fallback.GeneratePlan(ctx, req)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func clientTextProviderFromRequest(req StartRunRequest) map[string]interface{} {
	providers := clientModelProvidersFromContext(req.Context)
	if provider := providers["text_to_text"]; len(provider) > 0 {
		return provider
	}
	return nil
}

func providerConfigFromMap(provider map[string]interface{}) config.OpenAIConfig {
	cfg := config.OpenAIConfig{
		BaseURL: strings.TrimSpace(fmt.Sprint(provider["baseUrl"])),
		APIKey:  strings.TrimSpace(fmt.Sprint(provider["apiKey"])),
		Model:   strings.TrimSpace(fmt.Sprint(provider["model"])),
		Timeout: clientProviderPlannerTimeoutSeconds,
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4000
	}
	return cfg
}

var _ Planner = (*ClientProviderPlanner)(nil)
