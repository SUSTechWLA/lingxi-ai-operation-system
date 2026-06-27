package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/intent"
	"github.com/tangying-ai/aios-core/internal/agents/video/planjudge"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type EvalCase struct {
	Name                              string
	Input                             string
	ExpectedVideoType                 intent.VideoType
	Plan                              *agentruntime.AgentPlan
	ExpectedPlanGuardPassed           bool
	ExpectedPlanJudgePassed           bool
	ExpectedRequiredArtifactsComplete bool
	ExpectedForbiddenToolAbsent       bool
	ExpectedPublishCopyComplete       bool
}

type CaseResult struct {
	Name    string
	Passed  bool
	Metrics map[string]bool
}

type Report struct {
	Total  int
	Passed int
	Cases  []CaseResult
}

func main() {
	report := Run(DefaultCases())
	fmt.Print(report.String())
	if report.Passed != report.Total {
		os.Exit(1)
	}
}

func Run(cases []EvalCase) Report {
	results := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		results = append(results, runCase(c))
	}
	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}
	return Report{Total: len(results), Passed: passed, Cases: results}
}

func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Video Beta Eval: %d/%d cases passed\n", r.Passed, r.Total)
	for _, c := range r.Cases {
		status := "FAIL"
		if c.Passed {
			status = "PASS"
		}
		fmt.Fprintf(&b, "- [%s] %s", status, c.Name)
		keys := make([]string, 0, len(c.Metrics))
		for key := range c.Metrics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, " %s=%t", key, c.Metrics[key])
		}
		b.WriteString("\n")
	}
	return b.String()
}

func runCase(c EvalCase) CaseResult {
	inferred, intentErr := intent.Infer(context.Background(), c.Input)
	guardErr := agentruntime.NewPlanGuard(evalCatalog(), nil).Validate(c.Plan)
	judgeReport := planjudge.New().Evaluate(c.Plan)

	metrics := map[string]bool{
		"intent_correct":              intentErr == nil && inferred.VideoType == c.ExpectedVideoType,
		"plan_guard_passed":           (guardErr == nil) == c.ExpectedPlanGuardPassed,
		"plan_judge_passed":           judgeReport.Passed == c.ExpectedPlanJudgePassed,
		"required_artifacts_complete": requiredArtifactsComplete(inferred.RequiredArtifacts, c.Plan) == c.ExpectedRequiredArtifactsComplete,
		"forbidden_tool_absent":       forbiddenToolAbsent(c.Plan) == c.ExpectedForbiddenToolAbsent,
		"publish_copy_complete":       publishCopyComplete(c.Plan) == c.ExpectedPublishCopyComplete,
	}
	passed := true
	for _, ok := range metrics {
		if !ok {
			passed = false
			break
		}
	}
	return CaseResult{Name: c.Name, Passed: passed, Metrics: metrics}
}

