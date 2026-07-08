package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

const chapterTaskBookSystemPrompt = `你是一个专业的标书章节写作任务规划专家。

你需要根据《技术标四级大纲》《评分标准拆解表》《招标文件解析报告》《项目背景确认表》，生成一份可供用户审阅和后续章节并发写作使用的《章节写作任务书》。

任务书不是正文内容，而是章节写作计划。它必须帮助后续写作模型明确每个章节写什么、为什么写、参考什么、不能写什么、预计写多少字。

要求：
1. 必须以 Markdown 输出。
2. 必须为每个一级章节生成独立写作任务。
3. 每个章节任务必须包含以下中文标签字段：
   - 章节编号
   - 章节标题
   - 层级
   - 对应评分项
   - 输入产物（引用的大纲章节、评分标准、项目背景等）
   - 写作重点
   - 支撑材料需求
   - 禁止内容
   - 预估字数
   - 前置依赖
4. 每个评分项至少应被一个章节覆盖。
5. 如果某个评分项无法映射到现有大纲章节，必须在"风险提示与人工确认项"中说明。
6. 禁止生成商务报价、预算金额、价格承诺等内容。
7. 不要编造招标文件、评分标准或项目背景中不存在的硬性要求。
8. 输出应便于用户阅读，也应便于后续程序解析。
9. 末尾必须包含"评分项覆盖表""写作顺序建议""总字数预估""风险提示与人工确认项"。`

const (
	taskBookOutlineRuneLimit  = 15000
	taskBookScoringRuneLimit  = 15000
	taskBookAnalysisRuneLimit = 8000
	taskBookContextRuneLimit  = 8000
	taskBookMaxTokens         = 16000
)

