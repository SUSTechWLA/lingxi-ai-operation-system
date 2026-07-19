package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestRegenerateShotV2RetriesCASWithoutLosingConcurrentShotUpdate(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1},
		model.ShotUnit{ID: "shot-013", ProjectID: "vp-1", DurationSec: 6, Version: 1},
	)
	store.casConflicts = 1
	store.onCASConflict = func(project *model.VideoProject) {
		state := decodeStateFromTest(t, project.Config)
		state.Shots[1].ReviewStatus = model.ReviewStatusApproved
		setProjectStateForTest(t, project, state)
	}
	dispatcher := &recordingShotDispatcher{store: store}

	result, err := NewCreationService(store, dispatcher).RegenerateShotV2(
		context.Background(), "u-1", "vp-1", "shot-012",
		RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1"},
	)
	if err != nil {
		t.Fatalf("RegenerateShotV2: %v", err)
	}
	state := decodeStateFromTest(t, store.project.Config)
	if result.Shot.Version != 2 || state.Shots[1].ReviewStatus != model.ReviewStatusApproved {
		t.Fatalf("CAS retry lost state: result=%+v state=%+v", result, state)
	}
	if len(state.ShotHistory["shot-012"]) != 1 || len(state.RegenerationTasks) != 1 || dispatcher.calls != 1 {
		t.Fatalf("CAS retry duplicated mutation or dispatch: state=%+v calls=%d", state, dispatcher.calls)
	}
}

func TestRegenerateShotV2NeverDispatchesWhenTaskCASIsLost(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1})
	store.casConflicts = maxProjectCASAttempts + 1
	dispatcher := &recordingShotDispatcher{store: store}

	_, err := NewCreationService(store, dispatcher).RegenerateShotV2(
		context.Background(), "u-1", "vp-1", "shot-012",
		RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1"},
	)
	if !errors.Is(err, ErrShotVersionConflict) || dispatcher.calls != 0 {
		t.Fatalf("error=%v dispatch calls=%d", err, dispatcher.calls)
	}
}

func TestRegenerateShotV2RejectsMismatchedIdempotencyFingerprint(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1})
	svc := NewCreationService(store)
	first := RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", Instruction: "warmer", IdempotencyKey: "request-1"}
	if _, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", first); err != nil {
		t.Fatalf("first request: %v", err)
	}
	second := first
	second.Instruction = "colder"
	_, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", second)
	if !errors.Is(err, ErrShotIdempotencyConflict) {
		t.Fatalf("error=%v, want ErrShotIdempotencyConflict", err)
	}
}

func TestRegenerateShotV2ScopesIdempotencyKeyToShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1},
		model.ShotUnit{ID: "shot-013", ProjectID: "vp-1", DurationSec: 6, Version: 1},
	)
	svc := NewCreationService(store)
	req := RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", IdempotencyKey: "same-client-key"}
	first, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	second, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-013", req)
	if err != nil {
		t.Fatalf("second Shot request: %v", err)
	}
	if first.Task.TaskID == second.Task.TaskID || second.Shot.ID != "shot-013" || second.Shot.Version != 2 {
		t.Fatalf("idempotency escaped Shot scope: first=%+v second=%+v", first, second)
	}
}

func TestRegenerateShotV2RetryResumesDurableQueuedTaskWithoutBumpingShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1})
	req := RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1"}
	first, err := NewCreationService(store).RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("persist queued request: %v", err)
	}
	dispatcher := &recordingShotDispatcher{store: store}
	second, err := NewCreationService(store, dispatcher).RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("resume queued request: %v", err)
	}
	state := decodeStateFromTest(t, store.project.Config)
	if second.Task.TaskID != first.Task.TaskID || second.Shot.Version != 2 || dispatcher.calls != 1 {
		t.Fatalf("queued recovery changed identity/version: first=%+v second=%+v calls=%d", first, second, dispatcher.calls)
	}
	if len(state.ShotHistory["shot-012"]) != 1 || state.RegenerationTasks[first.Task.TaskID].Status != ShotRegenerationRunning {
		t.Fatalf("queued recovery state=%+v", state)
	}
}

