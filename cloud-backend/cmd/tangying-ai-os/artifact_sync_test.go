package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestBindArtifactIDToReviewGateInput(t *testing.T) {
	repo := &reviewBindingNodeRepo{nodes: []*model.Node{
		{
			ID:     "script_exec",
			TaskID: "task-1",
			Type:   model.NodeTypeTool,
			Status: model.NodeSuccess,
		},
		{
			ID:     "script_review",
			TaskID: "task-1",
			Type:   model.NodeTypeReviewGate,
			Status: model.NodeReady,
			Input: map[string]interface{}{
				"sourceNode":      "script_exec",
				"stage":           "script",
				"requiredOutputs": []interface{}{"VIDEO_SCRIPT"},
			},
		},
	}}

	err := bindArtifactIDToReviewGates(context.Background(), repo, "task-1", "script_exec", &artifact.Artifact{
		ID:        "art_real_script",
		StageName: "script",
		Kind:      artifact.KindMarkdown,
	})
	if err != nil {
		t.Fatalf("bind artifact id returned error: %v", err)
	}

	updated := repo.inputUpdates["script_review"]
	if updated["artifactId"] != "art_real_script" {
		t.Fatalf("review gate should receive real artifactId, got %#v", updated)
	}
	if updated["stage"] != "script" {
		t.Fatalf("review gate stage should be preserved, got %#v", updated)
	}
}

func TestLocalJobCompletion_HyperFramesRenderWritesVideoArtifactRequest(t *testing.T) {
	node := &model.Node{
		ID:     "render_exec",
		TaskID: "task-1",
		Input: map[string]interface{}{
			"stage":       "render",
			"roleAgentId": "render_producer",
		},
	}
	rawArtifacts := []interface{}{
		map[string]interface{}{
			"unitId":         "final-video",
			"kind":           "VIDEO",
			"name":           "final.mp4",
			"storageType":    "local",
			"storageRef":     "local://projects/project-1/renders/final.mp4",
			"mimeType":       "video/mp4",
			"sizeBytes":      float64(123456),
			"status":         "valid",
			"humanApproved":  false,
			"dependsOn":      []interface{}{"PREVIEW_SNAPSHOTS", "HYPERFRAMES_PROJECT"},
			"producedByTool": "hyperframes_renderer",
			"producedByRole": "渲染制片",
			"metadata": map[string]interface{}{
				"renderTimeMs": float64(12345),
				"fps":          float64(30),
			},
		},
	}

	requests, err := buildArtifactRequestsFromOutputArtifacts("project-1", "run-1", "task-1", "render", node, "hyperframes_renderer", rawArtifacts)
	if err != nil {
		t.Fatalf("build artifact requests returned error: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one artifact request, got %d", len(requests))
	}
	video := requests[0]
	if video.UnitID != "final-video" {
		t.Fatalf("VIDEO artifact should preserve executor unitId, got %q", video.UnitID)
	}
	if video.Kind != artifact.KindVideo {
		t.Fatalf("expected VIDEO kind, got %q", video.Kind)
	}
	if video.WorkflowRunID != "run-1" || video.TaskID != "task-1" {
		t.Fatalf("workflowRunID and taskID must stay distinct, got workflowRunID=%q taskID=%q", video.WorkflowRunID, video.TaskID)
	}
	if video.StageName != "render" || video.Metadata["status"] != "valid" {
		t.Fatalf("unexpected video artifact request: %+v", video)
	}
	if video.Metadata["producedByTool"] != "hyperframes_renderer" || video.Metadata["producedByRole"] != "渲染制片" {
		t.Fatalf("producer metadata not preserved: %+v", video.Metadata)
	}
}

func TestLocalJobArtifactManifestRejectsMissingUnitID(t *testing.T) {
	node := &model.Node{ID: "render_exec", TaskID: "task-1", Input: map[string]interface{}{"stage": "render"}}
	rawArtifacts := []interface{}{
		map[string]interface{}{
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"storageRef": "local://projects/project-1/renders/final.mp4",
		},
	}

	_, err := buildArtifactRequestsFromOutputArtifacts("project-1", "run-1", "task-1", "render", node, "hyperframes_renderer", rawArtifacts)
	if err == nil {
		t.Fatal("expected local job artifact without unitId to be rejected")
	}
	if err.Error() != "ARTIFACT_MANIFEST_INVALID: artifact unitId is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArtifactSyncCriticalArtifactCreateFailureReturnsError(t *testing.T) {
	node := buildSyncTestNode([]map[string]interface{}{
		{
			"unitId":     "final-video",
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"storageRef": "local://projects/project-1/renders/final.mp4",
		},
	})
	service := artifact.NewArtifactSyncService(
		artifactCreatorFunc(func(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
			if req.Kind == artifact.KindVideo {
				return nil, errors.New("db insert failed")
			}
			return &artifact.Artifact{ID: "art-ok", Kind: req.Kind, UnitID: req.UnitID}, nil
		}),
		nil,
	)

	created, err := service.SyncFromNodeOutput(context.Background(), "project-1", "run-1", "task-1", node)
	if err == nil {
		t.Fatal("expected critical artifact create failure to return error")
	}
	if !strings.Contains(err.Error(), "critical artifact sync failed") ||
		!strings.Contains(err.Error(), "VIDEO") ||
		!strings.Contains(err.Error(), "projectID=project-1") ||
		!strings.Contains(err.Error(), "workflowRunID=run-1") ||
		!strings.Contains(err.Error(), "taskID=task-1") ||
		!strings.Contains(err.Error(), "nodeID=render_exec") {
		t.Fatalf("critical sync error missing context: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no created artifacts after critical failure, got %+v", created)
	}
}

