package service

import (
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

	if len(plan.Windows) != 4 {
		t.Fatalf("window count = %d, want 4: %#v", len(plan.Windows), plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.DurationSec < 3 || window.DurationSec > 15 {
			t.Fatalf("window duration outside 3-15s: %#v", window)
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
