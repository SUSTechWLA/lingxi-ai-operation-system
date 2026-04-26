package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type LlmApiTool struct {
	cfg config.OpenAIConfig
}

func NewLlmApiTool(cfg config.OpenAIConfig) *LlmApiTool {
	return &LlmApiTool{cfg: cfg}
}

func (t *LlmApiTool) Name() string                  { return "llm_api" }
func (t *LlmApiTool) Description() string            { return "Call LLM API for chat completions" }
func (t *LlmApiTool) Type() tool.ToolType            { return tool.ToolTypeLLM }

func (t *LlmApiTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		prompt, _ = params["message"].(string)
	}
	if prompt == "" {
		prompt, _ = params["content"].(string)
	}
	if prompt == "" {
		return tool.FailureResult("Prompt is required (provide 'prompt', 'message', or 'content' field)")
	}

	if t.cfg.APIKey == "" {
		return tool.FailureResult("OpenAI API key not configured")
	}

	model := t.cfg.Model
	if m, ok := params["model"].(string); ok && m != "" {
		model = m
	}

	maxTokens := t.cfg.MaxTokens
	if mt, ok := params["max_tokens"]; ok {
		if f, ok := mt.(float64); ok {
			maxTokens = int(f)
		}
	}

	temperature := t.cfg.Temperature
	if temp, ok := params["temperature"]; ok {
		if f, ok := temp.(float64); ok {
			temperature = f
		}
	}

	zap.L().Info("Calling LLM API", zap.String("taskId", toolCtx.TaskID), zap.String("model", model))

	requestBody := map[string]interface{}{
		"model":       model,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, _ := json.Marshal(requestBody)

	baseURL := t.cfg.BaseURL
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return tool.FailureResult("Failed to create request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)

	client := &http.Client{Timeout: time.Duration(t.cfg.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return tool.FailureResult("LLM API call failed: " + err.Error())
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return tool.FailureResult("Failed to parse LLM response: " + err.Error())
	}

	content := extractContent(responseMap)

	return tool.SuccessResult(map[string]interface{}{
		"content":     content,
		"model":       model,
		"rawResponse": responseMap,
	})
}

func (t *LlmApiTool) ValidateParameters(params map[string]interface{}) bool {
	if _, ok := params["prompt"].(string); ok {
		return true
	}
	if _, ok := params["message"].(string); ok {
		return true
	}
	if _, ok := params["content"].(string); ok {
		return true
	}
	return false
}

func extractContent(response map[string]interface{}) string {
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return ""
	}

	content, _ := message["content"].(string)
	return content
}
