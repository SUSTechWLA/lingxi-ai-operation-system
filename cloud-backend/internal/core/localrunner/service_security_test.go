package localrunner

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

var _ StaleMCPJobRetirer = (*Service)(nil)

type staleMCPRetirementContract interface {
	RetireStaleMCPJobs(context.Context, JobMutationIdentity) ([]*LocalJob, error)
}

var _ staleMCPRetirementContract = (*Service)(nil)

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

func TestTerminalCallbackPredicateRetainsExactRunnerIsolation(t *testing.T) {
	predicate := strings.ToLower(terminalCallbackAccessPredicateSQL)
	for _, required := range []string{
		"lj.user_id", "lj.runner_id", "lj.target_runner_id", "lj.status in ('completed','failed')",
		"local_runners", "lr.user_id", "lr.device_id", "lr.session_id", "lr.status='online'", "lr.last_heartbeat",
	} {
		if !strings.Contains(strings.ReplaceAll(predicate, " ", ""), strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("terminal callback predicate missing %q: %s", required, predicate)
		}
	}
}

func TestTerminalCallbackClaimSQLIsAtomicAndLeaseRecoverable(t *testing.T) {
	query := strings.ToLower(claimTerminalCallbackSQL("result"))
	for _, required := range []string{
		"result_callback_state='processing'", "result_callback_claim_token=$6",
		"result_callback_lease_until", "result_callback_state='pending'",
		"result_callback_lease_until < now()", terminalCallbackAccessPredicateSQL,
	} {
		if !strings.Contains(strings.ReplaceAll(query, " ", ""), strings.ReplaceAll(strings.ToLower(required), " ", "")) {
			t.Fatalf("atomic callback claim missing %q: %s", required, query)
		}
	}
}

func TestTerminalCallbackAckAndReleaseAreAuthorizedOnlyByClaimToken(t *testing.T) {
	for _, query := range []string{ackTerminalCallbackSQL("result"), releaseTerminalCallbackSQL("result")} {
		normalized := strings.ToLower(query)
		for _, required := range []string{"result_callback_claim_token=$2", "result_callback_state='processing'"} {
			if !strings.Contains(strings.ReplaceAll(normalized, " ", ""), strings.ReplaceAll(required, " ", "")) {
				t.Fatalf("callback token mutation missing %q: %s", required, query)
			}
		}
		if strings.Contains(normalized, "session_id") || strings.Contains(normalized, "local_runners") {
			t.Fatalf("post-claim ack/release must not depend on a revocable runner session: %s", query)
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

func TestStaleMCPRetirementSQLAtomicallyScopesRunnerAndCatalogBinding(t *testing.T) {
	query := strings.ToLower(strings.ReplaceAll(retireStaleMCPJobsSQL, " ", ""))
	for _, required := range []string{
		"update local_jobs lj", "lj.status='pending'", "lj.command='local_mcp_tool_call'",
		"coalesce(lj.user_id,'')=$1", "coalesce(lj.target_runner_id,'')=$3", "runner_id=$3",
		"lr.id=$3", "coalesce(lr.user_id,'')=$1", "coalesce(lr.device_id,'')=$2",
		"coalesce(lr.session_id,'')=$4", "lr.status='online'", "lr.last_heartbeat",
		"cap->>'catalogrevision'=lj.catalog_revision", "advertised->>'providerid'=lj.mcp_provider_id",
		"advertised->>'logicaltoolname'=lj.mcp_logical_tool_name", "advertised->>'remotetoolname'=lj.mcp_remote_tool_name",
		"status='failed'", "'mcp_catalog_stale'", "result_callback_state='pending'", "followup_callback_state='pending'",
	} {
		if !strings.Contains(query, strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("stale MCP retirement SQL missing %q: %s", required, retireStaleMCPJobsSQL)
		}
	}
	if strings.Contains(query, "statusin('claimed','running')") {
		t.Fatalf("catalog changes must never retire claimed or running work: %s", retireStaleMCPJobsSQL)
	}
}

func TestPendingTerminalCallbackRecoverySQLIsTenantAndSessionScoped(t *testing.T) {
	query := strings.ToLower(strings.ReplaceAll(pendingStaleMCPCallbacksSQL, " ", ""))
	for _, required := range []string{
		"lj.statusin('completed','failed')", "coalesce(lj.user_id,'')=$1", "coalesce(lj.runner_id,'')=$3",
		"coalesce(lj.target_runner_id,'')=''orcoalesce(lj.target_runner_id,'')=$3",
		"result_callback_state='pending'", "followup_callback_state='pending'",
		"result_callback_state='processing'", "result_callback_lease_untilisnull", "result_callback_lease_until<now()",
		"followup_callback_state='processing'", "followup_callback_lease_untilisnull", "followup_callback_lease_until<now()",
		"lr.id=$3", "coalesce(lr.user_id,'')=$1", "coalesce(lr.device_id,'')=$2", "coalesce(lr.session_id,'')=$4",
		"lr.status='online'", "lr.last_heartbeat",
	} {
		if !strings.Contains(query, strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("terminal callback recovery SQL missing %q: %s", required, pendingStaleMCPCallbacksSQL)
		}
	}
}

func TestRetiredMCPJobCountUsesRowsAffected(t *testing.T) {
	if got := retiredMCPJobCount(pgconn.NewCommandTag("UPDATE 0")); got != 0 {
		t.Fatalf("zero-row retirement count=%d", got)
	}
	if got := retiredMCPJobCount(pgconn.NewCommandTag("UPDATE 3")); got != 3 {
		t.Fatalf("retirement count=%d want 3", got)
	}
}

func TestMCPIdempotencyIdentityIncludesFullCatalogRevision(t *testing.T) {
	base := "task-a-node-a"
	revisionA := strings.Repeat("a", 64)
	revisionB := strings.Repeat("b", 64)
	keyA := mcpDispatchIdempotencyKey(base, revisionA)
	if keyA == base || keyA != mcpDispatchIdempotencyKey(base, revisionA) {
		t.Fatalf("same MCP plan must have a stable revision-scoped identity: %q", keyA)
	}
	if keyA == mcpDispatchIdempotencyKey(base, revisionB) {
		t.Fatalf("new catalog revision was swallowed by old identity: %q", keyA)
	}
	if len(keyA) > 128 {
		t.Fatalf("MCP idempotency identity exceeds database column: %d", len(keyA))
	}
	if got := mcpDispatchIdempotencyKey("", revisionA); got != "" {
		t.Fatalf("empty idempotency key must remain empty, got %q", got)
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
