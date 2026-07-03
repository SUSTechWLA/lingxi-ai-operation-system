package localmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientListsToolsViaJSONRPC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "tools/list" {
			t.Fatalf("method = %q, want tools/list", req.Method)
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "jimeng.generate_video",
						"description": "generate video",
						"inputSchema": map[string]interface{}{"type": "object"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(ProviderConfig{ID: "jimeng", Endpoint: server.URL, Enabled: true}, server.Client())
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0].Name != "jimeng.generate_video" {
		t.Fatalf("tool name = %q, want jimeng.generate_video", tools[0].Name)
	}
}

func TestClientCallsToolViaJSONRPC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "tools/call" {
			t.Fatalf("method = %q, want tools/call", req.Method)
		}
		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Fatalf("params type = %T, want map", req.Params)
		}
		if params["name"] != "jimeng.generate_image" {
			t.Fatalf("tool name = %v, want jimeng.generate_image", params["name"])
		}
		arguments, ok := params["arguments"].(map[string]interface{})
		if !ok {
			t.Fatalf("arguments type = %T, want map", params["arguments"])
		}
		if arguments["prompt"] != "cinematic cat" {
			t.Fatalf("prompt = %v, want cinematic cat", arguments["prompt"])
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "submitted"},
				},
				"structuredContent": map[string]interface{}{
					"submit_id":  "submit-123",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(ProviderConfig{ID: "jimeng", Endpoint: server.URL, Enabled: true}, server.Client())
	result, err := client.CallTool(context.Background(), "jimeng.generate_image", map[string]interface{}{"prompt": "cinematic cat"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if result.StructuredContent["submit_id"] != "submit-123" {
		t.Fatalf("submit_id = %v, want submit-123", result.StructuredContent["submit_id"])
	}
	if len(result.Content) != 1 || result.Content[0].Text != "submitted" {
		t.Fatalf("content = %#v, want submitted text", result.Content)
	}
}

func TestClientReturnsRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      "1",
			Error:   &rpcError{Code: -32000, Message: "tool failed"},
		})
	}))
	defer server.Close()

	client := NewClient(ProviderConfig{ID: "jimeng", Endpoint: server.URL, Enabled: true}, server.Client())
	if _, err := client.CallTool(context.Background(), "jimeng.generate_image", nil); err == nil {
		t.Fatal("CallTool error = nil, want rpc error")
	}
}

func TestClientListsAndCallsToolsViaStdioTransport(t *testing.T) {
	client := NewClient(ProviderConfig{
		ID:        "fake",
		Transport: "stdio",
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestStdioMCPHelperProcess"},
		Env:       map[string]string{"GO_WANT_STDIO_MCP_HELPER": "1"},
		Enabled:   true,
	}, nil)
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "fake.echo" {
		t.Fatalf("tools = %#v, want fake.echo", tools)
	}

	result, err := client.CallTool(context.Background(), "fake.echo", map[string]interface{}{"text": "hello"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if result.StructuredContent["echo"] != "hello" {
		t.Fatalf("structured echo = %#v, want hello", result.StructuredContent)
	}
}

func TestClientMapsLogicalToolNameWithProviderPrefix(t *testing.T) {
	client := NewClient(ProviderConfig{
		ID:         "jimeng",
		Transport:  "stdio",
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestStdioMCPHelperProcess"},
		Env:        map[string]string{"GO_WANT_STDIO_MCP_HELPER": "1"},
		ToolPrefix: "jimeng.",
		Enabled:    true,
	}, nil)
	defer client.Close()

	result, err := client.CallTool(context.Background(), "jimeng.generate_video", map[string]interface{}{"text": "hello"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if result.StructuredContent["calledTool"] != "generate_video" {
		t.Fatalf("calledTool = %#v, want generate_video", result.StructuredContent["calledTool"])
	}
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
	client := NewClient(ProviderConfig{
		ID:         "jimeng",
		Transport:  "stdio",
		Command:    pythonCommand,
		Args:       []string{scriptPath},
		ToolPrefix: "jimeng.",
		Enabled:    true,
	}, nil)
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
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
	for _, candidate := range pythonCandidatesForIntegrationTest() {
		command := compatiblePythonExecutableForIntegrationTest(candidate)
		if command != "" {
			return command
		}
	}
	t.Fatal("Python 3.10+ is required for the Python MCP integration test")
	return ""
}

func pythonCandidatesForIntegrationTest() []string {
	candidates := []string{}
	if path, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if matches, err := filepath.Glob(filepath.Join(home, ".pyenv", "versions", "*", "bin", "python3")); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	candidates = append(candidates, "/opt/homebrew/bin/python3", "/usr/local/bin/python3", "python3")
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func compatiblePythonExecutableForIntegrationTest(candidate string) string {
	out, err := exec.Command(candidate, "-c", "import sys; sys.exit(1) if sys.version_info < (3, 10) else print(sys.executable)").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func TestStdioMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_STDIO_MCP_HELPER") != "1" {
		return
	}
	defer os.Exit(0)

	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var req rpcRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if req.ID == "" {
			continue
		}
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "fake", "version": "test"},
			}
		case "tools/list":
			resp.Result = map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "fake.echo",
						"description": "Echo input",
						"inputSchema": map[string]interface{}{"type": "object"},
					},
				},
			}
		case "tools/call":
			params, _ := req.Params.(map[string]interface{})
			args, _ := params["arguments"].(map[string]interface{})
			resp.Result = map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{"type": "text", "text": "ok"},
				},
				"structuredContent": map[string]interface{}{
					"echo":       args["text"],
					"calledTool": params["name"],
				},
			}
		default:
			resp.Error = &rpcError{Code: -32601, Message: "method not found"}
		}
		if err := encoder.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
}
