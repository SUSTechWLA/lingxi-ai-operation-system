package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

func TestCreatorViewMapsCurrentArtifactsAndShotSummaryIntoSixSteps(t *testing.T) {
	now := time.Now()
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{
			Summary: model.ShotSummary{Total: 32, Confirmed: 31, NeedsAction: 1},
			Tasks:   []model.ShotRegenerationTask{{TaskID: "shot-task-1", ShotID: "shot-032", Scope: "full_shot", Status: ShotRegenerationFailed, UpdatedAt: now}},
		}},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "brief-v1", StageName: "brief", Version: 1, Status: "valid", HumanApproved: true},
			{ID: "direction-v1", StageName: "proposal", Version: 1, Status: "valid", HumanApproved: true},
			{ID: "script-v3", StageName: "script", Version: 3, Status: "valid", HumanApproved: true, WorkflowRunID: "run-script"},
			{ID: "storyboard-v2", StageName: "storyboard", Version: 2, Status: "stale"},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}

	want := []model.CreatorStepState{
		model.CreatorStepConfirmed,
		model.CreatorStepConfirmed,
		model.CreatorStepConfirmed,
		model.CreatorStepNeedsAttention,
		model.CreatorStepNotStarted,
		model.CreatorStepNotStarted,
	}
	if got := stepStates(view.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("step states = %v, want %v", got, want)
	}
	if len(view.Steps) != 6 || view.Steps[2].CurrentArtifactID != "script-v3" || view.Steps[2].CurrentVersion != 3 || view.Steps[2].RunID != "run-script" {
		t.Fatalf("script step = %+v, want current script lineage", view.Steps[2])
	}
	if view.ActiveStep != model.CreatorStepShots {
		t.Fatalf("active step = %q, want %q", view.ActiveStep, model.CreatorStepShots)
	}
}

func TestCreatorViewUsesPriorityIgnoresUnknownStagesAndExposesDurableActiveTasks(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{
			Summary: model.ShotSummary{Total: 1, Confirmed: 1},
			Tasks: []model.ShotRegenerationTask{
				{TaskID: "task-b", ShotID: "shot-2", Scope: "full_shot", Status: ShotRegenerationRunning},
				{TaskID: "task-a", ShotID: "shot-1", Scope: "prompt", Status: ShotRegenerationQueued},
				{TaskID: "task-done", ShotID: "shot-3", Scope: "prompt", Status: ShotRegenerationCompleted},
			},
		}},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "unknown-failed", StageName: "developer_experiment", Version: 99, Status: "failed"},
			{ID: "script-approved", StageName: "script", Version: 1, Status: "valid", HumanApproved: true},
			{ID: "script-pending", StageName: "script", Version: 2, Status: "pending", TaskID: "artifact-task"},
			{ID: "script-failed", StageName: "script", Version: 3, Status: "failed"},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}

	if got := view.Steps[2].State; got != model.CreatorStepFailed {
		t.Fatalf("script state = %q, want failed", got)
	}
	if view.Steps[2].CurrentArtifactID != "script-failed" {
		t.Fatalf("primary script artifact = %q, want failed artifact", view.Steps[2].CurrentArtifactID)
	}
	if got := view.Steps[3].State; got != model.CreatorStepConfirmed {
		t.Fatalf("shots state = %q, want confirmed; unknown stage must not affect a creator step", got)
	}
	wantTasks := []model.CreatorTask{
		{ID: "task-a", Scope: "shots", ShotID: "shot-1", Status: ShotRegenerationQueued, Label: "正在重新生成镜头"},
		{ID: "task-b", Scope: "shots", ShotID: "shot-2", Status: ShotRegenerationRunning, Label: "正在重新生成镜头"},
	}
	if !reflect.DeepEqual(view.ActiveTasks, wantTasks) {
		t.Fatalf("active tasks = %#v, want %#v", view.ActiveTasks, wantTasks)
	}
	if got := view.Steps[2].AllowedActions; !reflect.DeepEqual(got, []string{"view", "retry"}) {
		t.Fatalf("failed actions = %v", got)
	}
}

func TestCreatorViewNeedsReviewAdvertisesConfirmAction(t *testing.T) {
	if got := actionsForCreatorState(model.CreatorStepNeedsReview); !reflect.DeepEqual(got, []string{"view", "confirm", "revise"}) {
		t.Fatalf("needs-review actions = %v, want creator confirmation contract", got)
	}
}

func TestCreatorViewMakesPreviewAndDeliveryNeedAttentionWhenAssemblyIsDirty(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{Summary: model.ShotSummary{Total: 1, Confirmed: 1}, AssemblyDirty: true}},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "preview-v1", StageName: "preview", Version: 1, Status: "valid", HumanApproved: true},
			{ID: "package-v1", StageName: "package", Version: 1, Status: "valid", HumanApproved: true},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if got := stepStates(view.Steps); !reflect.DeepEqual(got, []model.CreatorStepState{
		model.CreatorStepNotStarted, model.CreatorStepNotStarted, model.CreatorStepNotStarted,
		model.CreatorStepConfirmed, model.CreatorStepNeedsAttention, model.CreatorStepNeedsAttention,
	}) {
		t.Fatalf("step states = %v", got)
	}
	if !view.AssemblyDirty || view.ActiveStep != model.CreatorStepPreview {
		t.Fatalf("view = %+v, want dirty preview active", view)
	}
}

