package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	coremodel "github.com/tangying-ai/aios-core/internal/core/model"
)

func TestRegisterVideoProjectAndWorkflowRoutesNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	projectHandler := NewProjectHandler(nil)
	workflowHandler := NewWorkflowHandler(nil, nil)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("registering video project and workflow routes panicked: %v", recovered)
		}
	}()

	projectHandler.RegisterRoutes(r)
	workflowHandler.RegisterRoutes(r)
}

func TestApproveStageRouteDelegatesToDomainApprover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	approver := &fakeVideoStageApprover{}
	NewWorkflowHandler(nil, approver).RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/project-1/stages/script/approve", strings.NewReader(`{
		"runId": "run-1",
		"output": {"approved": true, "source": "test"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if approver.projectID != "project-1" || approver.runID != "run-1" || approver.stageName != "script" {
		t.Fatalf("unexpected approval target: project=%q run=%q stage=%q", approver.projectID, approver.runID, approver.stageName)
	}
	if approver.output["approved"] != true || approver.output["source"] != "test" {
		t.Fatalf("approval output was not forwarded: %#v", approver.output)
	}

	var body struct {
		Code int `json:"code"`
		Data struct {
			NodeID string `json:"nodeId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if body.Code != 200 || body.Data.NodeID != "script" {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

type fakeVideoStageApprover struct {
	projectID string
	runID     string
	stageName string
	output    map[string]interface{}
}

func (f *fakeVideoStageApprover) ApproveStage(ctx context.Context, projectID, runID, stageName string, output map[string]interface{}) (*coremodel.Node, error) {
	f.projectID = projectID
	f.runID = runID
	f.stageName = stageName
	f.output = output
	return &coremodel.Node{ID: stageName, TaskID: "task-1", Type: coremodel.NodeTypeControl, Status: coremodel.NodeSuccess}, nil
}
