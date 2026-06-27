package planjudge

import (
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
)

const (
	WarningMissingStage           = "missing_stage"
	WarningRedundantTool          = "redundant_tool"
	WarningBetaDisabledCapability = "beta_disabled_capability"
	WarningRenderRequiresPreview  = "render_requires_preview"
	WarningMissingPublishCopy     = "missing_publish_copy"
)

type Warning struct {
	Code     string `json:"code"`
	StepID   string `json:"stepId,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type Report struct {
	Passed   bool      `json:"passed"`
	Warnings []Warning `json:"warnings,omitempty"`
}

type Judge struct {
	disabledTools map[string]bool
}

func New() Judge {
	return Judge{disabledTools: map[string]bool{
		"cosyvoice":                true,
		"diffsinger":               true,
		"imagebind":                true,
		"fish-speech":              true,
		"seed-vc":                  true,
		"videorag":                 true,
		"video_qa":                 true,
		"long_video_understanding": true,
		"automatic_publisher":      true,
		"auto_publish":             true,
		"platform_auto_publish":    true,
		"long_video_understander":  true,
		"video_question_answering": true,
		"imagebind_retriever":      true,
		"videorag_retriever":       true,
		"fish_speech_voice_cloner": true,
		"seed_vc_voice_conversion": true,
		"diffsinger_vocal_synth":   true,
		"cosyvoice_speech_synth":   true,
	}}
}

func (j Judge) Evaluate(plan *agentruntime.AgentPlan) Report {
	if plan == nil {
		return Report{Passed: false, Warnings: []Warning{{Code: WarningMissingStage, Message: "agent plan is nil", Severity: "warning"}}}
	}
	warnings := make([]Warning, 0)
	warnings = append(warnings, j.disabledCapabilityWarnings(plan)...)
	warnings = append(warnings, redundantToolWarnings(plan)...)
	warnings = append(warnings, missingStageWarnings(plan)...)
	warnings = append(warnings, renderPreviewWarnings(plan)...)
	if !hasPublishCopy(plan) {
		warnings = append(warnings, Warning{
			Code:     WarningMissingPublishCopy,
			Message:  "plan should include publish_copy_generator before package/export in beta",
			Severity: "warning",
		})
	}
	return Report{Passed: len(warnings) == 0, Warnings: warnings}
}

func NewRuntimeJudge() agentruntime.PlanJudge {
	return runtimeJudge{judge: New()}
}

type runtimeJudge struct {
	judge Judge
}

func (r runtimeJudge) Evaluate(plan *agentruntime.AgentPlan) agentruntime.PlanJudgeReport {
	report := r.judge.Evaluate(plan)
	warnings := make([]agentruntime.PlanJudgeWarning, 0, len(report.Warnings))
	for _, warning := range report.Warnings {
		warnings = append(warnings, agentruntime.PlanJudgeWarning{
			Code:     warning.Code,
			StepID:   warning.StepID,
			Tool:     warning.Tool,
			Message:  warning.Message,
			Severity: warning.Severity,
		})
	}
	return agentruntime.PlanJudgeReport{Passed: report.Passed, Warnings: warnings}
}

func (j Judge) disabledCapabilityWarnings(plan *agentruntime.AgentPlan) []Warning {
	out := []Warning{}
	for _, step := range plan.Steps {
		tool := strings.ToLower(strings.TrimSpace(step.Tool))
		if j.disabledTools[tool] {
			out = append(out, Warning{
				Code:     WarningBetaDisabledCapability,
				StepID:   step.ID,
				Tool:     step.Tool,
				Message:  "tool is disabled in the lightweight video beta",
				Severity: "warning",
			})
		}
	}
	return out
}

func redundantToolWarnings(plan *agentruntime.AgentPlan) []Warning {
	counts := map[string]int{}
	out := []Warning{}
	for _, step := range plan.Steps {
		tool := strings.TrimSpace(step.Tool)
		if tool == "" || isQualityTool(tool) {
			continue
		}
		counts[tool]++
		if counts[tool] > 1 {
			out = append(out, Warning{
				Code:     WarningRedundantTool,
				StepID:   step.ID,
				Tool:     step.Tool,
				Message:  "same non-quality tool appears multiple times; check whether this duplicates an existing stage",
				Severity: "warning",
			})
		}
	}
	return out
}

func missingStageWarnings(plan *agentruntime.AgentPlan) []Warning {
	checks := []struct {
		codeName string
		terms    []string
	}{
		{"script", []string{"script", "voiceover_script"}},
		{"visual_or_shot_plan", []string{"beat_plan", "shot_list", "visual_component_plan"}},
		{"prompt_or_preview", []string{"video_prompt", "keyframe_prompt", "preview", "hyperframes_project"}},
	}
	out := []Warning{}
	for _, check := range checks {
		if !planMentionsAny(plan, check.terms) {
			out = append(out, Warning{
				Code:     WarningMissingStage,
				Message:  "plan is missing expected beta stage: " + check.codeName,
				Severity: "warning",
			})
		}
	}
	if planMentionsAny(plan, []string{"shot_list"}) && !planMentionsAny(plan, []string{"video_prompt", "keyframe_prompt"}) {
		out = append(out, Warning{
			Code:     WarningMissingStage,
			Message:  "aigc_shot-style plan has shot_list but is missing keyframe/video prompt stage",
			Severity: "warning",
		})
	}
	return out
}

func renderPreviewWarnings(plan *agentruntime.AgentPlan) []Warning {
	steps := map[string]agentruntime.AgentStep{}
	for _, step := range plan.Steps {
		steps[step.ID] = step
	}
	out := []Warning{}
	for _, step := range plan.Steps {
		if !isRenderTool(step.Tool) {
			continue
		}
		if !dependsOnPreview(step, steps) {
			out = append(out, Warning{
				Code:     WarningRenderRequiresPreview,
				StepID:   step.ID,
				Tool:     step.Tool,
				Message:  "render step should depend on preview/composition/render_strategy output before execution",
				Severity: "warning",
			})
		}
	}
	return out
}

func hasPublishCopy(plan *agentruntime.AgentPlan) bool {
	return planMentionsAny(plan, []string{"publish_copy", "publishcopy", "publish-copy", "publish_copy_generator"})
}

func planMentionsAny(plan *agentruntime.AgentPlan, terms []string) bool {
	for _, step := range plan.Steps {
		haystack := strings.ToLower(step.ID + " " + step.Intent + " " + step.Tool)
		if stage, ok := step.Arguments["stage"].(string); ok {
			haystack += " " + stage
		}
		for _, out := range step.ExpectedOutput {
			haystack += " " + strings.ToLower(out)
		}
		for _, term := range terms {
			if strings.Contains(haystack, strings.ToLower(term)) {
				return true
			}
		}
	}
	return false
}

func isQualityTool(tool string) bool {
	tool = strings.ToLower(tool)
	return strings.Contains(tool, "quality") || strings.Contains(tool, "checker")
}

func isRenderTool(tool string) bool {
	tool = strings.ToLower(strings.TrimSpace(tool))
	return tool == "hyperframes_renderer" ||
		tool == "text_image_to_video_generator" ||
		tool == "video_final_assembler"
}

func dependsOnPreview(step agentruntime.AgentStep, steps map[string]agentruntime.AgentStep) bool {
	for _, dep := range step.DependsOn {
		depStep := steps[dep]
		haystack := strings.ToLower(dep + " " + depStep.Tool + " " + depStep.Intent)
		if stage, ok := depStep.Arguments["stage"].(string); ok {
			haystack += " " + stage
		}
		for _, out := range depStep.ExpectedOutput {
			haystack += " " + strings.ToLower(out)
		}
		for _, term := range []string{"preview", "snapshot", "composition", "render_strategy", "hyperframes_project"} {
			if strings.Contains(haystack, term) {
				return true
			}
		}
	}
	return false
}