func TestCreatorViewHasSixNotStartedStepsWithoutArtifacts(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{},
		fakeCreatorArtifactReader{},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if len(view.Steps) != 6 || view.ActiveStep != model.CreatorStepRequirements {
		t.Fatalf("view = %+v, want six steps with requirements active", view)
	}
	for _, step := range view.Steps {
		if step.State != model.CreatorStepNotStarted || step.AllowedActions == nil {
			t.Fatalf("step = %+v, want a not-started step with stable actions", step)
		}
	}
}

func TestCreatorViewMapsMixedArtifactStatesWithFixedPriority(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{Summary: model.ShotSummary{Total: 1, Confirmed: 1}}},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "brief-stale", StageName: "brief", Status: "stale"},
			{ID: "direction-review", StageName: "proposal", Status: "pending"},
			{ID: "script-generating", StageName: "script", Status: "generating"},
			{ID: "preview-failed", StageName: "preview", Status: "failed"},
			{ID: "package-approved", StageName: "package", Status: "valid", HumanApproved: true},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	want := []model.CreatorStepState{
		model.CreatorStepNeedsAttention,
		model.CreatorStepNeedsReview,
		model.CreatorStepGenerating,
		model.CreatorStepConfirmed,
		model.CreatorStepFailed,
		model.CreatorStepConfirmed,
	}
	if got := stepStates(view.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("step states = %v, want %v", got, want)
	}
}

func TestCreatorViewUsesOnlyLatestDurableTaskPerShot(t *testing.T) {
	base := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	for _, terminal := range []string{ShotRegenerationCompleted, ShotRegenerationFailed, ShotRegenerationCancelled} {
		t.Run(terminal, func(t *testing.T) {
			store := newFakeCreationProjectStore()
			store.project = projectWithShotState(t, model.ShotUnit{
				ID: "shot-1", ProjectID: "vp-1", DurationSec: 6, ReviewStatus: model.ReviewStatusApproved,
			})
			state := decodeStateFromTest(t, store.project.Config)
			state.RegenerationTasks["old-running"] = model.ShotRegenerationTask{
				TaskID: "old-running", ShotID: "shot-1", Status: ShotRegenerationRunning,
				UpdatedAt: base, CreatedAt: base,
			}
			state.RegenerationTasks["latest-terminal"] = model.ShotRegenerationTask{
				TaskID: "latest-terminal", ShotID: "shot-1", Status: terminal,
				UpdatedAt: base.Add(time.Second), CreatedAt: base,
			}
			setProjectStateForTest(t, store.project, state)

			view, err := NewCreatorViewService(
				fakeCreatorProjectReader{project: store.project}, NewCreationService(store), fakeCreatorArtifactReader{},
			).GetCreationView(context.Background(), "u-1", "vp-1")
			if err != nil {
				t.Fatalf("GetCreationView() error = %v", err)
			}
			if len(view.ActiveTasks) != 0 {
				t.Fatalf("active tasks = %+v, terminal latest task must hide older running work", view.ActiveTasks)
			}
			if terminal == ShotRegenerationCompleted {
				if view.ShotSummary.Confirmed != 1 || view.ShotSummary.NeedsAction != 0 || view.ShotSummary.Generating != 0 {
					t.Fatalf("summary = %+v, want completed latest task", view.ShotSummary)
				}
			} else if view.ShotSummary.NeedsAction != 1 || view.ShotSummary.Generating != 0 {
				t.Fatalf("summary = %+v, want terminal failure/cancellation", view.ShotSummary)
			}
		})
	}
}

func TestCreatorViewLatestShotTaskUsesUpdatedCreatedAndTaskIDOrdering(t *testing.T) {
	base := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	for _, tasks := range [][]model.ShotRegenerationTask{
		{
			{TaskID: "running", ShotID: "shot-1", Status: ShotRegenerationRunning, UpdatedAt: base, CreatedAt: base},
			{TaskID: "completed", ShotID: "shot-1", Status: ShotRegenerationCompleted, UpdatedAt: base, CreatedAt: base.Add(time.Second)},
		},
		{
			{TaskID: "a-running", ShotID: "shot-1", Status: ShotRegenerationRunning, UpdatedAt: base, CreatedAt: base},
			{TaskID: "z-completed", ShotID: "shot-1", Status: ShotRegenerationCompleted, UpdatedAt: base, CreatedAt: base},
		},
	} {
		latest := latestCreatorShotTasks(tasks)
		if len(latest) != 1 || latest[0].Status != ShotRegenerationCompleted {
			t.Fatalf("latest = %+v, want the terminal task selected", latest)
		}
	}
}

func TestCreatorViewDeduplicatesDurableTaskIDsAndPrefersShotTask(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{Tasks: []model.ShotRegenerationTask{
			{TaskID: "shared-task", ShotID: "shot-1", Status: ShotRegenerationRunning},
			{TaskID: "shot-task", ShotID: "shot-2", Status: ShotRegenerationQueued},
		}}},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "direction-run", StageName: "proposal", Status: "running", TaskID: "duplicate-artifact-task"},
			{ID: "script-run", StageName: "script", Status: "running", TaskID: "duplicate-artifact-task"},
			{ID: "preview-run", StageName: "preview", Status: "running", TaskID: "shared-task"},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	want := []model.CreatorTask{
		{ID: "duplicate-artifact-task", Scope: "direction", Status: "running", Label: "正在准备创意方向"},
		{ID: "shared-task", Scope: "shots", ShotID: "shot-1", Status: ShotRegenerationRunning, Label: "正在重新生成镜头"},
		{ID: "shot-task", Scope: "shots", ShotID: "shot-2", Status: ShotRegenerationQueued, Label: "正在重新生成镜头"},
	}
	if !reflect.DeepEqual(view.ActiveTasks, want) {
		t.Fatalf("active tasks = %#v, want %#v", view.ActiveTasks, want)
	}
}

