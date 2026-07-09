package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

const chapterWritingSystemPrompt = `你是一个专业的标书技术标章节撰写专家。

你需要根据《章节写作任务书》《技术标四级大纲》《评分标准拆解表》《招标文件解析报告》《项目背景确认表》，撰写指定章节的完整正文。

## 写作规则

### 格式要求
- 使用 Markdown 输出
- 章节序号统一使用中文数字（一、二、三…），禁止混用阿拉伯数字
- 正文前加入格式注释块：
  <!-- doc-format
  font: SimSun
  body-size: 16pt
  title-level: 36pt
  sub-level: 32pt
  line-spacing: 26pt
  margins: 2cm
  first-line-indent: 0.74cm
  -->

### 内容结构
- 每小节（### N.N.N）必须有 ≥ 3 个独立段落，禁止单一长段落
- 每个章节尽量包含表格，不能全是纯文字
- 严格按评分标准和采购需求编写，不泛泛而谈
- 章节正文写完即结束，禁止添加"本章小结""本章总结"等总结段

### 用词约束
- 严禁出现"我方""我们"；统一使用"将""项目组""本方案"
- 严禁出现报价、预算金额、投标总价、价格承诺等商务内容
- 不要编造招标文件、评分标准或项目背景中不存在的内容
- 引用招标文件要求时，使用"招标文件要求…"或"根据招标文件第X章…"

### 质量要求
- 必须逐一回应本章对应的评分项
- 内容贴合当前招标项目的实际情况
- 技术措施具体可操作，避免空泛描述
- 如果任务书中标注了"禁止内容"，本章严禁涉及`

const (
	chapterWritingMaxTokens  = 8000
	chapterTaskBookRuneLimit = 20000
	chapterOutlineRuneLimit  = 15000
	chapterScoringRuneLimit  = 8000
	chapterAnalysisRuneLimit = 8000
	chapterContextRuneLimit  = 8000
	chapterConcurrency       = 3
	defaultTotalWordCount    = 50000
)

// ── Data structures ──

// GenerateChaptersRequest 批量生成请求
type GenerateChaptersRequest struct {
	TaskBookPath       string `json:"taskBookPath"`
	OutlinePath        string `json:"outlinePath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextReportPath  string `json:"contextReportPath,omitempty"`
	OutputDir          string `json:"outputDir"`
	Mode               string `json:"mode,omitempty"`
}

// ChapterInfo 从大纲提取的章节信息
type ChapterInfo struct {
	Number         int    // 1-based 序号
	Title          string // "一、总体项目管理方案"
	OutlineSection string // 大纲中该章的完整子树
}

// ChapterResult 单个章节生成结果
type ChapterResult struct {
	ChapterNumber int                    `json:"chapterNumber"`
	ChapterTitle  string                 `json:"chapterTitle"`
	FilePath      string                 `json:"filePath"`
	Content       string                 `json:"content,omitempty"`
	Artifact      map[string]interface{} `json:"artifact"`
	Error         string                 `json:"error,omitempty"`
	WordCount     int                    `json:"wordCount"`
	ScoreWeight   float64                `json:"scoreWeight"`
}

// GenerateChaptersResponse 批量生成响应
type GenerateChaptersResponse struct {
	Chapters       []ChapterResult `json:"chapters"`
	Success        int             `json:"success"`
	Failed         int             `json:"failed"`
	Warnings       []string        `json:"warnings"`
	TotalWordCount int             `json:"totalWordCount"`
	TotalScore     int             `json:"totalScore"`
}

// ── Regex patterns ──

var (
	chapterTitleRe = regexp.MustCompile(`^##\s*[一二三四五六七八九十]+`)
	totalScoreRe   = regexp.MustCompile(`(?:总分|合计|满分)[：:]\s*(\d+)\s*分?`)
	itemScoreRe    = regexp.MustCompile(`\((\d+)\s*分\)`)
	totalWordsRe   = regexp.MustCompile(`总字数预估[：:]\s*([\d,]+)`)
)

// ── Chapter extraction from outline ──

