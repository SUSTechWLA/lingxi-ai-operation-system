package service

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestClosedBetaShotPolicyDefaults(t *testing.T) {
	policy := model.DefaultShotPolicy()

	if policy.MinDurationSec != 3 || policy.MaxDurationSec != 14 {
		t.Fatalf("duration bounds = %d..%d, want 3..14", policy.MinDurationSec, policy.MaxDurationSec)
	}
	if policy.PreferredMinDurationSec != 6 || policy.PreferredMaxDurationSec != 8 {
		t.Fatalf("preferred range = %d..%d, want 6..8", policy.PreferredMinDurationSec, policy.PreferredMaxDurationSec)
	}
	if !policy.SplitByScriptSemantics || !policy.SplitByVisualChange {
		t.Fatalf("semantic/visual split flags should be enabled: %+v", policy)
	}
}

func TestShotDurationCheckerAcceptsFractionalBoundary(t *testing.T) {
	for _, duration := range []float64{3, 14.99} {
		if issues := CheckShotDurationSec("shot-ok", duration); len(issues) != 0 {
			t.Fatalf("duration %.2f should pass, got %+v", duration, issues)
		}
	}
	if issues := CheckShotDurationSec("shot-boundary", 15.0); !hasIssueCode(issues, "shot_duration_out_of_range") {
		t.Fatalf("15.0s should fail strict duration validation, got %+v", issues)
	}
	if issues := CheckShotDurationSec("shot-long", 15.1); !hasIssueCode(issues, "shot_duration_out_of_range") {
		t.Fatalf("15.1s should fail duration validation, got %+v", issues)
	}
}

func TestBuildShotGenerationPlanBlocksFifteenSecondShot(t *testing.T) {
	plan := BuildShotGenerationPlan(
		model.ShotUnit{ID: "shot-15", DurationSec: 15},
		model.VisualPlan{},
		model.DefaultRenderPreference(),
		RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true},
	)
	if plan.Mode != model.GenerationModePlaceholderPreview || !strings.Contains(plan.Reason, "less than 15 seconds") {
		t.Fatalf("fifteen-second shot must not receive a generation plan: %+v", plan)
	}
}

func TestBuildTimeWindowPlanSplitsByScriptSemanticsAndMergesContinuousShortSpans(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead},
		ScriptSpans: []model.ScriptSpan{
			{ID: "intro", StartSec: 0, EndSec: 2, Text: "主角推开门。", Scene: "studio", Subject: "host", Action: "enter", Camera: "wide", ShotSize: "wide"},
			{ID: "intro-detail", StartSec: 2, EndSec: 5, Text: "他把任务卡放到桌上。", Scene: "studio", Subject: "host", Action: "enter", Camera: "wide", ShotSize: "wide"},
			{ID: "switch", StartSec: 5, EndSec: 11, Text: "镜头切到桌面上的 MCP 插槽。", Scene: "studio", Subject: "task cards", Action: "sort", Camera: "top-down", ShotSize: "close"},
			{ID: "new-scene", StartSec: 11, EndSec: 17, Text: "时间跳到夜晚，系统开始自动检查。", Scene: "night-office", Subject: "qa panel", Action: "scan", Camera: "push-in", ShotSize: "medium"},
		},
	})

	if len(plan.Windows) != 3 {
		t.Fatalf("window count = %d, want 3 semantic windows: %#v", len(plan.Windows), plan.Windows)
	}
	if plan.Windows[0].DurationSec != 5 || plan.Windows[0].ScriptSpanID != "intro+intro-detail" {
		t.Fatalf("first short spans should merge into 5s visual unit: %#v", plan.Windows[0])
	}
	if plan.Windows[1].DurationSec != 6 || plan.Windows[1].VisualChangeReason == "" {
		t.Fatalf("subject/camera change should start a new shot with reason: %#v", plan.Windows[1])
	}
	if plan.Windows[2].SceneSummary != "night-office" || plan.Windows[2].VisualChangeReason == "" {
		t.Fatalf("scene/time jump should be represented: %#v", plan.Windows[2])
	}
	for _, window := range plan.Windows {
		if issues := CheckShotDurationSec(window.ShotID, window.DurationSec); len(issues) != 0 {
			t.Fatalf("window %s duration should be valid: %+v", window.ID, issues)
		}
	}
	if len(plan.SplitReport.PolicyReasons) == 0 {
		t.Fatalf("split report should explain semantic policy: %+v", plan.SplitReport)
	}
}