func TestCreatorViewDoesNotTreatPendingArtifactsAsActiveTasks(t *testing.T) {
	view, err := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{},
		fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{
			{ID: "materialized-pending", StageName: "script", Status: "pending", TaskID: "pending-task"},
			{ID: "script-running", StageName: "script", Status: "running", TaskID: "running-task"},
		}},
	).GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil {
		t.Fatalf("GetCreationView() error = %v", err)
	}
	if got := view.Steps[2].State; got != model.CreatorStepNeedsReview {
		t.Fatalf("script state = %q, want needs_review", got)
	}
	want := []model.CreatorTask{{ID: "running-task", Scope: "script", Status: "running", Label: "正在准备脚本"}}
	if !reflect.DeepEqual(view.ActiveTasks, want) {
		t.Fatalf("active tasks = %#v, want %#v", view.ActiveTasks, want)
	}
}

func TestCreatorViewExposesOnlyBackendVerifiedRunAndReviewIDs(t *testing.T) {
	current := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "task-1", TaskID: "task-1", StageName: "script", Version: 3, Status: "valid"}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{current}, history: []*artifact.Artifact{current}}
	for _, tc := range []struct {
		name       string
		reviews    *fakeCreatorReviewMutations
		wantRun    string
		wantReview string
	}{
		{name: "verified", reviews: &fakeCreatorReviewMutations{resolvedRunID: "agent-run-1", resolvedReviewID: "script-review"}, wantRun: "agent-run-1", wantReview: "script-review"},
		{name: "unresolved", reviews: &fakeCreatorReviewMutations{resolveErr: errors.New("ambiguous")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view, err := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).
				WithStepMutations(&fakeCreatorRevisionService{artifacts: artifacts}, tc.reviews).
				GetCreationView(context.Background(), "user-1", "vp-1")
			if err != nil {
				t.Fatalf("GetCreationView() error = %v", err)
			}
			if view.Steps[2].RunID != tc.wantRun || view.Steps[2].ReviewID != tc.wantReview {
				t.Fatalf("ids = run %q review %q", view.Steps[2].RunID, view.Steps[2].ReviewID)
			}
		})
	}
}

func TestConfirmedScriptStepRevisionCreatesNewVersionAndReopensOnlyItsReview(t *testing.T) {
	base := &artifact.Artifact{
		ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1",
		StageName: "script", Version: 3, IsCurrent: true, Status: "valid", HumanApproved: true,
		ProducedByNode: "script-exec",
	}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"}
	svc := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{
			Summary: model.ShotSummary{Total: 2}, ShotIDs: []string{"shot-2", "shot-1"},
		}},
		artifacts,
	).WithStepMutations(revisions, reviews)

	result, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
		IdempotencyKey: "revise-1",
		ArtifactID:     "script-v3", BaseVersion: 3, Mode: "direct", DirectContent: "新版脚本",
		ConfirmedAffectedShotIDs: []string{"shot-2", "shot-1"},
	})
	if err != nil {
		t.Fatalf("ReviseStep() error = %v", err)
	}
	if result.Artifact.Version != 4 || result.View.Steps[2].CurrentVersion != 4 {
		t.Fatalf("result = %+v, want script version 4", result)
	}
	if got, want := result.Impact.AffectedStepIDs, []model.CreatorStepID{model.CreatorStepShots, model.CreatorStepPreview, model.CreatorStepDelivery}; !reflect.DeepEqual(got, want) {
		t.Fatalf("affected steps = %v, want %v", got, want)
	}
	if got, want := result.Impact.AffectedShotIDs, []string{"shot-1", "shot-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("affected shots = %v, want %v", got, want)
	}
	receipt, ok := creatorReceiptFromArtifact(result.Artifact)
	if !ok || receipt.IdempotencyKey != "revise-1" || receipt.BaseArtifactID != "script-v3" || receipt.ParentArtifactID != "script-v3" ||
		receipt.RunID != "run-1" || receipt.ReviewID != "script-review" || !reflect.DeepEqual(receipt.AffectedShotIDs, []string{"shot-1", "shot-2"}) {
		t.Fatalf("receipt = %+v valid=%v", receipt, ok)
	}
	if reviews.reopenedArtifactID != result.Artifact.ID || reviews.reopenedReviewID != "script-review" {
		t.Fatalf("review reopen = %+v, want script-review with new artifact", reviews)
	}
}