// extractChaptersFromOutline 从大纲 markdown 提取一级章节列表
func extractChaptersFromOutline(outlineText string) ([]ChapterInfo, error) {
	lines := strings.Split(outlineText, "\n")
	zap.L().Info("extractChaptersFromOutline",
		zap.Int("totalLines", len(lines)),
	)
	// Debug: log first 5 lines that start with #
	debugCount := 0
	for _, l := range lines {
		if debugCount >= 5 {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(l), "##") {
			debugCount++
			zap.L().Info("outline line",
				zap.Int("n", debugCount),
				zap.String("line", l[:min(len(l), 80)]),
				zap.Bool("regexMatch", chapterTitleRe.MatchString(l)),
			)
		}
	}
	var chapters []ChapterInfo
	var current *ChapterInfo
	var currentLines []string
	idx := 0

	flushCurrent := func() {
		if current != nil {
			current.OutlineSection = strings.Join(currentLines, "\n")
			chapters = append(chapters, *current)
		}
		current = nil
		currentLines = nil
	}

	for _, line := range lines {
		if chapterTitleRe.MatchString(line) {
			flushCurrent()
			idx++
			title := strings.TrimLeft(line, "# ")
			title = strings.TrimSpace(title)
			current = &ChapterInfo{Number: idx, Title: title}
			currentLines = append(currentLines, line)
		} else if current != nil {
			currentLines = append(currentLines, line)
		}
	}
	flushCurrent()

	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapter headings found in outline")
	}
	return chapters, nil
}

// ── Score extraction ──

