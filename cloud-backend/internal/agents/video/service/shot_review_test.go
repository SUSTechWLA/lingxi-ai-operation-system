package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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
	second, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", req)
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
	dispatcher := &recordingShotDispatcher{store: store}
	svc := NewCreationService(store, dispatcher)

	result, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", RegenerateShotRequest{
		BaseVersion: 1, Scope: "prompt", IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("RegenerateShotV2 error: %v", err)
	}
	if !dispatcher.sawDurableQueuedTask {
		t.Fatalf("dispatcher called before queued task was durable; statuses=%v project=%s", store.persistedTaskStatuses, store.project.Config)
	}
	if result.Task.RunID == "" {
		t.Fatalf("result task = %+v", result.Task)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if state.RegenerationTasks[result.Task.TaskID].RunID != result.Task.RunID {
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
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), wrong); err == nil {
		t.Fatal("expected mismatched candidate target to fail")
	}
	state := decodeStateFromTest(t, store.project.Config)
	if len(state.Shots[0].Candidates) != 0 || state.RegenerationTasks[taskID].Status != "queued" {
		t.Fatalf("mismatched completion mutated state: %+v", state)
	}

	candidate := model.ShotCandidate{CandidateID: "candidate-2", ShotID: "shot-012", DurationSec: 6}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), candidate); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), candidate); err != nil {
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
	if err := svc.FailShotRegeneration(context.Background(), "u-1", "vp-1", provenanceForTask(taskID), "provider timeout"); err != nil {
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

func TestListShotPagePaginatesOneHundredShotsWithStableCursor(t *testing.T) {
	store := newFakeCreationProjectStore()
	shots := make([]model.ShotUnit, 100)
	for i := range shots {
		shots[i] = model.ShotUnit{
			ID: fmt.Sprintf("shot-%03d", i+1), ProjectID: "vp-1", SequenceIndex: i + 1,
			Title: fmt.Sprintf("Shot %03d", i+1), DurationSec: 6, Version: 3,
			ReviewStatus: model.ReviewStatusPending,
		}
	}
	store.project = projectWithShotState(t, shots...)
	svc := NewCreationService(store)

	page, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{
		Limit: 24, Status: model.ReviewStatusPending,
	})
	if err != nil || len(page.Items) != 24 || page.NextCursor == "" || page.Total != 100 {
		t.Fatalf("page = %+v error = %v", page, err)
	}
	if page.Items[0].SequenceIndex != 1 || page.Items[23].SequenceIndex != 24 {
		t.Fatalf("first page sequence indexes = %d..%d", page.Items[0].SequenceIndex, page.Items[23].SequenceIndex)
	}
	next, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Cursor: page.NextCursor, Limit: 24})
	if err != nil || len(next.Items) != 24 || next.Items[0].SequenceIndex != 25 || next.Items[23].SequenceIndex != 48 {
		t.Fatalf("next page = %+v error = %v", next, err)
	}
	if _, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Cursor: "not-an-index"}); err == nil {
		t.Fatal("invalid cursor should fail")
	}
	clamped, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 99})
	if err != nil || len(clamped.Items) != 50 {
		t.Fatalf("clamped page = %+v error = %v", clamped, err)
	}
	minimum, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: -1})
	if err != nil || len(minimum.Items) != 1 {
		t.Fatalf("minimum page = %+v error = %v", minimum, err)
	}
}

func TestListShotPageSearchesCreatorReviewFields(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-title", ProjectID: "vp-1", SequenceIndex: 1, Title: "Morning light", DurationSec: 6},
		model.ShotUnit{ID: "shot-narration", ProjectID: "vp-1", SequenceIndex: 2, Narration: "A useful hook", DurationSec: 6},
		model.ShotUnit{ID: "shot-scene", ProjectID: "vp-1", SequenceIndex: 3, SceneSummary: "A quiet library", DurationSec: 6},
		model.ShotUnit{ID: "shot-text", ProjectID: "vp-1", SequenceIndex: 4, ScreenText: []string{"Keep this exact phrase"}, DurationSec: 6},
	)
	svc := NewCreationService(store)
	for _, query := range []string{"morning", "hook", "library", "exact phrase"} {
		page, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Query: query})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("query %q page=%+v error=%v", query, page, err)
		}
	}
}

