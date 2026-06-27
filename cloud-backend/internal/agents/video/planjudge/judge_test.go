package planjudge

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
)

func TestJudgePassesGoodVoiceVisualPlan(t *testing.T) {
	report := New().Evaluate(goodVoicePlan())
	if !report.Passed {
		t.Fatalf("expected good plan to pass, warnings: %+v", report.Warnings)
	}
}

func TestJudgeWarnsWhenPublishCopyMissing(t *testing.T) {
	plan := goodVoicePlan()
	plan.Steps = plan.Steps[:len(plan.Steps)-1]
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningMissingPublishCopy) {
		t.Fatalf("expected missing publish copy warning, got %+v", report.Warnings)
	}
}

func TestJudgeWarnsForBetaDisabledTool(t *testing.T) {
	plan := goodVoicePlan()
	plan.Steps = append(plan.Steps, agentruntime.AgentStep{ID: "voice", Tool: "CosyVoice", Arguments: map[string]interface{}{"stage": "audio"}})
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningBetaDisabledCapability) {
		t.Fatalf("expected beta disabled capability warning, got %+v", report.Warnings)
	}
}

func TestJudgeWarnsForRenderWithoutPreviewDependency(t *testing.T) {
	plan := goodVoicePlan()
	for i := range plan.Steps {
		if plan.Steps[i].ID == "render" {
			plan.Steps[i].DependsOn = []string{"script"}
		}
	}
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningRenderRequiresPreview) {
		t.Fatalf("expected render preview warning, got %+v", report.Warnings)
	}
}

func TestJudgeWarnsForRedundantTool(t *testing.T) {
	plan := goodVoicePlan()
	plan.Steps = append(plan.Steps, agentruntime.AgentStep{ID: "script_again", Tool: "video_script_generator", Arguments: map[string]interface{}{"stage": "script"}})
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningRedundantTool) {
		t.Fatalf("expected redundant tool warning, got %+v", report.Warnings)
	}
}

func goodVoicePlan() *agentruntime.AgentPlan {
	return &agentruntime.AgentPlan{
		Goal:   "make voice visual video",
		Domain: "video_creation",
		Steps: []agentruntime.AgentStep{
			{ID: "brief", Tool: "proposal_generator", Arguments: map[string]interface{}{"stage": "brief"}, ExpectedOutput: []string{"creative_brief"}},
			{ID: "script", Tool: "video_script_generator", DependsOn: []string{"brief"}, Arguments: map[string]interface{}{"stage": "script"}, ExpectedOutput: []string{"voiceover_script"}},
			{ID: "beat", Tool: "shot_splitter", DependsOn: []string{"script"}, Arguments: map[string]interface{}{"stage": "beat"}, ExpectedOutput: []string{"beat_plan"}},
			{ID: "preview", Tool: "hyperframes_project_generator", DependsOn: []string{"beat"}, Arguments: map[string]interface{}{"stage": "preview"}, ExpectedOutput: []string{"preview"}},
			{ID: "render", Tool: "hyperframes_renderer", DependsOn: []string{"preview"}, Arguments: map[string]interface{}{"stage": "render"}, ExpectedOutput: []string{"final_video"}},
			{ID: "publish", Tool: "publish_copy_generator", DependsOn: []string{"render"}, Arguments: map[string]interface{}{"stage": "publish"}, ExpectedOutput: []string{"publish_copy"}},
		},
	}
}

func hasWarning(report Report, code string) bool {
	for _, warning := range report.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
