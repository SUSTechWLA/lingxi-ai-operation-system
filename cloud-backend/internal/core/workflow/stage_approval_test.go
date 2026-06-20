package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestStageApprovalServiceApprovesReadyControlStage(t *testing.T) {
	runs := &fakeApprovalRunStore{
		byID: map[string]*WorkflowRun{
			"run-1": {ID: "run-1", ProjectID: "project-1", TaskID: "task-1"},
		},
	}
	nodes := &fakeApprovalNodeStore{
		nodes: []*model.Node{
			{ID: "script_exec", TaskID: "task-1", Type: model.NodeTypeTool, Status: model.NodeSuccess},
			{ID: "script", TaskID: "task-1", Type: model.NodeTypeControl, Status: model.NodeReady},
		},
	}
	recorder := &fakeApprovalSuccessRecorder{}
	svc := NewStageApprovalService(runs, nodes, recorder)

	node, err := svc.ApproveStage(context.Background(), "project-1", "run-1", "script", map[string]interface{}{"approved": true})
	if err != nil {
		t.Fatalf("ApproveStage returned error: %v", err)
	}

	if node.ID != "script" {
		t.Fatalf("expected script approval node, got %q", node.ID)
	}
	if recorder.nodeID != "script" {
		t.Fatalf("expected recorder to approve script node, got %q", recorder.nodeID)
	}
	if recorder.output["approved"] != true {
		t.Fatalf("approval output was not forwarded: %#v", recorder.output)
	}
}

func TestStageApprovalServiceRejectsStageThatIsNotReady(t *testing.T) {
	runs := &fakeApprovalRunStore{
		byID: map[string]*WorkflowRun{
			"run-1": {ID: "run-1", ProjectID: "project-1", TaskID: "task-1"},
		},
	}
	nodes := &fakeApprovalNodeStore{
		nodes: []*model.Node{
			{ID: "script", TaskID: "task-1", Type: model.NodeTypeControl, Status: model.NodeCreated},
		},
	}
	recorder := &fakeApprovalSuccessRecorder{}
	svc := NewStageApprovalService(runs, nodes, recorder)

	_, err := svc.ApproveStage(context.Background(), "project-1", "run-1", "script", map[string]interface{}{"approved": true})
	if !errors.Is(err, ErrStageNotReady) {
		t.Fatalf("expected ErrStageNotReady, got %v", err)
	}
	if recorder.nodeID != "" {
		t.Fatalf("recorder should not be called, got node %q", recorder.nodeID)
	}
}

type fakeApprovalRunStore struct {
	byID      map[string]*WorkflowRun
	byProject map[string][]*WorkflowRun
}

func (f *fakeApprovalRunStore) FindByID(ctx context.Context, id string) (*WorkflowRun, error) {
	return f.byID[id], nil
}

func (f *fakeApprovalRunStore) FindByProject(ctx context.Context, projectID string) ([]*WorkflowRun, error) {
	return f.byProject[projectID], nil
}

type fakeApprovalNodeStore struct {
	nodes []*model.Node
}

func (f *fakeApprovalNodeStore) FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error) {
	return f.nodes, nil
}

type fakeApprovalSuccessRecorder struct {
	nodeID string
	output map[string]interface{}
}

func (f *fakeApprovalSuccessRecorder) OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error {
	f.nodeID = nodeID
	f.output = output
	return nil
}
