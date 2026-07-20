package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestShotRegenerationPlanRejectsStepOutsideTarget(t *testing.T) {
	catalog := shotRegenerationCatalog()
	guard := NewPlanGuard(catalog, nil)
	plan := &AgentPlan{Domain: "video_creation", Steps: []AgentStep{
		{ID: "target", Tool: "shot_tool", Arguments: map[string]interface{}{
			"operation": "shot_regeneration", "targetShotId": "shot-012", "allowedShotIds": []string{"shot-012"}, "shotId": "shot-012",
		}},
		{ID: "escape", Tool: "shot_tool", Arguments: map[string]interface{}{
			"operation": "shot_regeneration", "targetShotId": "shot-012", "allowedShotIds": []string{"shot-012"}, "shotId": "shot-013",
		}},
	}}

	err := guard.ValidatePlan(context.Background(), "", plan)
	if err == nil || !strings.Contains(err.Error(), "shot regeneration plan escapes target shot shot-012") {
		t.Fatalf("error=%v", err)
	}
}

func TestShotRegenerationPlanAllowsReferenceFromTargetGuardedProducer(t *testing.T) {
	catalog := shotRegenerationCatalog()
	guard := NewPlanGuard(catalog, nil)
	plan := &AgentPlan{Domain: "video_creation", Steps: []AgentStep{
		{ID: "target", Tool: "shot_tool", Arguments: map[string]interface{}{
			"operation": "shot_regeneration", "targetShotId": "shot-012", "allowedShotIds": []string{"shot-012"}, "shotId": "shot-012",
		}, ExpectedOutput: []string{"shotId"}},
		{ID: "consume", Tool: "shot_tool", DependsOn: []string{"target"}, Arguments: map[string]interface{}{
			"operation": "shot_regeneration", "targetShotId": "shot-012", "allowedShotIds": []string{"shot-012"}, "shotId": "{{target.output.shotId}}",
		}},
	}}

	if err := guard.ValidatePlan(context.Background(), "", plan); err != nil {
		t.Fatalf("target-guarded reference rejected: %v", err)
	}
}

func TestShotRegenerationPlanDefaultsOverwriteEveryStep(t *testing.T) {
	plan := &AgentPlan{Domain: "video_creation", Steps: []AgentStep{
		{ID: "one", Tool: "shot_tool", Arguments: map[string]interface{}{"targetShotId": "shot-999"}},
		{ID: "two", Tool: "shot_tool", Arguments: nil},
	}}
	applyRequestPlanDefaults(plan, StartRunRequest{Domain: "video_creation", Context: map[string]interface{}{
		"operation": "shot_regeneration", "targetShotId": "shot-012", "shotRegenerationTaskId": "regen-task-1",
	}})
	for _, step := range plan.Steps {
		if step.Arguments["operation"] != "shot_regeneration" || step.Arguments["targetShotId"] != "shot-012" {
			t.Fatalf("step not target scoped: %+v", step)
		}
		allowed, ok := step.Arguments["allowedShotIds"].([]string)
		if !ok || len(allowed) != 1 || allowed[0] != "shot-012" {
			t.Fatalf("allowed shots = %#v", step.Arguments["allowedShotIds"])
		}
		if step.Arguments["shotRegenerationTaskId"] != "regen-task-1" {
			t.Fatalf("durable task id missing from step: %+v", step)
		}
	}
}

func shotRegenerationCatalog() staticToolCatalog {
	return staticToolCatalog{"shot_tool": &tool.ToolManifest{
		Name: "shot_tool", Endpoint: "builtin://shot-tool", Output: map[string]tool.ParamDef{"shotId": {Type: "string"}},
	}}
}
