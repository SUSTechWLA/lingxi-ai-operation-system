package builtin

import (
	"context"
	"encoding/json"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

// MediaAnalyzerTool analyzes uploaded media and generates descriptive tags and content suggestions.
type MediaAnalyzerTool struct {
	cfg config.OpenAIConfig
}

func NewMediaAnalyzerTool(cfg config.OpenAIConfig) *MediaAnalyzerTool {
	return &MediaAnalyzerTool{cfg: cfg}
}

func (t *MediaAnalyzerTool) Name() string        { return "media_analyzer" }
func (t *MediaAnalyzerTool) Description() string  { return "Analyze media files (images/videos) and generate tags, descriptions, and content suggestions" }
func (t *MediaAnalyzerTool) Type() tool.ToolType  { return tool.ToolTypeCustom }
func (t *MediaAnalyzerTool) ValidateParameters(params map[string]interface{}) bool {
	_, hasMediaIDs := params["media_ids"]
	_, hasNames := params["file_names"]
	return hasMediaIDs || hasNames
}

func (t *MediaAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	mediaIDs, _ := params["media_ids"].([]interface{})
	fileNames, _ := params["file_names"].([]interface{})
	prompt := "分析这批自媒体素材，生成相关标签（3-5个中文标签）和内容创作建议"

	if p, ok := params["prompt"].(string); ok && p != "" {
		prompt = p
	}

	var mediaDesc string
	for i, id := range mediaIDs {
		name := ""
		if i < len(fileNames) {
			name, _ = fileNames[i].(string)
		}
		mediaDesc += "- " + name + " (ID: " + id.(string) + ")\n"
	}

	systemPrompt := `你是一个专业的自媒体素材分析助手。分析用户提供的素材，输出JSON格式的分析结果。
字段说明：
- tags: 3-5个中文标签，概括素材主题
- suggestions: 2-3个内容创作方向建议
- summary: 一句话总结素材特点

返回纯JSON，不要包含markdown格式。`

	userPrompt := "素材列表：\n" + mediaDesc + "\n\n额外说明：" + prompt

	if len(mediaIDs) == 0 && len(fileNames) == 0 {
		userPrompt = "用户提供的素材描述：" + prompt
	}

	callTool := &LlmApiTool{cfg: t.cfg}
	result := callTool.Execute(ctx, map[string]interface{}{
		"prompt":    systemPrompt + "\n\n---\n\n" + userPrompt,
		"max_tokens": 800,
	}, toolCtx)

	if !result.Success {
		return result
	}

	content, _ := result.Data["content"].(string)

	var analysis map[string]interface{}
	if err := json.Unmarshal([]byte(content), &analysis); err != nil {
		analysis = map[string]interface{}{
			"tags":        []string{"素材"},
			"suggestions": []string{"基于素材创作相关内容"},
			"summary":     content,
		}
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":     content,
		"analysis":    analysis,
		"tags":        analysis["tags"],
		"suggestions": analysis["suggestions"],
		"summary":     analysis["summary"],
	})
}
