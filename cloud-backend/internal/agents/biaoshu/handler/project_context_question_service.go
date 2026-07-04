package handler

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

// ProjectContextQuestion is a structured project context question.
type ProjectContextQuestion struct {
	ID        string                 `json:"id"`
	Category  string                 `json:"category"`
	Title     string                 `json:"title"`
	Prompt    string                 `json:"prompt"`
	HelpText  string                 `json:"helpText,omitempty"`
	InputType string                 `json:"inputType"`
	Required  bool                   `json:"required"`
	Options   []ProjectContextOption `json:"options,omitempty"`
}

// ProjectContextOption is an option for single/multi choice questions.
type ProjectContextOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Required project context questions (structured)
var requiredProjectContextQuestions = []ProjectContextQuestion{
	{
		ID:        "project_location",
		Category:  "basic",
		Title:     "项目所在地及行政区域",
		Prompt:    "项目所在地及行政区域是什么？",
		HelpText:  "请填写到市/区县，必要时补充具体道路、公园、片区或标段。",
		InputType: "text",
		Required:  true,
	},
	{
		ID:        "climate_conditions",
		Category:  "environment",
		Title:     "当地气候条件",
		Prompt:    "当地气候条件有哪些需要考虑？",
		HelpText:  "可多选，也可以补充季节性养护、雨季、高温等影响。",
		InputType: "multi_choice",
		Required:  true,
		Options: []ProjectContextOption{
			{Value: "high_temperature", Label: "高温", Description: "夏季高温、苗木补水、遮阴等"},
			{Value: "rainy_season", Label: "多雨/梅雨", Description: "排水、防涝、病虫害防治等"},
			{Value: "typhoon", Label: "台风/大风", Description: "加固、防倒伏、应急巡查等"},
			{Value: "freezing", Label: "冰冻/低温", Description: "防寒、防冻、越冬养护等"},
			{Value: "not_specified", Label: "招标文件未明确", Description: "后续作为待确认事项处理"},
			{Value: "other", Label: "其他", Description: "需要在补充说明中填写"},
		},
	},
	{
		ID:        "traffic_conditions",
		Category:  "environment",
		Title:     "现场交通条件",
		Prompt:    "项目现场交通条件如何？",
		HelpText:  "例如主干路、施工或服务通道、运输条件、交通组织限制等。",
		InputType: "textarea",
		Required:  true,
	},
	{
		ID:        "hydrology",
		Category:  "environment",
		Title:     "水文条件",
		Prompt:    "是否涉及水文条件？",
		HelpText:  "例如河流、地下水、雨洪、航道、水位变化等。",
		InputType: "multi_choice",
		Required:  false,
		Options: []ProjectContextOption{
			{Value: "river", Label: "河流", Description: "有河流穿越或毗邻项目区域"},
			{Value: "groundwater", Label: "地下水", Description: "地下水位较高或有涌水风险"},
			{Value: "flood", Label: "雨洪/洪水", Description: "雨季洪水或积水风险"},
			{Value: "waterway", Label: "航道", Description: "涉及通航水域"},
			{Value: "none", Label: "不涉及", Description: "无明显水文影响"},
			{Value: "not_specified", Label: "招标文件未明确"},
			{Value: "other", Label: "其他"},
		},
	},
	{
		ID:        "geology",
		Category:  "environment",
		Title:     "地质地形条件",
		Prompt:    "地质、地形、土地条件如何？",
		HelpText:  "例如地形地貌、软土、山地、农田、征拆、用地限制等。",
		InputType: "textarea",
		Required:  false,
	},
	{
		ID:        "sensitive_areas",
		Category:  "environment",
		Title:     "周边敏感点",
		Prompt:    "周边是否有居民区、学校、医院、厂区、生态保护区等敏感点？",
		InputType: "textarea",
		Required:  false,
	},
	{
		ID:        "construction_constraints",
		Category:  "environment",
		Title:     "施工或服务环境限制",
		Prompt:    "施工或服务环境有哪些限制？",
		HelpText:  "例如夜间施工、噪声、扬尘、环保、保通保畅等。可多选。",
		InputType: "multi_choice",
		Required:  false,
		Options: []ProjectContextOption{
			{Value: "night_work", Label: "夜间施工限制"},
			{Value: "noise", Label: "噪声限制"},
			{Value: "dust", Label: "扬尘控制"},
			{Value: "environmental", Label: "环保要求"},
			{Value: "traffic_flow", Label: "保通保畅"},
			{Value: "other", Label: "其他"},
		},
	},
	{
		ID:        "bidder_advantages",
		Category:  "bidder",
		Title:     "投标单位自身优势",
		Prompt:    "投标单位自身有哪些优势？",
		HelpText:  "例如类似业绩、设备、人员、工法、管理经验等。",
		InputType: "textarea",
		Required:  false,
	},
	{
		ID:        "emphasis_points",
		Category:  "writing",
		Title:     "希望重点强调的内容",
		Prompt:    "希望重点强调哪些内容？",
		HelpText:  "例如特定技术方案、质量保障措施、进度优势、成本控制等。",
		InputType: "textarea",
		Required:  false,
	},
	{
		ID:        "avoid_points",
		Category:  "writing",
		Title:     "希望避免的内容",
		Prompt:    "希望避免出现哪些内容？",
		HelpText:  "例如避免提及某类问题、避免夸大某方面等。",
		InputType: "textarea",
		Required:  false,
	},
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
	AnalysisReportPath string                   `json:"analysisReportPath"`
	Questions          []ProjectContextQuestion `json:"questions"`
	Markdown           string                   `json:"markdown"`
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

	questions := append([]ProjectContextQuestion{}, requiredProjectContextQuestions...)

	// Try LLM supplement if gateway is available
	if gw != nil {
		supplement, err := generateSupplementQuestions(ctx, gw, analysisReportPath)
		if err != nil {
			// Non-fatal: append a fallback question
			questions = append(questions, ProjectContextQuestion{
				ID:        "project_specific_fallback",
				Category:  "project_specific",
				Title:     "项目特有补充说明",
				Prompt:    "请补充任何项目特有的、上述问题未覆盖的背景信息。",
				InputType: "textarea",
				Required:  false,
			})
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

func generateSupplementQuestions(ctx context.Context, gw *modelgateway.Gateway, reportPath string) ([]ProjectContextQuestion, error) {
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read report: %w", err)
	}

	baseTitles := make([]string, len(requiredProjectContextQuestions))
	for i, q := range requiredProjectContextQuestions {
		baseTitles[i] = q.Prompt
	}
	baseList := strings.Join(baseTitles, "\n")
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

	var supplements []ProjectContextQuestion
	for _, line := range strings.Split(result.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			q := strings.TrimPrefix(trimmed, "- ")
			if q != "" {
				supplements = append(supplements, ProjectContextQuestion{
					ID:        stableQuestionIDFromText(q),
					Category:  "project_specific",
					Title:     "项目特有补充问题",
					Prompt:    q,
					InputType: "textarea",
					Required:  false,
				})
			}
		}
	}
	return supplements, nil
}

func stableQuestionIDFromText(text string) string {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return "project_specific"
	}
	sum := sha1.Sum([]byte(normalized))
	return fmt.Sprintf("project_specific_%x", sum[:4])
}

func buildQuestionsMarkdown(questions []ProjectContextQuestion, sourceFile string) string {
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
		b.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, q.Prompt))
		if len(q.Options) > 0 {
			b.WriteString("可选答案：\n")
			for _, opt := range q.Options {
				b.WriteString(fmt.Sprintf("- %s", opt.Label))
				if opt.Description != "" {
					b.WriteString(fmt.Sprintf("（%s）", opt.Description))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
		b.WriteString("**回答**：\n\n")
	}

	return b.String()
}
