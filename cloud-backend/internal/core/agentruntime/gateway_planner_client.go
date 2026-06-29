package agentruntime

import (
	"context"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

// GatewayPlannerClient implements PlannerLLMClient using the unified ModelGateway
// for caching, retry, and provider routing.
type GatewayPlannerClient struct {
	gateway *modelgateway.Gateway
	model   string
}

// NewGatewayPlannerClient creates a Gateway-backed planner LLM client.
func NewGatewayPlannerClient(gateway *modelgateway.Gateway, model string) *GatewayPlannerClient {
	if model == "" {
		model = "gpt-4"
	}
	return &GatewayPlannerClient{gateway: gateway, model: model}
}

func (c *GatewayPlannerClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c == nil || c.gateway == nil {
		return "", fmt.Errorf("gateway planner client is not configured")
	}

	result, err := c.gateway.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Model:      c.model,
		Messages: []modelgateway.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]interface{}{
			"model":            c.model,
			"temperature":     0.2,
			"max_tokens":      2000,
			"response_format": map[string]string{"type": "json_object"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("gateway planner call: %w", err)
	}
	if result.Content == "" {
		return "", fmt.Errorf("gateway planner returned empty content")
	}
	return result.Content, nil
}
