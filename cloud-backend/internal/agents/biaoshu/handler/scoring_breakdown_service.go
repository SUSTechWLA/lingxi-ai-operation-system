package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

const scoringBreakdownSystemPrompt = `你是专业的投标技术标评分标准拆解专家。请根据《招标文件解析报告》中的评分办法、技术要求、投标文件组成要求，生成《评分标准拆解表》。

必须输出 Markdown，结构固定如下：

# 评分标准拆解表

## 1. 评分总览
用表格列出评分大类、分值、评审重点、对技术标的影响。

## 2. 技术商务评分项逐项拆解
每个评分项必须包含：
- 评分项
- 分值
- 招标文件依据
- 响应章节建议
- 必须证明材料
- 写作要点
- 风险提示

## 3. 技术标大纲映射
用表格给出"评分项 -> 建议一级章节 -> 建议二级章节 -> 必须覆盖内容"。

## 4. 废标与扣分风险清单
列出技术商务标中容易导致扣分、不得分、废标的事项。

要求：
- 不编造招标文件未出现的分值和证明材料。
- 若信息缺失，标注"招标文件未明确"。
- 技术标章节建议必须服务于后续四级大纲生成。
- 不输出代码块。`

type GenerateScoringBreakdownRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

type GenerateScoringBreakdownResponse struct {
	AnalysisReportPath string                 `json:"analysisReportPath"`
	ScoringReportPath  string                 `json:"scoringReportPath"`
	Content            string                 `json:"content"`
	Artifact           map[string]interface{} `json:"artifact"`
}

func GenerateScoringBreakdown(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateScoringBreakdownRequest,
) (*GenerateScoringBreakdownResponse, error) {
	analysisReportPath := strings.TrimSpace(req.AnalysisReportPath)
	scoringReportPath := strings.TrimSpace(req.ScoringReportPath)
	sourceFile := strings.TrimSpace(req.SourceFile)

	if analysisReportPath == "" {
		return nil, fmt.Errorf("analysisReportPath is required")
	}
	if scoringReportPath == "" {
		return nil, fmt.Errorf("scoringReportPath is required")
	}
	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	analysisBytes, err := os.ReadFile(analysisReportPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read analysis report: %w", err)
	}

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: scoringBreakdownSystemPrompt},
			{Role: "user", Content: buildScoringBreakdownPrompt(string(analysisBytes))},
		},
		Parameters: map[string]any{
			"temperature": 0.2,
			"max_tokens":  12000.0,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty scoring breakdown")
	}

	if err := os.MkdirAll(filepath.Dir(scoringReportPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create scoring report directory: %w", err)
	}
	if err := os.WriteFile(scoringReportPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write scoring report: %w", err)
	}

	return &GenerateScoringBreakdownResponse{
		AnalysisReportPath: analysisReportPath,
		ScoringReportPath:  scoringReportPath,
		Content:            content,
		Artifact:           scoringBreakdownArtifact(scoringReportPath, sourceFile),
	}, nil
}

func buildScoringBreakdownPrompt(analysisText string) string {
	return fmt.Sprintf(`## 招标文件解析报告

%s

请基于上述解析报告生成评分标准拆解表。重点抽取"评分办法与关键得分点""技术要求与服务范围""投标文件组成、格式与递交要求""风险点和废标事项"。

输出必须服务于后续技术标四级大纲生成，尤其要给出：

| 评分项 | 分值 | 响应章节建议 | 必须证明材料 | 技术标大纲映射 | 风险提示 |
|---|---:|---|---|---|---|
`, strings.TrimSpace(analysisText))
}

func scoringBreakdownArtifact(scoringReportPath, sourceFile string) map[string]interface{} {
	return map[string]interface{}{
		"unitId":     "bid-scoring-breakdown",
		"kind":       "BID_SCORING_BREAKDOWN",
		"name":       filepath.Base(scoringReportPath),
		"mimeType":   "text/markdown",
		"storageRef": scoringReportPath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}
}
