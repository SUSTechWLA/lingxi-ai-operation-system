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

func TestApproveReviewApprovesCurrentArtifactByStageAndKindWhenArtifactIDMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runStore := newMemoryRunStore()
	runStore.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodeStore := &memoryReviewNodeStore{nodes: []*model.Node{{
		ID:     "preview_review",
		TaskID: "task-1",
		Type:   model.NodeTypeReviewGate,
		Status: model.NodeReady,
		Input: map[string]interface{}{
			"stage":           "preview",
			"roleAgentId":     "preview_director",
			"artifactKinds":   []interface{}{"PREVIEW_SNAPSHOTS"},
			"requiredOutputs": []interface{}{"PREVIEW_SNAPSHOTS"},
		},
	}}}
	stateMachine := &recordingReviewStateMachine{}
	artifactSvc := &recordingArtifactService{}
	handler := NewHandler(
		NewRunner(nil, runStore, nil, nil, nil),
		nodeStore,
		stateMachine,
	).
		WithArtifactService(artifactSvc).
		WithProjectIDResolver(staticProjectIDResolver{projectID: "project-1"})

	router := gin.New()
	handler.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/runs/run-1/reviews/preview_review/approve", bytes.NewBufferString(`{"reviewerId":"user-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if artifactSvc.approvedID != "" {
		t.Fatalf("fallback path should not approve by synthetic artifact id, got %q", artifactSvc.approvedID)
	}
	if artifactSvc.approvedProjectID != "project-1" || artifactSvc.approvedStageName != "preview" {
		t.Fatalf("fallback should use project+stage, got project=%q stage=%q", artifactSvc.approvedProjectID, artifactSvc.approvedStageName)
	}
	if len(artifactSvc.approvedKinds) != 1 || artifactSvc.approvedKinds[0] != "PREVIEW_SNAPSHOTS" {
		t.Fatalf("fallback should approve preview snapshots, got %#v", artifactSvc.approvedKinds)
	}
	if artifactSvc.approvedReviewerID != "user-1" {
		t.Fatalf("fallback should carry reviewer id, got %q", artifactSvc.approvedReviewerID)
	}
}

func TestListReviewsIncludesSourceNodeOutputForOnlineReview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runStore := newMemoryRunStore()
	runStore.runs["run-1"] = &Run{ID: "run-1", TaskID: "task-1", Status: RunStatusRunning}
	nodeStore := &memoryReviewNodeStore{nodes: []*model.Node{
		{
			ID:     "proposal_exec",
			TaskID: "task-1",
			Type:   model.NodeTypeTool,
			Status: model.NodeSuccess,
			Output: map[string]interface{}{
				"stdout": `{
					"content":"# Proposal Packet\n\n推荐方案：option_a",
					"artifacts":[{"kind":"JSON","name":"proposal_packet.json","unitId":"proposal_generator"}],
					"proposalPacket":{"recommendedOptionId":"option_a"}
				}`,
			},
		},
		{
			ID:     "proposal_review",
			TaskID: "task-1",
			Type:   model.NodeTypeReviewGate,
			Status: model.NodeReady,
			Input: map[string]interface{}{
				"sourceNode":   "proposal_exec",
				"stepId":       "proposal_generator",
				"tool":         "proposal_generator",
				"reviewPhase":  "after_artifact",
				"reviewReason": "确认创作方案后继续",
			},
		},
	}}
	handler := NewHandler(
		NewRunner(nil, runStore, nil, nil, nil),
		nodeStore,
		&recordingReviewStateMachine{},
	)

	router := gin.New()
	handler.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/runs/run-1/reviews", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Reviews []Review `json:"reviews"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Reviews) != 1 {
		t.Fatalf("expected one review, got %#v", resp.Data.Reviews)
	}
	review := resp.Data.Reviews[0]
	if review.SourceNodeID != "proposal_exec" {
		t.Fatalf("review should expose source node id, got %#v", review)
	}
	if review.ReviewContent != "# Proposal Packet\n\n推荐方案：option_a" {
		t.Fatalf("review should expose source content, got %#v", review.ReviewContent)
	}
	if len(review.ReviewArtifacts) != 1 || review.ReviewArtifacts[0]["name"] != "proposal_packet.json" {
		t.Fatalf("review should expose source artifacts, got %#v", review.ReviewArtifacts)
	}
	if review.ReviewOutput["proposalPacket"] == nil {
		t.Fatalf("review should expose parsed review output, got %#v", review.ReviewOutput)
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
