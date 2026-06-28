package builtin

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ===== validateBashCommand tests =====

func TestValidateBashCommand_AllowedCommands(t *testing.T) {
	allowed := []string{"ls", "cat", "echo", "curl", "python", "python3",
		"node", "head", "tail", "wc", "grep", "find", "which", "whoami",
		"date", "pwd", "uname", "df", "ps"}

	for _, cmd := range allowed {
		if err := validateBashCommand(cmd); err != nil {
			t.Errorf("Expected '%s' to be allowed, got error: %v", cmd, err)
		}
	}
}

func TestValidateBashCommand_DisallowedCommand(t *testing.T) {
	disallowed := []string{"rm", "chmod", "sudo", "dd", "mkfs", "shutdown"}

	for _, cmd := range disallowed {
		if err := validateBashCommand(cmd); err == nil {
			t.Errorf("Expected '%s' to be rejected", cmd)
		}
	}
}

func TestValidateBashCommand_DangerousPatterns(t *testing.T) {
	dangerous := []string{
		"ls;rm -rf /",
		"ls | grep foo",
		"ls && rm file",
		"ls || echo fail",
		"echo `whoami`",
		"echo $(whoami)",
		"ls > file.txt",
		"ls < input.txt",
		"ls >> file.txt",
		"ls &",
	}

	for _, cmd := range dangerous {
		if err := validateBashCommand(cmd); err == nil {
			t.Errorf("Expected dangerous pattern to be rejected: '%s'", cmd)
		}
	}
}

func TestValidateBashCommand_EmptyCommand(t *testing.T) {
	if err := validateBashCommand(""); err == nil {
		t.Error("Expected error for empty command")
	}
	if err := validateBashCommand("   "); err == nil {
		t.Error("Expected error for whitespace-only command")
	}
}

func TestValidateBashCommand_AllowedWithArgs(t *testing.T) {
	if err := validateBashCommand("ls -la /tmp"); err != nil {
		t.Errorf("Expected 'ls -la /tmp' to be allowed, got error: %v", err)
	}
	if err := validateBashCommand("grep -r pattern ."); err != nil {
		t.Errorf("Expected 'grep -r pattern .' to be allowed, got error: %v", err)
	}
}

// ===== BashTool tests =====

func TestBashTool_Interface(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	if bt.Name() != "bash" {
		t.Errorf("Expected name 'bash', got '%s'", bt.Name())
	}
	if bt.Type() != tool.ToolTypeCustom {
		t.Errorf("Expected CUSTOM type, got %v", bt.Type())
	}
}

func TestBashTool_ValidateParameters(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	if !bt.ValidateParameters(map[string]interface{}{"command": "ls"}) {
		t.Error("Expected valid for string command")
	}
	if bt.ValidateParameters(map[string]interface{}{"command": 123}) {
		t.Error("Expected invalid for non-string command")
	}
	if bt.ValidateParameters(map[string]interface{}{}) {
		t.Error("Expected invalid for missing command")
	}
}

func TestBashTool_Execute_EmptyCommand(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	result := bt.Execute(context.Background(), map[string]interface{}{"command": ""}, tool.ToolContext{})
	if result.Success {
		t.Error("Expected failure for empty command")
	}
}

func TestBashTool_Execute_DangerousCommand(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	result := bt.Execute(context.Background(), map[string]interface{}{"command": "rm -rf /"}, tool.ToolContext{})
	if result.Success {
		t.Error("Expected failure for dangerous command")
	}
}

func TestBashTool_Execute_SafeCommand(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	result := bt.Execute(context.Background(), map[string]interface{}{"command": "echo hello"}, tool.ToolContext{})
	if !result.Success {
		t.Errorf("Expected success for safe command, got error: %s", result.Error)
	}
}

func TestBashTool_Execute_CommandNotAllowed(t *testing.T) {
	cfg := testBashConfig()
	bt := NewBashTool(cfg)

	result := bt.Execute(context.Background(), map[string]interface{}{"command": "sudo ls"}, tool.ToolContext{})
	if result.Success {
		t.Error("Expected failure for non-allowed base command")
	}
}

func testBashConfig() config.BashToolConfig {
	return config.BashToolConfig{TimeoutSeconds: 10}
}

// ===== LlmApiTool tests =====