func TestStepRevisionImpactUsesExactDurableShotSetAndRejectsUnknownStep(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	svc := NewCreatorViewService(
		fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1"}},
		fakeCreatorShotReader{state: creatorShotReadState{ShotIDs: []string{"shot-c", "shot-a", "shot-b"}}}, artifacts,
	).WithStepMutations(&fakeCreatorRevisionService{artifacts: artifacts}, &fakeCreatorReviewMutations{})

	impact, err := svc.PreviewStepRevision(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{ArtifactID: "script-v3", BaseVersion: 3})
	if err != nil {
		t.Fatalf("PreviewStepRevision() error = %v", err)
	}
	if got, want := impact.AffectedShotIDs, []string{"shot-a", "shot-b", "shot-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("affected shots = %v, want %v", got, want)
	}
	if _, err := svc.PreviewStepRevision(context.Background(), "user-1", "vp-1", model.CreatorStepID("internal-stage"), model.StepRevisionRequest{ArtifactID: "script-v3", BaseVersion: 3}); !errors.Is(err, ErrCreatorStepInvalid) {
		t.Fatalf("unknown step error = %v", err)
	}
}

func TestStepRevisionInstructionPreservesNormalizedSelectionInProvenance(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", Config: json.RawMessage(`{"modelProviders":{"text_to_text":{"baseUrl":"https://model.test","apiKey":"secret","model":"writer"}}}`)}}, fakeCreatorShotReader{}, artifacts).
		WithStepMutations(revisions, reviews)
	x, y, width, height := 0.1, 0.2, 0.3, 0.4
	_, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
		IdempotencyKey: "revise-instruction-1",
		ArtifactID:     "script-v3", BaseVersion: 3, Mode: "instruction", Instruction: "语气更自然",
		Selection: &model.ArtifactSelection{Kind: " RECT ", X: &x, Y: &y, Width: &width, Height: &height},
	})
	if err != nil {
		t.Fatalf("ReviseStep() error = %v", err)
	}
	if revisions.reviseRequest.Message != "语气更自然" {
		t.Fatalf("revision message = %q", revisions.reviseRequest.Message)
	}
	wantSelection := map[string]interface{}{"kind": "rect", "x": x, "y": y, "width": width, "height": height}
	if got := revisions.reviseRequest.Provenance["selection"]; !reflect.DeepEqual(got, wantSelection) {
		t.Fatalf("selection provenance = %#v, want %#v", got, wantSelection)
	}
}

func TestStepRevisionRejectsStaleBaseAndMismatchedImpactBeforeMutation(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{state: creatorShotReadState{ShotIDs: []string{"shot-1"}}}, artifacts).
		WithStepMutations(revisions, &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"})

	_, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
		IdempotencyKey: "revise-stale-1",
		ArtifactID:     "script-v3", BaseVersion: 2, Mode: "direct", DirectContent: "新版", ConfirmedAffectedShotIDs: []string{"shot-1"},
	})
	if !errors.Is(err, ErrCreatorVersionConflict) || revisions.calls != 0 {
		t.Fatalf("stale revision error=%v calls=%d", err, revisions.calls)
	}
	_, err = svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
		IdempotencyKey: "revise-impact-1",
		ArtifactID:     "script-v3", BaseVersion: 3, Mode: "direct", DirectContent: "新版", ConfirmedAffectedShotIDs: []string{"shot-extra"},
	})
	if !errors.Is(err, ErrCreatorImpactMismatch) || revisions.calls != 0 {
		t.Fatalf("impact mismatch error=%v calls=%d", err, revisions.calls)
	}
}

func TestStepRevisionRejectsNonExactModeBeforeMutation(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).
		WithStepMutations(revisions, &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"})

	_, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{
		IdempotencyKey: "revise-mode-1",
		ArtifactID:     "script-v3", BaseVersion: 3, Mode: " DIRECT ", DirectContent: "新版",
	})
	if !errors.Is(err, ErrCreatorInvalidRequest) || revisions.calls != 0 {
		t.Fatalf("mode error=%v calls=%d", err, revisions.calls)
	}
}

func TestStepRestoreCreatesNewCurrentVersionAndReopensReview(t *testing.T) {
	historical := &artifact.Artifact{ID: "script-v1", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 1}
	current := &artifact.Artifact{ID: "script-v4", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 4, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{current}, history: []*artifact.Artifact{current, historical}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{state: creatorShotReadState{ShotIDs: []string{"shot-1"}}}, artifacts).
		WithStepMutations(revisions, reviews)

	result, err := svc.RestoreStepVersion(context.Background(), "user-1", "vp-1", model.CreatorStepScript, 1, model.StepRestoreRequest{
		IdempotencyKey: "restore-1",
		BaseVersion:    4, ConfirmedAffectedShotIDs: []string{"shot-1"},
	})
	if err != nil {
		t.Fatalf("RestoreStepVersion() error = %v", err)
	}
	if result.Artifact.Version != 5 || result.Artifact.ParentID != "script-v4" || result.Artifact.Metadata["restoredFromArtifactId"] != "script-v1" {
		t.Fatalf("restored artifact = %+v", result.Artifact)
	}
	receipt, ok := creatorReceiptFromArtifact(result.Artifact)
	if !ok || receipt.Operation != "restore" || receipt.HistoricalArtifactID != "script-v1" || receipt.HistoricalVersion != 1 || receipt.BaseArtifactID != "script-v4" {
		t.Fatalf("restore receipt=%+v valid=%v", receipt, ok)
	}
	if reviews.reopenedArtifactID != result.Artifact.ID || result.View.Steps[2].CurrentVersion != 5 {
		t.Fatalf("result/review = %+v / %+v", result, reviews)
	}
}

