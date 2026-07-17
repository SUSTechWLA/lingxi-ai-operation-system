package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

var finalAssemblyOnlyDimensions = map[string]bool{
	"final_subtitle": true,
	"subtitle":       true,
	"bgm":            true,
	"voiceover":      true,
	"audio":          true,
	"loudness":       true,
}

func RecordShotCandidateQA(shot model.ShotUnit, candidate model.ShotCandidate, report model.ShotQAReport, policy model.ShotRepairPolicy) model.ShotUnit {
	if policy.MaxRepairAttemptsPerShot <= 0 {
		policy = model.DefaultShotRepairPolicy()
	}
	if candidate.CandidateID == "" {
		candidate.CandidateID = fmt.Sprintf("%s-candidate-%02d", shot.ID, len(shot.Candidates)+1)
	}
	if candidate.ShotID == "" {
		candidate.ShotID = shot.ID
	}
	if candidate.DurationSec <= 0 && shot.DurationSec > 0 {
		candidate.DurationSec = float64(shot.DurationSec)
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	if report.ShotID == "" {
		report.ShotID = shot.ID
	}
	if report.CandidateID == "" {
		report.CandidateID = candidate.CandidateID
	}
	candidate.QAReport = &report
	candidate = model.NormalizeShotCandidateExecution(candidate)
	if shot.TimelineRevision != "" && candidate.TimelineRevision != shot.TimelineRevision {
		candidate.Status = model.CandidateHumanReviewRequired
		repair := model.RepairPlan{
			Action:            model.RepairActionRealignTimeline,
			Reason:            fmt.Sprintf("candidate timeline revision %q does not match shot audio master %q", candidate.TimelineRevision, shot.TimelineRevision),
			Severity:          "hard_fail",
			TargetShotID:      shot.ID,
			SourceCandidateID: candidate.CandidateID,
			AttemptIndex:      candidate.AttemptIndex,
			Preserve:          true,
			RepairTargets:     []string{"timeline_revision", "subtitle_timing", "lip_sync", "broll_timing"},
			NextToolCall:      model.RepairActionRealignTimeline,
		}
		candidate.RepairPlan = &repair
		shot.Candidates = append(shot.Candidates, candidate)
		shot.RepairPlans = append(shot.RepairPlans, repair)
		shot.QAStatus = model.ShotHumanReviewRequired
		shot.ReviewStatus = model.ReviewStatusPending
		shot.AcceptedCandidateID = ""
		shot.Stale = true
		shot.LastRejectReason = repair.Reason
		return shot
	}

	if report.Passed || report.HumanApproved || report.Status == model.ShotQAPassed {
		if model.IsStrictProductionMode(policy.ProductionMode) && !candidate.ProductionEligible {
			candidate.Status = model.CandidateHumanReviewRequired
			repair := model.RepairPlan{
				Action:            model.RepairActionHumanReview,
				Reason:            "candidate execution mode is not eligible for strict production acceptance",
				Severity:          "hard_fail",
				TargetShotID:      shot.ID,
				SourceCandidateID: candidate.CandidateID,
				AttemptIndex:      candidate.AttemptIndex,
				Preserve:          true,
				RepairTargets:     []string{"production_eligibility", "provenance"},
				NextToolCall:      model.RepairActionHumanReview,
			}
			candidate.RepairPlan = &repair
			shot.Candidates = append(shot.Candidates, candidate)
			shot.RepairPlans = append(shot.RepairPlans, repair)
			shot.QAStatus = model.ShotHumanReviewRequired
			shot.ReviewStatus = model.ReviewStatusPending
			shot.AcceptedCandidateID = ""
			return shot
		}
		candidate.Status = model.CandidateAcceptedForAssembly
		shot.Candidates = append(shot.Candidates, candidate)
		shot.QAStatus = model.ShotAcceptedForAssembly
		shot.AcceptedCandidateID = candidate.CandidateID
		shot.ReviewStatus = model.ReviewStatusApproved
		shot.Stale = false
		shot.ArtifactRefs = candidate.ArtifactRefs
		return shot
	}

	candidate.Status = model.CandidateShotQAFailed
	shot.Candidates = append(shot.Candidates, candidate)
	shot.QAStatus = model.ShotQAFailed
	shot.ReviewStatus = model.ReviewStatusPending

	if candidate.AttemptIndex >= policy.MaxRepairAttemptsPerShot {
		shot.QAStatus = model.ShotHumanReviewRequired
		return shot
	}

	repair := BuildRepairPlanForQA(shot.ID, candidate, report, policy)
	candidate.RepairPlan = &repair
	shot.Candidates[len(shot.Candidates)-1] = candidate
	shot.RepairPlans = append(shot.RepairPlans, repair)
	return shot
}

func BuildRepairPlanForQA(shotID string, candidate model.ShotCandidate, report model.ShotQAReport, policy model.ShotRepairPolicy) model.RepairPlan {
	action := repairActionForFailedDimensions(report)
	locked := []string{}
	if policy.PreservePassedDimensions {
		locked = lockablePassedDimensions(report.PassedDimensions)
	}
	return model.RepairPlan{
		Action:              action,
		Reason:              repairReason(action, report),
		Severity:            report.Severity,
		TargetShotID:        shotID,
		SourceCandidateID:   candidate.CandidateID,
		AttemptIndex:        candidate.AttemptIndex + 1,
		Preserve:            policy.PreservePassedDimensions,
		LockedDimensions:    locked,
		RepairTargets:       uniqueStrings(report.FailedDimensions),
		PromptPatch:         promptPatchForRepair(action),
		RenderStrategyPatch: renderStrategyPatchForRepair(action),
		NextToolCall:        nextToolCallForRepair(action),
	}
}

func repairActionForFailedDimensions(report model.ShotQAReport) string {
	failed := normalizedDimensionSet(report.FailedDimensions)
	if len(failed) > 0 {
		allFinalOnly := true
		for dim := range failed {
			if !finalAssemblyOnlyDimensions[dim] {
				allFinalOnly = false
				break
			}
		}
		if allFinalOnly {
			return model.RepairActionDeferToFinalAssembly
		}
	}
	if failed["voice_generation"] || failed["voice_profile"] || failed["clipping"] || failed["abnormal_silence"] {
		return model.RepairActionRegenerateVoice
	}
	if failed["timeline_revision"] || failed["cue_timing"] || failed["audio_alignment"] {
		return model.RepairActionRealignTimeline
	}
	if failed["text_intent"] || failed["text_layout"] || failed["safe_area"] || failed["screen_text"] {
		return model.RepairActionRerenderText
	}
	if failed["lip_sync"] || failed["viseme"] {
		return model.RepairActionRelipsyncIP
	}
	if failed["ip_asset_pack"] || failed["character_identity"] || failed["gesture"] || failed["expression"] {
		return model.RepairActionRegenerateIP
	}
	if failed["broll_source"] || failed["broll_license"] || failed["broll_relevance"] || failed["broll_crop"] {
		return model.RepairActionReplaceBroll
	}
	if failed["broll_generation"] || failed["broll_quality"] {
		return model.RepairActionRegenerateBroll
	}
	if failed["encode"] || failed["codec"] || failed["pixel_format"] {
		return model.RepairActionReencode
	}
	if failed["scene"] || failed["action"] || failed["temporal_stability"] {
		if strings.EqualFold(report.Severity, "severe") {
			return model.RepairActionRegenAIGCWithReference
		}
		return model.RepairActionRegenAIGC
	}
	if failed["prompt_alignment"] {
		return model.RepairActionPromptPatchRegen
	}
	if failed["layout"] || failed["composite"] || failed["artifact"] {
		return model.RepairActionRecomposite
	}
	return model.RepairActionRecomposite
}

func lockablePassedDimensions(values []string) []string {
	allowed := map[string]bool{
		"prompt_alignment":   true,
		"character_identity": true,
		"scene":              true,
		"action":             true,
		"text_intent":        true,
		"duration":           true,
		"style":              true,
	}
	out := []string{}
	for _, value := range values {
		key := strings.TrimSpace(value)
		if allowed[key] {
			out = append(out, key)
		}
	}
	return uniqueStrings(out)
}

func normalizedDimensionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func repairReason(action string, report model.ShotQAReport) string {
	if report.Summary != "" {
		return report.Summary
	}
	switch action {
	case model.RepairActionRerenderText:
		return "text, subtitle, or safe-area issue can be repaired locally without changing passed visual intent"
	case model.RepairActionDeferToFinalAssembly:
		return "global audio/subtitle issue should be fixed during final assembly"
	case model.RepairActionRegenAIGC, model.RepairActionRegenAIGCWithReference:
		return "AIGC regeneration is required for failed character, scene, action, or temporal intent"
	default:
		return "candidate did not pass shot QA and needs a scoped repair"
	}
}

func promptPatchForRepair(action string) map[string]interface{} {
	switch action {
	case model.RepairActionRerenderText, model.RepairActionRecomposite:
		return map[string]interface{}{"negativeAdditions": []string{"no embedded text", "no watermark", "no subtitles inside generated video"}}
	case model.RepairActionRegenAIGC, model.RepairActionRegenAIGCWithReference:
		return map[string]interface{}{"requiredAdditions": []string{"preserve approved character, scene, action, and style dimensions"}}
	case model.RepairActionPromptPatchRegen:
		return map[string]interface{}{"negativeAdditions": []string{"internal production terms", "ffmpeg", "artifact", "storageRef"}}
	default:
		return map[string]interface{}{}
	}
}

func renderStrategyPatchForRepair(action string) map[string]interface{} {
	switch action {
	case model.RepairActionRerenderText:
		return map[string]interface{}{"mode": "hybrid", "htmlRequired": true, "textOverlayNeeded": true, "needsCompositing": true}
	case model.RepairActionRecomposite:
		return map[string]interface{}{"needsCompositing": true}
	case model.RepairActionRegenAIGC, model.RepairActionRegenAIGCWithReference:
		return map[string]interface{}{"mode": "aigc_video", "fallbackAllowed": false}
	case model.RepairActionDeferToFinalAssembly:
		return map[string]interface{}{"deferGlobalSubtitleAudio": true}
	default:
		return map[string]interface{}{}
	}
}

func nextToolCallForRepair(action string) string {
	switch action {
	case model.RepairActionRerenderText,
		model.RepairActionRegenerateVoice,
		model.RepairActionRealignTimeline,
		model.RepairActionRegenerateIP,
		model.RepairActionRelipsyncIP,
		model.RepairActionReplaceBroll,
		model.RepairActionRegenerateBroll,
		model.RepairActionReencode:
		return action
	case model.RepairActionRecomposite:
		return "FFMPEG_RECOMPOSITE"
	case model.RepairActionRegenAIGC:
		return "REGEN_AIGC"
	case model.RepairActionRegenAIGCWithReference:
		return "REGEN_AIGC_WITH_REFERENCE"
	case model.RepairActionDeferToFinalAssembly:
		return "FINAL_ASSEMBLY"
	case model.RepairActionPromptPatchRegen:
		return "PROMPT_PATCH_REGEN"
	default:
		return "HUMAN_REVIEW"
	}
}
