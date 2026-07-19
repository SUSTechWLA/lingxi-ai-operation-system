package service

import (
	"context"
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

func (f fakeCreatorShotReader) getCreatorShotReadState(context.Context, string, string) (creatorShotReadState, error) {
	return f.state, f.err
}
