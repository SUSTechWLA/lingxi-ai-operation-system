package agentruntime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerStartAsyncUsesCallerSuppliedRunID(t *testing.T) {
	req := StartRunRequest{RunID: "agent_run_shot_stable", UserID: "u-1", Message: "regenerate"}
	if shell := newRunShell(req); shell.ID != req.RunID {
		t.Fatalf("run id=%q, want %q", shell.ID, req.RunID)
	}
}

func TestRunnerTerminalEventRetriesUntilAcknowledged(t *testing.T) {
	store := newMemoryRunStore()
	var attempts atomic.Int32
	runner := NewRunner(&fakeOrchestrator{}, store, staticPlanner{}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			if attempts.Add(1) == 1 {
				return errors.New("temporary callback failure")
			}
			return nil
		})
	run := &Run{ID: "agent_run_shot_stable", UserID: "u-1", Status: RunStatusFailed}
	event := RunTerminalEvent{RunID: run.ID, UserID: run.UserID, Status: run.Status, Context: map[string]interface{}{"projectId": "vp-1"}}
	if err := runner.persistAndDeliverTerminal(context.Background(), run, event); err == nil {
		t.Fatal("first callback failure was not reported")
	}
	if !store.hasPendingTerminal(run.ID) || store.terminalDelivered(run.ID) {
		t.Fatal("failed callback was not left durable and pending")
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 10); err != nil {
		t.Fatalf("retry terminal delivery: %v", err)
	}
	if attempts.Load() != 2 || !store.terminalDelivered(run.ID) {
		t.Fatalf("attempts=%d delivered=%v", attempts.Load(), store.terminalDelivered(run.ID))
	}
}

func TestRunnerNeverEmitsTerminalWhenTerminalSaveFails(t *testing.T) {
	store := newMemoryRunStore()
	store.saveTerminalErr = errors.New("database unavailable")
	var callbacks atomic.Int32
	runner := NewRunner(&fakeOrchestrator{}, store, staticPlanner{}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbacks.Add(1)
			return nil
		})
	run := &Run{ID: "agent_run_shot_stable", Status: RunStatusFailed}
	err := runner.persistAndDeliverTerminal(context.Background(), run, RunTerminalEvent{RunID: run.ID, Status: run.Status})
	if err == nil || callbacks.Load() != 0 || store.hasPendingTerminal(run.ID) {
		t.Fatalf("error=%v callbacks=%d pending=%v", err, callbacks.Load(), store.hasPendingTerminal(run.ID))
	}
}

func TestRunnerExistingFailedRunRedeliversPendingTerminalEvent(t *testing.T) {
	store := newMemoryRunStore()
	existing := &Run{ID: "agent_run_shot_stable", UserID: "u-1", Message: "regenerate", Status: RunStatusFailed}
	event := RunTerminalEvent{RunID: existing.ID, UserID: existing.UserID, Status: existing.Status}
	if err := store.SaveRunTerminal(context.Background(), existing, event); err != nil {
		t.Fatalf("seed terminal run: %v", err)
	}
	var callbacks atomic.Int32
	runner := NewRunner(&fakeOrchestrator{}, store, staticPlanner{}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbacks.Add(1)
			return nil
		})
	run, err := runner.StartAsync(context.Background(), StartRunRequest{RunID: existing.ID, UserID: "u-1", Message: "regenerate"})
	if err != nil || run.Status != RunStatusFailed || callbacks.Load() != 1 || !store.terminalDelivered(existing.ID) {
		t.Fatalf("run=%+v error=%v callbacks=%d delivered=%v", run, err, callbacks.Load(), store.terminalDelivered(existing.ID))
	}
}

func TestRunnerFirstDurableTerminalWinsOverReplacementAttempt(t *testing.T) {
	store := newMemoryRunStore()
	run := &Run{ID: "agent_run_shot_stable", Status: RunStatusFailed}
	first := RunTerminalEvent{EventID: "event-1", RunID: run.ID, Status: RunStatusFailed, Error: "first"}
	if err := store.SaveRunTerminal(context.Background(), run, first); err != nil {
		t.Fatalf("seed first event: %v", err)
	}
	claimed, err := store.ClaimTerminalEvents(context.Background(), 1, time.Now().Add(time.Minute), "claim-1")
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim first event: deliveries=%+v error=%v", claimed, err)
	}
	second := RunTerminalEvent{EventID: "event-2", RunID: run.ID, Status: RunStatusCancelled, Error: "replacement"}
	if err := store.SaveRunTerminal(context.Background(), run, second); err != nil {
		t.Fatalf("save replacement event: %v", err)
	}
	if marked, err := store.MarkTerminalCallbackDelivered(context.Background(), claimed[0]); err != nil || !marked {
		t.Fatalf("first terminal callback phase accepted=%v error=%v", marked, err)
	}
	if marked, err := store.MarkTerminalObservabilityDelivered(context.Background(), claimed[0]); err != nil || !marked {
		t.Fatalf("first terminal observability phase accepted=%v error=%v", marked, err)
	}
	if acked, err := store.AckTerminalEvent(context.Background(), claimed[0]); err != nil || !acked {
		t.Fatalf("first terminal ack accepted=%v error=%v", acked, err)
	}
	if released, err := store.ReleaseTerminalEvent(context.Background(), claimed[0]); err != nil || released {
		t.Fatalf("acked terminal release accepted=%v error=%v", released, err)
	}
	newer, err := store.ClaimTerminalEvents(context.Background(), 1, time.Now().Add(time.Minute), "claim-2")
	if err != nil || len(newer) != 0 {
		t.Fatalf("replacement event became deliverable: deliveries=%+v error=%v", newer, err)
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
