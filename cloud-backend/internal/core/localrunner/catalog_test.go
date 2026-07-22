package localrunner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateCapabilitiesRejectsUnsafeOrOversizedMCPCatalogs(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID:      "studio",
		LogicalToolName: "studio.render",
		RemoteToolName:  "remote_render",
		Description:     "Render one shot",
		InputSchema:     map[string]any{"type": "object"},
		OutputSchema:    map[string]any{"type": "object"},
		Annotations: MCPToolAnnotations{
			ReadOnlyHint: boolPointer(false),
		},
		ApprovalMode: "before_execute",
		TimeoutSec:   45,
	}}
	valid := []RunnerCapability{{
		ToolName:        "mcp_tool_call",
		Command:         CommandLocalMCPToolCall,
		Available:       true,
		CatalogRevision: MCPToolCatalogRevision(tools),
		MCPTools:        tools,
	}}
	if err := ValidateRunnerCapabilities(valid); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}

	oversized := valid
	oversized[0].MCPTools[0].Description = strings.Repeat("x", MaxMCPToolDescriptionBytes+1)
	if err := ValidateRunnerCapabilities(oversized); err == nil {
		t.Fatal("oversized description must be rejected")
	}
}

func TestValidateCapabilitiesRejectsForgedCatalogRevision(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID: "p", LogicalToolName: "p.echo", RemoteToolName: "echo", InputSchema: map[string]any{"type": "object"},
	}}
	err := ValidateRunnerCapabilities([]RunnerCapability{{
		ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true,
		CatalogRevision: strings.Repeat("0", 64), MCPTools: tools,
	}})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("forged revision must be rejected, got %v", err)
	}
}

func TestMCPToolCatalogFromCapabilitiesReturnsSafeCatalog(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID: "p", LogicalToolName: "p.echo", RemoteToolName: "echo", InputSchema: map[string]any{"type": "object"},
	}}
	catalog, ok := MCPToolCatalogFromCapabilities([]RunnerCapability{{
		ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true,
		CatalogRevision: MCPToolCatalogRevision(tools), MCPTools: tools,
	}})
	if !ok || catalog.Revision == "" || len(catalog.Tools) != 1 || catalog.Tools[0].RemoteToolName != "echo" {
		t.Fatalf("catalog extraction failed: %#v ok=%v", catalog, ok)
	}
}

func TestValidateCapabilitiesRejectsMultipleOrDuplicateMCPCatalogEntries(t *testing.T) {
	tools := []MCPToolAdvertisement{{
		ProviderID: "p", LogicalToolName: "p.echo", RemoteToolName: "echo", InputSchema: map[string]any{"type": "object"},
	}}
	revision := MCPToolCatalogRevision(tools)
	duplicateCatalog := []RunnerCapability{
		{ToolName: "mcp-1", Command: CommandLocalMCPToolCall, Available: true, CatalogRevision: revision, MCPTools: tools},
		{ToolName: "mcp-2", Command: CommandLocalMCPToolCall, Available: true, CatalogRevision: revision, MCPTools: tools},
	}
	if err := ValidateRunnerCapabilities(duplicateCatalog); err == nil {
		t.Fatal("multiple MCP catalog capabilities must be rejected")
	}

	duplicatedTools := append(append([]MCPToolAdvertisement{}, tools...), tools...)
	if err := ValidateRunnerCapabilities([]RunnerCapability{{
		ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true,
		CatalogRevision: MCPToolCatalogRevision(duplicatedTools), MCPTools: duplicatedTools,
	}}); err == nil {
		t.Fatal("duplicate MCP tool bindings must be rejected")
	}
}

func TestMCPToolAdvertisementWireCannotCarryTransportSecrets(t *testing.T) {
	const secret = "secret-canary-do-not-sync"
	ad := MCPToolAdvertisement{
		ProviderID:      "studio",
		LogicalToolName: "studio.render",
		RemoteToolName:  "render",
		InputSchema:     map[string]any{"type": "object"},
	}
	wire, err := json.Marshal(ad)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) || strings.Contains(string(wire), "endpoint") || strings.Contains(string(wire), "command") || strings.Contains(string(wire), "headers") || strings.Contains(string(wire), "env") {
		t.Fatalf("unsafe advertisement wire: %s", wire)
	}
}

func TestCloudCatalogFailsClosedOnInvalidSchemaStructure(t *testing.T) {
	base := MCPToolAdvertisement{
		ProviderID: "p", LogicalToolName: "p.echo", RemoteToolName: "echo",
		InputSchema: map[string]any{"type": "object"},
	}
	for name, schema := range map[string]map[string]any{
		"array root":   {"type": "array"},
		"remote ref":   {"type": "object", "properties": map[string]any{"x": map[string]any{"$ref": "https://evil.invalid/schema"}}},
		"bad required": {"type": "object", "required": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			tool := base
			tool.InputSchema = schema
			tools := []MCPToolAdvertisement{tool}
			err := ValidateRunnerCapabilities([]RunnerCapability{{
				ToolName: "mcp", Command: CommandLocalMCPToolCall, Available: true,
				CatalogRevision: MCPToolCatalogRevision(tools), MCPTools: tools,
			}})
			if err == nil {
				t.Fatalf("invalid schema accepted: %#v", schema)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