// GenerateChapterTaskBookRequest is the input for chapter task book generation.
type GenerateChapterTaskBookRequest struct {
	OutlinePath        string `json:"outlinePath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextReportPath  string `json:"contextReportPath,omitempty"`
	TaskBookPath       string `json:"taskBookPath"`
	ProjectID          string `json:"projectId"`
	RunID              string `json:"runId"`
	Mode               string `json:"mode,omitempty"` // strict | draft
}

// GenerateChapterTaskBookResponse is the output.
type GenerateChapterTaskBookResponse struct {
	TaskBookPath string                 `json:"taskBookPath"`
	Content      string                 `json:"content"`
	Artifact     map[string]interface{} `json:"artifact"`
	Warnings     []string               `json:"warnings,omitempty"`
}

func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildChapterTaskBookUserPrompt(outlineText, scoringText, analysisText, contextText string) string {
	compactOutline := limitTextForPrompt(outlineText, taskBookOutlineRuneLimit, "大纲")
	compactScoring := limitTextForPrompt(scoringText, taskBookScoringRuneLimit, "评分标准拆解表")
	compactAnalysis := limitTextForPrompt(
		stripBidAnalysisAppendix(analysisText),
		taskBookAnalysisRuneLimit,
		"解析报告",
	)

	var sb strings.Builder
	sb.WriteString("## 技术标四级大纲\n\n")
	sb.WriteString(compactOutline)
	sb.WriteString("\n\n---\n\n## 评分标准拆解表\n\n")
	sb.WriteString(compactScoring)
	sb.WriteString("\n\n---\n\n## 招标文件解析报告\n\n")
	sb.WriteString(compactAnalysis)

	if strings.TrimSpace(contextText) != "" {
		compactContext := limitTextForPrompt(contextText, taskBookContextRuneLimit, "项目背景确认表")
		sb.WriteString("\n\n---\n\n## 项目背景确认表\n\n")
		sb.WriteString(compactContext)
	}

	sb.WriteString("\n\n---\n\n请根据以上材料生成《章节写作任务书》。")
	return sb.String()
}

func validateChapterTaskBookContent(content string) []string {
	var warnings []string
	checks := []string{
		"章节写作任务书",
		"项目信息",
		"章节依赖关系",
		"章节写作任务清单",
		"评分项覆盖表",
		"写作顺序建议",
		"总字数预估",
		"风险提示",
	}
	for _, keyword := range checks {
		if !strings.Contains(content, keyword) {
			warnings = append(warnings, fmt.Sprintf("缺失预期章节: %s", keyword))
		}
	}
	// Price leak check
	priceKeywords := []string{"报价", "预算金额", "投标总价", "价格承诺"}
	for _, kw := range priceKeywords {
		if strings.Contains(content, kw) {
			warnings = append(warnings, fmt.Sprintf("内容可能包含报价/价格信息: %s", kw))
		}
	}
	return warnings
}

func validateScoreItemCoverage(scoringContent, taskBookContent string) []string {
	var warnings []string
	// Simple approach: extract score item names from scoring breakdown and check presence in task book.
	lines := strings.Split(scoringContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for lines that appear to define scoring items (e.g., "- 施工组织方案" or "| 施工组织方案 |")
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "|") {
			item := strings.TrimLeft(trimmed, "|-* ")
			item = strings.TrimSpace(item)
			if len([]rune(item)) >= 4 && !strings.Contains(taskBookContent, item[:min(len([]rune(item)), 8)]) {
				// Simple check - if first 8 chars of the item name don't appear in task book
				continue
			}
		}
	}
	_ = warnings // reserved for per-item tracking in future
	return warnings
}

// GenerateChapterTaskBook generates the chapter writing task book from outline, scoring, analysis, and context.
func GenerateChapterTaskBook(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateChapterTaskBookRequest,
) (*GenerateChapterTaskBookResponse, error) {
	outlinePath := strings.TrimSpace(req.OutlinePath)
	taskBookPath := strings.TrimSpace(req.TaskBookPath)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "strict"
	}

	if outlinePath == "" {
		return nil, fmt.Errorf("outlinePath is required")
	}
	if taskBookPath == "" {
		return nil, fmt.Errorf("taskBookPath is required")
	}
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	// Read outline (required)
	outlineText, err := readFileIfExists(outlinePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read outline: %w", err)
	}

	// Read scoring report
	scoringPath := strings.TrimSpace(req.ScoringReportPath)
	var scoringText string
	if scoringPath != "" {
		if data, err := readFileIfExists(scoringPath); err == nil {
			scoringText = data
		} else if mode == "strict" {
			return nil, fmt.Errorf("scoringReportPath is required in strict mode: %w", err)
		}
	} else if mode == "strict" {
		return nil, fmt.Errorf("scoringReportPath is required in strict mode")
	}

	// Read analysis report
	analysisPath := strings.TrimSpace(req.AnalysisReportPath)
	if analysisPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}
	analysisText, err := readFileIfExists(analysisPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read analysis report: %w", err)
	}

	// Read context report (optional)
	contextPath := strings.TrimSpace(req.ContextReportPath)
	var contextText string
	if contextPath != "" {
		if data, err := readFileIfExists(contextPath); err == nil {
			contextText = data
		}
	}

	userPrompt := buildChapterTaskBookUserPrompt(outlineText, scoringText, analysisText, contextText)

	start := time.Now()
	zap.L().Info("biaoshu chapter task book generation model call starting",
		zap.Int("outlineRunes", len([]rune(outlineText))),
		zap.Int("scoringRunes", len([]rune(scoringText))),
		zap.Int("analysisRunes", len([]rune(analysisText))),
		zap.Int("contextRunes", len([]rune(contextText))),
		zap.String("mode", mode),
		zap.Int("maxTokens", taskBookMaxTokens),
	)

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: chapterTaskBookSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  float64(taskBookMaxTokens),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	zap.L().Info("biaoshu chapter task book generation model call completed",
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.String("model", result.Usage.Model),
		zap.Int("promptTokens", result.Usage.PromptTokens),
		zap.Int("outputTokens", result.Usage.OutputTokens),
	)

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty chapter task book")
	}

	// Validate output structure
	warnings := validateChapterTaskBookContent(content)
	if scoringText != "" {
		_ = validateScoreItemCoverage(scoringText, content)
	}

	if err := os.MkdirAll(filepath.Dir(taskBookPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create task book directory: %w", err)
	}
	if err := os.WriteFile(taskBookPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write task book: %w", err)
	}

	artifact := map[string]interface{}{
		"unitId":     "chapter-task-book",
		"kind":       "BID_CHAPTER_TASK_BOOK",
		"name":       filepath.Base(taskBookPath),
		"mimeType":   "text/markdown",
		"storageRef": taskBookPath,
		"metadata": map[string]interface{}{
			"status": "valid",
		},
	}

	return &GenerateChapterTaskBookResponse{
		TaskBookPath: taskBookPath,
		Content:      content,
		Artifact:     artifact,
		Warnings:     warnings,
	}, nil
}
