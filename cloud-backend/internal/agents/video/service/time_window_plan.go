package service

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

// NormalizeNarrationForComparison removes presentation-only whitespace while
// preserving authored words and punctuation. It is intentionally exported so
// every pipeline boundary can apply the same coverage rule.
func NormalizeNarrationForComparison(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(text))
}

// ValidateCanonicalTimeWindowPlan proves that a talking-head Shot sequence is
// a lossless partition of the audio master. Downstream tools may enrich these
// windows, but they must never invent their own narration or timing.
func ValidateCanonicalTimeWindowPlan(master model.AudioMasterTimeline, plan model.TimeWindowPlan) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if master.Revision == "" || master.DurationMs <= 0 {
		return append(issues, ValidationIssue{
			Code: "audio_master_invalid", Field: "audioMaster", Severity: "error",
			Message: "canonical shot validation requires a revisioned audio master with positive duration",
		})
	}
	if plan.TimelineRevision != master.Revision {
		issues = append(issues, ValidationIssue{
			Code: "shot_timeline_revision_mismatch", Field: "timelineRevision", Severity: "error",
			Message: fmt.Sprintf("time-window revision %q does not match audio master %q", plan.TimelineRevision, master.Revision),
		})
	}
	if len(plan.Windows) == 0 {
		return append(issues, ValidationIssue{
			Code: "shot_timeline_empty", Field: "windows", Severity: "error",
			Message: "canonical shot timeline must contain at least one window",
		})
	}

	windowRanges := make([][2]int64, 0, len(plan.Windows))
	windowNarration := strings.Builder{}
	var previousEnd int64
	for index, window := range plan.Windows {
		startMs := window.StartMs
		endMs := window.EndMs
		if startMs == 0 && window.StartSec != 0 {
			startMs = MillisecondsFromSeconds(window.StartSec)
		}
		if endMs == 0 && window.EndSec != 0 {
			endMs = MillisecondsFromSeconds(window.EndSec)
		}
		if endMs <= startMs && window.DurationMs > 0 {
			endMs = startMs + window.DurationMs
		}
		if endMs <= startMs && window.DurationSec > 0 {
			endMs = startMs + MillisecondsFromSeconds(window.DurationSec)
		}
		if index == 0 && startMs != 0 {
			issues = append(issues, ValidationIssue{
				Code: "shot_timeline_gap", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("first shot starts at %d ms instead of 0", startMs),
			})
		}
		if index > 0 && startMs > previousEnd {
			issues = append(issues, ValidationIssue{
				Code: "shot_timeline_gap", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("gap before %s: previous end %d ms, start %d ms", window.ShotID, previousEnd, startMs),
			})
		}
		if index > 0 && startMs < previousEnd {
			issues = append(issues, ValidationIssue{
				Code: "shot_timeline_overlap", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("overlap before %s: previous end %d ms, start %d ms", window.ShotID, previousEnd, startMs),
			})
		}
		if startMs < 0 || endMs <= startMs || endMs > master.DurationMs {
			issues = append(issues, ValidationIssue{
				Code: "shot_timeline_window_invalid", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("shot %s range [%d,%d) is outside audio master duration %d", window.ShotID, startMs, endMs, master.DurationMs),
			})
		}
		if window.TimelineRevision != "" && window.TimelineRevision != master.Revision {
			issues = append(issues, ValidationIssue{
				Code: "shot_timeline_revision_mismatch", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("shot %s uses timeline revision %q instead of %q", window.ShotID, window.TimelineRevision, master.Revision),
			})
		}
		windowRanges = append(windowRanges, [2]int64{startMs, endMs})
		windowNarration.WriteString(window.ScriptText)
		previousEnd = endMs
	}
	if previousEnd < master.DurationMs {
		issues = append(issues, ValidationIssue{
			Code: "shot_timeline_gap", Field: "windows", Severity: "error",
			Message: fmt.Sprintf("shot timeline ends at %d ms before audio master duration %d", previousEnd, master.DurationMs),
		})
	} else if previousEnd > master.DurationMs {
		issues = append(issues, ValidationIssue{
			Code: "shot_timeline_window_invalid", Field: "windows", Severity: "error",
			Message: fmt.Sprintf("shot timeline ends at %d ms after audio master duration %d", previousEnd, master.DurationMs),
		})
	}

	expectedNarration := strings.Builder{}
	for _, cue := range master.Sentences {
		expectedNarration.WriteString(cue.Text)
		coverage := 0
		for _, windowRange := range windowRanges {
			if cue.StartMs >= windowRange[0] && cue.EndMs <= windowRange[1] {
				coverage++
			}
		}
		if coverage != 1 {
			issues = append(issues, ValidationIssue{
				Code: "shot_narration_cue_coverage_invalid", Field: "windows", Severity: "error",
				Message: fmt.Sprintf("audio cue %s is covered by %d shot windows, want exactly one", cue.ID, coverage),
			})
		}
	}
	if NormalizeNarrationForComparison(windowNarration.String()) != NormalizeNarrationForComparison(expectedNarration.String()) {
		issues = append(issues, ValidationIssue{
			Code: "shot_narration_coverage_mismatch", Field: "windows.scriptText", Severity: "error",
			Message: "concatenated shot narration does not equal the canonical audio-master narration",
		})
	}
	return issues
}

