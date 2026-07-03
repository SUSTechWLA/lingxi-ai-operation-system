package agentruntime

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestHeuristicPlanner_SelectsCapabilityToolsForDomain(t *testing.T) {
	planner := NewHeuristicPlanner(staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
		{Name: "shot_splitter", Capabilities: []string{"video_creation", "storyboard_generation"}},
		{Name: "platform_adapter", Capabilities: []string{"publish"}},
	})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我做一个小红书视频",
		Domain:  "video_creation",
		Context: map[string]interface{}{"platform": "小红书"},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected two video steps, got %#v", plan.Steps)
	}
	if plan.Steps[0].Tool != "video_script_generator" || plan.Steps[1].Tool != "shot_splitter" {
		t.Fatalf("unexpected tool order: %#v", plan.Steps)
	}
	if len(plan.Steps[1].DependsOn) != 1 || plan.Steps[1].DependsOn[0] != plan.Steps[0].ID {
		t.Fatalf("steps should be linear by default: %#v", plan.Steps)
	}
	if plan.Steps[0].Arguments["platform"] != "小红书" {
		t.Fatalf("request context not copied into arguments: %#v", plan.Steps[0].Arguments)
	}
}

func TestHeuristicPlanner_WiresRequiredInputsFromPreviousOutputs(t *testing.T) {
	tools := staticToolList{
		{
			Name:         "video_script_generator",
			Capabilities: []string{"video_creation", "script_generation"},
			Parameters: map[string]tool.ParamDef{
				"topic": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		{
			Name:         "shot_splitter",
			Capabilities: []string{"video_creation", "shot_split"},
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
	}
	planner := NewHeuristicPlanner(tools)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我做一个30秒视频，讲端午节和粽子的来历。",
		Domain:  "video_creation",
		Context: map[string]interface{}{"targetDurationSec": 30},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected two video steps, got %#v", plan.Steps)
	}
	if got := plan.Steps[1].Arguments["script"]; got != "{{video_script_generator.output.script}}" {
		t.Fatalf("shot_splitter should reference generated script, got %#v", plan.Steps[1].Arguments)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("wired fallback plan should pass PlanGuard: %v", err)
	}
}

func TestHeuristicPlanner_DomainFilterRecomputesLimit(t *testing.T) {
	planner := NewHeuristicPlannerWithMaxTools(staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
		{Name: "generic_video_tool", Capabilities: []string{"general"}, Tags: []string{"video"}},
		{Name: "generic_video_tool_2", Capabilities: []string{"general"}, Tags: []string{"video"}},
	}, 6)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "做一个 video",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "video_script_generator" {
		t.Fatalf("domain filter should keep only matching capability tools: %#v", plan.Steps)
	}
}

func TestHeuristicPlanner_OrdersVideoForgePipelineBeforeGeneration(t *testing.T) {
	planner := NewHeuristicPlannerWithMaxTools(staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
		{Name: "shot_splitter", Capabilities: []string{"video_creation", "storyboard_generation"}},
		{Name: "pipeline_selector", Capabilities: []string{"video_creation", "pipeline_selection", "videoforge_studio"}},
		{Name: "proposal_generator", Capabilities: []string{"video_creation", "proposal_generation", "videoforge_studio"}},
		{Name: "capability_preflight", Capabilities: []string{"video_creation", "capability_preflight", "videoforge_studio"}},
		{Name: "visual_feasibility_analyzer", Capabilities: []string{"video_creation", "visual_feasibility", "render_strategy"}},
		{Name: "render_strategy_planner", Capabilities: []string{"video_creation", "render_strategy", "dual_engine"}},
	}, 7)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	want := []string{
		"pipeline_selector",
		"capability_preflight",
		"proposal_generator",
		"video_script_generator",
		"shot_splitter",
		"visual_feasibility_analyzer",
		"render_strategy_planner",
	}
	if len(plan.Steps) != len(want) {
		t.Fatalf("expected %d steps, got %#v", len(want), plan.Steps)
	}
	for i, step := range plan.Steps {
		if step.Tool != want[i] {
			t.Fatalf("step %d should be %s, got %#v", i, want[i], plan.Steps)
		}
		if i > 0 && (len(step.DependsOn) != 1 || step.DependsOn[0] != plan.Steps[i-1].ID) {
			t.Fatalf("VideoForge fallback plan should be linear through approval gates: %#v", plan.Steps)
		}
	}
}

