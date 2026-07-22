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
		ID:         "jimeng",
		Label:      "JiMeng MCP",
		Transport:  "stdio",
		ToolPrefix: "jimeng.",
		Enabled:    true,
		Timeout:    120,
	}, []MCPTool{
		{
			Name:         "generate_video",
			Description:  "Generate video",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
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
