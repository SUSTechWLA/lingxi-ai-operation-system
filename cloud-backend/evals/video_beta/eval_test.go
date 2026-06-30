package main

import (
	"strings"
	"testing"
)

func TestRunVideoBetaEvalProducesPassFailReport(t *testing.T) {
	report := Run(DefaultCases())
	if len(report.Cases) < 10 {
		t.Fatalf("expected at least 10 eval cases, got %d", len(report.Cases))
	}
	if report.Total != len(report.Cases) {
		t.Fatalf("total = %d, cases = %d", report.Total, len(report.Cases))
	}
	if report.Passed == 0 {
		t.Fatalf("expected at least one passing eval case")
	}
	if report.Passed != report.Total {
		t.Fatalf("video beta eval must pass every case before closed beta:\n%s", report.String())
	}
	rendered := report.String()
	for _, metric := range []string{
		"intent_correct",
		"plan_guard_passed",
		"plan_judge_passed",
		"required_artifacts_complete",
		"forbidden_tool_absent",
		"publish_copy_complete",
	} {
		if !strings.Contains(rendered, metric) {
			t.Fatalf("report missing metric %q:\n%s", metric, rendered)
		}
	}
}
