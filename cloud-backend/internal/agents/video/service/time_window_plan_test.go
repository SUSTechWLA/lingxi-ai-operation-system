package service

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildTimeWindowPlanSplitsCinematicLongShot(t *testing.T) {
	profile := model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory}
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: profile,
		Shots: []model.ShotUnit{
			{ID: "SHOT_01", DurationSec: 40, SceneSummary: "夜晚街道追逐", MainAction: "角色穿过街道并躲入巷子"},
		},
	})

	if len(plan.Windows) != 6 {
		t.Fatalf("window count = %d, want 6 preferred 6-8s windows: %#v", len(plan.Windows), plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.DurationSec < 3 || window.DurationSec > 15 {
			t.Fatalf("window duration outside 3-15s: %#v", window)
		}
		if window.DurationSec < 6 || window.DurationSec > 8 {
			t.Fatalf("cinematic split should prefer 6-8s windows: %#v", window)
		}
		if window.ParentShotID != "SHOT_01" {
			t.Fatalf("parent shot mismatch: %#v", window)
		}
		if !window.AIGCEligible {
			t.Fatalf("cinematic windows should be AIGC eligible: %#v", window)
		}
	}
}

func TestBuildTimeWindowPlanKeepsTalkingHeadScriptTiming(t *testing.T) {
	profile := model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead}
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: profile,
		ScriptSpans: []model.ScriptSpan{
			{ID: "seg-1", StartSec: 0, EndSec: 6, Text: "第一句口播。"},
			{ID: "seg-2", StartSec: 6, EndSec: 14, Text: "第二句解释。"},
		},
	})

	if len(plan.Windows) != 2 {
		t.Fatalf("window count = %d, want 2", len(plan.Windows))
	}
	if plan.Windows[0].ScriptText != "第一句口播。" || plan.Windows[1].ScriptText != "第二句解释。" {
		t.Fatalf("script text should be preserved: %#v", plan.Windows)
	}
	if plan.Windows[0].StartSec != 0 || plan.Windows[1].StartSec != 6 {
		t.Fatalf("script timing should be preserved: %#v", plan.Windows)
	}
}

func TestBuildTimeWindowPlanDefaultsUnknownProfileInvalidScriptSpan(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: "unknown"},
		ScriptSpans: []model.ScriptSpan{
			{ID: "seg-invalid", StartSec: 10, EndSec: 8},
		},
	})

	if plan.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile ID = %q, want %q", plan.ProfileID, model.VideoProfileTalkingHead)
	}
	if len(plan.Windows) != 1 {
		t.Fatalf("window count = %d, want 1: %#v", len(plan.Windows), plan.Windows)
	}
	window := plan.Windows[0]
	if window.ID != "TW_01" {
		t.Fatalf("window ID = %q, want TW_01", window.ID)
	}
	if window.ShotID != "SHOT_01" {
		t.Fatalf("shot ID = %q, want SHOT_01", window.ShotID)
	}
	if window.ScriptSpanID != "seg-invalid" {
		t.Fatalf("script span ID = %q, want seg-invalid", window.ScriptSpanID)
	}
	if window.StartSec != 10 || window.EndSec != 16 || window.DurationSec != 6 {
		t.Fatalf("invalid span should default to 6s from start: %#v", window)
	}
}

func TestBuildTimeWindowPlanDefaultsCinematicZeroDurationShot(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
		Shots: []model.ShotUnit{
			{ID: "SHOT_ZERO", DurationSec: 0},
		},
	})

	if len(plan.Windows) != 1 {
		t.Fatalf("window count = %d, want 1: %#v", len(plan.Windows), plan.Windows)
	}
	window := plan.Windows[0]
	if window.DurationSec != 6 || window.EndSec != 6 {
		t.Fatalf("zero-duration cinematic shot should default to 6s: %#v", window)
	}
	if !window.AIGCEligible {
		t.Fatalf("default cinematic window should be AIGC eligible: %#v", window)
	}
	if window.Reason != "cinematic coarse shot split into AIGC-safe 3-15s window" {
		t.Fatalf("reason = %q", window.Reason)
	}
}

func TestBuildTimeWindowPlanUsesParentRelativeCinematicTiming(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
		Shots: []model.ShotUnit{
			{ID: "SHOT_A", DurationSec: 6},
			{ID: "SHOT_B", DurationSec: 6},
		},
	})

	if len(plan.Windows) != 2 {
		t.Fatalf("window count = %d, want 2: %#v", len(plan.Windows), plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.StartSec != 0 || window.EndSec != 6 {
			t.Fatalf("cinematic timing should be parent-shot-relative: %#v", plan.Windows)
		}
	}
}

