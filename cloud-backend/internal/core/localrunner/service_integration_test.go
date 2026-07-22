package localrunner

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceDispatchClaimCompleteAndFailJobPostgres(t *testing.T) {
	dsn := os.Getenv("LOCALRUNNER_TEST_DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("set LOCALRUNNER_TEST_DATABASE_URL to run postgres integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	service := NewService(pool)
	projectPrefix := "lrtest-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	taskID := projectPrefix + "-task"
	_, _ = pool.Exec(ctx, `DELETE FROM local_jobs WHERE project_id LIKE $1`, projectPrefix+"%")
	if _, err := pool.Exec(ctx, `INSERT INTO ai_task (id,user_id,status,input) VALUES ($1,$2,'RUNNING','{}'::jsonb)`, taskID, "test-user-localrunner-service"); err != nil {
		t.Fatalf("insert task owner: %v", err)
	}
	runner, err := service.RegisterRunner(ctx, RegisterRunnerRequest{
		DeviceID:      "test-device-localrunner-service",
		UserID:        "test-user-localrunner-service",
		RunnerVersion: "test",
		WorkspaceRoot: "local://test",
		Capabilities: []RunnerCapability{{
			ToolName:  "hyperframes_lint",
			Command:   CommandHyperFramesLint,
			Available: true,
		}},
	})
	if err != nil {
		t.Fatalf("register runner: %v", err)
	}
	runnerIdentity := JobMutationIdentity{UserID: "test-user-localrunner-service", DeviceID: "test-device-localrunner-service", RunnerID: runner.RunnerID, SessionID: runner.SessionID}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_jobs WHERE project_id LIKE $1`, projectPrefix+"%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM ai_task WHERE id=$1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_runners WHERE id=$1`, runner.RunnerID)
	})

	firstJob, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID:         "test-user-localrunner-service",
		ProjectID:      projectPrefix + "-complete",
		TaskID:         taskID,
		NodeID:         projectPrefix + "-node",
		Command:        CommandHyperFramesLint,
		Payload:        map[string]interface{}{"outputName": "final.mp4"},
		IdempotencyKey: projectPrefix + "-retryable-node",
	})
	if err != nil {
		t.Fatalf("dispatch first job: %v", err)
	}

	claimed, err := service.ClaimJob(ctx, runner.RunnerID)
	if err != nil {
		t.Fatalf("claim first job: %v", err)
	}
	if claimed == nil || claimed.ID != firstJob.ID || claimed.Status != JobClaimed {
		t.Fatalf("unexpected claimed job: %#v want id=%s status=%s", claimed, firstJob.ID, JobClaimed)
	}

	completed, err := service.CompleteJob(ctx, runnerIdentity, firstJob.ID, CompleteJobRequest{
		Output: map[string]interface{}{"storageRef": "local://test/final.mp4"},
	})
	if err != nil {
		t.Fatalf("complete first job: %v", err)
	}
	if completed.Status != JobCompleted || completed.Output["storageRef"] != "local://test/final.mp4" {
		t.Fatalf("unexpected completed job: %#v", completed)
	}

	retried, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID:         "test-user-localrunner-service",
		ProjectID:      projectPrefix + "-complete",
		TaskID:         taskID,
		NodeID:         projectPrefix + "-node",
		Command:        CommandHyperFramesLint,
		Payload:        map[string]interface{}{"outputName": "final-retry.mp4"},
		IdempotencyKey: projectPrefix + "-retryable-node",
	})
	if err != nil {
		t.Fatalf("redispatch completed job: %v", err)
	}
	if retried.ID != firstJob.ID || retried.Status != JobPending || retried.Attempt != firstJob.Attempt+1 {
		t.Fatalf("completed job should be reset for retry: first=%#v retried=%#v", firstJob, retried)
	}
	if retried.RunnerID != "" || len(retried.Output) != 0 {
		t.Fatalf("retried job must clear prior lease and output: %#v", retried)
	}

	duplicate, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID:         "test-user-localrunner-service",
		ProjectID:      projectPrefix + "-complete",
		TaskID:         taskID,
		NodeID:         projectPrefix + "-node",
		Command:        CommandHyperFramesLint,
		Payload:        map[string]interface{}{"outputName": "should-not-replace-pending.mp4"},
		IdempotencyKey: projectPrefix + "-retryable-node",
	})
	if err != nil {
		t.Fatalf("redispatch pending job: %v", err)
	}
	if duplicate.ID != retried.ID || duplicate.Status != JobPending || duplicate.Attempt != retried.Attempt {
		t.Fatalf("pending job should remain idempotent: retried=%#v duplicate=%#v", retried, duplicate)
	}
	if duplicate.Payload["outputName"] != "final-retry.mp4" {
		t.Fatalf("pending duplicate must not replace active payload: %#v", duplicate.Payload)
	}
	if _, err := service.ClaimJob(ctx, runner.RunnerID); err != nil {
		t.Fatalf("claim retried job: %v", err)
	}
	if _, err := service.CompleteJob(ctx, runnerIdentity, retried.ID, CompleteJobRequest{Output: map[string]interface{}{"retry": true}}); err != nil {
		t.Fatalf("complete retried job: %v", err)
	}

	secondJob, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID:    "test-user-localrunner-service",
		ProjectID: projectPrefix + "-fail",
		TaskID:    taskID,
		Command:   CommandHyperFramesLint,
		Payload:   map[string]interface{}{"outputName": "failed.mp4"},
	})
	if err != nil {
		t.Fatalf("dispatch second job: %v", err)
	}
	if _, err := service.ClaimJob(ctx, runner.RunnerID); err != nil {
		t.Fatalf("claim second job: %v", err)
	}

	failed, err := service.FailJob(ctx, runnerIdentity, secondJob.ID, FailJobRequest{
		Error:     map[string]interface{}{"message": "render failed"},
		Retryable: false,
	})
	if err != nil {
		t.Fatalf("fail second job: %v", err)
	}
	if failed.Status != JobFailed || failed.ErrorMessage != "render failed" || failed.Retryable {
		t.Fatalf("unexpected failed job: %#v", failed)
	}
}