func TestArtifactSyncNonCriticalArtifactCreateFailureContinues(t *testing.T) {
	node := buildSyncTestNode([]map[string]interface{}{
		{
			"unitId":     "style-profile",
			"kind":       "STYLE_PROFILE",
			"name":       "style_profile.json",
			"storageRef": "local://projects/project-1/style/profile.json",
		},
		{
			"unitId":     "final-video",
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"storageRef": "local://projects/project-1/renders/final.mp4",
		},
	})
	service := artifact.NewArtifactSyncService(
		artifactCreatorFunc(func(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
			if req.Kind == artifact.ArtifactKind("STYLE_PROFILE") {
				return nil, errors.New("temporary insert failed")
			}
			return &artifact.Artifact{ID: "art-video", Kind: req.Kind, UnitID: req.UnitID}, nil
		}),
		nil,
	)

	created, err := service.SyncFromNodeOutput(context.Background(), "project-1", "run-1", "task-1", node)
	if err != nil {
		t.Fatalf("non-critical artifact failure should not abort sync: %v", err)
	}
	if len(created) != 1 || created[0].Kind != artifact.KindVideo {
		t.Fatalf("expected VIDEO artifact to continue syncing, got %+v", created)
	}
}

func TestArtifactSyncUsesWorkflowRunIDNotTaskID(t *testing.T) {
	node := buildSyncTestNode([]map[string]interface{}{
		{
			"unitId":     "final-video",
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"storageRef": "local://projects/project-1/renders/final.mp4",
		},
	})
	var captured *artifact.CreateArtifactRequest
	service := artifact.NewArtifactSyncService(
		artifactCreatorFunc(func(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
			copy := *req
			captured = &copy
			return &artifact.Artifact{ID: "art-video", Kind: req.Kind, UnitID: req.UnitID}, nil
		}),
		nil,
	)

	_, err := service.SyncFromNodeOutput(context.Background(), "project-1", "run-1", "task-1", node)
	if err != nil {
		t.Fatalf("sync returned error: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateArtifact to be called")
	}
	if captured.WorkflowRunID != "run-1" || captured.TaskID != "task-1" || captured.WorkflowRunID == captured.TaskID {
		t.Fatalf("workflowRunID/taskID mixed: workflowRunID=%q taskID=%q", captured.WorkflowRunID, captured.TaskID)
	}
}

type artifactCreatorFunc func(context.Context, *artifact.CreateArtifactRequest) (*artifact.Artifact, error)

func (f artifactCreatorFunc) CreateArtifact(ctx context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	return f(ctx, req)
}

func buildSyncTestNode(artifacts []map[string]interface{}) *model.Node {
	items := make([]interface{}, 0, len(artifacts))
	for _, item := range artifacts {
		items = append(items, item)
	}
	return &model.Node{
		ID:     "render_exec",
		TaskID: "task-1",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage":       "render",
			"roleAgentId": "render_producer",
		},
		Output: map[string]interface{}{
			"success":   true,
			"artifacts": items,
		},
	}
}

type reviewBindingNodeRepo struct {
	nodes        []*model.Node
	inputUpdates map[string]map[string]interface{}
}

func (r *reviewBindingNodeRepo) FindByID(_ context.Context, id string) (*model.Node, error) {
	for _, node := range r.nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return nil, nil
}

func (r *reviewBindingNodeRepo) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	result := make([]*model.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.TaskID == taskID {
			result = append(result, node)
		}
	}
	return result, nil
}

func (r *reviewBindingNodeRepo) UpdateInputFields(_ context.Context, id string, fields map[string]interface{}) error {
	if r.inputUpdates == nil {
		r.inputUpdates = map[string]map[string]interface{}{}
	}
	r.inputUpdates[id] = fields
	for _, node := range r.nodes {
		if node.ID == id {
			if node.Input == nil {
				node.Input = map[string]interface{}{}
			}
			for key, value := range fields {
				node.Input[key] = value
			}
		}
	}
	return nil
}
