package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/fake"
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

func TestLlmApiTool_Execute_UsesIntegerMaxTokensAndJSONMode(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"facts\":[\"ok\"],\"summary\":\"done\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	lt := NewLlmApiTool(config.OpenAIConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "deepseek-v4-pro",
		MaxTokens:   512,
		Temperature: 0.7,
		Timeout:     5,
	})

	result := lt.Execute(context.Background(), map[string]interface{}{
		"prompt":          "只输出 JSON",
		"max_tokens":      8000,
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	}, tool.ToolContext{TaskID: "task-1"})
	if !result.Success {
		t.Fatalf("expected success, got %s", result.Error)
	}
	if gotBody["max_tokens"] != float64(8000) {
		t.Fatalf("expected max_tokens 8000, got %#v", gotBody["max_tokens"])
	}
	if gotBody["temperature"] != float64(0) {
		t.Fatalf("expected temperature 0, got %#v", gotBody["temperature"])
	}
	rf, ok := gotBody["response_format"].(map[string]interface{})
	if !ok || rf["type"] != "json_object" {
		t.Fatalf("expected json_object response_format, got %#v", gotBody["response_format"])
	}
}

func TestLlmApiTool_Execute_UsesRuntimeModelProviderConfig(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	oldBaseURL := os.Getenv("OPENAI_BASE_URL")
	oldModel := os.Getenv("OPENAI_MODEL")
	t.Cleanup(func() {
		setOrUnsetEnv("OPENAI_API_KEY", oldKey)
		setOrUnsetEnv("OPENAI_BASE_URL", oldBaseURL)
		setOrUnsetEnv("OPENAI_MODEL", oldModel)
		ClearRuntimeModelProviderConfig()
	})

	var initialHit bool
	initialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialHit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"initial"},"finish_reason":"stop"}]}`))
	}))
	defer initialServer.Close()

	var runtimeHit bool
	var gotAuth string
	var gotBody map[string]interface{}
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runtimeHit = true
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"runtime"},"finish_reason":"stop"}]}`))
	}))
	defer runtimeServer.Close()

	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:      "initial-key",
		BaseURL:     initialServer.URL,
		Model:       "initial-model",
		MaxTokens:   128,
		Temperature: 0.7,
		Timeout:     5,
	}, "")
	SetRuntimeModelProviderConfig(RuntimeModelProviderConfig{
		BaseURL: runtimeServer.URL,
		APIKey:  "runtime-key",
		Model:   "runtime-model",
	})

	lt := NewLlmApiTool(config.OpenAIConfig{
		APIKey:      "initial-key",
		BaseURL:     initialServer.URL,
		Model:       "initial-model",
		MaxTokens:   128,
		Temperature: 0.7,
		Timeout:     5,
	})

	result := lt.Execute(context.Background(), map[string]interface{}{"prompt": "hello"}, tool.ToolContext{})
	if !result.Success {
		t.Fatalf("expected success, got %s", result.Error)
	}
	if initialHit {
		t.Fatal("llm_api used stale startup config instead of runtime model provider config")
	}
	if !runtimeHit {
		t.Fatal("runtime model provider endpoint was not called")
	}
	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("expected runtime API key, got %q", gotAuth)
	}
	if gotBody["model"] != "runtime-model" {
		t.Fatalf("expected runtime model, got %#v", gotBody["model"])
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

func TestBidAnalysisReportToolWritesCloudGeneratedReport(t *testing.T) {
	// Save/restore global modelGateway around test.
	oldGW := modelGateway
	defer func() { modelGateway = oldGW }()

	gw := modelgateway.NewGateway("fake")
	fakeProvider := fake.NewProvider()
	gw.RegisterProvider(fakeProvider, modelgateway.CapTextToText)
	modelGateway = gw

	reportPath := filepath.Join(t.TempDir(), "00_招标文件解析报告.md")
	rawTextPath := filepath.Join(t.TempDir(), "00_招标文件原文解析.md")
	if err := os.WriteFile(rawTextPath, []byte("招标文件正文"), 0o644); err != nil {
		t.Fatalf("failed to write raw text file: %v", err)
	}
	bt := NewBidAnalysisReportTool()

	result := bt.Execute(context.Background(), map[string]interface{}{
		"raw_text_path": rawTextPath,
		"source_file":   "E:/bid/test.docx",
		"report_path":   reportPath,
	}, tool.ToolContext{TaskID: "task-1"})

	if !result.Success {
		t.Fatalf("expected success, got %s", result.Error)
	}
	content, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected report file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "E:/bid/test.docx") || !strings.Contains(text, "招标文件正文") {
		t.Fatalf("report content missing expected sections: %s", text)
	}
	if result.Data["report_path"] != reportPath {
		t.Fatalf("report_path output = %#v", result.Data["report_path"])
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) != 1 {
		t.Fatalf("expected one artifact manifest, got %#v", result.Data["artifacts"])
	}
	if artifacts[0]["kind"] != "BID_ANALYSIS" || artifacts[0]["storageRef"] != reportPath {
		t.Fatalf("unexpected artifact manifest: %#v", artifacts[0])
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
