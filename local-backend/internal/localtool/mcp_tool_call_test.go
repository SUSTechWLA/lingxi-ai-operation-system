package localtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		if params["name"] != "runway.generate_video" {
			t.Fatalf("tool = %v, want runway.generate_video", params["name"])
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
		return []localmcp.ProviderConfig{{ID: "runway", Label: "Runway MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "runway",
			"toolName":   "runway.generate_video",
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

func TestMCPToolCallExecutorRequiresProviderID(t *testing.T) {
	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Endpoint: "http://127.0.0.1:1", Enabled: true}}, nil
	})

	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"toolName": "jimeng.generate_video",
		},
	})
	if err == nil {
		t.Fatal("Execute error = nil, want missing providerId error")
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
		if params["name"] != "runway.generate_video" {
			t.Fatalf("tool = %v, want runway.generate_video", params["name"])
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
		return []localmcp.ProviderConfig{{ID: "runway", Label: "Runway MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "runway",
			"mcpTool":    "runway.generate_video",
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
	if aigcVideo["provider"] != "runway" {
		t.Fatalf("provider = %#v, want runway", aigcVideo["provider"])
	}
	if result.Output["generationResults"].([]interface{})[0].(map[string]interface{})["requestId"] != "extgen_video_SHOT_01" {
		t.Fatalf("generationResults should preserve request id: %#v", result.Output["generationResults"])
	}
}

func TestMCPToolCallExecutorPreservesToolErrorContentInExternalBatch(t *testing.T) {
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"isError": true,
				"content": []map[string]interface{}{
					{"type": "text", "text": "RuntimeError: ExceedConcurrencyLimit"},
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
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
					"prompt":    "wide shot",
					"target": map[string]interface{}{
						"durationSec": 5,
						"aspectRatio": "16:9",
						"resolution":  "1920x1080",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	results := result.Output["generationResults"].([]interface{})
	got := results[0].(map[string]interface{})
	if got["status"] != "failed" {
		t.Fatalf("status = %#v, want failed", got["status"])
	}
	if !strings.Contains(got["error"].(string), "ExceedConcurrencyLimit") {
		t.Fatalf("error should preserve content text, got %#v", got["error"])
	}
	remaining := result.Output["externalGenerationRequests"].([]interface{})
	failed := remaining[0].(map[string]interface{})
	if !strings.Contains(failed["error"].(string), "ExceedConcurrencyLimit") {
		t.Fatalf("remaining request should preserve error text, got %#v", failed)
	}
	mcpResult := failed["mcpResult"].(map[string]interface{})
	if mcpResult["isError"] != true {
		t.Fatalf("mcpResult isError = %#v, want true", mcpResult["isError"])
	}
}

func TestMCPArgumentsFromExternalRequestNormalizesResolution(t *testing.T) {
	args := mcpArgumentsFromExternalRequest(map[string]interface{}{
		"prompt": "wide shot",
		"target": map[string]interface{}{
			"durationSec": 8,
			"aspectRatio": "16:9",
			"resolution":  "1920x1080",
		},
	})
	if args["video_resolution"] != "1080p" {
		t.Fatalf("video_resolution = %#v, want 1080p", args["video_resolution"])
	}
}
