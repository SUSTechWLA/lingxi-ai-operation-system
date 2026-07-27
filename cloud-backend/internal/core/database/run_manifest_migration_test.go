package database

import (
	"strings"
	"testing"
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