func TestHeuristicPlanner_PreparesProfilePlanBeforeValidation(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	catalog["video_prompt_generator"].Output["externalGenerationRequests"] = tool.ParamDef{Type: "array"}
	catalog["video_prompt_generator"].Parameters["aigcProvider"] = tool.ParamDef{Type: "string", Required: false}
	catalog["hyperframes_project_generator"].Parameters["shotAssetPackages"] = tool.ParamDef{Type: "array", Required: false}
	catalog["mcp_generation_runner"] = &tool.ToolManifest{
		Name:           "mcp_generation_runner",
		ExecutionPlane: tool.ExecutionPlaneLocal,
		LocalCommand:   "LOCAL_MCP_TOOL_CALL",
		Parameters: map[string]tool.ParamDef{
			"externalGenerationRequests": {Type: "array", Required: true},
			"providerId":                 {Type: "string", Required: true},
			"mcpTool":                    {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"shotAssetPackages": {Type: "array"},
			"generationResults": {Type: "array"},
		},
	}
	tools := staticToolList{
		cloneManifestForPlanner(catalog["video_script_generator"], "script_generation"),
		cloneManifestForPlanner(catalog["shot_splitter"], "shot_split"),
		cloneManifestForPlanner(catalog["time_window_planner"], "time_window_planning"),
		cloneManifestForPlanner(catalog["visual_alignment_planner"], "visual_alignment"),
		cloneManifestForPlanner(catalog["shot_generation_planner"], "shot_planning"),
		cloneManifestForPlanner(catalog["video_prompt_generator"], "video_prompt_generation"),
		cloneManifestForPlanner(catalog["video_profile_classifier"], "profile_selection"),
		cloneManifestForPlanner(catalog["hyperframes_project_generator"], "composition_generation"),
		cloneManifestForPlanner(catalog["hyperframes_renderer"], "video_render"),
		cloneManifestForPlanner(catalog["publish_copy_generator"], "publish_copy"),
		cloneManifestForPlanner(catalog["mcp_generation_runner"], "aigc_generation"),
	}
	planner := NewHeuristicPlannerWithMaxTools(tools, len(tools))

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我制作一条宣传躺营 AI OS 的 30 秒视频",
		Domain:  "video_creation",
		Context: map[string]interface{}{
			"profileId":         "voice_visual",
			"targetDurationSec": 30,
			"videoType":         "voice_visual",
			"aigcProvider":      "jimeng_mcp",
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}

	if got := plan.Steps[0].ID; got != "profile_selection" {
		t.Fatalf("profile classifier should be canonicalized first, got %#v", plan.Steps)
	}
	timeWindow := findStep(t, plan, "time_window")
	if got := timeWindow.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("time_window creationProfile = %#v", got)
	}
	if got := timeWindow.Arguments["scriptSpans"]; got != "{{script_generation.output.scriptSpans}}" {
		t.Fatalf("time_window scriptSpans = %#v", got)
	}
	shotSplit := findStep(t, plan, "shot_split")
	if got := shotSplit.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("shot_split script = %#v", got)
	}
	mcpStep := findStep(t, plan, "mcp_generation")
	if got := mcpStep.Arguments["externalGenerationRequests"]; got != "{{video_prompt.output.externalGenerationRequests}}" {
		t.Fatalf("mcp_generation externalGenerationRequests = %#v", got)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("prepared heuristic plan should pass PlanGuard: %v", err)
	}
}

