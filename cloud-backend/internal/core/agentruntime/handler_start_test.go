package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestStartRunReturnsRunIDBeforeSlowPlannerCompletes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := &blockingPlanner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		plan: &AgentPlan{
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
		},
	}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{Name: "video_script_generator"},
	}
	handler := NewHandler(
		NewRunner(orch, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)),
		nil,
		nil,
	)

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

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-planner.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("planner was not invoked")
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(planner.release)
		<-done
		t.Fatal("StartRun waited for planner completion instead of returning a run id immediately")
	}
	defer close(planner.release)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			RunID  string    `json:"runId"`
			Status RunStatus `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Data.RunID == "" {
		t.Fatalf("response should include runId before planner completes")
	}
	if body.Data.Status != RunStatusCreated {
		t.Fatalf("response status = %q, want CREATED while planner is still running", body.Data.Status)
	}
	if run, _ := store.FindRun(context.Background(), body.Data.RunID); run == nil {
		t.Fatalf("run %q should be persisted before planner completes", body.Data.RunID)
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

type blockingPlanner struct {
	started chan struct{}
	release chan struct{}
	plan    *AgentPlan
}

func (p *blockingPlanner) GeneratePlan(ctx context.Context, _ StartRunRequest) (*AgentPlan, error) {
	close(p.started)
	select {
	case <-p.release:
		return p.plan, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
