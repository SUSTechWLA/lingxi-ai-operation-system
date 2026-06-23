package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestStageStatusKeyUsesOriginalAgentNodeID(t *testing.T) {
	node := &model.Node{
		ID:     "t1234567890-script_generation_exec",
		TaskID: "20260623152045-a3f2",
		Input:  map[string]interface{}{"agentOriginalNodeId": "script_generation_exec"},
	}
	if got := stageStatusKey(node); got != "script_generation_exec" {
		t.Fatalf("stageStatusKey = %q, want script_generation_exec", got)
	}
}

func TestStageStatusKeyStripsLegacyTaskPrefix(t *testing.T) {
	node := &model.Node{ID: "task-abc-script_generation_exec", TaskID: "task-abc"}
	if got := stageStatusKey(node); got != "script_generation_exec" {
		t.Fatalf("stageStatusKey = %q, want script_generation_exec", got)
	}
}

func TestStageStatusKeyKeepsWorkflowNodeID(t *testing.T) {
	node := &model.Node{ID: "script_generation_exec", TaskID: "task-abc"}
	if got := stageStatusKey(node); got != "script_generation_exec" {
		t.Fatalf("stageStatusKey = %q, want script_generation_exec", got)
	}
}
