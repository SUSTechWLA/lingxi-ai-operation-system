package service

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildShotGenerationPlanExactTextUsesHTMLOnly(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_01", DurationSec: 6, ScreenText: []string{"增长 42%"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeHTMLOnly {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeHTMLOnly)
	}
	if generationPlan.PrimaryTool != "hyperframes_renderer" {
		t.Fatalf("primary tool = %q", generationPlan.PrimaryTool)
	}
	if len(generationPlan.RequiredAssets) == 0 || generationPlan.RequiredAssets[0].Source != model.AssetSourceHyperFrames {
		t.Fatalf("required assets = %+v, want first source %q", generationPlan.RequiredAssets, model.AssetSourceHyperFrames)
	}
	if generationPlan.FusionPlan.Assembler != "hyperframes" {
		t.Fatalf("assembler = %q, want hyperframes", generationPlan.FusionPlan.Assembler)
	}
}

func TestBuildShotGenerationPlanMotionSceneUsesAIGCVideo(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_02", DurationSec: 8, MainAction: "角色穿过办公室并回头"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室自然光场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:      "host",
			Motion:  "walks through office and turns back",
			Emotion: "curious",
		}},
		CameraPlan: model.CameraPlan{
			Movement:     "tracking shot",
			RequiresAIGC: true,
		},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeAIGCVideo {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeAIGCVideo)
	}
	if generationPlan.PrimaryTool != "text_image_to_video_generator" {
		t.Fatalf("primary tool = %q", generationPlan.PrimaryTool)
	}
}

func TestBuildShotGenerationPlanExactTextWithAIGCSceneUsesHybrid(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_03", DurationSec: 7, ScreenText: []string{"几个表格"}}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室数据分析场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:     "host",
			Motion: "points to the screen",
		}},
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "几个表格",
			Role:        model.TextRoleKeyword,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeHybridAIGCBGHTMLOverlay {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeHybridAIGCBGHTMLOverlay)
	}
	if !containsGenerationPlanString(generationPlan.SecondaryTools, "hyperframes_renderer") {
		t.Fatalf("secondary tools = %+v, want hyperframes_renderer", generationPlan.SecondaryTools)
	}
	if generationPlan.FusionPlan.BaseLayer.Kind != "video" {
		t.Fatalf("base layer = %+v, want video", generationPlan.FusionPlan.BaseLayer)
	}
	if len(generationPlan.FusionPlan.OverlayLayers) == 0 {
		t.Fatalf("overlay layers = %+v, want non-empty", generationPlan.FusionPlan.OverlayLayers)
	}
}

func TestBuildShotGenerationPlanLogoRoutesToUserAsset(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_04", DurationSec: 5, SceneSummary: "展示客户上传 logo 和产品截图"}
	plan := model.VisualPlan{
		Props: []model.PropVisualSpec{{
			ID:          "brand-logo",
			Description: "客户上传 logo",
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeExternalOrUserAsset {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeExternalOrUserAsset)
	}
	if len(generationPlan.RequiredAssets) == 0 || generationPlan.RequiredAssets[0].Source != model.AssetSourceUserUpload {
		t.Fatalf("required assets = %+v, want first source %q", generationPlan.RequiredAssets, model.AssetSourceUserUpload)
	}
}

func TestBuildShotGenerationPlanMissingVideoProviderCreatesExternalNeed(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_05", DurationSec: 8, MainAction: "角色穿过办公室并回头"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室自然光场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:     "host",
			Motion: "walks through office and turns back",
		}},
		CameraPlan: model.CameraPlan{
			Movement:     "tracking shot",
			RequiresAIGC: true,
		},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: false, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
	if len(generationPlan.RequiredAssets) == 0 || generationPlan.RequiredAssets[0].Source != model.AssetSourceExternalGeneration {
		t.Fatalf("required assets = %+v, want first source %q", generationPlan.RequiredAssets, model.AssetSourceExternalGeneration)
	}
	if generationPlan.FallbackPlan == nil || generationPlan.FallbackPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("fallback plan = %+v, want placeholder preview", generationPlan.FallbackPlan)
	}
}