func TestServiceScopesJobsAndMCPCatalogsByUserTargetAndRevisionPostgres(t *testing.T) {
	dsn := os.Getenv("LOCALRUNNER_TEST_DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("set LOCALRUNNER_TEST_DATABASE_URL to run postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	service := NewService(pool)
	prefix := "lr-scope-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	userA, userB := prefix+"-user-a", prefix+"-user-b"
	taskID := prefix + "-task-a"
	if _, err := pool.Exec(ctx, `INSERT INTO ai_task (id,user_id,status,input) VALUES ($1,$2,'RUNNING','{}'::jsonb)`, taskID, userA); err != nil {
		t.Fatalf("insert task owner: %v", err)
	}

	tools := []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
		InputSchema: map[string]interface{}{"type": "object"},
	}}
	revision := MCPToolCatalogRevision(tools)
	capabilities := []RunnerCapability{
		{ToolName: "lint", Command: CommandHyperFramesLint, Available: true},
		{ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true, CatalogRevision: revision, MCPTools: tools},
	}
	register := func(userID, deviceID string) *RegisterRunnerResponse {
		runner, registerErr := service.RegisterRunner(ctx, RegisterRunnerRequest{
			DeviceID: deviceID, UserID: userID, RunnerVersion: "test", WorkspaceRoot: "local://test", Capabilities: capabilities,
		})
		if registerErr != nil {
			t.Fatalf("register runner %s/%s: %v", userID, deviceID, registerErr)
		}
		return runner
	}
	runnerA1 := register(userA, prefix+"-device-a1")
	runnerA2 := register(userA, prefix+"-device-a2")
	runnerB := register(userB, prefix+"-device-b")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_jobs WHERE project_id LIKE $1`, prefix+"%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM ai_task WHERE id=$1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_runners WHERE id=ANY($1)`, []string{runnerA1.RunnerID, runnerA2.RunnerID, runnerB.RunnerID})
	})
	userCatalogs, err := service.ListOnlineRunnerMCPToolCatalogs(ctx, userA, "", "")
	if err != nil || len(userCatalogs) != 2 {
		t.Fatalf("list request-scoped user catalogs: catalogs=%#v err=%v", userCatalogs, err)
	}
	deviceCatalogs, err := service.ListOnlineRunnerMCPToolCatalogs(ctx, userA, prefix+"-device-a1", "")
	if err != nil || len(deviceCatalogs) != 1 || deviceCatalogs[0].RunnerID != runnerA1.RunnerID {
		t.Fatalf("list request-scoped device catalog: catalogs=%#v err=%v", deviceCatalogs, err)
	}

	nonMCP, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID: userA, ProjectID: prefix + "-user-scope", TaskID: taskID, Command: CommandHyperFramesLint,
	})
	if err != nil {
		t.Fatalf("dispatch user-scoped job: %v", err)
	}
	if claimed, err := service.ClaimJob(ctx, runnerB.RunnerID); err != nil || claimed != nil {
		t.Fatalf("user B must not claim user A job: job=%#v err=%v", claimed, err)
	}
	claimed, err := service.ClaimJob(ctx, runnerA2.RunnerID)
	if err != nil || claimed == nil || claimed.ID != nonMCP.ID {
		t.Fatalf("user A runner should claim user A job: job=%#v err=%v", claimed, err)
	}
	if _, err := service.CompleteJob(ctx, JobMutationIdentity{UserID: userA, DeviceID: prefix + "-device-a2", RunnerID: runnerA2.RunnerID, SessionID: runnerA2.SessionID}, nonMCP.ID, CompleteJobRequest{}); err != nil {
		t.Fatalf("complete user-scoped job: %v", err)
	}

	targeted, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID: userA, TargetRunnerID: runnerA1.RunnerID, ProjectID: prefix + "-target-scope", TaskID: taskID, Command: CommandHyperFramesLint,
	})
	if err != nil {
		t.Fatalf("dispatch target-scoped job: %v", err)
	}
	if claimed, err := service.ClaimJob(ctx, runnerA2.RunnerID); err != nil || claimed != nil {
		t.Fatalf("sibling device must not claim targeted job: job=%#v err=%v", claimed, err)
	}
	claimed, err = service.ClaimJob(ctx, runnerA1.RunnerID)
	if err != nil || claimed == nil || claimed.ID != targeted.ID {
		t.Fatalf("target runner should claim job: job=%#v err=%v", claimed, err)
	}
	if _, err := service.CompleteJob(ctx, JobMutationIdentity{UserID: userA, DeviceID: prefix + "-device-a1", RunnerID: runnerA1.RunnerID, SessionID: runnerA1.SessionID}, targeted.ID, CompleteJobRequest{}); err != nil {
		t.Fatalf("complete targeted job: %v", err)
	}
	if _, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		UserID: userA, TargetRunnerID: runnerB.RunnerID, ProjectID: prefix + "-foreign-target", TaskID: taskID, Command: CommandHyperFramesLint,
	}); err == nil {
		t.Fatal("dispatch must reject a target runner owned by another user")
	}

	mcpRequest := DispatchLocalJobRequest{
		UserID: userA, TargetRunnerID: runnerA1.RunnerID, CatalogRevision: revision,
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
		ProjectID: prefix + "-mcp", TaskID: taskID, Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{"arguments": map[string]interface{}{"shot": "s1"}},
	}
	stale := mcpRequest
	stale.CatalogRevision = strings.Repeat("0", 64)
	if _, err := service.DispatchLocalJob(ctx, stale); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale revision must be rejected, got %v", err)
	}
	unadvertised := mcpRequest
	unadvertised.MCPRemoteToolName = "delete"
	if _, err := service.DispatchLocalJob(ctx, unadvertised); err == nil || !strings.Contains(err.Error(), "does not advertise") {
		t.Fatalf("unadvertised tool must be rejected, got %v", err)
	}
	mcpJob, err := service.DispatchLocalJob(ctx, mcpRequest)
	if err != nil {
		t.Fatalf("dispatch valid MCP job: %v", err)
	}
	if claimed, err := service.ClaimJob(ctx, runnerA2.RunnerID); err != nil || claimed != nil {
		t.Fatalf("non-target sibling must not claim MCP job: job=%#v err=%v", claimed, err)
	}
	claimed, err = service.ClaimJob(ctx, runnerA1.RunnerID)
	if err != nil || claimed == nil || claimed.ID != mcpJob.ID || claimed.MCPRemoteToolName != "render" {
		t.Fatalf("bound target must claim MCP job: job=%#v err=%v", claimed, err)
	}

	if err := service.ValidateJobAccess(ctx, userB, runnerA1.RunnerID, mcpJob.ID); err == nil {
		t.Fatal("user B must not access user A job")
	}
	if err := service.ValidateJobAccess(ctx, userA, runnerA2.RunnerID, mcpJob.ID); err == nil {
		t.Fatal("sibling runner must not mutate claimed target job")
	}

	before, err := service.GetOnlineRunnerMCPToolCatalog(ctx, userA, prefix+"-device-a1", runnerA1.RunnerID)
	if err != nil || before == nil || before.Revision != revision {
		t.Fatalf("read initial catalog: catalog=%#v err=%v", before, err)
	}
	if err := service.Heartbeat(ctx, runnerA1.RunnerID, HeartbeatRequest{SessionID: runnerA1.SessionID, Status: "online"}); err != nil {
		t.Fatalf("nil heartbeat: %v", err)
	}
	afterNil, err := service.GetOnlineRunnerMCPToolCatalog(ctx, userA, prefix+"-device-a1", runnerA1.RunnerID)
	if err != nil || afterNil == nil || afterNil.Revision != revision {
		t.Fatalf("nil heartbeat must preserve catalog: catalog=%#v err=%v", afterNil, err)
	}
	updatedTools := []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.inspect", RemoteToolName: "inspect",
		InputSchema: map[string]interface{}{"type": "object"},
	}}
	updatedRevision := MCPToolCatalogRevision(updatedTools)
	updatedCapabilities := []RunnerCapability{
		{ToolName: "lint", Command: CommandHyperFramesLint, Available: true},
		{ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true, CatalogRevision: updatedRevision, MCPTools: updatedTools},
	}
	if err := service.Heartbeat(ctx, runnerA1.RunnerID, HeartbeatRequest{SessionID: runnerA1.SessionID, Status: "online", Capabilities: &updatedCapabilities}); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	afterUpdate, err := service.GetOnlineRunnerMCPToolCatalog(ctx, userA, prefix+"-device-a1", runnerA1.RunnerID)
	if err != nil || afterUpdate == nil || afterUpdate.Revision != updatedRevision || afterUpdate.Tools[0].RemoteToolName != "inspect" {
		t.Fatalf("non-empty heartbeat must replace catalog: catalog=%#v err=%v", afterUpdate, err)
	}
	empty := []RunnerCapability{}
	if err := service.Heartbeat(ctx, runnerA1.RunnerID, HeartbeatRequest{SessionID: runnerA1.SessionID, Status: "online", Capabilities: &empty}); err != nil {
		t.Fatalf("clear heartbeat: %v", err)
	}
	afterClear, err := service.GetOnlineRunnerMCPToolCatalog(ctx, userA, prefix+"-device-a1", runnerA1.RunnerID)
	if err != nil || afterClear != nil {
		t.Fatalf("empty heartbeat must clear catalog: catalog=%#v err=%v", afterClear, err)
	}
}

