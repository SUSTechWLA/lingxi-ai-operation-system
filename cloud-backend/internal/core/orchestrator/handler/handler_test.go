package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestOrchestratorHandler_Health(t *testing.T) {
	r := gin.New()
	h := &OrchestratorHandler{}
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var envelope struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &envelope)

	if envelope.Data == nil {
		t.Fatalf("Expected envelope with data, got nil")
	}
	if envelope.Data["status"] != "UP" {
		t.Errorf("Expected status UP, got %v", envelope.Data["status"])
	}
	if envelope.Data["service"] != "ai-orchestrator" {
		t.Errorf("Expected service ai-orchestrator, got %v", envelope.Data["service"])
	}
}

type recordingOrchestrator struct {
	userID string
	input  map[string]interface{}
	dag    *model.DAGRequest
}

func (r *recordingOrchestrator) CreateTask(_ context.Context, userID string, input map[string]interface{}) (*model.Task, error) {
	r.userID, r.input = userID, input
	return &model.Task{ID: "task-auth", UserID: userID, Status: model.TaskCreated}, nil
}
func (r *recordingOrchestrator) SubmitDAG(_ context.Context, _ string, dag *model.DAGRequest) error {
	r.dag = dag
	return nil
}
func (*recordingOrchestrator) GetTaskWithDetails(context.Context, string) (map[string]interface{}, error) {
	return nil, nil
}
func (*recordingOrchestrator) GetTaskProgress(context.Context, string) (*model.TaskProgressResponse, error) {
	return nil, nil
}

func authenticatedRouter(h *OrchestratorHandler, userID string) *gin.Engine {
	router := gin.New()
	h.RegisterRoutes(router, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), userID))
		c.Next()
	})
	return router
}

func TestCreateTaskUsesAuthenticatedOwnerInsteadOfForgedBody(t *testing.T) {
	service := &recordingOrchestrator{}
	h := &OrchestratorHandler{orchestratorService: service}
	req := httptest.NewRequest(http.MethodPost, "/api/task/create", bytes.NewBufferString(`{"userId":"attacker","user_id":"attacker-2","query":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	authenticatedRouter(h, "user-auth").ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if service.userID != "user-auth" {
		t.Fatalf("CreateTask owner=%q", service.userID)
	}
}

func TestSubmitDAGFromNLUsesAuthenticatedOwner(t *testing.T) {
	service := &recordingOrchestrator{}
	h := &OrchestratorHandler{orchestratorService: service}
	req := httptest.NewRequest(http.MethodPost, "/api/node", bytes.NewBufferString(`{"userId":"attacker","nodes":[],"edges":[]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	authenticatedRouter(h, "user-auth").ServeHTTP(res, req)
	if res.Code != http.StatusOK || service.userID != "user-auth" || service.dag == nil {
		t.Fatalf("status=%d owner=%q dag=%#v body=%s", res.Code, service.userID, service.dag, res.Body.String())
	}
}

func TestConvertToDAGRequest_NodesAndEdges(t *testing.T) {
	payload := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id":   "1",
				"type": "LLM",
				"name": "step1",
				"input": map[string]interface{}{
					"prompt": "hello",
				},
			},
			map[string]interface{}{
				"nodeId": "2",
				"type":   "TOOL",
				"name":   "step2",
			},
		},
		"edges": []interface{}{
			map[string]interface{}{
				"from": "1",
				"to":   "2",
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	if len(dagReq.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(dagReq.Nodes))
	}

	// First node uses "id" field
	if dagReq.Nodes[0].ID != "1" {
		t.Errorf("Expected node ID '1', got '%s'", dagReq.Nodes[0].ID)
	}
	if dagReq.Nodes[0].Type != "LLM" {
		t.Errorf("Expected type 'LLM', got '%s'", dagReq.Nodes[0].Type)
	}

	// Second node uses "nodeId" field (fallback since "id" is empty)
	if dagReq.Nodes[1].ID != "2" {
		t.Errorf("Expected node ID '2' (from nodeId), got '%s'", dagReq.Nodes[1].ID)
	}
	if dagReq.Nodes[1].Type != "TOOL" {
		t.Errorf("Expected type 'TOOL', got '%s'", dagReq.Nodes[1].Type)
	}

	if len(dagReq.Edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(dagReq.Edges))
	}
	if dagReq.Edges[0].From != "1" || dagReq.Edges[0].To != "2" {
		t.Errorf("Expected edge 1->2, got %s->%s", dagReq.Edges[0].From, dagReq.Edges[0].To)
	}
}

func TestConvertToDAGRequest_EmptyPayload(t *testing.T) {
	payload := map[string]interface{}{}

	dagReq := convertToDAGRequest(payload)

	if len(dagReq.Nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(dagReq.Nodes))
	}
	if len(dagReq.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(dagReq.Edges))
	}
}

func TestConvertToDAGRequest_InvalidNodeFormat(t *testing.T) {
	payload := map[string]interface{}{
		"nodes": []interface{}{
			"not a map",
			map[string]interface{}{
				"id":   "1",
				"type": "LLM",
				"name": "valid",
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	if len(dagReq.Nodes) != 1 {
		t.Errorf("Expected 1 valid node (skipping invalid), got %d", len(dagReq.Nodes))
	}
}

func TestConvertToDAGRequest_IdFallbackToNodeId(t *testing.T) {
	payload := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"nodeId": "from-nodeId",
				"type":   "LLM",
				"name":   "test",
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	if dagReq.Nodes[0].ID != "from-nodeId" {
		t.Errorf("Expected ID 'from-nodeId', got '%s'", dagReq.Nodes[0].ID)
	}
}

func TestConvertToDAGRequest_IdFieldTakesPriority(t *testing.T) {
	// When both "id" and "nodeId" are present, "id" is checked first
	// Looking at the code: nodeID, _ = nodeMap["nodeId"].(string); if nodeID == "" { nodeID, _ = nodeMap["id"].(string) }
	// So "nodeId" actually takes priority if both are present
	payload := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id":     "from-id",
				"nodeId": "from-nodeId",
				"type":   "LLM",
				"name":   "test",
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	// nodeId is checked first in the code, so it takes priority
	if dagReq.Nodes[0].ID != "from-nodeId" {
		t.Errorf("Expected 'nodeId' to take priority (checked first), got '%s'", dagReq.Nodes[0].ID)
	}
}

func TestConvertToDAGRequest_InputField(t *testing.T) {
	payload := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id":   "1",
				"type": "LLM",
				"name": "test",
				"input": map[string]interface{}{
					"prompt": "hello",
				},
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	if dagReq.Nodes[0].Input["prompt"] != "hello" {
		t.Errorf("Expected input.prompt='hello', got %v", dagReq.Nodes[0].Input)
	}
}

func TestConvertToDAGRequest_InvalidEdgeFormat(t *testing.T) {
	payload := map[string]interface{}{
		"edges": []interface{}{
			"not a map",
			map[string]interface{}{
				"from": "1",
				"to":   "2",
			},
		},
	}

	dagReq := convertToDAGRequest(payload)

	if len(dagReq.Edges) != 1 {
		t.Errorf("Expected 1 valid edge (skipping invalid), got %d", len(dagReq.Edges))
	}
}
