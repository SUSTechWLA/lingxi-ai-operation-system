package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/llmutil"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type LlmApiTool struct {
	cfg config.OpenAIConfig
}

func NewLlmApiTool(cfg config.OpenAIConfig) *LlmApiTool {
	return &LlmApiTool{cfg: cfg}
}

func (t *LlmApiTool) Name() string        { return "llm_api" }
func (t *LlmApiTool) Description() string { return "Call LLM API for chat completions" }
func (t *LlmApiTool) Type() tool.ToolType { return tool.ToolTypeLLM }

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

	// Collect image URLs for multimodal requests
	var imageURLs []string
	if urls, ok := params["image_urls"].([]interface{}); ok {
		for _, u := range urls {
			if s, ok := u.(string); ok && s != "" {
				imageURLs = append(imageURLs, s)
			}
		}
	}

	msg := llmutil.BuildUserMessage(prompt, imageURLs)
	requestBody := map[string]interface{}{
		"model":       model,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"messages":    []interface{}{msg},
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

	if resp.StatusCode != http.StatusOK {
		return tool.FailureResult(fmt.Sprintf("LLM API returned status %d: %s", resp.StatusCode, string(respBody)))
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return tool.FailureResult("Failed to parse LLM response: " + err.Error())
	}

	content := extractContent(responseMap)
	finishReason := extractFinishReason(responseMap)

	return tool.SuccessResult(map[string]interface{}{
		"content":      content,
		"model":        model,
		"finishReason": finishReason,
		"rawResponse":  responseMap,
	})
}

func (t *LlmApiTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"prompt": {
				Type:        "string",
				Description: "Primary prompt text (one of prompt/message/content is required)",
				Required:    false,
			},
			"message": {
				Type:        "string",
				Description: "Alternate prompt text (used if prompt is empty)",
				Required:    false,
			},
			"content": {
				Type:        "string",
				Description: "Alternate prompt text (used if prompt and message are empty)",
				Required:    false,
			},
			"model": {
				Type:        "string",
				Description: "Model override (default: configured model)",
				Required:    false,
			},
			"max_tokens": {
				Type:        "number",
				Description: "Maximum tokens in response (default: configured value)",
				Required:    false,
			},
			"temperature": {
				Type:        "number",
				Description: "Temperature for generation (default: configured value)",
				Required:    false,
			},
			"image_urls": {
				Type:        "array",
				Description: "Base64 data URLs or HTTP URLs of images for multimodal vision (optional)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"content":     {Type: "string", Description: "Generated text response from the LLM"},
			"model":       {Type: "string", Description: "Model name used for generation"},
			"rawResponse": {Type: "object", Description: "Full API response object"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"prompt": "写一首关于春天的诗"},
				Output: map[string]interface{}{"content": "春风拂面来，花开满园香……", "model": "gpt-4"},
			},
		},
	}
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

	// Standard chat models return content. Reasoning models (e.g. deepseek-v4-pro)
	// return reasoning_content and may leave content empty.
	if content, _ := message["content"].(string); content != "" {
		return content
	}
	if reasoning, _ := message["reasoning_content"].(string); reasoning != "" {
		return reasoning
	}
	return ""
}

func extractFinishReason(response map[string]interface{}) string {
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}

	reason, _ := choice["finish_reason"].(string)
	return reason
}
