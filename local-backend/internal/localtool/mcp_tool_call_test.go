package localtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	promptText := validMCPVideoPrompt("开场流程被点亮")
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
		if args["prompt"] != promptText {
			t.Fatalf("prompt = %#v, want clear prompt", args["prompt"])
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
					"requestId":         "extgen_video_SHOT_01",
					"shotId":            "SHOT_01",
					"kind":              "video",
					"prompt":            promptText,
					"sourceArtifactIds": []interface{}{"script-1", "reference-1"},
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
	provenance := result.Output["assetProvenance"].([]interface{})
	entry := provenance[0].(map[string]interface{})
	if entry["schemaVersion"] != float64(1) && entry["schemaVersion"] != 1 {
		t.Fatalf("assetProvenance schemaVersion = %#v, want 1", entry["schemaVersion"])
	}
	for key, want := range map[string]interface{}{
		"sourceType":    "aigc_video",
		"providerName":  "runway",
		"providerJobId": "vid-1",
		"isFallback":    false,
	} {
		if entry[key] != want {
			t.Fatalf("assetProvenance[%s] = %#v, want %#v in %#v", key, entry[key], want, entry)
		}
	}
	if entry["generatedAt"] == "" {
		t.Fatalf("assetProvenance generatedAt should be set: %#v", entry)
	}
	if hash, ok := entry["inputPromptHash"].(string); !ok || !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("assetProvenance inputPromptHash = %#v, want sha256 hash", entry["inputPromptHash"])
	}
	ids, ok := entry["sourceArtifactIds"].([]interface{})
	if !ok || len(ids) != 2 || ids[0] != "script-1" || ids[1] != "reference-1" {
		t.Fatalf("assetProvenance sourceArtifactIds = %#v", entry["sourceArtifactIds"])
	}
}

func TestMCPToolCallExecutorDefersRemainingRequestsWhenGenerationIsPending(t *testing.T) {
	callCount := 0
	promptText := validMCPVideoPrompt("第一个片段开始排队")
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
		if args["prompt"] != promptText {
			t.Fatalf("prompt = %#v, want promptText fallback", args["prompt"])
		}
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-pending",
					"gen_status": "querying",
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
					"requestId":  "extgen_video_SHOT_01",
					"shotId":     "SHOT_01",
					"kind":       "video",
					"prompt":     map[string]interface{}{"redacted": true, "reason": "USER_ASSET_REDACTED"},
					"promptText": promptText,
					"target": map[string]interface{}{
						"durationSec": 5,
						"aspectRatio": "16:9",
					},
				},
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_02",
					"shotId":     "SHOT_02",
					"kind":       "video",
					"promptText": validMCPVideoPrompt("第二个片段继续推进"),
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
		t.Fatalf("callCount = %d, want only first request submitted", callCount)
	}
	results := result.Output["generationResults"].([]interface{})
	if got := results[0].(map[string]interface{})["status"]; got != "pending" {
		t.Fatalf("first status = %#v, want pending", got)
	}
	if got := results[1].(map[string]interface{})["status"]; got != "deferred" {
		t.Fatalf("second status = %#v, want deferred", got)
	}
	remaining := result.Output["externalGenerationRequests"].([]interface{})
	if got := remaining[0].(map[string]interface{})["submitId"]; got != "vid-pending" {
		t.Fatalf("pending submitId = %#v, want vid-pending", got)
	}
	if got := remaining[1].(map[string]interface{})["status"]; got != "deferred" {
		t.Fatalf("deferred status = %#v, want deferred", got)
	}
}