func TestLlmApiTool_Interface(t *testing.T) {
	lt := NewLlmApiTool(config.OpenAIConfig{})

	if lt.Name() != "llm_api" {
		t.Errorf("Expected name 'llm_api', got '%s'", lt.Name())
	}
	if lt.Type() != tool.ToolTypeLLM {
		t.Errorf("Expected LLM type, got %v", lt.Type())
	}
}

func TestLlmApiTool_ValidateParameters(t *testing.T) {
	lt := NewLlmApiTool(config.OpenAIConfig{})

	if !lt.ValidateParameters(map[string]interface{}{"prompt": "hello"}) {
		t.Error("Expected valid with 'prompt'")
	}
	if !lt.ValidateParameters(map[string]interface{}{"message": "hello"}) {
		t.Error("Expected valid with 'message'")
	}
	if !lt.ValidateParameters(map[string]interface{}{"content": "hello"}) {
		t.Error("Expected valid with 'content'")
	}
	if lt.ValidateParameters(map[string]interface{}{}) {
		t.Error("Expected invalid with no prompt/message/content")
	}
	if lt.ValidateParameters(map[string]interface{}{"prompt": 123}) {
		t.Error("Expected invalid with non-string prompt")
	}
}

func TestLlmApiTool_Execute_NoAPIKey(t *testing.T) {
	lt := NewLlmApiTool(config.OpenAIConfig{})

	result := lt.Execute(context.Background(), map[string]interface{}{"prompt": "hello"}, tool.ToolContext{})
	if result.Success {
		t.Error("Expected failure when API key not configured")
	}
}

func TestLlmApiTool_Execute_NoPrompt(t *testing.T) {
	lt := NewLlmApiTool(config.OpenAIConfig{APIKey: "test-key"})

	result := lt.Execute(context.Background(), map[string]interface{}{}, tool.ToolContext{})
	if result.Success {
		t.Error("Expected failure when no prompt provided")
	}
}

func TestBuildChatMessagesIncludesSystemPrompt(t *testing.T) {
	messages := buildChatMessages("只输出JSON", "生成事实包", nil)
	if len(messages) != 2 {
		t.Fatalf("expected system and user messages, got %#v", messages)
	}
	system, ok := messages[0].(map[string]interface{})
	if !ok || system["role"] != "system" || system["content"] != "只输出JSON" {
		t.Fatalf("unexpected system message: %#v", messages[0])
	}
	user, ok := messages[1].(map[string]interface{})
	if !ok || user["role"] != "user" {
		t.Fatalf("unexpected user message: %#v", messages[1])
	}
}

// ===== extractContent tests =====

func TestExtractContent_ValidResponse(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "Hello, world!",
				},
			},
		},
	}

	content := extractContent(response)
	if content != "Hello, world!" {
		t.Errorf("Expected 'Hello, world!', got '%s'", content)
	}
}

func TestExtractContent_DoesNotExposeReasoningContent(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content":           "",
					"reasoning_content": "先分析，再输出答案。",
				},
			},
		},
	}

	content := extractContent(response)
	if content != "" {
		t.Errorf("Expected empty content when only reasoning_content is present, got %q", content)
	}
}

func TestExtractContent_EmptyChoices(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{},
	}

	content := extractContent(response)
	if content != "" {
		t.Errorf("Expected empty string, got '%s'", content)
	}
}

func TestExtractContent_NoChoices(t *testing.T) {
	response := map[string]interface{}{}

	content := extractContent(response)
	if content != "" {
		t.Errorf("Expected empty string, got '%s'", content)
	}
}

func TestExtractFinishReason(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"finish_reason": "length",
				"message": map[string]interface{}{
					"content": "partial",
				},
			},
		},
	}

	if got := extractFinishReason(response); got != "length" {
		t.Fatalf("expected finish reason length, got %q", got)
	}
}

func TestExtractContent_InvalidChoiceFormat(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{"invalid"},
	}

	content := extractContent(response)
	if content != "" {
		t.Errorf("Expected empty string for invalid format, got '%s'", content)
	}
}

func TestExtractContent_MissingMessage(t *testing.T) {
	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{},
		},
	}

	content := extractContent(response)
	if content != "" {
		t.Errorf("Expected empty string, got '%s'", content)
	}
}
