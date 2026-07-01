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
	defer pool.Close()

	service := NewService(pool)
	projectPrefix := "lrtest-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	_, _ = pool.Exec(ctx, `DELETE FROM local_jobs WHERE project_id LIKE $1`, projectPrefix+"%")
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
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_jobs WHERE project_id LIKE $1`, projectPrefix+"%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM local_runners WHERE id=$1`, runner.RunnerID)
	})

	firstJob, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		ProjectID: projectPrefix + "-complete",
		Command:   CommandHyperFramesLint,
		Payload:   map[string]interface{}{"outputName": "final.mp4"},
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

	completed, err := service.CompleteJob(ctx, firstJob.ID, CompleteJobRequest{
		Output: map[string]interface{}{"storageRef": "local://test/final.mp4"},
	})
	if err != nil {
		t.Fatalf("complete first job: %v", err)
	}
	if completed.Status != JobCompleted || completed.Output["storageRef"] != "local://test/final.mp4" {
		t.Fatalf("unexpected completed job: %#v", completed)
	}

	secondJob, err := service.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		ProjectID: projectPrefix + "-fail",
		Command:   CommandHyperFramesLint,
		Payload:   map[string]interface{}{"outputName": "failed.mp4"},
	})
	if err != nil {
		t.Fatalf("dispatch second job: %v", err)
	}
	if _, err := service.ClaimJob(ctx, runner.RunnerID); err != nil {
		t.Fatalf("claim second job: %v", err)
	}

	failed, err := service.FailJob(ctx, secondJob.ID, FailJobRequest{
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
