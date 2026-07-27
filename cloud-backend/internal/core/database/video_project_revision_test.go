package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestVideoProjectConfigRevisionMigrationIsBackwardCompatible(t *testing.T) {
	if !strings.Contains(videoProjectConfigRevisionMigration, "ADD COLUMN IF NOT EXISTS config_revision") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "NOT NULL DEFAULT 0") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "video_project_config_revision_guard") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "app.video_project_expected_revision") {
		t.Fatalf("migration is not backward compatible: %s", videoProjectConfigRevisionMigration)
	}
}

func TestEnsureVideoProjectConfigRevisionReturnsStartupError(t *testing.T) {
	execer := revisionMigrationExecer{err: errors.New("permission denied")}
	err := ensureVideoProjectConfigRevision(context.Background(), execer)
	if err == nil || !strings.Contains(err.Error(), "required video project config revision schema") {
		t.Fatalf("error=%v", err)
	}
}

func TestAgentTerminalOutboxMigrationIncludesEventAndClaimIdentity(t *testing.T) {
	for _, column := range []string{
		"terminal_event_id", "terminal_event_claim_token",
		"terminal_event_callback_delivered_at", "terminal_event_observability_delivered_at",
	} {
		if !strings.Contains(agentTerminalOutboxMigration, column) {
			t.Fatalf("terminal outbox migration missing %s: %s", column, agentTerminalOutboxMigration)
		}
	}
	for _, fragment := range []string{
		"evt_agent_terminal_legacy_", "MD5(id)", "terminal_event_json->>'eventId'",
		"agent_terminal_event_identity_required", "agent_terminal_event_phase_order",
		"terminal_event_callback_delivered_at=COALESCE",
		"terminal_event_observability_delivered_at=COALESCE",
	} {
		if !strings.Contains(agentTerminalOutboxMigration, fragment) {
			t.Fatalf("terminal outbox migration missing %q: %s", fragment, agentTerminalOutboxMigration)
		}
	}
}

func TestEnsureAgentTerminalOutboxReturnsStartupError(t *testing.T) {
	execer := revisionMigrationExecer{err: errors.New("permission denied")}
	err := ensureAgentTerminalOutbox(context.Background(), execer)
	if err == nil || !strings.Contains(err.Error(), "required agent terminal outbox schema") {
		t.Fatalf("error=%v", err)
	}
}

func TestLocalJobCallbackOutboxMigrationIncludesAtomicClaimState(t *testing.T) {
	for _, column := range []string{
		"result_callback_claim_token", "result_callback_lease_until",
		"followup_callback_claim_token", "followup_callback_lease_until",
	} {
		if !strings.Contains(localJobCallbackOutboxMigration, column) {
			t.Fatalf("local callback outbox migration missing %s", column)
		}
	}
}

type revisionMigrationExecer struct{ err error }

func (e revisionMigrationExecer) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, e.err
}
