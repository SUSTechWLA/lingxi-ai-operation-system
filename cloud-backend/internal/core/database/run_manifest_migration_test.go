package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRunManifestMigrationIsIdempotentForAgentAndWorkflowRuns(t *testing.T) {
	for _, table := range []string{"agent_runs", "workflow_runs"} {
		for _, column := range []string{"trace_id", "tool_registry_snapshot_id", "run_manifest", "parent_run_id", "replay_from_stage_id"} {
			fragment := "ALTER TABLE " + table + " ADD COLUMN IF NOT EXISTS " + column
			if !strings.Contains(runManifestMigration, fragment) {
				t.Fatalf("migration missing %q: %s", fragment, runManifestMigration)
			}
		}
	}
}

type recordingRunManifestMigrationExecer struct {
	queries []string
}

func (e *recordingRunManifestMigrationExecer) Exec(_ context.Context, query string, _ ...interface{}) (pgconn.CommandTag, error) {
	e.queries = append(e.queries, query)
	return pgconn.NewCommandTag("ALTER TABLE"), nil
}

func TestEnsureRunManifestSchemaExecutesIdempotentMigrationTwice(t *testing.T) {
	execer := &recordingRunManifestMigrationExecer{}
	if err := ensureRunManifestSchema(context.Background(), execer); err != nil {
		t.Fatal(err)
	}
	if err := ensureRunManifestSchema(context.Background(), execer); err != nil {
		t.Fatal(err)
	}
	if len(execer.queries) != 2 || execer.queries[0] != runManifestMigration || execer.queries[1] != runManifestMigration {
		t.Fatalf("migration calls = %#v", execer.queries)
	}
	if strings.Count(execer.queries[0], "ADD COLUMN IF NOT EXISTS") != 10 {
		t.Fatalf("migration is not idempotent: %s", execer.queries[0])
	}
}