func TestRestoreShotCandidateCreatesNewImmutableCandidate(t *testing.T) {
	store := newFakeCreationProjectStore()
	historical := model.ShotCandidate{
		CandidateID: "candidate-v1", ShotID: "shot-001", DurationSec: 6,
		Status:       model.CandidateShotQAPassed,
		QAReport:     &model.ShotQAReport{Status: model.ShotQAPassed, Passed: true},
		ArtifactRefs: model.ShotArtifactRefs{VideoClipArtifactID: "artifact-video-v1"},
	}
	store.project = projectWithShotState(t, model.ShotUnit{
		ID: "shot-001", ProjectID: "vp-1", SequenceIndex: 1, DurationSec: 6, Version: 3,
		AcceptedCandidateID: "candidate-v2", Candidates: []model.ShotCandidate{historical},
	})
	svc := NewCreationService(store)

	restored, err := svc.RestoreShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-v1", candidateMutationRequestForTest(CandidateRestoreScope, 3))
	if err != nil || restored.Version != 4 || restored.AcceptedCandidateID == "candidate-v1" {
		t.Fatalf("restored shot = %+v error = %v", restored, err)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if len(state.Shots[0].Candidates) != 2 || state.Shots[0].Candidates[0].CandidateID != "candidate-v1" {
		t.Fatalf("historical candidate was changed: %+v", state.Shots[0].Candidates)
	}
	copy := state.Shots[0].Candidates[1]
	if copy.CandidateID == historical.CandidateID || copy.ArtifactRefs != historical.ArtifactRefs || copy.Status != model.CandidateAcceptedForAssembly || !state.AssemblyDirty {
		t.Fatalf("restored copy = %+v", copy)
	}
	if _, err := svc.RestoreShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-v1", candidateMutationRequestForTest(CandidateRestoreScope, 2)); !errors.Is(err, ErrShotVersionConflict) {
		t.Fatalf("duplicate restore error = %v, want version conflict", err)
	}
}

func TestAcceptShotCandidateRequiresOwnedQAPassedCandidateAndExactVersion(t *testing.T) {
	base := model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3}
	valid := model.ShotCandidate{
		CandidateID: "candidate-ok", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateShotQAPassed,
		QAReport: &model.ShotQAReport{Status: model.ShotQAPassed, Passed: true},
	}
	tests := []struct {
		name      string
		candidate model.ShotCandidate
		base      int
		want      string
	}{
		{name: "wrong target", candidate: model.ShotCandidate{CandidateID: "candidate-wrong", ShotID: "shot-002", DurationSec: 6, Status: model.CandidateShotQAPassed, QAReport: valid.QAReport}, base: 3, want: "belongs"},
		{name: "qa gate", candidate: model.ShotCandidate{CandidateID: "candidate-qa", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateShotQAFailed}, base: 3, want: "QA"},
		{name: "strict duration", candidate: model.ShotCandidate{CandidateID: "candidate-15", ShotID: "shot-001", DurationSec: 15, Status: model.CandidateShotQAPassed, QAReport: valid.QAReport}, base: 3, want: "less than 15"},
		{name: "base version", candidate: valid, base: 2, want: "version conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeCreationProjectStore()
			shot := base
			shot.Candidates = []model.ShotCandidate{tt.candidate}
			store.project = projectWithShotState(t, shot)
			_, err := NewCreationService(store).AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", tt.candidate.CandidateID, candidateMutationRequestForTest(CandidateAcceptScope, tt.base))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			state := decodeStateFromTest(t, store.project.Config)
			if state.Shots[0].Version != 3 || state.Shots[0].AcceptedCandidateID != "" {
				t.Fatalf("rejected acceptance mutated state: %+v", state.Shots[0])
			}
		})
	}
}