func TestBuildTimeWindowPlanSplitsLongScriptSpanIntoValidSemanticWindows(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead},
		ScriptSpans: []model.ScriptSpan{
			{
				ID:       "long-story",
				StartSec: 0,
				EndSec:   31,
				Text:     "主角先打开导演台，随后任务卡排队，最后 QA 面板亮起并给出通过提示。",
				Scene:    "studio",
				Subject:  "host",
				Action:   "operate director console",
				Camera:   "tracking",
				ShotSize: "medium",
				Framing:  "center",
				Visual:   "同一导演台场景内的连续操作，但时长超过 AIGC 单 shot 上限。",
				Emotion:  "from chaos to relief",
			},
		},
	})

	if len(plan.Windows) < 3 {
		t.Fatalf("31s script span should split into multiple windows: %#v", plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.DurationSec < 3 || window.DurationSec > 15 {
			t.Fatalf("split window duration outside 3-15s: %#v", window)
		}
		if window.ScriptText == "" || window.VisualChangeReason == "" {
			t.Fatalf("split windows should keep script and split reason: %#v", window)
		}
	}
}

func TestShotRepairLoopPreservesPassedDimensionsAndDoesNotOverwriteCandidates(t *testing.T) {
	policy := model.DefaultShotRepairPolicy()
	shot := model.ShotUnit{ID: "SHOT_01", DurationSec: 6}
	firstCandidate := model.ShotCandidate{
		CandidateID:  "SHOT_01-cand-01",
		ShotID:       "SHOT_01",
		AttemptIndex: 0,
		DurationSec:  6,
		SourceType:   model.ArtifactSourceAIGCVideo,
	}

	shot = RecordShotCandidateQA(shot, firstCandidate, model.ShotQAReport{
		ShotID:           "SHOT_01",
		CandidateID:      "SHOT_01-cand-01",
		Status:           model.ShotQAFailed,
		Passed:           false,
		PassedDimensions: []string{"prompt_alignment", "character_identity", "scene", "action", "duration", "style"},
		FailedDimensions: []string{"text_intent"},
		Summary:          "底部文字安全区拥挤。",
	}, policy)

	if len(shot.Candidates) != 1 {
		t.Fatalf("candidate should be recorded once: %+v", shot.Candidates)
	}
	if len(shot.RepairPlans) != 1 {
		t.Fatalf("failed QA should create one repair plan: %+v", shot.RepairPlans)
	}
	repair := shot.RepairPlans[0]
	if repair.Action != model.RepairActionRerenderHTML {
		t.Fatalf("text failure should prefer HTML rerender, got %+v", repair)
	}
	for _, dim := range []string{"prompt_alignment", "character_identity", "scene", "action", "duration", "style"} {
		if !containsString(repair.LockedDimensions, dim) {
			t.Fatalf("repair plan should lock passed dimension %q: %+v", dim, repair.LockedDimensions)
		}
	}

	secondCandidate := model.ShotCandidate{
		CandidateID:  "SHOT_01-cand-02",
		ShotID:       "SHOT_01",
		AttemptIndex: 1,
		DurationSec:  6,
		SourceType:   model.ArtifactSourceFFmpegComposite,
	}
	shot = RecordShotCandidateQA(shot, secondCandidate, model.ShotQAReport{
		ShotID:      "SHOT_01",
		CandidateID: "SHOT_01-cand-02",
		Status:      model.ShotQAPassed,
		Passed:      true,
		Summary:     "repair passed",
	}, policy)

	if len(shot.Candidates) != 2 {
		t.Fatalf("repair must append a new candidate, not overwrite: %+v", shot.Candidates)
	}
	if shot.AcceptedCandidateID != "SHOT_01-cand-02" {
		t.Fatalf("accepted candidate = %q", shot.AcceptedCandidateID)
	}
	if shot.QAStatus != model.ShotAcceptedForAssembly {
		t.Fatalf("shot QA status = %q, want accepted", shot.QAStatus)
	}
}

func TestShotRepairLoopStopsAtMaxAttemptsWithHumanReview(t *testing.T) {
	policy := model.DefaultShotRepairPolicy()
	shot := model.ShotUnit{ID: "SHOT_02", DurationSec: 6}

	for attempt := 0; attempt <= policy.MaxRepairAttemptsPerShot; attempt++ {
		candidate := model.ShotCandidate{
			CandidateID:  "SHOT_02-cand-" + string(rune('0'+attempt)),
			ShotID:       "SHOT_02",
			AttemptIndex: attempt,
			DurationSec:  6,
			SourceType:   model.ArtifactSourceAIGCVideo,
		}
		shot = RecordShotCandidateQA(shot, candidate, model.ShotQAReport{
			ShotID:           "SHOT_02",
			CandidateID:      candidate.CandidateID,
			Status:           model.ShotQAFailed,
			Passed:           false,
			FailedDimensions: []string{"character_identity"},
			Severity:         "severe",
		}, policy)
	}

	if shot.QAStatus != model.ShotHumanReviewRequired {
		t.Fatalf("QA status = %q, want HUMAN_REVIEW_REQUIRED", shot.QAStatus)
	}
	if len(shot.RepairPlans) != policy.MaxRepairAttemptsPerShot {
		t.Fatalf("repair loop should stop at %d plans, got %d", policy.MaxRepairAttemptsPerShot, len(shot.RepairPlans))
	}
}

