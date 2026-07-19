package service

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestVideoCreationSpecDefaults(t *testing.T) {
	spec := model.NewVideoCreationSpec("vp-1", "做一个60秒口播视频")

	if spec.ProjectID != "vp-1" {
		t.Fatalf("ProjectID = %q, want vp-1", spec.ProjectID)
	}
	if spec.SourceMessage != "做一个60秒口播视频" {
		t.Fatalf("SourceMessage = %q", spec.SourceMessage)
	}
	if spec.ReviewMode != model.ReviewModeShotLevel {
		t.Fatalf("ReviewMode = %q, want %q", spec.ReviewMode, model.ReviewModeShotLevel)
	}
	if spec.AspectRatio != "16:9" || spec.Language != "zh-CN" {
		t.Fatalf("aspect/language = %q/%q", spec.AspectRatio, spec.Language)
	}
	if spec.ShotPolicy.MinDurationSec != 3 || spec.ShotPolicy.MaxDurationSec != 14 || spec.ShotPolicy.PreferDurationSec != 6 {
		t.Fatalf("shot policy defaults = %+v", spec.ShotPolicy)
	}
	if !spec.ShotPolicy.SingleSceneRequired || !spec.ShotPolicy.LowVisualChangeRequired || !spec.ShotPolicy.AvoidCrossShotDependency {
		t.Fatalf("shot policy booleans = %+v", spec.ShotPolicy)
	}
	if spec.RenderPreference.DefaultRenderStrategy != model.RenderStrategyAuto {
		t.Fatalf("default strategy = %q", spec.RenderPreference.DefaultRenderStrategy)
	}
	if !spec.RenderPreference.PreferHTMLForText || !spec.RenderPreference.PreferHTMLForCharts || !spec.RenderPreference.PreferHTMLForUI {
		t.Fatalf("HTML render preference defaults = %+v", spec.RenderPreference)
	}
	if !spec.RenderPreference.PreferAIGCForPeople || !spec.RenderPreference.PreferAIGCForScene || !spec.RenderPreference.AllowHybridRender || !spec.RenderPreference.PreferLowCostPreview {
		t.Fatalf("AIGC render preference defaults = %+v", spec.RenderPreference)
	}
}

func TestShotDurationCheckerRejectsOutside3To14(t *testing.T) {
	for _, tc := range []struct {
		name string
		sec  int
	}{
		{name: "under", sec: 2},
		{name: "over", sec: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := CheckShotDuration(model.ShotUnit{ID: "shot-1", DurationSec: tc.sec})
			if len(issues) == 0 {
				t.Fatalf("expected duration issue for %d seconds", tc.sec)
			}
			if issues[0].Code != "shot_duration_out_of_range" {
				t.Fatalf("issue code = %q", issues[0].Code)
			}
		})
	}
}

func TestShotDurationCheckerAccepts3To14(t *testing.T) {
	for _, sec := range []int{3, 6, 14} {
		issues := CheckShotDuration(model.ShotUnit{ID: "shot-1", DurationSec: sec})
		if len(issues) != 0 {
			t.Fatalf("duration %d issues = %+v", sec, issues)
		}
	}
}

func TestShotSceneComplexityCheckerRejectsMultiSceneAndHighChange(t *testing.T) {
	issues := CheckShotSceneComplexity(model.ShotUnit{
		ID:                "shot-1",
		DurationSec:       6,
		SingleScene:       false,
		VisualChangeLevel: model.VisualChangeHigh,
		MainAction:        "主角从办公室跑到街头再进入会议室",
	})

	if len(issues) < 2 {
		t.Fatalf("issues = %+v, want at least multi-scene and high-change issues", issues)
	}
	if !hasIssueCode(issues, "shot_multi_scene") {
		t.Fatalf("missing shot_multi_scene in %+v", issues)
	}
	if !hasIssueCode(issues, "shot_visual_change_high") {
		t.Fatalf("missing shot_visual_change_high in %+v", issues)
	}
}

func TestRenderStrategyTextOnlyUsesHTMLOnly(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, ScreenText: []string{"几个表格"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", MustBeExact: true, Role: model.TextRoleKeyword}},
	}

	strategy := DecideRenderStrategy(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	if strategy.Mode != model.RenderModeHTMLOnly {
		t.Fatalf("mode = %q, want %q", strategy.Mode, model.RenderModeHTMLOnly)
	}
	if !strategy.HTMLRequired || strategy.AIGCRequired || !strategy.TextOverlayNeeded || strategy.NeedsCompositing {
		t.Fatalf("strategy flags = %+v", strategy)
	}
}

func TestRenderStrategyAIGCSceneNoTextUsesAIGCOnly(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "非真人风格化办公室空间", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{ID: "host", Motion: "turns from calm to hesitant", Emotion: "hesitant"}},
	}

	strategy := DecideRenderStrategy(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	if strategy.Mode != model.RenderModeAIGCOnly {
		t.Fatalf("mode = %q, want %q", strategy.Mode, model.RenderModeAIGCOnly)
	}
	if strategy.HTMLRequired || !strategy.AIGCRequired || strategy.TextOverlayNeeded || strategy.NeedsCompositing {
		t.Fatalf("strategy flags = %+v", strategy)
	}
}