func TestAcceptShotCandidateAcceptsHumanReviewRequiredAndRejectsStaleDuplicate(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{
		ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3,
		Candidates: []model.ShotCandidate{{
			CandidateID: "candidate-review", ShotID: "shot-001", DurationSec: 6,
			Status: model.CandidateHumanReviewRequired,
		}}},
	)
	svc := NewCreationService(store)
	accepted, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-review", candidateMutationRequestForTest(CandidateAcceptScope, 3))
	if err != nil || accepted.Version != 4 || accepted.AcceptedCandidateID != "candidate-review" || accepted.ReviewStatus != model.ReviewStatusApproved {
		t.Fatalf("accepted=%+v error=%v", accepted, err)
	}
	if _, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-review", candidateMutationRequestForTest(CandidateAcceptScope, 2)); !errors.Is(err, ErrShotVersionConflict) {
		t.Fatalf("duplicate accept error = %v, want version conflict", err)
	}
}

func TestAcceptShotCandidateRetriesCASWithoutChangingOtherShots(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3, Candidates: []model.ShotCandidate{{
			CandidateID: "candidate-ok", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateShotQAPassed,
			QAReport: &model.ShotQAReport{Status: model.ShotQAPassed, Passed: true},
		}}},
		model.ShotUnit{ID: "shot-002", ProjectID: "vp-1", DurationSec: 6, Version: 7},
	)
	store.casConflicts = 1
	accepted, err := NewCreationService(store).AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-ok", candidateMutationRequestForTest(CandidateAcceptScope, 3))
	if err != nil || accepted.Version != 4 || store.casCalls != 2 {
		t.Fatalf("accepted=%+v error=%v CAS calls=%d", accepted, err, store.casCalls)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if state.Shots[1].Version != 7 || state.Shots[1].AcceptedCandidateID != "" {
		t.Fatalf("CAS retry changed non-target shot: %+v", state.Shots[1])
	}
}

func TestAcceptShotCandidatePreservesImmutableCandidateAndReturnsDurableRetryReceipt(t *testing.T) {
	store := newFakeCreationProjectStore()
	original := model.ShotCandidate{
		CandidateID: "candidate-ok", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateShotQAPassed,
		QAReport: &model.ShotQAReport{Status: model.ShotQAPassed, Passed: true, Scores: map[string]int{"visual": 95}},
	}
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3, Candidates: []model.ShotCandidate{original}})
	svc := NewCreationService(store)
	req := CandidateMutationRequest{BaseVersion: 3, Scope: "candidate_accept", IdempotencyKey: "accept-1"}

	first, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-ok", req)
	if err != nil {
		t.Fatalf("accept candidate: %v", err)
	}
	second, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-ok", req)
	firstJSON, firstJSONErr := json.Marshal(first)
	secondJSON, secondJSONErr := json.Marshal(second)
	if err != nil || firstJSONErr != nil || secondJSONErr != nil || string(firstJSON) != string(secondJSON) {
		t.Fatalf("retry result first=%+v second=%+v error=%v", first, second, err)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	if !reflect.DeepEqual(state.Shots[0].Candidates[0], original) || len(state.ShotHistory["shot-001"]) != 1 || len(state.ShotMutationReceipts) != 1 {
		t.Fatalf("accept mutated immutable candidate or receipt/history: %+v", state)
	}
	if _, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-other", req); !errors.Is(err, ErrShotIdempotencyConflict) {
		t.Fatalf("mismatched idempotency reuse error=%v", err)
	}
}

func TestCandidateMutationsRequireOperationScopeLocksAndIdempotencyKey(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3, Candidates: []model.ShotCandidate{{
		CandidateID: "candidate-ok", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateHumanReviewRequired,
	}}})
	svc := NewCreationService(store)
	for _, req := range []CandidateMutationRequest{
		{BaseVersion: 3, Scope: "candidate_accept"},
		{BaseVersion: 3, Scope: "base_media", IdempotencyKey: "k"},
		{BaseVersion: 3, Scope: "candidate_accept", Locks: []string{"unknown"}, IdempotencyKey: "k"},
	} {
		if _, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-ok", req); err == nil {
			t.Fatalf("request %+v should be rejected", req)
		}
	}
}

