package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

// Config is the effective OpenAI-compatible provider configuration.
type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
}

// ConfigResolver returns the latest provider configuration.
type ConfigResolver func() Config

// Provider implements modelgateway.Provider for OpenAI Images API (DALL-E).
type Provider struct {
	apiKey         string
	baseURL        string
	configResolver ConfigResolver
	client         *http.Client
}

// NewProvider creates a new OpenAI image generation provider.
// Reads OPENAI_API_KEY and OPENAI_BASE_URL from environment.
func NewProvider() *Provider {
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return NewProviderWithConfig(Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: baseURL,
		Model:   os.Getenv("OPENAI_MODEL"),
	})
}

// NewProviderWithConfig creates a provider from a fixed configuration.
func NewProviderWithConfig(cfg Config) *Provider {
	return NewProviderWithConfigResolver(func() Config {
		return cfg
	})
}

// NewProviderWithConfigResolver creates a provider that resolves configuration
// for every request, allowing runtime model-provider updates without restart.
func NewProviderWithConfigResolver(resolver ConfigResolver) *Provider {
	if resolver == nil {
		resolver = func() Config { return Config{} }
	}
	cfg := resolver()
	return &Provider{
		apiKey:         cfg.APIKey,
		baseURL:        cfg.BaseURL,
		configResolver: resolver,
		client:         &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *Provider) Name() string { return "openai" }

func (p *Provider) Supports(cap modelgateway.Capability) bool {
	return cap == modelgateway.CapTextToImage || cap == modelgateway.CapTextToText
}

func (p *Provider) Health(ctx context.Context) error {
	cfg := p.effectiveConfig()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIEndpoint(cfg.BaseURL, "/models"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("openai health check returned %d", resp.StatusCode)
	}
	return nil
}

type imageRequestBody struct {
	Prompt  string `json:"prompt"`
	Model   string `json:"model,omitempty"`
	Size    string `json:"size,omitempty"`
	N       int    `json:"n,omitempty"`
	Quality string `json:"quality,omitempty"`
	Style   string `json:"style,omitempty"`
}

type imageResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Provider) Execute(ctx context.Context, req *modelgateway.ModelRequest) (*modelgateway.ModelResult, error) {
	cfg := p.effectiveConfig()
	if cfg.APIKey == "" {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrAuthFailed,
			Message: "OPENAI_API_KEY is not set",
			Retry:   false,
		}
	}

	if req.Capability == modelgateway.CapTextToText {
		return p.executeChatCompletion(ctx, req, cfg)
	}

	return p.executeImageGeneration(ctx, req, cfg)
}

func (p *Provider) executeChatCompletion(ctx context.Context, req *modelgateway.ModelRequest, cfg Config) (*modelgateway.ModelResult, error) {
	defaultModel := cfg.Model
	if defaultModel == "" {
		defaultModel = "gpt-4"
	}
	model := req.Model
	if model == "" {
		model = stringParam(req.Parameters, "model", defaultModel)
	}
	defaultTemperature := cfg.Temperature
	if defaultTemperature == 0 {
		defaultTemperature = 0.2
	}
	defaultMaxTokens := cfg.MaxTokens
	if defaultMaxTokens == 0 {
		defaultMaxTokens = 2000
	}
	temperature := floatParam(req.Parameters, "temperature", defaultTemperature)
	maxTokens := intParam(req.Parameters, "max_tokens", defaultMaxTokens)

	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	messages := make([]chatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, chatMessage{Role: m.Role, Content: m.Content})
	}

	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	// Pass response_format if specified (DeepSeek/OpenAI JSON mode)
	if rf, ok := req.Parameters["response_format"]; ok {
		body["response_format"] = rf
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("failed to marshal chat request: %v", err),
			Retry:   false,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint(cfg.BaseURL, "/chat/completions"), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("failed to create chat request: %v", err),
			Retry:   true,
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrTimeout,
			Message: fmt.Sprintf("chat request failed: %v", err),
			Retry:   true,
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrAuthFailed,
			Message: fmt.Sprintf("openai returned %d: %s", resp.StatusCode, string(respBody)),
			Retry:   false,
		}
	}
	if resp.StatusCode == 429 {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrRateLimited,
			Message: "openai rate limited",
			Retry:   true,
		}
	}
	if resp.StatusCode >= 500 {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("openai server error %d", resp.StatusCode),
			Retry:   true,
		}
	}
	if resp.StatusCode != 200 {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("openai returned %d: %s", resp.StatusCode, string(respBody)),
			Retry:   false,
		}
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("failed to decode chat response: %v", err),
			Retry:   false,
		}
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrContentRejected,
			Message: "openai returned empty content",
			Retry:   false,
		}
	}

	usage := modelgateway.Usage{
		Model:      model,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if decoded.Usage != nil {
		usage.PromptTokens = decoded.Usage.PromptTokens
		usage.OutputTokens = decoded.Usage.CompletionTokens
	}

	return &modelgateway.ModelResult{
		Content: decoded.Choices[0].Message.Content,
		Usage:   usage,
	}, nil
}

