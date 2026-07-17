package service

import (
	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func BuildVideoDiagnosticsSnapshot(
	state model.ShotDrivenState,
	timeWindows model.TimeWindowPlan,
	assembly model.FinalAssemblyPlan,
	finalQA model.FinalQAReport,
) model.VideoDiagnosticsSnapshot {
	shotCandidates := []model.ShotCandidate{}
	shotReports := []model.ShotQAReport{}
	repairPlans := []model.RepairPlan{}
	manifest := append([]model.ArtifactProvenance{}, assembly.ArtifactProvenance...)
	summary := model.ProvenanceSummary{
		AcceptedShotCount:       len(assembly.AcceptedShots),
		NormalizedShotClipCount: len(assembly.AcceptedShots),
	}

	for _, shot := range state.Shots {
		for _, candidate := range shot.Candidates {
			shotCandidates = append(shotCandidates, candidate)
			if candidate.AttemptIndex == 0 {
				summary.RawShotCandidateCount++
			} else {
				summary.RepairedShotCandidateCount++
			}
			if candidate.QAReport != nil {
				shotReports = append(shotReports, *candidate.QAReport)
			}
			if candidate.RepairPlan != nil {
				repairPlans = append(repairPlans, *candidate.RepairPlan)
			}
			provenance := model.ArtifactProvenance{
				ArtifactID:  acceptedCandidateArtifactID(candidate.ArtifactRefs),
				Kind:        "shot_candidate",
				ShotID:      shot.ID,
				CandidateID: candidate.CandidateID,
				SourceType:  candidate.SourceType,
				IsFallback:  candidate.IsFallback,
			}
			if provenance.SourceType != "" {
				manifest = append(manifest, provenance)
			}
			if candidate.SourceType == model.ArtifactSourceAIGCVideo && !candidate.IsFallback {
				summary.RealAIGCVideoCount++
			}
			if candidate.IsFallback || candidate.SourceType == model.ArtifactSourceFallbackPreview || candidate.SourceType == model.ArtifactSourceFallbackStoryboard {
				summary.FallbackCount++
			}
		}
		repairPlans = append(repairPlans, shot.RepairPlans...)
	}

	if hasString(assembly.Steps, model.AssemblyStepFFmpegConcat) {
		summary.ConcatVideoCount = 1
	}
	if hasString(assembly.Steps, model.AssemblyStepGlobalAudioMix) {
		summary.FinalAudioMixCount = 1
	}
	if hasString(assembly.Steps, model.AssemblyStepGlobalSubtitleRender) {
		summary.FinalSubtitleTrackCount = 1
	}
	if finalQA.Passed {
		summary.FinalVideoCount = 1
	}

	return model.VideoDiagnosticsSnapshot{
		SchemaVersion:          1,
		ShotList:               state.Shots,
		ShotSplitReport:        timeWindows.SplitReport,
		ShotDurationValidation: timeWindows.SplitReport.DurationValidation,
		ShotCandidates:         shotCandidates,
		ShotQAReports:          shotReports,
		RepairPlans:            repairPlans,
		AcceptedShots:          assembly.AcceptedShots,
		AssemblyPlan:           assembly,
		SubtitleTimeline:       assembly.SubtitleTimeline,
		AudioMixPlan:           assembly.AudioMixPlan,
		FinalQAReport:          finalQA,
		ArtifactManifest:       manifest,
		ProvenanceSummary:      summary,
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
