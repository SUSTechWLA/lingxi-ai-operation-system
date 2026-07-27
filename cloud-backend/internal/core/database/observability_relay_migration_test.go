package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestObservabilityRelayMigrationIsIdempotentAndOwnershipScoped(t *testing.T) {
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS observability_event_outbox",
		"event_id VARCHAR(128) PRIMARY KEY",
		"user_id VARCHAR(64) NOT NULL",
		"redacted_payload JSONB NOT NULL",
		"delivered_at TIMESTAMPTZ",
		"expires_at TIMESTAMPTZ NOT NULL",
		"CREATE INDEX IF NOT EXISTS idx_observability_event_outbox_user",
		"CREATE INDEX IF NOT EXISTS idx_observability_event_outbox_correlation",
		"CREATE TABLE IF NOT EXISTS observability_run_summaries",
		"PRIMARY KEY (user_id, run_id)",
		"last_event_id VARCHAR(128) NOT NULL",
		"cardinality(error_fingerprints) <= 128",
	} {
		if !strings.Contains(observabilityRelayMigration, fragment) {
			t.Fatalf("migration missing %q: %s", fragment, observabilityRelayMigration)
		}
	}
}

type recordingObservabilityMigrationExecer struct{ calls []string }

func (execer *recordingObservabilityMigrationExecer) Exec(_ context.Context, query string, _ ...interface{}) (pgconn.CommandTag, error) {
	execer.calls = append(execer.calls, query)
	return pgconn.NewCommandTag("CREATE TABLE"), nil
}

func TestEnsureObservabilityRelaySchemaExecutesMigration(t *testing.T) {
	execer := &recordingObservabilityMigrationExecer{}
	if err := ensureObservabilityRelaySchema(context.Background(), execer); err != nil {
		t.Fatal(err)
	}
	if len(execer.calls) != 1 || execer.calls[0] != observabilityRelayMigration {
		t.Fatalf("migration calls = %#v", execer.calls)
	}
}