func TestMCPToolCallExecutorDefersBatchWhenGenerateCallTimesOut(t *testing.T) {
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-late",
					"gen_status": "success",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	started := time.Now()
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId":           "jimeng",
			"mcpTool":              "jimeng.generate_video",
			"mcpToolCallTimeoutMs": 50,
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_01",
					"shotId":     "SHOT_01",
					"kind":       "video",
					"promptText": validMCPVideoPrompt("正能量搞笑片段开始运转"),
					"target": map[string]interface{}{
						"durationSec": 5,
						"aspectRatio": "16:9",
					},
				},
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_02",
					"shotId":     "SHOT_02",
					"kind":       "video",
					"promptText": validMCPVideoPrompt("第二段画面继续运转"),
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
	if time.Since(started) > time.Second {
		t.Fatalf("timed-out MCP call should return quickly")
	}
	results := result.Output["generationResults"].([]interface{})
	first := results[0].(map[string]interface{})
	if first["status"] != "deferred" || first["reason"] != "tool_call_timeout" {
		t.Fatalf("first result should be deferred by timeout: %#v", first)
	}
	second := results[1].(map[string]interface{})
	if second["status"] != "deferred" || second["reason"] != "previous_generation_timeout" {
		t.Fatalf("second result should be deferred after timeout: %#v", second)
	}
	remaining := result.Output["externalGenerationRequests"].([]interface{})
	if len(remaining) != 2 {
		t.Fatalf("remaining requests = %d, want 2", len(remaining))
	}
}

func TestMCPToolCallExecutorDownloadsGeneratedVideoIntoShotFusionPlan(t *testing.T) {
	dataDir := t.TempDir()
	sourceDir := t.TempDir()
	sourceVideo := filepath.Join(sourceDir, "dreamina-result.mp4")
	if err := os.WriteFile(sourceVideo, []byte("fake mp4"), 0o644); err != nil {
		t.Fatalf("write source video: %v", err)
	}

	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		switch params["name"] {
		case "jimeng.generate_video":
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
		case "jimeng.query_result":
			args := params["arguments"].(map[string]interface{})
			if args["submit_id"] != "vid-1" {
				t.Fatalf("query submit_id = %#v, want vid-1", args["submit_id"])
			}
			if strings.TrimSpace(args["download_dir"].(string)) == "" {
				t.Fatalf("query_result should receive download_dir: %#v", args)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"submit_id":  "vid-1",
						"gen_status": "success",
						"result_json": map[string]interface{}{
							"videos": []map[string]interface{}{
								{"path": sourceVideo, "width": 1920, "height": 1080, "duration": 5.0},
							},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected tool call: %#v", params["name"])
		}
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutorWithDataDir(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	}, dataDir)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId": "extgen_video_SHOT_01",
					"shotId":    "SHOT_01",
					"kind":      "video",
					"prompt":    validMCPVideoPrompt("即梦结果进入素材包"),
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
	packages := result.Output["shotAssetPackages"].([]interface{})
	pkg := packages[0].(map[string]interface{})
	generationPlan := pkg["generationPlan"].(map[string]interface{})
	fusionPlan := generationPlan["fusionPlan"].(map[string]interface{})
	baseLayer := fusionPlan["baseLayer"].(map[string]interface{})
	storageRef := baseLayer["storageRef"].(string)
	if !strings.HasPrefix(storageRef, "local://projects/project_001/artifacts/extgen_video_SHOT_01/sha256_") {
		t.Fatalf("unexpected storageRef: %q", storageRef)
	}
	if baseLayer["kind"] != "video" || fusionPlan["assembler"] != "hyperframes" {
		t.Fatalf("unexpected fusion plan: %#v", fusionPlan)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "artifacts", "project_001", "extgen_video_SHOT_01", "content")); err != nil {
		t.Fatalf("expected imported content file: %v", err)
	}
	got := result.Output["generationResults"].([]interface{})[0].(map[string]interface{})
	if got["storageRef"] != storageRef {
		t.Fatalf("generation result should expose storageRef, got %#v want %q", got, storageRef)
	}
}

