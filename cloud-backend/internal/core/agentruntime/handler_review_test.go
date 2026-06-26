package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestReviewFromNodeIncludesArtifactID(t *testing.T) {
	review := reviewFromNode(&model.Node{
		ID:     "script_review",
		Status: model.NodeReady,
		Input: map[string]interface{}{
			"artifactId":      "art_real_script",
			"stage":           "script",
			"requiredOutputs": []interface{}{"VIDEO_SCRIPT"},
		},
	})

	if review.ArtifactID != "art_real_script" {
		t.Fatalf("review should expose artifactId, got %#v", review)
	}
}

func TestApproveReviewMarksArtifactAndCarriesArtifactIDInOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runStore := newMemoryRunStore()
	runStore.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodeStore := &memoryReviewNodeStore{nodes: []*model.Node{{
		ID:     "script_review",
		TaskID: "task-1",
		Type:   model.NodeTypeReviewGate,
		Status: model.NodeReady,
		Input: map[string]interface{}{
			"artifactId":      "art_real_script",
			"stage":           "script",
			"roleAgentId":     "script_writer",
			"requiredOutputs": []interface{}{"VIDEO_SCRIPT"},
		},
	}}}
	stateMachine := &recordingReviewStateMachine{}
	artifactSvc := &recordingArtifactService{}
	handler := NewHandler(
		NewRunner(nil, runStore, nil, nil, nil),
		nodeStore,
		stateMachine,
	).WithArtifactService(artifactSvc)

	router := gin.New()
	handler.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/runs/run-1/reviews/script_review/approve", bytes.NewBufferString(`{"reviewerId":"user-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if artifactSvc.approvedID != "art_real_script" {
		t.Fatalf("approve should mark artifact humanApproved by real artifact id, got %q", artifactSvc.approvedID)
	}
	if stateMachine.successOutput["artifactId"] != "art_real_script" {
		raw, _ := json.Marshal(stateMachine.successOutput)
		t.Fatalf("approval output should carry artifactId, got %s", raw)
	}
}

type memoryReviewNodeStore struct {
	nodes []*model.Node
}

func (s *memoryReviewNodeStore) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	result := make([]*model.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.TaskID == taskID {
			result = append(result, node)
		}
	}
	return result, nil
}

func (s *memoryReviewNodeStore) UpdateStatus(_ context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error {
	for _, node := range s.nodes {
		if node.ID == id {
			node.Status = status
			node.Output = output
			node.ErrorMessage = errMsg
			return nil
		}
	}
	return nil
}

type recordingReviewStateMachine struct {
	successNodeID string
	successOutput map[string]interface{}
	failureNodeID string
	failureError  string
}

func (s *recordingReviewStateMachine) OnSuccess(_ context.Context, nodeID string, output map[string]interface{}) error {
	s.successNodeID = nodeID
	s.successOutput = output
	return nil
}

func (s *recordingReviewStateMachine) OnFailure(_ context.Context, nodeID string, errorMessage string) error {
	s.failureNodeID = nodeID
	s.failureError = errorMessage
	return nil
}
