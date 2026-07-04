package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
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
			scoringText = "\n\n## 评分标准拆解表\n\n" + string(scoringBytes)
		}
	}

	userPrompt := fmt.Sprintf("## 招标文件解析报告\n\n%s\n\n## 项目背景信息确认表\n\n%s%s",
		string(analysisBytes), string(contextBytes), scoringText)

	result, err := gw.Execute(ctx, &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: outlineSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Parameters: map[string]any{
			"temperature": 0.3,
			"max_tokens":  50000.0,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

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
