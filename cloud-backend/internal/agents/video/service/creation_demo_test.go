package service

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestFakeDemoOneSentenceOralVideoExactTextFlow(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = &model.VideoProject{ID: "vp-1", UserID: "u-1", AspectRatio: "16:9", Language: "zh-CN", TargetDuration: 60}
	svc := NewCreationService(store)

	spec, err := svc.GenerateSpec(context.Background(), "u-1", "vp-1", GenerateSpecRequest{
		SourceMessage: "做一个60秒口播视频，主题是：AI替代的不是岗位，而是整套工作流程。画面中需要出现“几个表格”“几份文档”“十几条聊天记录”。",
	})
	if err != nil {
		t.Fatalf("GenerateSpec error: %v", err)
	}
	if spec.ReviewMode != model.ReviewModeShotLevel {
		t.Fatalf("review mode = %q", spec.ReviewMode)
	}

	shots, err := svc.GenerateShots(context.Background(), "u-1", "vp-1")
	if err != nil {
		t.Fatalf("GenerateShots error: %v", err)
	}
	if len(shots) < 8 || len(shots) > 12 {
		t.Fatalf("shot count = %d, want 8..12", len(shots))
	}
	for _, shot := range shots {
		if shot.DurationSec < 3 || shot.DurationSec > 15 {
			t.Fatalf("shot %s duration = %d", shot.ID, shot.DurationSec)
		}
	}

	textShot := firstShotWithScreenText(t, shots, "几个表格")
	plan, err := svc.GenerateVisualPlan(context.Background(), "u-1", "vp-1", textShot.ID)
	if err != nil {
		t.Fatalf("GenerateVisualPlan error: %v", err)
	}
	strategy, err := svc.DecideShotRenderStrategy(context.Background(), "u-1", "vp-1", textShot.ID)
	if err != nil {
		t.Fatalf("DecideShotRenderStrategy error: %v", err)
	}
	if strategy.Mode == model.RenderModeAIGCOnly {
		t.Fatalf("exact text shot used aigc_only: %+v plan=%+v", strategy, plan)
	}
	for _, text := range []string{"几个表格", "几份文档", "十几条聊天记录"} {
		if !textLayerMustBeExact(plan.TextLayers, text) {
			t.Fatalf("text layer %q missing or not exact in %+v", text, plan.TextLayers)
		}
	}
}

func firstShotWithScreenText(t *testing.T, shots []model.ShotUnit, text string) model.ShotUnit {
	t.Helper()
	for _, shot := range shots {
		if containsString(shot.ScreenText, text) {
			return shot
		}
	}
	t.Fatalf("no shot contains screen text %q: %+v", text, shots)
	return model.ShotUnit{}
}
