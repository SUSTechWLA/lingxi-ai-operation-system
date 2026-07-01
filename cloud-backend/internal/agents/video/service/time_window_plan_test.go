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