func TestBuildTimeWindowPlanKeepsSubThreeSecondCinematicAsTransitionOverlay(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
		Shots: []model.ShotUnit{
			{ID: "SHOT_SHORT", DurationSec: 2},
			{ID: "SHOT_NEXT", DurationSec: 6},
		},
	})

	if len(plan.Windows) != 2 {
		t.Fatalf("window count = %d, want 2: %#v", len(plan.Windows), plan.Windows)
	}
	shortWindow := plan.Windows[0]
	if shortWindow.DurationSec != 2 || shortWindow.EndSec != 2 {
		t.Fatalf("sub-3s cinematic duration should stay authored: %#v", shortWindow)
	}
	if shortWindow.AIGCEligible {
		t.Fatalf("sub-3s cinematic window should not be AIGC eligible: %#v", shortWindow)
	}
	if shortWindow.RecommendedMode != model.GenerationModeHTMLOnly {
		t.Fatalf("mode = %q, want %q", shortWindow.RecommendedMode, model.GenerationModeHTMLOnly)
	}
	if !strings.Contains(shortWindow.Reason, "shorter than AIGC minimum") ||
		!strings.Contains(shortWindow.Reason, "transition/overlay") {
		t.Fatalf("reason should explain transition/overlay handling: %q", shortWindow.Reason)
	}
	if plan.Windows[1].StartSec != 0 || plan.Windows[1].EndSec != 6 {
		t.Fatalf("short prior shot should not shift later parent-relative timing: %#v", plan.Windows[1])
	}
}

func TestBuildTimeWindowPlanUsesUniqueEffectiveCinematicShotIDs(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
		Shots: []model.ShotUnit{
			{ID: "", DurationSec: 6},
			{ID: "SHOT_01", DurationSec: 6},
			{ID: "SHOT_01", DurationSec: 6},
		},
	})

	if len(plan.Windows) != 3 {
		t.Fatalf("window count = %d, want 3: %#v", len(plan.Windows), plan.Windows)
	}
	parentIDs := map[string]bool{}
	windowIDs := map[string]bool{}
	for _, window := range plan.Windows {
		if window.ParentShotID == "" {
			t.Fatalf("parent shot ID should not be empty: %#v", window)
		}
		if parentIDs[window.ParentShotID] {
			t.Fatalf("parent shot ID should be unique: %#v", plan.Windows)
		}
		parentIDs[window.ParentShotID] = true
		if window.ID == "" {
			t.Fatalf("window ID should not be empty: %#v", window)
		}
		if windowIDs[window.ID] {
			t.Fatalf("window ID should be unique: %#v", plan.Windows)
		}
		windowIDs[window.ID] = true
	}
}

func TestBuildTimeWindowPlanAvoidsEffectiveCinematicIDCollisionWithAuthoredDuplicateSuffix(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
		Shots: []model.ShotUnit{
			{ID: "", DurationSec: 6},
			{ID: "SHOT_01", DurationSec: 6},
			{ID: "SHOT_01_DUP_02", DurationSec: 6},
		},
	})

	if len(plan.Windows) != 3 {
		t.Fatalf("window count = %d, want 3: %#v", len(plan.Windows), plan.Windows)
	}
	parentIDs := map[string]bool{}
	windowIDs := map[string]bool{}
	for _, window := range plan.Windows {
		if window.ParentShotID == "" {
			t.Fatalf("parent shot ID should not be empty: %#v", window)
		}
		if parentIDs[window.ParentShotID] {
			t.Fatalf("parent shot ID should be globally unique: %#v", plan.Windows)
		}
		parentIDs[window.ParentShotID] = true
		if window.ID == "" {
			t.Fatalf("window ID should not be empty: %#v", window)
		}
		if windowIDs[window.ID] {
			t.Fatalf("window ID should be globally unique: %#v", plan.Windows)
		}
		windowIDs[window.ID] = true
	}
}

func TestBuildTimeWindowPlanCinematicNoShotsUsesEmptyWindowsSlice(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory},
	})

	if plan.Windows == nil {
		t.Fatal("windows should be an initialized empty slice, got nil")
	}
	if len(plan.Windows) != 0 {
		t.Fatalf("window count = %d, want 0: %#v", len(plan.Windows), plan.Windows)
	}
}

func TestBuildTimeWindowPlanTalkingHeadEligibilityBoundaries(t *testing.T) {
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead},
		ScriptSpans: []model.ScriptSpan{
			{ID: "short", StartSec: 0, EndSec: 2},
			{ID: "max", StartSec: 2, EndSec: 17},
			{ID: "long", StartSec: 17, EndSec: 33},
		},
	})

	if len(plan.Windows) != 5 {
		t.Fatalf("window count = %d, want 5 after splitting >15s span: %#v", len(plan.Windows), plan.Windows)
	}
	if plan.Windows[0].DurationSec != 2 || plan.Windows[0].AIGCEligible {
		t.Fatalf("short talking-head span should preserve duration and be ineligible: %#v", plan.Windows[0])
	}
	if !plan.Windows[1].AIGCEligible {
		t.Fatalf("15s talking-head span should be eligible: %#v", plan.Windows[1])
	}
	for _, window := range plan.Windows[2:] {
		if window.DurationSec < 3 || window.DurationSec > 15 || !window.AIGCEligible {
			t.Fatalf(">15s talking-head span should split into eligible windows: %#v", window)
		}
	}
	for _, window := range plan.Windows {
		if window.RecommendedMode != model.GenerationModeHTMLOnly {
			t.Fatalf("talking-head windows should stay HTML-only: %#v", window)
		}
	}
}
