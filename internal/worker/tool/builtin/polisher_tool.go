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

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type PolisherTool struct {
	cfg config.OpenAIConfig
}

func NewPolisherTool(cfg config.OpenAIConfig) *PolisherTool {
	return &PolisherTool{cfg: cfg}
}

func (t *PolisherTool) Name() string { return "polisher" }
func (t *PolisherTool) Description() string {
	return "Polish text (title or description) for social media publishing"
}
func (t *PolisherTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *PolisherTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	text, _ := params["text"].(string)
	if text == "" {
		return tool.FailureResult("text is required")
	}

	polishType, _ := params["polishType"].(string)
	if polishType == "" {
		polishType = "description"
	}

	zap.L().Info("PolisherTool executing",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("polishType", polishType),
		zap.Int("textLength", len(text)),
	)

	systemPrompt := fmt.Sprintf(`你是一个自媒体内容润色专家。请润色以下%s，使其更加吸引人、专业。
要求：
1. 保持原意不变
2. 使表达更加流畅、吸引人
3. 直接返回润色后的文本，不要加引号或额外格式`, polishType)

	return t.callOpenAI(ctx, systemPrompt, text, polishType, toolCtx)
}

func (t *PolisherTool) callOpenAI(ctx context.Context, systemPrompt, userPrompt, polishType string, toolCtx tool.ToolContext) tool.ToolResult {
	if t.cfg.APIKey == "" {
		return tool.FailureResult("API key is not configured")
	}

	requestBody := map[string]interface{}{
		"model":       t.cfg.Model,
		"temperature": t.cfg.Temperature,
		"max_tokens":  t.cfg.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
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

	choices, ok := responseMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return tool.FailureResult("No choices in LLM response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return tool.FailureResult("Invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return tool.FailureResult("Invalid message format")
	}

	content, _ := message["content"].(string)
	if content == "" {
		return tool.FailureResult("Empty response from LLM")
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":    content,
		"polishType": polishType,
		"model":      t.cfg.Model,
	})
}

func (t *PolisherTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["text"].(string)
	return ok
}
