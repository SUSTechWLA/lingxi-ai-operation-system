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

## 一、评分项分类

先把评分项分成两类：

### 硬性条件项（X分）-- 证明文件到位即可，正文不展开

用于整理人员证书、设备发票、车辆证件、业绩合同、资质证书、社保证明等客观证明材料类得分项。表格列固定为：

| 序号 | 评分项 | 分值 | 证明材料 | 正文处理 |
|------|--------|------|----------|----------|

正文处理必须说明该项是否仅在证明材料、人员配置表、设备投入清单、业绩表或附件中体现；不得把客观证明材料项扩写成大篇幅正文。

### 正文写作重点项（X分）-- 需详实展开，争取评委会主观打分

用于整理施工组织、技术方案、服务方案、质量管理、安全生产、应急预案、进度保障、设计方案、养护方案等需要正文展开的主观评分项。表格列固定为：

| 序号 | 评分项 | 分值 | 分值占比 | 写作优先级 |
|------|--------|------|----------|------------|

分值占比 = 该评分项分值 ÷ 正文写作重点项总分。写作优先级按分值和评审影响分为"★★★ 最高""★★☆ 中高""★☆☆ 中低"。

## 二、字数目标计算

必须给出公式：

**目标字数 = 评分分值 × (总页数 ÷ 总分) × 780**

计算规则：
- 总分优先使用招标文件评分办法中的技术商务或技术标总分。
- 总页数优先使用招标文件或用户已确认信息中的页数要求。
- 如果总页数未明确，使用"建议按300页测算，待用户确认"。

用表格列出每个正文写作重点项的字数目标：

| 评分项 | 分值 | 目标字数 | 合格范围 |
|--------|------|----------|----------|

## 三、各评分项写作策略

对每一个"正文写作重点项"分别设置小节，格式为：

### 3.N 评分项名称（X分）-- 重要性判断

优先使用表格拆解。常规表格列为：

| 分解 | 分值 | 对应招标要求 | 写作策略 |
|------|------|-------------|----------|

如果招标文件没有给出子项分值，则列为"招标文件未明确"，但仍要按得分点、服务场景、技术措施或管理流程拆解。

对紧急预案、风险处置、服务保障等场景类评分项，可使用：

| 核心场景 | 写作策略 |
|----------|----------|

写作策略必须具体到可落入后续大纲的内容，例如流程、表格、职责、响应时限、设备人员投入、质量控制点、风险闭环、检查频次、验收标准。不得只写"详细阐述""加强管理"等空泛表达。

## 四、技术标章节目录（预规划）

按评分项组织章节，确保每个正文写作重点项独立成章或独立成节。必须输出章节清单，并在章节名后标注对应评分项和分值，例如：

第一章  项目概况与总体方案
第二章  XXX方案（X分）
第三章  XXX管理体系（X分）

章节目录必须服务于后续《技术标四级大纲》生成。

## 五、待用户确认

固定输出以下确认问题：

1. 以上评分项拆解和写作策略是否准确？
2. 章节目录结构是否合理？是否需要调整？
3. 字数目标分配是否合适？

最后固定输出：

> 确认后回复"**确认**"进入下一阶段（生成4级标题详细大纲）。

要求：
- 不编造招标文件未出现的分值、评分项、证明材料和硬性时限。
- 若信息缺失，标注"招标文件未明确"或"待用户确认"。
- 技术标章节建议必须服务于后续四级大纲生成。
- 不输出代码块。
- 不硬套样例中的项目事实；样例只代表结构，不代表当前项目内容。`

var scoringBreakdownRequiredSections = []string{
	"## 一、评分项分类",
	"### 硬性条件项",
	"### 正文写作重点项",
	"## 二、字数目标计算",
	"目标字数 = 评分分值",
	"## 三、各评分项写作策略",
	"## 四、技术标章节目录（预规划）",
	"## 五、待用户确认",
}

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
	if err := validateScoringBreakdownContent(content); err != nil {
		return nil, err
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

请基于上述解析报告生成评分标准拆解表。重点抽取评分办法与关键得分点、技术要求与服务范围、投标文件组成与格式要求，以及风险点、扣分点和废标事项。

拆解原则：
- 人员证书、车辆设备、业绩合同、资质证明、社保证明等客观材料项归入"硬性条件项"，只说明证明材料和正文处理方式。
- 施工组织、技术方案、服务方案、质量管理、安全生产、应急预案、进度保障、设计方案、养护方案等主观评分项归入"正文写作重点项"，必须展开写作策略。
- 正文写作重点项必须计算分值占比、写作优先级、目标字数和合格范围。
- 字数测算公式必须使用"目标字数 = 评分分值 × (总页数 ÷ 总分) × 780"。
- 如果总页数未明确，写明"建议按300页测算，待用户确认"，不要把300页描述为招标文件事实。
- 后续技术标章节目录必须与正文写作重点项一一对应，方便生成四级大纲。
`, strings.TrimSpace(analysisText))
}

func validateScoringBreakdownContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("LLM returned empty scoring breakdown")
	}
	for _, section := range scoringBreakdownRequiredSections {
		if !strings.Contains(trimmed, section) {
			return fmt.Errorf("scoring breakdown missing required section %q", section)
		}
	}
	if strings.Contains(trimmed, "```") {
		return fmt.Errorf("scoring breakdown must not contain code fences")
	}
	return nil
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