func TestHeuristicPlanner_PreparesCinematicProfileWithSelectedKeyframes(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	catalog["video_prompt_generator"].Output["externalGenerationRequests"] = tool.ParamDef{Type: "array"}
	catalog["video_prompt_generator"].Parameters["aigcProvider"] = tool.ParamDef{Type: "string", Required: false}
	catalog["hyperframes_project_generator"].Parameters["shotAssetPackages"] = tool.ParamDef{Type: "array", Required: false}
	catalog["mcp_generation_runner"] = &tool.ToolManifest{
		Name:           "mcp_generation_runner",
		ExecutionPlane: tool.ExecutionPlaneLocal,
		LocalCommand:   "LOCAL_MCP_TOOL_CALL",
		Parameters: map[string]tool.ParamDef{
			"externalGenerationRequests": {Type: "array", Required: true},
			"providerId":                 {Type: "string", Required: true},
			"mcpTool":                    {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"shotAssetPackages": {Type: "array"},
			"generationResults": {Type: "array"},
		},
	}
	tools := staticToolList{
		{Name: "knowledge_researcher", Capabilities: []string{"video_creation", "fresh_knowledge"}, Output: map[string]tool.ParamDef{"facts": {Type: "array"}, "sources": {Type: "array"}}},
		cloneManifestForPlanner(catalog["proposal_generator"], "proposal_generation"),
		cloneManifestForPlanner(catalog["video_script_generator"], "script_generation"),
		cloneManifestForPlanner(catalog["continuity_checker"], "continuity"),
		cloneManifestForPlanner(catalog["reference_asset_planner"], "reference_assets"),
		cloneManifestForPlanner(catalog["cinematic_shot_designer"], "shot_planning"),
		cloneManifestForPlanner(catalog["time_window_planner"], "time_window_planning"),
		cloneManifestForPlanner(catalog["keyframe_prompt_generator"], "keyframe_generation"),
		cloneManifestForPlanner(catalog["shot_generation_planner"], "shot_planning"),
		cloneManifestForPlanner(catalog["video_prompt_generator"], "video_prompt_generation"),
		cloneManifestForPlanner(catalog["video_profile_classifier"], "profile_selection"),
		cloneManifestForPlanner(catalog["hyperframes_project_generator"], "composition_generation"),
		cloneManifestForPlanner(catalog["hyperframes_renderer"], "video_render"),
		cloneManifestForPlanner(catalog["publish_copy_generator"], "publish_copy"),
		cloneManifestForPlanner(catalog["mcp_generation_runner"], "aigc_generation"),
	}
	planner := NewHeuristicPlannerWithMaxTools(tools, len(tools))

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我制作一条开源项目上线宣传片",
		Domain:  "video_creation",
		Context: map[string]interface{}{
			"profileId":         "aigc_shot",
			"targetDurationSec": 120,
			"videoType":         "aigc_shot",
			"aigcProvider":      "jimeng_mcp",
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}

	keyframes := findStep(t, plan, "keyframes_storyboards")
	if keyframes.Tool != "keyframe_prompt_generator" {
		t.Fatalf("keyframes step tool = %s", keyframes.Tool)
	}
	if got := keyframes.Arguments["shotList"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("keyframes shotList = %#v, want time window output", got)
	}
	if got := keyframes.Arguments["referenceAssetPlan"]; got != "{{reference_assets.output.referenceAssetPlan}}" {
		t.Fatalf("keyframes reference assets = %#v", got)
	}
	if raw := planStepByTool(plan, "keyframe_prompt_generator"); raw != nil && raw.ID != "keyframes_storyboards" {
		t.Fatalf("unexpected unprepared keyframe step left in plan: %#v", raw)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("prepared cinematic heuristic plan should pass PlanGuard: %v", err)
	}
}

type staticToolList []tool.ToolManifest

func (l staticToolList) ListManifests() []*tool.ToolManifest {
	result := make([]*tool.ToolManifest, 0, len(l))
	for i := range l {
		result = append(result, &l[i])
	}
	return result
}

func (l staticToolList) GetManifest(name string) *tool.ToolManifest {
	for i := range l {
		if l[i].Name == name {
			return &l[i]
		}
	}
	return nil
}

func cloneManifestForPlanner(manifest *tool.ToolManifest, capability string) tool.ToolManifest {
	clone := *manifest
	clone.Capabilities = append([]string{"video_creation"}, capability)
	return clone
}