func (p *Provider) executeImageGeneration(ctx context.Context, req *modelgateway.ModelRequest, cfg Config) (*modelgateway.ModelResult, error) {
	prompt, _ := req.Parameters["prompt"].(string)
	if prompt == "" {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: "prompt is required",
			Retry:   false,
		}
	}

	// Build request body from parameters
	body := imageRequestBody{
		Prompt:  prompt,
		Model:   stringParam(req.Parameters, "model", "dall-e-3"),
		Size:    stringParam(req.Parameters, "size", "1024x1024"),
		N:       intParam(req.Parameters, "n", 1),
		Quality: stringParam(req.Parameters, "quality", "standard"),
	}
	if style, ok := req.Parameters["style"].(string); ok && style != "" {
		body.Style = style
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
			Retry:   false,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint(cfg.BaseURL, "/images/generations"), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("failed to create request: %v", err),
			Retry:   true,
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrTimeout,
			Message: fmt.Sprintf("request failed: %v", err),
			Retry:   true,
		}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("failed to read response: %v", err),
			Retry:   true,
		}
	}

	if resp.StatusCode >= 400 {
		return nil, classifyError(resp.StatusCode, string(respBytes))
	}

	var result imageResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrSchemaInvalid,
			Message: fmt.Sprintf("failed to parse response: %v", err),
			Retry:   false,
		}
	}

	if result.Error != nil {
		return nil, &modelgateway.GatewayError{
			Code:    modelgateway.ErrContentRejected,
			Message: result.Error.Message,
			Retry:   false,
		}
	}

	images := make([]modelgateway.ImageOutput, 0, len(result.Data))
	for _, d := range result.Data {
		img := modelgateway.ImageOutput{}
		if d.URL != "" {
			img.URL = d.URL
		}
		if d.B64JSON != "" {
			img.Data = d.B64JSON
			img.Format = "png"
		}
		images = append(images, img)
	}

	return &modelgateway.ModelResult{
		Content: string(respBytes),
		Images:  images,
		Usage: modelgateway.Usage{
			Model:      body.Model,
			DurationMs: 0,
		},
	}, nil
}

func (p *Provider) effectiveConfig() Config {
	cfg := Config{}
	if p.configResolver != nil {
		cfg = p.configResolver()
	}
	if cfg.APIKey == "" {
		cfg.APIKey = p.apiKey
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = p.baseURL
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return cfg
}

func openAIEndpoint(baseURL, resourcePath string) string {
	base := strings.TrimRight(baseURL, "/")
	return base + resourcePath
}

func classifyError(statusCode int, body string) *modelgateway.GatewayError {
	switch statusCode {
	case 401, 403:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrAuthFailed,
			Message: fmt.Sprintf("authentication failed (%d): %s", statusCode, body),
			Retry:   false,
		}
	case 429:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrRateLimited,
			Message: fmt.Sprintf("rate limited (%d): %s", statusCode, body),
			Retry:   true,
		}
	case 400, 422:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrInvalidRequest,
			Message: fmt.Sprintf("invalid request (%d): %s", statusCode, body),
			Retry:   false,
		}
	default:
		return &modelgateway.GatewayError{
			Code:    modelgateway.ErrUnavailable,
			Message: fmt.Sprintf("server error (%d): %s", statusCode, body),
			Retry:   true,
		}
	}
}

func stringParam(params map[string]interface{}, key, defaultVal string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

func intParam(params map[string]interface{}, key string, defaultVal int) int {
	switch v := params[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		// ignore; use default
	}
	return defaultVal
}

func floatParam(params map[string]interface{}, key string, defaultVal float64) float64 {
	switch v := params[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return defaultVal
}
