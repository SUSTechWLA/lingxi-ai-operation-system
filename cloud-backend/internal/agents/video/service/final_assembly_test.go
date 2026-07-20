package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildFinalAssemblyPlanRejectsFifteenSecondAcceptedCandidate(t *testing.T) {
	shot := model.ShotUnit{
		ID:                  "shot-1",
		DurationSec:         14,
		AcceptedCandidateID: "candidate-15",
		Candidates: []model.ShotCandidate{{
			CandidateID:        "candidate-15",
			ShotID:             "shot-1",
			Status:             model.CandidateAcceptedForAssembly,
			DurationSec:        15,
			SourceType:         model.ArtifactSourceAIGCVideo,
			ProductionEligible: true,
			ArtifactRefs:       model.ShotArtifactRefs{VideoClipArtifactID: "candidate-15.mp4"},
			QAReport:           &model.ShotQAReport{Passed: true, Status: model.ShotQAPassed},
		}},
	}

	plan, issues := BuildFinalAssemblyPlanWithPolicy([]model.ShotUnit{shot}, model.AssemblyPolicy{ProductionMode: model.ProductionModeStrict})
	if plan.Status != "blocked" || !hasIssueCode(issues, "shot_duration_out_of_range") {
		t.Fatalf("fifteen-second candidate must block assembly: plan=%+v issues=%+v", plan, issues)
	}
}
