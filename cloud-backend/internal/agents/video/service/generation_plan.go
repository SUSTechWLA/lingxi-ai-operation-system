package service

import (
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

const (
	artifactKindHyperFramesShot     = "HYPERFRAMES_SHOT"
	artifactKindShotVideoClip       = "SHOT_VIDEO_CLIP"
	artifactKindCompositedShotVideo = "COMPOSITED_SHOT_VIDEO"
	artifactKindShotMediaFusionPlan = "SHOT_MEDIA_FUSION_PLAN"
)

type shotGenerationSignals struct {
	HTMLScore        int
	AIGCScore        int
	UserAssetScore   int
	DynamicAIGC      bool
	HTMLReasons      []string
	AIGCReasons      []string
	UserAssetReasons []string
}

func BuildShotGenerationPlan(
	shot model.ShotUnit,
	visual model.VisualPlan,
	pref model.RenderPreference,
	caps RenderCapabilities,
) model.ShotGenerationPlan {
	plan := buildShotGenerationPlan(shot, visual, pref, caps)
	plan.VisualLayers = buildShotVisualLayerContract(shot, visual, plan, pref, caps)
	return plan
}

func buildShotGenerationPlan(
	shot model.ShotUnit,
	visual model.VisualPlan,
	pref model.RenderPreference,
	caps RenderCapabilities,
) model.ShotGenerationPlan {
	signals := scoreShotGenerationSignals(shot, visual, pref)
	durationSec := resolveShotDuration(shot, visual)
	if err := model.ValidateShotDuration(model.ShotUnit{DurationSec: durationSec}); err != nil {
		return model.ShotGenerationPlan{
			ShotID:    shot.ID,
			Mode:      model.GenerationModePlaceholderPreview,
			Reason:    err.Error(),
			RiskLevel: "high",
		}
	}
	aigcDurationSec := normalizeAIGCGenerationDurationSec(durationSec)
	htmlNeeded := signals.HTMLScore > 0
	aigcNeeded := signals.AIGCScore > 0

	switch {
	case signals.UserAssetScore > 0:
		return buildUserAssetPlan(shot, visual, signals, durationSec, caps.HTMLAvailable)
	case htmlNeeded && aigcNeeded:
		if pref.AllowHybridRender && caps.HTMLAvailable && caps.AIGCAvailable {
			return buildHybridPlan(shot, visual, signals, aigcDurationSec)
		}
		return buildPlaceholderPlan(
			shot,
			visual,
			signals,
			aigcDurationSec,
			"exact text and AIGC both required but safe hybrid rendering is unavailable",
			[]string{"text_safety_boundary", "hybrid_render_unavailable", "provider_availability"},
		)
	case aigcNeeded:
		if !caps.AIGCAvailable {
			return buildPlaceholderPlan(
				shot,
				visual,
				signals,
				aigcDurationSec,
				"AIGC is required but no video provider is available",
				[]string{"external_generation_needed", "placeholder_accuracy", "provider_availability"},
			)
		}
		if signals.DynamicAIGC {
			return buildAIGCVideoPlan(shot, visual, signals, aigcDurationSec)
		}
		if !caps.HTMLAvailable {
			return buildPlaceholderPlan(
				shot,
				visual,
				signals,
				aigcDurationSec,
				"static AIGC image plus HyperFrames requires HTML rendering but HyperFrames is unavailable",
				[]string{"html_provider_required", "placeholder_accuracy", "provider_availability"},
			)
		}
		return buildAIGCImageThenHyperFramesPlan(shot, visual, signals, aigcDurationSec, caps.HTMLAvailable)
	case htmlNeeded:
		if !caps.HTMLAvailable {
			return buildPlaceholderPlan(
				shot,
				visual,
				signals,
				durationSec,
				"HTML rendering is required but HyperFrames is unavailable",
				[]string{"html_provider_required", "placeholder_accuracy", "provider_availability"},
			)
		}
		return buildHTMLOnlyPlan(shot, visual, signals, durationSec)
	default:
		if caps.HTMLAvailable {
			return buildHTMLOnlyPlan(shot, visual, signals, durationSec)
		}
		return buildPlaceholderPlan(
			shot,
			visual,
			signals,
			durationSec,
			"no executable renderer is available for a default preview",
			[]string{"html_provider_required", "placeholder_accuracy", "provider_availability"},
		)
	}
}

func buildShotVisualLayerContract(
	shot model.ShotUnit,
	visual model.VisualPlan,
	plan model.ShotGenerationPlan,
	pref model.RenderPreference,
	caps RenderCapabilities,
) model.ShotVisualLayerContract {
	durationSec := float64(resolveShotDuration(shot, visual))
	if durationSec < 0 {
		durationSec = 0
	}
	signals := scoreShotGenerationSignals(shot, visual, pref)
	aigcPolicy := model.LayerExecutionOptional
	if signals.AIGCScore > 0 {
		aigcPolicy = model.LayerExecutionDeferred
		if caps.AIGCAvailable {
			aigcPolicy = model.LayerExecutionGenerate
		}
	}
	hyperframesPolicy := model.LayerExecutionDeferred
	if caps.HTMLAvailable {
		hyperframesPolicy = model.LayerExecutionGenerate
	}
	outputKind := plan.FusionPlan.OutputArtifactKind
	if outputKind == "" {
		outputKind = artifactKindCompositedShotVideo
	}
	narration := strings.TrimSpace(shot.Narration)
	if narration == "" {
		narration = strings.TrimSpace(shot.MainAction)
	}
	if narration == "" {
		narration = strings.TrimSpace(shot.SceneSummary)
	}
	if narration == "" {
		narration = "本 Shot 的口播与角色表演"
	}
	locks := textLocks(shot, visual)
	hyperframesPrompt := "使用 HyperFrames/HyperKeyframes 精确排版字幕、标题、信息卡片和可控关键帧特效；文字必须与口播一致。"
	if len(locks) > 0 {
		hyperframesPrompt += " 锁定内容：" + strings.Join(locks, "；") + "。"
	}

	return model.ShotVisualLayerContract{
		SchemaVersion: "shot_visual_layers_v1",
		ShotID:        shot.ID,
		Description:   "同一 Shot 由 IP A-roll、HyperFrames 精确文字/特效和 AIGC 丰富素材三层组成，再按统一时间窗合成。",
		IPAroll: model.ShotVisualLayerDesign{
			LayerKey:        "ip_aroll",
			Designed:        true,
			Enabled:         true,
			Required:        false,
			ExecutionPolicy: model.LayerExecutionAuto,
			Role:            "character_aroll_subject",
			Description:     "使用正式 3D IP 角色、正式 Armature、表情和声音拍摄为 2D A-roll，承载口播、眼神、口型和表演连续性。",
			Prompt:          "树懒 IP 在正式演播室中完成本 Shot 表演与口播：" + narration,
			Renderer:        "ip_avatar_3d",
			ArtifactKinds:   []string{"IP_AROLL_VIDEO", "AROLL_ASSET_PACKAGE"},
			SafeArea:        "主体不得遮挡字幕和关键数据；构图需为 AIGC 插入素材与文字层保留安全区。",
			ZIndex:          10,
			StartSec:        0,
			DurationSec:     durationSec,
		},
		HyperFrames: model.ShotVisualLayerDesign{
			LayerKey:        "hyperframes_text",
			Designed:        true,
			Enabled:         caps.HTMLAvailable,
			Required:        true,
			ExecutionPolicy: hyperframesPolicy,
			Role:            "exact_text_and_keyframe_effects",
			Description:     "负责所有精确文字、字幕、标题、UI/信息卡片和可控特效，禁止把可读文字交给 AIGC 生成。",
			Prompt:          hyperframesPrompt,
			Renderer:        "hyperframes",
			ArtifactKinds:   []string{"HYPERFRAMES_SHOT", "SHOT_SUBTITLE"},
			SafeArea:        "遵守字幕、标题和主体安全区；不得遮挡眼睛、嘴部与关键动作。",
			ZIndex:          20,
			StartSec:        0,
			DurationSec:     durationSec,
		},
		AIGC: model.ShotVisualLayerDesign{
			LayerKey:        "aigc_enrichment",
			Designed:        true,
			Enabled:         aigcPolicy == model.LayerExecutionGenerate,
			Required:        signals.AIGCScore > 0,
			ExecutionPolicy: aigcPolicy,
			Role:            "background_broll_or_partial_insert",
			Description:     "生成无文字背景、B-roll 或局部动态素材，补充信息密度、情绪和视觉变化，不替代 IP 口播与精确文字层。",
			Prompt:          aigcPrompt(shot, visual, true),
			Renderer:        "aigc_provider",
			ArtifactKinds:   []string{"SHOT_VIDEO_CLIP", "SHOT_IMAGE"},
			SafeArea:        "必须为 IP 主体和 HyperFrames 文字留出干净区域；禁止生成文字、字幕、Logo 或水印。",
			ZIndex:          0,
			StartSec:        0,
			DurationSec:     durationSec,
		},
		Composition: model.ShotCompositionDesign{
			Description:        "按 Shot 时间窗将 AIGC 背景/插入素材、IP A-roll 主体和 HyperFrames 文字特效合成为一个可独立审核的完整镜头。",
			Assembler:          "ffmpeg_hyperframes_compositor",
			LayerOrder:         []string{"aigc_enrichment", "ip_aroll", "hyperframes_text"},
			TimingPolicy:       "所有层对齐同一 Shot 起止时间；允许 AIGC 仅覆盖局部时间窗，IP 与字幕保持连续。",
			SafeAreaPolicy:     "AIGC 不生成文字，IP 不遮挡文字，HyperFrames 不遮挡眼睛、嘴部和关键动作。",
			OutputArtifactKind: outputKind,
		},
	}
}

func normalizeAIGCGenerationDurationSec(durationSec int) int {
	policy := model.DefaultShotPolicy()
	if durationSec < policy.MinDurationSec {
		return policy.MinDurationSec
	}
	if durationSec > policy.MaxDurationSec {
		return policy.MaxDurationSec
	}
	return durationSec
}

func scoreShotGenerationSignals(shot model.ShotUnit, visual model.VisualPlan, pref model.RenderPreference) shotGenerationSignals {
	var signals shotGenerationSignals

	if len(shot.ScreenText) > 0 {
		signals.HTMLScore += 2
		signals.HTMLReasons = append(signals.HTMLReasons, "screen text needs deterministic typography")
	}
	for _, layer := range visual.TextLayers {
		if layer.MustBeExact {
			signals.HTMLScore += 3
			signals.HTMLReasons = append(signals.HTMLReasons, "must-be-exact text layer")
		}
		if exactTextRole(layer.Role) {
			signals.HTMLScore += 2
			signals.HTMLReasons = append(signals.HTMLReasons, "exact text role: "+layer.Role)
		}
	}
	if pref.PreferHTMLForUI && len(visual.UILayers) > 0 {
		signals.HTMLScore += 2
		signals.HTMLReasons = append(signals.HTMLReasons, "UI layers need deterministic layout")
	}
	if pref.PreferHTMLForCharts && len(visual.DataVisuals) > 0 {
		signals.HTMLScore += 2
		signals.HTMLReasons = append(signals.HTMLReasons, "data visuals need deterministic rendering")
	}

	if pref.PreferAIGCForScene && visual.Background.RequiresAIGC {
		signals.AIGCScore += 3
		signals.AIGCReasons = append(signals.AIGCReasons, "background requires AIGC")
	}
	if visual.MotionPlan.RequiresAIGC {
		signals.AIGCScore += 3
		signals.DynamicAIGC = true
		signals.AIGCReasons = append(signals.AIGCReasons, "motion plan requires AIGC")
	}
	if visual.CameraPlan.RequiresAIGC {
		signals.AIGCScore += 3
		signals.DynamicAIGC = true
		signals.AIGCReasons = append(signals.AIGCReasons, "camera plan requires AIGC")
	}
	if strings.TrimSpace(visual.CameraPlan.Movement) != "" {
		signals.AIGCScore += 2
		signals.DynamicAIGC = true
		signals.AIGCReasons = append(signals.AIGCReasons, "camera movement needs video generation")
	}
	for _, character := range visual.Characters {
		if character.RequiresAIGC {
			signals.AIGCScore += 3
			signals.AIGCReasons = append(signals.AIGCReasons, "character requires AIGC")
		}
		if pref.PreferAIGCForPeople && strings.TrimSpace(character.Motion) != "" {
			signals.AIGCScore += 2
			signals.DynamicAIGC = true
			signals.AIGCReasons = append(signals.AIGCReasons, "character motion needs video generation")
		}
		if pref.PreferAIGCForPeople && strings.TrimSpace(character.Emotion) != "" {
			signals.AIGCScore++
			signals.AIGCReasons = append(signals.AIGCReasons, "character emotion needs AIGC")
		}
	}
	if looksLikeMotion(shot.MainAction) {
		signals.AIGCScore++
		signals.DynamicAIGC = true
		signals.AIGCReasons = append(signals.AIGCReasons, "main action implies natural motion")
	}

	if reason := userAssetReason(shot, visual); reason != "" {
		signals.UserAssetScore += 4
		signals.UserAssetReasons = append(signals.UserAssetReasons, reason)
	}

	return signals
}

func buildHTMLOnlyPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int) model.ShotGenerationPlan {
	asset := shotAssetNeed(shot.ID+"-html-render", "video", "base", model.AssetSourceHyperFrames, shot.ID, textLocks(shot, visual))
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModeHTMLOnly,
		PrimaryTool: "hyperframes_renderer",
		Reason:      planReason("HTML-only render", signals.HTMLReasons, signals.AIGCReasons),
		Confidence:  0.88,
		RiskLevel:   "low",
		RequiredAssets: []model.ShotAssetNeed{
			asset,
		},
		RenderInputs: map[string]interface{}{
			"durationSec": durationSec,
			"textLayers":  visual.TextLayers,
			"screenText":  shot.ScreenText,
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			BaseLayer:          fusionLayer(asset, 0, float64(durationSec)),
			Assembler:          "hyperframes",
			OutputArtifactKind: artifactKindHyperFramesShot,
		},
		ReviewFocus: []string{"exact_text", "layout", "timing"},
	}
}

func buildAIGCVideoPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int) model.ShotGenerationPlan {
	asset := shotAssetNeed(shot.ID+"-aigc-video", "video", "base", model.AssetSourceAIGCVideo, shot.ID, nil)
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModeAIGCVideo,
		PrimaryTool: "text_image_to_video_generator",
		Reason:      planReason("AIGC video render", signals.AIGCReasons, signals.HTMLReasons),
		Confidence:  0.82,
		RiskLevel:   "medium",
		RequiredAssets: []model.ShotAssetNeed{
			asset,
		},
		RenderInputs: map[string]interface{}{
			"durationSec":    durationSec,
			"prompt":         aigcPrompt(shot, visual, false),
			"negativePrompt": "",
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			BaseLayer:          fusionLayer(asset, 0, float64(durationSec)),
			Assembler:          "aigc_video_generator",
			OutputArtifactKind: artifactKindShotVideoClip,
		},
		ReviewFocus: []string{"motion", "character_consistency", "scene_consistency"},
	}
}

func buildAIGCImageThenHyperFramesPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int, htmlAvailable bool) model.ShotGenerationPlan {
	base := shotAssetNeed(shot.ID+"-aigc-image", "image", "base", model.AssetSourceAIGCImage, shot.ID, nil)
	required := []model.ShotAssetNeed{base}
	secondaryTools := []string{}
	overlayLayers := []model.FusionLayer{}
	assembler := "image_to_video"
	if htmlAvailable {
		html := shotAssetNeed(shot.ID+"-html-animation", "video", "animation", model.AssetSourceHyperFrames, shot.ID, textLocks(shot, visual))
		required = append(required, html)
		secondaryTools = append(secondaryTools, "hyperframes_renderer")
		overlayLayers = append(overlayLayers, model.FusionLayer{
			ID:          html.ID,
			Kind:        "html_overlay",
			Role:        "animation_overlay",
			StartSec:    0,
			DurationSec: float64(durationSec),
		})
		assembler = "hyperframes"
	}
	return model.ShotGenerationPlan{
		ShotID:         shot.ID,
		Mode:           model.GenerationModeAIGCImageThenHyperFrames,
		PrimaryTool:    "text_to_image_generator",
		SecondaryTools: secondaryTools,
		Reason:         planReason("AIGC image with deterministic animation", signals.AIGCReasons, signals.HTMLReasons),
		Confidence:     0.78,
		RiskLevel:      "medium",
		RequiredAssets: required,
		RenderInputs: map[string]interface{}{
			"durationSec": durationSec,
			"prompt":      aigcPrompt(shot, visual, signals.HTMLScore > 0),
			"textLayers":  visual.TextLayers,
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			BaseLayer:          fusionLayer(base, 0, float64(durationSec)),
			OverlayLayers:      overlayLayers,
			TimedMedia:         timedTextLayers(durationSec, visual),
			Assembler:          assembler,
			OutputArtifactKind: artifactKindHyperFramesShot,
		},
		ReviewFocus: []string{"background_image", "html_animation", "timing"},
	}
}

func buildHybridPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int) model.ShotGenerationPlan {
	base := shotAssetNeed(shot.ID+"-aigc-background-video", "video", "base", model.AssetSourceAIGCVideo, shot.ID, nil)
	overlay := shotAssetNeed(shot.ID+"-html-overlay", "video", "overlay", model.AssetSourceHyperFrames, shot.ID, textLocks(shot, visual))
	return model.ShotGenerationPlan{
		ShotID:         shot.ID,
		Mode:           model.GenerationModeHybridAIGCBGHTMLOverlay,
		PrimaryTool:    "text_image_to_video_generator",
		SecondaryTools: []string{"hyperframes_renderer", "ffmpeg_compositor"},
		Reason:         planReason("AIGC background with HTML exact overlay", signals.AIGCReasons, signals.HTMLReasons),
		Confidence:     0.84,
		RiskLevel:      "medium",
		RequiredAssets: []model.ShotAssetNeed{
			base,
			overlay,
		},
		RenderInputs: map[string]interface{}{
			"durationSec":    durationSec,
			"prompt":         aigcPrompt(shot, visual, true),
			"textLayers":     visual.TextLayers,
			"negativePrompt": noReadableTextPrompt(),
		},
		FusionPlan: model.FusionPlan{
			ShotID: shot.ID,
			BaseLayer: model.FusionLayer{
				ID:          base.ID,
				Kind:        "video",
				Role:        "aigc_background",
				StartSec:    0,
				DurationSec: float64(durationSec),
			},
			OverlayLayers: []model.FusionLayer{{
				ID:          overlay.ID,
				Kind:        "html_overlay",
				Role:        "exact_text_overlay",
				StartSec:    0,
				DurationSec: float64(durationSec),
			}},
			TimedMedia:         timedTextLayers(durationSec, visual),
			Assembler:          "ffmpeg_compositor",
			OutputArtifactKind: artifactKindCompositedShotVideo,
		},
		ReviewFocus: []string{"aigc_prompt_bans_text", "overlay_text_exactness", "composite_timing"},
	}
}

func buildUserAssetPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int, htmlAvailable bool) model.ShotGenerationPlan {
	assetID := shot.ID + "-user-asset"
	if len(visual.Props) > 0 && strings.TrimSpace(visual.Props[0].ID) != "" {
		assetID = visual.Props[0].ID
	}
	asset := shotAssetNeed(assetID, "image_or_video", "reference_asset", model.AssetSourceUserUpload, shot.ID, nil)
	required := []model.ShotAssetNeed{asset}
	overlayLayers := []model.FusionLayer{}
	timedMedia := []model.TimedMediaLayer{}
	if signals.HTMLScore > 0 && htmlAvailable {
		overlay := shotAssetNeed(shot.ID+"-html-overlay", "video", "overlay", model.AssetSourceHyperFrames, shot.ID, textLocks(shot, visual))
		required = append(required, overlay)
		overlayLayers = append(overlayLayers, model.FusionLayer{
			ID:          overlay.ID,
			Kind:        "html_overlay",
			Role:        "exact_text_overlay",
			StartSec:    0,
			DurationSec: float64(durationSec),
		})
		timedMedia = timedTextLayers(durationSec, visual)
	}
	return model.ShotGenerationPlan{
		ShotID:         shot.ID,
		Mode:           model.GenerationModeExternalOrUserAsset,
		PrimaryTool:    "user_asset_resolver",
		Reason:         planReason("shot depends on uploaded or external brand/product asset", signals.UserAssetReasons, nil),
		Confidence:     0.9,
		RiskLevel:      "low",
		RequiredAssets: required,
		RenderInputs: map[string]interface{}{
			"durationSec":  durationSec,
			"sceneSummary": shot.SceneSummary,
			"props":        visual.Props,
			"textLayers":   visual.TextLayers,
			"screenText":   shot.ScreenText,
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			BaseLayer:          fusionLayer(asset, 0, float64(durationSec)),
			OverlayLayers:      overlayLayers,
			TimedMedia:         timedMedia,
			Assembler:          "user_asset_resolver",
			OutputArtifactKind: artifactKindShotMediaFusionPlan,
		},
		ReviewFocus: []string{"asset_rights", "asset_quality", "brand_accuracy"},
	}
}

