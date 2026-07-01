package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

const (
	minAIGCWindowDurationSec    = 3.0
	maxAIGCWindowDurationSec    = 15.0
	preferredCinematicWindowSec = 10.0
	talkingHeadWindowReason     = "script-aligned talking head window"
	defaultWindowDurationSec    = 6.0
	cinematicWindowReason       = "cinematic coarse shot split into AIGC-safe 3-15s window"
	cinematicTransitionReason   = "cinematic window is shorter than AIGC minimum and should be handled as transition/overlay"
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
	parentIDCounts := map[string]int{}
	for shotIndex, shot := range req.Shots {
		parentShotID := effectiveCinematicParentShotID(shot.ID, shotIndex, parentIDCounts)
		durationSec := float64(shot.DurationSec)
		if durationSec <= 0 {
			durationSec = defaultWindowDurationSec
		}
		if durationSec < minAIGCWindowDurationSec {
			windowID := fmt.Sprintf("%s_TW_01", parentShotID)
			plan.Windows = append(plan.Windows, model.TimeWindowUnit{
				ID:              windowID,
				ShotID:          windowID,
				ParentShotID:    parentShotID,
				SequenceIndex:   sequenceIndex,
				StartSec:        0,
				EndSec:          durationSec,
				DurationSec:     durationSec,
				SceneSummary:    shot.SceneSummary,
				MainAction:      shot.MainAction,
				AIGCEligible:    false,
				RecommendedMode: model.GenerationModeHTMLOnly,
				Reason:          cinematicTransitionReason,
			})
			sequenceIndex++
			continue
		}

		windowCount := cinematicWindowCount(durationSec)
		windowDurationSec := durationSec / float64(windowCount)
		for i := 0; i < windowCount; i++ {
			startSec := float64(i) * windowDurationSec
			endSec := startSec + windowDurationSec
			windowID := fmt.Sprintf("%s_TW_%02d", parentShotID, i+1)
			plan.Windows = append(plan.Windows, model.TimeWindowUnit{
				ID:              windowID,
				ShotID:          windowID,
				ParentShotID:    parentShotID,
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
	}

	return plan
}

func effectiveCinematicParentShotID(rawID string, shotIndex int, counts map[string]int) string {
	baseID := strings.TrimSpace(rawID)
	if baseID == "" {
		baseID = fmt.Sprintf("SHOT_%02d", shotIndex+1)
	}
	counts[baseID]++
	if counts[baseID] == 1 {
		return baseID
	}
	return fmt.Sprintf("%s_DUP_%02d", baseID, counts[baseID])
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
