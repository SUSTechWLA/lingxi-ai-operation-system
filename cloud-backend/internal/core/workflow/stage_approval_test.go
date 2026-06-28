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

func TestStageApprovalServiceApprovesCompositionAliases(t *testing.T) {
	for _, stageName := range []string{"composition", "A04", "video_structure", "timeline"} {
		t.Run(stageName, func(t *testing.T) {
			runs := &fakeApprovalRunStore{
				byID: map[string]*WorkflowRun{
					"run-1": {ID: "run-1", ProjectID: "project-1", TaskID: "task-1"},
				},
			}
			nodes := &fakeApprovalNodeStore{
				nodes: []*model.Node{
					{ID: "composition_exec", TaskID: "task-1", Type: model.NodeTypeTool, Status: model.NodeSuccess, Input: map[string]interface{}{"stage": "composition"}},
					{ID: "composition_review_gate", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeReady, Input: map[string]interface{}{"stage": "composition", "requiredOutputs": []interface{}{"VIDEO_COMPOSITION_SPEC"}}},
				},
			}
			recorder := &fakeApprovalSuccessRecorder{}
			svc := NewStageApprovalService(runs, nodes, recorder)

			node, err := svc.ApproveStage(context.Background(), "project-1", "run-1", stageName, map[string]interface{}{"approved": true})
			if err != nil {
				t.Fatalf("ApproveStage(%q) returned error: %v", stageName, err)
			}

			if node.ID != "composition_review_gate" {
				t.Fatalf("expected composition_review_gate for %q, got %q", stageName, node.ID)
			}
			if recorder.nodeID != "composition_review_gate" {
				t.Fatalf("expected recorder to approve composition_review_gate, got %q", recorder.nodeID)
			}
		})
	}
}

func TestStageApprovalServiceMarksCompositionArtifactApproved(t *testing.T) {
	runs := &fakeApprovalRunStore{
		byID: map[string]*WorkflowRun{
			"run-1": {ID: "run-1", ProjectID: "project-1", TaskID: "task-1"},
		},
	}
	nodes := &fakeApprovalNodeStore{
		nodes: []*model.Node{
			{ID: "composition_review_gate", TaskID: "task-1", Type: model.NodeTypeReviewGate, Status: model.NodeReady, Input: map[string]interface{}{"stage": "composition", "requiredOutputs": []interface{}{"VIDEO_COMPOSITION_SPEC"}}},
		},
	}
	recorder := &fakeApprovalSuccessRecorder{}
	artifacts := &fakeApprovalArtifactApprover{approvedIDs: []string{"art-composition"}}
	svc := NewStageApprovalService(runs, nodes, recorder).WithArtifactApprover(artifacts)

	_, err := svc.ApproveStage(context.Background(), "project-1", "run-1", "composition", map[string]interface{}{"reviewerId": "user-1"})
	if err != nil {
		t.Fatalf("ApproveStage returned error: %v", err)
	}
	if artifacts.projectID != "project-1" || artifacts.stageName != "composition" || artifacts.reviewerID != "user-1" {
		t.Fatalf("unexpected artifact approval target: %#v", artifacts)
	}
	if len(artifacts.kinds) != 1 || artifacts.kinds[0] != "VIDEO_COMPOSITION_SPEC" {
		t.Fatalf("expected VIDEO_COMPOSITION_SPEC approval, got %#v", artifacts.kinds)
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

type fakeApprovalArtifactApprover struct {
	projectID   string
	stageName   string
	kinds       []string
	reviewerID  string
	approvedIDs []string
}

func (f *fakeApprovalArtifactApprover) ApproveCurrentArtifactsByStageAndKinds(ctx context.Context, projectID string, stageName string, artifactKinds []string, reviewerID string) ([]string, error) {
	f.projectID = projectID
	f.stageName = stageName
	f.kinds = artifactKinds
	f.reviewerID = reviewerID
	return f.approvedIDs, nil
}