func buildPlaceholderPlan(shot model.ShotUnit, visual model.VisualPlan, signals shotGenerationSignals, durationSec int, reason string, reviewFocus []string) model.ShotGenerationPlan {
	need := shotAssetNeed(shot.ID+"-external-generation", "video", "base", model.AssetSourceExternalGeneration, shot.ID, nil)
	placeholder := shotAssetNeed(shot.ID+"-placeholder-preview", "video", "preview", model.AssetSourcePlaceholder, shot.ID, nil)
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModePlaceholderPreview,
		PrimaryTool: "placeholder_renderer",
		Reason:      planReason(reason, signals.AIGCReasons, signals.HTMLReasons),
		Confidence:  0.62,
		RiskLevel:   "high",
		RequiredAssets: []model.ShotAssetNeed{
			need,
			placeholder,
		},
		RenderInputs: map[string]interface{}{
			"durationSec": durationSec,
			"prompt":      aigcPrompt(shot, visual, signals.HTMLScore > 0),
			"textLayers":  visual.TextLayers,
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			BaseLayer:          fusionLayer(placeholder, 0, float64(durationSec)),
			Assembler:          "placeholder_renderer",
			OutputArtifactKind: artifactKindShotMediaFusionPlan,
		},
		FallbackPlan: &model.ShotGenerationFallback{
			Mode:   model.GenerationModePlaceholderPreview,
			Reason: reason,
		},
		ReviewFocus: reviewFocus,
	}
}