func TestStepRevisionRetryAfterReopenFailureReusesReceiptArtifact(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review", reopenErr: errors.New("temporary reopen failure")}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).WithStepMutations(revisions, reviews)
	req := model.StepRevisionRequest{IdempotencyKey: "same-key", ArtifactID: base.ID, BaseVersion: 3, Mode: "direct", DirectContent: "new"}
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); err == nil {
		t.Fatal("expected reopen failure")
	}
	createdID := artifacts.current[0].ID
	reviews.reopenErr = nil
	result, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req)
	if err != nil {
		t.Fatal(err)
	}
	if revisions.calls != 1 || result.Artifact.ID != createdID || reviews.reopenCalls != 2 {
		t.Fatalf("revision calls=%d artifact=%q/%q reopen=%d", revisions.calls, result.Artifact.ID, createdID, reviews.reopenCalls)
	}
}

func TestStepRevisionSameKeyDifferentIntentConflictsAndConcurrentLoserRecovers(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 3, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts, reviseErrAfterCreate: artifact.ErrArtifactVersionConflict}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review"}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).WithStepMutations(revisions, reviews)
	req := model.StepRevisionRequest{IdempotencyKey: "race-key", ArtifactID: base.ID, BaseVersion: 3, Mode: "direct", DirectContent: "new"}
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); err != nil {
		t.Fatalf("concurrent loser recovery: %v", err)
	}
	req.DirectContent = "different"
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); !errors.Is(err, ErrCreatorIdempotencyConflict) {
		t.Fatalf("different intent error=%v", err)
	}
	req.IdempotencyKey, req.DirectContent = "different-key", "new"
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); !errors.Is(err, ErrCreatorVersionConflict) {
		t.Fatalf("different key error=%v", err)
	}
}

func TestStepRestoreRetryReusesSingleNewVersion(t *testing.T) {
	historical := &artifact.Artifact{ID: "script-v1", ProjectID: "vp-1", StageName: "script", Version: 1}
	current := &artifact.Artifact{ID: "script-v4", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 4, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{current}, history: []*artifact.Artifact{current, historical}}
	revisions := &fakeCreatorRevisionService{artifacts: artifacts}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review", reopenErr: errors.New("temporary")}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{state: creatorShotReadState{ShotIDs: []string{"shot-1"}}}, artifacts).WithStepMutations(revisions, reviews)
	req := model.StepRestoreRequest{IdempotencyKey: "restore-key", BaseVersion: 4, Reason: "undo", ConfirmedAffectedShotIDs: []string{"shot-1"}}
	if _, err := svc.RestoreStepVersion(context.Background(), "user-1", "vp-1", model.CreatorStepScript, 1, req); err == nil {
		t.Fatal("expected reopen failure")
	}
	reviews.reopenErr = nil
	req.ConfirmedAffectedShotIDs = nil
	if _, err := svc.RestoreStepVersion(context.Background(), "user-1", "vp-1", model.CreatorStepScript, 1, req); !errors.Is(err, ErrCreatorImpactMismatch) {
		t.Fatalf("retry impact error=%v", err)
	}
	req.ConfirmedAffectedShotIDs = []string{"shot-1"}
	result, err := svc.RestoreStepVersion(context.Background(), "user-1", "vp-1", model.CreatorStepScript, 1, req)
	if err != nil {
		t.Fatal(err)
	}
	if revisions.calls != 1 || result.Artifact.Version != 5 {
		t.Fatalf("calls=%d artifact=%+v", revisions.calls, result.Artifact)
	}
}

func TestInstructionRevisionUsesVerifiedProjectProviderAndRejectsMissingConfig(t *testing.T) {
	base := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 3, IsCurrent: true}
	newService := func(config json.RawMessage) (*CreatorViewService, *fakeCreatorRevisionService) {
		artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{base}, history: []*artifact.Artifact{base}}
		revisions := &fakeCreatorRevisionService{artifacts: artifacts}
		return NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "user-1", Config: config}}, fakeCreatorShotReader{}, artifacts).
			WithStepMutations(revisions, &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review"}), revisions
	}
	req := model.StepRevisionRequest{IdempotencyKey: "provider-key", ArtifactID: base.ID, BaseVersion: 3, Mode: "instruction", Instruction: "rewrite"}
	svc, revisions := newService(json.RawMessage(`{"modelProviders":{"text_to_text":{"baseUrl":"https://model.test","apiKey":"secret","model":"writer"}}}`))
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); err != nil {
		t.Fatal(err)
	}
	if revisions.reviseRequest.ModelProviders["text_to_text"].(map[string]interface{})["model"] != "writer" {
		t.Fatalf("providers=%+v", revisions.reviseRequest.ModelProviders)
	}
	svc, revisions = newService(nil)
	if _, err := svc.ReviseStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, req); !errors.Is(err, ErrCreatorModelProviderUnavailable) || revisions.calls != 0 {
		t.Fatalf("missing provider error=%v calls=%d", err, revisions.calls)
	}
}

