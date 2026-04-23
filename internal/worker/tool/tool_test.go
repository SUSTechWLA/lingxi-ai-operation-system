package tool

import (
	"context"
	"testing"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
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

func TestToolRegistry_Register(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "test"})

	if !registry.Has("test") {
		t.Error("Expected tool to be registered")
	}

	if registry.Has("nonexistent") {
		t.Error("Should not have nonexistent tool")
	}
}

func TestToolRegistry_Get(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockTool{name: "bash"})

	tool, ok := registry.Get("bash")
	if !ok {
		t.Error("Expected to find tool")
	}
	if tool.Name() != "bash" {
		t.Errorf("Expected tool name 'bash', got '%s'", tool.Name())
	}

	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("Should not find nonexistent tool")
	}
}

func TestDetermineToolName(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		payload  map[string]interface{}
		expected string
	}{
		{
			name:     "tool field in payload",
			nodeType: string(model.NodeTypeLLM),
			payload:  map[string]interface{}{"tool": "bash", "command": "ls"},
			expected: "bash",
		},
		{
			name:     "name field for TOOL type",
			nodeType: string(model.NodeTypeTool),
			payload:  map[string]interface{}{"name": "custom_tool"},
			expected: "custom_tool",
		},
		{
			name:     "default to llm",
			nodeType: string(model.NodeTypeLLM),
			payload:  map[string]interface{}{"prompt": "hello"},
			expected: "llm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineToolName(tt.nodeType, tt.payload)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractParameters(t *testing.T) {
	params := map[string]interface{}{
		"parameters": map[string]interface{}{
			"command": "ls -la",
		},
	}

	result := ExtractParameters(params)
	if result["command"] != "ls -la" {
		t.Errorf("Expected to extract from parameters field, got %v", result)
	}

	inputParams := map[string]interface{}{
		"input": map[string]interface{}{
			"prompt": "hello",
		},
	}

	result = ExtractParameters(inputParams)
	if result["prompt"] != "hello" {
		t.Errorf("Expected to extract from input field, got %v", result)
	}
}

func TestSuccessResult(t *testing.T) {
	data := map[string]interface{}{"key": "value"}
	result := SuccessResult(data)

	if !result.Success {
		t.Error("Expected success to be true")
	}
	if result.Data["key"] != "value" {
		t.Error("Expected data to contain key=value")
	}
}

func TestFailureResult(t *testing.T) {
	result := FailureResult("something went wrong")

	if result.Success {
		t.Error("Expected success to be false")
	}
	if result.Error != "something went wrong" {
		t.Errorf("Expected error message, got %s", result.Error)
	}
}
