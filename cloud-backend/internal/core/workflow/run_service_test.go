package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type workflowEventSink struct {
	mu     sync.Mutex
	events []observability.Event
}

func (s *workflowEventSink) Write(_ context.Context, event observability.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (*workflowEventSink) Close(context.Context) error { return nil }

func (s *workflowEventSink) snapshot() []observability.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]observability.Event(nil), s.events...)
}

func TestUpdateStageStatusEmitsSingleMappedTransition(t *testing.T) {
	sink := &workflowEventSink{}
	emitter := observability.NewEmitter(
		observability.Source{Service: "cloud-backend", Component: "workflow", Environment: "test"},
		observability.Runtime{},
		sink,
		8,
	)
	service := (&RunService{runRepo: newRunRepositoryWithDB(&fakeWorkflowRunDB{})}).
		WithObservability(emitter)
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
	})
	if err := service.UpdateStageStatus(ctx, "wfr-domain-id", "render-stage", StageRunning); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateStageStatus(ctx, "wfr-domain-id", "render-stage", StageSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := sink.snapshot()
	if len(events) != 2 ||
		events[0].EventType != observability.EventTypeWorkflowStageStarted ||
		events[1].EventType != observability.EventTypeWorkflowStageCompleted {
		t.Fatalf("stage transitions=%+v", events)
	}
}

func TestDecisionLogSaveRejectsMissingWorkflowRunID(t *testing.T) {
	store := &pgxDecisionLogStore{}
	err := store.Save(context.Background(), &DecisionLogRecord{TaskID: "task-1", DecisionType: DecisionStageApproval})
	if err == nil || !strings.Contains(err.Error(), "workflowRunId") {
		t.Fatalf("Save error=%v, want missing workflowRunId", err)
	}
}

type decisionRunResolver struct {
	runID string
	err   error
}

func (r decisionRunResolver) FindRunIDByTaskID(context.Context, string) (string, error) {
	return r.runID, r.err
}

func TestDecisionLogExplicitResolverReturnsWorkflowRunID(t *testing.T) {
	store := &pgxDecisionLogStore{resolver: decisionRunResolver{runID: "wfr-resolved"}}
	runID, err := store.resolveWorkflowRunID(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "wfr-resolved" {
		t.Fatalf("resolved run ID=%q", runID)
	}
}

func TestDecisionLogExplicitResolverRejectsEmptyResolution(t *testing.T) {
	store := &pgxDecisionLogStore{resolver: decisionRunResolver{}}
	if _, err := store.resolveWorkflowRunID(context.Background(), "task-1"); err == nil ||
		!strings.Contains(err.Error(), "workflowRunId") {
		t.Fatalf("resolver error=%v, want missing workflowRunId", err)
	}
}

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