func DefaultCases() []EvalCase {
	return []EvalCase{
		{Name: "voice visual xhs", Input: "做一期90秒口播视频，讲AI Agent替代工作流，发小红书", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: voicePlan(), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: true, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "voice visual bilibili", Input: "我想做知识分享口播，主题是个人AI工作流，发B站", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: voicePlan(), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: true, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "aigc shot story", Input: "写一个有角色和场景的镜头式AI短片故事，60秒", ExpectedVideoType: intent.VideoTypeAIGCShot, Plan: aigcPlan(), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: true, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "aigc shot scene", Input: "做一条分镜清晰的剧情短片，需要角色、场景和视频Prompt", ExpectedVideoType: intent.VideoTypeAIGCShot, Plan: aigcPlan(), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: true, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "missing publish copy", Input: "做一期口播观点视频，讲效率工具", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: withoutPublishCopy(voicePlan()), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: false, ExpectedRequiredArtifactsComplete: false, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: false},
		{Name: "forbidden cosyvoice", Input: "做一期口播知识视频，讲AI工作流", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: withExtraTool(voicePlan(), "voice_clone", "CosyVoice"), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: false, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: false, ExpectedPublishCopyComplete: true},
		{Name: "render missing preview", Input: "做一期90秒口播视频，讲AI Agent", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: renderWithoutPreview(voicePlan()), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: false, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "redundant script", Input: "做一期口播视频，讲内容生产", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: withExtraTool(voicePlan(), "script_again", "video_script_generator"), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: false, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "aigc missing prompt", Input: "做一个镜头式故事短片", ExpectedVideoType: intent.VideoTypeAIGCShot, Plan: withoutStep(aigcPlan(), "prompt"), ExpectedPlanGuardPassed: false, ExpectedPlanJudgePassed: false, ExpectedRequiredArtifactsComplete: false, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
		{Name: "voice visual default platform", Input: "帮我做一个观点口播，讲普通人怎么用AI", ExpectedVideoType: intent.VideoTypeVoiceVisual, Plan: voicePlan(), ExpectedPlanGuardPassed: true, ExpectedPlanJudgePassed: true, ExpectedRequiredArtifactsComplete: true, ExpectedForbiddenToolAbsent: true, ExpectedPublishCopyComplete: true},
	}
}

func voicePlan() *agentruntime.AgentPlan {
	return &agentruntime.AgentPlan{Goal: "voice visual", Domain: "video_creation", Steps: []agentruntime.AgentStep{
		{ID: "brief", Tool: "proposal_generator", Arguments: map[string]interface{}{"stage": "brief"}, ExpectedOutput: []string{"creative_brief"}},
		{ID: "script", Tool: "video_script_generator", DependsOn: []string{"brief"}, Arguments: map[string]interface{}{"stage": "script"}, ExpectedOutput: []string{"voiceover_script"}},
		{ID: "beat", Tool: "shot_splitter", DependsOn: []string{"script"}, Arguments: map[string]interface{}{"stage": "beat"}, ExpectedOutput: []string{"beat_plan", "visual_component_plan"}},
		{ID: "preview", Tool: "hyperframes_project_generator", DependsOn: []string{"beat"}, Arguments: map[string]interface{}{"stage": "preview"}, ExpectedOutput: []string{"hyperframes_project", "preview"}},
		{ID: "render", Tool: "hyperframes_renderer", DependsOn: []string{"preview"}, Arguments: map[string]interface{}{"stage": "render"}, ExpectedOutput: []string{"final_video"}},
		{ID: "publish", Tool: "publish_copy_generator", DependsOn: []string{"render"}, Arguments: map[string]interface{}{"stage": "publish"}, ExpectedOutput: []string{"publish_copy"}},
	}}
}

func aigcPlan() *agentruntime.AgentPlan {
	return &agentruntime.AgentPlan{Goal: "aigc shot", Domain: "video_creation", Steps: []agentruntime.AgentStep{
		{ID: "brief", Tool: "proposal_generator", Arguments: map[string]interface{}{"stage": "brief"}, ExpectedOutput: []string{"creative_brief", "story_outline"}},
		{ID: "script", Tool: "video_script_generator", DependsOn: []string{"brief"}, Arguments: map[string]interface{}{"stage": "script"}, ExpectedOutput: []string{"script", "character_bible", "scene_bible"}},
		{ID: "shot", Tool: "shot_splitter", DependsOn: []string{"script"}, Arguments: map[string]interface{}{"stage": "storyboard"}, ExpectedOutput: []string{"shot_list"}},
		{ID: "prompt", Tool: "video_prompt_generator", DependsOn: []string{"shot"}, Arguments: map[string]interface{}{"stage": "prompt"}, ExpectedOutput: []string{"keyframe_prompt", "video_prompt"}},
		{ID: "preview", Tool: "render_strategy_planner", DependsOn: []string{"prompt"}, Arguments: map[string]interface{}{"stage": "preview"}, ExpectedOutput: []string{"render_strategy", "preview"}},
		{ID: "render", Tool: "hyperframes_renderer", DependsOn: []string{"preview"}, Arguments: map[string]interface{}{"stage": "render"}, ExpectedOutput: []string{"final_video"}},
		{ID: "publish", Tool: "publish_copy_generator", DependsOn: []string{"render"}, Arguments: map[string]interface{}{"stage": "publish"}, ExpectedOutput: []string{"publish_copy"}},
	}}
}

func withoutPublishCopy(plan *agentruntime.AgentPlan) *agentruntime.AgentPlan {
	return withoutStep(plan, "publish")
}

func withoutStep(plan *agentruntime.AgentPlan, id string) *agentruntime.AgentPlan {
	copied := clonePlan(plan)
	steps := copied.Steps[:0]
	for _, step := range copied.Steps {
		if step.ID != id {
			steps = append(steps, step)
		}
	}
	copied.Steps = steps
	return copied
}

func renderWithoutPreview(plan *agentruntime.AgentPlan) *agentruntime.AgentPlan {
	copied := clonePlan(plan)
	for i := range copied.Steps {
		if copied.Steps[i].ID == "render" {
			copied.Steps[i].DependsOn = []string{"script"}
		}
	}
	return copied
}

func withExtraTool(plan *agentruntime.AgentPlan, id, toolName string) *agentruntime.AgentPlan {
	copied := clonePlan(plan)
	copied.Steps = append(copied.Steps, agentruntime.AgentStep{ID: id, Tool: toolName, Arguments: map[string]interface{}{"stage": id}})
	return copied
}

func clonePlan(plan *agentruntime.AgentPlan) *agentruntime.AgentPlan {
	copied := *plan
	copied.Steps = append([]agentruntime.AgentStep(nil), plan.Steps...)
	return &copied
}

func requiredArtifactsComplete(required []string, plan *agentruntime.AgentPlan) bool {
	for _, artifact := range required {
		if !planMentions(plan, artifact) {
			return false
		}
	}
	return true
}

func publishCopyComplete(plan *agentruntime.AgentPlan) bool {
	return planMentions(plan, "publish_copy") || planMentions(plan, "publish_copy_generator")
}

func forbiddenToolAbsent(plan *agentruntime.AgentPlan) bool {
	for _, step := range plan.Steps {
		toolName := strings.ToLower(step.Tool)
		for _, forbidden := range []string{"cosyvoice", "diffsinger", "imagebind", "fish-speech", "seed-vc", "videorag"} {
			if toolName == forbidden {
				return false
			}
		}
	}
	return true
}

func planMentions(plan *agentruntime.AgentPlan, term string) bool {
	term = strings.ToLower(term)
	for _, step := range plan.Steps {
		haystack := strings.ToLower(step.ID + " " + step.Tool + " " + step.Intent)
		for _, out := range step.ExpectedOutput {
			haystack += " " + strings.ToLower(out)
		}
		if strings.Contains(haystack, term) {
			return true
		}
	}
	return false
}

func evalCatalog() agentruntime.ToolCatalog {
	catalog := map[string]*tool.ToolManifest{}
	for _, name := range []string{
		"proposal_generator",
		"video_script_generator",
		"shot_splitter",
		"hyperframes_project_generator",
		"hyperframes_renderer",
		"publish_copy_generator",
		"video_prompt_generator",
		"render_strategy_planner",
		"CosyVoice",
	} {
		catalog[name] = &tool.ToolManifest{Name: name, Type: "builtin", Parameters: map[string]tool.ParamDef{}}
	}
	return staticCatalog(catalog)
}

type staticCatalog map[string]*tool.ToolManifest

func (c staticCatalog) GetManifest(name string) *tool.ToolManifest {
	return c[name]
}
