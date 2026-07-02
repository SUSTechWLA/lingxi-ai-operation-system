package localtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

func TestMCPToolCallExecutorCallsConfiguredProvider(t *testing.T) {
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["method"] != "tools/call" {
			t.Fatalf("method = %v, want tools/call", req["method"])
		}
		params := req["params"].(map[string]interface{})
		if params["name"] != "jimeng.generate_video" {
			t.Fatalf("tool = %v, want jimeng.generate_video", params["name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "ok"},
				},
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-1",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"toolName":   "jimeng.generate_video",
			"arguments":  map[string]interface{}{"prompt": "wide shot"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	structured := result.Output["structuredContent"].(map[string]interface{})
	if structured["submit_id"] != "vid-1" {
		t.Fatalf("submit_id = %v, want vid-1", structured["submit_id"])
	}
}

func TestMCPToolCallExecutorRejectsUnknownProvider(t *testing.T) {
	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "other", Endpoint: "http://127.0.0.1:1", Enabled: true}}, nil
	})

	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"toolName":   "jimeng.generate_video",
		},
	})
	if err == nil {
		t.Fatal("Execute error = nil, want unknown provider error")
	}
}

func TestMCPToolCallExecutorGeneratesExternalRequestBatch(t *testing.T) {
	callCount := 0
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		if params["name"] != "jimeng.generate_video" {
			t.Fatalf("tool = %v, want jimeng.generate_video", params["name"])
		}
		args := params["arguments"].(map[string]interface{})
		if args["prompt"] != "wide shot" {
			t.Fatalf("prompt = %#v, want wide shot", args["prompt"])
		}
		if args["duration"].(float64) != 5 {
			t.Fatalf("duration = %#v, want 5", args["duration"])
		}
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-1",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId": "extgen_video_SHOT_01",
					"shotId":    "SHOT_01",
					"kind":      "video",
					"prompt":    "wide shot",
					"target": map[string]interface{}{
						"durationSec": 5,
						"aspectRatio": "16:9",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d, want 1", callCount)
	}
	packages := result.Output["shotAssetPackages"].([]interface{})
	pkg := packages[0].(map[string]interface{})
	aigcVideo := pkg["aigcVideo"].(map[string]interface{})
	if aigcVideo["submitId"] != "vid-1" {
		t.Fatalf("submitId = %#v, want vid-1", aigcVideo["submitId"])
	}
	if result.Output["generationResults"].([]interface{})[0].(map[string]interface{})["requestId"] != "extgen_video_SHOT_01" {
		t.Fatalf("generationResults should preserve request id: %#v", result.Output["generationResults"])
	}
}
