package localtool

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newMCPProtocolTestServer upgrades focused executor fixtures to the standard
// MCP lifecycle while leaving each test in control of its tools/call result.
// The localmcp package tests use the official SDK server for protocol coverage.
func newMCPProtocolTestServer(t *testing.T, toolHandler http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read MCP request: %v", err)
		}
		var request map[string]interface{}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode MCP request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      request["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2025-11-25",
					"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
					"serverInfo":      map[string]interface{}{"name": "executor-test", "version": "1.0.0"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			if _, hasID := request["id"]; !hasID {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			toolHandler.ServeHTTP(w, r)
		}
	}))
}
