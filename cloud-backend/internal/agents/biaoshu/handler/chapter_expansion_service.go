package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

const chapterExpansionConcurrency = 2

// ── Expansion prompts ──

const expansionSystemPrompt = `你是专业的投标技术标章节扩写专家。

你的任务不是重写章节，而是在保留原章节标题结构、事实边界和写作风格的基础上，对字数不足或评分点展开不足的章节进行扩写。

必须遵守：
1. 保留原有一级、二级、三级、四级标题结构，不得删除原文有效内容。
2. 不得新增"本章小结""本章总结""小结""总结"等总结性章节。
3. 严禁出现"我方""我们"，统一使用"本方案""项目组""将"。
4. 严禁出现报价、预算金额、投标总价、价格承诺等商务内容。
5. 不得编造招标文件、评分标准、项目背景中不存在的硬性事实。
6. 扩写内容必须围绕评分项、采购需求、项目背景、实施措施展开。
7. 每个被扩写小节至少补充 2-4 个自然段。
8. 必要时增加表格，但不得堆砌空泛表格。
9. 扩写后正文应自然连续，不得出现"以下为扩写内容"等提示语。
10. 输出完整扩写后的章节 Markdown。`

// ── Expansion task book ──

// GenerateExpansionTaskBook 基于字数检查结果生成章节扩写任务书
func GenerateExpansionTaskBook(outputDir string, wordCountItems []WordCountItem, taskBookContent string) (*ExpansionTaskBookResult, error) {
	shortChapters := make([]WordCountItem, 0)
	totalGap := 0
	for _, item := range wordCountItems {
		if item.Status == "too_short" {
			shortChapters = append(shortChapters, item)
			totalGap += item.GapWords
		}
	}

	if len(shortChapters) == 0 {
		taskBookPath := outputDir + "/06_章节扩写任务书.md"
		content := "# 章节扩写任务书\n\n✅ 所有章节字数均达标，无需扩写。\n"
		if err := os.WriteFile(taskBookPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write expansion task book: %w", err)
		}
		return &ExpansionTaskBookResult{
			TaskBookPath: taskBookPath,
			TotalGap:     0,
			ShortCount:   0,
			Artifact: map[string]any{
				"id":         "expansion-task-book",
				"kind":       "BID_CHAPTER_EXPANSION_TASK_BOOK",
				"name":       "章节扩写任务书",
				"storageRef": taskBookPath,
				"mimeType":   "text/markdown",
				"status":     "valid",
				"metadata":   map[string]any{"totalGap": 0, "shortCount": 0, "generatedAt": time.Now().Format(time.RFC3339)},
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("# 章节扩写任务书\n\n")
	sb.WriteString(fmt.Sprintf("> 生成时间：%s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**需扩写章节**：%d 章\n", len(shortChapters)))
	sb.WriteString(fmt.Sprintf("**总计需补充字数**：%d 字\n\n", totalGap))
	sb.WriteString("---\n\n")

	for _, item := range shortChapters {
		sb.WriteString(fmt.Sprintf("## %s\n\n", item.ChapterTitle))
		sb.WriteString(fmt.Sprintf("| 项目 | 数值 |\n|---|---|\n"))
		sb.WriteString(fmt.Sprintf("| 当前字数 | %d |\n", item.CurrentWords))
		sb.WriteString(fmt.Sprintf("| 目标字数 | %d |\n", item.TargetWords))
		sb.WriteString(fmt.Sprintf("| 合格下限 | %d |\n", item.QualifiedMin))
		sb.WriteString(fmt.Sprintf("| 需补充字数 | %d |\n\n", item.GapWords))

		sb.WriteString("### 扩写原因\n\n")
		sb.WriteString(fmt.Sprintf("当前仅完成目标字数的 %.0f%%，需扩充内容以满足技术标深度要求。\n\n",
			float64(item.CurrentWords)/float64(item.TargetWords)*100))

		sb.WriteString("### 扩写方向\n\n优先补充以下方面：\n\n")
		sb.WriteString("1. **流程步骤**：关键工序的详细操作流程\n")
		sb.WriteString("2. **质量保障**：各工序质量控制点与检查频次\n")
		sb.WriteString("3. **资源配置**：人员、设备、材料的配置方案\n")
		sb.WriteString("4. **安全措施**：施工安全与环保措施\n")
		sb.WriteString("5. **进度保障**：进度风险与应对措施\n")
		sb.WriteString("6. **针对性措施**：结合项目所在地气候、交通等特点的措施\n\n")

		sb.WriteString("### 小节扩写建议\n\n")
		sb.WriteString("1. 补充措施表格，细化到具体操作步骤\n")
		sb.WriteString("2. 对段落不足3段的小节进行扩充\n")
		sb.WriteString("3. 增加验收标准与检查频次\n")
		if item.GapWords > 10000 {
			sb.WriteString("4. 补充案例分析或类似项目经验\n")
		}
		sb.WriteString("\n---\n\n")
	}

	taskBookPath := outputDir + "/06_章节扩写任务书.md"
	if err := os.WriteFile(taskBookPath, []byte(sb.String()), 0644); err != nil {
		return nil, fmt.Errorf("failed to write expansion task book: %w", err)
	}

	return &ExpansionTaskBookResult{
		TaskBookPath: taskBookPath,
		Items:        shortChapters,
		TotalGap:     totalGap,
		ShortCount:   len(shortChapters),
		Artifact: map[string]any{
			"id":         "expansion-task-book",
			"kind":       "BID_CHAPTER_EXPANSION_TASK_BOOK",
			"name":       "章节扩写任务书",
			"storageRef": taskBookPath,
			"mimeType":   "text/markdown",
			"status":     "valid",
			"metadata":   map[string]any{"totalGap": totalGap, "shortCount": len(shortChapters), "generatedAt": time.Now().Format(time.RFC3339)},
		},
	}, nil
}

// ExpansionTaskBookResult is the output of task book generation.
type ExpansionTaskBookResult struct {
	TaskBookPath string          `json:"taskBookPath"`
	Items        []WordCountItem `json:"items"`
	TotalGap     int             `json:"totalGap"`
	ShortCount   int             `json:"shortCount"`
	Artifact     map[string]any  `json:"artifact"`
}

// ── Chapter expansion ──

// ExpandChaptersRequest is the input for batch chapter expansion.
type ExpandChaptersRequest struct {
	Items        []WordCountItem `json:"items"` // only too_short chapters
	TaskBookPath string          `json:"taskBookPath"`
	OutlinePath  string          `json:"outlinePath"`
	ScoringPath  string          `json:"scoringReportPath"`
	AnalysisPath string          `json:"analysisReportPath"`
	ContextPath  string          `json:"contextReportPath,omitempty"`
	OutputDir    string          `json:"outputDir"`
}

// ExpandChapterItem is a single chapter expansion result.
type ExpandChapterItem struct {
	ChapterNumber   int            `json:"chapterNumber"`
	ChapterTitle    string         `json:"chapterTitle"`
	SourcePath      string         `json:"sourcePath"`
	ExpandedPath    string         `json:"expandedPath"`
	WordCountBefore int            `json:"wordCountBefore"`
	WordCountAfter  int            `json:"wordCountAfter"`
	TargetWordCount int            `json:"targetWordCount"`
	Artifact        map[string]any `json:"artifact,omitempty"`
	Error           string         `json:"error,omitempty"`
}

// ExpandChaptersResponse is the aggregated result.
type ExpandChaptersResponse struct {
	Results []ExpandChapterItem `json:"results"`
	Success int                 `json:"success"`
	Failed  int                 `json:"failed"`
}

// ExpandChapters expands short chapters using the LLM gateway.
func ExpandChapters(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req ExpandChaptersRequest,
) (*ExpandChaptersResponse, error) {
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}
	if len(req.Items) == 0 {
		return &ExpandChaptersResponse{}, nil
	}

	// Read supporting files for prompt context
	taskBookText := readFileOrEmpty(req.TaskBookPath)
	outlineText := readFileOrEmpty(req.OutlinePath)
	scoringText := readFileOrEmpty(req.ScoringPath)
	analysisText := readFileOrEmpty(req.AnalysisPath)
	contextText := readFileOrEmpty(req.ContextPath)

	results := make([]ExpandChapterItem, len(req.Items))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, chapterExpansionConcurrency)

	start := time.Now()

	for i, item := range req.Items {
		if item.Status != "too_short" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, it WordCountItem) {
			defer wg.Done()
			defer func() { <-sem }()

			r := expandOneChapter(ctx, gw, it, taskBookText, outlineText, scoringText, analysisText, contextText, req.OutputDir)

			mu.Lock()
			results[idx] = r
			mu.Unlock()
		}(i, item)
	}
	wg.Wait()

	zap.L().Info("biaoshu chapter expansion completed",
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.Int("chapterCount", len(req.Items)),
	)

	resp := &ExpandChaptersResponse{Results: results}
	for _, r := range results {
		if r.Error != "" {
			resp.Failed++
		} else {
			resp.Success++
		}
	}
	return resp, nil
}

