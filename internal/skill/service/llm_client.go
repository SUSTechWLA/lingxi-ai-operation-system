package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/common/jsonx"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
)

// LLMClient provides direct HTTP access to the OpenAI-compatible API.
// Used for conversational phases where low latency is critical.
type LLMClient struct {
	cfg        config.OpenAIConfig
	httpClient *http.Client
}

func NewLLMClient(cfg config.OpenAIConfig) *LLMClient {
	return &LLMClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

// Chat sends messages to the LLM and returns the raw content string.
func (c *LLMClient) Chat(ctx context.Context, messages []map[string]string) (string, error) {
	if c.cfg.APIKey == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	requestBody := map[string]interface{}{
		"model":       c.cfg.Model,
		"temperature": c.cfg.Temperature,
		"max_tokens":  c.cfg.MaxTokens,
		"messages":    messages,
	}

	body, _ := json.Marshal(requestBody)

	baseURL := c.cfg.BaseURL
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	choices, ok := responseMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := msg["content"].(string)
	if content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return content, nil
}

// ChatWithJSON calls Chat and unmarshals the response into the target struct.
func (c *LLMClient) ChatWithJSON(ctx context.Context, messages []map[string]string, result interface{}) error {
	content, err := c.Chat(ctx, messages)
	if err != nil {
		return err
	}
	if err := jsonx.ExtractJSON(content, result); err != nil {
		return err
	}
	return nil
}
