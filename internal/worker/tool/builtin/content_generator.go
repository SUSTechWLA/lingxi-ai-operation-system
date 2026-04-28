package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

// ContentGeneratorTool generates full content packages from media analysis results.
type ContentGeneratorTool struct {
	cfg config.OpenAIConfig
}

func NewContentGeneratorTool(cfg config.OpenAIConfig) *ContentGeneratorTool {
	return &ContentGeneratorTool{cfg: cfg}
}

func (t *ContentGeneratorTool) Name() string       { return "content_generator" }
func (t *ContentGeneratorTool) Description() string { return "Generate full content package (title, description, script, tags) from media analysis" }
func (t *ContentGeneratorTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *ContentGeneratorTool) ValidateParameters(params map[string]interface{}) bool {
	_, hasPrompt := params["prompt"]
	_, hasMedia := params["media_ids"]
	return hasPrompt || hasMedia
}

func (t *ContentGeneratorTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	prompt, _ := params["prompt"].(string)
	platform, _ := params["platform"].(string)
	style, _ := params["style"].(string)
	keywords, _ := params["keywords"].(string)

	if platform == "" {
		platform = "通用自媒体"
	}
	if style == "" {
		style = "轻松自然"
	}
	if prompt == "" {
		prompt = "创作一篇自媒体内容"
	}

	analysis, _ := params["analysis"].(string)
	var analysisInfo string
	if analysis != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(analysis), &parsed); err == nil {
			if tags, ok := parsed["tags"]; ok {
				analysisInfo += "素材标签：" + fmt.Sprintf("%v", tags) + "\n"
			}
			if summary, ok := parsed["summary"]; ok {
				analysisInfo += "素材概要：" + fmt.Sprintf("%v", summary) + "\n"
			}
		}
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的自媒体内容创作助手。目标平台：%s。
创作风格：%s。

根据素材分析结果和用户需求，输出完整的内容包（纯JSON格式）：
{
  "title": "吸引眼球的标题（不超过30字）",
  "description": "内容简介（200字以内，突出亮点）",
  "script": "完整的内容脚本/文案",
  "tags": ["标签1", "标签2", "标签3"]
}`, platform, style)

	userPrompt := "用户需求：" + prompt
	if analysisInfo != "" {
		userPrompt += "\n\n素材分析结果：\n" + analysisInfo
	}
	if keywords != "" {
		userPrompt += "\n\n关键词：" + keywords
	}

	callTool := &LlmApiTool{cfg: t.cfg}
	result := callTool.Execute(ctx, map[string]interface{}{
		"prompt":     systemPrompt + "\n\n---\n\n" + userPrompt,
		"max_tokens": 2000,
	}, toolCtx)

	if !result.Success {
		return result
	}

	content, _ := result.Data["content"].(string)

	var contentPkg map[string]interface{}
	if err := json.Unmarshal([]byte(content), &contentPkg); err != nil {
		contentPkg = map[string]interface{}{
			"title":       "AI 生成内容",
			"description": content,
			"tags":        []string{"AI生成"},
		}
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":     content,
		"package":     contentPkg,
		"title":       contentPkg["title"],
		"description": contentPkg["description"],
		"script":      contentPkg["script"],
		"tags":        contentPkg["tags"],
	})
}
