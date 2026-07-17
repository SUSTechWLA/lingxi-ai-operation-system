package fake

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

// Provider is a fake model provider for testing. It returns deterministic
// outputs based on the request capability and supports fault injection.
type Provider struct {
	LatencyMs int64
	callCount atomic.Int64
}

func NewProvider() *Provider {
	return &Provider{LatencyMs: 10}
}

func (p *Provider) Name() string { return "fake" }

func (p *Provider) Supports(cap modelgateway.Capability) bool {
	return true // fake provider supports all capabilities
}

func (p *Provider) Health(ctx context.Context) error {
	return nil
}

func (p *Provider) Execute(ctx context.Context, req *modelgateway.ModelRequest) (*modelgateway.ModelResult, error) {
	p.callCount.Add(1)

	// Simulate latency
	if p.LatencyMs > 0 {
		time.Sleep(time.Duration(p.LatencyMs) * time.Millisecond)
	}

	// Fault injection
	if req.Test != nil {
		switch req.Test.FailMode {
		case "rate_limit":
			return nil, &modelgateway.GatewayError{Code: modelgateway.ErrRateLimited, Message: "simulated 429", Retry: true}
		case "timeout":
			return nil, &modelgateway.GatewayError{Code: modelgateway.ErrTimeout, Message: "simulated timeout", Retry: true}
		case "server_error":
			return nil, &modelgateway.GatewayError{Code: modelgateway.ErrUnavailable, Message: "simulated 500", Retry: true}
		case "invalid_schema":
			return nil, &modelgateway.GatewayError{Code: modelgateway.ErrSchemaInvalid, Message: "simulated schema error", Retry: false}
		}
	}

	// Fixture-based responses
	fingerprint := req.Fingerprint
	if fingerprint == "" {
		fingerprint = fmt.Sprintf("fake-%s-%d", req.Capability, p.callCount.Load())
	}

	var content string
	switch req.Capability {
	case modelgateway.CapTextToText:
		content = p.fixtureTextContent(req)
	case modelgateway.CapTextToImage:
		content = `{"url":"https://fake.example.com/generated.png","width":1024,"height":1024}`
	case modelgateway.CapTextToVideo:
		content = `{"jobId":"fake-job-123","status":"completed","url":"https://fake.example.com/video.mp4"}`
	case modelgateway.CapImageToVideo:
		content = `{"jobId":"fake-image-video-123","status":"completed","url":"https://fake.example.com/image-to-video.mp4"}`
	case modelgateway.CapImageToText:
		content = `{"description":"A well-composed scene with balanced lighting and clear subject focus."}`
	default:
		content = `{"result":"fake response"}`
	}

	return &modelgateway.ModelResult{
		Content:     content,
		Usage:       modelgateway.Usage{Model: "fake-v1", PromptTokens: 10, OutputTokens: 20, CostUSD: 0.0, DurationMs: p.LatencyMs},
		Fingerprint: fingerprint,
		Cached:      false,
	}, nil
}

func (p *Provider) fixtureTextContent(req *modelgateway.ModelRequest) string {
	if len(req.Messages) > 0 {
		msg := req.Messages[len(req.Messages)-1].Content
		// Simple keyword matching for fake fixtures
		if containsKeyword(msg, "shot", "script", "剧本") {
			return `{"title":"测试短片","logline":"一只猫用叫声传输二进制消息","scenes":[{"id":"s1","name":"开场","location":"城市街道"}],"characters":[{"name":"猫咪阿零","role":"主角"}],"shots":[{"id":"shot-01","shot_number":1,"duration_sec":5,"description":"开场全景"},{"id":"shot-02","shot_number":2,"duration_sec":5,"description":"猫咪特写"},{"id":"shot-03","shot_number":3,"duration_sec":5,"description":"结局"}]}`
		}
		if containsKeyword(msg, "opinion", "口播", "观点") {
			return `{"core_opinion":"AI替代的不是岗位，而是工作流程","narration_beats":[{"id":"nb-1","text":"开篇观点","duration_sec":15},{"id":"nb-2","text":"论据展开","duration_sec":15},{"id":"nb-3","text":"总结升华","duration_sec":15}],"visual_beats":[{"id":"vb-1","start_time_sec":0,"duration_sec":15,"components":[{"type":"TITLE","props":{"text":"AI与工作流程"}}]},{"id":"vb-2","start_time_sec":15,"duration_sec":15,"components":[{"type":"BULLET_LIST","props":{"items":["效率提升","流程重塑","角色转变"]}}]},{"id":"vb-3","start_time_sec":30,"duration_sec":15,"components":[{"type":"QUOTE","props":{"text":"未来已来","author":"AIOS"}}]}]}`
		}
	}
	return `{"result":"fake text response"}`
}

func containsKeyword(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if len(text) > 0 && len(kw) > 0 {
			// Simple contains check
			for i := 0; i <= len(text)-len(kw); i++ {
				if text[i:i+len(kw)] == kw {
					return true
				}
			}
		}
	}
	return false
}
