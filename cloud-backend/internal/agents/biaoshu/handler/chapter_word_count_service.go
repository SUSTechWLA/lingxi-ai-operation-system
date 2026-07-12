package handler

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ── Word count check data structures ──

// WordCountItem 单个章节的字数检查结果
type WordCountItem struct {
	ChapterNumber int      `json:"chapterNumber"`
	ChapterTitle  string   `json:"chapterTitle"`
	FilePath      string   `json:"filePath"`
	CurrentWords  int      `json:"currentWords"`
	TargetWords   int      `json:"targetWords"`
	QualifiedMin  int      `json:"qualifiedMin"`
	QualifiedMax  int      `json:"qualifiedMax"`
	GapWords      int      `json:"gapWords"`
	Status        string   `json:"status"` // qualified | too_short | too_long | unknown
	ScoreWeight   float64  `json:"scoreWeight"`
	Warnings      []string `json:"warnings,omitempty"`
}

// CheckWordCountRequest 字数检查请求
type CheckWordCountRequest struct {
	ChapterPaths      []string `json:"chapterPaths"`
	TaskBookPath      string   `json:"taskBookPath"`
	ScoringReportPath string   `json:"scoringReportPath"`
	OutputReportPath  string   `json:"outputReportPath"`
	Mode              string   `json:"mode,omitempty"` // strict | draft
}

// CheckWordCountResponse 字数检查响应
type CheckWordCountResponse struct {
	ReportPath string          `json:"reportPath"`
	Items      []WordCountItem `json:"items"`
	Artifact   map[string]any  `json:"artifact"`
	Warnings   []string        `json:"warnings"`
}

// ExpandChapterRequest 章节扩写请求
type ExpandChapterRequest struct {
	SourcePath    string `json:"sourcePath"`
	ChapterNumber int    `json:"chapterNumber"`
	ChapterTitle  string `json:"chapterTitle"`
	TargetWords   int    `json:"targetWords"`
	GapWords      int    `json:"gapWords"`
}

// ── Regex patterns ──

var pageCountRe = regexp.MustCompile(`(?:页数|页码|总页数|投标文件.*页|技术标.*页|页\s*数)[：:]\s*(\d+)\s*页?`)

const (
	defaultPageCount    = 300
	wordsPerPage        = 780
	qualifiedLowerRatio = 0.75
	qualifiedUpperRatio = 1.25
)

// ── Word count calculation ──

// extractTotalPages 从解析报告提取页数要求
// 优先匹配"页数"等字段，失败返回默认值 300
func extractTotalPages(analysisContent string) (int, bool) {
	m := pageCountRe.FindStringSubmatch(analysisContent)
	if m == nil {
		return defaultPageCount, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return defaultPageCount, false
	}
	return n, true
}

// calculateTargetWordCounts 计算各章字数目标
// 优先级：
//  1. 从任务书 estimatedWordCount 提取（复用已有函数）
//  2. 公式计算：评分分值 × (页数 / 总分) × 780
//  3. 均分 50000 字（最终降级）
func calculateTargetWordCounts(
	taskBookContent, scoringContent, analysisContent string,
	chapterCount int,
	perChapterWordCounts map[int]int,
) (map[int]int, int, []string) {
	wordCounts := make(map[int]int)
	var warnings []string

	// 1. 优先 from task book estimatedWordCount
	if len(perChapterWordCounts) == chapterCount {
		return perChapterWordCounts, sumMap(perChapterWordCounts), warnings
	}

	// 2. 公式计算
	totalPages, hasPages := extractTotalPages(analysisContent)
	if !hasPages {
		warnings = append(warnings, fmt.Sprintf(
			"未从招标文件解析报告提取到页数要求，按 %d 页测算（非事实，仅为字数测算假设）", defaultPageCount,
		))
	}

	totalScore, hasScore := extractTotalScore(scoringContent)
	if !hasScore {
		warnings = append(warnings, "未从评分标准提取到总分值，使用均分策略")
		perChapter := defaultTotalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, defaultTotalWordCount, warnings
	}

	// 3. 提取章节目录分值
	chapterScores := extractChapterScores(taskBookContent, chapterCount)
	if len(chapterScores) == 0 {
		warnings = append(warnings, "未从任务书提取到章节分值，使用均分策略")
		perChapter := defaultTotalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, defaultTotalWordCount, warnings
	}

	coveredScore := 0
	for _, s := range chapterScores {
		coveredScore += s
	}
	if coveredScore == 0 {
		perChapter := defaultTotalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, defaultTotalWordCount, warnings
	}

	totalWordCount := 0
	for i := 1; i <= chapterCount; i++ {
		score := chapterScores[i]
		if score == 0 {
			avg := defaultTotalWordCount / chapterCount
			wordCounts[i] = avg
			warnings = append(warnings, fmt.Sprintf("第%d章未提取到评分分值，使用均分字数 %d", i, avg))
		} else {
			wordCounts[i] = score * totalPages * wordsPerPage / totalScore / coveredScore
		}
		totalWordCount += wordCounts[i]
	}

	return wordCounts, totalWordCount, warnings
}

