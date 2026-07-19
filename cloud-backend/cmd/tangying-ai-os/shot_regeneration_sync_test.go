package main

import (
	"context"
	"strings"
	"testing"

	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoservice "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
)

func TestShotRegenerationDispatcherStartsTargetOnlyAgentRun(t *testing.T) {
	runner := &fakeAsyncAgentRunner{}
	dispatcher := &shotRegenerationAgentDispatcher{runner: runner}
	task := videomodel.ShotRegenerationTask{
		TaskID: "regen-task-1", RunID: "agent_run_shot_stable", ShotID: "shot-012", Scope: "base_media",
		Locks: []string{"duration"}, BaseVersion: 3,
	}
	runID, err := dispatcher.EnqueueShotRegeneration(context.Background(), "u-1", "vp-1", task)
	if err != nil || runID != task.RunID {
		t.Fatalf("runID=%q error=%v", runID, err)
	}
	ctx := runner.req.Context
	if runner.req.RunID != task.RunID || runner.req.UserID != "u-1" || runner.req.Domain != "video_creation" ||
		ctx["operation"] != "shot_regeneration" || ctx["targetShotId"] != "shot-012" ||
		ctx["shotRegenerationTaskId"] != "regen-task-1" || ctx["shotRegenerationRunId"] != task.RunID {
		t.Fatalf("dispatch request=%+v", runner.req)
	}
}

func TestCompleteShotRegenerationFromLocalJobUsesDurableProvenance(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	job := shotRegenerationLocalJob()
	output := map[string]interface{}{"metadata": map[string]interface{}{
		"shotRegenerationTaskId": "evil-task", "relatedShotId": "shot-013", "shotRegenerationRunId": "evil-run",
		"candidateId": "candidate-2", "durationSec": float64(6),
	}}

	if err := completeShotRegenerationFromLocalJob(context.Background(), completion, projects, job, output); err != nil {
		t.Fatalf("complete from local job: %v", err)
	}
	if completion.userID != "u-1" || completion.projectID != "vp-1" || completion.provenance.TaskID != "regen-task-1" ||
		completion.provenance.RunID != "agent_run_shot_stable" || completion.provenance.ShotID != "shot-012" ||
		completion.candidate.ShotID != "shot-012" || completion.candidate.CandidateID != "candidate-2" {
		t.Fatalf("completion=%+v", completion)
	}
}

func TestCompleteShotRegenerationFromLocalJobDerivesRetryStableCandidateID(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	job := shotRegenerationLocalJob()
	output := map[string]interface{}{"metadata": map[string]interface{}{}}
	if err := completeShotRegenerationFromLocalJob(context.Background(), completion, projects, job, output); err != nil {
		t.Fatalf("complete from output: %v", err)
	}
	firstID := completion.candidate.CandidateID
	completion.candidate = videomodel.ShotCandidate{}
	if err := completeShotRegenerationFromLocalJob(context.Background(), completion, projects, job, output); err != nil {
		t.Fatalf("retry complete from output: %v", err)
	}
	if firstID == "" || completion.candidate.CandidateID != firstID {
		t.Fatalf("candidate IDs are not retry-stable: first=%q second=%q", firstID, completion.candidate.CandidateID)
	}
}

func TestCompleteShotRegenerationFromLocalJobRejectsMissingProjectOwner(t *testing.T) {
	completion := &fakeShotCompletionService{}
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1"}}
	err := completeShotRegenerationFromLocalJob(context.Background(), completion, projects, shotRegenerationLocalJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "project owner") || completion.provenance.TaskID != "" {
		t.Fatalf("error=%v completion=%+v", err, completion)
	}
}

func TestFailShotRegenerationFromJobUsesDurableTaskID(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	err := failShotRegenerationFromJob(context.Background(), completion, projects, shotRegenerationLocalJob(), "provider timeout")
	if err != nil || completion.provenance.TaskID != "regen-task-1" || completion.provenance.RunID != "agent_run_shot_stable" || completion.reason != "provider timeout" {
		t.Fatalf("error=%v completion=%+v", err, completion)
	}
}

func TestFailShotRegenerationFromAgentTerminalUsesRequestContext(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	event := agentruntime.RunTerminalEvent{
		RunID: "agent_run_shot_stable", Status: agentruntime.RunStatusFailed,
		Context: map[string]interface{}{
			"projectId": "vp-1", "shotRegenerationTaskId": "regen-task-1",
			"shotRegenerationRunId": "agent_run_shot_stable", "targetShotId": "shot-012",
		},
	}
	if err := failShotRegenerationFromAgentTerminal(context.Background(), completion, projects, event); err != nil {
		t.Fatalf("fail from terminal event: %v", err)
	}
	if completion.provenance.TaskID != "regen-task-1" || completion.provenance.RunID != event.RunID || completion.reason == "" {
		t.Fatalf("completion=%+v", completion)
	}
}

func shotRegenerationLocalJob() *localrunner.LocalJob {
	return &localrunner.LocalJob{ProjectID: "vp-1", Payload: map[string]interface{}{
		"shotRegenerationTaskId": "regen-task-1", "shotRegenerationRunId": "agent_run_shot_stable", "targetShotId": "shot-012",
	}}
}

type fakeAsyncAgentRunner struct {
	req agentruntime.StartRunRequest
	run *agentruntime.Run
	err error
}

func (f *fakeAsyncAgentRunner) StartAsync(_ context.Context, req agentruntime.StartRunRequest) (*agentruntime.Run, error) {
	f.req = req
	if f.run == nil && f.err == nil {
		f.run = &agentruntime.Run{ID: req.RunID}
	}
	return f.run, f.err
}

type fakeShotProjectFinder struct {
	project *videomodel.VideoProject
	err     error
}

func (f *fakeShotProjectFinder) FindByID(_ context.Context, _ string) (*videomodel.VideoProject, error) {
	return f.project, f.err
}

type fakeShotCompletionService struct {
	userID     string
	projectID  string
	provenance videoservice.ShotRegenerationProvenance
	candidate  videomodel.ShotCandidate
	reason     string
	err        error
}

func (f *fakeShotCompletionService) CompleteShotRegeneration(_ context.Context, userID, projectID string, provenance videoservice.ShotRegenerationProvenance, candidate videomodel.ShotCandidate) error {
	f.userID, f.projectID, f.provenance, f.candidate = userID, projectID, provenance, candidate
	return f.err
}

func (f *fakeShotCompletionService) FailShotRegeneration(_ context.Context, userID, projectID string, provenance videoservice.ShotRegenerationProvenance, reason string) error {
	f.userID, f.projectID, f.provenance, f.reason = userID, projectID, provenance, reason
	return f.err
}