func TestRegenerateShotV2RetryReclaimsExpiredDispatchLease(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 2})
	req := RegenerateShotRequest{BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1"}
	state := decodeStateFromTest(t, store.project.Config)
	fingerprint := shotRegenerationFingerprint("shot-012", req)
	task := model.ShotRegenerationTask{
		TaskID: "regen-task-1", RunID: shotRegenerationRunID("regen-task-1"), ShotID: "shot-012",
		BaseVersion: 1, Scope: "prompt", IdempotencyKey: req.IdempotencyKey,
		RequestFingerprint: fingerprint, Status: ShotRegenerationDispatching,
		DispatchLeaseUntil: time.Now().Add(-time.Minute),
	}
	state.RegenerationTasks[task.TaskID] = task
	state.IdempotencyTasks[shotRegenerationIdempotencyScope("shot-012", req.IdempotencyKey)] = task.TaskID
	state.ShotHistory["shot-012"] = []model.ShotRevision{{ShotID: "shot-012", Version: 1}}
	setProjectStateForTest(t, store.project, state)
	dispatcher := &recordingShotDispatcher{store: store}

	result, err := NewCreationService(store, dispatcher).RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil || dispatcher.calls != 1 || result.Task.TaskID != task.TaskID || result.Shot.Version != 2 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, dispatcher.calls)
	}
}

func TestCompleteShotRegenerationRejectsInvalidCandidateDuration(t *testing.T) {
	for _, duration := range []float64{0, 15} {
		t.Run(time.Duration(duration*1000).String(), func(t *testing.T) {
			store, svc, taskID := projectWithQueuedRegeneration(t)
			err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), model.ShotCandidate{
				CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: duration,
			})
			if err == nil {
				t.Fatalf("duration %v accepted", duration)
			}
			state := decodeStateFromTest(t, store.project.Config)
			if len(state.Shots[0].Candidates) != 0 || state.RegenerationTasks[taskID].Status != ShotRegenerationQueued {
				t.Fatalf("invalid candidate mutated state: %+v", state)
			}
		})
	}
}

func TestShotRegenerationTerminalStatesAreImmutable(t *testing.T) {
	store, svc, taskID := projectWithQueuedRegeneration(t)
	if err := svc.FailShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), "first failure"); err != nil {
		t.Fatalf("initial failure: %v", err)
	}
	failed := decodeStateFromTest(t, store.project.Config).RegenerationTasks[taskID]
	if err := svc.FailShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), "rewrite failure"); err != nil {
		t.Fatalf("duplicate failure: %v", err)
	}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), model.ShotCandidate{
		CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: 6,
	}); err != nil {
		t.Fatalf("completion after failure should be idempotent no-op: %v", err)
	}
	after := decodeStateFromTest(t, store.project.Config)
	if after.RegenerationTasks[taskID].Status != ShotRegenerationFailed ||
		after.RegenerationTasks[taskID].FailureReason != failed.FailureReason ||
		!after.RegenerationTasks[taskID].UpdatedAt.Equal(failed.UpdatedAt) || len(after.Shots[0].Candidates) != 0 {
		t.Fatalf("failed terminal task was rewritten: before=%+v after=%+v", failed, after)
	}

	completedStore, completedSvc, completedTaskID := projectWithQueuedRegeneration(t)
	candidate := model.ShotCandidate{CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: 6}
	if err := completedSvc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(completedTaskID), candidate); err != nil {
		t.Fatalf("initial completion: %v", err)
	}
	completed := decodeStateFromTest(t, completedStore.project.Config).RegenerationTasks[completedTaskID]
	if err := completedSvc.FailShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(completedTaskID), "late failure"); err != nil {
		t.Fatalf("failure after completion should be no-op: %v", err)
	}
	completedAfter := decodeStateFromTest(t, completedStore.project.Config)
	if completedAfter.RegenerationTasks[completedTaskID].Status != ShotRegenerationCompleted ||
		!completedAfter.RegenerationTasks[completedTaskID].UpdatedAt.Equal(completed.UpdatedAt) || len(completedAfter.Shots[0].Candidates) != 1 {
		t.Fatalf("completed terminal task was rewritten: before=%+v after=%+v", completed, completedAfter)
	}
}

func TestShotRegenerationCompletionValidatesDurableProvenance(t *testing.T) {
	store, svc, taskID := projectWithQueuedRegeneration(t)
	candidate := model.ShotCandidate{CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: 6}
	err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", ShotRegenerationProvenance{
		TaskID: taskID, RunID: "wrong-run", ShotID: "shot-012",
	}, candidate)
	if err == nil {
		t.Fatal("completion accepted mismatched durable run provenance")
	}
	state := decodeStateFromTest(t, store.project.Config)
	if len(state.Shots[0].Candidates) != 0 || state.RegenerationTasks[taskID].Status != ShotRegenerationQueued {
		t.Fatalf("mismatched provenance mutated state: %+v", state)
	}
}
