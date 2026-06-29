package builtin

import (
	"context"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ContentCheckerTool checks generated content for compliance and quality issues.
type ContentCheckerTool struct {
	cfg config.OpenAIConfig
}

func NewContentCheckerTool(cfg config.OpenAIConfig) *ContentCheckerTool {
	return &ContentCheckerTool{cfg: cfg}
}

func (t *ContentCheckerTool) Name() string { return "content_checker" }
func (t *ContentCheckerTool) Description() string {
	return "Check content for compliance, sensitive content, and quality issues"
}
func (t *ContentCheckerTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *ContentCheckerTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"content": {
				Type:        "string",
				Description: "The body content text to check",
				Required:    false,
			},
			"title": {
				Type:        "string",
				Description: "The title text to check",
				Required:    false,
			},
			"platform": {
				Type:        "string",
				Description: "Target platform name (default: 视频创作平台)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"is_compliant":  {Type: "boolean", Description: "Whether the content passes compliance checks"},
			"quality_score": {Type: "number", Description: "Content quality score (0-100)"},
			"violations":    {Type: "array", Description: "List of violation objects with type, detail, and severity"},
			"suggestions":   {Type: "array", Description: "List of improvement suggestions"},
			"raw_result":    {Type: "string", Description: "Raw LLM response text"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"content": "这是最好的产品", "title": "最佳选择", "platform": "抖音"},
				Output: map[string]interface{}{"is_compliant": false, "quality_score": 75, "violations": []interface{}{map[string]interface{}{"type": "极限词", "detail": "使用'最'字极限词", "severity": "medium"}}},
			},
		},
	}
}

func (t *ContentCheckerTool) ValidateParameters(params map[string]interface{}) bool {
	_, hasContent := params["content"]
	_, hasTitle := params["title"]
	return hasContent || hasTitle
}

func (t *ContentCheckerTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	content, _ := params["content"].(string)
	title, _ := params["title"].(string)
	platform, _ := params["platform"].(string)

	if platform == "" {
		platform = "视频创作平台"
	}

	systemPrompt := fmt.Sprintf(`你是一个内容合规审查助手。请审查以下内容是否适合在"%s"发布。
检查项：
1. 敏感词/违规内容
2. 广告法违规（如"最"、"第一"等极限词）
3. 平台特定规范
4. 内容质量（是否通顺、有价值）

返回纯JSON格式：
{
  "is_compliant": true/false,
  "violations": [{"type": "敏感词/极限词/质量", "detail": "具体问题描述", "severity": "high/medium/low"}],
  "suggestions": ["修改建议1", "修改建议2"],
  "quality_score": 0-100
}`, platform)

	callTool := &LlmApiTool{cfg: t.cfg}
	result := callTool.Execute(ctx, map[string]interface{}{
		"prompt":     systemPrompt + "\n\n---\n\n需要审查的内容：\n标题：" + title + "\n正文：" + content,
		"max_tokens": 1000,
	}, toolCtx)

	if !result.Success {
		return result
	}

	responseContent, _ := result.Data["content"].(string)

	var checkResult map[string]interface{}
	if err := jsonx.ExtractJSON(responseContent, &checkResult); err != nil {
		checkResult = map[string]interface{}{
			"is_compliant":  true,
			"violations":    []interface{}{},
			"suggestions":   []interface{}{},
			"quality_score": 80,
		}
	}

	isCompliant, _ := checkResult["is_compliant"].(bool)
	if !isCompliant {
		return tool.SuccessResult(map[string]interface{}{
			"is_compliant":  false,
			"violations":    checkResult["violations"],
			"suggestions":   checkResult["suggestions"],
			"quality_score": checkResult["quality_score"],
			"raw_result":    responseContent,
		})
	}

	return tool.SuccessResult(map[string]interface{}{
		"is_compliant":  true,
		"quality_score": checkResult["quality_score"],
		"suggestions":   checkResult["suggestions"],
		"raw_result":    responseContent,
	})
}