func TestRestoreShotCandidateDeepClonesNestedCandidateData(t *testing.T) {
	store := newFakeCreationProjectStore()
	historical := model.ShotCandidate{
		CandidateID: "candidate-v1", ShotID: "shot-001", DurationSec: 6, Status: model.CandidateShotQAPassed,
		QAReport:   &model.ShotQAReport{Status: model.ShotQAPassed, Passed: true, Scores: map[string]int{"visual": 98}, PassedDimensions: []string{"visual"}},
		RepairPlan: &model.RepairPlan{LockedDimensions: []string{"visual"}, RepairTargets: []string{"text"}, PromptPatch: map[string]interface{}{"nested": map[string]interface{}{"tone": "warm"}}, RenderStrategyPatch: map[string]interface{}{"layers": []interface{}{"base"}}},
	}
	store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: 6, Version: 3, Candidates: []model.ShotCandidate{historical}})
	restored, err := NewCreationService(store).RestoreShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-v1", CandidateMutationRequest{BaseVersion: 3, Scope: "candidate_restore", IdempotencyKey: "restore-1"})
	if err != nil || restored.AcceptedCandidateID == "candidate-v1" {
		t.Fatalf("restored=%+v error=%v", restored, err)
	}
	state := decodeStateFromTest(t, store.updated.Config)
	original := state.Shots[0].Candidates[0]
	copy := &state.Shots[0].Candidates[1]
	copy.QAReport.Scores["visual"] = 1
	copy.RepairPlan.LockedDimensions[0] = "changed"
	copy.RepairPlan.PromptPatch["nested"].(map[string]interface{})["tone"] = "cool"
	copy.RepairPlan.RenderStrategyPatch["layers"].([]interface{})[0] = "changed"
	if !reflect.DeepEqual(original, historical) {
		t.Fatalf("historical candidate changed after restored copy mutation: %+v", original)
	}
}

func TestListShotPageCursorRetainsDuplicateSequenceIndexesAndAcceptedOnlyThumbnail(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-b", ProjectID: "vp-1", SequenceIndex: 1, DurationSec: 6, ArtifactRefs: model.ShotArtifactRefs{KeyframeImageArtifactID: "must-not-leak"}},
		model.ShotUnit{ID: "shot-a", ProjectID: "vp-1", SequenceIndex: 1, DurationSec: 6},
		model.ShotUnit{ID: "shot-c", ProjectID: "vp-1", SequenceIndex: 2, DurationSec: 6},
	)
	svc := NewCreationService(store)
	first, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 1})
	if err != nil || first.Items[0].ID != "shot-a" || first.Items[0].ThumbnailRef != "" || first.NextCursor == "1" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	second, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 1, Cursor: first.NextCursor})
	if err != nil || second.Items[0].ID != "shot-b" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	third, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 1, Cursor: second.NextCursor})
	if err != nil || third.Items[0].ID != "shot-c" {
		t.Fatalf("third=%+v error=%v", third, err)
	}
}

func TestShotPageAndSummarySurfaceLatestFailedOrCancelledTask(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-failed", ProjectID: "vp-1", SequenceIndex: 1, DurationSec: 6, ReviewStatus: model.ReviewStatusPending},
		model.ShotUnit{ID: "shot-cancelled", ProjectID: "vp-1", SequenceIndex: 2, DurationSec: 6, ReviewStatus: model.ReviewStatusPending},
	)
	state := decodeStateFromTest(t, store.project.Config)
	state.RegenerationTasks["failed"] = model.ShotRegenerationTask{TaskID: "failed", ShotID: "shot-failed", Status: ShotRegenerationFailed, UpdatedAt: time.Now()}
	state.RegenerationTasks["cancelled"] = model.ShotRegenerationTask{TaskID: "cancelled", ShotID: "shot-cancelled", Status: ShotRegenerationCancelled, UpdatedAt: time.Now()}
	setProjectStateForTest(t, store.project, state)
	svc := NewCreationService(store)
	page, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 10})
	if err != nil || page.Items[0].GenerationStatus != ShotRegenerationFailed || page.Items[1].GenerationStatus != ShotRegenerationCancelled {
		t.Fatalf("page=%+v error=%v", page, err)
	}
	summary, err := svc.GetShotSummary(context.Background(), "u-1", "vp-1")
	if err != nil || summary.NeedsAction != 2 || summary.Generating != 0 {
		t.Fatalf("summary=%+v error=%v", summary, err)
	}
}

