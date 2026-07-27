package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

func TestNormalizeAgentTerminalEventIDRepairsEveryInvalidSQLIdentity(t *testing.T) {
	tests := []struct {
		name   string
		runID  string
		sqlID  string
		jsonID string
		want   string
	}{
		{name: "null", runID: "agr_null", jsonID: "evt_json_null", want: "evt_json_null"},
		{name: "blank", runID: "agr_blank", sqlID: "", jsonID: "evt_json_blank", want: "evt_json_blank"},
		{name: "whitespace", runID: "agr_whitespace", sqlID: "  ", jsonID: "evt_json_whitespace", want: "evt_json_whitespace"},
		{name: "old legacy", runID: "agr_old_legacy", sqlID: "legacy_terminal_old", jsonID: "evt_json_legacy", want: "evt_json_legacy"},
		{name: "invalid characters", runID: "agr_invalid", sqlID: "event invalid", jsonID: "evt_json_invalid", want: "evt_json_invalid"},
		{name: "invalid json", runID: "agr_fallback", sqlID: "bad", jsonID: " event bad ", want: "evt_agent_terminal_legacy_agr_fallback"},
		{name: "missing json", runID: "agr_missing", sqlID: "bad", want: "evt_agent_terminal_legacy_agr_missing"},
		{name: "unsafe run id", runID: "run id with spaces", sqlID: "bad", want: "evt_agent_terminal_legacy_aa51038bc6a3d88a95e9fa7b05f6b653"},
		{name: "valid sql remains authoritative", runID: "agr_conflict", sqlID: "evt_sql_valid", jsonID: "evt_json_conflict", want: "evt_sql_valid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeAgentTerminalEventID(test.runID, test.sqlID, test.jsonID); got != test.want {
				t.Fatalf("normalizeAgentTerminalEventID() = %q, want %q", got, test.want)
			}
			if !ValidAgentTerminalEventID(test.want) {
				t.Fatalf("normalized identity %q is invalid", test.want)
			}
		})
	}
}

func TestValidAgentTerminalEventIDRejectsWhitespaceAndInvalidForms(t *testing.T) {
	for _, id := range []string{"", " ", " evt_ok", "evt_ok ", "legacy_terminal_old", "evt_bad/value", "evt_秘密", "evt_" + strings.Repeat("x", 93)} {
		if ValidAgentTerminalEventID(id) {
			t.Errorf("ValidAgentTerminalEventID(%q) = true", id)
		}
	}
	for _, id := range []string{"evt_a", "evt_agent_terminal_legacy_agr_123", "evt_" + strings.Repeat("x", 92)} {
		if !ValidAgentTerminalEventID(id) {
			t.Errorf("ValidAgentTerminalEventID(%q) = false", id)
		}
	}
}

func TestAgentTerminalOutboxMigrationFreshAndUpgradePostgres(t *testing.T) {
	dsn := os.Getenv("AGENT_TERMINAL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("LOCALRUNNER_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set AGENT_TERMINAL_TEST_DATABASE_URL to exercise the migration against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE agent_runs (id TEXT PRIMARY KEY, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	// Fresh install must be executable before any legacy rows exist.
	if _, err := tx.Exec(ctx, agentTerminalOutboxMigration); err != nil {
		t.Fatalf("fresh migration: %v", err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE agent_runs DROP CONSTRAINT agent_terminal_event_identity_required`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_runs (id, terminal_event_json, terminal_event_id) VALUES
		('agr_null', '{"eventId":"evt_json_null"}', NULL),
		('agr_blank', '{"eventId":"evt_json_blank"}', ''),
		('agr_space', '{"eventId":"evt_json_space"}', '  '),
		('agr_invalid', '{"eventId":"evt_json_invalid"}', 'bad/value'),
		('agr_fallback', '{"eventId":"bad value"}', 'legacy_terminal_old'),
		('agr_conflict', '{"eventId":"evt_json_conflict"}', 'evt_sql_valid')`); err != nil {
		t.Fatal(err)
	}
	// Upgrade rerun must repair all invalid identities without rewriting a valid,
	// potentially conflicting SQL identity that ClaimTerminalEvents must reject.
	if _, err := tx.Exec(ctx, agentTerminalOutboxMigration); err != nil {
		t.Fatalf("upgrade migration: %v", err)
	}
	rows, err := tx.Query(ctx, `SELECT id, terminal_event_id FROM agent_runs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, eventID string
		if err := rows.Scan(&id, &eventID); err != nil {
			t.Fatal(err)
		}
		got[id] = eventID
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"agr_null": "evt_json_null", "agr_blank": "evt_json_blank", "agr_space": "evt_json_space",
		"agr_invalid": "evt_json_invalid", "agr_fallback": "evt_agent_terminal_legacy_agr_fallback",
		"agr_conflict": "evt_sql_valid",
	} {
		if got[id] != want {
			t.Errorf("%s terminal_event_id = %q, want %q", id, got[id], want)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO agent_runs (id, terminal_event_json, terminal_event_id) VALUES ('agr_reject', '{}', ' ')`); err == nil {
		t.Fatal("strict identity constraint accepted whitespace")
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
