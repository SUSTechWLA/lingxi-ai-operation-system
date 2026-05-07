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

	"github.com/tangying-ai/tangying-ai-operation-system/internal/common/llmutil"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/config"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/worker/tool"
)

type ChatGenerateTool struct {
	cfg config.OpenAIConfig
}

func NewChatGenerateTool(cfg config.OpenAIConfig) *ChatGenerateTool {
	return &ChatGenerateTool{cfg: cfg}
}

func (t *ChatGenerateTool) Name() string        { return "chat_generate" }
func (t *ChatGenerateTool) Description() string  { return "Conversational content generation with full message history support" }
func (t *ChatGenerateTool) Type() tool.ToolType  { return tool.ToolTypeCustom }

func (t *ChatGenerateTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	messagesRaw, ok := params["messages"].([]interface{})
	if !ok || len(messagesRaw) == 0 {
		return tool.FailureResult("messages array is required")
	}

	messages := make([]map[string]interface{}, 0, len(messagesRaw))
	for _, m := range messagesRaw {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "" {
			continue
		}
		entry := map[string]interface{}{"role": role}
		// Preserve content as-is: string for text, array for multimodal
		if content, exists := msg["content"]; exists {
			entry["content"] = content
		}
		messages = append(messages, entry)
	}

	if len(messages) == 0 {
		return tool.FailureResult("no valid messages found in messages array")
	}

	// Inject image_urls into the last user message if provided
	if imageURLs, ok := params["image_urls"].([]interface{}); ok && len(imageURLs) > 0 {
		var urls []string
		for _, u := range imageURLs {
			if s, ok := u.(string); ok && s != "" {
				urls = append(urls, s)
			}
		}
		if len(urls) > 0 {
			// Find last user message and rebuild it as multimodal
			for i := len(messages) - 1; i >= 0; i-- {
				if role, _ := messages[i]["role"].(string); role == "user" {
					text := ""
					switch c := messages[i]["content"].(type) {
					case string:
						text = c
					case []interface{}:
						for _, part := range c {
							if p, ok := part.(map[string]interface{}); ok {
								if t, ok := p["text"].(string); ok {
									text += t
								}
							}
						}
					}
					messages[i] = llmutil.BuildUserMessage(text, urls)
					break
				}
			}
		}
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

	zap.L().Info("ChatGenerateTool executing",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("model", model),
		zap.Int("messageCount", len(messages)))

	requestBody := map[string]interface{}{
		"model":       model,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"messages":    messages,
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

	return tool.SuccessResult(map[string]interface{}{
		"content": content,
		"model":   model,
	})
}

func (t *ChatGenerateTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"messages": {
				Type:        "array",
				Description: "Array of message objects with 'role' (system/user/assistant) and 'content' fields",
				Required:    true,
			},
			"model": {
				Type:        "string",
				Description: "Model override (default: configured model)",
				Required:    false,
			},
			"image_urls": {
				Type:        "array",
				Description: "Image URLs or base64 data URLs for multimodal vision requests",
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
		},
		Output: map[string]tool.ParamDef{
			"content": {Type: "string", Description: "Generated text response from the LLM"},
			"model":   {Type: "string", Description: "Model name used for generation"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"messages": []interface{}{map[string]interface{}{"role": "user", "content": "写一段产品介绍"}}},
				Output: map[string]interface{}{"content": "这是一款革命性的产品...", "model": "gpt-4"},
			},
		},
	}
}

func (t *ChatGenerateTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["messages"].([]interface{})
	return ok
}