func TestRenderStrategyExactTextUsesHybridWithAIGCScene(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, VideoType: "口播视频", ScreenText: []string{"几个表格"}}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "非真人风格化办公室空间", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{ID: "host", Motion: "walks through office"}},
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", MustBeExact: true, Role: model.TextRoleKeyword}},
	}

	strategy := DecideRenderStrategy(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	if strategy.Mode != model.RenderModeHTMLPreviewThenHybrid && strategy.Mode != model.RenderModeHybridAIGCBGHTMLOverlay {
		t.Fatalf("mode = %q, want hybrid or preview-then-hybrid", strategy.Mode)
	}
	if !strategy.HTMLRequired || !strategy.AIGCRequired || !strategy.TextOverlayNeeded || !strategy.NeedsCompositing {
		t.Fatalf("strategy flags = %+v", strategy)
	}
}

func TestTextLayerExactnessRequiresScreenTextInTextLayers(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, ScreenText: []string{"几个表格", "几份文档"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", MustBeExact: true, StartSec: 0, EndSec: 3}},
	}
	strategy := model.RenderStrategy{Mode: model.RenderModeHTMLOnly, HTMLRequired: true, TextOverlayNeeded: true}

	issues := CheckTextLayerExactness(shot, plan, strategy)
	if !hasIssueCode(issues, "screen_text_missing_text_layer") {
		t.Fatalf("issues = %+v, want screen_text_missing_text_layer", issues)
	}
}

func TestRenderStrategyCheckerRejectsExactTextAIGCOnly(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, ScreenText: []string{"几个表格"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", MustBeExact: true}},
	}
	strategy := model.RenderStrategy{Mode: model.RenderModeAIGCOnly, AIGCRequired: true}

	issues := CheckRenderStrategy(shot, plan, strategy)
	if !hasIssueCode(issues, "exact_text_requires_html") {
		t.Fatalf("issues = %+v, want exact_text_requires_html", issues)
	}
}

func TestAIGCPromptNoTextWhenHybrid(t *testing.T) {
	strategy := model.RenderStrategy{Mode: model.RenderModeHybridAIGCBGHTMLOverlay, AIGCRequired: true, HTMLRequired: true}
	issues := CheckAIGCPromptNoText(strategy, "办公室中人物被文件包围，画面出现几个表格")
	if !hasIssueCode(issues, "hybrid_aigc_prompt_must_ban_text") {
		t.Fatalf("issues = %+v, want hybrid_aigc_prompt_must_ban_text", issues)
	}

	okPrompt := strings.Join([]string{
		"非真人风格化办公室中人物被文件包围",
		"禁止生成任何可读文字",
		"禁止生成中文字符",
		"禁止生成英文单词",
		"禁止生成 UI 文字",
		"禁止生成字幕、标签、水印",
	}, "。")
	if issues := CheckAIGCPromptNoText(strategy, okPrompt); len(issues) != 0 {
		t.Fatalf("ok prompt issues = %+v", issues)
	}
}

func TestFinalAssemblyUsesOnlyApprovedNonStaleShots(t *testing.T) {
	shots := []model.ShotUnit{
		{ID: "shot-approved", ReviewStatus: model.ReviewStatusApproved, ArtifactRefs: model.ShotArtifactRefs{CompositedShotVideoArtifactID: "art-ok"}},
		{ID: "shot-pending", ReviewStatus: model.ReviewStatusPending, ArtifactRefs: model.ShotArtifactRefs{CompositedShotVideoArtifactID: "art-pending"}},
		{ID: "shot-stale", ReviewStatus: model.ReviewStatusApproved, Stale: true, ArtifactRefs: model.ShotArtifactRefs{CompositedShotVideoArtifactID: "art-stale"}},
	}

	issues := CheckFinalAssembly(shots)
	if !hasIssueCode(issues, "final_assembly_requires_approved_shot") {
		t.Fatalf("issues = %+v, want approval issue", issues)
	}
	if !hasIssueCode(issues, "final_assembly_rejects_stale_shot") {
		t.Fatalf("issues = %+v, want stale issue", issues)
	}
}

func TestOralVideoPrefersHTMLPreview(t *testing.T) {
	shot := model.ShotUnit{ID: "shot-1", DurationSec: 6, VideoType: "口播视频", ScreenText: []string{"十几条聊天记录"}}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "非真人风格化办公室空间", RequiresAIGC: true},
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "十几条聊天记录", MustBeExact: true, Role: model.TextRoleKeyword}},
	}

	strategy := DecideRenderStrategy(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	if strategy.Mode != model.RenderModeHTMLPreviewThenHybrid {
		t.Fatalf("mode = %q, want %q", strategy.Mode, model.RenderModeHTMLPreviewThenHybrid)
	}
}

func hasIssueCode(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
