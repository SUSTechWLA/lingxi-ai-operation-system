package localrunner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localtool"
)

func TestLoopClaimsExecutesAndCompletesOneJob(t *testing.T) {
	client := &fakeClient{
		register: RegisterRunnerResponse{RunnerID: "runner_001", SessionID: "session_001", HeartbeatIntervalSec: 15, PollIntervalSec: 3},
		claim: &localtool.Job{
			ID:         "local_job_001",
			ProjectID:  "project_001",
			NodeID:     "node_001",
			ToolName:   "bundle_extractor",
			Command:    "BUNDLE_EXTRACT",
			Payload:    map[string]interface{}{"name": "bundle"},
			TimeoutSec: 30,
		},
	}
	reg := localtool.NewRegistry()
	reg.Register(localtool.ExecutorFunc(func(_ context.Context, job localtool.Job) (*localtool.Result, error) {
		if job.ID != "local_job_001" {
			t.Fatalf("unexpected job: %#v", job)
		}
		return &localtool.Result{Output: map[string]interface{}{"summary": "packaged"}}, nil
	}), "BUNDLE_EXTRACT")

	loop := NewLoop(client, reg, LoopOptions{
		DeviceID:      "device_001",
		RunnerVersion: "1.0.0",
		WorkspaceRoot: "local://aios/projects",
		PollInterval:  time.Millisecond,
	})
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if client.completedJobID != "local_job_001" {
		t.Fatalf("job not completed: %#v", client)
	}
	if client.completed.Output["summary"] != "packaged" {
		t.Fatalf("unexpected complete output: %#v", client.completed)
	}
}

func TestLoopRegistersOnlyExecutableCapabilities(t *testing.T) {
	client := &fakeClient{
		register: RegisterRunnerResponse{RunnerID: "runner_001", SessionID: "session_001", HeartbeatIntervalSec: 15, PollIntervalSec: 3},
	}
	reg := localtool.NewRegistry()
	reg.Register(localtool.ExecutorFunc(func(context.Context, localtool.Job) (*localtool.Result, error) {
		return &localtool.Result{Output: map[string]interface{}{}}, nil
	}), "BUNDLE_EXTRACT")

	loop := NewLoop(client, reg, LoopOptions{PollInterval: time.Millisecond})
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(client.registerReq.Capabilities) != 1 {
		t.Fatalf("expected only registered executable capabilities, got %#v", client.registerReq.Capabilities)
	}
	if client.registerReq.Capabilities[0].Command != "BUNDLE_EXTRACT" {
		t.Fatalf("unexpected registered capability: %#v", client.registerReq.Capabilities[0])
	}
}

func TestLoopFailsUnsupportedCommand(t *testing.T) {
	client := &fakeClient{
		register: RegisterRunnerResponse{RunnerID: "runner_001", SessionID: "session_001", HeartbeatIntervalSec: 15, PollIntervalSec: 3},
		claim:    &localtool.Job{ID: "local_job_001", Command: "bash"},
	}
	loop := NewLoop(client, localtool.NewRegistry(), LoopOptions{PollInterval: time.Millisecond})

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once should report failure to cloud instead of returning: %v", err)
	}
	if client.failedJobID != "local_job_001" {
		t.Fatalf("unsupported command should fail job: %#v", client)
	}
	if client.failed.Error["code"] != "LOCAL_COMMAND_NOT_ALLOWED" {
		t.Fatalf("unexpected failure payload: %#v", client.failed)
	}
}

type fakeClient struct {
	register       RegisterRunnerResponse
	claim          *localtool.Job
	registerReq    RegisterRunnerRequest
	completedJobID string
	completed      CompleteJobRequest
	failedJobID    string
	failed         FailJobRequest
}

func (f *fakeClient) Register(_ context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	f.registerReq = req
	return &f.register, nil
}

func (f *fakeClient) Heartbeat(context.Context, string, HeartbeatRequest) error {
	return nil
}

func (f *fakeClient) ClaimJob(context.Context, string) (*localtool.Job, error) {
	job := f.claim
	f.claim = nil
	return job, nil
}

func (f *fakeClient) ReportProgress(context.Context, string, ProgressRequest) error {
	return nil
}

func (f *fakeClient) CompleteJob(_ context.Context, jobID string, req CompleteJobRequest) error {
	f.completedJobID = jobID
	f.completed = req
	return nil
}

func (f *fakeClient) FailJob(_ context.Context, jobID string, req FailJobRequest) error {
	f.failedJobID = jobID
	f.failed = req
	return nil
}

var errFake = errors.New("fake")

var _ = errFake
