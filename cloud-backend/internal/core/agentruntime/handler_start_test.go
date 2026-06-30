package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestStartRunMarksLinkedProjectRunning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:             "script",
				Tool:           "video_script_generator",
				Arguments:      map[string]interface{}{"topic": "Cape Verde"},
				ExpectedOutput: []string{"script"},
			},
		},
	}}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{Name: "video_script_generator"},
	}
	projectUpdater := &recordingProjectLifecycleUpdater{}
	handler := NewHandler(
		NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)),
		nil,
		nil,
	).WithProjectLifecycleUpdater(projectUpdater)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	handler.RegisterRoutes(router)

	reqBody := bytes.NewBufferString(`{
		"message": "帮我介绍一下佛得角国家",
		"domain": "video_creation",
		"mode": "dynamic_agent",
		"context": {"projectId": "project-1"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/runs", reqBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if projectUpdater.userID != "user-1" {
		t.Fatalf("project lifecycle updater userID = %q, want user-1", projectUpdater.userID)
	}
	if projectUpdater.projectID != "project-1" {
		t.Fatalf("project lifecycle updater projectID = %q, want project-1", projectUpdater.projectID)
	}
	if projectUpdater.runID == "" {
		t.Fatalf("project lifecycle updater should receive runID")
	}
	run := store.runs[projectUpdater.runID]
	if run == nil {
		t.Fatalf("run %q was not stored", projectUpdater.runID)
	}
	if run.UserID != "user-1" {
		t.Fatalf("run.UserID = %q, want user-1", run.UserID)
	}
	var body struct {
		Data struct {
			Status RunStatus `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Data.Status != RunStatusRunning {
		t.Fatalf("response status = %q, want RUNNING", body.Data.Status)
	}
}

type recordingProjectLifecycleUpdater struct {
	userID    string
	projectID string
	runID     string
}

func (u *recordingProjectLifecycleUpdater) MarkAgentRunStarted(_ context.Context, userID, projectID, runID string) error {
	u.userID = userID
	u.projectID = projectID
	u.runID = runID
	return nil
}

var _ ProjectLifecycleUpdater = (*recordingProjectLifecycleUpdater)(nil)
