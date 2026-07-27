package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestWorkflowFailuresNormalizeStableCodesThroughEmitter(t *testing.T) {
	sink := &workflowEventSink{}
	emitter := observability.NewEmitter(observability.Source{Service: "cloud", Component: "workflow", Environment: "test"}, observability.Runtime{}, sink, 8)
	service := (&RunService{runRepo: newRunRepositoryWithDB(&fakeWorkflowRunDB{})}).WithObservability(emitter)
	if err := service.UpdateRunStatus(context.Background(), "wfr-failed", RunFailed); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateStageStatus(context.Background(), "wfr-failed", "render", StageFailed); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[observability.EventType]string{
		observability.EventTypeWorkflowRunFailed:   "WORKFLOW.RUN.EXECUTION_FAILED",
		observability.EventTypeWorkflowStageFailed: "WORKFLOW.STAGE.EXECUTION_FAILED",
	}
	for _, event := range sink.snapshot() {
		if err := event.Validate(); err != nil {
			t.Fatalf("invalid workflow event %s: %v", event.EventType, err)
		}
		if event.Error == nil || event.Error.Code != want[event.EventType] {
			t.Fatalf("event %s error=%+v", event.EventType, event.Error)
		}
		delete(want, event.EventType)
	}
	if len(want) != 0 {
		t.Fatalf("missing workflow failure events: %+v", want)
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

type workflowTemplateStoreFake struct {
	template *Template
}

func (s *workflowTemplateStoreFake) FindByID(context.Context, string) (*Template, error) {
	return s.template, nil
}

type workflowRunStoreFake struct {
	created  *WorkflowRun
	sequence *[]string
}

func (s *workflowRunStoreFake) Create(_ context.Context, run *WorkflowRun) error {
	*s.sequence = append(*s.sequence, "run")
	s.created = run
	return nil
}
func (*workflowRunStoreFake) FindByID(context.Context, string) (*WorkflowRun, error) {
	return nil, nil
}
func (*workflowRunStoreFake) FindByProject(context.Context, string) ([]*WorkflowRun, error) {
	return nil, nil
}
func (*workflowRunStoreFake) UpdateStatus(context.Context, string, RunStatus) error {
	return nil
}
func (*workflowRunStoreFake) UpdateStageStatus(context.Context, string, string, StageStatus) error {
	return nil
}

type workflowOrchestratorFake struct {
	taskInput map[string]interface{}
	dag       *model.DAGRequest
	sequence  *[]string
}

func (o *workflowOrchestratorFake) CreateTask(_ context.Context, userID string, input map[string]interface{}) (*model.Task, error) {
	*o.sequence = append(*o.sequence, "task")
	o.taskInput = input
	return &model.Task{ID: "task-1", UserID: userID, Status: model.TaskCreated, CreatedAt: time.Now()}, nil
}
func (o *workflowOrchestratorFake) SubmitDAG(_ context.Context, _ string, dag *model.DAGRequest) error {
	*o.sequence = append(*o.sequence, "dag")
	o.dag = dag
	return nil
}

type workflowToolSnapshotProviderFake struct {
	snapshot ToolRegistrySnapshot
	sequence *[]string
	calls    int
}

func (p *workflowToolSnapshotProviderFake) Snapshot(context.Context) (ToolRegistrySnapshot, error) {
	p.calls++
	*p.sequence = append(*p.sequence, "snapshot")
	return p.snapshot, nil
}

func TestCreateRunFreezesRealToolSnapshotBeforeTaskAndUsesItForExecutionAndManifest(t *testing.T) {
	sequence := []string{}
	canonical := json.RawMessage(`[{"name":"alpha","version":"1"},{"name":"zeta","version":"2"}]`)
	digest := sha256.Sum256(canonical)
	hash := hex.EncodeToString(digest[:])
	provider := &workflowToolSnapshotProviderFake{
		snapshot: ToolRegistrySnapshot{
			ID:            "tool_snapshot_" + hash,
			SHA256:        hash,
			CanonicalJSON: canonical,
		},
		sequence: &sequence,
	}
	expected := provider.snapshot
	templates := &workflowTemplateStoreFake{template: &Template{
		ID: "template-1", Version: "7",
		DAG: json.RawMessage(`{"nodes":[{"id":"stage-1","type":"TOOL","name":"external","input":{"parameters":{"tool":"alpha"}}}],"edges":[]}`),
	}}
	runs := &workflowRunStoreFake{sequence: &sequence}
	orch := &workflowOrchestratorFake{sequence: &sequence}
	service := (&RunService{
		repo: templates, runRepo: runs, orchService: orch,
	}).WithToolRegistrySnapshotProvider(provider)
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
	})

	run, err := service.CreateRun(ctx, "user-1", "project-1", "template-1", "7", map[string]interface{}{"brief": "safe"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || strings.Join(sequence, ",") != "snapshot,task,dag,run" {
		t.Fatalf("snapshot calls=%d sequence=%v", provider.calls, sequence)
	}
	if run.ToolRegistrySnapshotID != expected.ID ||
		run.RunManifest == nil ||
		run.RunManifest.ToolRegistrySnapshotID != expected.ID ||
		run.RunManifest.ToolRegistrySHA256 != expected.SHA256 ||
		runs.created != run {
		t.Fatalf("workflow run snapshot/manifest=%+v expected=%+v", run, expected)
	}
	for _, metadata := range []map[string]interface{}{orch.taskInput, orch.dag.Nodes[0].Input} {
		if metadata["toolRegistrySnapshotId"] != expected.ID ||
			metadata["toolRegistrySha256"] != expected.SHA256 {
			t.Fatalf("execution metadata=%+v, expected snapshot=%+v", metadata, expected)
		}
	}
	parameters, _ := orch.dag.Nodes[0].Input["parameters"].(map[string]interface{})
	if parameters["toolRegistrySnapshotId"] != expected.ID ||
		parameters["toolRegistrySha256"] != expected.SHA256 {
		t.Fatalf("execution parameters=%+v, expected snapshot=%+v", parameters, expected)
	}
}

func TestCreateRunRequiresSnapshotProviderBeforeCreatingTask(t *testing.T) {
	sequence := []string{}
	service := &RunService{
		repo: &workflowTemplateStoreFake{template: &Template{
			ID:  "template-1",
			DAG: json.RawMessage(`{"nodes":[],"edges":[]}`),
		}},
		runRepo:     &workflowRunStoreFake{sequence: &sequence},
		orchService: &workflowOrchestratorFake{sequence: &sequence},
	}

	if _, err := service.CreateRun(context.Background(), "user-1", "project-1", "template-1", "1", nil); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "snapshot") {
		t.Fatalf("CreateRun error=%v, want missing snapshot provider", err)
	}
	if len(sequence) != 0 {
		t.Fatalf("side effects occurred before snapshot validation: %v", sequence)
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
