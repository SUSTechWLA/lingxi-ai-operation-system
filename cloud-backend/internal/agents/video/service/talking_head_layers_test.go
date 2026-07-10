package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestSubtitleStyleChangeInvalidatesOnlyTextDownstream(t *testing.T) {
	shot := acceptedLayeredShot()
	result := InvalidateTalkingHeadShot(&shot, LayerChange{Kind: LayerChangeTextStyle, Reason: "subtitle theme changed"})

	assertLayerStatus(t, shot.TalkingHeadLayers.Audio.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.IP.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.Broll.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.Text.State, model.LayerStatusStale)
	assertLayerStatus(t, shot.TalkingHeadLayers.Composition.State, model.LayerStatusStale)
	if shot.AcceptedCandidateID != "" || !shot.Stale || shot.ReviewStatus != model.ReviewStatusStale {
		t.Fatalf("accepted state must be invalidated: %+v", shot)
	}
	if !containsLayer(result.InvalidatedLayers, model.ShotLayerText) || containsLayer(result.InvalidatedLayers, model.ShotLayerAudio) {
		t.Fatalf("invalidated layers = %+v", result.InvalidatedLayers)
	}
}

func TestBrollReplacementPreservesAudioIPAndText(t *testing.T) {
	shot := acceptedLayeredShot()
	InvalidateTalkingHeadShot(&shot, LayerChange{Kind: LayerChangeBrollReplacement, Reason: "replace source asset"})
	assertLayerStatus(t, shot.TalkingHeadLayers.Audio.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.IP.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.Text.State, model.LayerStatusCurrent)
	assertLayerStatus(t, shot.TalkingHeadLayers.Broll.State, model.LayerStatusStale)
	assertLayerStatus(t, shot.TalkingHeadLayers.Composition.State, model.LayerStatusStale)
}

func TestScriptChangeInvalidatesAudioMasterAndAllDependentLayers(t *testing.T) {
	shot := acceptedLayeredShot()
	result := InvalidateTalkingHeadShot(&shot, LayerChange{Kind: LayerChangeScript, Reason: "narration revised"})
	for _, state := range []model.LayerArtifactState{
		shot.TalkingHeadLayers.Audio.State,
		shot.TalkingHeadLayers.IP.State,
		shot.TalkingHeadLayers.Text.State,
		shot.TalkingHeadLayers.Broll.State,
		shot.TalkingHeadLayers.Composition.State,
	} {
		assertLayerStatus(t, state, model.LayerStatusStale)
	}
	if len(result.InvalidatedLayers) < 8 {
		t.Fatalf("script invalidation should include candidate, QA, and final assembly: %+v", result)
	}
}

func TestAcceptedGateRejectsTimelineRevisionMismatch(t *testing.T) {
	shot := acceptedLayeredShot()
	shot.AcceptedCandidateID = ""
	shot.Candidates = nil
	candidate := model.ShotCandidate{
		CandidateID: "c-new", ShotID: shot.ID, TimelineRevision: "audio-old",
		ExecutionMode: model.ExecutionModeReal, ProductionEligible: true,
	}
	report := model.ShotQAReport{Passed: true, Status: model.ShotQAPassed}
	shot = RecordShotCandidateQA(shot, candidate, report, model.DefaultShotRepairPolicy())
	if shot.AcceptedCandidateID != "" || shot.QAStatus != model.ShotHumanReviewRequired {
		t.Fatalf("timeline mismatch must not be accepted: %+v", shot)
	}
}

func TestFinalAssemblyRejectsStaleAcceptedDependency(t *testing.T) {
	shot := acceptedLayeredShot()
	shot.Candidates[0].DurationSec = 6
	shot.Candidates[0].ArtifactRefs.VideoClipArtifactID = "shot-02.mp4"
	shot.Candidates[0].QAReport = &model.ShotQAReport{Passed: true, Status: model.ShotQAPassed}
	shot.Candidates[0].Stale = true
	shot.Candidates[0].StaleReason = "text layer revision changed"
	_, issues := BuildFinalAssemblyPlan([]model.ShotUnit{shot})
	if !hasIssueCode(issues, "final_assembly_rejects_stale_candidate") {
		t.Fatalf("stale accepted dependency should block assembly: %+v", issues)
	}
}

