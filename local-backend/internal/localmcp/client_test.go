package localmcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClientUsesStandardHTTPHandshakePaginatesAndReusesSession(t *testing.T) {
	var initialized atomic.Int32
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0.0"},
		&mcp.ServerOptions{
			PageSize: 1,
			InitializedHandler: func(context.Context, *mcp.InitializedRequest) {
				initialized.Add(1)
			},
		},
	)
	readOnly := true
	server.AddTool(&mcp.Tool{
		Name:        "alpha",
		Title:       "Alpha display name",
		Description: "first tool",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
		OutputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		},
		Annotations: &mcp.ToolAnnotations{Title: "Annotated alpha", ReadOnlyHint: readOnly},
		Icons:       []mcp.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+", MIMEType: "image/svg+xml", Sizes: []string{"any"}, Theme: mcp.IconThemeDark}},
		Meta:        mcp.Meta{"vendor/quality": "production"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return richToolResult(), nil
	})
	server.AddTool(&mcp.Tool{
		Name:        "beta",
		Description: "second tool",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "beta"}}}, nil
	})

	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()

	client := NewClient(ProviderConfig{ID: "standard", Endpoint: httpServer.URL, Enabled: true}, httpServer.Client())
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "alpha" || tools[1].Name != "beta" {
		t.Fatalf("Tools() pagination result = %#v, want alpha and beta", tools)
	}
	alpha := tools[0]
	if alpha.Title != "Alpha display name" || alpha.Description != "first tool" {
		t.Fatalf("tool display metadata was lost: %#v", alpha)
	}
	if alpha.InputSchema["type"] != "object" || alpha.OutputSchema["type"] != "object" {
		t.Fatalf("tool schemas were lost: input=%#v output=%#v", alpha.InputSchema, alpha.OutputSchema)
	}
	if alpha.Annotations["title"] != "Annotated alpha" || alpha.Annotations["readOnlyHint"] != true {
		t.Fatalf("tool annotations were lost: %#v", alpha.Annotations)
	}
	if len(alpha.Icons) != 1 || alpha.Icons[0]["src"] == "" || alpha.Icons[0]["theme"] != "dark" {
		t.Fatalf("tool icons were lost: %#v", alpha.Icons)
	}
	if alpha.Meta["vendor/quality"] != "production" {
		t.Fatalf("tool meta was lost: %#v", alpha.Meta)
	}

	result, err := client.CallTool(context.Background(), "alpha", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	assertRichToolResult(t, result)
	if got := initialized.Load(); got != 1 {
		t.Fatalf("initialized sessions = %d, want one cached session", got)
	}
}

func TestClientInjectsConfiguredHeadersWithoutMutatingHTTPClient(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "headers", Version: "1.0.0"}, nil)
	server.AddTool(&mcp.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
	})
	standardHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var missingHeader atomic.Bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Workspace") != "test" {
			missingHeader.Store(true)
		}
		standardHandler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	baseClient := httpServer.Client()
	originalTransport := baseClient.Transport
	client := NewClient(ProviderConfig{
		ID:       "headers",
		Endpoint: httpServer.URL,
		Headers:  map[string]string{"Authorization": "Bearer secret", "X-Workspace": "test"},
		Enabled:  true,
	}, baseClient)
	defer client.Close()
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if missingHeader.Load() {
		t.Fatal("configured headers were not attached to every MCP HTTP request")
	}
	if baseClient.Transport != originalTransport {
		t.Fatal("NewClient mutated the caller-owned http.Client")
	}
}

