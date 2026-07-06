package handler

import (
	"strings"
	"testing"
)

func TestScoringBreakdownSystemPromptIncludesRequiredSections(t *testing.T) {
	required := []string{
		"评分项分类",
		"硬性条件项",
		"正文写作重点项",
		"字数目标计算",
		"目标字数 = 评分分值",
		"各评分项写作策略",
		"技术标章节目录（预规划）",
		"待用户确认",
	}
	for _, item := range required {
		if !strings.Contains(scoringBreakdownSystemPrompt, item) {
			t.Fatalf("expected system prompt to contain %q", item)
		}
	}
}

func TestBuildScoringBreakdownPromptIncludesAnalysisFacts(t *testing.T) {
	analysis := "# 招标文件解析报告\n\n### **5. 评分办法与关键得分点**\n\n技术商务分 30 分。\n- 综合实力 7 分\n- 业绩 2 分\n- 种植养护方案 8 分"

	prompt := buildScoringBreakdownPrompt(analysis)

	for _, item := range []string{"综合实力 7 分", "评分办法与关键得分点", "硬性条件项", "正文写作重点项"} {
		if !strings.Contains(prompt, item) {
			t.Fatalf("expected prompt to contain %q, got %q", item, prompt)
		}
	}
}

func TestScoringBreakdownArtifactMetadata(t *testing.T) {
	artifact := scoringBreakdownArtifact("E:/out/02_评分标准拆解表.md", "source.pdf")

	if artifact["kind"] != "BID_SCORING_BREAKDOWN" {
		t.Fatalf("unexpected kind: %#v", artifact["kind"])
	}
	if artifact["unitId"] != "bid-scoring-breakdown" {
		t.Fatalf("unexpected unitId: %#v", artifact["unitId"])
	}
	if artifact["storageRef"] != "E:/out/02_评分标准拆解表.md" {
		t.Fatalf("unexpected storageRef: %#v", artifact["storageRef"])
	}
}

func TestValidateScoringBreakdownContentAcceptsSampleStyleSections(t *testing.T) {
	content := `# 评分标准拆解表

## 一、评分项分类

### 硬性条件项（9分）-- 证明文件到位即可，正文不展开

| 序号 | 评分项 | 分值 | 证明材料 | 正文处理 |
|------|--------|------|----------|----------|
| 1 | 项目负责人职称 | 3 | 身份证+职称证+社保证明 | 仅在人员配置表中体现 |

### 正文写作重点项（21分）-- 需详实展开，争取评委会主观打分

| 序号 | 评分项 | 分值 | 分值占比 | 写作优先级 |
|------|--------|------|----------|------------|
| 2 | 施工组织方案 | 8 | 38.1% | ★★★ 最高 |

## 二、字数目标计算

公式：**目标字数 = 评分分值 × (总页数 ÷ 总分) × 780**

## 三、各评分项写作策略

### 3.1 施工组织方案（8分）

| 分解 | 分值 | 对应招标要求 | 写作策略 |
|------|------|-------------|----------|
| 施工部署 | 4分 | 招标文件要求 | 分阶段展开 |

## 四、技术标章节目录（预规划）

第一章  项目概况与总体方案

## 五、待用户确认

1. 以上评分项拆解和写作策略是否准确？
`

	if err := validateScoringBreakdownContent(content); err != nil {
		t.Fatalf("expected valid scoring breakdown content, got %v", err)
	}
}

func TestValidateScoringBreakdownContentRejectsMissingSampleSections(t *testing.T) {
	content := `# 评分标准拆解表

## 1. 评分总览

| 评分项 | 分值 |
|--------|------|
| 施工组织方案 | 8 |
`

	if err := validateScoringBreakdownContent(content); err == nil {
		t.Fatal("expected missing-section validation error")
	}
}