func TestCandidateMutationsRejectInvalidShotOrCandidateDurationBeforeReceipt(t *testing.T) {
	tests := []struct {
		name              string
		shotDuration      int
		candidateDuration float64
	}{
		{name: "zero shot", shotDuration: 0, candidateDuration: 6},
		{name: "fifteen second shot", shotDuration: 15, candidateDuration: 6},
		{name: "zero candidate", shotDuration: 6, candidateDuration: 0},
		{name: "fifteen second candidate", shotDuration: 6, candidateDuration: 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeCreationProjectStore()
			store.project = projectWithShotState(t, model.ShotUnit{ID: "shot-001", ProjectID: "vp-1", DurationSec: tt.shotDuration, Version: 3, Candidates: []model.ShotCandidate{{
				CandidateID: "candidate-1", ShotID: "shot-001", DurationSec: tt.candidateDuration, Status: model.CandidateHumanReviewRequired,
			}}})
			svc := NewCreationService(store)
			if _, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-1", CandidateMutationRequest{BaseVersion: 3, Scope: CandidateAcceptScope, IdempotencyKey: "accept-1"}); err == nil {
				t.Fatal("accept should reject invalid duration")
			}
			if _, err := svc.RestoreShotCandidate(context.Background(), "u-1", "vp-1", "shot-001", "candidate-1", CandidateMutationRequest{BaseVersion: 3, Scope: CandidateRestoreScope, IdempotencyKey: "restore-1"}); err == nil {
				t.Fatal("restore should reject invalid duration")
			}
			state := decodeStateFromTest(t, store.project.Config)
			if state.Shots[0].Version != 3 || len(state.ShotHistory["shot-001"]) != 0 || len(state.ShotMutationReceipts) != 0 {
				t.Fatalf("invalid duration mutated durable state: %+v", state)
			}
		})
	}
}

func TestShotPageAndSummaryUseOneDeterministicLatestTask(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t,
		model.ShotUnit{ID: "shot-completed", ProjectID: "vp-1", SequenceIndex: 1, DurationSec: 6, ReviewStatus: model.ReviewStatusPending},
		model.ShotUnit{ID: "shot-failed", ProjectID: "vp-1", SequenceIndex: 2, DurationSec: 6, ReviewStatus: model.ReviewStatusPending},
	)
	state := decodeStateFromTest(t, store.project.Config)
	base := time.Now().Round(0)
	state.RegenerationTasks["queued-old"] = model.ShotRegenerationTask{TaskID: "queued-old", ShotID: "shot-completed", Status: ShotRegenerationQueued, CreatedAt: base, UpdatedAt: base}
	state.RegenerationTasks["completed-new"] = model.ShotRegenerationTask{TaskID: "completed-new", ShotID: "shot-completed", Status: ShotRegenerationCompleted, CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second)}
	state.RegenerationTasks["running-old"] = model.ShotRegenerationTask{TaskID: "running-old", ShotID: "shot-failed", Status: ShotRegenerationRunning, CreatedAt: base, UpdatedAt: base}
	state.RegenerationTasks["failed-new"] = model.ShotRegenerationTask{TaskID: "z-failed-new", ShotID: "shot-failed", Status: ShotRegenerationFailed, CreatedAt: base, UpdatedAt: base}
	setProjectStateForTest(t, store.project, state)
	page, err := NewCreationService(store).ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Limit: 10})
	if err != nil || page.Items[0].GenerationStatus != model.ShotPlanned || page.Items[1].GenerationStatus != ShotRegenerationFailed {
		t.Fatalf("page=%+v error=%v", page, err)
	}
	summary, err := NewCreationService(store).GetShotSummary(context.Background(), "u-1", "vp-1")
	if err != nil || summary.Generating != 0 || summary.NeedsAction != 1 || summary.AwaitingReview != 1 {
		t.Fatalf("summary=%+v error=%v", summary, err)
	}
}

