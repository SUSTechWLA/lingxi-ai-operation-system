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

const outlineSystemPrompt = `你是一个专业的标书大纲规划专家。请根据提供的《招标文件解析报告》和《项目背景信息确认表》，生成技术标四级大纲。

大纲必须：
1. 严格按照招标文件评分标准组织章节。
2. 结合项目背景信息，在相关章节体现项目属地特征。
3. 不编造投标单位资质、人员、业绩和设备数量——仅基于背景确认表中的信息。
4. 使用四级标题结构：
   - ## 一级标题（章节）
   - ### 二级标题（节）
   - #### 三级标题（子节）
   - ##### 四级标题（要点）
5. 在每级标题下补充1-2句话说明该章节应覆盖的内容范围。
6. 输出整洁的 Markdown，不要包含代码块围栏。`

const (
	outlineAnalysisPromptRuneLimit = 18000
	outlineContextPromptRuneLimit  = 8000
	outlineScoringPromptRuneLimit  = 6000
	outlineMaxTokens               = 12000
)

type outlinePromptMetadata struct {
	AnalysisOriginalRunes int
	AnalysisPromptRunes   int
	ContextOriginalRunes  int
	ContextPromptRunes    int
	ScoringOriginalRunes  int
	ScoringPromptRunes    int
}

func stripBidAnalysisAppendix(text string) string {
	markers := []string{
		"\n## 附录：招标文件原文",
		"\n# 附录：招标文件原文",
		"\n---\n\n## 附录：招标文件原文",
	}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return strings.TrimSpace(text)
}

func limitTextForPrompt(text string, maxRunes int, label string) string {
	trimmed := strings.TrimSpace(text)
	if maxRunes <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	kept := strings.TrimSpace(string(runes[:maxRunes]))
	return fmt.Sprintf("%s\n\n> 注：%s已截断，仅保留前 %d 字用于本次大纲生成。", kept, label, maxRunes)
}

func buildOutlineUserPrompt(analysisText, contextText, scoringText string) (string, outlinePromptMetadata) {
	analysisOriginalRunes := len([]rune(analysisText))
	contextOriginalRunes := len([]rune(contextText))
	scoringOriginalRunes := len([]rune(scoringText))

	compactAnalysis := limitTextForPrompt(
		stripBidAnalysisAppendix(analysisText),
		outlineAnalysisPromptRuneLimit,
		"招标文件解析报告",
	)
	compactContext := limitTextForPrompt(
		contextText,
		outlineContextPromptRuneLimit,
		"项目背景信息确认表",
	)
	compactScoring := ""
	if strings.TrimSpace(scoringText) != "" {
		compactScoring = "\n\n## 评分标准拆解表\n\n" + limitTextForPrompt(
			scoringText,
			outlineScoringPromptRuneLimit,
			"评分标准拆解表",
		)
	}

	prompt := fmt.Sprintf(
		"## 招标文件解析报告\n\n%s\n\n## 项目背景信息确认表\n\n%s%s",
		compactAnalysis,
		compactContext,
		compactScoring,
	)

	return prompt, outlinePromptMetadata{
		AnalysisOriginalRunes: analysisOriginalRunes,
		AnalysisPromptRunes:   len([]rune(compactAnalysis)),
		ContextOriginalRunes:  contextOriginalRunes,
		ContextPromptRunes:    len([]rune(compactContext)),
		ScoringOriginalRunes:  scoringOriginalRunes,
		ScoringPromptRunes:    len([]rune(compactScoring)),
	}
}

// GenerateOutlineRequest is the input for outline generation.
type GenerateOutlineRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextReportPath  string `json:"contextReportPath"`
	ScoringReportPath  string `json:"scoringReportPath,omitempty"`
	OutlinePath        string `json:"outlinePath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

// GenerateOutlineResponse is the output.
type GenerateOutlineResponse struct {
	AnalysisReportPath string                 `json:"analysisReportPath"`
	ContextReportPath  string                 `json:"contextReportPath"`
	OutlinePath        string                 `json:"outlinePath"`
	Content            string                 `json:"content"`
	Artifact           map[string]interface{} `json:"artifact"`
}

// GenerateOutline generates the technical bid outline from analysis report and project context.
func GenerateOutline(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateOutlineRequest,
) (*GenerateOutlineResponse, error) {
	analysisReportPath := strings.TrimSpace(req.AnalysisReportPath)
	contextReportPath := strings.TrimSpace(req.ContextReportPath)
	outlinePath := strings.TrimSpace(req.OutlinePath)
	sourceFile := strings.TrimSpace(req.SourceFile)

	if analysisReportPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}
	if contextReportPath == "" {
		return nil, fmt.Errorf("contextReportPath is required")
	}
	if outlinePath == "" {
		return nil, fmt.Errorf("outlinePath is required")
	}
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	// Read analysis report
	analysisBytes, err := os.ReadFile(analysisReportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read analysis report: %w", err)
	}

	// Read context report
	contextBytes, err := os.ReadFile(contextReportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read project context report: %w", err)
	}

	// Optionally read scoring report
	var scoringText string
	scoringReportPath := strings.TrimSpace(req.ScoringReportPath)
	if scoringReportPath != "" {
		if scoringBytes, err := os.ReadFile(scoringReportPath); err == nil {
			scoringText = string(scoringBytes)
		}
	}

	userPrompt, promptMeta := buildOutlineUserPrompt(string(analysisBytes), string(contextBytes), scoringText)

	start := time.Now()
	zap.L().Info("biaoshu outline generation model call starting",
		zap.Int("analysisOriginalRunes", promptMeta.AnalysisOriginalRunes),
		zap.Int("analysisPromptRunes", promptMeta.AnalysisPromptRunes),
		zap.Int("contextOriginalRunes", promptMeta.ContextOriginalRunes),
		zap.Int("contextPromptRunes", promptMeta.ContextPromptRunes),
		zap.Int("scoringOriginalRunes", promptMeta.ScoringOriginalRunes),
		zap.Int("scoringPromptRunes", promptMeta.ScoringPromptRunes),
		zap.Int("maxTokens", outlineMaxTokens),
	)

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: outlineSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  float64(outlineMaxTokens),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	zap.L().Info("biaoshu outline generation model call completed",
		zap.Int64("durationMs", time.Since(start).Milliseconds()),
		zap.String("model", result.Usage.Model),
		zap.Int("promptTokens", result.Usage.PromptTokens),
		zap.Int("outputTokens", result.Usage.OutputTokens),
	)

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty outline")
	}

	if err := os.MkdirAll(filepath.Dir(outlinePath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create outline directory: %w", err)
	}
	if err := os.WriteFile(outlinePath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write outline: %w", err)
	}

	artifact := map[string]interface{}{
		"unitId":     "bid-outline",
		"kind":       "BID_OUTLINE",
		"name":       filepath.Base(outlinePath),
		"mimeType":   "text/markdown",
		"storageRef": outlinePath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}

	return &GenerateOutlineResponse{
		AnalysisReportPath: analysisReportPath,
		ContextReportPath:  contextReportPath,
		OutlinePath:        outlinePath,
		Content:            content,
		Artifact:           artifact,
	}, nil
}