func shotAssetNeed(id, kind, role, source, shotID string, locks []string) model.ShotAssetNeed {
	return model.ShotAssetNeed{
		ID:             id,
		Kind:           kind,
		Role:           role,
		Source:         source,
		Required:       true,
		ApprovalStatus: model.ReviewStatusPending,
		RelatedShotID:  shotID,
		Locks:          locks,
	}
}

func fusionLayer(asset model.ShotAssetNeed, startSec, durationSec float64) model.FusionLayer {
	return model.FusionLayer{
		ID:          asset.ID,
		Kind:        asset.Kind,
		Role:        asset.Role,
		StorageRef:  asset.StorageRef,
		StartSec:    startSec,
		DurationSec: durationSec,
	}
}

func resolveShotDuration(shot model.ShotUnit, visual model.VisualPlan) int {
	switch {
	case shot.DurationSec > 0:
		return shot.DurationSec
	case visual.Canvas.DurationSec > 0:
		return visual.Canvas.DurationSec
	default:
		return model.DefaultShotPolicy().PreferDurationSec
	}
}

func timedTextLayers(durationSec int, visual model.VisualPlan) []model.TimedMediaLayer {
	layers := make([]model.TimedMediaLayer, 0, len(visual.TextLayers))
	for index, layer := range visual.TextLayers {
		startSec := layer.StartSec
		if startSec < 0 {
			startSec = 0
		}
		endSec := layer.EndSec
		if endSec <= 0 || endSec > float64(durationSec) {
			endSec = float64(durationSec)
		}
		layerDurationSec := endSec - startSec
		if layerDurationSec < 0 {
			layerDurationSec = 0
		}
		layers = append(layers, model.TimedMediaLayer{
			ID:          layer.ID,
			Kind:        "text",
			Role:        layer.Role,
			StartSec:    startSec,
			DurationSec: layerDurationSec,
			TrackIndex:  index,
			Fit:         "contain",
			Opacity:     1,
		})
	}
	return layers
}