func TestBuildShotGenerationPlanExactTextAIGCWithoutHybridUsesPlaceholder(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_06", DurationSec: 7, ScreenText: []string{"增长 42%"}, MainAction: "角色穿过办公室"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室数据场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:     "host",
			Motion: "walks through office",
		}},
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}
	pref := model.DefaultRenderPreference()
	pref.AllowHybridRender = false

	generationPlan := BuildShotGenerationPlan(shot, plan, pref, RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode == model.GenerationModeAIGCVideo {
		t.Fatalf("mode = %q, exact text with AIGC must not use pure AIGC video", generationPlan.Mode)
	}
	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
}

func TestBuildShotGenerationPlanExactTextAIGCWithoutHTMLUsesPlaceholder(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_07", DurationSec: 7, ScreenText: []string{"增长 42%"}, MainAction: "角色穿过办公室"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室数据场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:     "host",
			Motion: "walks through office",
		}},
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: false})

	if generationPlan.Mode == model.GenerationModeAIGCVideo {
		t.Fatalf("mode = %q, exact text with unavailable HTML must not use pure AIGC video", generationPlan.Mode)
	}
	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
}

func TestBuildShotGenerationPlanHTMLOnlyWithoutHTMLUsesPlaceholder(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_08", DurationSec: 6, ScreenText: []string{"增长 42%"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: false})

	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
}

func TestBuildShotGenerationPlanDefaultPreviewWithoutHTMLUsesPlaceholder(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_08B", DurationSec: 6}

	generationPlan := BuildShotGenerationPlan(shot, model.VisualPlan{}, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: false})

	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
	if generationPlan.PrimaryTool == "hyperframes_renderer" {
		t.Fatalf("primary tool = %q, HyperFrames is unavailable", generationPlan.PrimaryTool)
	}
}

func TestBuildShotGenerationPlanUsesCanvasDurationWhenShotDurationMissing(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_09", ScreenText: []string{"增长 42%"}}
	plan := model.VisualPlan{
		Canvas: model.CanvasSpec{DurationSec: 9},
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.FusionPlan.BaseLayer.DurationSec != 9 {
		t.Fatalf("base duration = %v, want 9", generationPlan.FusionPlan.BaseLayer.DurationSec)
	}
	if generationPlan.RenderInputs["durationSec"] != 9 {
		t.Fatalf("render duration = %#v, want 9", generationPlan.RenderInputs["durationSec"])
	}
}

func TestBuildShotGenerationPlanHybridPromptExcludesExactText(t *testing.T) {
	shot := model.ShotUnit{
		ID:           "SHOT_10",
		DurationSec:  7,
		SceneSummary: "办公室大屏展示增长 42%",
		ScreenText:   []string{"增长 42%"},
	}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室背景墙出现增长 42%", RequiresAIGC: true},
		TextLayers: []model.TextLayerSpec{{
			ID:          "txt-1",
			Text:        "增长 42%",
			Role:        model.TextRoleDataText,
			MustBeExact: true,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
	prompt, ok := generationPlan.RenderInputs["prompt"].(string)
	if !ok {
		t.Fatalf("prompt = %#v, want string", generationPlan.RenderInputs["prompt"])
	}
	if strings.Contains(prompt, "增长 42%") {
		t.Fatalf("prompt %q contains exact text that should stay in HTML overlay", prompt)
	}
	if !strings.Contains(prompt, "禁止生成任何可读文字") {
		t.Fatalf("prompt %q missing no-readable-text guidance", prompt)
	}
}

func TestBuildShotGenerationPlanAIGCImageHyperFramesAddsOverlayLayer(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_11", DurationSec: 6}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "静态未来办公室背景", RequiresAIGC: true},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeAIGCImageThenHyperFrames {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeAIGCImageThenHyperFrames)
	}
	if len(generationPlan.FusionPlan.OverlayLayers) == 0 {
		t.Fatalf("overlay layers = %+v, want HyperFrames overlay", generationPlan.FusionPlan.OverlayLayers)
	}
	if generationPlan.FusionPlan.OverlayLayers[0].Kind != "html_overlay" {
		t.Fatalf("overlay layer = %+v, want html_overlay", generationPlan.FusionPlan.OverlayLayers[0])
	}
}

func TestBuildShotGenerationPlanCustomerMentionAloneDoesNotRouteToUserAsset(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_12", DurationSec: 5, SceneSummary: "客户在会议中讨论年度预算"}

	generationPlan := BuildShotGenerationPlan(shot, model.VisualPlan{}, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode == model.GenerationModeExternalOrUserAsset {
		t.Fatalf("mode = %q, bare customer mention must not route to user asset", generationPlan.Mode)
	}
}

func containsGenerationPlanString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
