package tool

import (
	"reflect"
	"testing"
)

func TestMCPProviderToolsConvertToToolManifestWithPrefix(t *testing.T) {
	inputSchema := nestedLLMToolTestSchema()
	outputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"videoUrl": map[string]interface{}{"type": "string", "format": "uri"},
		},
		"required":             []interface{}{"videoUrl"},
		"additionalProperties": false,
	}
	manifests := ManifestsFromMCPTools(MCPProviderConfig{
		ID:           "jimeng",
		Label:        "JiMeng MCP",
		Transport:    "stdio",
		ToolPrefix:   "jimeng.",
		Enabled:      true,
		Timeout:      120,
		ApprovalMode: ApprovalBeforeExecute,
	}, []MCPTool{
		{
			Name:         "generate_video",
			Description:  "Generate video",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Annotations:  map[string]interface{}{"readOnlyHint": false, "destructiveHint": true},
		},
	})

	if len(manifests) != 1 {
		t.Fatalf("len(manifests) = %d, want 1", len(manifests))
	}
	m := manifests[0]
	if m.Name != "jimeng.generate_video" {
		t.Fatalf("name = %q, want jimeng.generate_video", m.Name)
	}
	if m.Boundary != BoundaryMCPProvider || m.ExecutionPlane != ExecutionPlaneLocal || !m.RequiresUserDevice {
		t.Fatalf("manifest should be local MCP provider boundary: %#v", m)
	}
	if m.ProviderBinding == nil || m.ProviderBinding.ProviderID != "jimeng" || m.ProviderBinding.RemoteToolName != "generate_video" {
		t.Fatalf("provider binding not set correctly: %#v", m.ProviderBinding)
	}
	if m.LocalCommand != "LOCAL_MCP_TOOL_CALL" {
		t.Fatalf("localCommand = %q, want LOCAL_MCP_TOOL_CALL", m.LocalCommand)
	}
	if !m.ApprovalPolicy.Required || m.ApprovalPolicy.Mode != ApprovalBeforeExecute || !m.ApprovalPolicy.BlocksDownstream {
		t.Fatalf("before_execute provider approval did not become an executable manifest policy: %#v", m.ApprovalPolicy)
	}
	if m.Parameters["scenes"].Type != "array" || !m.Parameters["scenes"].Required {
		t.Fatalf("input schema should convert to ParamDef: %#v", m.Parameters)
	}
	if m.Output["videoUrl"].Type != "string" {
		t.Fatalf("output schema should convert to ParamDef: %#v", m.Output)
	}
	if m.ProviderCapabilities["inputSchema"] == nil || m.ProviderCapabilities["outputSchema"] == nil {
		t.Fatalf("raw MCP schemas should be preserved in providerCapabilities: %#v", m.ProviderCapabilities)
	}
	if !reflect.DeepEqual(m.InputSchema, inputSchema) || !reflect.DeepEqual(m.OutputSchema, outputSchema) {
		t.Fatalf("canonical MCP schemas were not preserved: input=%#v output=%#v", m.InputSchema, m.OutputSchema)
	}
	if !reflect.DeepEqual(m.ProviderCapabilities["inputSchema"], inputSchema) || !reflect.DeepEqual(m.ProviderCapabilities["outputSchema"], outputSchema) {
		t.Fatalf("provider capability schemas were not preserved: %#v", m.ProviderCapabilities)
	}
	if !reflect.DeepEqual(m.ProviderCapabilities["annotations"], map[string]interface{}{"readOnlyHint": false, "destructiveHint": true}) {
		t.Fatalf("MCP annotations were not preserved in manifest provider metadata: %#v", m.ProviderCapabilities["annotations"])
	}
	remoteAnnotations := map[string]interface{}{"readOnlyHint": false}
	cloned := ManifestsFromMCPTools(MCPProviderConfig{ID: "clone", Enabled: true}, []MCPTool{{
		Name: "clone", InputSchema: map[string]interface{}{"type": "object"}, Annotations: remoteAnnotations,
	}})[0]
	remoteAnnotations["readOnlyHint"] = true
	if cloned.ProviderCapabilities["annotations"].(map[string]interface{})["readOnlyHint"] != false {
		t.Fatalf("manifest annotation metadata aliases the MCP discovery payload")
	}

	m.InputSchema["type"] = "string"
	if inputSchema["type"] != "object" || m.ProviderCapabilities["inputSchema"].(map[string]interface{})["type"] != "object" {
		t.Fatalf("canonical input schema must not share references with source or provider capabilities")
	}
	inputSchema["properties"].(map[string]interface{})["callback"].(map[string]interface{})["type"] = "number"
	if reflect.DeepEqual(m.InputSchema["properties"].(map[string]interface{})["callback"].(map[string]interface{})["type"], "number") ||
		reflect.DeepEqual(m.ProviderCapabilities["inputSchema"].(map[string]interface{})["properties"].(map[string]interface{})["callback"].(map[string]interface{})["type"], "number") {
		t.Fatalf("source input schema mutation leaked into manifest copies")
	}
	m.ProviderCapabilities["inputSchema"].(map[string]interface{})["additionalProperties"] = true
	if m.InputSchema["additionalProperties"] != false || inputSchema["additionalProperties"] != false {
		t.Fatalf("provider capability input schema mutation leaked into canonical or source schema")
	}

	m.OutputSchema["additionalProperties"] = true
	if outputSchema["additionalProperties"] != false || m.ProviderCapabilities["outputSchema"].(map[string]interface{})["additionalProperties"] != false {
		t.Fatalf("canonical output schema must not share references with source or provider capabilities")
	}
	outputSchema["properties"].(map[string]interface{})["videoUrl"].(map[string]interface{})["type"] = "number"
	if m.OutputSchema["properties"].(map[string]interface{})["videoUrl"].(map[string]interface{})["type"] != "string" ||
		m.ProviderCapabilities["outputSchema"].(map[string]interface{})["properties"].(map[string]interface{})["videoUrl"].(map[string]interface{})["type"] != "string" {
		t.Fatalf("source output schema mutation leaked into manifest copies")
	}
}

