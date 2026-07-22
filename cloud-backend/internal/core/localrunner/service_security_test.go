package localrunner

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDispatchOwnerResolutionSQLUsesDurableTaskWorkflowAndProjectOwners(t *testing.T) {
	query := strings.ToLower(resolveDispatchOwnerSQL)
	for _, required := range []string{"ai_task", "workflow_runs", "video_projects", "task_id", "user_id"} {
		if !strings.Contains(query, required) {
			t.Fatalf("owner resolution query does not use %s: %s", required, query)
		}
	}
	if strings.Contains(query, "coalesce($3") {
		t.Fatalf("requested user must never be an owner fallback: %s", query)
	}
}

func TestTaskOwnerBackfillCannotOverwriteConcurrentOwner(t *testing.T) {
	query := strings.ToLower(backfillDispatchOwnerSQL)
	for _, required := range []string{"user_id=$2", "user_id is null", "btrim(user_id)=''", "user_id=$2", "returning user_id"} {
		if !strings.Contains(strings.ReplaceAll(query, " ", ""), strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("owner backfill missing %q: %s", required, query)
		}
	}
}

func TestJobMutationPredicateAtomicallyBindsOwnerRunnerTargetSessionLeaseAndState(t *testing.T) {
	predicate := strings.ToLower(jobMutationAccessPredicateSQL)
	for _, required := range []string{
		"lj.user_id", "lj.runner_id", "lj.target_runner_id", "lj.lease_expires_at",
		"lj.status in ('claimed','running')", "local_runners", "lr.user_id", "lr.device_id",
		"lr.session_id", "lr.status='online'", "lr.last_heartbeat",
	} {
		if !strings.Contains(strings.ReplaceAll(predicate, " ", ""), strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("atomic mutation predicate missing %q: %s", required, predicate)
		}
	}
}

func TestRequireSingleJobMutationRejectsZeroRows(t *testing.T) {
	if err := requireSingleJobMutation(pgconn.NewCommandTag("UPDATE 0")); err == nil {
		t.Fatal("zero-row authorization/mutation must be rejected")
	}
	if err := requireSingleJobMutation(pgconn.NewCommandTag("UPDATE 1")); err != nil {
		t.Fatalf("one authorized mutation rejected: %v", err)
	}
}

func TestLeaseMaintenanceCoversClaimedAndRunningJobs(t *testing.T) {
	heartbeat := strings.ToLower(runnerHeartbeatLeaseExtensionSQL)
	if !strings.Contains(heartbeat, "runner_id=$1") || !strings.Contains(heartbeat, "status in ('claimed','running')") {
		t.Fatalf("runner heartbeat must renew only its active claims: %s", heartbeat)
	}
	reap := strings.ToLower(reapExpiredLeasesSQL)
	if !strings.Contains(reap, "status in ('claimed','running')") || !strings.Contains(reap, "runner_id=null") {
		t.Fatalf("lease reaper must invalidate claimed and running jobs: %s", reap)
	}
}

func TestDispatchRequiresResolvableDurableOwner(t *testing.T) {
	if err := validateResolvedDispatchOwner("", "attacker"); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("unresolved owner must fail closed, got %v", err)
	}
	if err := validateResolvedDispatchOwner("user-auth", "attacker"); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("requested owner mismatch must be rejected, got %v", err)
	}
	if err := validateResolvedDispatchOwner("user-auth", "user-auth"); err != nil {
		t.Fatalf("matching durable owner rejected: %v", err)
	}
}
