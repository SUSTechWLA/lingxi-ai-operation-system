package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ContentGeneratorTool generates full content packages from media analysis results.
type ContentGeneratorTool struct {
	cfg config.OpenAIConfig
}

func NewContentGeneratorTool(cfg config.OpenAIConfig) *ContentGeneratorTool {
	return &ContentGeneratorTool{cfg: cfg}
}

func (t *ContentGeneratorTool) Name() string { return "content_generator" }
func (t *ContentGeneratorTool) Description() string {
	return "Generate full content package (title, description, script, tags) from media analysis"
}
func (t *ContentGeneratorTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *ContentGeneratorTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:         t.Name(),
		Description:  t.Description(),
		Type:         "builtin",
		Sandbox:      false,
		Capabilities: []string{"bid_writing", "content_generation", "text_extraction", "knowledge_retrieval"},
		Parameters: map[string]tool.ParamDef{
			"prompt": {
				Type:        "string",
				Description: "User content creation request (default: 创作一篇内容)",
				Required:    false,
			},
			"platform": {
				Type:        "string",
				Description: "Target platform name (default: 视频创作平台)",
				Required:    false,
			},
			"style": {
				Type:        "string",
				Description: "Writing style (default: 轻松自然)",
				Required:    false,
			},
			"keywords": {
				Type:        "string",
				Description: "Additional keywords as plain text",
				Required:    false,
			},
			"analysis": {
				Type:        "string",
				Description: "JSON string of media analysis result (includes tags and summary)",
				Required:    false,
			},
			"media_ids": {
				Type:        "array",
				Description: "Array of media ID strings from uploaded files",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"content":     {Type: "string", Description: "Raw LLM response text"},
			"package":     {Type: "object", Description: "Parsed content package with title, description, script, tags"},
			"title":       {Type: "string", Description: "Generated title"},
			"description": {Type: "string", Description: "Generated description"},
			"script":      {Type: "string", Description: "Generated content script"},
			"tags":        {Type: "array", Description: "Generated tags"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"prompt": "创作一篇关于咖啡的自媒体内容", "platform": "小红书", "style": "轻松自然"},
				Output: map[string]interface{}{"title": "手冲咖啡入门指南", "description": "今天和大家分享手冲咖啡的基本步骤...", "tags": []interface{}{"咖啡", "手冲", "生活方式"}},
			},
		},
	}
}

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
		platform = "视频创作平台"
	}
	if style == "" {
		style = "轻松自然"
	}
	if prompt == "" {
		prompt = "创作一篇内容"
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

	systemPrompt := fmt.Sprintf(`你是一个专业的智能内容创作助手。目标平台：%s。
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
	if err := jsonx.ExtractJSON(content, &contentPkg); err != nil {
		contentPkg = map[string]interface{}{
			"title":       "AI 生成内容",
			"description": content,
			"tags":        []string{"AI生成"},
		}
	}

	// Build artifact manifest for on-read materialization.
	// Always emit a markdown artifact with the best available readable content.
	artifacts := []map[string]interface{}{}
	scriptText := bestString(contentPkg, "script", "narration", "core_opinion", "logline", "description")
	if scriptText == "" {
		scriptText = content // fallback to raw LLM response
	}
	artifacts = append(artifacts, map[string]interface{}{
		"unitId":   "script-content",
		"kind":     "MARKDOWN",
		"name":     "脚本内容.md",
		"mimeType": "text/markdown",
	})

	// Always emit a publish-copy artifact so the user sees structured metadata.
	artifacts = append(artifacts, map[string]interface{}{
		"unitId":   "publish-copy",
		"kind":     "JSON",
		"name":     "发布文案",
		"mimeType": "application/json",
	})

	return tool.SuccessResult(map[string]interface{}{
		"content":     content,
		"package":     contentPkg,
		"title":       contentPkg["title"],
		"description": contentPkg["description"],
		"script":      scriptText,
		"tags":        contentPkg["tags"],
		"artifacts":   artifacts,
	})
}

// bestString returns the first non-empty string value from the given keys in
// the content package. Used to pick the most useful readable content when the
// LLM response format varies (e.g. different fake fixtures or provider models).
func bestString(pkg map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := pkg[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