func TestClientFiltersAndMapsLogicalNames(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "mapping", Version: "1.0.0"}, &mcp.ServerOptions{PageSize: 1})
	for _, name := range []string{"generate_video", "list_task", "check_status"} {
		name := name
		server.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "ok"}},
				StructuredContent: map[string]any{"calledTool": req.Params.Name},
			}, nil
		})
	}
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()

	client := NewClient(ProviderConfig{
		ID:            "jimeng",
		Endpoint:      httpServer.URL,
		ToolPrefix:    "jimeng.",
		ToolNameMap:   map[string]string{"jimeng.create": "generate_video"},
		EnabledTools:  []string{"jimeng.create", "jimeng.list_task"},
		DisabledTools: []string{"list_task"},
		Enabled:       true,
	}, httpServer.Client())
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "jimeng.create" {
		t.Fatalf("filtered logical tools = %#v, want jimeng.create", tools)
	}
	result, err := client.CallTool(context.Background(), "jimeng.create", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.StructuredContent["calledTool"] != "generate_video" {
		t.Fatalf("remote tool mapping = %#v, want generate_video", result.StructuredContent)
	}
	if _, err := client.CallTool(context.Background(), "jimeng.list_task", nil); err == nil {
		t.Fatal("disabled tool call succeeded")
	}
}

func TestClientUsesStandardStdioTransportAndPreservesProcessConfig(t *testing.T) {
	workingDir := t.TempDir()
	client := NewClient(ProviderConfig{
		ID:         "stdio",
		Transport:  "stdio",
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestSDKStdioHelperProcess"},
		Env:        map[string]string{"GO_WANT_SDK_STDIO_HELPER": "1", "MCP_TEST_ENV": "preserved"},
		WorkingDir: workingDir,
		ToolPrefix: "stdio.",
		Enabled:    true,
	}, nil)
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "stdio.inspect" {
		t.Fatalf("stdio tools = %#v", tools)
	}
	result, err := client.CallTool(context.Background(), "stdio.inspect", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.StructuredContent["env"] != "preserved" {
		t.Fatalf("stdio env = %#v", result.StructuredContent)
	}
	resolvedWorkingDir, err := filepath.EvalSymlinks(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent["workingDir"] != resolvedWorkingDir {
		t.Fatalf("stdio workingDir = %#v, want %q", result.StructuredContent, resolvedWorkingDir)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close must be idempotent: %v", err)
	}
}

func TestClientHonorsShorterCallerDeadline(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "timeout", Version: "1.0.0"}, nil)
	server.AddTool(&mcp.Tool{Name: "slow", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "late"}}}, nil
		}
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	client := NewClient(ProviderConfig{ID: "timeout", Endpoint: httpServer.URL, TimeoutSec: 5, Enabled: true}, httpServer.Client())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.CallTool(ctx, "slow", nil)
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), context.DeadlineExceeded.Error())) {
		t.Fatalf("CallTool error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("caller deadline was not respected, elapsed %v", elapsed)
	}
}

func TestToolContentPreservesUnknownCompatibilityFields(t *testing.T) {
	raw := []byte(`{"type":"text","text":"ok","futureStandardField":{"enabled":true},"_meta":{"vendor/id":"1"}}`)
	var content ToolContent
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip["futureStandardField"] == nil || content.Text != "ok" || content.Meta["vendor/id"] != "1" {
		t.Fatalf("unknown fields or readable fields were lost: content=%#v json=%#v", content, roundTrip)
	}
}

func TestDisabledProviderAndEmptyToolAreRejectedWithoutConnecting(t *testing.T) {
	client := NewClient(ProviderConfig{ID: "disabled", Endpoint: "http://127.0.0.1:1", Enabled: false}, nil)
	if _, err := client.ListTools(context.Background()); err == nil {
		t.Fatal("disabled provider ListTools succeeded")
	}
	if _, err := client.CallTool(context.Background(), "", nil); err == nil {
		t.Fatal("empty tool CallTool succeeded")
	}
}

