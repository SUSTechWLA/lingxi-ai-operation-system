package workflow

import (
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
