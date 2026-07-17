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
	cache     map[string]cacheEntry // fingerprint → result
	mu        sync.RWMutex
	mode      string // "fake" | "real"
	cacheTTL  time.Duration
	maxCache  int
}

// NewGateway creates a new model gateway.
func NewGateway(mode string) *Gateway {
	return &Gateway{
		providers: make(map[Capability][]Provider),
		cache:     make(map[string]cacheEntry),
		mode:      mode,
		cacheTTL:  5 * time.Minute,
		maxCache:  1024,
	}
}

type cacheEntry struct {
	result    *ModelResult
	expiresAt time.Time
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

	cacheable := req.Test == nil
	if cacheable {
		g.mu.Lock()
		if cached, ok := g.cache[req.Fingerprint]; ok {
			if time.Now().Before(cached.expiresAt) {
				g.mu.Unlock()
				result := cloneModelResult(cached.result)
				result.Cached = true
				zap.L().Debug("Model call served from cache", zap.String("fp", req.Fingerprint))
				return result, nil
			}
			delete(g.cache, req.Fingerprint)
		}
		g.mu.Unlock()
	}

	// Get providers for this capability
	providers := g.providers[req.Capability]
	if len(providers) == 0 {
		return nil, &GatewayError{Code: ErrUnavailable, Message: fmt.Sprintf("no provider for capability %s", req.Capability)}
	}

	// Retry with exponential backoff
	var lastErr error
	maxRetries := 3
	for _, provider := range providers {
		if !provider.Supports(req.Capability) {
			continue
		}
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(1<<uint(attempt-1)) * time.Second
				zap.L().Debug("Retrying model call",
					zap.String("provider", provider.Name()),
					zap.Int("attempt", attempt),
					zap.Duration("backoff", backoff))
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
			}

			result, err := provider.Execute(ctx, req)
			if err == nil {
				if cacheable {
					g.storeCache(req.Fingerprint, result)
				}
				return result, nil
			}

			lastErr = err
			if gwErr, ok := err.(*GatewayError); ok && !gwErr.Retry {
				break // try the next provider, but do not retry this one
			}
		}
	}

	if lastErr == nil {
		lastErr = &GatewayError{Code: ErrUnavailable, Message: fmt.Sprintf("no provider supports capability %s", req.Capability)}
	}
	return nil, lastErr
}

func (g *Gateway) computeFingerprint(req *ModelRequest) string {
	data, _ := json.Marshal(struct {
		Capability Capability             `json:"cap"`
		Model      string                 `json:"model"`
		Messages   []Message              `json:"msgs"`
		Parameters map[string]interface{} `json:"params"`
		Test       *TestConfig            `json:"test,omitempty"`
	}{req.Capability, req.Model, req.Messages, req.Parameters, req.Test})

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:32]
}

func (g *Gateway) storeCache(fingerprint string, result *ModelResult) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.evictExpiredLocked(time.Now())
	if g.maxCache > 0 && len(g.cache) >= g.maxCache {
		for key := range g.cache {
			delete(g.cache, key)
			break
		}
	}
	g.cache[fingerprint] = cacheEntry{
		result:    cloneModelResult(result),
		expiresAt: time.Now().Add(g.cacheTTL),
	}
}

func (g *Gateway) evictExpiredLocked(now time.Time) {
	for key, entry := range g.cache {
		if !now.Before(entry.expiresAt) {
			delete(g.cache, key)
		}
	}
}

func cloneModelResult(in *ModelResult) *ModelResult {
	if in == nil {
		return nil
	}
	out := *in
	if in.Images != nil {
		out.Images = append([]ImageOutput(nil), in.Images...)
	}
	return &out
}

func (g *Gateway) SetCacheTTL(ttl time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cacheTTL = ttl
}

func (g *Gateway) SetMaxCacheEntries(maxEntries int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.maxCache = maxEntries
}

// SetLatency configures the fake provider latency for testing.
func (g *Gateway) SetLatency(ms int64) {
	// This is a no-op for non-fake providers; the fake provider handles its own latency.
}
