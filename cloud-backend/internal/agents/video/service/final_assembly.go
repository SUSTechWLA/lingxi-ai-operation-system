package service

import (
	"fmt"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func BuildFinalAssemblyPlan(shots []model.ShotUnit) (model.FinalAssemblyPlan, []ValidationIssue) {
	plan := model.FinalAssemblyPlan{
		Status:      "planned",
		Resolution:  "1920x1080",
		FPS:         30,
		PixelFormat: "yuv420p",
		Codec:       "h264",
		Steps: []string{
			model.AssemblyStepAllShotsAcceptedGate,
			model.AssemblyStepNormalizeAcceptedShots,
			model.AssemblyStepFFmpegConcat,
			model.AssemblyStepGlobalVoiceoverAlign,
			model.AssemblyStepGlobalAudioMix,
			model.AssemblyStepGlobalSubtitleRender,
			model.AssemblyStepFinalVideoQA,
			model.AssemblyStepExportPublish,
		},
		SubtitleTimeline: model.SubtitleTimeline{Scope: model.AssemblyScopeGlobal},
		AudioMixPlan: model.AudioMixPlan{
			Scope:          model.AssemblyScopeGlobal,
			VoiceoverAlign: true,
			BGMDucking:     true,
			TargetLUFS:     -16,
		},
	}

	issues := make([]ValidationIssue, 0)
	cursor := 0.0
	for _, shot := range shots {
		if durationIssues := CheckShotDurationSec(shot.ID, float64(shot.DurationSec)); len(durationIssues) > 0 {
			issues = append(issues, durationIssues...)
		}
		candidate, ok := acceptedCandidateForShot(shot)
		if !ok {
			issues = append(issues, ValidationIssue{
				Code:     "final_assembly_requires_accepted_candidate",
				Field:    "acceptedCandidateId",
				Message:  "final assembly can only use accepted shot candidates: " + shot.ID,
				Severity: "error",
			})
			continue
		}
		if candidate.Status == model.CandidateShotQAFailed || candidate.QAReport == nil || (!candidate.QAReport.Passed && !candidate.QAReport.HumanApproved) {
			issues = append(issues, ValidationIssue{
				Code:     "final_assembly_rejects_failed_candidate",
				Field:    "candidates",
				Message:  "failed candidate cannot enter final assembly: " + candidate.CandidateID,
				Severity: "error",
			})
			continue
		}
		artifactID := acceptedCandidateArtifactID(candidate.ArtifactRefs)
		if artifactID == "" {
			issues = append(issues, ValidationIssue{
				Code:     "final_assembly_requires_shot_video",
				Field:    "artifactRefs",
				Message:  "accepted candidate requires a video artifact: " + candidate.CandidateID,
				Severity: "error",
			})
			continue
		}
		duration := candidate.DurationSec
		if duration <= 0 {
			duration = float64(shot.DurationSec)
		}
		plan.AcceptedShots = append(plan.AcceptedShots, model.AcceptedShotRef{
			ShotID:      shot.ID,
			CandidateID: candidate.CandidateID,
			DurationSec: duration,
			SourceType:  candidate.SourceType,
			IsFallback:  candidate.IsFallback,
			ArtifactID:  artifactID,
		})
		plan.SubtitleTimeline.Cues = append(plan.SubtitleTimeline.Cues, model.SubtitleCue{
			ShotID:   shot.ID,
			StartSec: cursor,
			EndSec:   cursor + duration,
			Text:     shot.Narration,
		})
		plan.ArtifactProvenance = append(plan.ArtifactProvenance, model.ArtifactProvenance{
			ArtifactID:  artifactID,
			Kind:        "accepted_shot",
			ShotID:      shot.ID,
			CandidateID: candidate.CandidateID,
			SourceType:  candidate.SourceType,
			IsFallback:  candidate.IsFallback,
		})
		cursor += duration
	}
	if len(issues) > 0 {
		plan.Status = "blocked"
	}
	return plan, issues
}

func CheckFinalAssembly(shots []model.ShotUnit) []ValidationIssue {
	_, issues := BuildFinalAssemblyPlan(shots)
	for _, shot := range shots {
		if shot.ReviewStatus != "" && shot.ReviewStatus != model.ReviewStatusApproved {
			issues = append(issues, ValidationIssue{
				Code:     "final_assembly_requires_approved_shot",
				Field:    "reviewStatus",
				Message:  "final assembly can only use approved shots: " + shot.ID,
				Severity: "error",
			})
		}
		if shot.Stale {
			issues = append(issues, ValidationIssue{
				Code:     "final_assembly_rejects_stale_shot",
				Field:    "stale",
				Message:  "final assembly cannot use stale shots: " + shot.ID,
				Severity: "error",
			})
		}
	}
	return issues
}

func CheckFinalExportGate(report model.FinalQAReport) []ValidationIssue {
	if !report.Passed {
		return []ValidationIssue{{
			Code:     "final_qa_failed_blocks_export",
			Field:    "finalQaReport",
			Message:  fmt.Sprintf("final QA must pass before export or publish, status=%s", report.Status),
			Severity: "error",
		}}
	}
	return nil
}

func acceptedCandidateForShot(shot model.ShotUnit) (model.ShotCandidate, bool) {
	if shot.AcceptedCandidateID == "" {
		return model.ShotCandidate{}, false
	}
	for _, candidate := range shot.Candidates {
		if candidate.CandidateID == shot.AcceptedCandidateID {
			return candidate, true
		}
	}
	return model.ShotCandidate{}, false
}

func acceptedCandidateArtifactID(refs model.ShotArtifactRefs) string {
	for _, id := range []string{
		refs.CompositedShotVideoArtifactID,
		refs.VideoClipArtifactID,
		refs.AIGCBackgroundVideoArtifactID,
		refs.HTMLPreviewVideoArtifactID,
	} {
		if id != "" {
			return id
		}
	}
	return ""
}
