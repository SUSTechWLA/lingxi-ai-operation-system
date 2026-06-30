package planjudge

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
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
	if report.Passed {
		t.Fatalf("missing publish copy should fail the beta video plan")
	}
}

func TestJudgeWarnsForBetaDisabledTool(t *testing.T) {
	plan := goodVoicePlan()
	plan.Steps = append(plan.Steps, agentruntime.AgentStep{ID: "voice", Tool: "CosyVoice", Arguments: map[string]interface{}{"stage": "audio"}})
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningBetaDisabledCapability) {
		t.Fatalf("expected beta disabled capability warning, got %+v", report.Warnings)
	}
	if !report.Passed {
		t.Fatalf("non-critical beta capability warning should not block plan: %+v", report.Warnings)
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
	if report.Passed {
		t.Fatalf("render without preview dependency should fail the beta video plan")
	}
}

func TestJudgeAcceptsCardPlanGeneratorAsVisualStage(t *testing.T) {
	plan := goodVoicePlan()
	for i := range plan.Steps {
		switch plan.Steps[i].ID {
		case "beat":
			plan.Steps[i] = agentruntime.AgentStep{
				ID:        "storyboard",
				Intent:    "把脚本拆成图文卡片和字幕节奏",
				Tool:      "card_plan_generator",
				DependsOn: []string{"script"},
				Arguments: map[string]interface{}{"stage": "storyboard"},
				ExpectedOutput: []string{
					"cardPlan",
					"CARD_PLAN",
				},
				ProduceArtifact: true,
			}
		case "preview":
			plan.Steps[i].DependsOn = []string{"storyboard"}
		}
	}

	report := New().Evaluate(plan)

	if !report.Passed {
		t.Fatalf("card plan should satisfy visual beta stage, warnings: %+v", report.Warnings)
	}
}

func TestJudgeWarnsForRedundantTool(t *testing.T) {
	plan := goodVoicePlan()
	plan.Steps = append(plan.Steps, agentruntime.AgentStep{ID: "script_again", Tool: "video_script_generator", Arguments: map[string]interface{}{"stage": "script"}})
	report := New().Evaluate(plan)
	if !hasWarning(report, WarningRedundantTool) {
		t.Fatalf("expected redundant tool warning, got %+v", report.Warnings)
	}
	if !report.Passed {
		t.Fatalf("redundant tool warning should not block plan: %+v", report.Warnings)
	}
}

func TestJudgeFailsWhenRequiredVideoStagesMissing(t *testing.T) {
	plan := &agentruntime.AgentPlan{
		Goal:   "make incomplete video",
		Domain: "video_creation",
		Steps: []agentruntime.AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"stage": "script"}, ExpectedOutput: []string{"voiceover_script"}},
		},
	}

	report := New().Evaluate(plan)

	if report.Passed {
		t.Fatalf("plan missing visual, preview, render and publish stages should fail: %+v", report.Warnings)
	}
	for _, code := range []string{WarningMissingStage, WarningMissingPublishCopy} {
		if !hasWarning(report, code) {
			t.Fatalf("expected warning code %s, got %+v", code, report.Warnings)
		}
	}
}

func TestPreparedScriptOnlyPlanPassesBetaJudge(t *testing.T) {
	plan := &agentruntime.AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain: "video_creation",
		Steps: []agentruntime.AgentStep{
			{
				ID:              "script",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"stage": "script"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
		},
	}

	compiler := agentruntime.NewPlanCompiler(judgeStaticToolCatalog{
		"video_script_generator":        {Name: "video_script_generator"},
		"shot_splitter":                 {Name: "shot_splitter", Output: map[string]tool.ParamDef{"shotList": {Type: "array"}}},
		"video_prompt_generator":        {Name: "video_prompt_generator", Output: map[string]tool.ParamDef{"videoPrompts": {Type: "array"}}},
		"hyperframes_project_generator": {Name: "hyperframes_project_generator", Parameters: map[string]tool.ParamDef{"shotList": {Type: "array"}, "videoPrompts": {Type: "array"}}, Output: map[string]tool.ParamDef{"projectDir": {Type: "string"}}},
		"hyperframes_renderer":          {Name: "hyperframes_renderer", Parameters: map[string]tool.ParamDef{"projectDir": {Type: "string"}}, Output: map[string]tool.ParamDef{"outputPath": {Type: "string"}}},
		"publish_copy_generator":        {Name: "publish_copy_generator"},
	})
	prepared := compiler.PreparePlan(plan)

	report := New().Evaluate(prepared)
	if !report.Passed {
		t.Fatalf("prepared script-only plan should pass beta judge, warnings: %+v, steps: %+v", report.Warnings, prepared.Steps)
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

type judgeStaticToolCatalog map[string]*tool.ToolManifest

func (c judgeStaticToolCatalog) GetManifest(name string) *tool.ToolManifest {
	return c[name]
}

func hasWarning(report Report, code string) bool {
	for _, warning := range report.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
