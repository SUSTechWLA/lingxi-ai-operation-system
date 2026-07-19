package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestRegenerateShotV2MutatesOnlyTargetShot(t *testing.T) {
	store := newFakeCreationProjectStore()
	shots := make([]model.ShotUnit, 100)
	for i := range shots {
		shots[i] = model.ShotUnit{
			ID: fmt.Sprintf("shot-%03d", i+1), ProjectID: "vp-1",
			DurationSec: 6, Version: 1, ReviewStatus: model.ReviewStatusApproved,
		}
	}
	store.project = projectWithShotState(t, shots...)
	svc := NewCreationService(store)
	before := append([]model.ShotUnit(nil), shots...)

	result, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", RegenerateShotRequest{
		BaseVersion: 1, Scope: "base_media", Locks: []string{"duration", "character"},
		Instruction: "晨光更柔和", IdempotencyKey: "regen-shot-012-v1",
	})
	if err != nil {
		t.Fatalf("RegenerateShotV2 error: %v", err)
	}
	if result.Shot.ID != "shot-012" || result.Shot.Version != 2 {
		t.Fatalf("target result = %+v", result.Shot)
	}
	after := decodeStateFromTest(t, store.updated.Config)
	for i, shot := range after.Shots {
		if shot.ID == "shot-012" {
			continue
		}
		if !reflect.DeepEqual(shot, before[i]) {
			t.Fatalf("non-target shot changed: before=%+v after=%+v", before[i], shot)
		}
	}
	if !after.AssemblyDirty || after.Shots[11].AcceptedCandidateID != "" {
		t.Fatalf("target invalidation not persisted: %+v", after.Shots[11])
	}
}

func TestRegenerateShotV2IsIdempotentAndHistoryIncludesCurrentVersion(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{
		ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1,
		ReviewStatus: model.ReviewStatusApproved, AcceptedCandidateID: "candidate-v1",
	})
	svc := NewCreationService(store)
	req := RegenerateShotRequest{BaseVersion: 1, Scope: "base_media", IdempotencyKey: "request-1"}

	first, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("first regeneration: %v", err)
	}
	retryReq := req
	retryReq.BaseVersion = 999
	retryReq.Scope = "invalid-on-retry"
	retryReq.Locks = []string{"invalid-on-retry"}
	second, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", retryReq)
	if err != nil {
		t.Fatalf("idempotent regeneration: %v", err)
	}
	if first.Task.TaskID == "" || second.Task.TaskID != first.Task.TaskID || second.Shot.Version != 2 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	history, err := svc.GetShotHistory(context.Background(), "u-1", "vp-1", "shot-012")
	if err != nil || len(history) != 2 {
		t.Fatalf("history=%+v error=%v", history, err)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if len(state.ShotHistory["shot-012"]) != 1 || len(state.RegenerationTasks) != 1 {
		t.Fatalf("duplicate request mutated durable history/tasks: %+v", state)
	}
}

func TestRegenerateShotV2PersistsBeforeDispatchAndStoresRunID(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1})
	dispatcher := &recordingShotDispatcher{store: store, runID: "agent-run-1"}
	svc := NewCreationService(store, dispatcher)

	result, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", RegenerateShotRequest{
		BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("RegenerateShotV2 error: %v", err)
	}
	if !dispatcher.sawDurableQueuedTask {
		t.Fatal("dispatcher called before queued task was durable")
	}
	if result.Task.RunID != "agent-run-1" {
		t.Fatalf("result task = %+v", result.Task)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if state.RegenerationTasks[result.Task.TaskID].RunID != "agent-run-1" {
		t.Fatalf("run id not durable: %+v", state.RegenerationTasks[result.Task.TaskID])
	}
}

func TestRegenerateShotV2DispatchFailureFailsOnlyTargetTask(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1},
		model.ShotUnit{ID: "shot-013", ProjectID: "vp-1", DurationSec: 6, Version: 1},
	)
	state := decodeStateFromTest(t, store.project.Config)
	state.RegenerationTasks["other-task"] = model.ShotRegenerationTask{TaskID: "other-task", ShotID: "shot-013", Status: "queued"}
	setProjectStateForTest(t, store.project, state)
	svc := NewCreationService(store, &recordingShotDispatcher{store: store, err: errors.New("agent unavailable")})

	result, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", RegenerateShotRequest{
		BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1",
	})
	if err == nil || !strings.Contains(err.Error(), "agent unavailable") {
		t.Fatalf("error = %v", err)
	}
	after := decodeStateFromTest(t, store.updated.Config)
	if after.RegenerationTasks[result.Task.TaskID].Status != "failed" {
		t.Fatalf("target task not failed: %+v", after.RegenerationTasks[result.Task.TaskID])
	}
	if after.RegenerationTasks["other-task"].Status != "queued" || after.Shots[1].Version != 1 {
		t.Fatalf("dispatch failure escaped target: %+v", after)
	}
}