// expandOneChapter expands a single chapter.
func expandOneChapter(
	ctx context.Context,
	gw *modelgateway.Gateway,
	item WordCountItem,
	taskBookText, outlineText, scoringText, analysisText, contextText, outputDir string,
) ExpandChapterItem {
	// Read the original chapter content
	originalContent, err := os.ReadFile(item.FilePath)
	if err != nil {
		return ExpandChapterItem{
			ChapterNumber:   item.ChapterNumber,
			ChapterTitle:    item.ChapterTitle,
			SourcePath:      item.FilePath,
			TargetWordCount: item.TargetWords,
			Error:           fmt.Sprintf("failed to read chapter file: %v", err),
		}
	}

	wordCountBefore := countChineseMarkdownWords(string(originalContent))

	// Build prompts
	userPrompt := buildExpansionUserPrompt(item, string(originalContent), taskBookText, outlineText, scoringText, analysisText, contextText)

	modelStart := time.Now()
	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: expansionSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  float64(16000),
		},
	})
	if err != nil {
		return ExpandChapterItem{
			ChapterNumber:   item.ChapterNumber,
			ChapterTitle:    item.ChapterTitle,
			SourcePath:      item.FilePath,
			TargetWordCount: item.TargetWords,
			Error:           fmt.Sprintf("LLM call failed: %v", err),
		}
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return ExpandChapterItem{
			ChapterNumber:   item.ChapterNumber,
			ChapterTitle:    item.ChapterTitle,
			SourcePath:      item.FilePath,
			TargetWordCount: item.TargetWords,
			Error:           "LLM returned empty content",
		}
	}

	// Generate expanded file path
	safeTitle := strings.TrimLeft(item.ChapterTitle, "一二三四五六七八九十、 ")
	safeTitle = strings.TrimSpace(safeTitle)
	if safeTitle == "" {
		safeTitle = item.ChapterTitle
	}
	safeTitle = strings.NewReplacer("/", "-", "\\", "-", ":", "：", "?", "", "*", "", "\"", "", "<", "", ">", "", "|", "").Replace(safeTitle)
	expandedPath := filepath.Join(outputDir, fmt.Sprintf("%02d_%s_扩写稿.md", item.ChapterNumber, safeTitle))

	if err := os.WriteFile(expandedPath, []byte(content), 0644); err != nil {
		return ExpandChapterItem{
			ChapterNumber:   item.ChapterNumber,
			ChapterTitle:    item.ChapterTitle,
			SourcePath:      item.FilePath,
			TargetWordCount: item.TargetWords,
			Error:           fmt.Sprintf("failed to write expanded file: %v", err),
		}
	}

	wordCountAfter := countChineseMarkdownWords(content)

	zap.L().Info("biaoshu chapter expanded",
		zap.Int("chapter", item.ChapterNumber),
		zap.Int("wordCountBefore", wordCountBefore),
		zap.Int("wordCountAfter", wordCountAfter),
		zap.Int("targetWords", item.TargetWords),
		zap.Int64("durationMs", time.Since(modelStart).Milliseconds()),
	)

	artifact := map[string]any{
		"id":              fmt.Sprintf("chapter-%d-expanded", item.ChapterNumber),
		"kind":            "BID_CHAPTERS",
		"unitId":          fmt.Sprintf("chapter-%d-expanded", item.ChapterNumber),
		"name":            item.ChapterTitle + "（扩写稿）",
		"storageRef":      expandedPath,
		"mimeType":        "text/markdown",
		"status":          "valid",
		"chapterNumber":   item.ChapterNumber,
		"chapterTitle":    item.ChapterTitle,
		"draftStage":      "expanded",
		"sourceDraft":     item.FilePath,
		"wordCountBefore": wordCountBefore,
		"wordCountAfter":  wordCountAfter,
		"targetWordCount": item.TargetWords,
		"metadata": map[string]any{
			"chapterNumber":   item.ChapterNumber,
			"draftStage":      "expanded",
			"wordCountBefore": wordCountBefore,
			"wordCountAfter":  wordCountAfter,
			"targetWordCount": item.TargetWords,
			"qualifiedMin":    item.QualifiedMin,
			"qualifiedMax":    item.QualifiedMax,
			"expandedAt":      time.Now().Format(time.RFC3339),
		},
	}

	return ExpandChapterItem{
		ChapterNumber:   item.ChapterNumber,
		ChapterTitle:    item.ChapterTitle,
		SourcePath:      item.FilePath,
		ExpandedPath:    expandedPath,
		WordCountBefore: wordCountBefore,
		WordCountAfter:  wordCountAfter,
		TargetWordCount: item.TargetWords,
		Artifact:        artifact,
	}
}

