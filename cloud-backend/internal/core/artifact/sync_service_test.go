package artifact

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

// fakeArtifactCreator is a test double that implements ArtifactCreator.
type fakeArtifactCreator struct {
	create func(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error)
}

func (f fakeArtifactCreator) CreateArtifact(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
	return f.create(ctx, req)
}

// buildRenderNodeWithVideoArtifact builds a node representing a completed
// HYPERFRAMES_RENDER that produced a VIDEO artifact.
func buildRenderNodeWithVideoArtifact() *model.Node {
	return &model.Node{
		ID:     "render_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "render",
			"tool":  "hyperframes_renderer",
		},
		Output: map[string]interface{}{
			"success": true,
			"summary": "HyperFrames 渲染完成",
			"artifacts": []interface{}{
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
						"width":        float64(1920),
						"height":       float64(1080),
						"localPath":    "/tmp/projects/project-1/renders/final.mp4",
					},
				},
			},
		},
	}
}

// buildNodeWithStyleProfileAndVideo builds a node that produced both a
// STYLE_PROFILE (non-critical) and a VIDEO (critical) artifact.
func buildNodeWithStyleProfileAndVideo() *model.Node {
	return &model.Node{
		ID:     "reference_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "reference",
			"tool":  "reference_asset_planner",
		},
		Output: map[string]interface{}{
			"success": true,
			"artifacts": []interface{}{
				map[string]interface{}{
					"unitId":      "style-profile",
					"kind":        "STYLE_PROFILE",
					"name":        "style.json",
					"storageType": "local",
					"storageRef":  "local://projects/project-1/artifacts/reference/style-profile/hash/style.json",
					"mimeType":    "application/json",
					"sizeBytes":   float64(2048),
					"status":      "valid",
				},
				map[string]interface{}{
					"unitId":      "final-video",
					"kind":        "VIDEO",
					"name":        "final.mp4",
					"storageType": "local",
					"storageRef":  "local://projects/project-1/renders/final.mp4",
					"mimeType":    "video/mp4",
					"sizeBytes":   float64(123456),
					"status":      "valid",
				},
			},
		},
	}
}

// TestArtifactSync_CriticalArtifactCreateFailureReturnsError verifies that
// when a critical artifact (VIDEO) fails to create, SyncFromNodeOutput returns
// an error containing CRITICAL_ARTIFACT_SYNC_FAILED.
func TestArtifactSync_CriticalArtifactCreateFailureReturnsError(t *testing.T) {
	ctx := context.Background()

	creator := fakeArtifactCreator{
		create: func(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
			if req.Kind == KindVideo {
				return nil, errors.New("db insert failed")
			}
			return &Artifact{ID: "art-ok", Kind: req.Kind, UnitID: req.UnitID}, nil
		},
	}

	svc := NewArtifactSyncService(creator, zap.NewNop())
	node := buildRenderNodeWithVideoArtifact()

	created, err := svc.SyncFromNodeOutput(ctx, "project-1", "run-1", "task-1", node)

	if err == nil {
		t.Fatal("expected error for critical artifact create failure")
	}
	if !strings.Contains(err.Error(), "CRITICAL_ARTIFACT_SYNC_FAILED") {
		t.Fatalf("error should contain CRITICAL_ARTIFACT_SYNC_FAILED, got: %v", err)
	}
	if !strings.Contains(err.Error(), "VIDEO") {
		t.Fatalf("error should mention VIDEO kind, got: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no created artifacts on critical failure, got %d", len(created))
	}
}

// TestArtifactSync_NonCriticalArtifactCreateFailureContinues verifies that
// when a non-critical artifact (STYLE_PROFILE) fails to create, the sync
// continues and the critical artifact (VIDEO) is still created successfully.
func TestArtifactSync_NonCriticalArtifactCreateFailureContinues(t *testing.T) {
	ctx := context.Background()

	creator := fakeArtifactCreator{
		create: func(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
			if req.Kind == ArtifactKind("STYLE_PROFILE") {
				return nil, errors.New("temporary insert failed")
			}
			return &Artifact{
				ID:     "art-" + req.UnitID,
				Kind:   req.Kind,
				UnitID: req.UnitID,
			}, nil
		},
	}

	svc := NewArtifactSyncService(creator, zap.NewNop())
	node := buildNodeWithStyleProfileAndVideo()

	created, err := svc.SyncFromNodeOutput(ctx, "project-1", "run-1", "task-1", node)

	if err != nil {
		t.Fatalf("expected no error when non-critical artifact fails, got: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created artifact (VIDEO), got %d", len(created))
	}
	if created[0].Kind != KindVideo {
		t.Fatalf("expected VIDEO artifact to be created, got kind=%s", created[0].Kind)
	}
}

