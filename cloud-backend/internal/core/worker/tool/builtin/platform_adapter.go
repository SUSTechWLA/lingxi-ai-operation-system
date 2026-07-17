package builtin

import (
	"context"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// PlatformAdapterTool adapts content for specific social media platforms.
type PlatformAdapterTool struct {
	cfg config.OpenAIConfig
}

func NewPlatformAdapterTool(cfg config.OpenAIConfig) *PlatformAdapterTool {
	return &PlatformAdapterTool{cfg: cfg}
}

func (t *PlatformAdapterTool) Name() string { return "platform_adapter" }
func (t *PlatformAdapterTool) Description() string {
	return "Adapt content for specific social media platform requirements"
}
func (t *PlatformAdapterTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *PlatformAdapterTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"target_platform": {
				Type:        "string",
				Description: "Target platform name (抖音/小红书/微博/B站/公众号/快手/知乎)",
				Required:    true,
				Enum:        []string{"抖音", "小红书", "微博", "B站", "公众号", "快手", "知乎"},
			},
			"source_content": {
				Type:        "string",
				Description: "The content text to adapt",
				Required:    true,
			},
			"title": {
				Type:        "string",
				Description: "Current title (included in prompt context)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"content":             {Type: "string", Description: "Raw LLM response text"},
			"adapted":             {Type: "object", Description: "Parsed adapted content object"},
			"adapted_title":       {Type: "string", Description: "Platform-adapted title"},
			"adapted_description": {Type: "string", Description: "Platform-adapted description"},
			"adapted_tags":        {Type: "array", Description: "Platform-adapted tags"},
			"platform_notes":      {Type: "string", Description: "Adaptation notes"},
			"target_platform":     {Type: "string", Description: "Echoed target platform name"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"target_platform": "小红书", "source_content": "这个产品非常好用", "title": "好物推荐"},
				Output: map[string]interface{}{"adapted_title": "好物推荐｜亲测好用！✨", "adapted_description": "今天给大家分享一款真的超好用的产品...", "adapted_tags": []interface{}{"好物分享", "真实体验"}},
			},
		},
	}
}

func (t *PlatformAdapterTool) ValidateParameters(params map[string]interface{}) bool {
	target, _ := params["target_platform"].(string)
	_, hasContent := params["source_content"]
	return target != "" && hasContent
}

func (t *PlatformAdapterTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	sourceContent, _ := params["source_content"].(string)
	targetPlatform, _ := params["target_platform"].(string)
	title, _ := params["title"].(string)

	if targetPlatform == "" {
		return tool.FailureResult("target_platform is required")
	}

	platformRules := map[string]string{
		"抖音":  "短视频平台，标题15字以内，文案口语化，多用热点话题和互动提问，配热门BGM",
		"小红书": "种草平台，标题20字以内使用Emoji，文案分段清晰，突出真实体验，加相关话题标签",
		"微博":  "社交平台，标题醒目，文案140字内提炼核心，适当使用话题#话题#",
		"B站":  "中长视频平台，标题吸引点击，文案详细有深度，适当玩梗，加时间戳",
		"公众号": "图文平台，标题有信息量，正文结构清晰，适当加粗引用，注重排版",
		"快手":  "短视频平台，标题接地气，文案简单直接，突出真实感和实用性",
		"知乎":  "问答/专栏平台，标题问题导向，正文专业有深度，引用数据来源",
	}

	rules, ok := platformRules[targetPlatform]
	if !ok {
		rules = "视频创作平台平台，保持内容自然流畅"
	}

	callTool := &LlmApiTool{cfg: t.cfg}
	result := callTool.Execute(ctx, map[string]interface{}{
		"prompt": fmt.Sprintf(`你是一个跨平台内容适配专家。请将内容适配到"%s"平台。

平台特点：%s

适配要求：
1. 保持核心信息不变
2. 调整语气和格式匹配平台风格
3. 优化标题吸引目标平台用户
4. 文字长度符合平台习惯

返回纯JSON格式：
{
  "adapted_title": "适配后的标题",
  "adapted_description": "适配后的简介/文案",
  "adapted_tags": ["标签1", "标签2"],
  "platform_notes": "适配说明"
}

---

源内容%s：
标题：%s
%s`, targetPlatform, rules, targetPlatform, title, sourceContent),
		"max_tokens": 1500,
	}, toolCtx)

	if !result.Success {
		return result
	}

	content, _ := result.Data["content"].(string)

	var adapted map[string]interface{}
	if err := jsonx.ExtractJSON(content, &adapted); err != nil {
		adapted = map[string]interface{}{
			"adapted_title":       title,
			"adapted_description": sourceContent,
			"adapted_tags":        []string{},
			"platform_notes":      "直接适配（未解析结构化结果）",
		}
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":             content,
		"adapted":             adapted,
		"adapted_title":       adapted["adapted_title"],
		"adapted_description": adapted["adapted_description"],
		"adapted_tags":        adapted["adapted_tags"],
		"platform_notes":      adapted["platform_notes"],
		"target_platform":     targetPlatform,
	})
}