func TestListShotPageFriendlyStatusFiltersFullSetBeforePagination(t *testing.T) {
	store := newFakeCreationProjectStore()
	shots := make([]model.ShotUnit, 100)
	for index := range shots {
		shots[index] = model.ShotUnit{
			ID: fmt.Sprintf("shot-%03d", index+1), ProjectID: "vp-1", SequenceIndex: index + 1,
			DurationSec: 6, Version: 1, ReviewStatus: model.ReviewStatusApproved,
		}
	}
	shots[0].ReviewStatus = model.ReviewStatusPending
	shots[49].ReviewStatus = model.ReviewStatusRejected
	shots[89].ReviewStatus = model.ReviewStatusStale
	shots[96].QAStatus = "SHOT_QA_FAILED"
	store.project = projectWithShotState(t, shots...)
	state := decodeStateFromTest(t, store.project.Config)
	state.RegenerationTasks["queued"] = model.ShotRegenerationTask{TaskID: "queued", ShotID: "shot-020", Status: ShotRegenerationQueued, UpdatedAt: time.Now()}
	state.RegenerationTasks["failed"] = model.ShotRegenerationTask{TaskID: "failed", ShotID: "shot-095", Status: ShotRegenerationFailed, UpdatedAt: time.Now()}
	setProjectStateForTest(t, store.project, state)

	svc := NewCreationService(store)
	attention, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Status: "needs_attention", Limit: 2})
	if err != nil {
		t.Fatalf("needs attention page: %v", err)
	}
	if attention.Total != 5 || len(attention.Items) != 2 || attention.Items[0].ID != "shot-001" || attention.Items[1].ID != "shot-050" || attention.NextCursor == "" {
		t.Fatalf("friendly filter must be applied before cursor pagination: %+v", attention)
	}
	second, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Status: "needs_attention", Limit: 2, Cursor: attention.NextCursor})
	if err != nil || len(second.Items) != 2 || second.Items[0].ID != "shot-090" || second.Items[1].ID != "shot-095" {
		t.Fatalf("second friendly filter page=%+v error=%v", second, err)
	}
	third, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Status: "needs_attention", Limit: 2, Cursor: second.NextCursor})
	if err != nil || len(third.Items) != 1 || third.Items[0].ID != "shot-097" {
		t.Fatalf("QA failure must remain in attention queue: page=%+v error=%v", third, err)
	}
	generating, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Status: "generating", Limit: 24})
	if err != nil || generating.Total != 1 || generating.Items[0].ID != "shot-020" {
		t.Fatalf("generating page=%+v error=%v", generating, err)
	}
	failed, err := svc.ListShotPage(context.Background(), "u-1", "vp-1", model.ShotPageQuery{Status: "failed", Limit: 24})
	if err != nil || failed.Total != 2 || len(failed.Items) != 2 || failed.Items[0].ID != "shot-095" || failed.Items[1].ID != "shot-097" {
		t.Fatalf("failed page=%+v error=%v", failed, err)
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
	calls                int
}

func (d *recordingShotDispatcher) EnqueueShotRegeneration(_ context.Context, _, _ string, task model.ShotRegenerationTask) (string, error) {
	d.calls++
	if d.store != nil && d.store.project != nil {
		state, err := DecodeShotDrivenState(d.store.project.Config)
		if err == nil {
			durable, ok := state.RegenerationTasks[task.TaskID]
			d.sawDurableQueuedTask = ok && durable.Status == ShotRegenerationDispatching && d.store.hasPersistedTaskStatus(ShotRegenerationQueued)
		}
	}
	if d.runID == "" {
		d.runID = task.RunID
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
	state.RegenerationTasks[taskID] = model.ShotRegenerationTask{TaskID: taskID, RunID: shotRegenerationRunID(taskID), ShotID: "shot-012", Status: ShotRegenerationQueued}
	setProjectStateForTest(t, store.project, state)
	return store, NewCreationService(store), taskID
}

func provenanceForTask(taskID string) ShotRegenerationProvenance {
	return ShotRegenerationProvenance{TaskID: taskID, RunID: shotRegenerationRunID(taskID), ShotID: "shot-012"}
}

func candidateMutationRequestForTest(scope string, baseVersion int) CandidateMutationRequest {
	return CandidateMutationRequest{BaseVersion: baseVersion, Scope: scope, IdempotencyKey: fmt.Sprintf("%s-%d", scope, baseVersion)}
}

func setProjectStateForTest(t *testing.T, project *model.VideoProject, state model.ShotDrivenState) {
	t.Helper()
	raw, err := EncodeShotDrivenState(project.Config, state)
	if err != nil {
		t.Fatalf("EncodeShotDrivenState: %v", err)
	}
	project.Config = raw
}
