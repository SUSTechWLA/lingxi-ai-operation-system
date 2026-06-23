package hyperframes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the HyperFrames Render Service.
// It replaces the previous approach of shelling out to npx/CLI.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new HyperFrames Render Service client.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// NewClientFromConfig creates a Client from a Config.
func NewClientFromConfig(cfg Config) *Client {
	return NewClient(cfg.ServiceURL, cfg.Timeout())
}

// Health checks the render service readiness.
func (c *Client) Health(ctx context.Context) (*HealthResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("hyperframes health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyperframes health check failed: %w", err)
	}
	defer resp.Body.Close()

	var result HealthResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hyperframes health decode: %w", err)
	}

	if resp.StatusCode >= 300 || !result.OK {
		return nil, fmt.Errorf("hyperframes health check returned status=%d", resp.StatusCode)
	}

	return &result, nil
}

// Render submits a render job and blocks until completion.
func (c *Client) Render(ctx context.Context, reqBody RenderRequest) (*RenderResult, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("hyperframes render marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/render",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("hyperframes render request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyperframes render service unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result RenderResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hyperframes render decode: %w", err)
	}

	if resp.StatusCode >= 300 || !result.OK {
		if result.Error != "" {
			return &result, fmt.Errorf("hyperframes render failed: %s", result.Error)
		}
		return &result, fmt.Errorf("hyperframes render failed: status=%d", resp.StatusCode)
	}

	return &result, nil
}

// Lint validates a HyperFrames project.
func (c *Client) Lint(ctx context.Context, reqBody LintRequest) (*LintResult, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("hyperframes lint marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/lint",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("hyperframes lint request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyperframes lint service unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result LintResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hyperframes lint decode: %w", err)
	}

	return &result, nil
}