func TestMCPProviderToolsConvertWithToolNameMapAndDisabledTools(t *testing.T) {
	manifests := ManifestsFromMCPTools(MCPProviderConfig{
		ID:            "jimeng",
		Transport:     "stdio",
		ToolPrefix:    "jimeng.",
		ToolNameMap:   map[string]string{"image.generate": "generate_image"},
		DisabledTools: []string{"jimeng.list_task"},
		Enabled:       true,
	}, []MCPTool{
		{Name: "generate_image", Description: "Generate image", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "list_task", Description: "List task", InputSchema: map[string]interface{}{"type": "object"}},
	})

	if len(manifests) != 1 {
		t.Fatalf("manifests = %#v, want only mapped non-disabled tool", manifests)
	}
	if manifests[0].Name != "image.generate" {
		t.Fatalf("mapped logical tool name = %q, want image.generate", manifests[0].Name)
	}
	if manifests[0].ProviderBinding.RemoteToolName != "generate_image" {
		t.Fatalf("remote tool = %q, want generate_image", manifests[0].ProviderBinding.RemoteToolName)
	}
}

func TestMCPProviderInvalidApprovalModeDoesNotDowngradeToNoApproval(t *testing.T) {
	manifests := ManifestsFromMCPTools(MCPProviderConfig{
		ID:           "invalid-config",
		Transport:    "stdio",
		Enabled:      true,
		ApprovalMode: "typo",
	}, []MCPTool{{
		Name:        "dangerous_tool",
		Description: "A tool discovered from an invalid programmatic provider configuration.",
		InputSchema: map[string]interface{}{"type": "object"},
	}})
	if len(manifests) != 1 {
		t.Fatalf("manifests = %#v, want one guarded manifest", manifests)
	}
	policy := manifests[0].ApprovalPolicy
	if !policy.Required || policy.Mode != ApprovalAlways || !policy.BlocksDownstream {
		t.Fatalf("invalid programmatic approval mode must fail closed behind review: %#v", policy)
	}
}

func TestMCPProviderMissingApprovalModeDefaultsToBeforeExecute(t *testing.T) {
	manifests := ManifestsFromMCPTools(MCPProviderConfig{
		ID:        "safe-default",
		Transport: "stdio",
		Enabled:   true,
	}, []MCPTool{{
		Name:        "tool",
		Description: "A tool whose provider omitted approvalMode.",
		InputSchema: map[string]interface{}{"type": "object"},
	}})
	if len(manifests) != 1 {
		t.Fatalf("manifests = %#v, want one guarded manifest", manifests)
	}
	policy := manifests[0].ApprovalPolicy
	if !policy.Required || policy.Mode != ApprovalBeforeExecute || !policy.BlocksDownstream {
		t.Fatalf("missing approval mode must default to before_execute: %#v", policy)
	}
	if manifests[0].ProviderCapabilities["approvalMode"] != ApprovalBeforeExecute {
		t.Fatalf("normalized approval mode missing from provider capabilities: %#v", manifests[0].ProviderCapabilities)
	}
}

func TestMCPProviderExplicitNoneIsTheOnlyNoApprovalMode(t *testing.T) {
	manifests := ManifestsFromMCPTools(MCPProviderConfig{
		ID: "explicit-none", Enabled: true, ApprovalMode: ApprovalNone,
	}, []MCPTool{{Name: "read", InputSchema: map[string]interface{}{"type": "object"}}})
	if len(manifests) != 1 {
		t.Fatalf("manifests = %#v, want one manifest", manifests)
	}
	policy := manifests[0].ApprovalPolicy
	if policy.Required || policy.Mode != ApprovalNone || policy.BlocksDownstream {
		t.Fatalf("explicit none must remain the opt-out from approval: %#v", policy)
	}
}
