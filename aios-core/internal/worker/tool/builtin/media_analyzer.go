package builtin

import (
	"context"

	"github.com/tangying-ai/aios-core/internal/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/config"
	"github.com/tangying-ai/aios-core/internal/worker/tool"
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
func (t *MediaAnalyzerTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"media_ids": {
				Type:        "array",
				Description: "Array of media ID strings from uploaded files",
				Required:    false,
			},
			"file_names": {
				Type:        "array",
				Description: "Array of filename strings (paired by index with media_ids)",
				Required:    false,
			},
			"prompt": {
				Type:        "string",
				Description: "Custom analysis instructions (default: 分析这批多媒体素材，生成相关标签和内容创作建议)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"content":     {Type: "string", Description: "Raw LLM response text"},
			"analysis":    {Type: "object", Description: "Parsed analysis with tags, suggestions, summary"},
			"tags":        {Type: "array", Description: "Generated tags (3-5 Chinese tags)"},
			"suggestions": {Type: "array", Description: "Content creation suggestions"},
			"summary":     {Type: "string", Description: "One-sentence summary of media features"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"media_ids": []interface{}{"media_001", "media_002"}, "file_names": []interface{}{"photo1.jpg", "photo2.jpg"}},
				Output: map[string]interface{}{"tags": []interface{}{"美食", "烘焙", "甜品"}, "suggestions": []interface{}{"制作烘焙教程", "分享甜点故事"}},
			},
		},
	}
}

func (t *MediaAnalyzerTool) ValidateParameters(params map[string]interface{}) bool {
	_, hasMediaIDs := params["media_ids"]
	_, hasNames := params["file_names"]
	return hasMediaIDs || hasNames
}

func (t *MediaAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	mediaIDs, _ := params["media_ids"].([]interface{})
	fileNames, _ := params["file_names"].([]interface{})
	prompt := "分析这批多媒体素材，生成相关标签（3-5个中文标签）和内容创作建议"

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

	systemPrompt := `你是一个专业的多媒体素材分析助手。分析用户提供的素材，输出JSON格式的分析结果。
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
	if err := jsonx.ExtractJSON(content, &analysis); err != nil {
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