func TestTalkingHeadVisualModePolicy(t *testing.T) {
	policy := DefaultTalkingHeadVisualModePolicy()
	tests := []struct {
		name string
		req  VisualModeDecisionRequest
		want model.VisualMode
	}{
		{name: "hook returns to IP", req: VisualModeDecisionRequest{Position: NarrativePositionHook, HasExactText: true}, want: model.VisualModeIPWithText},
		{name: "operation uses screen", req: VisualModeDecisionRequest{HasScreenRecording: true, EvidenceStrength: 0.9}, want: model.VisualModeScreenRecordingFull},
		{name: "case evidence uses broll", req: VisualModeDecisionRequest{HasBroll: true, HasCaseOrEvidence: true, EvidenceStrength: 0.9}, want: model.VisualModeBrollFullscreen},
		{name: "ordinary narration stays IP", req: VisualModeDecisionRequest{}, want: model.VisualModeIPPrimary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideTalkingHeadVisualMode(policy, tt.req)
			if decision.Mode != tt.want || decision.Reason == "" || decision.Confidence == 0 {
				t.Fatalf("decision = %+v, want %s", decision, tt.want)
			}
		})
	}
}

func TestBrollManifestUnknownLicenseBlocksPublishOnly(t *testing.T) {
	manifest := model.BrollManifest{SchemaVersion: model.TalkingHeadSchemaVersion, Revision: "broll-r1", Entries: []model.BrollManifestEntry{{
		ID: "b1", ShotID: "SHOT_03", SemanticPurpose: "show product operation", AssetType: "video", SourceType: "external",
		SourceURI: "https://example.test/demo.mp4", LicenseStatus: "unknown", StartMs: 12000, EndMs: 18000, ReviewStatus: "pending",
	}}}
	if issues := ValidateBrollManifest(manifest, false); hasIssueCode(issues, "broll_license_blocks_publish") {
		t.Fatalf("draft should retain unknown-license b-roll for review: %+v", issues)
	}
	if issues := ValidateBrollManifest(manifest, true); !hasIssueCode(issues, "broll_license_blocks_publish") {
		t.Fatalf("publish must block unknown license: %+v", issues)
	}
}

func acceptedLayeredShot() model.ShotUnit {
	current := func(layer model.ShotLayerKind) model.LayerArtifactState {
		return model.LayerArtifactState{SchemaVersion: model.TalkingHeadSchemaVersion, Layer: layer, Status: model.LayerStatusCurrent, Revision: string(layer) + "-r1", ProductionEligible: true}
	}
	return model.ShotUnit{
		ID: "SHOT_02", TimelineRevision: "audio-r1", AcceptedCandidateID: "candidate-r1", QAStatus: model.ShotAcceptedForAssembly,
		ReviewStatus: model.ReviewStatusApproved,
		Candidates:   []model.ShotCandidate{{CandidateID: "candidate-r1", Status: model.CandidateAcceptedForAssembly, TimelineRevision: "audio-r1", ExecutionMode: model.ExecutionModeReal, ProductionEligible: true}},
		TalkingHeadLayers: &model.TalkingHeadShotLayers{
			SchemaVersion: model.TalkingHeadSchemaVersion, TimelineRevision: "audio-r1",
			Audio:       model.AudioLayerPlan{State: current(model.ShotLayerAudio), AudioMasterRevision: "audio-r1"},
			IP:          model.IPLayerPlan{State: current(model.ShotLayerIP)},
			Text:        model.TextGraphicsLayerPlan{State: current(model.ShotLayerText), TimelineRevision: "audio-r1", Renderer: "hyperframes"},
			Broll:       model.BrollLayerPlan{State: current(model.ShotLayerBroll)},
			Composition: model.CompositionLayerPlan{State: current(model.ShotLayerComposition)},
		},
	}
}

func assertLayerStatus(t *testing.T, state model.LayerArtifactState, want string) {
	t.Helper()
	if state.Status != want {
		t.Fatalf("layer %s status = %s, want %s", state.Layer, state.Status, want)
	}
}

func containsLayer(values []model.ShotLayerKind, want model.ShotLayerKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
