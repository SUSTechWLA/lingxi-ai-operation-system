package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

const bidAnalysisSystemPrompt = `你是专业的招标文件分析师，请根据用户提供的招标文件原文，生成结构化的招标文件解析报告。

报告必须覆盖：
1. 项目基本信息
2. 投标人资格条件
3. 技术要求与服务范围
4. 商务条款、工期、质量、安全要求
5. 评分办法与关键得分点
6. 投标文件组成、格式与递交要求
7. 风险点、澄清点与后续标书编写建议

要求：
- 只基于原文分析，不要编造不存在的信息。
- 对缺失或不明确的信息标注"未在文件中明确"。
- 使用 Markdown 标题和列表，便于后续生成 Word 文档。`

// GenerateBidAnalysisReportRequest is the input for the shared service.
type GenerateBidAnalysisReportRequest struct {
	RawTextPath string `json:"rawTextPath"`
	ReportPath  string `json:"reportPath"`
	SourceFile  string `json:"sourceFile,omitempty"`
}

// GenerateBidAnalysisReportResponse is the output of the shared service.
type GenerateBidAnalysisReportResponse struct {
	RawTextPath string                 `json:"rawTextPath"`
	ReportPath  string                 `json:"reportPath"`
	Content     string                 `json:"content"`
	Artifact    map[string]interface{} `json:"artifact"`
}

// GenerateBidAnalysisReport reads the raw text, calls the LLM, and writes the report.
// It uses a high max_tokens (32000) to ensure the full report is generated without truncation.
func GenerateBidAnalysisReport(
	ctx context.Context,
	gw *modelgateway.Gateway,
	req GenerateBidAnalysisReportRequest,
) (*GenerateBidAnalysisReportResponse, error) {
	rawTextPath := strings.TrimSpace(req.RawTextPath)
	reportPath := strings.TrimSpace(req.ReportPath)
	sourceFile := strings.TrimSpace(req.SourceFile)

	// Validate inputs
	if rawTextPath == "" {
		return nil, fmt.Errorf("rawTextPath is required")
	}
	if reportPath == "" {
		return nil, fmt.Errorf("reportPath is required")
	}

	// Read raw text
	rawBytes, err := os.ReadFile(rawTextPath)
	if err != nil {
		return nil, fmt.Errorf("rawTextPath does not exist: %w", err)
	}
	rawText := string(rawBytes)
	if strings.TrimSpace(rawText) == "" {
		return nil, fmt.Errorf("raw text file is empty")
	}

	if gw == nil {
		return nil, fmt.Errorf("model gateway is not available")
	}

	// Call LLM
	gwReq := &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: bidAnalysisSystemPrompt},
			{Role: "user", Content: rawText},
		},
		Parameters: map[string]interface{}{
			"temperature": 0.3,
			"max_tokens":  32000.0,
		},
	}

	gwResult, err := gw.Execute(ctx, gwReq)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := strings.TrimSpace(gwResult.Content)
	if content == "" {
		return nil, fmt.Errorf("LLM returned empty bid analysis content")
	}

	// Build and write report
	report := buildBidAnalysisReport(content, rawText, sourceFile)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create report directory: %w", err)
	}
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write report: %w", err)
	}

	artifact := map[string]interface{}{
		"unitId":     "bid-analysis",
		"kind":       "BID_ANALYSIS",
		"name":       filepath.Base(reportPath),
		"mimeType":   "text/markdown",
		"storageRef": reportPath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}

	return &GenerateBidAnalysisReportResponse{
		RawTextPath: rawTextPath,
		ReportPath:  reportPath,
		Content:     content,
		Artifact:    artifact,
	}, nil
}

func buildBidAnalysisReport(content, rawText, sourceFile string) string {
	var b strings.Builder
	b.WriteString("# 招标文件解析报告\n\n")
	if sourceFile != "" {
		b.WriteString(fmt.Sprintf("**源文件**: %s\n\n", sourceFile))
	}
	b.WriteString("---\n\n")
	b.WriteString(content)
	b.WriteString("\n\n---\n\n## 附录：招标文件原文\n\n")
	b.WriteString(rawText)
	return b.String()
}
