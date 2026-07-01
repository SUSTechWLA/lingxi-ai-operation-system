package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestExternalToolRejectsNonJSONSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>wrong service</title>"))
	}))
	defer server.Close()

	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:     "parse_bid_files",
		Type:     "http",
		Endpoint: server.URL,
	})
	external := NewExternalTool(registry)

	result := external.Execute(context.Background(), map[string]interface{}{
		"tool":      "parse_bid_files",
		"file_path": "E:/bid/test_bid.txt",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if result.Success {
		t.Fatalf("non-JSON external response should fail, got success data=%#v", result.Data)
	}
	if !strings.Contains(result.Error, "non-JSON") {
		t.Fatalf("error = %q, want non-JSON explanation", result.Error)
	}
}

func TestExternalToolRejectsJSONSuccessFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error":"file not found"}`))
	}))
	defer server.Close()

	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:     "parse_bid_files",
		Type:     "http",
		Endpoint: server.URL,
	})
	external := NewExternalTool(registry)

	result := external.Execute(context.Background(), map[string]interface{}{
		"tool":      "parse_bid_files",
		"file_path": "E:/bid/missing.docx",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if result.Success {
		t.Fatalf("success:false external response should fail, got success data=%#v", result.Data)
	}
	if !strings.Contains(result.Error, "file not found") {
		t.Fatalf("error = %q, want external error message", result.Error)
	}
}

func TestExternalToolUsesRegisteredManifestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"report_path":"E:/bid/out/analysis.md"}}`))
	}))
	defer server.Close()

	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:     "parse_bid_files",
		Type:     "http",
		Endpoint: server.URL,
		Timeout:  1,
	})
	external := NewExternalTool(registry)

	start := time.Now()
	result := external.Execute(context.Background(), map[string]interface{}{
		"tool":      "parse_bid_files",
		"file_path": "E:/bid/test_bid.txt",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if result.Success {
		t.Fatalf("manifest timeout should fail slow external call, got success data=%#v", result.Data)
	}
	if elapsed := time.Since(start); elapsed >= 1400*time.Millisecond {
		t.Fatalf("external call ignored manifest timeout; elapsed=%s error=%q", elapsed, result.Error)
	}
}