func TestStepVersionsAreSortedAndCurrentMarked(t *testing.T) {
	v1 := &artifact.Artifact{ID: "script-v1", ProjectID: "vp-1", StageName: "script", Version: 1}
	v3 := &artifact.Artifact{ID: "script-v3", ProjectID: "vp-1", StageName: "script", Version: 3, IsCurrent: true}
	v2 := &artifact.Artifact{ID: "script-v2", ProjectID: "vp-1", StageName: "script", Version: 2}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{v3}, history: []*artifact.Artifact{v1, v3, v2}}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).
		WithStepMutations(&fakeCreatorRevisionService{artifacts: artifacts}, &fakeCreatorReviewMutations{})

	versions, err := svc.GetStepVersions(context.Background(), "user-1", "vp-1", model.CreatorStepScript)
	if err != nil {
		t.Fatalf("GetStepVersions() error = %v", err)
	}
	if got := []int{versions.Versions[0].Version, versions.Versions[1].Version, versions.Versions[2].Version}; !reflect.DeepEqual(got, []int{3, 2, 1}) || !versions.Versions[0].IsCurrent {
		t.Fatalf("versions = %+v", versions.Versions)
	}
}

func TestConfirmStepResolvesCurrentArtifactGateBeforeConfirming(t *testing.T) {
	current := &artifact.Artifact{ID: "script-v4", ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", Version: 4, IsCurrent: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{current}, history: []*artifact.Artifact{current}}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "script-review"}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).
		WithStepMutations(&fakeCreatorRevisionService{artifacts: artifacts}, reviews)

	view, err := svc.ConfirmStep(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepConfirmRequest{
		ArtifactID: "script-v4", RunID: "run-1", ReviewID: "script-review", Comment: "确认",
	})
	if err != nil {
		t.Fatalf("ConfirmStep() error = %v", err)
	}
	if reviews.confirmedRunID != "run-1" || reviews.confirmedReviewID != "script-review" || view == nil {
		t.Fatalf("confirm=%+v view=%+v", reviews, view)
	}
}

func TestStepMutationAuthorityMatchesArtifactSelectedByCreatorView(t *testing.T) {
	failedScript := &artifact.Artifact{ID: "script-v1", ProjectID: "vp-1", StageName: "script", Version: 1, IsCurrent: true, Status: "failed"}
	approvedVoice := &artifact.Artifact{ID: "voice-v3", ProjectID: "vp-1", StageName: "voiceover", Version: 3, IsCurrent: true, Status: "valid", HumanApproved: true}
	artifacts := &fakeCreatorMutationArtifacts{current: []*artifact.Artifact{approvedVoice, failedScript}, history: []*artifact.Artifact{approvedVoice, failedScript}}
	svc := NewCreatorViewService(fakeCreatorProjectReader{project: &model.VideoProject{ID: "vp-1"}}, fakeCreatorShotReader{}, artifacts).
		WithStepMutations(&fakeCreatorRevisionService{artifacts: artifacts}, &fakeCreatorReviewMutations{})

	view, err := svc.GetCreationView(context.Background(), "user-1", "vp-1")
	if err != nil || view.Steps[2].CurrentArtifactID != "script-v1" {
		t.Fatalf("view script = %+v error=%v", view.Steps[2], err)
	}
	if _, err := svc.PreviewStepRevision(context.Background(), "user-1", "vp-1", model.CreatorStepScript, model.StepRevisionRequest{ArtifactID: "script-v1", BaseVersion: 1}); err != nil {
		t.Fatalf("PreviewStepRevision(view artifact) error = %v", err)
	}
}

func TestStepImpactIncludesAllShotsForProjectLevelVisualAndAudioSources(t *testing.T) {
	svc := NewCreatorViewService(nil, fakeCreatorShotReader{state: creatorShotReadState{ShotIDs: []string{"shot-2", "shot-1"}}}, nil)
	for _, tc := range []struct {
		stage string
		step  model.CreatorStepID
	}{
		{"script", model.CreatorStepScript},
		{"character", model.CreatorStepDirection},
		{"audio_master", model.CreatorStepScript},
		{"style", model.CreatorStepDirection},
	} {
		impact, err := svc.stepImpact(context.Background(), "user-1", "vp-1", tc.step, &artifact.Artifact{StageName: tc.stage})
		if err != nil || !reflect.DeepEqual(impact.AffectedShotIDs, []string{"shot-1", "shot-2"}) {
			t.Fatalf("stage %s impact=%+v error=%v", tc.stage, impact, err)
		}
	}
}

func TestArtifactSelectionValidationCoversRectAndTimeBounds(t *testing.T) {
	f64 := func(value float64) *float64 { return &value }
	i64 := func(value int64) *int64 { return &value }
	validTime, err := normalizeArtifactSelection(&model.ArtifactSelection{Kind: " time ", StartMs: i64(0), EndMs: i64(1000)})
	if err != nil || !reflect.DeepEqual(validTime, map[string]interface{}{"kind": "time", "startMs": int64(0), "endMs": int64(1000)}) {
		t.Fatalf("valid time selection=%#v error=%v", validTime, err)
	}
	for name, selection := range map[string]*model.ArtifactSelection{
		"rect outside":  {Kind: "rect", X: f64(.8), Y: f64(.1), Width: f64(.3), Height: f64(.2)},
		"zero width":    {Kind: "rect", X: f64(.1), Y: f64(.1), Width: f64(0), Height: f64(.2)},
		"negative time": {Kind: "time", StartMs: i64(-1), EndMs: i64(2)},
		"empty time":    {Kind: "time", StartMs: i64(2), EndMs: i64(2)},
		"mixed fields":  {Kind: "time", StartMs: i64(0), EndMs: i64(2), X: f64(.1)},
		"unknown":       {Kind: "pixels", X: f64(.1), Y: f64(.1), Width: f64(.2), Height: f64(.2)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeArtifactSelection(selection); !errors.Is(err, ErrCreatorInvalidRequest) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestImpactConfirmationIsOrderInsensitiveButRejectsDuplicatesOmissionsAndExtras(t *testing.T) {
	expected := []string{"shot-1", "shot-2"}
	if !sameExactIDs([]string{"shot-2", "shot-1"}, expected) {
		t.Fatal("same set in a different order must match")
	}
	for _, actual := range [][]string{{"shot-1"}, {"shot-1", "shot-2", "shot-3"}, {"shot-1", "shot-1"}} {
		if sameExactIDs(actual, expected) {
			t.Fatalf("non-exact set matched: %v", actual)
		}
	}
}

func stepStates(steps []model.CreatorStep) []model.CreatorStepState {
	states := make([]model.CreatorStepState, len(steps))
	for i, step := range steps {
		states[i] = step.State
	}
	return states
}

type fakeCreatorProjectReader struct {
	project *model.VideoProject
	err     error
}

func (f fakeCreatorProjectReader) GetProject(context.Context, string, string) (*model.VideoProject, error) {
	return f.project, f.err
}

type fakeCreatorArtifactReader struct {
	artifacts []*artifact.Artifact
	err       error
}

func (f fakeCreatorArtifactReader) ListCurrentByProject(context.Context, string) ([]*artifact.Artifact, error) {
	return f.artifacts, f.err
}

type fakeCreatorShotReader struct {
	state creatorShotReadState
	err   error
}

type fakeCreatorAssemblyReader struct {
	fakeCreatorShotReader
	receipt               model.AssemblyReceipt
	rebuildStatus         string
	markErr               error
	claimCalls, markCalls int
}

func (f *fakeCreatorAssemblyReader) RebuildFinalAssembly(context.Context, string, string, string) (AssemblyRebuildResult, error) {
	return AssemblyRebuildResult{Status: f.rebuildStatus}, nil
}
func (f *fakeCreatorAssemblyReader) ClaimFinalAssemblyDispatch(context.Context, string, string, string) (AssemblyRebuildResult, error) {
	f.claimCalls++
	return AssemblyRebuildResult{Status: "dispatching"}, nil
}
func (f *fakeCreatorAssemblyReader) MarkFinalAssemblyQueued(context.Context, string, string, string, string, string) (AssemblyRebuildResult, error) {
	f.markCalls++
	return AssemblyRebuildResult{Status: "queued"}, f.markErr
}
func (f *fakeCreatorAssemblyReader) LatestAssemblyReceipt(context.Context, string, string) (model.AssemblyReceipt, bool, error) {
	return f.receipt, true, nil
}
func (f *fakeCreatorAssemblyReader) GetAssemblyReceipt(context.Context, string, string, string) (model.AssemblyReceipt, bool, error) {
	return f.receipt, true, nil
}

func TestCreatorAssemblyRetryDoesNotRedispatchAfterMarkFailureWhenSourceIsActive(t *testing.T) {
	shots := &fakeCreatorAssemblyReader{rebuildStatus: "validated", markErr: errors.New("crash after dispatch")}
	reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review-1", regenerationStatus: "RUNNING"}
	artifacts := fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{{ID: "preview-1", ProjectID: "vp-1", StageName: "assembly", Version: 1}}}
	svc := NewCreatorViewService(fakeCreatorProjectReader{}, shots, artifacts).WithStepMutations(nil, reviews)
	if _, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "key-1"); err == nil {
		t.Fatal("expected mark failure")
	}
	if reviews.regenerateCalls != 1 {
		t.Fatalf("dispatch calls=%d", reviews.regenerateCalls)
	}
	shots.rebuildStatus, shots.markErr, shots.receipt = "dispatching", nil, model.AssemblyReceipt{Status: "dispatching", BasePreviewArtifactID: "preview-1", PreviewTaskID: "run-1"}
	if _, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "key-1"); err != nil {
		t.Fatal(err)
	}
	if reviews.regenerateCalls != 1 {
		t.Fatalf("active retry redispatched: %d", reviews.regenerateCalls)
	}
}

func TestCreatorAssemblyQueuedTerminalRetriesButRunningAndSuccessDoNot(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   int
	}{{"RUNNING", 0}, {"SUCCESS", 0}, {"FAILED", 1}} {
		t.Run(tc.status, func(t *testing.T) {
			shots := &fakeCreatorAssemblyReader{rebuildStatus: "queued", receipt: model.AssemblyReceipt{Status: "queued", BasePreviewArtifactID: "preview-1", PreviewTaskID: "run-1"}}
			reviews := &fakeCreatorReviewMutations{resolvedRunID: "run-1", resolvedReviewID: "review-1", regenerationStatus: tc.status}
			svc := NewCreatorViewService(fakeCreatorProjectReader{}, shots, fakeCreatorArtifactReader{artifacts: []*artifact.Artifact{{ID: "preview-1", ProjectID: "vp-1", StageName: "assembly", Status: "COMPLETED", Version: 1}}}).WithStepMutations(nil, reviews)
			if _, err := svc.RebuildFinalAssembly(context.Background(), "u-1", "vp-1", "key-1"); err != nil {
				t.Fatal(err)
			}
			if reviews.regenerateCalls != tc.want {
				t.Fatalf("calls=%d want=%d", reviews.regenerateCalls, tc.want)
			}
		})
	}
}