// buildExpansionUserPrompt builds the user prompt for chapter expansion.
func buildExpansionUserPrompt(
	item WordCountItem,
	chapterContent, taskBookText, outlineText, scoringText, analysisText, contextText string,
) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 扩写任务\n\n请对【%s】进行字数扩充。\n\n", item.ChapterTitle))
	sb.WriteString(fmt.Sprintf("当前字数：%d\n", item.CurrentWords))
	sb.WriteString(fmt.Sprintf("目标字数：%d\n", item.TargetWords))
	sb.WriteString(fmt.Sprintf("合格下限：%d\n", item.QualifiedMin))
	sb.WriteString(fmt.Sprintf("建议补充字数：%d\n\n", item.GapWords))

	sb.WriteString("## 原章节正文\n\n")
	sb.WriteString(chapterContent)
	sb.WriteString("\n\n---\n\n")

	if outlineText != "" {
		sb.WriteString("## 本章大纲\n\n")
		outlineSection := extractOutlineSubtree(outlineText, item.ChapterTitle)
		if outlineSection != "" {
			sb.WriteString(outlineSection)
		}
		sb.WriteString("\n\n---\n\n")
	}

	if scoringText != "" {
		sb.WriteString("## 评分标准\n\n")
		sb.WriteString(truncateText(scoringText, 3000))
		sb.WriteString("\n\n---\n\n")
	}

	if contextText != "" {
		sb.WriteString("## 项目背景\n\n")
		sb.WriteString(truncateText(contextText, 2000))
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString("请输出扩写后的完整章节 Markdown。不得输出解释说明，不得包裹代码块。")
	return sb.String()
}

