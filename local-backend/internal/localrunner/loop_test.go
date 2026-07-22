package localrunner

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
			ToolName:   "artifact_packager",
			Command:    "ARTIFACT_PACKAGE",
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
	}), "ARTIFACT_PACKAGE")

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
	}), "ARTIFACT_PACKAGE")

	loop := NewLoop(client, reg, LoopOptions{PollInterval: time.Millisecond})
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(client.registerReq.Capabilities) != 1 {
		t.Fatalf("expected only registered executable capabilities, got %#v", client.registerReq.Capabilities)
	}
	if client.registerReq.Capabilities[0].Command != "ARTIFACT_PACKAGE" {
		t.Fatalf("unexpected registered capability: %#v", client.registerReq.Capabilities[0])
	}
}

func TestLoopRegistersMCPToolCatalogAndHeartbeatsOnlyRevisionChanges(t *testing.T) {
	client := &fakeClient{
		register: RegisterRunnerResponse{RunnerID: "runner_001", SessionID: "session_001", HeartbeatIntervalSec: 15, PollIntervalSec: 3},
	}
	reg := localtool.NewRegistry()
	reg.Register(localtool.ExecutorFunc(func(context.Context, localtool.Job) (*localtool.Result, error) {
		return &localtool.Result{Output: map[string]interface{}{}}, nil
	}), localtool.CommandLocalMCPToolCall)
	source := &fakeCatalogSource{catalog: MCPToolCatalog{
		Revision: strings.Repeat("a", 64),
		Tools: []MCPToolAdvertisement{{
			ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
	}}
	loop := NewLoop(client, reg, LoopOptions{
		PollInterval: time.Millisecond, MCPToolCatalogSource: source, MCPCatalogRefreshInterval: time.Nanosecond,
	})

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	mcpCapability := findCapability(client.registerReq.Capabilities, localtool.CommandLocalMCPToolCall)
	if mcpCapability == nil || mcpCapability.CatalogRevision != source.catalog.Revision || len(mcpCapability.MCPTools) != 1 {
		t.Fatalf("register catalog missing: %#v", client.registerReq.Capabilities)
	}
	if len(client.heartbeats) != 1 || client.heartbeats[0].Capabilities != nil {
		t.Fatalf("unchanged revision should not resend capabilities: %#v", client.heartbeats)
	}

	source.catalog = MCPToolCatalog{Revision: strings.Repeat("b", 64), Tools: []MCPToolAdvertisement{}}
	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(client.heartbeats) != 2 || client.heartbeats[1].Capabilities == nil {
		t.Fatalf("changed revision must replace capabilities: %#v", client.heartbeats)
	}
	cleared := findCapability(*client.heartbeats[1].Capabilities, localtool.CommandLocalMCPToolCall)
	if cleared == nil || cleared.CatalogRevision != source.catalog.Revision || len(cleared.MCPTools) != 0 {
		t.Fatalf("empty catalog should explicitly clear advertised tools: %#v", client.heartbeats[1])
	}
}

func findCapability(capabilities []Capability, command string) *Capability {
	for index := range capabilities {
		if capabilities[index].Command == command {
			return &capabilities[index]
		}
	}
	return nil
}

type fakeCatalogSource struct {
	catalog     MCPToolCatalog
	diagnostics []MCPProviderDiagnostic
}

func (f *fakeCatalogSource) Discover(context.Context) (MCPToolCatalog, []MCPProviderDiagnostic) {
	return f.catalog, f.diagnostics
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

func TestLoopDropsTerminalPendingReportErrors(t *testing.T) {
	client := &fakeClient{
		register: RegisterRunnerResponse{RunnerID: "runner_001", SessionID: "session_001", HeartbeatIntervalSec: 15, PollIntervalSec: 3},
		failErr:  &HTTPStatusError{Method: http.MethodPost, Path: "/api/local-jobs/local_job_stale/fail", StatusCode: http.StatusForbidden},
	}
	loop := NewLoop(client, localtool.NewRegistry(), LoopOptions{
		DataDir:      t.TempDir(),
		PollInterval: time.Millisecond,
	})
	failReq := FailJobRequest{
		Success:   false,
		Retryable: false,
		Error: map[string]interface{}{
			"message": "stale failure from previous runner session",
		},
	}
	if err := loop.pendingReports.Save(PendingReport{JobID: "local_job_stale", Type: "fail", Fail: &failReq}); err != nil {
		t.Fatalf("save pending report: %v", err)
	}

	if err := loop.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once should drop terminal pending report without failing: %v", err)
	}

	reports, err := loop.pendingReports.List()
	if err != nil {
		t.Fatalf("list pending reports: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("terminal pending report should be removed, got %#v", reports)
	}
	if client.failedJobID != "local_job_stale" {
		t.Fatalf("pending failure should have been retried before removal: %#v", client)
	}
}

type fakeClient struct {
	register       RegisterRunnerResponse
	claim          *localtool.Job
	registerReq    RegisterRunnerRequest
	heartbeats     []HeartbeatRequest
	completedJobID string
	completed      CompleteJobRequest
	failedJobID    string
	failed         FailJobRequest
	completeErr    error
	failErr        error
}

func (f *fakeClient) Register(_ context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	f.registerReq = req
	return &f.register, nil
}

func (f *fakeClient) Heartbeat(_ context.Context, _ string, req HeartbeatRequest) error {
	f.heartbeats = append(f.heartbeats, req)
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
	return f.completeErr
}

func (f *fakeClient) FailJob(_ context.Context, jobID string, req FailJobRequest) error {
	f.failedJobID = jobID
	f.failed = req
	return f.failErr
}

var errFake = errors.New("fake")

var _ = errFake
