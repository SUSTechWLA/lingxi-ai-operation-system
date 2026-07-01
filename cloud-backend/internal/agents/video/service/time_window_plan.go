package service

import (
	"fmt"
	"math"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

const (
	minAIGCWindowDurationSec       = 3.0
	maxAIGCWindowDurationSec       = 15.0
	preferredCinematicWindowSec    = 10.0
	talkingHeadWindowReason        = "script-aligned talking head window"
	defaultWindowDurationSec       = 6.0
	cinematicWindowReason          = "cinematic coarse shot split into AIGC-safe 3-15s window"
	shortCinematicWindowWarningFmt = "shot %s duration %.2fs was extended to 3.00s for AIGC eligibility"
)

type TimeWindowRequest struct {
	Profile     model.VideoCreationProfile
	Shots       []model.ShotUnit
	ScriptSpans []model.ScriptSpan
}

func BuildTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	if req.Profile.ProfileID == model.VideoProfileCinematicStory {
		return buildCinematicTimeWindowPlan(req)
	}
	return buildTalkingHeadTimeWindowPlan(req)
}

func buildTalkingHeadTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	plan := model.TimeWindowPlan{
		ProfileID: model.VideoProfileTalkingHead,
		Windows:   make([]model.TimeWindowUnit, 0, len(req.ScriptSpans)),
	}

	for i, span := range req.ScriptSpans {
		durationSec := span.EndSec - span.StartSec
		endSec := span.EndSec
		if durationSec <= 0 {
			durationSec = defaultWindowDurationSec
			endSec = span.StartSec + durationSec
		}
		windowID := fmt.Sprintf("TW_%02d", i+1)
		shotID := fmt.Sprintf("SHOT_%02d", i+1)

		plan.Windows = append(plan.Windows, model.TimeWindowUnit{
			ID:              windowID,
			ShotID:          shotID,
			SequenceIndex:   i,
			StartSec:        span.StartSec,
			EndSec:          endSec,
			DurationSec:     durationSec,
			ScriptSpanID:    span.ID,
			ScriptText:      span.Text,
			AIGCEligible:    isAIGCWindowDuration(durationSec),
			RecommendedMode: model.GenerationModeHTMLOnly,
			Reason:          talkingHeadWindowReason,
		})
	}

	return plan
}

func buildCinematicTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	plan := model.TimeWindowPlan{
		ProfileID: req.Profile.ProfileID,
	}

	var sequenceIndex int
	var timelineStartSec float64
	for _, shot := range req.Shots {
		durationSec := float64(shot.DurationSec)
		if durationSec <= 0 {
			durationSec = defaultWindowDurationSec
		}
		if durationSec < minAIGCWindowDurationSec {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(shortCinematicWindowWarningFmt, shot.ID, durationSec))
			durationSec = minAIGCWindowDurationSec
		}

		windowCount := cinematicWindowCount(durationSec)
		windowDurationSec := durationSec / float64(windowCount)
		for i := 0; i < windowCount; i++ {
			startSec := timelineStartSec + (float64(i) * windowDurationSec)
			endSec := startSec + windowDurationSec
			windowID := fmt.Sprintf("%s_TW_%02d", shot.ID, i+1)
			plan.Windows = append(plan.Windows, model.TimeWindowUnit{
				ID:              windowID,
				ShotID:          windowID,
				ParentShotID:    shot.ID,
				SequenceIndex:   sequenceIndex,
				StartSec:        startSec,
				EndSec:          endSec,
				DurationSec:     windowDurationSec,
				SceneSummary:    shot.SceneSummary,
				MainAction:      shot.MainAction,
				AIGCEligible:    true,
				RecommendedMode: model.GenerationModeAIGCVideo,
				Reason:          cinematicWindowReason,
			})
			sequenceIndex++
		}
		timelineStartSec += durationSec
	}

	return plan
}

func cinematicWindowCount(durationSec float64) int {
	windowCount := int(math.Ceil(durationSec / preferredCinematicWindowSec))
	if windowCount < 1 {
		return 1
	}
	for durationSec/float64(windowCount) > maxAIGCWindowDurationSec {
		windowCount++
	}
	for windowCount > 1 && durationSec/float64(windowCount) < minAIGCWindowDurationSec {
		windowCount--
	}
	return windowCount
}

func isAIGCWindowDuration(durationSec float64) bool {
	return durationSec >= minAIGCWindowDurationSec && durationSec <= maxAIGCWindowDurationSec
}
