package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
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
- 对缺失或不明确的信息标注“未在文件中明确”。
- 使用 Markdown 标题和列表，便于后续生成 Word 文档。`

type BidAnalysisReportTool struct{}

func NewBidAnalysisReportTool() *BidAnalysisReportTool {
	return &BidAnalysisReportTool{}
}

func (t *BidAnalysisReportTool) Name() string {
	return "bid_analysis_report"
}

func (t *BidAnalysisReportTool) Description() string {
	return "Generate a tender parsing report through the cloud LLM backend"
}

func (t *BidAnalysisReportTool) Type() tool.ToolType {
	return tool.ToolTypeCustom
}

func (t *BidAnalysisReportTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	rawTextPath := strings.TrimSpace(stringParam(params, "raw_text_path"))
	reportPath := strings.TrimSpace(stringParam(params, "report_path"))
	if rawTextPath == "" {
		return tool.FailureResult("raw_text_path is required")
	}
	if reportPath == "" {
		return tool.FailureResult("report_path is required")
	}

	rawBytes, err := os.ReadFile(rawTextPath)
	if err != nil {
		return tool.FailureResult("failed to read raw text file: " + err.Error())
	}
	rawText := string(rawBytes)

	if modelGateway == nil {
		return tool.FailureResult("model gateway is not available")
	}

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

	gwResult, err := modelGateway.Execute(ctx, gwReq)
	if err != nil {
		return tool.FailureResult("LLM call failed: " + err.Error())
	}

	content := strings.TrimSpace(gwResult.Content)
	if content == "" {
		return tool.FailureResult("LLM returned empty bid analysis content")
	}

	sourceFile := strings.TrimSpace(stringParam(params, "source_file"))
	report := buildBidAnalysisReport(content, rawText, sourceFile)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return tool.FailureResult("failed to create report directory: " + err.Error())
	}
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return tool.FailureResult("failed to write bid analysis report: " + err.Error())
	}

	rawTextArtifact := map[string]interface{}{
		"unitId":     "bid-raw-text",
		"kind":       "BID_RAW_TEXT",
		"name":       filepath.Base(rawTextPath),
		"mimeType":   "text/markdown",
		"storageRef": rawTextPath,
		"metadata": map[string]interface{}{
			"status":     "valid",
			"sourceFile": sourceFile,
		},
	}

	reportArtifact := map[string]interface{}{
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

	return tool.SuccessResult(map[string]interface{}{
		"content":     content,
		"report_path": reportPath,
		"artifacts":   []map[string]interface{}{rawTextArtifact, reportArtifact},
	})
}

func (t *BidAnalysisReportTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:           t.Name(),
		Description:    t.Description(),
		Type:           "builtin",
		Sandbox:        false,
		Capabilities:   []string{"bid_writing", "bid_analysis", "bid_parsing", "report_generation", "document_parsing"},
		Tags:           []string{"biaoshu", "tender", "cloud_llm"},
		CostLevel:      tool.CostMedium,
		LatencyLevel:   tool.LatencyMedium,
		RiskLevel:      tool.RiskLow,
		SideEffect:     false,
		Idempotent:     true,
		ExecutionPlane: tool.ExecutionPlaneCloud,
		ArtifactPolicy: tool.ArtifactPolicy{
			ProduceArtifact: true,
			ArtifactKinds:   []string{"BID_RAW_TEXT", "BID_ANALYSIS"},
			Storage:         "local",
		},
		Parameters: map[string]tool.ParamDef{
			"raw_text_path": {
				Type:        "string",
				Description: "招标文件原文解析 Markdown 文件路径",
				Required:    true,
			},
			"report_path": {
				Type:        "string",
				Description: "最终解析报告 Markdown 文件写入路径",
				Required:    true,
			},
			"source_file": {
				Type:        "string",
				Description: "原始招标文件路径",
				Required:    false,
			},
			"output_dir": {
				Type:        "string",
				Description: "项目输出目录标识",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"content":     {Type: "string", Description: "云端大模型生成的解析报告正文"},
			"report_path": {Type: "string", Description: "最终解析报告 Markdown 文件路径"},
			"artifacts":   {Type: "array", Description: "解析报告 artifact 元数据"},
		},
	}
}

func (t *BidAnalysisReportTool) ValidateParameters(params map[string]interface{}) bool {
	return strings.TrimSpace(stringParam(params, "raw_text_path")) != "" &&
		strings.TrimSpace(stringParam(params, "report_path")) != ""
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

func stringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}
