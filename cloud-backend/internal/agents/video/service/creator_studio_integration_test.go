package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestCreatorStudioTargetRegenerationAndAssemblyKeepOtherNinetyNineShotsByteEquivalent(t *testing.T) {
	store := newFakeCreationProjectStore()
	shots := make([]model.ShotUnit, 100)
	for index := range shots {
		shotID := fmt.Sprintf("shot-%03d", index+1)
		candidateID := "candidate-" + shotID
		shots[index] = acceptedProductionShot(shotID, index+1, candidateID)
	}
	store.project = projectWithShotState(t, shots...)
	svc := NewCreationService(store, &recordingShotDispatcher{store: store})
	before := decodeStateFromTest(t, store.project.Config)
	beforeOther := marshalUntouchedShots(t, before.Shots)

	request := RegenerateShotRequest{BaseVersion: 1, Scope: "base_media", Locks: []string{"duration"}, IdempotencyKey: "regen-shot-012"}
	first, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", request)
	if err != nil {
		t.Fatalf("regenerate Shot 12: %v", err)
	}
	duplicate, err := svc.RegenerateShotV2(context.Background(), "u-1", "vp-1", "shot-012", request)
	if err != nil || duplicate.Task.TaskID != first.Task.TaskID || duplicate.Shot.ID != first.Shot.ID || duplicate.Shot.Version != first.Shot.Version {
		t.Fatalf("duplicate regeneration = %+v, %v; want durable first result %+v", duplicate, err, first)
	}
	if err := svc.CompleteShotRegeneration(context.Background(), "u-1", "vp-1", ShotRegenerationProvenance{TaskID: first.Task.TaskID, RunID: first.Task.RunID, ShotID: first.Task.ShotID}, productionCandidate("candidate-shot-012-v2", "shot-012")); err != nil {
		t.Fatalf("complete regenerated Shot 12: %v", err)
	}
	if _, err := svc.AcceptShotCandidate(context.Background(), "u-1", "vp-1", "shot-012", "candidate-shot-012-v2", CandidateMutationRequest{
		BaseVersion: 2, Scope: CandidateAcceptScope, IdempotencyKey: "accept-shot-012-v2",
	}); err != nil {
		t.Fatalf("accept regenerated Shot 12 candidate: %v", err)
	}
	middle := decodeStateFromTest(t, store.updated.Config)
	if !middle.AssemblyDirty || middle.Shots[11].AcceptedCandidateID != "candidate-shot-012-v2" {
		t.Fatalf("target acceptance did not leave assembly dirty: %+v", middle.Shots[11])
	}
	if got := marshalUntouchedShots(t, middle.Shots); !reflect.DeepEqual(got, beforeOther) {
		t.Fatal("regenerate and acceptance changed a non-target Shot")
	}

	rebuilt, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "assembly-shot-012-v2")
	if err != nil || rebuilt.Status != "validated" || !rebuilt.AssemblyDirty || rebuilt.AcceptedShotCount != 100 || len(rebuilt.Issues) != 0 {
		t.Fatalf("rebuild = %+v, error = %v", rebuilt, err)
	}
	secondRebuild, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "assembly-shot-012-v2")
	if err != nil || !reflect.DeepEqual(secondRebuild, rebuilt) {
		t.Fatalf("duplicate rebuild = %+v, %v; want %+v", secondRebuild, err, rebuilt)
	}
	after := decodeStateFromTest(t, store.updated.Config)
	if !after.AssemblyDirty || after.AssemblyReceipts["assembly-shot-012-v2"].Plan.AcceptedShots[11].CandidateID != "candidate-shot-012-v2" {
		t.Fatalf("assembly receipt did not contain the accepted current candidate: %+v", after.AssemblyReceipts)
	}
	if got := marshalUntouchedShots(t, after.Shots); !reflect.DeepEqual(got, beforeOther) {
		t.Fatal("assembly changed a non-target Shot")
	}
}

