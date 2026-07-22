package database

import (
	"strings"
	"testing"
)

func TestLegacyMCPPendingJobsRequireExplicitCatalogReplan(t *testing.T) {
	query := strings.ToLower(localMCPReplanMigrationSQL)
	for _, required := range []string{
		"command='local_mcp_tool_call'", "status in ('pending','claimed','running')",
		"catalog_revision", "mcp_provider_id", "mcp_logical_tool_name", "mcp_remote_tool_name",
		"status='failed'", "mcp_catalog_replan_required", "retryable=false",
	} {
		if !strings.Contains(strings.ReplaceAll(query, " ", ""), strings.ReplaceAll(required, " ", "")) {
			t.Fatalf("MCP compatibility migration missing %q: %s", required, query)
		}
	}
}
