package artifact

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestBuildArtifactRecordPromotesBetaIndexMetadata(t *testing.T) {
	record := buildArtifactRecord(&CreateArtifactRequest{
		ProjectID:     "project-1",
		WorkflowRunID: "run-1",
		TaskID:        "task-1",
		StageName:     "script",
		RoleAgentID:   "script_writer",
		UnitID:        "script-content",
		Kind:          KindMarkdown,
		Name:          "视频脚本",
		Data:          []byte("脚本内容"),
		Provider:      "workflow-node",
		Metadata: map[string]interface{}{
			"status":         "stale",
			"humanApproved":  true,
			"dependsOn":      []interface{}{"VIDEO_PROPOSAL"},
			"producedByNode": "script_exec",
			"producedByTool": "video_script_generator",
			"producedByRole": "脚本编剧",
		},
	}, 1, "")

	if record.TaskID != "task-1" || record.RoleAgentID != "script_writer" {
		t.Fatalf("artifact should promote task/role fields: %+v", record)
	}
	if record.Status != "stale" || !record.HumanApproved {
		t.Fatalf("artifact should promote status and human approval: %+v", record)
	}
	if len(record.DependsOn) != 1 || record.DependsOn[0] != "VIDEO_PROPOSAL" {
		t.Fatalf("artifact should promote dependency list: %+v", record.DependsOn)
	}
	if record.ProducedByNode != "script_exec" || record.ProducedByTool != "video_script_generator" || record.ProducedByRole != "脚本编剧" {
		t.Fatalf("artifact should promote producer metadata: %+v", record)
	}
}

func TestBuildArtifactRequestsFromNodeCarriesBetaIndexMetadata(t *testing.T) {
	node := &model.Node{
		ID:     "script_exec",
		TaskID: "task-1",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage":       "script",
			"roleAgentId": "script_writer",
			"roleAgent": map[string]interface{}{
				"displayName": "脚本编剧",
			},
			"requiredInputs": []interface{}{"VIDEO_PROPOSAL"},
		},
		Output: map[string]interface{}{
			"stdout": `{
				"script":"脚本内容",
				"artifacts":[{"unitId":"script-content","kind":"MARKDOWN","name":"视频脚本","mimeType":"text/markdown"}]
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("project-1", "run-1", node)
	if len(requests) != 1 {
		t.Fatalf("expected one artifact request, got %d", len(requests))
	}
	req := requests[0]
	if req.TaskID != "task-1" || req.RoleAgentID != "script_writer" {
		t.Fatalf("request should carry task and role agent: %+v", req)
	}
	if req.Metadata["status"] != "valid" || req.Metadata["humanApproved"] != false {
		t.Fatalf("request should default beta index state: %+v", req.Metadata)
	}
	if req.Metadata["producedByNode"] != "script_exec" || req.Metadata["producedByTool"] != "external" || req.Metadata["producedByRole"] != "脚本编剧" {
		t.Fatalf("request should carry producer metadata: %+v", req.Metadata)
	}
}

func TestDownstreamStaleArtifactKinds(t *testing.T) {
	// DownstreamStaleArtifactKinds now returns stage_name values (matching the
	// artifacts.stage_name column) for use with MarkStaleByStageNames.
	got := DownstreamStaleArtifactKinds("VIDEO_SCRIPT")
	want := []string{"storyboard", "composition", "reference", "continuity", "preview", "render", "quality", "package"}
	if len(got) != len(want) {
		t.Fatalf("downstream len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("downstream[%d] = %s, want %s; all=%#v", i, got[i], want[i], got)
		}
	}
}
