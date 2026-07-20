package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestReviewMutationReopenChangesOnlyGateAndLeavesSourceSuccessful(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{
		{ID: "script-exec", TaskID: "task-1", Type: model.NodeTypeTool, Status: model.NodeSuccess},
		{ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{
			"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v3",
		}},
		{ID: "other-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{
			"sourceNode": "other-exec", "stage": "preview", "artifactId": "preview-v1",
		}},
	}}
	state := &recordingReviewStateMachine{}
	dispatcher := &recordingRegenerationDispatcher{}
	log := &mutationDecisionLog{}
	atomic := &mutationAtomicReopener{}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, state).
		WithDecisionLogWriter(log).
		WithRegenerationDispatcher(dispatcher).
		WithAtomicReviewReopener(atomic)

	if err := svc.ReopenWithArtifact(context.Background(), "run-1", "script-review", "vp-1", "script-v3", "script-v4", "user-1", "修改后重新确认"); err != nil {
		t.Fatalf("ReopenWithArtifact() error = %v", err)
	}
	if nodes.nodes[0].Status != model.NodeSuccess {
		t.Fatalf("source status = %s, want SUCCESS", nodes.nodes[0].Status)
	}
	if len(atomic.requests) != 1 || atomic.requests[0].ExpectedArtifactID != "script-v3" || atomic.requests[0].NewArtifactID != "script-v4" {
		t.Fatalf("atomic request = %+v", atomic.requests)
	}
	if nodes.nodes[2].Status != model.NodeSuccess || nodes.nodes[2].Input["artifactId"] != "preview-v1" {
		t.Fatalf("unrelated gate changed: %+v", nodes.nodes[2])
	}
	if state.successNodeID != "" || dispatcher.resumedTaskID != "" || dispatcher.retriedNodeID != "" {
		t.Fatalf("reopen resumed work: state=%q resume=%q retry=%q", state.successNodeID, dispatcher.resumedTaskID, dispatcher.retriedNodeID)
	}
	if len(log.records) != 0 {
		t.Fatalf("decision audit must be owned by atomic store: %+v", log.records)
	}
}

func TestReviewMutationAtomicFailureLeavesInMemoryGateAndAuditUnchanged(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	node := &model.Node{ID: "review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess,
		Input: map[string]interface{}{"sourceNode": "source", "stage": "script", "artifactId": "v1"}}
	nodes := &mutationNodeStore{nodes: []*model.Node{node}}
	wantErr := errors.New("transaction failed")
	atomic := &mutationAtomicReopener{err: wantErr}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, &recordingReviewStateMachine{}).
		WithAtomicReviewReopener(atomic)

	err := svc.ReopenWithArtifact(context.Background(), "run-1", "review", "vp-1", "v1", "v2", "user", "edit")
	if !errors.Is(err, wantErr) || node.Status != model.NodeSuccess || node.Input["artifactId"] != "v1" {
		t.Fatalf("error=%v node=%+v", err, node)
	}
}

func TestReviewMutationReopenRetryUsesStableAuditIdentity(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{{ID: "review", TaskID: "task-1", Type: model.NodeTypeReviewGate,
		Status: model.NodeReady, Input: map[string]interface{}{"sourceNode": "source", "stage": "script", "artifactId": "v2"}}}}
	atomic := &mutationAtomicReopener{}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, nil).WithAtomicReviewReopener(atomic)
	for i := 0; i < 2; i++ {
		if err := svc.ReopenWithArtifact(context.Background(), "run-1", "review", "vp-1", "v1", "v2", "user", "edit"); err != nil {
			t.Fatal(err)
		}
	}
	if len(atomic.requests) != 2 || atomic.requests[0].AuditID == "" || atomic.requests[0].AuditID != atomic.requests[1].AuditID {
		t.Fatalf("requests=%+v", atomic.requests)
	}
}

func TestReviewMutationConfirmForArtifactRejectsSuccessfulWrongArtifact(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1"}
	nodes := &mutationNodeStore{nodes: []*model.Node{{ID: "review", TaskID: "task-1", Type: model.NodeTypeReviewGate,
		Status: model.NodeSuccess, Input: map[string]interface{}{"artifactId": "v1"}}}}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, nil)
	if err := svc.ConfirmForArtifact(context.Background(), "run-1", "review", "v2", "user", ""); !errors.Is(err, ErrReviewReferenceMismatch) {
		t.Fatalf("error=%v", err)
	}
}

