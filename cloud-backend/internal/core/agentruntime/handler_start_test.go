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

func TestStartRunMarksLinkedProjectAndReturnsCreatedRun(t *testing.T) {
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
	run, _ := store.FindRun(context.Background(), projectUpdater.runID)
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
	if body.Data.Status != RunStatusCreated {
		t.Fatalf("response status = %q, want CREATED", body.Data.Status)
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

func TestStartRunIdempotencyKeyReturnsPersistedRunWithoutPlanningAgain(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	planner := &blockingPlanner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		plan: &AgentPlan{Goal: "make video", Domain: "video_creation", Mode: "dynamic_agent", Steps: []AgentStep{{
			ID: "script", Tool: "video_script_generator", ExpectedOutput: []string{"script"},
		}}},
	}
	catalog := staticToolCatalog{"video_script_generator": &tool.ToolManifest{Name: "video_script_generator"}}
	handler := NewHandler(
		NewRunner(&fakeOrchestrator{taskID: "task-1"}, store, planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)),
		nil,
		nil,
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	handler.RegisterRoutes(router)

	start := func(key string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/runs", bytes.NewBufferString(`{"message":"make a video","domain":"video_creation"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data struct {
				RunID string `json:"runId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return body.Data.RunID
	}

	firstID := start("creator-start:project-1")
	select {
	case <-planner.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("planner was not invoked for the first request")
	}
	secondID := start("creator-start:project-1")
	if secondID != firstID {
		t.Fatalf("duplicate run id = %q, want %q", secondID, firstID)
	}
	if store.Len() != 1 {
		t.Fatalf("stored runs = %d, want one idempotent run", store.Len())
	}
	close(planner.release)
}

func TestIdempotentRunIDFitsPersistentStoreColumn(t *testing.T) {
	const persistentRunIDLimit = 64
	runID := idempotentRunID("user-with-a-realistic-identifier", "creator-start:vp-4f158892")
	if len(runID) > persistentRunIDLimit {
		t.Fatalf("idempotent run id length = %d, want at most %d: %q", len(runID), persistentRunIDLimit, runID)
	}
}

func TestStartRunIdempotencyKeySeparatesDifferentKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	catalog := staticToolCatalog{"video_script_generator": &tool.ToolManifest{Name: "video_script_generator"}}
	handler := NewHandler(
		NewRunner(&fakeOrchestrator{taskID: "task-1"}, store, staticPlanner{plan: &AgentPlan{Goal: "make video", Steps: []AgentStep{{ID: "script", Tool: "video_script_generator"}}}}, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)),
		nil,
		nil,
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	handler.RegisterRoutes(router)

	start := func(key string) string {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/runs", bytes.NewBufferString(`{"message":"make a video"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data struct {
				RunID string `json:"runId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return body.Data.RunID
	}

	if first, second := start("creator-start:project-1"), start("creator-start:project-2"); first == second {
		t.Fatalf("different idempotency keys returned the same run %q", first)
	}
	if store.Len() != 2 {
		t.Fatalf("stored runs = %d, want two distinct runs", store.Len())
	}
}

func TestStartRunIdempotencyKeyRejectsDifferentProjectWithoutRelinking(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	catalog := staticToolCatalog{"video_script_generator": &tool.ToolManifest{Name: "video_script_generator"}}
	updater := &recordingProjectLifecycleUpdater{}
	handler := NewHandler(
		NewRunner(&fakeOrchestrator{taskID: "task-1"}, store, staticPlanner{plan: &AgentPlan{Goal: "make video", Steps: []AgentStep{{ID: "script", Tool: "video_script_generator"}}}}, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog)),
		nil,
		nil,
	).WithProjectLifecycleUpdater(updater)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	handler.RegisterRoutes(router)

	start := func(projectID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/runs", bytes.NewBufferString(`{"message":"make a video","domain":"video_creation","mode":"dynamic_agent","context":{"projectId":"`+projectID+`"}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "creator-start:project-a")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := start("project-a"); rec.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := start("project-b"); rec.Code != http.StatusConflict {
		t.Fatalf("conflicting request status=%d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if store.Len() != 1 {
		t.Fatalf("stored runs = %d, want one", store.Len())
	}
	if updater.projectID != "project-a" {
		t.Fatalf("project lifecycle updated project %q, want project-a only", updater.projectID)
	}
}

func TestCancelRunPausesTaskAndMarksProjectStopped(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMemoryRunStore()
	_ = store.SaveRun(context.Background(), &Run{
		ID:     "run-1",
		TaskID: "task-1",
		UserID: "user-1",
		Status: RunStatusRunning,
	})
	projectUpdater := &recordingProjectLifecycleUpdater{}
	taskPauser := &recordingTaskPauser{}
	handler := NewHandler(
		NewRunner(&fakeOrchestrator{taskID: "task-1"}, store, staticPlanner{}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)),
		nil,
		nil,
	).WithProjectLifecycleUpdater(projectUpdater).WithTaskPauser(taskPauser)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "user-1")
		c.Next()
	})
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/runs/run-1/cancel", bytes.NewBufferString(`{"projectId":"project-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	run, _ := store.FindRun(context.Background(), "run-1")
	if run == nil || run.Status != RunStatusCancelled {
		t.Fatalf("run status = %#v, want CANCELLED", run)
	}
	if taskPauser.taskID != "task-1" {
		t.Fatalf("task pauser taskID = %q, want task-1", taskPauser.taskID)
	}
	if projectUpdater.stoppedProjectID != "project-1" || projectUpdater.stoppedRunID != "run-1" {
		t.Fatalf("project stop updater got project=%q run=%q", projectUpdater.stoppedProjectID, projectUpdater.stoppedRunID)
	}
}

type recordingProjectLifecycleUpdater struct {
	userID           string
	projectID        string
	runID            string
	stoppedUserID    string
	stoppedProjectID string
	stoppedRunID     string
}

func (u *recordingProjectLifecycleUpdater) MarkAgentRunStarted(_ context.Context, userID, projectID, runID string) error {
	u.userID = userID
	u.projectID = projectID
	u.runID = runID
	return nil
}

func (u *recordingProjectLifecycleUpdater) MarkAgentRunStopped(_ context.Context, userID, projectID, runID string) error {
	u.stoppedUserID = userID
	u.stoppedProjectID = projectID
	u.stoppedRunID = runID
	return nil
}

var _ ProjectLifecycleUpdater = (*recordingProjectLifecycleUpdater)(nil)

type recordingTaskPauser struct {
	taskID string
	reason string
}

func (p *recordingTaskPauser) PauseTask(_ context.Context, taskID, reason string) error {
	p.taskID = taskID
	p.reason = reason
	return nil
}

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
