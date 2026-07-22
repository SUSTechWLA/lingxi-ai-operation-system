package localrunner

import (
	"strings"
	"testing"
)

func TestValidateMCPDispatchBindingRequiresImmutableRunnerCatalogIdentity(t *testing.T) {
	req := DispatchLocalJobRequest{
		UserID:             "user-a",
		TargetRunnerID:     "runner-a",
		CatalogRevision:    strings.Repeat("a", 64),
		MCPProviderID:      "studio",
		MCPLogicalToolName: "studio.render",
		MCPRemoteToolName:  "render",
		Command:            CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "studio",
			"mcpTool":    "studio.render",
			"arguments":  map[string]interface{}{"shot": "s1"},
		},
	}
	if err := validateMCPDispatchBinding(&req); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	if req.Payload["providerId"] != "studio" || req.Payload["toolName"] != "studio.render" {
		t.Fatalf("payload was not normalized to immutable binding: %#v", req.Payload)
	}
	if _, ok := req.Payload["mcpTool"]; ok {
		t.Fatalf("legacy routing alias must be removed after binding: %#v", req.Payload)
	}

	req.TargetRunnerID = ""
	if err := validateMCPDispatchBinding(&req); err == nil {
		t.Fatal("target runner is required for MCP jobs")
	}
}

func TestValidateMCPDispatchBindingRejectsNestedToolOverride(t *testing.T) {
	req := DispatchLocalJobRequest{
		UserID: "user-a", TargetRunnerID: "runner-a", CatalogRevision: strings.Repeat("a", 64),
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "studio",
			"toolName":   "studio.render",
			"arguments": map[string]interface{}{
				"toolName": "studio.delete_everything",
			},
		},
	}
	if err := validateMCPDispatchBinding(&req); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("nested override must be rejected, got %v", err)
	}
}

func TestValidateMCPDispatchBindingRejectsTypedNestedToolOverride(t *testing.T) {
	req := DispatchLocalJobRequest{
		UserID: "user-a", TargetRunnerID: "runner-a", CatalogRevision: strings.Repeat("a", 64),
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"arguments": map[string]string{"toolName": "studio.delete_everything"},
		},
	}
	if err := validateMCPDispatchBinding(&req); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("typed nested override must be rejected, got %v", err)
	}
}

func TestCatalogAdvertisesBoundProviderAndRemoteTool(t *testing.T) {
	catalog := MCPToolCatalog{Revision: "rev", Tools: []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render", InputSchema: map[string]interface{}{"type": "object"},
	}}}
	if !catalogAdvertisesBinding(catalog, "studio", "studio.render", "render") {
		t.Fatal("advertised binding was not found")
	}
	if catalogAdvertisesBinding(catalog, "studio", "studio.render", "delete") {
		t.Fatal("unadvertised remote tool must not match")
	}
}

func TestBindMCPContractSnapshotCapturesNonSecretCanonicalSchemas(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"prompt"}, "additionalProperties": false,
		},
		OutputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}}},
	}}
	revision := MCPToolCatalogRevision(tools)
	req := &DispatchLocalJobRequest{
		MCPProviderID: "studio", MCPLogicalToolName: "studio.render", MCPRemoteToolName: "render",
		CatalogRevision: revision, Payload: map[string]interface{}{"arguments": map[string]interface{}{"prompt": "hello"}},
	}
	catalog := &RunnerMCPToolCatalog{RunnerID: "runner-a", UserID: "user-a", Revision: revision, Tools: tools}
	if err := bindMCPContractSnapshot(req, catalog); err != nil {
		t.Fatalf("bindMCPContractSnapshot returned error: %v", err)
	}
	snapshot, _ := req.Payload[mcpContractSnapshotKey].(map[string]interface{})
	if snapshot["catalogRevision"] != revision || snapshot["providerId"] != "studio" || snapshot["remoteToolName"] != "render" {
		t.Fatalf("immutable contract binding missing: %#v", snapshot)
	}
	if _, ok := snapshot["outputSchema"].(map[string]interface{}); !ok {
		t.Fatalf("canonical output schema missing: %#v", snapshot)
	}
	if _, exists := snapshot["command"]; exists {
		t.Fatalf("provider transport leaked into contract snapshot: %#v", snapshot)
	}
	invalid := *req
	invalid.Payload = map[string]interface{}{"arguments": map[string]interface{}{}}
	if err := bindMCPContractSnapshot(&invalid, catalog); err == nil || !strings.Contains(err.Error(), "INPUT_SCHEMA_INVALID") {
		t.Fatalf("invalid dynamic arguments must fail before queueing, got %v", err)
	}
}
