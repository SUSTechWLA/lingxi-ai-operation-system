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

func TestObservabilityRelayMigrationNormalizesFingerprintsBeforeConstraintValidation(t *testing.T) {
	for _, fragment := range []string{
		"UPDATE observability_run_summaries AS summaries",
		"unnest(COALESCE(summaries.error_fingerprints, ARRAY[]::TEXT[]))",
		"SELECT DISTINCT fingerprint",
		"WHERE fingerprint ~ '^[a-f0-9]{64}$'",
		"ORDER BY fingerprint",
		"LIMIT 128",
		"ADD CONSTRAINT observability_run_summary_fingerprint_limit",
		"NOT VALID",
		"VALIDATE CONSTRAINT observability_run_summary_fingerprint_limit",
	} {
		if !strings.Contains(observabilityRelayMigration, fragment) {
			t.Errorf("migration missing %q: %s", fragment, observabilityRelayMigration)
		}
	}
	cleanup := strings.Index(observabilityRelayMigration, "UPDATE observability_run_summaries AS summaries")
	add := strings.Index(observabilityRelayMigration, "ADD CONSTRAINT observability_run_summary_fingerprint_limit")
	validate := strings.Index(observabilityRelayMigration, "VALIDATE CONSTRAINT observability_run_summary_fingerprint_limit")
	if cleanup < 0 || add < 0 || validate < 0 || !(cleanup < add && add < validate) {
		t.Fatalf("migration order cleanup=%d add=%d validate=%d", cleanup, add, validate)
	}
}

func TestObservabilityRelayConstraintLookupIsScopedToTargetRelation(t *testing.T) {
	lookup := "conrelid = 'observability_run_summaries'::regclass"
	if !strings.Contains(observabilityRelayMigration, lookup) {
		t.Fatalf("constraint lookup is not relation-scoped; a same-name constraint on another table could suppress ADD: %s", observabilityRelayMigration)
	}
	create := strings.Index(observabilityRelayMigration, "CREATE TABLE IF NOT EXISTS observability_run_summaries")
	cast := strings.Index(observabilityRelayMigration, lookup)
	if create < 0 || cast < 0 || create >= cast {
		t.Fatalf("target table must exist before regclass lookup: create=%d lookup=%d", create, cast)
	}
	compact := strings.Join(strings.Fields(observabilityRelayMigration), " ")
	if !strings.Contains(compact, "WHERE conname='observability_run_summary_fingerprint_limit' AND "+lookup) {
		t.Fatalf("constraint name and relation are not checked together: %s", compact)
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