// checkWordCountForChapter 检查单章字数是否达标
func checkWordCountForChapter(chapterPath string, chapterNumber int, targetWords int,
) (WordCountItem, error) {
	content, err := os.ReadFile(chapterPath)
	if err != nil {
		return WordCountItem{}, fmt.Errorf("failed to read chapter file: %w", err)
	}

	currentWords := countChineseMarkdownWords(string(content))
	qualifiedMin := int(float64(targetWords) * qualifiedLowerRatio)
	qualifiedMax := int(float64(targetWords) * qualifiedUpperRatio)

	status := "qualified"
	if currentWords < qualifiedMin {
		status = "too_short"
	} else if currentWords > qualifiedMax {
		status = "too_long"
	}

	gapWords := 0
	if status == "too_short" {
		gapWords = qualifiedMin - currentWords
	}

	return WordCountItem{
		ChapterNumber: chapterNumber,
		FilePath:      chapterPath,
		CurrentWords:  currentWords,
		TargetWords:   targetWords,
		QualifiedMin:  qualifiedMin,
		QualifiedMax:  qualifiedMax,
		GapWords:      gapWords,
		Status:        status,
	}, nil
}

// ── Chinese word counting ──

var (
	markdownSymbolRe = regexp.MustCompile(`[#*_\-\>\|\[\]\(\)~` + "`" + `]`)
	htmlCommentRe    = regexp.MustCompile(`<!--[\s\S]*?-->`)
)

// countChineseMarkdownWords 统计中文 Markdown 内容的有效字数
//   - 去除 HTML 注释
//   - 中文汉字计数
//   - 英文单词按 token 计数
//   - 数字和标点不计入字数
func countChineseMarkdownWords(text string) int {
	text = htmlCommentRe.ReplaceAllString(text, "")
	text = markdownSymbolRe.ReplaceAllString(text, " ")

	count := 0
	inWord := false
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			// CJK Unified Ideographs
			count++
			inWord = true
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if !inWord {
				count++
				inWord = true
			}
		} else if r >= '0' && r <= '9' {
			inWord = true // numbers keep inWord but don't add count
		} else {
			inWord = false
		}
	}
	return count
}

// ── Helpers ──

func sumMap(m map[int]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// generateWordCountReport 生成字数检查报告 Markdown
func generateWordCountReport(items []WordCountItem) string {
	var sb strings.Builder
	sb.WriteString("# 章节字数检查报告\n\n")
	sb.WriteString("| 章节 | 标题 | 当前字数 | 目标字数 | 合格下限 | 合格上限 | 缺口 | 状态 |\n")
	sb.WriteString("|------|------|----------|----------|----------|----------|------|------|\n")

	for _, item := range items {
		statusText := "✅ 合格"
		if item.Status == "too_short" {
			statusText = "⚠️ 不足"
		} else if item.Status == "too_long" {
			statusText = "⚠️ 超量"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %d | %d | %d | %d | %d | %s |\n",
			item.ChapterNumber, item.ChapterTitle,
			item.CurrentWords, item.TargetWords,
			item.QualifiedMin, item.QualifiedMax,
			item.GapWords, statusText,
		))
	}
	return sb.String()
}

// ── Main service entry point ──

