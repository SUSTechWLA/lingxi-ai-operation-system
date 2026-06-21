package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/auth"
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

func TestWorkflowRunUserIDFromContext(t *testing.T) {
	ctx := auth.ContextWithUser(context.Background(), "u_auth")
	if got := workflowRunUserID(ctx); got != "u_auth" {
		t.Fatalf("workflowRunUserID = %q, want authenticated user", got)
	}
}

func TestWorkflowRunUserIDDefaultsWhenUnauthenticated(t *testing.T) {
	if got := workflowRunUserID(context.Background()); got != "default" {
		t.Fatalf("workflowRunUserID = %q, want default", got)
	}
}