func TestReviewMutationConfirmIsOnlyResumeTransitionAndIsIdempotentAfterSuccess(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{{
		ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeReady,
		Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v4"},
	}}}
	state := &recordingReviewStateMachine{}
	artifacts := &recordingArtifactService{}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, state).WithArtifactService(artifacts)

	if err := svc.Confirm(context.Background(), "run-1", "script-review", "user-1", "确认新版"); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if state.successNodeID != "script-review" || artifacts.approvedID != "script-v4" {
		t.Fatalf("confirm transition=%q approved=%q", state.successNodeID, artifacts.approvedID)
	}
	state.successNodeID = ""
	nodes.nodes[0].Status = model.NodeSuccess
	if err := svc.Confirm(context.Background(), "run-1", "script-review", "user-1", "retry"); err != nil {
		t.Fatalf("repeated Confirm() error = %v", err)
	}
	if state.successNodeID != "" {
		t.Fatalf("repeated confirm transitioned state again: %q", state.successNodeID)
	}
}

func TestReviewMutationResolverUsesArtifactRunTaskAndUniqueSource(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	runs.runs["run-2"] = &Run{ID: "run-2", TaskID: "task-2", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{
		{ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v3"}},
		{ID: "unrelated-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{"sourceNode": "other-exec", "stage": "script"}},
		{ID: "foreign-review", TaskID: "task-2", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v3"}},
	}}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, &recordingReviewStateMachine{})
	current := &artifact.Artifact{ID: "script-v3", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", ProducedByNode: "script-exec"}

	runID, reviewID, err := svc.ResolveReviewGate(context.Background(), current, "", "")
	if err != nil || runID != "run-1" || reviewID != "script-review" {
		t.Fatalf("ResolveReviewGate() = (%q,%q,%v)", runID, reviewID, err)
	}
	if _, _, err := svc.ResolveReviewGate(context.Background(), current, "run-2", "foreign-review"); !errors.Is(err, ErrReviewReferenceMismatch) {
		t.Fatalf("foreign ids error = %v, want reference mismatch", err)
	}
}

func TestReviewMutationResolverDerivesRunIDWhenArtifactCarriesTaskID(t *testing.T) {
	base := newMemoryRunStore()
	base.runs["agent-run-1"] = &Run{ID: "agent-run-1", TaskID: "task-1", Status: RunStatusRunning}
	runs := &taskAwareMemoryRunStore{memoryRunStore: base}
	nodes := &mutationNodeStore{nodes: []*model.Node{{
		ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess,
		Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v3"},
	}}}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, &recordingReviewStateMachine{})
	current := &artifact.Artifact{ID: "script-v3", WorkflowRunID: "task-1", TaskID: "task-1", StageName: "script", ProducedByNode: "script-exec"}

	runID, reviewID, err := svc.ResolveReviewGate(context.Background(), current, "", "")
	if err != nil || runID != "agent-run-1" || reviewID != "script-review" {
		t.Fatalf("ResolveReviewGate() = (%q,%q,%v)", runID, reviewID, err)
	}
}

func TestReviewMutationResolverMatchesCompiledSourceToGateOriginalNodeID(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{
		{ID: "compiled-script-exec", TaskID: "task-1", Type: model.NodeTypeTool, Status: model.NodeSuccess, Input: map[string]interface{}{"agentOriginalNodeId": "script-exec"}},
		{ID: "compiled-script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeSuccess, Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script"}},
	}}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, &recordingReviewStateMachine{})
	current := &artifact.Artifact{ID: "script-v3", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", ProducedByNode: "compiled-script-exec"}

	_, reviewID, err := svc.ResolveReviewGate(context.Background(), current, "", "")
	if err != nil || reviewID != "compiled-script-review" {
		t.Fatalf("ResolveReviewGate() review=%q error=%v", reviewID, err)
	}
}

