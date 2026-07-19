package main

import (
	"context"
	"strings"
	"testing"

	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
)

func TestShotRegenerationDispatcherStartsTargetOnlyAgentRun(t *testing.T) {
	runner := &fakeAsyncAgentRunner{run: &agentruntime.Run{ID: "agent-run-1"}}
	dispatcher := &shotRegenerationAgentDispatcher{runner: runner}
	task := videomodel.ShotRegenerationTask{
		TaskID: "regen-task-1", ShotID: "shot-012", Scope: "base_media",
		Locks: []string{"duration"}, BaseVersion: 3,
	}
	runID, err := dispatcher.EnqueueShotRegeneration(context.Background(), "u-1", "vp-1", task)
	if err != nil || runID != "agent-run-1" {
		t.Fatalf("runID=%q error=%v", runID, err)
	}
	ctx := runner.req.Context
	if runner.req.UserID != "u-1" || runner.req.Domain != "video_creation" ||
		ctx["operation"] != "shot_regeneration" || ctx["targetShotId"] != "shot-012" ||
		ctx["shotRegenerationTaskId"] != "regen-task-1" {
		t.Fatalf("dispatch request=%+v", runner.req)
	}
}

func TestCompleteShotRegenerationFromOutputResolvesProjectOwner(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	output := map[string]interface{}{"metadata": map[string]interface{}{
		"shotRegenerationTaskId": "regen-task-1", "relatedShotId": "shot-012",
		"candidateId": "candidate-2", "durationSec": float64(6),
	}}

	if err := completeShotRegenerationFromOutput(context.Background(), completion, projects, "vp-1", output); err != nil {
		t.Fatalf("complete from output: %v", err)
	}
	if completion.userID != "u-1" || completion.projectID != "vp-1" || completion.taskID != "regen-task-1" ||
		completion.candidate.ShotID != "shot-012" || completion.candidate.CandidateID != "candidate-2" {
		t.Fatalf("completion=%+v", completion)
	}
}

func TestCompleteShotRegenerationFromOutputDerivesRetryStableCandidateID(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	output := map[string]interface{}{"metadata": map[string]interface{}{
		"shotRegenerationTaskId": "regen-task-1", "relatedShotId": "shot-012",
	}}
	if err := completeShotRegenerationFromOutput(context.Background(), completion, projects, "vp-1", output); err != nil {
		t.Fatalf("complete from output: %v", err)
	}
	firstID := completion.candidate.CandidateID
	completion.candidate = videomodel.ShotCandidate{}
	if err := completeShotRegenerationFromOutput(context.Background(), completion, projects, "vp-1", output); err != nil {
		t.Fatalf("retry complete from output: %v", err)
	}
	if firstID == "" || completion.candidate.CandidateID != firstID {
		t.Fatalf("candidate IDs are not retry-stable: first=%q second=%q", firstID, completion.candidate.CandidateID)
	}
}

func TestCompleteShotRegenerationFromOutputRejectsMissingProjectOwner(t *testing.T) {
	completion := &fakeShotCompletionService{}
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1"}}
	output := map[string]interface{}{"metadata": map[string]interface{}{
		"shotRegenerationTaskId": "regen-task-1", "relatedShotId": "shot-012",
	}}
	err := completeShotRegenerationFromOutput(context.Background(), completion, projects, "vp-1", output)
	if err == nil || !strings.Contains(err.Error(), "project owner") || completion.taskID != "" {
		t.Fatalf("error=%v completion=%+v", err, completion)
	}
}

func TestFailShotRegenerationFromJobUsesDurableTaskID(t *testing.T) {
	projects := &fakeShotProjectFinder{project: &videomodel.VideoProject{ID: "vp-1", UserID: "u-1"}}
	completion := &fakeShotCompletionService{}
	err := failShotRegenerationFromJob(context.Background(), completion, projects, "vp-1", map[string]interface{}{
		"shotRegenerationTaskId": "regen-task-1",
	}, "provider timeout")
	if err != nil || completion.taskID != "regen-task-1" || completion.reason != "provider timeout" {
		t.Fatalf("error=%v completion=%+v", err, completion)
	}
}

type fakeAsyncAgentRunner struct {
	req agentruntime.StartRunRequest
	run *agentruntime.Run
	err error
}

func (f *fakeAsyncAgentRunner) StartAsync(_ context.Context, req agentruntime.StartRunRequest) (*agentruntime.Run, error) {
	f.req = req
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
	userID    string
	projectID string
	taskID    string
	candidate videomodel.ShotCandidate
	reason    string
	err       error
}

func (f *fakeShotCompletionService) CompleteShotRegeneration(_ context.Context, userID, projectID, taskID string, candidate videomodel.ShotCandidate) error {
	f.userID, f.projectID, f.taskID, f.candidate = userID, projectID, taskID, candidate
	return f.err
}

func (f *fakeShotCompletionService) FailShotRegeneration(_ context.Context, userID, projectID, taskID, reason string) error {
	f.userID, f.projectID, f.taskID, f.reason = userID, projectID, taskID, reason
	return f.err
}