// ── Expansion QA ──

// ExpansionQAResult is a single chapter QA result.
type ExpansionQAResult struct {
	ChapterNumber int            `json:"chapterNumber"`
	ChapterTitle  string         `json:"chapterTitle"`
	ExpandedPath  string         `json:"expandedPath"`
	Status        string         `json:"status"` // pass | warn | fail
	Issues        []string       `json:"issues"`
	WordCount     int            `json:"wordCount"`
	TargetWords   int            `json:"targetWords"`
	QualifiedMin  int            `json:"qualifiedMin"`
}

// ExpansionQAResponse is the QA report.
type ExpansionQAResponse struct {
	ReportPath string             `json:"reportPath"`
	Results    []ExpansionQAResult `json:"results"`
	PassCount  int                `json:"passCount"`
	WarnCount  int                `json:"warnCount"`
	FailCount  int                `json:"failCount"`
}

var (
	qaForbiddenWords = []string{"我方", "我们"}
	qaBizWords       = []string{"报价", "预算金额", "投标总价", "价格承诺", "投标报价", "总价", "单价"}
	qaSummaryHeaders = []string{"本章小结", "本章总结", "小结", "总结"}
)

// RunExpansionQA checks expanded chapters for quality issues.
func RunExpansionQA(outputDir string, expandedItems []ExpandChapterItem) (*ExpansionQAResponse, error) {
	results := make([]ExpansionQAResult, 0, len(expandedItems))
	pass, warn, fail := 0, 0, 0

	for _, item := range expandedItems {
		if item.Error != "" {
			continue
		}
		content, err := os.ReadFile(item.ExpandedPath)
		if err != nil {
			continue
		}
		text := string(content)
		r := ExpansionQAResult{
			ChapterNumber: item.ChapterNumber,
			ChapterTitle:  item.ChapterTitle,
			ExpandedPath:  item.ExpandedPath,
			TargetWords:   item.TargetWordCount,
			QualifiedMin:  int(float64(item.TargetWordCount) * qualifiedLowerRatio),
		}

		// 1. Check forbidden words
		for _, w := range qaForbiddenWords {
			if strings.Contains(text, w) {
				r.Issues = append(r.Issues, fmt.Sprintf("包含禁用词「%s」", w))
			}
		}

		// 2. Check business/price words
		for _, w := range qaBizWords {
			if strings.Contains(text, w) {
				r.Issues = append(r.Issues, fmt.Sprintf("包含商务价格词「%s」", w))
			}
		}

		// 3. Check summary headers
		for _, h := range qaSummaryHeaders {
			if strings.Contains(text, h) {
				r.Issues = append(r.Issues, fmt.Sprintf("包含禁止的总结标题「%s」", h))
			}
		}

		// 4. Check word count
		wc := countChineseMarkdownWords(text)
		r.WordCount = wc
		if wc < r.QualifiedMin {
			r.Issues = append(r.Issues, fmt.Sprintf("字数 %d 未达合格下限 %d", wc, r.QualifiedMin))
		}

		// Determine status
		if len(r.Issues) == 0 {
			r.Status = "pass"
			pass++
		} else {
			hasWordIssue := false
			for _, issue := range r.Issues {
				if strings.Contains(issue, "字数") {
					hasWordIssue = true
					break
				}
			}
			if hasWordIssue {
				r.Status = "fail"
				fail++
			} else {
				r.Status = "warn"
				warn++
			}
		}
		results = append(results, r)
	}

	// Generate report
	reportPath := outputDir + "/91_章节扩写质量检查报告.md"
	report := buildQaReport(results, pass, warn, fail)
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		return nil, fmt.Errorf("failed to write QA report: %w", err)
	}

	return &ExpansionQAResponse{
		ReportPath: reportPath,
		Results:    results,
		PassCount:  pass,
		WarnCount:  warn,
		FailCount:  fail,
	}, nil
}

func buildQaReport(results []ExpansionQAResult, pass, warn, fail int) string {
	var sb strings.Builder
	sb.WriteString("# 章节扩写质量检查报告\n\n")
	sb.WriteString(fmt.Sprintf("> 通过: %d, 警告: %d, 未通过: %d\n\n", pass, warn, fail))
	sb.WriteString("| 章节 | 字数 | 目标 | 状态 | 问题 |\n")
	sb.WriteString("|------|------|------|------|------|\n")

	for _, r := range results {
		statusText := "✅ 通过"
		if r.Status == "fail" {
			statusText = "❌ 未通过"
		} else if r.Status == "warn" {
			statusText = "⚠️ 警告"
		}
		issues := strings.Join(r.Issues, "；")
		if issues == "" {
			issues = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %s | %s |\n",
			r.ChapterTitle, r.WordCount, r.TargetWords, statusText, issues))
	}
	return sb.String()
}

// ── Helpers ──

func readFileOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func truncateText(text string, maxChars int) string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n\n...（内容过长已截断）"
}