func (f fakeCreatorShotReader) getCreatorShotReadState(context.Context, string, string) (creatorShotReadState, error) {
	return f.state, f.err
}

type fakeCreatorMutationArtifacts struct {
	current []*artifact.Artifact
	history []*artifact.Artifact
}

func (f *fakeCreatorMutationArtifacts) ListCurrentByProject(context.Context, string) ([]*artifact.Artifact, error) {
	return f.current, nil
}

func (f *fakeCreatorMutationArtifacts) GetByID(_ context.Context, id string) (*artifact.Artifact, error) {
	for _, candidate := range f.history {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return nil, nil
}

func (f *fakeCreatorMutationArtifacts) GetCurrent(_ context.Context, projectID, stageName, unitID string) (*artifact.Artifact, error) {
	for _, candidate := range f.current {
		if candidate.ProjectID == projectID && candidate.StageName == stageName && candidate.UnitID == unitID {
			return candidate, nil
		}
	}
	return nil, nil
}

func (f *fakeCreatorMutationArtifacts) GetHistory(_ context.Context, projectID, stageName, unitID string) ([]*artifact.Artifact, error) {
	return f.history, nil
}

type fakeCreatorRevisionService struct {
	artifacts            *fakeCreatorMutationArtifacts
	reviseRequest        artifact.ReviseRequest
	calls                int
	reviseErrAfterCreate error
}

func (f *fakeCreatorRevisionService) Revise(_ context.Context, req artifact.ReviseRequest) (*artifact.RevisionResult, error) {
	f.calls++
	f.reviseRequest = req
	base, _ := f.artifacts.GetByID(context.Background(), req.ArtifactID)
	revised := *base
	revised.ID = req.NewArtifactID
	if revised.ID == "" {
		revised.ID = "script-v4"
	}
	revised.Version = 4
	revised.ParentID = base.ID
	revised.HumanApproved = false
	revised.Metadata = map[string]interface{}{}
	for key, value := range req.Provenance {
		revised.Metadata[key] = value
	}
	f.artifacts.current = []*artifact.Artifact{&revised}
	f.artifacts.history = append([]*artifact.Artifact{&revised}, f.artifacts.history...)
	if f.reviseErrAfterCreate != nil {
		return nil, f.reviseErrAfterCreate
	}
	return &artifact.RevisionResult{Artifact: &revised}, nil
}

func (f *fakeCreatorRevisionService) Restore(_ context.Context, req artifact.RestoreRequest) (*artifact.RevisionResult, error) {
	f.calls++
	historical, _ := f.artifacts.GetByID(context.Background(), req.ArtifactID)
	current, _ := f.artifacts.GetCurrent(context.Background(), historical.ProjectID, historical.StageName, historical.UnitID)
	restored := *historical
	restored.ID = req.NewArtifactID
	if restored.ID == "" {
		restored.ID = "script-v5"
	}
	restored.Version = current.Version + 1
	restored.ParentID = current.ID
	restored.IsCurrent = true
	restored.HumanApproved = false
	restored.Metadata = map[string]interface{}{"restoredFromArtifactId": historical.ID}
	for key, value := range req.Provenance {
		restored.Metadata[key] = value
	}
	current.IsCurrent = false
	f.artifacts.current = []*artifact.Artifact{&restored}
	f.artifacts.history = append([]*artifact.Artifact{&restored}, f.artifacts.history...)
	return &artifact.RevisionResult{Artifact: &restored}, nil
}

type fakeCreatorReviewMutations struct {
	resolvedRunID      string
	resolvedReviewID   string
	reopenedRunID      string
	reopenedReviewID   string
	reopenedArtifactID string
	confirmedRunID     string
	confirmedReviewID  string
	resolveErr         error
	reopenErr          error
	reopenCalls        int
	regenerateCalls    int
	regenerationStatus string
}

func (f *fakeCreatorReviewMutations) ResolveReviewGate(context.Context, *artifact.Artifact, string, string) (string, string, error) {
	return f.resolvedRunID, f.resolvedReviewID, f.resolveErr
}

func (f *fakeCreatorReviewMutations) Confirm(_ context.Context, runID, reviewID, reviewerID, comment string) error {
	f.confirmedRunID, f.confirmedReviewID = runID, reviewID
	return nil
}

func (f *fakeCreatorReviewMutations) ConfirmForArtifact(_ context.Context, runID, reviewID, artifactID, reviewerID, comment string) error {
	return f.Confirm(context.Background(), runID, reviewID, reviewerID, comment)
}

func (f *fakeCreatorReviewMutations) ReopenWithArtifact(_ context.Context, runID, reviewID, projectID, expectedArtifactID, artifactID, reviewerID, reason string) error {
	f.reopenCalls++
	f.reopenedRunID, f.reopenedReviewID, f.reopenedArtifactID = runID, reviewID, artifactID
	return f.reopenErr
}

func (f *fakeCreatorReviewMutations) Regenerate(context.Context, string, string, string, string) ([]string, error) {
	f.regenerateCalls++
	return nil, f.reopenErr
}

func (f *fakeCreatorReviewMutations) RegenerationStatus(context.Context, string, string) (string, error) {
	if f.regenerationStatus != "" {
		return f.regenerationStatus, nil
	}
	return "RUNNING", nil
}