const (
	minAIGCWindowDurationSec    = 3.0
	maxAIGCWindowDurationSec    = 15.0
	preferredCinematicWindowSec = 7.0
	talkingHeadWindowReason     = "script-aligned talking head window"
	defaultWindowDurationSec    = 6.0
	cinematicWindowReason       = "cinematic coarse shot split into AIGC-safe 3-15s window"
	cinematicTransitionReason   = "cinematic window is shorter than AIGC minimum and should be handled as transition/overlay"
)

type TimeWindowRequest struct {
	Profile     model.VideoCreationProfile
	Shots       []model.ShotUnit
	ScriptSpans []model.ScriptSpan
	AudioMaster *model.AudioMasterTimeline
}

func BuildTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	if req.AudioMaster != nil {
		req.ScriptSpans = scriptSpansFromAudioMaster(*req.AudioMaster)
	}
	var plan model.TimeWindowPlan
	if req.Profile.ProfileID == model.VideoProfileCinematicStory {
		plan = buildCinematicTimeWindowPlan(req)
	} else {
		plan = buildTalkingHeadTimeWindowPlan(req)
	}
	plan.SchemaVersion = model.TalkingHeadSchemaVersion
	if req.AudioMaster != nil {
		plan.TimelineRevision = req.AudioMaster.Revision
	}
	for index := range plan.Windows {
		window := &plan.Windows[index]
		window.StartMs = MillisecondsFromSeconds(window.StartSec)
		window.EndMs = MillisecondsFromSeconds(window.EndSec)
		if window.EndMs > window.StartMs {
			window.DurationMs = window.EndMs - window.StartMs
		} else {
			window.DurationMs = MillisecondsFromSeconds(window.DurationSec)
			window.EndMs = window.StartMs + window.DurationMs
		}
		window.StartSec = SecondsFromMilliseconds(window.StartMs)
		window.EndSec = SecondsFromMilliseconds(window.EndMs)
		window.DurationSec = SecondsFromMilliseconds(window.DurationMs)
		window.TimelineRevision = plan.TimelineRevision
	}
	return plan
}

func scriptSpansFromAudioMaster(master model.AudioMasterTimeline) []model.ScriptSpan {
	spans := make([]model.ScriptSpan, 0, len(master.Sentences))
	for _, cue := range master.Sentences {
		spans = append(spans, model.ScriptSpan{
			ID:       cue.ID,
			StartSec: SecondsFromMilliseconds(cue.StartMs),
			EndSec:   SecondsFromMilliseconds(cue.EndMs),
			Text:     cue.Text,
			Emotion:  cue.Emotion,
		})
	}
	return spans
}

func buildTalkingHeadTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	plan := model.TimeWindowPlan{
		SchemaVersion: model.TalkingHeadSchemaVersion,
		ProfileID:     model.VideoProfileTalkingHead,
		Windows:       make([]model.TimeWindowUnit, 0, len(req.ScriptSpans)),
		SplitReport:   defaultShotSplitReport(),
	}

	drafts, forcedSplits := scriptSpanWindowDrafts(req.ScriptSpans)
	windows, mergeCount := mergeShortCompatibleDrafts(drafts)
	plan.SplitReport.ForcedSplitCount = forcedSplits
	plan.SplitReport.MergeCount = mergeCount

	for i, draft := range windows {
		windowID := fmt.Sprintf("TW_%02d", i+1)
		shotID := fmt.Sprintf("SHOT_%02d", i+1)
		window := model.TimeWindowUnit{
			ID:                 windowID,
			ShotID:             shotID,
			SequenceIndex:      i,
			StartSec:           draft.StartSec,
			EndSec:             draft.EndSec,
			DurationSec:        draft.DurationSec,
			ScriptSpanID:       strings.Join(draft.SpanIDs, "+"),
			ScriptText:         strings.Join(nonEmptyStrings(draft.Texts), " "),
			SceneSummary:       draft.Scene,
			Subject:            draft.Subject,
			Action:             draft.Action,
			MainAction:         draft.Action,
			Camera:             draft.Camera,
			ShotSize:           draft.ShotSize,
			Framing:            draft.Framing,
			VisualChangeReason: draft.VisualChangeReason,
			RecommendedMode:    model.GenerationModeHTMLOnly,
			Reason:             talkingHeadWindowReason,
		}
		window.AIGCEligible = isAIGCWindowDuration(window.DurationSec)
		plan.Windows = append(plan.Windows, window)
		if issues := CheckShotDurationSec(window.ShotID, window.DurationSec); len(issues) > 0 {
			plan.SplitReport.DurationValidation = append(plan.SplitReport.DurationValidation, window.ShotID)
		}
	}

	return plan
}

func buildCinematicTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	plan := model.TimeWindowPlan{
		SchemaVersion: model.TalkingHeadSchemaVersion,
		ProfileID:     req.Profile.ProfileID,
		Windows:       make([]model.TimeWindowUnit, 0, len(req.Shots)),
		SplitReport:   defaultShotSplitReport(),
	}

	var sequenceIndex int
	emittedParentIDs := map[string]bool{}
	for shotIndex, shot := range req.Shots {
		parentShotID := effectiveCinematicParentShotID(shot.ID, shotIndex, emittedParentIDs)
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
		if windowCount > 1 {
			plan.SplitReport.ForcedSplitCount += windowCount - 1
		}
		windowDurationSec := durationSec / float64(windowCount)
		for i := 0; i < windowCount; i++ {
			startSec := float64(i) * windowDurationSec
			endSec := startSec + windowDurationSec
			windowID := fmt.Sprintf("%s_TW_%02d", parentShotID, i+1)
			plan.Windows = append(plan.Windows, model.TimeWindowUnit{
				ID:                 windowID,
				ShotID:             windowID,
				ParentShotID:       parentShotID,
				SequenceIndex:      sequenceIndex,
				StartSec:           startSec,
				EndSec:             endSec,
				DurationSec:        windowDurationSec,
				SceneSummary:       shot.SceneSummary,
				MainAction:         shot.MainAction,
				Subject:            shot.Subject,
				Action:             firstNonEmptyString(shot.Action, shot.MainAction),
				Camera:             shot.Camera,
				ShotSize:           shot.ShotSize,
				Framing:            shot.Framing,
				VisualChangeReason: cinematicWindowVisualReason(durationSec, windowCount),
				AIGCEligible:       true,
				RecommendedMode:    model.GenerationModeAIGCVideo,
				Reason:             cinematicWindowReason,
			})
			sequenceIndex++
		}
	}

	return plan
}

