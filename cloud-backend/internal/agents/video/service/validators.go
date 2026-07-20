package service

import (
	"fmt"
	"math"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type ValidationIssue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

func CheckShotDuration(shot model.ShotUnit) []ValidationIssue {
	return CheckShotDurationSec(shot.ID, float64(shot.DurationSec))
}

func CheckShotDurationSec(shotID string, durationSec float64) []ValidationIssue {
	if durationSec < 3 || durationSec >= 15 || math.IsNaN(durationSec) || math.IsInf(durationSec, 0) {
		return []ValidationIssue{{
			Code:     "shot_duration_out_of_range",
			Field:    "durationSec",
			Message:  fmt.Sprintf("shot duration must be at least 3 seconds and less than 15 seconds, got %.2f for %s", durationSec, shotID),
			Severity: "error",
		}}
	}
	return nil
}

func CheckShotSceneComplexity(shot model.ShotUnit) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if !shot.SingleScene {
		issues = append(issues, ValidationIssue{
			Code:     "shot_multi_scene",
			Field:    "singleScene",
			Message:  "shot must stay in a single scene",
			Severity: "error",
		})
	}
	if shot.VisualChangeLevel == model.VisualChangeHigh {
		issues = append(issues, ValidationIssue{
			Code:     "shot_visual_change_high",
			Field:    "visualChangeLevel",
			Message:  "shot visual change level must not be high",
			Severity: "error",
		})
	}
	if containsAny(shot.MainAction, "多个场景", "蒙太奇", "快速转场", "跑到街头", "再进入") {
		issues = append(issues, ValidationIssue{
			Code:     "shot_action_too_complex",
			Field:    "mainAction",
			Message:  "shot action suggests multiple locations, montage, or rapid transitions",
			Severity: "error",
		})
	}
	return issues
}

func CheckTextLayerExactness(shot model.ShotUnit, plan model.VisualPlan, strategy model.RenderStrategy) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, text := range shot.ScreenText {
		if !textLayerContains(plan.TextLayers, text) {
			issues = append(issues, ValidationIssue{
				Code:     "screen_text_missing_text_layer",
				Field:    "textLayers",
				Message:  "screen text must be represented in TextLayers: " + text,
				Severity: "error",
			})
		}
	}
	for _, layer := range plan.TextLayers {
		if layer.MustBeExact && strategy.Mode == model.RenderModeAIGCOnly {
			issues = append(issues, ValidationIssue{
				Code:     "exact_text_requires_html",
				Field:    "renderStrategy.mode",
				Message:  "mustBeExact text cannot use aigc_only",
				Severity: "error",
			})
		}
		if layer.StartSec < 0 || layer.EndSec < 0 || layer.EndSec > float64(shot.DurationSec) || layer.StartSec > layer.EndSec {
			issues = append(issues, ValidationIssue{
				Code:     "text_layer_timing_out_of_range",
				Field:    "textLayers",
				Message:  "text layer timing must stay within shot duration",
				Severity: "error",
			})
		}
	}
	return issues
}

func CheckRenderStrategy(shot model.ShotUnit, plan model.VisualPlan, strategy model.RenderStrategy) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if strategy.Mode == model.RenderModeAIGCOnly && (len(shot.ScreenText) > 0 || hasExactText(plan)) {
		issues = append(issues, ValidationIssue{
			Code:     "exact_text_requires_html",
			Field:    "mode",
			Message:  "shots with exact text, UI, charts, tables, chat records, code, or numbers cannot use aigc_only",
			Severity: "error",
		})
	}
	if strategy.Mode == model.RenderModeAIGCOnly && !needsAIGC(plan, model.DefaultRenderPreference()) {
		issues = append(issues, ValidationIssue{
			Code:     "aigc_only_without_aigc_need",
			Field:    "mode",
			Message:  "aigc_only has no people, scene, motion, camera, or emotion signal",
			Severity: "warning",
		})
	}
	return issues
}

func CheckAIGCPromptNoText(strategy model.RenderStrategy, prompt string) []ValidationIssue {
	if strategy.Mode != model.RenderModeHybridAIGCBGHTMLOverlay && strategy.Mode != model.RenderModeHTMLPreviewThenHybrid {
		return nil
	}
	required := []string{
		"禁止生成任何可读文字",
		"禁止生成中文字符",
		"禁止生成英文单词",
		"禁止生成 UI 文字",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			return []ValidationIssue{{
				Code:     "hybrid_aigc_prompt_must_ban_text",
				Field:    "prompt",
				Message:  "hybrid AIGC prompt must explicitly ban readable text, UI text, subtitles, labels, and watermarks",
				Severity: "error",
			}}
		}
	}
	for _, concept := range []string{"字幕", "标签", "水印"} {
		if !strings.Contains(prompt, concept) {
			return []ValidationIssue{{
				Code:     "hybrid_aigc_prompt_must_ban_text",
				Field:    "prompt",
				Message:  "hybrid AIGC prompt must explicitly ban readable text, UI text, subtitles, labels, and watermarks",
				Severity: "error",
			}}
		}
	}
	return nil
}

func hasExactText(plan model.VisualPlan) bool {
	if len(plan.UILayers) > 0 || len(plan.DataVisuals) > 0 {
		return true
	}
	for _, layer := range plan.TextLayers {
		if layer.MustBeExact || exactTextRole(layer.Role) {
			return true
		}
	}
	return false
}

func textLayerContains(layers []model.TextLayerSpec, text string) bool {
	for _, layer := range layers {
		if strings.TrimSpace(layer.Text) == strings.TrimSpace(text) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
