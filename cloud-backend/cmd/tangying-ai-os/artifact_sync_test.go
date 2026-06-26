package main

import (
	"context"
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

	requests, err := buildArtifactRequestsFromOutputArtifacts("project-1", "task-1", "render", node, "hyperframes_renderer", rawArtifacts)
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

	_, err := buildArtifactRequestsFromOutputArtifacts("project-1", "task-1", "render", node, "hyperframes_renderer", rawArtifacts)
	if err == nil {
		t.Fatal("expected local job artifact without unitId to be rejected")
	}
	if err.Error() != "ARTIFACT_MANIFEST_INVALID: artifact unitId is required" {
		t.Fatalf("unexpected error: %v", err)
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
