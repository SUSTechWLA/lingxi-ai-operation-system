package agentruntime

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

func TestGatewayPlannerClientAllowsProviderDefaultModel(t *testing.T) {
	provider := &captureProvider{
		result: &modelgateway.ModelResult{Content: `{"goal":"ok","steps":[]}`},
	}
	gateway := modelgateway.NewGateway("real")
	gateway.RegisterProvider(provider, modelgateway.CapTextToText)

	client := NewGatewayPlannerClient(gateway, "")
	_, err := client.Complete(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if provider.request == nil {
		t.Fatal("provider was not called")
	}
	if provider.request.Model != "" {
		t.Fatalf("expected planner request model to be omitted, got %q", provider.request.Model)
	}
	if _, ok := provider.request.Parameters["model"]; ok {
		t.Fatalf("expected planner parameters to omit model override, got %#v", provider.request.Parameters)
	}
}

type captureProvider struct {
	request *modelgateway.ModelRequest
	result  *modelgateway.ModelResult
}

func (p *captureProvider) Name() string { return "capture" }

func (p *captureProvider) Supports(cap modelgateway.Capability) bool {
	return cap == modelgateway.CapTextToText
}

func (p *captureProvider) Execute(ctx context.Context, req *modelgateway.ModelRequest) (*modelgateway.ModelResult, error) {
	_ = ctx
	p.request = req
	return p.result, nil
}

func (p *captureProvider) Health(ctx context.Context) error {
	_ = ctx
	return nil
}