func richToolResult() *mcp.CallToolResult {
	size := int64(42)
	annotations := &mcp.Annotations{Audience: []mcp.Role{mcp.Role("user")}, Priority: 0.75, LastModified: "2026-07-22T00:00:00Z"}
	return &mcp.CallToolResult{
		Meta: mcp.Meta{"request/id": "req-1"},
		Content: []mcp.Content{
			&mcp.TextContent{Text: "complete", Annotations: annotations, Meta: mcp.Meta{"kind": "summary"}},
			&mcp.ImageContent{Data: []byte("image-bytes"), MIMEType: "image/png", Annotations: annotations, Meta: mcp.Meta{"kind": "preview"}},
			&mcp.AudioContent{Data: []byte("audio-bytes"), MIMEType: "audio/wav", Annotations: annotations, Meta: mcp.Meta{"kind": "voice"}},
			&mcp.ResourceLink{URI: "file:///tmp/report.json", Name: "report", Title: "Report", Description: "generated report", MIMEType: "application/json", Size: &size, Annotations: annotations, Meta: mcp.Meta{"kind": "link"}, Icons: []mcp.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+"}}},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "memory://note", MIMEType: "text/plain", Text: "embedded", Meta: mcp.Meta{"resource/id": "note-1"}}, Annotations: annotations, Meta: mcp.Meta{"kind": "embedded"}},
		},
		StructuredContent: map[string]any{"ok": true, "count": float64(5)},
		IsError:           true,
	}
}

func assertRichToolResult(t *testing.T, result *ToolCallResult) {
	t.Helper()
	if result == nil || !result.IsError || result.Meta["request/id"] != "req-1" {
		t.Fatalf("result flags/meta were lost: %#v", result)
	}
	if result.StructuredContent["ok"] != true || len(result.Content) != 5 {
		t.Fatalf("structured content or content was lost: %#v", result)
	}
	if result.Content[0].Type != "text" || result.Content[0].Text != "complete" || result.Content[0].Annotations["priority"] != 0.75 {
		t.Fatalf("text content was lost: %#v", result.Content[0])
	}
	if result.Content[1].Type != "image" || result.Content[1].MIMEType != "image/png" || result.Content[1].Data != base64.StdEncoding.EncodeToString([]byte("image-bytes")) {
		t.Fatalf("image content was lost: %#v", result.Content[1])
	}
	if result.Content[2].Type != "audio" || result.Content[2].MIMEType != "audio/wav" {
		t.Fatalf("audio content was lost: %#v", result.Content[2])
	}
	if result.Content[3].Type != "resource_link" || result.Content[3].URI != "file:///tmp/report.json" || result.Content[3].Size == nil {
		t.Fatalf("resource link content was lost: %#v", result.Content[3])
	}
	if result.Content[4].Type != "resource" || result.Content[4].Resource["text"] != "embedded" {
		t.Fatalf("embedded resource content was lost: %#v", result.Content[4])
	}
}

func TestSDKStdioHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SDK_STDIO_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio-helper", Version: "1.0.0"}, nil)
	server.AddTool(&mcp.Tool{Name: "inspect", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cwd, _ := os.Getwd()
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "ok"}},
			StructuredContent: map[string]any{"env": os.Getenv("MCP_TEST_ENV"), "workingDir": cwd},
		}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestClientListsToolsFromPythonMCPServer(t *testing.T) {
	if os.Getenv("RUN_PYTHON_MCP_INTEGRATION") != "1" {
		t.Skip("set RUN_PYTHON_MCP_INTEGRATION=1 to run the Python MCP integration test")
	}
	scriptPath := filepath.Clean(filepath.Join("..", "..", "..", "mcp", "jimeng", "server.py"))
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("python mcp server not found: %v", err)
	}
	pythonCommand := pythonCommandForIntegrationTest(t)
	client := NewClient(ProviderConfig{ID: "jimeng", Transport: "stdio", Command: pythonCommand, Args: []string{scriptPath}, ToolPrefix: "jimeng.", Enabled: true}, nil)
	defer client.Close()
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == "jimeng.generate_video" {
			return
		}
	}
	t.Fatalf("tools = %#v, want jimeng.generate_video", tools)
}

func pythonCommandForIntegrationTest(t *testing.T) string {
	t.Helper()
	if command := strings.TrimSpace(os.Getenv("PYTHON")); command != "" {
		return command
	}
	for _, candidate := range []string{"python3", "/opt/homebrew/bin/python3", "/usr/local/bin/python3"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	t.Fatal("Python 3.10+ is required for the Python MCP integration test")
	return ""
}