func TestReviewMutationSubmitEditedPreservesLegacyApprovalAndStaleBehavior(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{{
		ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeReady,
		Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script", "artifactId": "script-v3", "requiredOutputs": []string{"VIDEO_SCRIPT"}},
	}}}
	state := &recordingReviewStateMachine{}
	artifacts := &recordingArtifactService{}
	log := &mutationDecisionLog{}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, state).
		WithArtifactService(artifacts).
		WithProjectIDResolver(staticProjectIDResolver{projectID: "vp-1"}).
		WithDecisionLogWriter(log)

	if err := svc.SubmitEdited(context.Background(), "run-1", "script-review", "user-1", "手工微调", map[string]interface{}{"text": "新版"}); err != nil {
		t.Fatalf("SubmitEdited() error = %v", err)
	}
	if state.successOutput["edited"] != true || state.successOutput["editContent"] == nil {
		t.Fatalf("success output = %+v", state.successOutput)
	}
	if artifacts.approvedID != "script-v3" || artifacts.markByArtifactID != "script-v3" {
		t.Fatalf("approved=%q stale=%q", artifacts.approvedID, artifacts.markByArtifactID)
	}
	if len(log.records) != 1 || log.records[0].DecisionType != DecisionStageEdit {
		t.Fatalf("decision log = %+v", log.records)
	}
}

func TestReviewMutationRegenerateResetsSourceAndGateAndDispatches(t *testing.T) {
	runs := newMemoryRunStore()
	runs.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodes := &mutationNodeStore{nodes: []*model.Node{
		{ID: "actual-script-exec", TaskID: "task-1", Type: model.NodeTypeTool, Status: model.NodeSuccess, Input: map[string]interface{}{"agentOriginalNodeId": "script-exec"}},
		{ID: "script-review", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeReady, Input: map[string]interface{}{"sourceNode": "script-exec", "stage": "script"}},
	}}
	dispatcher := &recordingRegenerationDispatcher{}
	svc := NewReviewMutationService(NewRunner(nil, runs, nil, nil, nil), nodes, &recordingReviewStateMachine{}).
		WithRegenerationDispatcher(dispatcher)

	if _, err := svc.Regenerate(context.Background(), "run-1", "script-review", "user-1", "重做"); err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if nodes.nodes[0].Status != model.NodeCreated || nodes.nodes[1].Status != model.NodeCreated {
		t.Fatalf("statuses = %s/%s", nodes.nodes[0].Status, nodes.nodes[1].Status)
	}
	if dispatcher.resumedTaskID != "task-1" || dispatcher.retriedNodeID != "actual-script-exec" {
		t.Fatalf("dispatch = resume %q retry %q", dispatcher.resumedTaskID, dispatcher.retriedNodeID)
	}
}

type mutationNodeStore struct {
	nodes []*model.Node
}

func (s *mutationNodeStore) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	var found []*model.Node
	for _, node := range s.nodes {
		if node.TaskID == taskID {
			found = append(found, node)
		}
	}
	return found, nil
}

func (s *mutationNodeStore) UpdateStatus(_ context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error {
	for _, node := range s.nodes {
		if node.ID == id {
			node.Status, node.Output, node.ErrorMessage = status, output, errMsg
			return nil
		}
	}
	return errors.New("node not found")
}

func (s *mutationNodeStore) UpdateInputFields(_ context.Context, id string, fields map[string]interface{}) error {
	for _, node := range s.nodes {
		if node.ID == id {
			if node.Input == nil {
				node.Input = map[string]interface{}{}
			}
			for key, value := range fields {
				node.Input[key] = value
			}
			return nil
		}
	}
	return errors.New("node not found")
}

type mutationDecisionLog struct {
	records []*DecisionLogRecord
}

type mutationAtomicReopener struct {
	requests []ReviewReopenRequest
	err      error
}

func (s *mutationAtomicReopener) ReopenReviewGateAtomic(_ context.Context, req ReviewReopenRequest) error {
	s.requests = append(s.requests, req)
	return s.err
}

type taskAwareMemoryRunStore struct {
	*memoryRunStore
}

func (s *taskAwareMemoryRunStore) FindRunByTaskID(_ context.Context, taskID string) (*Run, error) {
	for _, run := range s.runs {
		if run.TaskID == taskID {
			return run, nil
		}
	}
	return nil, nil
}

func (l *mutationDecisionLog) Save(_ context.Context, record *DecisionLogRecord) error {
	copy := *record
	l.records = append(l.records, &copy)
	return nil
}