// extractTotalScore 从评分标准拆解表提取总分值
func extractTotalScore(scoringContent string) (int, bool) {
	m := totalScoreRe.FindStringSubmatch(scoringContent)
	if len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// extractTotalWordCount 从任务书提取总字数预估
func extractTotalWordCount(taskBookContent string) (int, bool) {
	m := totalWordsRe.FindStringSubmatch(taskBookContent)
	if len(m) >= 2 {
		s := strings.ReplaceAll(m[1], ",", "")
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// extractChapterScores 从任务书提取每章的分值
// 解析任务书中每章的"对应评分项"区块，累加其中 (N分) 标记
func extractChapterScores(taskBookContent string, chapterCount int) map[int]int {
	scores := make(map[int]int)

	// 按章节分割任务书内容
	// 任务书中的章节标记可能是 "### 章节编号：一" 或 "**章节编号**：一" 等形式
	// 使用宽松匹配：找到数字序号后提取该章的评分项
	sectionRe := regexp.MustCompile(`(?i)(?:章节编号|章节)[：:]\s*([一二三四五六七八九十]+)`)
	sections := sectionRe.FindAllStringSubmatchIndex(taskBookContent, -1)

	if len(sections) == 0 {
		return scores
	}

	// 中文数字 → 阿拉伯数字映射
	cnToNum := map[string]int{
		"一": 1, "二": 2, "三": 3, "四": 4, "五": 5,
		"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
		"十一": 11, "十二": 12, "十三": 13, "十四": 14, "十五": 15,
	}

	for i, sec := range sections {
		// 提取中文序号
		if len(sec) < 4 {
			continue
		}
		cnNum := taskBookContent[sec[2]:sec[3]]
		num, ok := cnToNum[cnNum]
		if !ok {
			continue
		}

		// 提取该章节区块的文本（从当前章节到下一章节或结尾）
		start := sec[1]
		var end int
		if i+1 < len(sections) {
			end = sections[i+1][0]
		} else {
			end = len(taskBookContent)
		}
		sectionText := taskBookContent[start:end]

		// 在该区块内查找 (N分)
		scoreMatches := itemScoreRe.FindAllStringSubmatch(sectionText, -1)
		chapterScore := 0
		for _, sm := range scoreMatches {
			if n, err := strconv.Atoi(sm[1]); err == nil {
				chapterScore += n
			}
		}
		if chapterScore > 0 {
			scores[num] = chapterScore
		}
	}

	return scores
}

// calculateChapterWordCounts 计算每章目标字数
func calculateChapterWordCounts(
	scoringContent string,
	taskBookContent string,
	chapterCount int,
) (map[int]int, int, int, []string) {
	wordCounts := make(map[int]int)
	var warnings []string

	// 1. 总目标字数
	totalWordCount, found := extractTotalWordCount(taskBookContent)
	if !found {
		totalWordCount = defaultTotalWordCount
		warnings = append(warnings, "未从任务书提取到总字数预估，使用默认值 50000 字")
	}

	// 2. 总分值
	totalScore, found := extractTotalScore(scoringContent)
	if !found {
		warnings = append(warnings, "未从评分标准提取到总分值，使用均分策略")
		// 均分
		perChapter := totalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, totalWordCount, 0, warnings
	}

	// 3. 每章分值
	chapterScores := extractChapterScores(taskBookContent, chapterCount)
	if len(chapterScores) == 0 {
		warnings = append(warnings, "未从任务书提取到章节分值，使用均分策略")
		perChapter := totalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, totalWordCount, totalScore, warnings
	}

	// 4. 按权重分配
	coveredScore := 0
	for _, s := range chapterScores {
		coveredScore += s
	}
	if coveredScore == 0 {
		perChapter := totalWordCount / chapterCount
		for i := 1; i <= chapterCount; i++ {
			wordCounts[i] = perChapter
		}
		return wordCounts, totalWordCount, totalScore, warnings
	}

	for i := 1; i <= chapterCount; i++ {
		score := chapterScores[i]
		if score == 0 {
			// 该章无分值 → 均分剩余字数
			wordCounts[i] = totalWordCount / chapterCount
			warnings = append(warnings, fmt.Sprintf("第%d章未提取到评分分值，使用均分字数", i))
		} else {
			wordCounts[i] = totalWordCount * score / coveredScore
		}
	}

	return wordCounts, totalWordCount, totalScore, warnings
}

// ── Prompt builders ──

func buildChapterUserPrompt(
	taskBookText, outlineSection, scoringText, analysisText, contextText string,
	chapterTitle string,
	wordCount, chapterScore, totalScore int,
) string {
	compactTaskBook := limitTextForPrompt(taskBookText, chapterTaskBookRuneLimit, "任务书")
	compactOutline := outlineSection
	compactScoring := limitTextForPrompt(scoringText, chapterScoringRuneLimit, "评分标准")
	compactAnalysis := limitTextForPrompt(
		stripBidAnalysisAppendix(analysisText),
		chapterAnalysisRuneLimit,
		"解析报告",
	)

	var sb strings.Builder
	sb.WriteString("## 章节任务\n\n")
	sb.WriteString("以下是《章节写作任务书》的完整内容，请仔细阅读后撰写【")
	sb.WriteString(chapterTitle)
	sb.WriteString("】：\n\n")
	sb.WriteString(compactTaskBook)
	sb.WriteString("\n\n---\n\n## 大纲结构\n\n")
	sb.WriteString("以下是技术标大纲中【")
	sb.WriteString(chapterTitle)
	sb.WriteString("】的详细结构：\n\n")
	sb.WriteString(compactOutline)
	sb.WriteString("\n\n---\n\n## 评分标准\n\n")
	sb.WriteString("本章需要回应的评分项：\n\n")
	sb.WriteString(compactScoring)

	if strings.TrimSpace(contextText) != "" {
		compactContext := limitTextForPrompt(contextText, chapterContextRuneLimit, "项目背景")
		sb.WriteString("\n\n---\n\n## 项目背景\n\n")
		sb.WriteString(compactContext)
	}

	sb.WriteString("\n\n---\n\n## 招标文件核心要求\n\n")
	sb.WriteString(compactAnalysis)

	sb.WriteString("\n\n---\n\n请按以上信息撰写【")
	sb.WriteString(chapterTitle)
	sb.WriteString("】的完整正文")

	if totalScore > 0 && chapterScore > 0 {
		ratio := float64(chapterScore) / float64(totalScore) * 100
		sb.WriteString(fmt.Sprintf("，本章评分权重 %.0f%%（%d分/%d分）", ratio, chapterScore, totalScore))
	}
	sb.WriteString(fmt.Sprintf("，目标字数 %d 字。", wordCount))

	return sb.String()
}

// ── Output validation ──

func validateChapterContent(content string) []string {
	var warnings []string
	forbiddenWords := []string{"本章小结", "本章总结", "我方", "我们", "报价", "预算金额", "投标总价", "价格承诺"}
	for _, w := range forbiddenWords {
		if strings.Contains(content, w) {
			warnings = append(warnings, fmt.Sprintf("内容包含禁止词汇: %s", w))
		}
	}
	return warnings
}

// ── Main service ──

// GenerateChapters 并发生成所有章节
func GenerateChapters(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateChaptersRequest,
) (*GenerateChaptersResponse, error) {
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}
	if req.TaskBookPath == "" {
		return nil, fmt.Errorf("taskBookPath is required")
	}
	if req.OutlinePath == "" {
		return nil, fmt.Errorf("outlinePath is required")
	}
	if req.OutputDir == "" {
		return nil, fmt.Errorf("outputDir is required")
	}

	// 1. Read all input files
	taskBookText, err := readFileIfExists(req.TaskBookPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read task book: %w", err)
	}
	outlineText, err := readFileIfExists(req.OutlinePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read outline: %w", err)
	}

	var scoringText, analysisText, contextText string
	if req.ScoringReportPath != "" {
		scoringText, err = readFileIfExists(req.ScoringReportPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read scoring report: %w", err)
		}
	}
	if req.AnalysisReportPath != "" {
		analysisText, err = readFileIfExists(req.AnalysisReportPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read analysis report: %w", err)
		}
	}
	if req.ContextReportPath != "" {
		contextText, _ = readFileIfExists(req.ContextReportPath)
	}

	// 2. Extract chapters from outline
	chapters, err := extractChaptersFromOutline(outlineText)
	if err != nil {
		return nil, fmt.Errorf("cannot extract chapters from outline: %w", err)
	}

	// 3. Calculate word counts by scoring weight
	wordCounts, totalWordCount, totalScore, calcWarnings := calculateChapterWordCounts(
		scoringText, taskBookText, len(chapters),
	)

	// 4. Prepare output directory
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// 5. Generate chapters concurrently
	results := make([]ChapterResult, len(chapters))
	var wg sync.WaitGroup
	sem := make(chan struct{}, chapterConcurrency)
	var mu sync.Mutex

	start := time.Now()
	zap.L().Info("biaoshu chapter generation starting",
		zap.Int("chapterCount", len(chapters)),
		zap.Int("totalWordCount", totalWordCount),
		zap.Int("totalScore", totalScore),
	)

	for i, ch := range chapters {
		wg.Add(1)
		go func(idx int, info ChapterInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := generateOneChapter(ctx, gw, info, taskBookText, outlineText,
				scoringText, analysisText, contextText,
				wordCounts, totalScore, req.OutputDir)

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, ch)
	}
	wg.Wait()

	zap.L().Info("biaoshu chapter generation completed",
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.Int("chapterCount", len(chapters)),
	)

	// 6. Aggregate results
	resp := &GenerateChaptersResponse{
		Chapters:       results,
		Warnings:       calcWarnings,
		TotalWordCount: totalWordCount,
		TotalScore:     totalScore,
	}
	for _, r := range results {
		if r.Error != "" {
			resp.Failed++
		} else {
			resp.Success++
		}
	}

	return resp, nil
}

// generateOneChapter 生成单个章节
func generateOneChapter(
	ctx context.Context,
	gw *modelgateway.Gateway,
	info ChapterInfo,
	taskBookText, outlineText, scoringText, analysisText, contextText string,
	wordCounts map[int]int,
	totalScore int,
	outputDir string,
) ChapterResult {
	wordCount := wordCounts[info.Number]
	if wordCount == 0 {
		wordCount = defaultTotalWordCount / 10 // fallback
	}

	// 提取该章在大纲中的子树
	outlineSection := extractOutlineSubtree(outlineText, info.Title)

	// 计算该章评分权重
	var chapterScore int
	chapterScores := extractChapterScores(taskBookText, 0) // 第二个参数在此仅用于预分配
	if s, ok := chapterScores[info.Number]; ok {
		chapterScore = s
	}
	scoreWeight := float64(0)
	if totalScore > 0 {
		scoreWeight = float64(chapterScore) / float64(totalScore)
	}

	userPrompt := buildChapterUserPrompt(
		taskBookText, outlineSection, scoringText, analysisText, contextText,
		info.Title, wordCount, chapterScore, totalScore,
	)

	// 生成文件名：{序号}_{章节名}_初稿.md
	safeTitle := strings.TrimLeft(info.Title, "一二三四五六七八九十、 ")
	safeTitle = strings.TrimSpace(safeTitle)
	if safeTitle == "" {
		safeTitle = info.Title
	}
	// 移除文件名不安全字符
	safeTitle = strings.NewReplacer("/", "-", "\\", "-", ":", "：", "?", "", "*", "", "\"", "", "<", "", ">", "", "|", "").Replace(safeTitle)
	fileName := fmt.Sprintf("%02d_%s_初稿.md", info.Number, safeTitle)
	filePath := filepath.Join(outputDir, fileName)

	start := time.Now()
	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: chapterWritingSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  float64(chapterWritingMaxTokens),
		},
	})
	if err != nil {
		return ChapterResult{
			ChapterNumber: info.Number,
			ChapterTitle:  info.Title,
			FilePath:      filePath,
			Error:         fmt.Sprintf("LLM call failed: %v", err),
		}
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return ChapterResult{
			ChapterNumber: info.Number,
			ChapterTitle:  info.Title,
			FilePath:      filePath,
			Error:         "LLM returned empty content",
		}
	}

	// Validate output
	validationWarnings := validateChapterContent(content)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return ChapterResult{
			ChapterNumber: info.Number,
			ChapterTitle:  info.Title,
			FilePath:      filePath,
			Error:         fmt.Sprintf("failed to write file: %v", err),
		}
	}

	wc := len([]rune(content))
	zap.L().Info("biaoshu chapter generated",
		zap.Int("chapter", info.Number),
		zap.String("title", info.Title),
		zap.Int("wordCount", wc),
		zap.Int("targetWordCount", wordCount),
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.String("model", result.Usage.Model),
		zap.Int("promptTokens", result.Usage.PromptTokens),
		zap.Int("outputTokens", result.Usage.OutputTokens),
	)

	artifact := map[string]interface{}{
		"unitId":       fmt.Sprintf("chapter-%d", info.Number),
		"kind":         "BID_CHAPTERS",
		"name":         info.Title,
		"chapterTitle": info.Title,
		"storageRef":   filePath,
		"mimeType":     "text/markdown",
		"metadata": map[string]interface{}{
			"chapterNumber": info.Number,
			"chapterTitle":  info.Title,
			"wordCount":     wc,
			"targetWords":   wordCount,
			"scoreWeight":   scoreWeight,
			"warnings":      validationWarnings,
			"status":        "valid",
		},
	}

	return ChapterResult{
		ChapterNumber: info.Number,
		ChapterTitle:  info.Title,
		FilePath:      filePath,
		Content:       content,
		Artifact:      artifact,
		WordCount:     wc,
		ScoreWeight:   scoreWeight,
	}
}

// extractOutlineSubtree 从大纲中提取指定章节的子树
func extractOutlineSubtree(outlineText, chapterTitle string) string {
	lines := strings.Split(outlineText, "\n")
	inSection := false
	var sb strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if strings.Contains(trimmed, chapterTitle) || strings.Contains(trimmed, strings.TrimLeft(chapterTitle, "# ")) {
				inSection = true
				sb.WriteString(line)
				sb.WriteString("\n")
				continue
			}
			if inSection {
				break
			}
		}
		if inSection {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
