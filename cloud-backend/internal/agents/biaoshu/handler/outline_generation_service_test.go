package handler

import (
	"strings"
	"testing"
)

func TestStripBidAnalysisAppendixRemovesRawTenderText(t *testing.T) {
	input := "# 招标文件解析报告\n\n## 项目基本信息\n\n保留结构化分析。\n\n---\n\n## 附录：招标文件原文\n\n这里是很长的招标文件原文。"

	got := stripBidAnalysisAppendix(input)

	if strings.Contains(got, "这里是很长的招标文件原文") {
		t.Fatalf("expected raw tender appendix to be removed, got %q", got)
	}
	if !strings.Contains(got, "保留结构化分析") {
		t.Fatalf("expected structured analysis to be kept, got %q", got)
	}
}

func TestStripBidAnalysisAppendixKeepsReportWithoutAppendix(t *testing.T) {
	input := "# 招标文件解析报告\n\n## 评分办法\n\n技术部分 80 分。"

	got := stripBidAnalysisAppendix(input)

	if got != input {
		t.Fatalf("expected report without appendix to stay unchanged, got %q", got)
	}
}

func TestLimitTextForPromptKeepsHeadingAndAddsTruncationNotice(t *testing.T) {
	input := "标题\n" + strings.Repeat("内容", 200)

	got := limitTextForPrompt(input, 20, "解析报告")

	if len([]rune(got)) > 120 {
		t.Fatalf("expected compact text, got rune length %d: %q", len([]rune(got)), got)
	}
	if !strings.Contains(got, "标题") {
		t.Fatalf("expected beginning of text to be preserved, got %q", got)
	}
	if !strings.Contains(got, "解析报告已截断") {
		t.Fatalf("expected truncation notice, got %q", got)
	}
}

func TestBuildOutlineUserPromptStripsAndLimitsInputs(t *testing.T) {
	analysis := "# 招标文件解析报告\n\n## 评分办法\n\n技术方案 80 分。\n\n## 附录：招标文件原文\n\n" + strings.Repeat("原文", 100)
	context := "# 项目背景信息确认表\n\n项目位于杭州。"
	scoring := "# 评分标准拆解表\n\n施工组织设计 40 分。"

	prompt, meta := buildOutlineUserPrompt(analysis, context, scoring)

	if strings.Contains(prompt, "原文原文原文") {
		t.Fatalf("expected raw tender appendix to be stripped, got %q", prompt)
	}
	if !strings.Contains(prompt, "技术方案 80 分") {
		t.Fatalf("expected analysis scoring content to be kept, got %q", prompt)
	}
	if !strings.Contains(prompt, "项目位于杭州") {
		t.Fatalf("expected context report to be included, got %q", prompt)
	}
	if !strings.Contains(prompt, "施工组织设计 40 分") {
		t.Fatalf("expected scoring report to be included, got %q", prompt)
	}
	if meta.AnalysisOriginalRunes <= meta.AnalysisPromptRunes {
		t.Fatalf("expected analysis prompt to be smaller after appendix strip, meta=%+v", meta)
	}
}

func TestOutlineMaxTokensIsBounded(t *testing.T) {
	if outlineMaxTokens > 16000 {
		t.Fatalf("outlineMaxTokens should stay bounded for synchronous requests, got %d", outlineMaxTokens)
	}
	if outlineMaxTokens < 8000 {
		t.Fatalf("outlineMaxTokens should be large enough for a four-level outline, got %d", outlineMaxTokens)
	}
}