func TestCreatorStudioAssemblyRetainsDirtyStateWhenAcceptedCandidateIsInvalid(t *testing.T) {
	store := newFakeCreationProjectStore()
	invalid := acceptedProductionShot("shot-001", 1, "candidate-001")
	invalid.Candidates[0].DurationSec = 15
	store.project = projectWithShotState(t, invalid)
	state := decodeStateFromTest(t, store.project.Config)
	state.AssemblyDirty = true
	setProjectStateForTest(t, store.project, state)
	svc := NewCreationService(store)
	result, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "assembly-invalid")
	if err != nil || result.Status != "blocked" || !result.AssemblyDirty || len(result.Issues) == 0 {
		t.Fatalf("invalid assembly result = %+v, err=%v", result, err)
	}
}

func TestCreatorStudioAssemblyDispatchClaimQueuesAndClearsOnlyMatchingSnapshot(t *testing.T) {
	store := newFakeCreationProjectStore()
	store.project = projectWithShotState(t, acceptedProductionShot("shot-001", 1, "candidate-001"))
	state := decodeStateFromTest(t, store.project.Config)
	state.AssemblyDirty = true
	setProjectStateForTest(t, store.project, state)
	svc := NewCreationService(store)

	if result, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "assembly-1"); err != nil || result.Status != "validated" {
		t.Fatalf("rebuild = %+v, err=%v", result, err)
	}
	if result, err := svc.ClaimFinalAssemblyDispatch(context.Background(), "u-1", "vp-1", "assembly-1"); err != nil || result.Status != "dispatching" {
		t.Fatalf("claim = %+v, err=%v", result, err)
	}
	if result, err := svc.MarkFinalAssemblyQueued(context.Background(), "u-1", "vp-1", "assembly-1", "preview-1", "run-1"); err != nil || result.Status != "queued" || result.AssemblyDirty {
		t.Fatalf("mark matching snapshot = %+v, err=%v", result, err)
	}
	queued := decodeStateFromTest(t, store.project.Config)
	if queued.AssemblyDirty || queued.AssemblyReceipts["assembly-1"].Status != "queued" {
		t.Fatalf("queued state = %+v", queued.AssemblyReceipts["assembly-1"])
	}

	queued.AssemblyDirty = true
	queued.Shots[0].AcceptedCandidateID = "candidate-concurrent"
	setProjectStateForTest(t, store.project, queued)
	result, err := svc.MarkFinalAssemblyQueued(context.Background(), "u-1", "vp-1", "assembly-1", "preview-2", "run-2")
	if !errors.Is(err, ErrShotIdempotencyConflict) {
		t.Fatalf("concurrent mark = %+v, err=%v", result, err)
	}
	afterConflict := decodeStateFromTest(t, store.project.Config)
	if !afterConflict.AssemblyDirty || afterConflict.AssemblyReceipts["assembly-1"].PreviewTaskID != "run-1" {
		t.Fatalf("concurrent Shot update was cleared: %+v", afterConflict.AssemblyReceipts["assembly-1"])
	}
}

func acceptedProductionShot(shotID string, sequenceIndex int, candidateID string) model.ShotUnit {
	return model.ShotUnit{
		ID: shotID, ProjectID: "vp-1", SequenceIndex: sequenceIndex, DurationSec: 6, Version: 1,
		ReviewStatus: model.ReviewStatusApproved, AcceptedCandidateID: candidateID,
		Candidates: []model.ShotCandidate{productionCandidate(candidateID, shotID)},
	}
}

func productionCandidate(candidateID, shotID string) model.ShotCandidate {
	return model.ShotCandidate{
		CandidateID: candidateID, ShotID: shotID, DurationSec: 6, Status: model.CandidateShotQAPassed,
		ExecutionMode: model.ExecutionModeReal, ProductionEligible: true,
		ArtifactRefs: model.ShotArtifactRefs{VideoClipArtifactID: "video-" + candidateID},
		QAReport:     &model.ShotQAReport{CandidateID: candidateID, ShotID: shotID, Status: model.ShotQAPassed, Passed: true},
	}
}

func marshalUntouchedShots(t *testing.T, shots []model.ShotUnit) []byte {
	t.Helper()
	untouched := make([]model.ShotUnit, 0, len(shots)-1)
	for _, shot := range shots {
		if shot.ID != "shot-012" {
			untouched = append(untouched, shot)
		}
	}
	encoded, err := json.Marshal(untouched)
	if err != nil {
		t.Fatalf("marshal untouched Shots: %v", err)
	}
	return encoded
}
