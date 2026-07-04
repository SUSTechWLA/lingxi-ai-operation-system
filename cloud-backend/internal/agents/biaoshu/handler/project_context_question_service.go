package handler

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

var requiredProjectContextQuestions = []string{
	"项目所在地及行政区域是什么？",
	"当地气候条件有哪些需要考虑？例如温度、降雨、台风、冰冻、高温等。",
	"项目现场交通条件如何？例如主干路、施工或服务通道、运输条件、交通组织限制等。",
	"是否涉及水文条件？例如河流、地下水、雨洪、航道、水位变化等。",
	"地质、地形、土地条件如何？例如地形地貌、软土、山地、农田、征拆、用地限制等。",
	"周边是否有居民区、学校、医院、厂区、生态保护区等敏感点？",
	"施工或服务环境有哪些限制？例如夜间施工、噪声、扬尘、环保、保通保畅等。",
	"投标单位自身优势有哪些？例如类似业绩、设备、人员、工法、管理经验等。",
	"希望重点强调哪些内容？",
	"希望避免出现哪些内容？",
}

const projectContextQuestionPrompt = `你是一个专业的标书撰写顾问。请根据下面的《招标文件解析报告》，补充项目特有的背景信息采集问题（最多5个），这些问题必须是基础问题清单中没有覆盖的、针对本项目的特有问题。

基础问题清单已包含：
%s

请基于解析报告中的项目类型，输出不超过5个补充问题，每行一个，以"- "开头。
如果招标文件解析报告中已经回答了部分问题，则不需要再提问相关的问题。
如果招标文件没有特别类型信息，可以不输出补充问题。`

// GenerateProjectContextQuestionsRequest is the input for generating project context questions.
type GenerateProjectContextQuestionsRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

// GenerateProjectContextQuestionsResponse is the output.
type GenerateProjectContextQuestionsResponse struct {
	AnalysisReportPath string   `json:"analysisReportPath"`
	Questions          []string `json:"questions"`
	Markdown           string   `json:"markdown"`
}

// GenerateProjectContextQuestions generates the project context questions list.
func GenerateProjectContextQuestions(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateProjectContextQuestionsRequest,
) (*GenerateProjectContextQuestionsResponse, error) {
	analysisReportPath := strings.TrimSpace(req.AnalysisReportPath)
	if analysisReportPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}

	questions := append([]string{}, requiredProjectContextQuestions...)

	// Try LLM supplement if gateway is available
	if gw != nil {
		supplement, err := generateSupplementQuestions(ctx, gw, analysisReportPath)
		if err != nil {
			// Non-fatal: use only the fixed questions
			questions = append(questions, "(未能生成补充问题)")
		} else if len(supplement) > 0 {
			questions = append(questions, supplement...)
		}
	}

	markdown := buildQuestionsMarkdown(questions, req.SourceFile)

	return &GenerateProjectContextQuestionsResponse{
		AnalysisReportPath: analysisReportPath,
		Questions:          questions,
		Markdown:           markdown,
	}, nil
}

func generateSupplementQuestions(ctx context.Context, gw *modelgateway.Gateway, reportPath string) ([]string, error) {
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read report: %w", err)
	}

	baseList := strings.Join(requiredProjectContextQuestions, "\n")
	prompt := fmt.Sprintf(projectContextQuestionPrompt, baseList)

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: string(reportBytes)},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  2000.0,
		},
	})
	if err != nil {
		return nil, err
	}

	var supplements []string
	for _, line := range strings.Split(result.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			q := strings.TrimPrefix(trimmed, "- ")
			if q != "" {
				supplements = append(supplements, q)
			}
		}
	}
	return supplements, nil
}

func buildQuestionsMarkdown(questions []string, sourceFile string) string {
	var b strings.Builder
	b.WriteString("# 项目背景信息采集问题\n\n")
	if sourceFile != "" {
		b.WriteString(fmt.Sprintf("**源文件**: %s\n\n", sourceFile))
	}
	b.WriteString("请逐项回答以下问题，也可以一次性粘贴完整说明。\n\n")
	b.WriteString("---\n\n")

	baseCount := len(requiredProjectContextQuestions)
	for i, q := range questions {
		if i == baseCount {
			b.WriteString("\n## 项目特有补充问题\n\n")
		}
		b.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, q))
		b.WriteString("**回答**：\n\n")
	}

	return b.String()
}
