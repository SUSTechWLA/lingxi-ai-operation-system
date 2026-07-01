package service

import (
	"encoding/json"
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
	if generationPlan.FusionPlan.OutputArtifactKind != "HYPERFRAMES_SHOT" {
		t.Fatalf("output artifact kind = %q, want HYPERFRAMES_SHOT", generationPlan.FusionPlan.OutputArtifactKind)
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
	if generationPlan.FusionPlan.OutputArtifactKind != "SHOT_VIDEO_CLIP" {
		t.Fatalf("output artifact kind = %q, want SHOT_VIDEO_CLIP", generationPlan.FusionPlan.OutputArtifactKind)
	}
}

func TestBuildShotGenerationPlanAIGCDurationClampsToProviderWindow(t *testing.T) {
	tests := []struct {
		name        string
		durationSec int
		want        int
	}{
		{name: "too_long", durationSec: 40, want: 15},
		{name: "too_short", durationSec: 1, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shot := model.ShotUnit{
				ID:          "SHOT_" + tt.name,
				DurationSec: tt.durationSec,
				MainAction:  "角色在雨夜街口回头并向前走",
			}
			plan := model.VisualPlan{
				Background: model.BackgroundSpec{Description: "雨夜街口电影感场景", RequiresAIGC: true},
				Characters: []model.CharacterVisualSpec{{
					ID:           "lead",
					Description:  "主角站在雨夜街口",
					Motion:       "walks forward and turns back",
					RequiresAIGC: true,
				}},
				CameraPlan: model.CameraPlan{
					Description:  "镜头缓慢推进",
					Movement:     "tracking shot",
					RequiresAIGC: true,
				},
			}

			generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

			if generationPlan.Mode != model.GenerationModeAIGCVideo {
				t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeAIGCVideo)
			}
			if generationPlan.RenderInputs["durationSec"] != tt.want {
				t.Fatalf("render duration = %#v, want %d", generationPlan.RenderInputs["durationSec"], tt.want)
			}
			if generationPlan.FusionPlan.BaseLayer.DurationSec != float64(tt.want) {
				t.Fatalf("base layer duration = %v, want %d", generationPlan.FusionPlan.BaseLayer.DurationSec, tt.want)
			}
		})
	}
}

