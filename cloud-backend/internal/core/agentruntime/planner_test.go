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

type staticToolList []tool.ToolManifest

func (l staticToolList) ListManifests() []*tool.ToolManifest {
	result := make([]*tool.ToolManifest, 0, len(l))
	for i := range l {
		result = append(result, &l[i])
	}
	return result
}