func TestServiceClaimAllowsManuallyReopenedAgentTaskPostgres(t *testing.T) {
	dsn := os.Getenv("LOCALRUNNER_TEST_DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		t.Skip("set LOCALRUNNER_TEST_DATABASE_URL to run postgres integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	service := NewService(pool)
	prefix := "lr-reopen-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	taskID := prefix + "-task"
	runID := prefix + "-run"
	runner, err := service.RegisterRunner(ctx, RegisterRunnerRequest{
		DeviceID:      prefix + "-device",
		UserID:        prefix + "-user",
		RunnerVersion: "test",
		WorkspaceRoot: "local://test",
		Capabilities: []RunnerCapability{{
			ToolName:  "hyperframes_lint",
			Command:   CommandHyperFramesLint,
			Available: true,
		}},
	})
	if err != nil {
		t.Fatalf("register runner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_jobs WHERE project_id=$1`, prefix)
		_, _ = pool.Exec(context.Background(), `DELETE FROM agent_runs WHERE id=$1`, runID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM ai_task WHERE id=$1`, taskID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_runners WHERE id=$1`, runner.RunnerID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO ai_task (id, user_id, status, input) VALUES ($1,$2,'RUNNING','{}'::jsonb)`, taskID, prefix+"-user"); err != nil {
		t.Fatalf("insert reopened task: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agent_runs (id, task_id, user_id, domain, message, plan_json, status) VALUES ($1,$2,$3,'video_creation','test','{}'::jsonb,'FAILED')`, runID, taskID, prefix+"-user"); err != nil {
		t.Fatalf("insert terminal run: %v", err)
	}
	job, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		ProjectID: prefix,
		TaskID:    taskID,
		NodeID:    prefix + "-node",
		Command:   CommandHyperFramesLint,
	})
	if err != nil {
		t.Fatalf("dispatch reopened task job: %v", err)
	}
	claimed, err := service.ClaimJob(ctx, runner.RunnerID)
	if err != nil {
		t.Fatalf("claim reopened task job: %v", err)
	}
	if claimed == nil || claimed.ID != job.ID {
		t.Fatalf("manually reopened task should be claimable despite terminal run status: got %#v want %s", claimed, job.ID)
	}
}