// TestArtifactSync_UsesWorkflowRunIDNotTaskID verifies that the
// workflowRunID and taskID are correctly separated in the artifact
// creation request — they must not be mixed up.
func TestArtifactSync_UsesWorkflowRunIDNotTaskID(t *testing.T) {
	ctx := context.Background()

	var captured *CreateArtifactRequest

	creator := fakeArtifactCreator{
		create: func(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
			copyReq := *req
			captured = &copyReq
			return &Artifact{ID: "art-video", Kind: req.Kind, UnitID: req.UnitID}, nil
		},
	}

	svc := NewArtifactSyncService(creator, zap.NewNop())
	node := buildRenderNodeWithVideoArtifact()

	_, err := svc.SyncFromNodeOutput(ctx, "project-1", "run-1", "task-1", node)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("expected CreateArtifactRequest to be captured")
	}
	if captured.WorkflowRunID != "run-1" {
		t.Fatalf("expected WorkflowRunID=run-1, got %q", captured.WorkflowRunID)
	}
	if captured.TaskID != "task-1" {
		t.Fatalf("expected TaskID=task-1, got %q", captured.TaskID)
	}
	if captured.WorkflowRunID == captured.TaskID {
		t.Fatal("WorkflowRunID and TaskID must not be equal")
	}
}

// TestLocalJobCompletion_HyperFramesRenderWritesVideoArtifact verifies the
// full LocalJob completion → VIDEO ArtifactIndex integration: a completed
// HYPERFRAMES_RENDER node produces a VIDEO artifact request with correct
// fields.
func TestLocalJobCompletion_HyperFramesRenderWritesVideoArtifact(t *testing.T) {
	ctx := context.Background()

	var captured *CreateArtifactRequest

	creator := fakeArtifactCreator{
		create: func(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
			copyReq := *req
			captured = &copyReq
			return &Artifact{
				ID:         "art-video",
				ProjectID:  req.ProjectID,
				StageName:  req.StageName,
				Kind:       req.Kind,
				UnitID:     req.UnitID,
				Status:     "valid",
				StorageRef: req.StorageRef,
			}, nil
		},
	}

	svc := NewArtifactSyncService(creator, zap.NewNop())
	node := buildRenderNodeWithVideoArtifact()

	created, err := svc.SyncFromNodeOutput(ctx, "project-1", "run-1", "task-1", node)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created artifact, got %d", len(created))
	}

	if captured == nil {
		t.Fatal("expected CreateArtifactRequest to be captured")
	}
	if captured.ProjectID != "project-1" {
		t.Fatalf("expected ProjectID=project-1, got %q", captured.ProjectID)
	}
	if captured.WorkflowRunID != "run-1" {
		t.Fatalf("expected WorkflowRunID=run-1, got %q", captured.WorkflowRunID)
	}
	if captured.TaskID != "task-1" {
		t.Fatalf("expected TaskID=task-1, got %q", captured.TaskID)
	}
	if captured.StageName != "render" {
		t.Fatalf("expected StageName=render, got %q", captured.StageName)
	}
	if captured.UnitID != "final-video" {
		t.Fatalf("expected UnitID=final-video, got %q", captured.UnitID)
	}
	if captured.Kind != KindVideo {
		t.Fatalf("expected Kind=VIDEO, got %q", captured.Kind)
	}
	if captured.StorageRef != "local://projects/project-1/renders/final.mp4" {
		t.Fatalf("expected StorageRef=local://projects/project-1/renders/final.mp4, got %q", captured.StorageRef)
	}
}