func TestStrictShotAcceptanceRejectsFallbackCandidate(t *testing.T) {
	policy := model.DefaultShotRepairPolicy()
	shot := model.ShotUnit{ID: "SHOT_STRICT", DurationSec: 6}
	candidate := model.ShotCandidate{
		CandidateID:        "fallback-candidate",
		ShotID:             shot.ID,
		DurationSec:        6,
		SourceType:         model.ArtifactSourceFallbackPreview,
		IsFallback:         true,
		ExecutionMode:      model.ExecutionModeFallback,
		ProductionEligible: false,
		FallbackReason:     "provider unavailable",
		ArtifactRefs:       model.ShotArtifactRefs{VideoClipArtifactID: "fallback-preview"},
	}

	shot = RecordShotCandidateQA(shot, candidate, model.ShotQAReport{Passed: true, Status: model.ShotQAPassed}, policy)

	if shot.AcceptedCandidateID != "" || shot.QAStatus != model.ShotHumanReviewRequired {
		t.Fatalf("strict mode must not accept fallback candidate: %+v", shot)
	}
	if len(shot.RepairPlans) != 1 || shot.RepairPlans[0].Action != model.RepairActionHumanReview {
		t.Fatalf("fallback candidate should route to human review: %+v", shot.RepairPlans)
	}
}

func TestFinalAssemblyStrictRejectsFallbackButDraftCanInspectIt(t *testing.T) {
	shot := model.ShotUnit{
		ID:                  "SHOT_FALLBACK",
		DurationSec:         6,
		ReviewStatus:        model.ReviewStatusApproved,
		AcceptedCandidateID: "fallback-candidate",
		Candidates: []model.ShotCandidate{{
			CandidateID:        "fallback-candidate",
			ShotID:             "SHOT_FALLBACK",
			Status:             model.CandidateAcceptedForAssembly,
			DurationSec:        6,
			SourceType:         model.ArtifactSourceFallbackPreview,
			IsFallback:         true,
			ExecutionMode:      model.ExecutionModeFallback,
			ProductionEligible: false,
			ArtifactRefs:       model.ShotArtifactRefs{VideoClipArtifactID: "fallback-preview"},
			QAReport:           &model.ShotQAReport{Passed: true, Status: model.ShotQAPassed},
		}},
	}

	if _, issues := BuildFinalAssemblyPlanWithPolicy([]model.ShotUnit{shot}, model.AssemblyPolicy{ProductionMode: model.ProductionModeStrict}); !hasIssueCode(issues, "final_assembly_rejects_production_ineligible_candidate") {
		t.Fatalf("strict assembly must reject fallback candidate, got %+v", issues)
	}
	plan, issues := BuildFinalAssemblyPlanWithPolicy([]model.ShotUnit{shot}, model.AssemblyPolicy{ProductionMode: model.ProductionModeDraft})
	if len(issues) != 0 || len(plan.AcceptedShots) != 1 {
		t.Fatalf("draft assembly should keep fallback inspectable: plan=%+v issues=%+v", plan, issues)
	}
}

