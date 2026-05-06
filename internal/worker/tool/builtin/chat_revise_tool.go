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

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/common/jsonx"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/common/llmutil"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type ChatReviseTool struct {
	cfg config.OpenAIConfig
}

func NewChatReviseTool(cfg config.OpenAIConfig) *ChatReviseTool {
	return &ChatReviseTool{cfg: cfg}
}

func (t *ChatReviseTool) Name() string        { return "chat_revise" }
func (t *ChatReviseTool) Description() string  { return "Revise or generate content fields (title, description, keywords) based on natural language instructions" }
func (t *ChatReviseTool) Type() tool.ToolType  { return tool.ToolTypeCustom }

func (t *ChatReviseTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	message, _ := params["message"].(string)
	if message == "" {
		return tool.FailureResult("message is required")
	}

	title, _ := params["title"].(string)
	description, _ := params["description"].(string)

	var keywords []string
	if kw, ok := params["keywords"].([]interface{}); ok {
		for _, k := range kw {
			if s, ok := k.(string); ok {
				keywords = append(keywords, s)
			}
		}
	} else if kw, ok := params["keywords"].([]string); ok {
		keywords = kw
	}

	// Extract optional media context
	var mediaCount int
	var mediaNames []string
	if mc, ok := params["media_count"]; ok {
		switch v := mc.(type) {
		case float64:
			mediaCount = int(v)
		case int:
			mediaCount = v
		}
	}
	if mn, ok := params["media_names"].([]interface{}); ok {
		for _, n := range mn {
			if s, ok := n.(string); ok {
				mediaNames = append(mediaNames, s)
			}
		}
	}

	// Extract image URLs for multimodal requests
	var imageURLs []string
	if urls, ok := params["image_urls"].([]interface{}); ok {
		for _, u := range urls {
			if s, ok := u.(string); ok && s != "" {
				imageURLs = append(imageURLs, s)
			}
		}
	}

	zap.L().Info("ChatReviseTool executing",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("message", message),
		zap.Int("mediaCount", mediaCount),
	)

	// Build context block with current content and optional media info
	mediaBlock := ""
	if mediaCount > 0 {
		mediaBlock = fmt.Sprintf("\n用户已上传 %d 个素材文件（图片/视频）。", mediaCount)
		if len(mediaNames) > 0 {
			mediaBlock += fmt.Sprintf(" 文件名: %v", mediaNames)
		}
		mediaBlock += "\n当用户要求生成内容时，请根据素材信息直接创作，不要要求用户提供更多素材。"
	}

	currentContent := fmt.Sprintf(`当前内容：
标题：%s
简介：%s
关键词：%v%s`, title, description, keywords, mediaBlock)

	systemPrompt := `你是一个内容创作和修改助手。根据用户的要求进行内容操作：

1. 如果用户要求生成新内容（如"生成标题和简介"、"生成标题"等），请创建全新的标题、简介和关键词
2. 如果用户要求修改现有内容，只修改指定的字段，其他字段保持不变
3. 当用户已上传素材时，基于素材信息合理创作，不要要求用户提供更多素材

## 回答格式
{
  "type": "revise",
  "reply": "简要说明操作结果",
  "fields": {
    "title": "标题（如果没生成或修改则留空）",
    "description": "简介（如果没生成或修改则留空）",
    "keywords": ["关键词（如果没生成或修改则留空数组）"]
  }
}

请用中文回复。

` + currentContent

	userMessage := fmt.Sprintf("用户要求：%s", message)

	reply, err := t.callOpenAI(ctx, systemPrompt, userMessage, imageURLs)
	if err != nil {
		return tool.FailureResult("OpenAI call failed: " + err.Error())
	}

	var parsed struct {
		Type   string `json:"type"`
		Reply  string `json:"reply"`
		Fields struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Keywords    []string `json:"keywords"`
		} `json:"fields"`
	}

	result := map[string]interface{}{
		"reply":       reply,
		"title":       "",
		"description": "",
		"keywords":    []string{},
	}

	if err := jsonx.ExtractJSON(reply, &parsed); err == nil && (parsed.Type == "revise" || parsed.Type == "generate") {
		result["reply"] = parsed.Reply
		result["title"] = parsed.Fields.Title
		result["description"] = parsed.Fields.Description
		result["keywords"] = parsed.Fields.Keywords
	}

	return tool.SuccessResult(result)
}

func (t *ChatReviseTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"message": {
				Type:        "string",
				Description: "User's natural language instruction (e.g., '生成一个标题', '修改简介')",
				Required:    true,
			},
			"title": {
				Type:        "string",
				Description: "Current title (passed as context for revision)",
				Required:    false,
			},
			"description": {
				Type:        "string",
				Description: "Current description (passed as context for revision)",
				Required:    false,
			},
			"keywords": {
				Type:        "array",
				Description: "Current keywords list (passed as context for revision)",
				Required:    false,
			},
			"image_urls": {
				Type:        "array",
				Description: "Image URLs or base64 data URLs for multimodal vision requests",
				Required:    false,
			},
			"media_count": {
				Type:        "number",
				Description: "Number of uploaded media files",
				Required:    false,
			},
			"media_names": {
				Type:        "array",
				Description: "Filenames of uploaded media",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"reply":       {Type: "string", Description: "Brief explanation of what was modified (Chinese)"},
			"title":       {Type: "string", Description: "Revised or generated title (empty if unchanged)"},
			"description": {Type: "string", Description: "Revised or generated description (empty if unchanged)"},
			"keywords":    {Type: "array", Description: "Revised or generated keywords (empty if unchanged)"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"message": "生成一个吸引人的标题", "title": "", "description": "这是一篇关于咖啡的文章"},
				Output: map[string]interface{}{"reply": "已为您生成标题", "title": "手冲咖啡的秘密，99%的人都不知道", "description": "", "keywords": []interface{}{}},
			},
		},
	}
}

func (t *ChatReviseTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["message"].(string)
	return ok
}

func (t *ChatReviseTool) callOpenAI(ctx context.Context, systemPrompt, userPrompt string, imageURLs []string) (string, error) {
	if t.cfg.APIKey == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	userMsg := llmutil.BuildUserMessage(userPrompt, imageURLs)
	requestBody := map[string]interface{}{
		"model":       t.cfg.Model,
		"temperature": t.cfg.Temperature,
		"max_tokens":  t.cfg.MaxTokens,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			userMsg,
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
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)

	client := &http.Client{Timeout: time.Duration(t.cfg.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

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

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := message["content"].(string)
	if content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return content, nil
}
