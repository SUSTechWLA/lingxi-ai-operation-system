package modelgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Gateway routes model requests to the appropriate provider with retry and caching.
type Gateway struct {
	providers map[Capability][]Provider
	cache     map[string]*ModelResult // fingerprint → result
	mu        sync.RWMutex
	mode      string // "fake" | "real"
}

// NewGateway creates a new model gateway.
func NewGateway(mode string) *Gateway {
	return &Gateway{
		providers: make(map[Capability][]Provider),
		cache:     make(map[string]*ModelResult),
		mode:      mode,
	}
}

// RegisterProvider adds a provider for specific capabilities.
func (g *Gateway) RegisterProvider(p Provider, caps ...Capability) {
	for _, cap := range caps {
		g.providers[cap] = append(g.providers[cap], p)
	}
}

// Execute runs a model request with retry, caching, and provider routing.
func (g *Gateway) Execute(ctx context.Context, req *ModelRequest) (*ModelResult, error) {
	// Compute fingerprint if not provided
	if req.Fingerprint == "" {
		req.Fingerprint = g.computeFingerprint(req)
	}

	// Check cache for idempotent requests
	g.mu.RLock()
	if cached, ok := g.cache[req.Fingerprint]; ok {
		g.mu.RUnlock()
		cached.Cached = true
		zap.L().Debug("Model call served from cache", zap.String("fp", req.Fingerprint))
		return cached, nil
	}
	g.mu.RUnlock()

	// Get providers for this capability
	providers := g.providers[req.Capability]
	if len(providers) == 0 {
		return nil, &GatewayError{Code: ErrUnavailable, Message: fmt.Sprintf("no provider for capability %s", req.Capability)}
	}

	// Use the first provider that supports this capability
	provider := providers[0]

	// Retry with exponential backoff
	var lastErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			zap.L().Debug("Retrying model call", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := provider.Execute(ctx, req)
		if err == nil {
			// Cache successful result
			g.mu.Lock()
			g.cache[req.Fingerprint] = result
			g.mu.Unlock()
			return result, nil
		}

		lastErr = err
		if gwErr, ok := err.(*GatewayError); ok && !gwErr.Retry {
			break // non-retryable error
		}
	}

	return nil, lastErr
}

func (g *Gateway) computeFingerprint(req *ModelRequest) string {
	data, _ := json.Marshal(struct {
		Capability Capability              `json:"cap"`
		Model      string                  `json:"model"`
		Messages   []Message               `json:"msgs"`
		Parameters map[string]interface{}  `json:"params"`
	}{req.Capability, req.Model, req.Messages, req.Parameters})

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:32]
}

// SetLatency configures the fake provider latency for testing.
func (g *Gateway) SetLatency(ms int64) {
	// This is a no-op for non-fake providers; the fake provider handles its own latency.
}