func TestFinalAssemblyUsesOnlyAcceptedCandidatesAndGlobalTimeline(t *testing.T) {
	accepted := model.ShotUnit{
		ID:                  "SHOT_OK",
		DurationSec:         6,
		QAStatus:            model.ShotAcceptedForAssembly,
		AcceptedCandidateID: "ok-candidate",
		Candidates: []model.ShotCandidate{{
			CandidateID:  "ok-candidate",
			ShotID:       "SHOT_OK",
			AttemptIndex: 1,
			Status:       model.CandidateAcceptedForAssembly,
			DurationSec:  6,
			SourceType:   model.ArtifactSourceFFmpegComposite,
			ArtifactRefs: model.ShotArtifactRefs{CompositedShotVideoArtifactID: "clip-ok"},
			QAReport:     &model.ShotQAReport{Passed: true, Status: model.ShotQAPassed},
		}},
	}
	failed := model.ShotUnit{
		ID:                  "SHOT_FAIL",
		DurationSec:         6,
		QAStatus:            model.ShotQAFailed,
		AcceptedCandidateID: "failed-candidate",
		Candidates: []model.ShotCandidate{{
			CandidateID:  "failed-candidate",
			ShotID:       "SHOT_FAIL",
			AttemptIndex: 0,
			Status:       model.CandidateShotQAFailed,
			DurationSec:  6,
			SourceType:   model.ArtifactSourceAIGCVideo,
			ArtifactRefs: model.ShotArtifactRefs{VideoClipArtifactID: "clip-failed"},
			QAReport:     &model.ShotQAReport{Passed: false, Status: model.ShotQAFailed},
		}},
	}

	if _, issues := BuildFinalAssemblyPlan([]model.ShotUnit{accepted, failed}); !hasIssueCode(issues, "final_assembly_rejects_failed_candidate") {
		t.Fatalf("failed candidate must not enter assembly, got %+v", issues)
	}

	plan, issues := BuildFinalAssemblyPlan([]model.ShotUnit{accepted})
	if len(issues) != 0 {
		t.Fatalf("accepted shot should build assembly plan: %+v", issues)
	}
	if len(plan.AcceptedShots) != 1 || plan.AcceptedShots[0].CandidateID != "ok-candidate" {
		t.Fatalf("accepted shots = %+v", plan.AcceptedShots)
	}
	if !containsString(plan.Steps, model.AssemblyStepFFmpegConcat) ||
		!containsString(plan.Steps, model.AssemblyStepGlobalAudioMix) ||
		!containsString(plan.Steps, model.AssemblyStepGlobalSubtitleRender) {
		t.Fatalf("assembly steps should include concat, global audio, global subtitle: %+v", plan.Steps)
	}
	if len(plan.SubtitleTimeline.Cues) != 1 || plan.SubtitleTimeline.Cues[0].StartSec != 0 || plan.SubtitleTimeline.Cues[0].EndSec != 6 {
		t.Fatalf("subtitle timeline should use global timing: %+v", plan.SubtitleTimeline)
	}
	if plan.AudioMixPlan.Scope != model.AssemblyScopeGlobal {
		t.Fatalf("audio mix should be global: %+v", plan.AudioMixPlan)
	}
	if issues := CheckFinalExportGate(model.FinalQAReport{Passed: false, Status: "failed"}); !hasIssueCode(issues, "final_qa_failed_blocks_export") {
		t.Fatalf("final QA failure must block export/publish, got %+v", issues)
	}
}

func TestDiagnosticsSnapshotIncludesClosedBetaChainAndFallbackProvenance(t *testing.T) {
	shot := model.ShotUnit{
		ID:                  "SHOT_01",
		DurationSec:         6,
		QAStatus:            model.ShotAcceptedForAssembly,
		AcceptedCandidateID: "fallback-candidate",
		Candidates: []model.ShotCandidate{{
			CandidateID:  "fallback-candidate",
			ShotID:       "SHOT_01",
			AttemptIndex: 0,
			Status:       model.CandidateAcceptedForAssembly,
			DurationSec:  6,
			SourceType:   model.ArtifactSourceFallbackStoryboard,
			IsFallback:   true,
			ArtifactRefs: model.ShotArtifactRefs{VideoClipArtifactID: "fallback-preview"},
			QAReport:     &model.ShotQAReport{Passed: true, Status: model.ShotQAPassed},
		}},
	}
	assembly, issues := BuildFinalAssemblyPlanWithPolicy([]model.ShotUnit{shot}, model.AssemblyPolicy{ProductionMode: model.ProductionModeDraft})
	if len(issues) != 0 {
		t.Fatalf("assembly issues = %+v", issues)
	}
	diagnostics := BuildVideoDiagnosticsSnapshot(model.ShotDrivenState{Shots: []model.ShotUnit{shot}}, model.TimeWindowPlan{}, assembly, model.FinalQAReport{Passed: true, Status: "passed"})

	if len(diagnostics.ShotList) != 1 ||
		len(diagnostics.ShotCandidates) != 1 ||
		len(diagnostics.ShotQAReports) != 1 ||
		len(diagnostics.AcceptedShots) != 1 {
		t.Fatalf("diagnostics chain incomplete: %+v", diagnostics)
	}
	if diagnostics.ProvenanceSummary.FallbackCount != 1 || diagnostics.ProvenanceSummary.RealAIGCVideoCount != 0 {
		t.Fatalf("fallback must not be counted as real AIGC: %+v", diagnostics.ProvenanceSummary)
	}
	if diagnostics.AssemblyPlan.AudioMixPlan.Scope != model.AssemblyScopeGlobal || len(diagnostics.SubtitleTimeline.Cues) == 0 {
		t.Fatalf("diagnostics should expose global audio/subtitle plans: %+v", diagnostics)
	}
}
