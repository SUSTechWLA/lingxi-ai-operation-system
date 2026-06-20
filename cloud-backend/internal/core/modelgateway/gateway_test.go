package modelgateway_test

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/fake"
)

func TestGatewayWithFakeProvider(t *testing.T) {
	g := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	g.RegisterProvider(fp, modelgateway.CapTextToText, modelgateway.CapTextToImage, modelgateway.CapTextToVideo, modelgateway.CapImageToText)

	req := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages:   []modelgateway.Message{{Role: "user", Content: "generate a script about cats"}},
	}

	result, err := g.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
	if result.Usage.Model != "fake-v1" {
		t.Errorf("expected fake-v1, got %s", result.Usage.Model)
	}
}

func TestGatewayFingerprintCaching(t *testing.T) {
	g := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	g.RegisterProvider(fp, modelgateway.CapTextToText)

	req := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages:   []modelgateway.Message{{Role: "user", Content: "hello"}},
	}

	result1, err := g.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result1.Cached {
		t.Error("first call should not be cached")
	}

	result2, err := g.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result2.Cached {
		t.Error("second call should be cached")
	}
}

func TestGatewayFakeProviderFaultInjection(t *testing.T) {
	g := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	g.RegisterProvider(fp, modelgateway.CapTextToText)

	req := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Test:       &modelgateway.TestConfig{FailMode: "rate_limit", FailCount: 1},
	}

	_, err := g.Execute(context.Background(), req)
	if err == nil {
		t.Error("expected error for rate_limit injection")
	}
}

func TestCapabilityConstants(t *testing.T) {
	caps := []modelgateway.Capability{
		modelgateway.CapTextToText, modelgateway.CapImageToText,
		modelgateway.CapTextToImage, modelgateway.CapTextToVideo,
		modelgateway.CapImageToVideo,
	}
	for _, c := range caps {
		if string(c) == "" {
			t.Error("capability should not be empty")
		}
	}
}

func TestGatewayError(t *testing.T) {
	err := &modelgateway.GatewayError{Code: modelgateway.ErrRateLimited, Message: "test", Retry: true}
	if !err.Retry {
		t.Error("rate limited errors should be retryable")
	}
	if err.Error() == "" {
		t.Error("error string should not be empty")
	}

	err2 := &modelgateway.GatewayError{Code: modelgateway.ErrSchemaInvalid, Message: "test", Retry: false}
	if err2.Retry {
		t.Error("schema invalid errors should NOT be retryable")
	}
}
