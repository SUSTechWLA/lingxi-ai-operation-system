package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunnerStartAsyncUsesCallerSuppliedRunID(t *testing.T) {
	req := StartRunRequest{RunID: "agent_run_shot_stable", UserID: "u-1", Message: "regenerate"}
	if shell := newRunShell(req); shell.ID != req.RunID {
		t.Fatalf("run id=%q, want %q", shell.ID, req.RunID)
	}
}

func TestRunnerStartAsyncReturnsExistingCallerSuppliedRun(t *testing.T) {
	store := newMemoryRunStore()
	existing := &Run{
		ID: "agent_run_shot_stable", UserID: "u-1", Message: "regenerate",
		Status: RunStatusRunning, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := store.SaveRun(context.Background(), existing); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	runner := NewRunner(&fakeOrchestrator{}, store, staticPlanner{err: errors.New("must not replan")}, NewPlanGuard(nil, nil), NewPlanCompiler(nil))
	run, err := runner.StartAsync(context.Background(), StartRunRequest{
		RunID: existing.ID, UserID: "u-1", Message: "regenerate",
	})
	if err != nil {
		t.Fatalf("StartAsync retry: %v", err)
	}
	if run.ID != existing.ID || run.Status != RunStatusRunning {
		t.Fatalf("run=%+v, want existing running run", run)
	}
	time.Sleep(25 * time.Millisecond)
	stored, _ := store.FindRun(context.Background(), existing.ID)
	if stored.Status != RunStatusRunning {
		t.Fatalf("retry replanned and rewrote durable run: %+v", stored)
	}
}

func TestRunnerBackgroundPlanningFailureEmitsScopedTerminalEvent(t *testing.T) {
	store := newMemoryRunStore()
	events := make(chan RunTerminalEvent, 1)
	runner := NewRunner(&fakeOrchestrator{}, store, staticPlanner{err: errors.New("planner unavailable")}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).
		WithTerminalCallback(func(_ context.Context, event RunTerminalEvent) error {
			events <- event
			return nil
		})
	req := StartRunRequest{
		RunID: "agent_run_shot_stable", UserID: "u-1", Message: "regenerate", Domain: "video_creation",
		Context: map[string]interface{}{
			"projectId": "vp-1", "targetShotId": "shot-012", "shotRegenerationTaskId": "regen-task-1",
		},
	}
	if _, err := runner.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	select {
	case event := <-events:
		if event.RunID != req.RunID || event.Status != RunStatusFailed || event.UserID != "u-1" ||
			event.Context["projectId"] != "vp-1" || event.Context["targetShotId"] != "shot-012" ||
			event.Context["shotRegenerationTaskId"] != "regen-task-1" {
			t.Fatalf("terminal event=%+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal event")
	}
}