func effectiveCinematicParentShotID(rawID string, shotIndex int, used map[string]bool) string {
	baseID := strings.TrimSpace(rawID)
	if baseID == "" {
		baseID = fmt.Sprintf("SHOT_%02d", shotIndex+1)
	}
	if !used[baseID] {
		used[baseID] = true
		return baseID
	}
	for suffix := 2; ; suffix++ {
		candidateID := fmt.Sprintf("%s_DUP_%02d", baseID, suffix)
		if !used[candidateID] {
			used[candidateID] = true
			return candidateID
		}
	}
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

type timeWindowDraft struct {
	SpanIDs            []string
	Texts              []string
	StartSec           float64
	EndSec             float64
	DurationSec        float64
	Scene              string
	Subject            string
	Action             string
	Camera             string
	ShotSize           string
	Framing            string
	Visual             string
	Emotion            string
	VisualChangeReason string
}

func defaultShotSplitReport() model.ShotSplitReport {
	policy := model.DefaultShotPolicy()
	return model.ShotSplitReport{
		Policy:                 policy,
		SplitByScriptSemantics: policy.SplitByScriptSemantics,
		SplitByVisualChange:    policy.SplitByVisualChange,
		PolicyReasons: []string{
			"scene_change",
			"subject_change",
			"action_goal_change",
			"camera_or_shot_size_change",
			"emotion_or_information_change",
			"duration_outside_3_15s",
		},
	}
}

func scriptSpanWindowDrafts(spans []model.ScriptSpan) ([]timeWindowDraft, int) {
	drafts := make([]timeWindowDraft, 0, len(spans))
	forcedSplits := 0
	for _, span := range spans {
		durationSec := span.EndSec - span.StartSec
		if durationSec <= 0 {
			durationSec = defaultWindowDurationSec
			span.EndSec = span.StartSec + durationSec
		}
		if durationSec <= maxAIGCWindowDurationSec {
			draft := draftFromScriptSpan(span, span.StartSec, span.EndSec, durationSec)
			if len(drafts) == 0 {
				draft.VisualChangeReason = "script_semantic_start"
			} else {
				draft.VisualChangeReason = visualChangeReason(drafts[len(drafts)-1], draft)
			}
			drafts = append(drafts, draft)
			continue
		}
		count := int(math.Ceil(durationSec / preferredCinematicWindowSec))
		for durationSec/float64(count) > maxAIGCWindowDurationSec {
			count++
		}
		for count > 1 && durationSec/float64(count) < minAIGCWindowDurationSec {
			count--
		}
		partDuration := durationSec / float64(count)
		for i := 0; i < count; i++ {
			start := span.StartSec + float64(i)*partDuration
			end := start + partDuration
			draft := draftFromScriptSpan(span, start, end, partDuration)
			draft.SpanIDs = []string{fmt.Sprintf("%s_part_%02d", span.ID, i+1)}
			draft.VisualChangeReason = "duration_exceeds_15s_split"
			drafts = append(drafts, draft)
		}
		forcedSplits += count - 1
	}
	return drafts, forcedSplits
}

func draftFromScriptSpan(span model.ScriptSpan, startSec, endSec, durationSec float64) timeWindowDraft {
	return timeWindowDraft{
		SpanIDs:     []string{span.ID},
		Texts:       []string{span.Text},
		StartSec:    startSec,
		EndSec:      endSec,
		DurationSec: durationSec,
		Scene:       firstNonEmptyString(span.Scene, span.Visual),
		Subject:     span.Subject,
		Action:      span.Action,
		Camera:      span.Camera,
		ShotSize:    span.ShotSize,
		Framing:     span.Framing,
		Visual:      span.Visual,
		Emotion:     span.Emotion,
	}
}

func mergeShortCompatibleDrafts(drafts []timeWindowDraft) ([]timeWindowDraft, int) {
	out := make([]timeWindowDraft, 0, len(drafts))
	mergeCount := 0
	for i := 0; i < len(drafts); i++ {
		current := drafts[i]
		if current.DurationSec < minAIGCWindowDurationSec && len(out) > 0 {
			prev := out[len(out)-1]
			if prev.DurationSec+current.DurationSec <= maxAIGCWindowDurationSec && compatibleWindowDraft(prev, current) {
				out[len(out)-1] = mergeWindowDraft(prev, current, "merged_short_continuous_script_span")
				mergeCount++
				continue
			}
		}
		if current.DurationSec < minAIGCWindowDurationSec && i+1 < len(drafts) {
			next := drafts[i+1]
			if current.DurationSec+next.DurationSec <= maxAIGCWindowDurationSec && compatibleWindowDraft(current, next) {
				out = append(out, mergeWindowDraft(current, next, "merged_short_continuous_script_span"))
				mergeCount++
				i++
				continue
			}
		}
		out = append(out, current)
	}
	return out, mergeCount
}

func mergeWindowDraft(a, b timeWindowDraft, reason string) timeWindowDraft {
	return timeWindowDraft{
		SpanIDs:            append(append([]string{}, a.SpanIDs...), b.SpanIDs...),
		Texts:              append(append([]string{}, a.Texts...), b.Texts...),
		StartSec:           a.StartSec,
		EndSec:             b.EndSec,
		DurationSec:        a.DurationSec + b.DurationSec,
		Scene:              firstNonEmptyString(a.Scene, b.Scene),
		Subject:            firstNonEmptyString(a.Subject, b.Subject),
		Action:             firstNonEmptyString(a.Action, b.Action),
		Camera:             firstNonEmptyString(a.Camera, b.Camera),
		ShotSize:           firstNonEmptyString(a.ShotSize, b.ShotSize),
		Framing:            firstNonEmptyString(a.Framing, b.Framing),
		Visual:             strings.TrimSpace(strings.Join(nonEmptyStrings([]string{a.Visual, b.Visual}), " ")),
		Emotion:            firstNonEmptyString(a.Emotion, b.Emotion),
		VisualChangeReason: reason,
	}
}

func compatibleWindowDraft(a, b timeWindowDraft) bool {
	return compatibleSemanticField(a.Scene, b.Scene) &&
		compatibleSemanticField(a.Subject, b.Subject) &&
		compatibleSemanticField(a.Camera, b.Camera) &&
		compatibleSemanticField(a.ShotSize, b.ShotSize) &&
		compatibleSemanticField(a.Framing, b.Framing)
}

func compatibleSemanticField(a, b string) bool {
	a = strings.TrimSpace(strings.ToLower(a))
	b = strings.TrimSpace(strings.ToLower(b))
	return a == "" || b == "" || a == b
}

func visualChangeReason(prev, current timeWindowDraft) string {
	reasons := []string{}
	if !compatibleSemanticField(prev.Scene, current.Scene) {
		reasons = append(reasons, "scene_change")
	}
	if !compatibleSemanticField(prev.Subject, current.Subject) {
		reasons = append(reasons, "subject_change")
	}
	if !compatibleSemanticField(prev.Camera, current.Camera) || !compatibleSemanticField(prev.ShotSize, current.ShotSize) || !compatibleSemanticField(prev.Framing, current.Framing) {
		reasons = append(reasons, "camera_or_shot_size_change")
	}
	if strings.TrimSpace(prev.Action) != "" && strings.TrimSpace(current.Action) != "" && !strings.EqualFold(prev.Action, current.Action) {
		reasons = append(reasons, "action_goal_change")
	}
	if strings.TrimSpace(prev.Emotion) != "" && strings.TrimSpace(current.Emotion) != "" && !strings.EqualFold(prev.Emotion, current.Emotion) {
		reasons = append(reasons, "emotion_change")
	}
	if len(reasons) == 0 {
		return "script_semantic_boundary"
	}
	return strings.Join(uniqueStrings(reasons), "+")
}

func cinematicWindowVisualReason(durationSec float64, windowCount int) string {
	if windowCount > 1 || durationSec > maxAIGCWindowDurationSec {
		return "duration_exceeds_15s_split"
	}
	return "cinematic_shot_unit"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isAIGCWindowDuration(durationSec float64) bool {
	return durationSec >= minAIGCWindowDurationSec && durationSec <= maxAIGCWindowDurationSec
}
