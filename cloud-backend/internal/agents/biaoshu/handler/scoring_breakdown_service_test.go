package handler

import (
	"strings"
	"testing"
)

func TestBuildScoringBreakdownPromptIncludesRequiredSections(t *testing.T) {
	analysis := "# 招标文件解析报告\n\n### **5. 评分办法与关键得分点**\n\n技术商务分 30 分。\n- 综合实力 7 分\n- 业绩 2 分\n- 种植养护方案 8 分"

	prompt := buildScoringBreakdownPrompt(analysis)

	required := []string{
		"评分项",
		"分值",
		"响应章节建议",
		"必须证明材料",
		"技术标大纲映射",
		"综合实力 7 分",
	}
	for _, item := range required {
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
