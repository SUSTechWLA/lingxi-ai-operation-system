package localmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
