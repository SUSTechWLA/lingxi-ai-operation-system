package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

const projectContextReportSystemPrompt = `你是一个专业的标书撰写顾问。请根据《招标文件解析报告》和用户提供的项目背景信息回答，生成结构化的《项目背景信息确认表》。

报告必须覆盖以下章节：
1. 项目所在地
2. 气候条件
3. 交通条件
4. 水文条件
5. 地质、地形、土地条件
6. 周边敏感点
7. 施工或服务环境限制
8. 投标单位自身优势
9. 重点强调内容
10. 应避免内容

每个章节必须包含：
- 用户确认内容：来自用户直接回答的信息
- 系统推断内容：根据招标文件和项目类型合理推断的信息
- 待确认事项：用户未提供且无法推断的信息

要求：
- 信息不足时标注"待用户确认"，不得编造。
- 系统推断内容必须标注依据（例如"根据招标文件第X条推断"）。
- 使用 Markdown 格式，便于后续大纲生成引用。`

// GenerateProjectContextReportRequest is the input for generating the confirmation table.
type GenerateProjectContextReportRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextAnswers     string `json:"contextAnswers"`
	ContextReportPath  string `json:"contextReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

// GenerateProjectContextReportResponse is the output.
type GenerateProjectContextReportResponse struct {
	AnalysisReportPath string                 `json:"analysisReportPath"`
	ContextReportPath  string                 `json:"contextReportPath"`
	Content            string                 `json:"content"`
	Artifact           map[string]interface{} `json:"artifact"`
}

// GenerateProjectContextReport generates the project context confirmation table.
func GenerateProjectContextReport(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateProjectContextReportRequest,
) (*GenerateProjectContextReportResponse, error) {
	analysisReportPath := strings.TrimSpace(req.AnalysisReportPath)
	contextAnswers := strings.TrimSpace(req.ContextAnswers)
	contextReportPath := strings.TrimSpace(req.ContextReportPath)
	sourceFile := strings.TrimSpace(req.SourceFile)

	if analysisReportPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}
	if contextAnswers == "" {
		return nil, fmt.Errorf("contextAnswers is required")
	}
	if contextReportPath == "" {
		return nil, fmt.Errorf("contextReportPath is required")
	}
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	reportBytes, err := os.ReadFile(analysisReportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read analysis report: %w", err)
	}

	// Parse contextAnswers — may be plain text or structured JSON
	answersText := buildContextAnswersText(contextAnswers)

	userPrompt := fmt.Sprintf("## 招标文件解析报告\n\n%s\n\n## 用户回答\n\n%s", string(reportBytes), answersText)

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: projectContextReportSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  32000.0,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty project context report")
	}

	if err := os.MkdirAll(filepath.Dir(contextReportPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create report directory: %w", err)
	}
	if err := os.WriteFile(contextReportPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write report: %w", err)
	}

	artifact := map[string]interface{}{
		"unitId":     "bid-project-context",
		"kind":       "BID_PROJECT_CONTEXT",
		"name":       filepath.Base(contextReportPath),
		"mimeType":   "text/markdown",
		"storageRef": contextReportPath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}

	return &GenerateProjectContextReportResponse{
		AnalysisReportPath: analysisReportPath,
		ContextReportPath:  contextReportPath,
		Content:            content,
		Artifact:           artifact,
	}, nil
}

// questionnaireAnswer is the minimal struct for parsing a question's answer from JSON.
type questionnaireAnswer struct {
	Selected    []string `json:"selected"`
	Text        string   `json:"text"`
	ExtraText   string   `json:"extraText"`
	NeedsReview bool     `json:"needsReview"`
}

type questionnaireQuestion struct {
	ID     string              `json:"id"`
	Prompt string              `json:"prompt"`
	Answer questionnaireAnswer `json:"answer"`
}

type questionnaire struct {
	Questions []questionnaireQuestion `json:"questions"`
}

// buildContextAnswersText tries to parse contextAnswers as JSON questionnaire.
// If successful, it formats a structured Markdown for the LLM.
// Otherwise, it returns the raw text unchanged.
func buildContextAnswersText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return raw
	}

	var q questionnaire
	if err := json.Unmarshal([]byte(trimmed), &q); err != nil {
		return raw
	}

	if len(q.Questions) == 0 {
		return raw
	}

	var b strings.Builder
	b.WriteString("以下是用户对每个问题的结构化回答：\n\n")

	for i, question := range q.Questions {
		prompt := question.Prompt
		if prompt == "" {
			prompt = "(问题文本缺失)"
		}
		b.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, prompt))

		ans := question.Answer

		// Selected options
		if len(ans.Selected) > 0 {
			b.WriteString("- 用户选择：")
			for j, sel := range ans.Selected {
				if j > 0 {
					b.WriteString("、")
				}
				b.WriteString(sel)
			}
			b.WriteString("\n")
		}

		// Text answer
		if strings.TrimSpace(ans.Text) != "" {
			b.WriteString(fmt.Sprintf("- 用户回答：%s\n", ans.Text))
		}

		// Extra text
		if strings.TrimSpace(ans.ExtraText) != "" {
			b.WriteString(fmt.Sprintf("- 补充说明：%s\n", ans.ExtraText))
		}

		// Needs review flag
		if ans.NeedsReview {
			b.WriteString("- **此内容待用户确认，请勿作为已确认事实。**\n")
		}

		// No answer at all
		if len(ans.Selected) == 0 && strings.TrimSpace(ans.Text) == "" && strings.TrimSpace(ans.ExtraText) == "" {
			b.WriteString("- 用户未回答此问题。\n")
		}

		b.WriteString("\n")
	}

	return b.String()
}