// CheckWordCount 字数检查主入口
//  1. 从任务书提取各章 estimatedWordCount
//  2. 计算各章目标字数
//  3. 统计每章实际字数
//  4. 生成检查报告 Markdown
func CheckWordCount(req CheckWordCountRequest) (*CheckWordCountResponse, error) {
	if len(req.ChapterPaths) == 0 {
		return nil, fmt.Errorf("chapterPaths is empty")
	}

	// 1. Read task book for per-chapter estimated word counts
	taskBookContent, err := os.ReadFile(req.TaskBookPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task book: %w", err)
	}
	perChapterWordCounts := extractChapterWordCountsFromTaskBook(string(taskBookContent), len(req.ChapterPaths))

	// 2. Read scoring report for score-based calculation as fallback
	scoringContent := ""
	if req.ScoringReportPath != "" {
		b, err := os.ReadFile(req.ScoringReportPath)
		if err == nil {
			scoringContent = string(b)
		}
	}

	// 3. Read analysis report for page count extraction
	analysisContent := ""
	if req.ScoringReportPath != "" {
		// Try 00_招标文件解析报告.md (inferred from output dir)
		analysisPath := strings.Replace(req.ScoringReportPath, "02_评分标准拆解表.md", "00_招标文件解析报告.md", 1)
		b, err := os.ReadFile(analysisPath)
		if err == nil {
			analysisContent = string(b)
		}
	}

	chapterCount := len(req.ChapterPaths)

	// 4. Calculate target word counts per chapter
	targetWordCounts, totalWords, warnings := calculateTargetWordCounts(
		string(taskBookContent), scoringContent, analysisContent,
		chapterCount, perChapterWordCounts,
	)

	// 5. Extract chapter titles from task book
	chapterTitles := extractChapterTitles(string(taskBookContent))

	// 6. Check each chapter
	var items []WordCountItem
	shortCount, qualifiedCount, longCount := 0, 0, 0

	for i, chapterPath := range req.ChapterPaths {
		chapterNumber := i + 1
		targetWords := targetWordCounts[chapterNumber]
		title := chapterTitles[chapterNumber]
		if title == "" {
			title = fmt.Sprintf("第%d章", chapterNumber)
		}

		item, err := checkWordCountForChapter(chapterPath, chapterNumber, targetWords)
		if err != nil {
			zap.L().Warn("failed to check chapter word count",
				zap.String("path", chapterPath),
				zap.Error(err),
			)
			item = WordCountItem{
				ChapterNumber: chapterNumber,
				ChapterTitle:  title,
				FilePath:      chapterPath,
				TargetWords:   targetWords,
				QualifiedMin:  int(float64(targetWords) * qualifiedLowerRatio),
				QualifiedMax:  int(float64(targetWords) * qualifiedUpperRatio),
				Status:        "unknown",
				Warnings:      []string{err.Error()},
			}
		}
		item.ChapterTitle = title
		items = append(items, item)

		switch item.Status {
		case "too_short":
			shortCount++
		case "too_long":
			longCount++
		default:
			qualifiedCount++
		}
	}

	warnings = append(warnings, fmt.Sprintf(
		"检查完成: %d 章合格, %d 章不足, %d 章超量, 总计目标 %d 字",
		qualifiedCount, shortCount, longCount, totalWords,
	))

	// 7. Generate report
	reportContent := generateWordCountReport(items)
	if req.OutputReportPath == "" {
		req.OutputReportPath = strings.Replace(
			req.ChapterPaths[0],
			"章节/",
			"",
			1,
		)
		idx := strings.LastIndex(req.OutputReportPath, "/")
		if idx >= 0 {
			req.OutputReportPath = req.OutputReportPath[:idx+1]
		} else {
			req.OutputReportPath = ""
		}
		req.OutputReportPath += "90_章节字数检查报告.md"
	}

	if err := os.WriteFile(req.OutputReportPath, []byte(reportContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write word count report: %w", err)
	}

	artifact := map[string]any{
		"id":         "word-count-report",
		"kind":       "BID_WORD_COUNT_REPORT",
		"name":       "章节字数检查报告",
		"storageRef": req.OutputReportPath,
		"mimeType":   "text/markdown",
		"status":     "valid",
		"version":    "1.0",
		"sourceTool": "word_count_check",
		"metadata": map[string]any{
			"totalWords":     totalWords,
			"chapterCount":   chapterCount,
			"shortCount":     shortCount,
			"qualifiedCount": qualifiedCount,
			"longCount":      longCount,
			"generatedAt":    time.Now().Format(time.RFC3339),
		},
	}

	return &CheckWordCountResponse{
		ReportPath: req.OutputReportPath,
		Items:      items,
		Artifact:   artifact,
		Warnings:   warnings,
	}, nil
}

// extractChapterTitles returns a map of chapterNumber → title from the task book.
func extractChapterTitles(taskBookContent string) map[int]string {
	titles := make(map[int]string)
	chapterSectionRe := regexp.MustCompile(`###\s*第([一二三四五六七八九十百千]+)章[：:]\s*(.+)`)
	matches := chapterSectionRe.FindAllStringSubmatch(taskBookContent, -1)
	for _, m := range matches {
		num := chineseToNumber(m[1])
		if num > 0 {
			titles[num] = strings.TrimSpace(m[2])
		}
	}
	return titles
}

var chineseNumMap = map[rune]int{
	'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
	'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
}

func chineseToNumber(s string) int {
	result := 0
	multiplier := 1
	for _, r := range []rune(s) {
		if v, ok := chineseNumMap[r]; ok {
			if v == 10 && result == 0 {
				result = 10
			} else if v == 10 {
				result *= v
			} else {
				result += v * multiplier
			}
		} else if r == '百' {
			multiplier = 100
		} else if r == '千' {
			multiplier = 1000
		}
	}
	return result
}
