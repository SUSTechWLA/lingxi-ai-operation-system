package tool

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/model"
)

type mockTool struct {
	name string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string  { return "mock tool" }
func (m *mockTool) Type() ToolType       { return ToolTypeCustom }
func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult {
	return SuccessResult(map[string]interface{}{"ok": true})
}
func (m *mockTool) ValidateParameters(params map[string]interface{}) bool { return true }

// ===== ToolRegistry tests =====

func TestToolRegistry_NewRegistryIsEmpty(t *testing.T) {
	registry := NewToolRegistry()
	if len(registry.tools) != 0 {
		t.Error("New registry should be empty")
	}
}

func TestToolRegistry_RegisterMultiple(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "bash"})
	registry.Register(&mockTool{name: "test_tool"})
	registry.Register(&mockTool{name: "llm_api"})

	if len(registry.tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(registry.tools))
	}
}

func TestToolRegistry_RegisterOverwrite(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "test"})
	registry.Register(&mockTool{name: "test"}) // overwrite

	if len(registry.tools) != 1 {
		t.Errorf("Expected 1 tool after overwrite, got %d", len(registry.tools))
	}
}

func TestToolRegistry_All(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "bash"})
	registry.Register(&mockTool{name: "test_tool"})

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("Expected 2 tools in All(), got %d", len(all))
	}
}

// ===== DetermineToolName tests =====

func TestDetermineToolName_ToolOverride(t *testing.T) {
	result := DetermineToolName("LLM", map[string]interface{}{"tool": "custom_tool"})
	if result != "custom_tool" {
		t.Errorf("Expected 'custom_tool', got '%s'", result)
	}
}

func TestDetermineToolName_ToolTypeUsesName(t *testing.T) {
	result := DetermineToolName(string(model.NodeTypeTool), map[string]interface{}{"name": "test_tool"})
	if result != "test_tool" {
		t.Errorf("Expected 'test_tool', got '%s'", result)
	}
}

func TestDetermineToolName_LLMDefault(t *testing.T) {
	result := DetermineToolName(string(model.NodeTypeLLM), map[string]interface{}{"prompt": "hello"})
	if result != "llm_api" {
		t.Errorf("Expected 'llm_api', got '%s'", result)
	}
}

func TestDetermineToolName_EmptyPayload(t *testing.T) {
	result := DetermineToolName("UNKNOWN", map[string]interface{}{})
	if result != "llm_api" {
		t.Errorf("Expected 'llm_api' as default, got '%s'", result)
	}
}

func TestDetermineToolName_ToolOverridePriority(t *testing.T) {
	// Even for TOOL type, explicit "tool" field takes priority over "name"
	result := DetermineToolName(string(model.NodeTypeTool), map[string]interface{}{
		"tool": "override",
		"name": "test_tool",
	})
	if result != "override" {
		t.Errorf("Expected 'override' (tool field priority), got '%s'", result)
	}
}

func TestDetermineToolName_ToolTypeEmptyName(t *testing.T) {
	result := DetermineToolName(string(model.NodeTypeTool), map[string]interface{}{
		"name": "",
	})
	// Falls through to default
	if result != "llm_api" {
		t.Errorf("Expected 'llm_api' for empty name, got '%s'", result)
	}
}

func TestDetermineToolName_ToolTypeNonStringName(t *testing.T) {
	result := DetermineToolName(string(model.NodeTypeTool), map[string]interface{}{
		"name": 123,
	})
	if result != "llm_api" {
		t.Errorf("Expected 'llm_api' for non-string name, got '%s'", result)
	}
}

// ===== ExtractParameters tests =====

func TestExtractParameters_ParametersField(t *testing.T) {
	payload := map[string]interface{}{
		"parameters": map[string]interface{}{
			"command": "ls",
		},
	}

	result := ExtractParameters(payload)
	if result["command"] != "ls" {
		t.Errorf("Expected to extract 'command=ls', got %v", result)
	}
}

func TestExtractParameters_InputField(t *testing.T) {
	payload := map[string]interface{}{
		"input": map[string]interface{}{
			"prompt": "hello",
		},
	}

	result := ExtractParameters(payload)
	if result["prompt"] != "hello" {
		t.Errorf("Expected to extract 'prompt=hello', got %v", result)
	}
}

func TestExtractParameters_ParametersPriorityOverInput(t *testing.T) {
	payload := map[string]interface{}{
		"parameters": map[string]interface{}{"key": "from_params"},
		"input":      map[string]interface{}{"key": "from_input"},
	}

	result := ExtractParameters(payload)
	if result["key"] != "from_params" {
		t.Errorf("Expected 'parameters' to take priority, got %v", result["key"])
	}
}

func TestExtractParameters_FallbackToPayload(t *testing.T) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	}

	result := ExtractParameters(payload)
	if result["key1"] != "value1" || result["key2"] != "value2" {
		t.Errorf("Expected fallback to payload itself, got %v", result)
	}
}

func TestExtractParameters_NonMapParameters(t *testing.T) {
	payload := map[string]interface{}{
		"parameters": "not a map",
		"input":      map[string]interface{}{"key": "from_input"},
	}

	result := ExtractParameters(payload)
	if result["key"] != "from_input" {
		t.Errorf("Expected fallback to 'input' when 'parameters' is not a map, got %v", result)
	}
}

// ===== ToolResult tests =====

func TestSuccessResult_Fields(t *testing.T) {
	data := map[string]interface{}{"exitCode": 0, "output": "done"}
	result := SuccessResult(data)

	if !result.Success {
		t.Error("Expected Success=true")
	}
	if result.Data["exitCode"] != 0 {
		t.Error("Expected exitCode in data")
	}
	if result.Error != "" {
		t.Errorf("Expected empty error, got '%s'", result.Error)
	}
}

func TestFailureResult_Fields(t *testing.T) {
	result := FailureResult("timeout")

	if result.Success {
		t.Error("Expected Success=false")
	}
	if result.Error != "timeout" {
		t.Errorf("Expected error='timeout', got '%s'", result.Error)
	}
	if result.Data != nil {
		t.Errorf("Expected nil data, got %v", result.Data)
	}
}

func TestToolContext_Fields(t *testing.T) {
	tc := ToolContext{TaskID: "t1", NodeID: "n1", RetryCount: 3}
	if tc.TaskID != "t1" || tc.NodeID != "n1" || tc.RetryCount != 3 {
		t.Error("ToolContext fields mismatch")
	}
}
