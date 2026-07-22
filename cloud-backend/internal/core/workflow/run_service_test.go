package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestJSONUnmarshalAcceptsRawMessageDAG(t *testing.T) {
	raw := json.RawMessage(`{"nodes":[{"id":"brief","type":"TOOL","name":"external"}],"edges":[]}`)

	var dag model.DAGRequest
	if err := jsonUnmarshal(raw, &dag); err != nil {
		t.Fatalf("expected json.RawMessage DAG to unmarshal, got %v", err)
	}
	if len(dag.Nodes) != 1 || dag.Nodes[0].ID != "brief" {
		t.Fatalf("unexpected DAG nodes: %+v", dag.Nodes)
	}
}

func TestCreateRunFailsClosedWithoutAuthenticatedUser(t *testing.T) {
	if _, err := (&RunService{}).CreateRun(context.Background(), "", "project", "template", "1", nil); err == nil {
		t.Fatal("workflow run task creation must require an explicit authenticated owner")
	}
}

func TestApplyRunInputToDAGPropagatesBriefIntoExternalToolParameters(t *testing.T) {
	dag := model.DAGRequest{
		Nodes: []model.NodeRequest{
			{
				ID:   "viewpoint_dossier",
				Type: string(model.NodeTypeTool),
				Name: "external",
				Input: map[string]interface{}{
					"tool":  "skill_stage_agent",
					"stage": "viewpoint_dossier",
					"parameters": map[string]interface{}{
						"tool":  "skill_stage_agent",
						"stage": "viewpoint_dossier",
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"brief":               "把端午节和粽子的来源做成 60 秒口播知识视频",
		"target_duration_sec": 60,
		"expected_output":     "publish_pack",
	}

	applyRunInputToDAG(&dag, input)

	nodeInput := dag.Nodes[0].Input
	if nodeInput["brief"] != input["brief"] {
		t.Fatalf("node input brief = %v, want %v", nodeInput["brief"], input["brief"])
	}
	params, ok := nodeInput["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters map, got %+v", nodeInput["parameters"])
	}
	for _, key := range []string{"brief", "target_duration_sec", "expected_output"} {
		if params[key] != input[key] {
			t.Fatalf("parameter %s = %v, want %v", key, params[key], input[key])
		}
	}
}