func TestMCPToolCallExecutorDefersAfterDefaultReadyGenerationBudget(t *testing.T) {
	dataDir := t.TempDir()
	sourceDir := t.TempDir()
	sourceVideo := filepath.Join(sourceDir, "dreamina-ready.mp4")
	if err := os.WriteFile(sourceVideo, []byte("fake mp4"), 0o644); err != nil {
		t.Fatalf("write source video: %v", err)
	}

	callCount := 0
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		callCount++
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-ready",
					"gen_status": "success",
					"result_json": map[string]interface{}{
						"videos": []map[string]interface{}{
							{"path": sourceVideo, "width": 1920, "height": 1080, "duration": 5.0},
						},
					},
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutorWithDataDir(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	}, dataDir)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{"requestId": "extgen_video_SHOT_01", "shotId": "SHOT_01", "promptText": validMCPVideoPrompt("第一个 ready 视频"), "target": map[string]interface{}{"durationSec": 5}},
				map[string]interface{}{"requestId": "extgen_video_SHOT_02", "shotId": "SHOT_02", "promptText": validMCPVideoPrompt("第二个 ready 视频"), "target": map[string]interface{}{"durationSec": 5}},
				map[string]interface{}{"requestId": "extgen_video_SHOT_03", "shotId": "SHOT_03", "promptText": validMCPVideoPrompt("第三个预算外视频"), "target": map[string]interface{}{"durationSec": 5}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d, want 2", callCount)
	}
	packages := result.Output["shotAssetPackages"].([]interface{})
	if len(packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(packages))
	}
	results := result.Output["generationResults"].([]interface{})
	third := results[2].(map[string]interface{})
	if third["status"] != "deferred" || third["reason"] != "generation_budget_reached" {
		t.Fatalf("third result should be deferred by generation budget: %#v", third)
	}
	remaining := result.Output["externalGenerationRequests"].([]interface{})
	if len(remaining) != 1 || remaining[0].(map[string]interface{})["reason"] != "generation_budget_reached" {
		t.Fatalf("remaining should contain budget-deferred request: %#v", remaining)
	}
}

func TestMCPToolCallExecutorDefersWhenBatchTimeoutIsReached(t *testing.T) {
	dataDir := t.TempDir()
	sourceDir := t.TempDir()
	sourceVideo := filepath.Join(sourceDir, "dreamina-ready.mp4")
	if err := os.WriteFile(sourceVideo, []byte("fake mp4"), 0o644); err != nil {
		t.Fatalf("write source video: %v", err)
	}

	callCount := 0
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		callCount++
		if callCount == 2 {
			time.Sleep(250 * time.Millisecond)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-ready",
					"gen_status": "success",
					"result_json": map[string]interface{}{
						"videos": []map[string]interface{}{
							{"path": sourceVideo, "width": 1920, "height": 1080, "duration": 5.0},
						},
					},
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutorWithDataDir(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	}, dataDir)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId":           "jimeng",
			"mcpTool":              "jimeng.generate_video",
			"mcpToolCallTimeoutMs": 500,
			"mcpBatchTimeoutMs":    100,
			"maxReadyGenerations":  3,
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{"requestId": "extgen_video_SHOT_01", "shotId": "SHOT_01", "promptText": validMCPVideoPrompt("第一个批次视频"), "target": map[string]interface{}{"durationSec": 5}},
				map[string]interface{}{"requestId": "extgen_video_SHOT_02", "shotId": "SHOT_02", "promptText": validMCPVideoPrompt("第二个批次视频"), "target": map[string]interface{}{"durationSec": 5}},
				map[string]interface{}{"requestId": "extgen_video_SHOT_03", "shotId": "SHOT_03", "promptText": validMCPVideoPrompt("第三个批次视频"), "target": map[string]interface{}{"durationSec": 5}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d, want 2", callCount)
	}
	results := result.Output["generationResults"].([]interface{})
	second := results[1].(map[string]interface{})
	if second["status"] != "deferred" || second["reason"] != "tool_call_timeout" {
		t.Fatalf("second result should be deferred by batch-bounded timeout: %#v", second)
	}
	third := results[2].(map[string]interface{})
	if third["status"] != "deferred" || third["reason"] != "previous_generation_timeout" {
		t.Fatalf("third result should be deferred after previous timeout: %#v", third)
	}
}

