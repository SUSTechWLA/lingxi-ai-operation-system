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
