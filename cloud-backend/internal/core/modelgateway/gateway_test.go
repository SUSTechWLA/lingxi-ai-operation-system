package modelgateway_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestGatewayDoesNotCacheFaultInjectionRequests(t *testing.T) {
	g := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	g.RegisterProvider(fp, modelgateway.CapTextToText)

	req := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Test:       &modelgateway.TestConfig{FailMode: "invalid_schema"},
	}
	_, _ = g.Execute(context.Background(), req)

	req.Test = nil
	result, err := g.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("non-test request should not reuse fault injection cache: %v", err)
	}
	if result.Cached {
		t.Fatal("non-test request should execute fresh after a test request")
	}
}

func TestGatewayCacheExpires(t *testing.T) {
	g := modelgateway.NewGateway("fake")
	g.SetCacheTTL(-time.Second)
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	g.RegisterProvider(fp, modelgateway.CapTextToText)

	req := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages:   []modelgateway.Message{{Role: "user", Content: "hello"}},
	}
	if _, err := g.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := g.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cached {
		t.Fatal("expired cache entry should not be reused")
	}
}

func TestGatewayFallsBackToNextProvider(t *testing.T) {
	g := modelgateway.NewGateway("real")
	first := &stubProvider{name: "first", err: &modelgateway.GatewayError{Code: modelgateway.ErrUnavailable, Message: "down", Retry: false}}
	second := &stubProvider{name: "second", content: "ok"}
	g.RegisterProvider(first, modelgateway.CapTextToText)
	g.RegisterProvider(second, modelgateway.CapTextToText)

	result, err := g.Execute(context.Background(), &modelgateway.ModelRequest{Capability: modelgateway.CapTextToText})
	if err != nil {
		t.Fatalf("expected fallback provider to succeed: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("unexpected provider calls: first=%d second=%d", first.calls, second.calls)
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

type stubProvider struct {
	name    string
	content string
	err     error
	calls   int
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) Supports(cap modelgateway.Capability) bool {
	return cap == modelgateway.CapTextToText
}

func (p *stubProvider) Execute(context.Context, *modelgateway.ModelRequest) (*modelgateway.ModelResult, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return &modelgateway.ModelResult{Content: p.content}, nil
}

func (p *stubProvider) Health(context.Context) error {
	if p.err != nil {
		return p.err
	}
	return errors.New("not implemented")
}