func TestMCPToolCallExecutorMarksFailedQueryResultWithoutWaiting(t *testing.T) {
	dataDir := t.TempDir()
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		switch params["name"] {
		case "jimeng.generate_video":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"submit_id":  "vid-failed",
						"gen_status": "querying",
					},
				},
			})
		case "jimeng.query_result":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"isError": true,
					"content": []map[string]interface{}{
						{"type": "text", "text": "generation failed: post-TNS check did not pass"},
					},
				},
			})
		default:
			t.Fatalf("unexpected tool call: %#v", params["name"])
		}
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutorWithDataDir(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	}, dataDir)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId": "extgen_video_SHOT_01",
					"shotId":    "SHOT_01",
					"kind":      "video",
					"prompt":    validMCPVideoPrompt("查询失败片段"),
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
	got := result.Output["generationResults"].([]interface{})[0].(map[string]interface{})
	if got["status"] != "failed" {
		t.Fatalf("status = %#v, want failed", got["status"])
	}
	if !strings.Contains(got["error"].(string), "post-TNS") {
		t.Fatalf("error should preserve query_result failure, got %#v", got["error"])
	}
}

func TestMCPToolCallExecutorDefersProviderBusyExternalBatch(t *testing.T) {
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
					"prompt":    validMCPVideoPrompt("provider busy 片段"),
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
	if got["status"] != "deferred" {
		t.Fatalf("status = %#v, want deferred", got["status"])
	}
	if !strings.Contains(got["error"].(string), "ExceedConcurrencyLimit") {
		t.Fatalf("error should preserve content text, got %#v", got["error"])
	}
	remaining := result.Output["externalGenerationRequests"].([]interface{})
	deferred := remaining[0].(map[string]interface{})
	if deferred["status"] != "deferred" || deferred["reason"] != "provider_busy" {
		t.Fatalf("remaining request should be deferred as provider_busy, got %#v", deferred)
	}
	if !strings.Contains(deferred["error"].(string), "ExceedConcurrencyLimit") {
		t.Fatalf("remaining request should preserve error text, got %#v", deferred)
	}
	mcpResult := deferred["mcpResult"].(map[string]interface{})
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

func TestMCPToolCallExecutorRoutesImageExternalRequestToGenerateImage(t *testing.T) {
	var toolName string
	var toolArgs map[string]interface{}
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		toolName, _ = params["name"].(string)
		toolArgs, _ = params["arguments"].(map[string]interface{})
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "img-1",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-image",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId": "extgen_ref_char_main",
					"shotId":    "GLOBAL_REFERENCE",
					"kind":      "image",
					"prompt":    "multi view character sheet",
					"target": map[string]interface{}{
						"aspectRatio": "16:9",
						"resolution":  "1920x1080",
						"generateNum": 1,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if toolName != "jimeng.generate_image" {
		t.Fatalf("toolName = %q, want jimeng.generate_image", toolName)
	}
	if toolArgs["prompt"] != "multi view character sheet" {
		t.Fatalf("prompt = %#v, want prompt text", toolArgs["prompt"])
	}
	if toolArgs["ratio"] != "16:9" {
		t.Fatalf("ratio = %#v, want 16:9", toolArgs["ratio"])
	}
	if toolArgs["video_resolution"] != nil {
		t.Fatalf("image request should not send video_resolution: %#v", toolArgs)
	}
	if toolArgs["resolution_type"] != "2k" || mcpIntFromInterface(toolArgs["generate_num"]) != 1 {
		t.Fatalf("image args should map resolution/generate_num, got %#v", toolArgs)
	}
	results := result.Output["generationResults"].([]interface{})
	got := results[0].(map[string]interface{})
	if got["toolName"] != "jimeng.generate_image" {
		t.Fatalf("generation result toolName = %#v", got["toolName"])
	}
	packages := result.Output["shotAssetPackages"].([]interface{})
	pkg := packages[0].(map[string]interface{})
	if pkg["kind"] != "image" {
		t.Fatalf("image package kind = %#v, want image", pkg["kind"])
	}
	summary := result.Output["sourceSummary"].(map[string]interface{})
	if summary["videoRequestCount"] != 0 || summary["externalVideoRequirementSatisfied"] != true {
		t.Fatalf("image-only batch should not fail video requirement: %#v", summary)
	}
}

func TestMCPToolCallExecutorReportsMissingReadyVideoAssets(t *testing.T) {
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
					{"type": "text", "text": "dreamina generation failed: CreditPreDeductNotEnough"},
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-missing-ready-video",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId":               "jimeng",
			"mcpTool":                  "jimeng.generate_video",
			"minReadyVideoGenerations": 1,
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_01",
					"shotId":     "SHOT_01",
					"kind":       "video",
					"promptText": validMCPVideoPrompt("第一段额度失败片段"),
					"target":     map[string]interface{}{"durationSec": 5, "aspectRatio": "16:9"},
				},
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_02",
					"shotId":     "SHOT_02",
					"kind":       "video",
					"promptText": validMCPVideoPrompt("第二段额度失败片段"),
					"target":     map[string]interface{}{"durationSec": 5, "aspectRatio": "16:9"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	summary := result.Output["sourceSummary"].(map[string]interface{})
	if summary["readyVideoCount"] != 0 {
		t.Fatalf("readyVideoCount = %#v, want 0", summary["readyVideoCount"])
	}
	if summary["requiredReadyVideoCount"] != 1 || summary["externalVideoRequirementSatisfied"] != false {
		t.Fatalf("video requirement should be unmet: %#v", summary)
	}
	if summary["fallbackRequired"] != true || summary["needsAttention"] != true {
		t.Fatalf("missing ready videos should require attention and fallback: %#v", summary)
	}
	if !strings.Contains(result.Output["summary"].(string), "0/2") {
		t.Fatalf("summary should expose ready/total video count, got %q", result.Output["summary"])
	}
	provenance := result.Output["assetProvenance"].([]interface{})
	if len(provenance) != 2 {
		t.Fatalf("assetProvenance length = %d, want 2", len(provenance))
	}
	first := provenance[0].(map[string]interface{})
	if first["requestId"] != "extgen_video_SHOT_01" || first["status"] != "failed" || first["kind"] != "video" {
		t.Fatalf("unexpected provenance item: %#v", first)
	}
	if first["isFallback"] != false || first["fallbackReason"] != "" {
		t.Fatalf("failed provider output must not be disguised as fallback AIGC: %#v", first)
	}
}

func TestMCPToolCallExecutorBlocksUnclearVideoPromptBeforeProviderCall(t *testing.T) {
	callCount := 0
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		t.Fatalf("provider should not be called when prompt QA fails")
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-block-unclear-prompt",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_BAD",
					"shotId":     "SHOT_BAD",
					"kind":       "video",
					"promptText": "wide shot",
					"target":     map[string]interface{}{"durationSec": 5, "aspectRatio": "16:9"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("provider callCount = %d, want 0", callCount)
	}
	results := result.Output["generationResults"].([]interface{})
	got := results[0].(map[string]interface{})
	if got["status"] != "blocked" || got["reason"] != "prompt_qa_failed" {
		t.Fatalf("unclear prompt should be blocked before provider call: %#v", got)
	}
	qa := got["preflightQa"].(map[string]interface{})
	if qa["passed"] != false || mcpIntFromInterface(qa["score"]) >= 85 {
		t.Fatalf("preflight QA should fail with low score: %#v", qa)
	}
	summary := result.Output["sourceSummary"].(map[string]interface{})
	if summary["blockedCount"] != 1 || summary["needsAttention"] != true {
		t.Fatalf("source summary should count blocked request: %#v", summary)
	}
}

func TestMCPToolCallExecutorBlocksMissingUsableReferencesBeforeProviderCall(t *testing.T) {
	callCount := 0
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		t.Fatalf("provider should not be called when reference QA fails")
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-block-bad-reference",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":         "extgen_video_SHOT_REF",
					"shotId":            "SHOT_REF",
					"kind":              "video",
					"promptText":        validMCPVideoPrompt("创作桌流程变清楚"),
					"referenceAssetIds": []interface{}{"char_creator"},
					"references": []interface{}{
						map[string]interface{}{"id": "char_creator", "storageRef": "manual://references/SHOT_REF/01"},
					},
					"target": map[string]interface{}{"durationSec": 6, "aspectRatio": "16:9"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("provider callCount = %d, want 0", callCount)
	}
	got := result.Output["generationResults"].([]interface{})[0].(map[string]interface{})
	if got["status"] != "blocked" || got["reason"] != "reference_qa_failed" {
		t.Fatalf("bad references should be blocked before provider call: %#v", got)
	}
	qa := got["preflightQa"].(map[string]interface{})
	if qa["referencePassed"] != false {
		t.Fatalf("reference QA should fail: %#v", qa)
	}
}

func TestMCPToolCallExecutorPassesAIGCLayerPromptToJiMeng(t *testing.T) {
	callCount := 0
	aigcPrompt := validMCPVideoPrompt("AIGC 视频层提示词被正确投放给即梦")
	wrongLayerPrompt := "HyperFrames 文字层：只负责字幕、标题、UI 卡片和精确中文渲染。"
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		if params["name"] != "jimeng.generate_video" {
			t.Fatalf("tool = %v, want jimeng.generate_video", params["name"])
		}
		args := params["arguments"].(map[string]interface{})
		if args["prompt"] != aigcPrompt {
			t.Fatalf("prompt = %#v, want AIGC layer prompt", args["prompt"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-aigc-layer",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-aigc-layer-prompt",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":   "extgen_video_SHOT_AIGC",
					"shotId":      "SHOT_AIGC",
					"kind":        "video",
					"prompt":      wrongLayerPrompt,
					"promptText":  wrongLayerPrompt,
					"videoPrompt": "完整 shot 说明，不应直接投放给即梦。",
					"aigcPlan":    map[string]interface{}{"prompt": aigcPrompt},
					"target":      map[string]interface{}{"durationSec": 6, "aspectRatio": "16:9"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("provider callCount = %d, want 1", callCount)
	}
	got := result.Output["generationResults"].([]interface{})[0].(map[string]interface{})
	if got["status"] != "pending" {
		t.Fatalf("AIGC prompt should reach provider and become pending, got %#v", got)
	}
}

func TestMCPToolCallExecutorAllowsClearTimedVideoPrompt(t *testing.T) {
	callCount := 0
	promptText := validMCPVideoPrompt("创作桌流程变清楚")
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		params := req["params"].(map[string]interface{})
		args := params["arguments"].(map[string]interface{})
		if args["prompt"] != promptText {
			t.Fatalf("prompt = %#v, want clear prompt", args["prompt"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"submit_id":  "vid-clear",
					"gen_status": "querying",
				},
			},
		})
	}))
	defer mcp.Close()

	executor := NewMCPToolCallExecutor(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "jimeng", Label: "JiMeng MCP", Endpoint: mcp.URL, Enabled: true}}, nil
	})
	result, err := executor.Execute(context.Background(), Job{
		ID:      "job-clear-prompt",
		Command: CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "jimeng",
			"mcpTool":    "jimeng.generate_video",
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":  "extgen_video_SHOT_CLEAR",
					"shotId":     "SHOT_CLEAR",
					"kind":       "video",
					"promptText": promptText,
					"target":     map[string]interface{}{"durationSec": 6, "aspectRatio": "16:9"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("provider callCount = %d, want 1", callCount)
	}
	got := result.Output["generationResults"].([]interface{})[0].(map[string]interface{})
	if got["status"] != "pending" {
		t.Fatalf("clear prompt should reach provider and become pending, got %#v", got)
	}
	qa := got["preflightQa"].(map[string]interface{})
	if qa["passed"] != true || mcpIntFromInterface(qa["score"]) < 85 {
		t.Fatalf("preflight QA should pass: %#v", qa)
	}
}

func validMCPVideoPrompt(subject string) string {
	return "非真人风格化动画，16:9 横屏，画面干净明亮，情绪轻松积极。\n" +
		"这个画面表达：" + subject + "，复杂创作被清楚流程轻松送到成片。\n" +
		"0-2秒：明亮的创作桌上，一颗写着想法的小星星被脚本纸、分镜卡和抽帧 QA 放大镜围住，便利贴像小弹簧一样乱跳。\n" +
		"2-4秒：桌面打开成迷你传送带，脚本、分镜、即梦素材、抽帧 QA 四个发光小工位依次亮起，便利贴排队盖章通过。\n" +
		"4-6秒：传送带尽头弹出视频胶囊和开源星标，小机器人挥手，画面从热闹收束到清爽稳定。"
}