func textLocks(shot model.ShotUnit, visual model.VisualPlan) []string {
	locks := make([]string, 0, len(shot.ScreenText)+len(visual.TextLayers))
	for _, text := range shot.ScreenText {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			locks = append(locks, "text:"+trimmed)
		}
	}
	for _, layer := range visual.TextLayers {
		if (layer.MustBeExact || exactTextRole(layer.Role)) && strings.TrimSpace(layer.Text) != "" {
			locks = append(locks, "text:"+strings.TrimSpace(layer.Text))
		}
	}
	return uniqueStrings(locks)
}

func planReason(prefix string, primary []string, secondary []string) string {
	reasons := append([]string{}, primary...)
	reasons = append(reasons, secondary...)
	if len(reasons) == 0 {
		return prefix + "."
	}
	return fmt.Sprintf("%s: %s.", prefix, strings.Join(uniqueStrings(reasons), "; "))
}

func aigcPrompt(shot model.ShotUnit, visual model.VisualPlan, banText bool) string {
	parts := []string{
		shot.SceneSummary,
		shot.MainAction,
		visual.Background.Description,
		visual.MotionPlan.Description,
		visual.CameraPlan.Description,
	}
	for _, character := range visual.Characters {
		parts = append(parts, character.Description, character.Motion, character.Emotion)
	}
	if banText {
		exactTexts := exactTextValues(shot, visual)
		for i := range parts {
			parts[i] = removeExactTexts(parts[i], exactTexts)
		}
	}
	prompt := strings.Join(nonEmptyStrings(parts), "。")
	if banText {
		if prompt != "" {
			prompt += "。"
		}
		prompt += noReadableTextPrompt()
	}
	return prompt
}

func noReadableTextPrompt() string {
	return "禁止生成任何可读文字。禁止生成中文字符。禁止生成英文单词。禁止生成 UI 文字。禁止生成字幕、标签、水印"
}

func exactTextValues(shot model.ShotUnit, visual model.VisualPlan) []string {
	values := make([]string, 0, len(shot.ScreenText)+len(visual.TextLayers))
	for _, text := range shot.ScreenText {
		if strings.TrimSpace(text) != "" {
			values = append(values, strings.TrimSpace(text))
		}
	}
	for _, layer := range visual.TextLayers {
		if (layer.MustBeExact || exactTextRole(layer.Role)) && strings.TrimSpace(layer.Text) != "" {
			values = append(values, strings.TrimSpace(layer.Text))
		}
	}
	return uniqueStrings(values)
}

func removeExactTexts(value string, exactTexts []string) string {
	for _, text := range exactTexts {
		value = strings.ReplaceAll(value, text, "")
	}
	return strings.Trim(value, " \t\n\r，,。;；:：")
}

func userAssetReason(shot model.ShotUnit, visual model.VisualPlan) string {
	values := []string{shot.SceneSummary, shot.MainAction, shot.Title}
	for _, prop := range visual.Props {
		values = append(values, prop.ID, prop.Description)
	}
	combined := strings.ToLower(strings.Join(values, " "))
	switch {
	case containsAny(combined, "上传", "user upload", "uploaded", "客户提供", "提供素材", "提供的素材", "provided asset", "user-provided"):
		return "mentions uploaded or customer-provided asset"
	case containsAny(combined, "logo", "品牌", "brand"):
		return "mentions logo or brand asset"
	case containsAny(combined, "截图", "screenshot", "产品", "product"):
		return "mentions screenshot or product asset"
	case containsAny(combined, "新闻", "news"):
		return "mentions news asset requiring external sourcing"
	default:
		return ""
	}
}

func looksLikeMotion(value string) bool {
	return containsAny(strings.ToLower(value), "穿过", "走", "跑", "回头", "转身", "walk", "run", "turn", "move", "camera")
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
