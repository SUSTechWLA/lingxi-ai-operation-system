package service

import (
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type LayerChangeKind string

const (
	LayerChangeScript           LayerChangeKind = "script"
	LayerChangeAudioMaster      LayerChangeKind = "audio_master"
	LayerChangeTextStyle        LayerChangeKind = "text_style"
	LayerChangeBrollReplacement LayerChangeKind = "broll_replacement"
	LayerChangeIPMotion         LayerChangeKind = "ip_motion"
)

type LayerChange struct {
	Kind   LayerChangeKind `json:"kind"`
	Reason string          `json:"reason"`
}

type LayerInvalidationResult struct {
	ShotID              string                `json:"shotId"`
	InvalidatedLayers   []model.ShotLayerKind `json:"invalidatedLayers"`
	AcceptedInvalidated bool                  `json:"acceptedInvalidated"`
	StaleReason         string                `json:"staleReason"`
}

var talkingHeadInvalidationScopes = map[LayerChangeKind][]model.ShotLayerKind{
	LayerChangeScript: {
		model.ShotLayerAudio, model.ShotLayerIP, model.ShotLayerText, model.ShotLayerBroll,
		model.ShotLayerComposition, model.ShotLayerCandidate, model.ShotLayerQA, model.ShotLayerFinal,
	},
	LayerChangeAudioMaster: {
		model.ShotLayerIP, model.ShotLayerText, model.ShotLayerBroll, model.ShotLayerComposition,
		model.ShotLayerCandidate, model.ShotLayerQA, model.ShotLayerFinal,
	},
	LayerChangeTextStyle: {
		model.ShotLayerText, model.ShotLayerComposition, model.ShotLayerCandidate, model.ShotLayerQA, model.ShotLayerFinal,
	},
	LayerChangeBrollReplacement: {
		model.ShotLayerBroll, model.ShotLayerComposition, model.ShotLayerCandidate, model.ShotLayerQA, model.ShotLayerFinal,
	},
	LayerChangeIPMotion: {
		model.ShotLayerIP, model.ShotLayerComposition, model.ShotLayerCandidate, model.ShotLayerQA, model.ShotLayerFinal,
	},
}

// InvalidateTalkingHeadShot applies the authoritative dependency graph to the
// existing ShotUnit and its accepted candidate. Old artifacts remain referenced
// for comparison; only their eligibility changes.
func InvalidateTalkingHeadShot(shot *model.ShotUnit, change LayerChange) LayerInvalidationResult {
	result := LayerInvalidationResult{StaleReason: strings.TrimSpace(change.Reason)}
	if shot == nil {
		return result
	}
	result.ShotID = shot.ID
	if result.StaleReason == "" {
		result.StaleReason = "upstream " + string(change.Kind) + " changed"
	}
	scope := talkingHeadInvalidationScopes[change.Kind]
	result.InvalidatedLayers = append(result.InvalidatedLayers, scope...)
	for _, layer := range scope {
		markShotLayerStale(shot.TalkingHeadLayers, layer, result.StaleReason)
	}

	if containsShotLayer(scope, model.ShotLayerCandidate) {
		for index := range shot.Candidates {
			candidate := &shot.Candidates[index]
			if candidate.CandidateID == shot.AcceptedCandidateID || candidate.Status == model.CandidateAcceptedForAssembly {
				candidate.Stale = true
				candidate.StaleReason = result.StaleReason
				candidate.Status = model.ReviewStatusStale
				if candidate.QAReport != nil {
					candidate.QAReport.Passed = false
					candidate.QAReport.Status = model.ReviewStatusStale
				}
			}
		}
		result.AcceptedInvalidated = shot.AcceptedCandidateID != ""
		shot.AcceptedCandidateID = ""
		shot.QAStatus = model.ReviewStatusStale
		shot.ReviewStatus = model.ReviewStatusStale
		shot.Stale = true
		shot.LastRejectReason = result.StaleReason
	}
	return result
}

func markShotLayerStale(layers *model.TalkingHeadShotLayers, layer model.ShotLayerKind, reason string) {
	if layers == nil {
		return
	}
	var state *model.LayerArtifactState
	switch layer {
	case model.ShotLayerAudio:
		state = &layers.Audio.State
	case model.ShotLayerIP:
		state = &layers.IP.State
	case model.ShotLayerText:
		state = &layers.Text.State
	case model.ShotLayerBroll:
		state = &layers.Broll.State
	case model.ShotLayerComposition:
		state = &layers.Composition.State
	}
	if state != nil {
		state.Status = model.LayerStatusStale
		state.StaleReason = reason
	}
}

func containsShotLayer(values []model.ShotLayerKind, want model.ShotLayerKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type NarrativePosition string

const (
	NarrativePositionBody    NarrativePosition = "body"
	NarrativePositionHook    NarrativePosition = "hook"
	NarrativePositionCore    NarrativePosition = "core"
	NarrativePositionClosing NarrativePosition = "closing"
)

type TalkingHeadVisualModePolicy struct {
	MinDecisionConfidence float64 `json:"minDecisionConfidence"`
	MaxIPAbsentMs         int64   `json:"maxIpAbsentMs"`
}

func DefaultTalkingHeadVisualModePolicy() TalkingHeadVisualModePolicy {
	return TalkingHeadVisualModePolicy{MinDecisionConfidence: 0.6, MaxIPAbsentMs: 12000}
}

type VisualModeDecisionRequest struct {
	Position           NarrativePosition
	HasExactText       bool
	HasScreenRecording bool
	HasBroll           bool
	HasCaseOrEvidence  bool
	HasComplexConcept  bool
	NeedsDataGraphic   bool
	NeedsAIGCScene     bool
	EvidenceStrength   float64
}

type VisualModeDecision struct {
	Mode             model.VisualMode `json:"mode"`
	Reason           string           `json:"reason"`
	Confidence       float64          `json:"confidence"`
	NeedsHumanReview bool             `json:"needsHumanReview"`
}

func DecideTalkingHeadVisualMode(policy TalkingHeadVisualModePolicy, req VisualModeDecisionRequest) VisualModeDecision {
	if policy.MinDecisionConfidence <= 0 {
		policy = DefaultTalkingHeadVisualModePolicy()
	}
	decision := VisualModeDecision{Mode: model.VisualModeIPPrimary, Reason: "ordinary narration stays anchored on the IP", Confidence: 0.9}
	switch {
	case req.Position == NarrativePositionHook || req.Position == NarrativePositionClosing:
		if req.HasExactText {
			decision = VisualModeDecision{Mode: model.VisualModeIPWithText, Reason: "hook and closing keep the IP visible while deterministic text reinforces the message", Confidence: 0.95}
		} else {
			decision = VisualModeDecision{Mode: model.VisualModeIPPrimary, Reason: "hook and closing return to the IP anchor", Confidence: 0.95}
		}
	case req.HasScreenRecording:
		decision = VisualModeDecision{Mode: model.VisualModeScreenRecordingFull, Reason: "the narration explains an operation that requires a readable screen recording", Confidence: maxConfidence(req.EvidenceStrength, 0.85)}
	case req.NeedsDataGraphic || (req.HasComplexConcept && req.HasExactText):
		decision = VisualModeDecision{Mode: model.VisualModeTextGraphicsFullscreen, Reason: "structured deterministic graphics explain the concept more accurately than generated imagery", Confidence: maxConfidence(req.EvidenceStrength, 0.82)}
	case req.HasBroll && req.HasCaseOrEvidence:
		decision = VisualModeDecision{Mode: model.VisualModeBrollFullscreen, Reason: "the b-roll provides case or evidence content directly relevant to the narration", Confidence: maxConfidence(req.EvidenceStrength, 0.8)}
	case req.HasBroll:
		decision = VisualModeDecision{Mode: model.VisualModeIPWithBrollPIP, Reason: "relevant b-roll supports the narration without removing the IP anchor", Confidence: maxConfidence(req.EvidenceStrength, 0.72)}
	case req.NeedsAIGCScene:
		decision = VisualModeDecision{Mode: model.VisualModeAIGCFullscreen, Reason: "a described scene benefits from generated illustrative b-roll while audio remains primary", Confidence: maxConfidence(req.EvidenceStrength, 0.7)}
	case req.HasExactText:
		decision = VisualModeDecision{Mode: model.VisualModeIPWithText, Reason: "deterministic text reinforces the IP-led narration", Confidence: 0.9}
	}
	decision.NeedsHumanReview = decision.Confidence < policy.MinDecisionConfidence
	if decision.NeedsHumanReview {
		decision.Mode = model.VisualModeIPPrimary
		decision.Reason += "; confidence is below policy threshold, so the safe IP default requires review"
	}
	return decision
}

func maxConfidence(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func ValidateBrollManifest(manifest model.BrollManifest, publishing bool) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, entry := range manifest.Entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.ShotID) == "" || entry.EndMs <= entry.StartMs {
			issues = append(issues, ValidationIssue{Code: "broll_manifest_entry_invalid", Field: "entries", Message: "b-roll entry requires id, shotId, and a positive millisecond interval", Severity: "error"})
		}
		if strings.TrimSpace(entry.SourceURI) == "" && strings.TrimSpace(entry.ArtifactRef) == "" {
			issues = append(issues, ValidationIssue{Code: "broll_source_missing", Field: "sourceUri", Message: fmt.Sprintf("b-roll entry %s has no traceable source", entry.ID), Severity: "error"})
		}
		license := strings.ToLower(strings.TrimSpace(entry.LicenseStatus))
		if publishing && license != "cleared" && license != "owned" && license != "generated" {
			issues = append(issues, ValidationIssue{Code: "broll_license_blocks_publish", Field: "licenseStatus", Message: fmt.Sprintf("b-roll entry %s requires license confirmation before publish", entry.ID), Severity: "error"})
		}
	}
	return issues
}