func TestRegenerateShotV2ValidatesVersionScopeLocksAndDuration(t *testing.T) {
	base := model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 3}
	tests := []struct {
		name string
		shot model.ShotUnit
		req  RegenerateShotRequest
		want string
	}{
		{name: "version", shot: base, req: RegenerateShotRequest{BaseVersion: 2, Scope: "prompt", IdempotencyKey: "k"}, want: "version conflict"},
		{name: "scope", shot: base, req: RegenerateShotRequest{BaseVersion: 3, Scope: "everything", IdempotencyKey: "k"}, want: "scope"},
		{name: "lock", shot: base, req: RegenerateShotRequest{BaseVersion: 3, Scope: "prompt", Locks: []string{"lighting"}, IdempotencyKey: "k"}, want: "lock"},
		{name: "duration", shot: model.ShotUnit{ID: "shot-012", DurationSec: 15, Version: 3}, req: RegenerateShotRequest{BaseVersion: 3, Scope: "prompt", IdempotencyKey: "k"}, want: "less than 15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeCreationProjectStore()
			store.project = projectWithShotState(t, tt.shot)
			_, err := NewCreationService(store).RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want substring %q", err, tt.want)
			}
		})
	}
}

func TestCompleteShotRegenerationValidatesDurableTargetAndIsIdempotent(t *testing.T) {
	store, svc, taskID := projectWithQueuedRegeneration(t)
	wrong := model.ShotCandidate{CandidateID: "candidate-wrong", ShotID: "shot-013", DurationSec: 6}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", taskID, wrong); err == nil {
		t.Fatal("expected mismatched candidate target to fail")
	}
	state := decodeStateFromTest(t, store.project.Config)
	if len(state.Shots[0].Candidates) != 0 || state.RegenerationTasks[taskID].Status != "queued" {
		t.Fatalf("mismatched completion mutated state: %+v", state)
	}

	candidate := model.ShotCandidate{CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: 6}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", taskID, candidate); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", taskID, candidate); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
	state = decodeStateFromTest(t, store.project.Config)
	if len(state.Shots[0].Candidates) != 1 || state.RegenerationTasks[taskID].Status != "completed" {
		t.Fatalf("completion state: %+v", state)
	}
}

func TestFailShotRegenerationMutatesOnlyDurableTask(t *testing.T) {
	store, svc, taskID := projectWithQueuedRegeneration(t)
	before := decodeStateFromTest(t, store.project.Config).Shots
	if err := svc.FailShotRegeneration(context.Background(), "u-1", "vp-1", taskID, "provider timeout"); err != nil {
		t.Fatalf("fail regeneration: %v", err)
	}
	state := decodeStateFromTest(t, store.project.Config)
	if !reflect.DeepEqual(state.Shots, before) || state.RegenerationTasks[taskID].Status != "failed" {
		t.Fatalf("failure escaped durable task: %+v", state)
	}
}

func TestPreviewShotRegenerationReportsTargetOnlyImpact(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1},
		model.ShotUnit{ID: "shot-013", ProjectID: "vp-1", DurationSec: 6, Version: 1},
	)
	impact, err := NewCreationService(store).PreviewShotRegeneration(context.Background(), "u-1", "vp-1", "shot-012")
	if err != nil || !reflect.DeepEqual(impact.AffectedShotIDs, []string{"shot-012"}) || impact.RegeneratesOtherShots {
		t.Fatalf("impact=%+v error=%v", impact, err)
	}
}

func TestLegacyRegenerateShotDerivesStableRetryKeyBeforeDefaultingVersion(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{
		ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 1,
	})
	svc := NewCreationService(store)
	req := RegenerateShotRequest{Scope: "text_layers", Instruction: "文字更大"}
	first, err := svc.RegenerateShot(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("legacy regeneration: %v", err)
	}
	second, err := svc.RegenerateShot(context.Background(), "u-1", "vp-1", "shot-012", req)
	if err != nil {
		t.Fatalf("legacy retry: %v", err)
	}
	state := decodeStateFromTest(t, store.project.Config)
	if first.Version != 2 || second.Version != 2 || len(state.RegenerationTasks) != 1 || len(state.ShotHistory["shot-012"]) != 1 {
		t.Fatalf("legacy retry was not idempotent: first=%+v second=%+v state=%+v", first, second, state)
	}
}

type recordingShotDispatcher struct {
	store                *fakeCreationProjectStore
	runID                string
	err                  error
	sawDurableQueuedTask bool
}

func (d *recordingShotDispatcher) EnqueueShotRegeneration(_ context.Context, _, _ string, task model.ShotRegenerationTask) (string, error) {
	if d.store != nil && d.store.project != nil {
		state, err := DecodeShotDrivenState(d.store.project.Config)
		if err == nil {
			durable, ok := state.RegenerationTasks[task.TaskID]
			d.sawDurableQueuedTask = ok && durable.Status == "queued"
		}
	}
	return d.runID, d.err
}

func projectWithQueuedRegeneration(t *testing.T) (*fakeCreationProjectStore, *CreationService, string) {
	t.Helper()
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-012", ProjectID: "vp-1", DurationSec: 6, Version: 2},
		model.ShotUnit{ID: "shot-013", ProjectID: "vp-1", DurationSec: 6, Version: 1},
	)
	state := decodeStateFromTest(t, store.project.Config)
	taskID := "regen-task-1"
	state.RegenerationTasks[taskID] = model.ShotRegenerationTask{TaskID: taskID, ShotID: "shot-012", Status: "queued"}
	setProjectStateForTest(t, store.project, state)
	return store, NewCreationService(store), taskID
}

func setProjectStateForTest(t *testing.T, project *model.VideoProject, state model.ShotDrivenState) {
	t.Helper()
	raw, err := EncodeShotDrivenState(project.Config, state)
	if err != nil {
		t.Fatalf("EncodeShotDrivenState: %v", err)
	}
	project.Config = raw
}
