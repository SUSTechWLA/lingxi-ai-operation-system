package service

import (
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type RenderCapabilities struct {
	AIGCAvailable bool
	HTMLAvailable bool
}

func DecideRenderStrategy(
	shot model.ShotUnit,
	plan model.VisualPlan,
	pref model.RenderPreference,
	caps RenderCapabilities,
) model.RenderStrategy {
	htmlRequired := needsHTML(shot, plan, pref)
	aigcRequired := needsAIGC(plan, pref)

	if htmlRequired && !caps.HTMLAvailable {
		htmlRequired = false
	}
	if aigcRequired && !caps.AIGCAvailable {
		aigcRequired = false
	}

	switch {
	case htmlRequired && aigcRequired:
		if pref.PreferLowCostPreview && shouldPreviewFirst(shot, plan) {
			return model.RenderStrategy{
				Mode:              model.RenderModeHTMLPreviewThenHybrid,
				PrimaryTool:       "hyperframes_renderer",
				SecondaryTools:    []string{"text_image_to_video_generator", "ffmpeg_compositor"},
				Reason:            "口播或信息密集镜头先用 HyperFrames 预览节奏和准确文字，再生成 AIGC 背景并合成。",
				AIGCRequired:      true,
				HTMLRequired:      true,
				TextOverlayNeeded: true,
				NeedsCompositing:  true,
				AIGCInput:         &model.AIGCInputSpec{DurationSec: shot.DurationSec},
				HTMLInput:         &model.HTMLInputSpec{DurationSec: shot.DurationSec, TextLayers: plan.TextLayers},
				CompositePlan:     &model.CompositePlan{OutputArtifactType: "composited_shot_video"},
			}
		}
		return model.RenderStrategy{
			Mode:              model.RenderModeHybridAIGCBGHTMLOverlay,
			PrimaryTool:       "text_image_to_video_generator",
			SecondaryTools:    []string{"hyperframes_renderer", "ffmpeg_compositor"},
			Reason:            "AIGC 负责人物、场景、情绪或运动，HyperFrames 负责准确文字层。",
			AIGCRequired:      true,
			HTMLRequired:      true,
			TextOverlayNeeded: true,
			NeedsCompositing:  true,
			AIGCInput:         &model.AIGCInputSpec{DurationSec: shot.DurationSec},
			HTMLInput:         &model.HTMLInputSpec{DurationSec: shot.DurationSec, TextLayers: plan.TextLayers},
			CompositePlan:     &model.CompositePlan{OutputArtifactType: "composited_shot_video"},
		}
	case htmlRequired:
		return model.RenderStrategy{
			Mode:              model.RenderModeHTMLOnly,
			PrimaryTool:       "hyperframes_renderer",
			Reason:            "镜头以准确文字、UI、图表或信息结构为主，使用 HyperFrames 确定性渲染。",
			HTMLRequired:      true,
			TextOverlayNeeded: true,
			HTMLInput:         &model.HTMLInputSpec{DurationSec: shot.DurationSec, TextLayers: plan.TextLayers},
		}
	case aigcRequired:
		if pref.PreferLowCostPreview && shouldPreviewFirst(shot, plan) {
			return model.RenderStrategy{
				Mode:           model.RenderModeHTMLPreviewThenAIGC,
				PrimaryTool:    "hyperframes_renderer",
				SecondaryTools: []string{"text_image_to_video_generator"},
				Reason:         "高成本或需要反复审核的 AIGC 镜头先生成低成本 HTML 预览。",
				AIGCRequired:   true,
				AIGCInput:      &model.AIGCInputSpec{DurationSec: shot.DurationSec},
				HTMLInput:      &model.HTMLInputSpec{DurationSec: shot.DurationSec, TextLayers: plan.TextLayers},
			}
		}
		return model.RenderStrategy{
			Mode:         model.RenderModeAIGCOnly,
			PrimaryTool:  "text_image_to_video_generator",
			Reason:       "镜头主要需求是人物、场景、情绪或自然运动，且没有准确文字要求。",
			AIGCRequired: true,
			AIGCInput:    &model.AIGCInputSpec{DurationSec: shot.DurationSec},
		}
	default:
		return model.RenderStrategy{
			Mode:              model.RenderModeHTMLOnly,
			PrimaryTool:       "hyperframes_renderer",
			Reason:            "没有强 AIGC 需求，默认使用低成本确定性 HTML 渲染。",
			HTMLRequired:      caps.HTMLAvailable,
			TextOverlayNeeded: len(plan.TextLayers) > 0 || len(shot.ScreenText) > 0,
			HTMLInput:         &model.HTMLInputSpec{DurationSec: shot.DurationSec, TextLayers: plan.TextLayers},
		}
	}
}

func needsHTML(shot model.ShotUnit, plan model.VisualPlan, pref model.RenderPreference) bool {
	if pref.PreferHTMLForText && len(shot.ScreenText) > 0 {
		return true
	}
	for _, layer := range plan.TextLayers {
		if layer.MustBeExact || exactTextRole(layer.Role) {
			return true
		}
	}
	if pref.PreferHTMLForUI && len(plan.UILayers) > 0 {
		return true
	}
	if pref.PreferHTMLForCharts && len(plan.DataVisuals) > 0 {
		return true
	}
	return false
}

func needsAIGC(plan model.VisualPlan, pref model.RenderPreference) bool {
	if plan.Background.RequiresAIGC && pref.PreferAIGCForScene {
		return true
	}
	if plan.MotionPlan.RequiresAIGC || plan.CameraPlan.RequiresAIGC {
		return true
	}
	if strings.TrimSpace(plan.CameraPlan.Movement) != "" {
		return true
	}
	for _, character := range plan.Characters {
		if character.RequiresAIGC {
			return true
		}
		if pref.PreferAIGCForPeople && (strings.TrimSpace(character.Motion) != "" || strings.TrimSpace(character.Emotion) != "") {
			return true
		}
	}
	return false
}

func shouldPreviewFirst(shot model.ShotUnit, plan model.VisualPlan) bool {
	text := strings.ToLower(strings.Join([]string{shot.VideoType, shot.Title, shot.SceneSummary, shot.Narration}, " "))
	if strings.Contains(text, "口播") || strings.Contains(text, "oral") {
		return true
	}
	return len(shot.ScreenText) >= 2 || len(plan.TextLayers) >= 2 || len(plan.DataVisuals) > 0 || len(plan.UILayers) > 0
}

func exactTextRole(role string) bool {
	switch role {
	case model.TextRoleTitle,
		model.TextRoleSubtitle,
		model.TextRoleKeyword,
		model.TextRoleLabel,
		model.TextRoleUIText,
		model.TextRoleCaption,
		model.TextRoleDataText,
		model.TextRoleButtonText,
		model.TextRoleFileName,
		model.TextRoleChatMessage,
		model.TextRoleCode,
		model.TextRoleNumber,
		model.TextRoleBrandName:
		return true
	default:
		return false
	}
}
