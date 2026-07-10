package model

import "testing"

func TestNormalizeShotCandidateExecutionFailsClosedWithoutKnownProvenance(t *testing.T) {
	for _, sourceType := range []string{"", "mystery_renderer"} {
		candidate := NormalizeShotCandidateExecution(ShotCandidate{SourceType: sourceType})
		if candidate.ExecutionMode != ExecutionModeUnknown || candidate.ProductionEligible {
			t.Fatalf("source %q normalized to mode=%q eligible=%v, want unknown/ineligible", sourceType, candidate.ExecutionMode, candidate.ProductionEligible)
		}
	}
}

func TestNormalizeShotCandidateExecutionKeepsKnownLegacyRealSourceEligible(t *testing.T) {
	candidate := NormalizeShotCandidateExecution(ShotCandidate{SourceType: ArtifactSourceHyperFrames})
	if candidate.ExecutionMode != ExecutionModeReal || !candidate.ProductionEligible {
		t.Fatalf("known legacy real source should remain eligible: %+v", candidate)
	}
}

func TestIsStrictProductionModeNormalizesWhitespace(t *testing.T) {
	for _, value := range []string{"", "strict", " STRICT ", "\tstrict\n"} {
		if !IsStrictProductionMode(value) {
			t.Fatalf("IsStrictProductionMode(%q) = false", value)
		}
	}
	if IsStrictProductionMode(ProductionModeDraft) {
		t.Fatal("draft mode must not be strict")
	}
}
