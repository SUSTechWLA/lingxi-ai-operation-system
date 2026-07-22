package localrunner

import (
	"strings"
	"testing"
)

func TestMCPJobManifestUsesExactBoundCatalogContract(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
		InputSchema: map[string]interface{}{"type": "object"},
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"assetId"}, "additionalProperties": false,
		},
	}}
	revision := MCPToolCatalogRevision(tools)
	job := &LocalJob{
		UserID: "user-a", TargetRunnerID: "runner-a", CatalogRevision: revision, Command: CommandLocalMCPToolCall,
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
	}
	catalog := &RunnerMCPToolCatalog{RunnerID: "runner-a", DeviceID: "device-a", UserID: "user-a", Revision: revision, Tools: tools}

	manifest, err := mcpJobManifestFromCatalog(job, catalog)
	if err != nil {
		t.Fatalf("mcpJobManifestFromCatalog returned error: %v", err)
	}
	if manifest == nil || manifest.OutputSchema == nil || manifest.ProviderBinding == nil ||
		manifest.ProviderBinding.TargetRunnerID != "runner-a" || manifest.ProviderBinding.CatalogRevision != revision {
		t.Fatalf("bound output contract missing: %#v", manifest)
	}

	stale := *catalog
	stale.Tools = []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.inspect", RemoteToolName: "inspect",
		InputSchema: map[string]interface{}{"type": "object"},
	}}
	stale.Revision = MCPToolCatalogRevision(stale.Tools)
	if _, err := mcpJobManifestFromCatalog(job, &stale); err == nil || !strings.Contains(err.Error(), "MCP_CATALOG_STALE") {
		t.Fatalf("stale completion contract must fail closed, got %v", err)
	}
	foreign := *catalog
	foreign.UserID = "user-b"
	if _, err := mcpJobManifestFromCatalog(job, &foreign); err == nil || !strings.Contains(err.Error(), "MCP_CATALOG_REPLAN_REQUIRED") {
		t.Fatalf("foreign catalog must fail closed, got %v", err)
	}
}

func TestMCPJobManifestUsesImmutablePayloadSnapshot(t *testing.T) {
	outputSchema := map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
		"required": []interface{}{"assetId"}, "additionalProperties": false,
	}
	job := &LocalJob{
		UserID: "user-a", TargetRunnerID: "runner-a", CatalogRevision: strings.Repeat("a", 64), Command: CommandLocalMCPToolCall,
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
	}
	snapshot := map[string]interface{}{
		"catalogRevision": job.CatalogRevision, "providerId": "studio", "logicalToolName": "studio.render", "remoteToolName": "render",
		"inputSchema": map[string]interface{}{"type": "object"}, "outputSchema": outputSchema, "approvalMode": "before_execute", "timeoutSec": float64(90),
	}
	manifest, err := mcpJobManifestFromSnapshot(job, snapshot)
	if err != nil || manifest == nil || manifest.OutputSchema == nil {
		t.Fatalf("snapshot contract could not be reconstructed: manifest=%#v err=%v", manifest, err)
	}
	snapshot["catalogRevision"] = strings.Repeat("b", 64)
	if _, err := mcpJobManifestFromSnapshot(job, snapshot); err == nil || !strings.Contains(err.Error(), "MCP_CATALOG_REPLAN_REQUIRED") {
		t.Fatalf("tampered snapshot must fail closed, got %v", err)
	}
}
