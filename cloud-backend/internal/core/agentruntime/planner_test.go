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

func TestHeuristicPlanner_BidWritingUsesSnakeCaseFilePath(t *testing.T) {
	tools := staticToolList{
		{
			Name:         "parse_bid_files",
			Capabilities: []string{"bid_writing", "bid_parsing", "document_parsing"},
			Parameters: map[string]tool.ParamDef{
				"file_path": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"stdout": {Type: "string"},
			},
		},
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
	}
	planner := NewHeuristicPlanner(tools)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "please parse the tender document",
		Domain:  "bid_writing",
		Context: map[string]interface{}{"file_path": "E:\\bid\\sample.docx"},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "parse_bid_files" {
		t.Fatalf("expected parse_bid_files step, got %#v", plan.Steps)
	}
	if got := plan.Steps[0].Arguments["file_path"]; got != "E:\\bid\\sample.docx" {
		t.Fatalf("file_path should be copied into arguments, got %#v", plan.Steps[0].Arguments)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("bid writing plan should pass PlanGuard: %v", err)
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
