package builtin

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestExternalToolExecutesRegisteredLocalVideoCreationTool(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	ext := NewExternalTool(registry)
	result := ext.Execute(context.Background(), map[string]interface{}{
		"tool":            "skill_stage_agent",
		"skill_name":      "create-opinion-videos",
		"stage":           "viewpoint_dossier",
		"brief":           "做一条 60 秒口播视频",
		"instruction_ref": "skills/create-opinion-videos/1.0.0/stages/viewpoint_dossier.md",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "viewpoint_dossier"})

	if !result.Success {
		t.Fatalf("expected local external tool success, got %s", result.Error)
	}
	if result.Data["content"] == "" {
		t.Fatalf("expected reviewable content in result: %+v", result.Data)
	}
	if result.Data["publishCopy"] == nil {
		t.Fatalf("expected publish copy draft in result: %+v", result.Data)
	}
}