func TestBuildShotGenerationPlanExternalAIGCNeedClampsDurationToProviderWindow(t *testing.T) {
	tests := []struct {
		name        string
		durationSec int
		want        int
	}{
		{name: "too_long", durationSec: 40, want: 15},
		{name: "too_short", durationSec: 1, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shot := model.ShotUnit{
				ID:          "SHOT_EXTERNAL_" + tt.name,
				DurationSec: tt.durationSec,
				MainAction:  "角色在雨夜街口回头并向前走",
			}
			plan := model.VisualPlan{
				Background: model.BackgroundSpec{Description: "雨夜街口电影感场景", RequiresAIGC: true},
				Characters: []model.CharacterVisualSpec{{
					ID:           "lead",
					Description:  "主角站在雨夜街口",
					Motion:       "walks forward and turns back",
					RequiresAIGC: true,
				}},
				CameraPlan: model.CameraPlan{
					Description:  "镜头缓慢推进",
					Movement:     "tracking shot",
					RequiresAIGC: true,
				},
			}

			generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: false, HTMLAvailable: true})

			if generationPlan.Mode != model.GenerationModePlaceholderPreview {
				t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
			}
			if generationPlan.RenderInputs["durationSec"] != tt.want {
				t.Fatalf("render duration = %#v, want %d", generationPlan.RenderInputs["durationSec"], tt.want)
			}
			if generationPlan.FusionPlan.BaseLayer.DurationSec != float64(tt.want) {
				t.Fatalf("base layer duration = %v, want %d", generationPlan.FusionPlan.BaseLayer.DurationSec, tt.want)
			}
		})
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
	if generationPlan.FusionPlan.OutputArtifactKind != "COMPOSITED_SHOT_VIDEO" {
		t.Fatalf("output artifact kind = %q, want COMPOSITED_SHOT_VIDEO", generationPlan.FusionPlan.OutputArtifactKind)
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
	if generationPlan.FusionPlan.OutputArtifactKind != "SHOT_MEDIA_FUSION_PLAN" {
		t.Fatalf("output artifact kind = %q, want SHOT_MEDIA_FUSION_PLAN", generationPlan.FusionPlan.OutputArtifactKind)
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
	if generationPlan.FusionPlan.OutputArtifactKind != "SHOT_MEDIA_FUSION_PLAN" {
		t.Fatalf("output artifact kind = %q, want SHOT_MEDIA_FUSION_PLAN", generationPlan.FusionPlan.OutputArtifactKind)
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

func TestBuildShotGenerationPlanScreenTextStaysExactWhenHTMLPreferenceDisabled(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_06B", DurationSec: 7, ScreenText: []string{"必须准确"}}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "办公室运动场景", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{
			ID:     "host",
			Motion: "walks through office",
		}},
		CameraPlan: model.CameraPlan{Movement: "tracking shot"},
	}
	pref := model.DefaultRenderPreference()
	pref.PreferHTMLForText = false
	pref.AllowHybridRender = false

	generationPlan := BuildShotGenerationPlan(shot, plan, pref, RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode == model.GenerationModeAIGCVideo {
		t.Fatalf("mode = %q, screen text must not be sent to pure AIGC when HTML preference is disabled", generationPlan.Mode)
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

func TestBuildShotGenerationPlanExactTextRoleCreatesTextLock(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_08A", DurationSec: 6}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{
			ID:   "txt-1",
			Text: "  增长 42%  ",
			Role: model.TextRoleDataText,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if len(generationPlan.RequiredAssets) == 0 {
		t.Fatalf("required assets = %+v, want HyperFrames asset", generationPlan.RequiredAssets)
	}
	if !assetLocksContain(generationPlan.RequiredAssets, model.AssetSourceHyperFrames, "text:增长 42%") {
		t.Fatalf("required assets = %+v, want trimmed data_text lock", generationPlan.RequiredAssets)
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
	if generationPlan.FusionPlan.OutputArtifactKind != "HYPERFRAMES_SHOT" {
		t.Fatalf("output artifact kind = %q, want HYPERFRAMES_SHOT", generationPlan.FusionPlan.OutputArtifactKind)
	}
}

func TestBuildShotGenerationPlanStaticAIGCWithoutHTMLDoesNotDeclareHyperFrames(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_11B", DurationSec: 6}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "静态未来办公室背景", RequiresAIGC: true},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: false})

	if generationPlan.Mode == model.GenerationModeAIGCImageThenHyperFrames {
		t.Fatalf("mode = %q, image+HyperFrames must not be declared when HTML is unavailable", generationPlan.Mode)
	}
	if generationPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModePlaceholderPreview)
	}
}

func TestBuildShotGenerationPlanUserAssetWithTextAddsHyperFramesOverlay(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_11C", DurationSec: 6, SceneSummary: "展示客户上传 logo", ScreenText: []string{"必须准确"}}
	plan := model.VisualPlan{
		Props: []model.PropVisualSpec{{
			ID:          "brand-logo",
			Description: "客户上传 logo",
		}},
		TextLayers: []model.TextLayerSpec{{
			ID:   "txt-1",
			Text: "必须准确",
			Role: model.TextRoleKeyword,
		}},
	}

	generationPlan := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode != model.GenerationModeExternalOrUserAsset {
		t.Fatalf("mode = %q, want %q", generationPlan.Mode, model.GenerationModeExternalOrUserAsset)
	}
	if !assetSourceExists(generationPlan.RequiredAssets, model.AssetSourceUserUpload) {
		t.Fatalf("required assets = %+v, want user upload", generationPlan.RequiredAssets)
	}
	if !assetSourceExists(generationPlan.RequiredAssets, model.AssetSourceHyperFrames) {
		t.Fatalf("required assets = %+v, want HyperFrames overlay asset", generationPlan.RequiredAssets)
	}
	if len(generationPlan.FusionPlan.OverlayLayers) == 0 {
		t.Fatalf("overlay layers = %+v, want text overlay", generationPlan.FusionPlan.OverlayLayers)
	}
	layers, ok := generationPlan.RenderInputs["textLayers"].([]model.TextLayerSpec)
	if !ok || len(layers) == 0 {
		t.Fatalf("render text layers = %#v, want text layers", generationPlan.RenderInputs["textLayers"])
	}
	if !assetLocksContain(generationPlan.RequiredAssets, model.AssetSourceHyperFrames, "text:必须准确") {
		t.Fatalf("required assets = %+v, want exact text lock", generationPlan.RequiredAssets)
	}
}

func TestBuildShotGenerationPlanCustomerMentionAloneDoesNotRouteToUserAsset(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_12", DurationSec: 5, SceneSummary: "客户在会议中讨论年度预算"}

	generationPlan := BuildShotGenerationPlan(shot, model.VisualPlan{}, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if generationPlan.Mode == model.GenerationModeExternalOrUserAsset {
		t.Fatalf("mode = %q, bare customer mention must not route to user asset", generationPlan.Mode)
	}
}

func TestTimedMediaLayerJSONKeepsZeroTimingFields(t *testing.T) {
	payload, err := json.Marshal(model.TimedMediaLayer{ID: "txt-1", Kind: "text", Role: model.TextRoleDataText})
	if err != nil {
		t.Fatalf("marshal TimedMediaLayer: %v", err)
	}
	text := string(payload)
	for _, field := range []string{`"startSec":0`, `"durationSec":0`, `"trackIndex":0`} {
		if !strings.Contains(text, field) {
			t.Fatalf("json = %s, missing %s", text, field)
		}
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

func assetSourceExists(assets []model.ShotAssetNeed, source string) bool {
	for _, asset := range assets {
		if asset.Source == source {
			return true
		}
	}
	return false
}

func assetLocksContain(assets []model.ShotAssetNeed, source string, want string) bool {
	for _, asset := range assets {
		if asset.Source != source {
			continue
		}
		for _, lock := range asset.Locks {
			if lock == want {
				return true
			}
		}
	}
	return false
}
