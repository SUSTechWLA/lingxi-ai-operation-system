package localtool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHyperFramesRenderExecutorInvalidProjectDir(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:9999", 0)

	// Missing projectDir in payload, no ProjectID in job
	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandHyperFramesRender,
		Payload: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing project ID")
	}
}

func TestHyperFramesRenderExecutorTraversalProjectDir(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:9999", 0)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/../escape",
		},
	})
	if err == nil {
		t.Fatal("expected error for traversal projectDir")
	}
}

func TestHyperFramesRenderExecutorMissingEntry(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:9999", 0)

	// Create project directory but no index.html
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing entry file")
	}
}

func TestHyperFramesRenderExecutorInvalidOutputPath(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:9999", 0)

	// Create project with entry file
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
			"outputPath": "local://projects/../escape/final.mp4",
		},
	})
	if err == nil {
		t.Fatal("expected error for traversal outputPath")
	}
}

func TestHyperFramesRenderExecutorServiceUnreachable(t *testing.T) {
	root := t.TempDir()
	// Use a port that nothing is listening on
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:19999", 0)

	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executor.Execute(context.Background(), Job{
		ID:         "job-1",
		ProjectID:  "project_001",
		Command:    CommandHyperFramesRender,
		TimeoutSec: 1, // short timeout for test
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
			"fps":        float64(30),
		},
	})
	if err == nil {
		t.Fatal("expected error when render service is unreachable")
	}
}

func TestHyperFramesRenderExecutorDefaultTimeout(t *testing.T) {
	root := t.TempDir()
	// Zero timeout should default to 30 minutes
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:8787", 0)
	if executor.timeout.Seconds() < 60 {
		t.Fatalf("expected default timeout >= 60s, got %v", executor.timeout)
	}
}

func TestHyperFramesRenderExecutorReturnsClientFetchableLocalArtifactRef(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req hyperFramesRenderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode render request: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
			t.Fatalf("mkdir output: %v", err)
		}
		if err := os.WriteFile(req.OutputPath, []byte("fake mp4 bytes"), 0o644); err != nil {
			t.Fatalf("write output: %v", err)
		}
		_ = json.NewEncoder(w).Encode(hyperFramesRenderResponse{
			OK:         true,
			JobID:      "render_test",
			OutputPath: req.OutputPath,
			DurationMs: 12,
		})
	}))
	defer server.Close()

	executor := NewHyperFramesRenderExecutor(root, server.URL, 0)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	outputRef, _ := result.Output["outputRef"].(string)
	if !strings.HasPrefix(outputRef, "local://projects/project_001/artifacts/final-video/") {
		t.Fatalf("outputRef should be fetchable by client artifact endpoint, got %q", outputRef)
	}
	if !strings.HasSuffix(outputRef, "/final.mp4") {
		t.Fatalf("outputRef should preserve final filename, got %q", outputRef)
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts", "project_001", "final-video", "content")); err != nil {
		t.Fatalf("final video should be mirrored into local artifact content path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts", "project_001", "final-video", "metadata.json")); err != nil {
		t.Fatalf("final video should have local artifact metadata: %v", err)
	}

	artifacts, ok := result.Output["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) != 1 {
		t.Fatalf("expected one artifact manifest, got %#v", result.Output["artifacts"])
	}
	if artifacts[0]["storageRef"] != outputRef {
		t.Fatalf("artifact storageRef should match outputRef: %#v", artifacts[0])
	}
	metadataBytes, err := os.ReadFile(filepath.Join(root, "artifacts", "project_001", "final-video", "metadata.json"))
	if err != nil {
		t.Fatalf("read final metadata: %v", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("decode final metadata: %v", err)
	}
	if metadata["sourceType"] != "hyperframes" || metadata["isFallback"] != false {
		t.Fatalf("final metadata should expose hyperframes provenance, got %#v", metadata)
	}
}

func TestHyperFramesRenderExecutorFastStoryboardRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not available")
	}
	if err := exec.Command("python3", "-c", "import PIL").Run(); err != nil {
		t.Skip("python3 Pillow is not available")
	}

	root := t.TempDir()
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(filepath.Join(projectDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := map[string]interface{}{
		"shotList": []map[string]interface{}{
			{
				"id":              "SHOT_01_TW_01",
				"shotId":          "SHOT_01_TW_01",
				"sceneSummary":    "AIGC_VIDEO | 非真人风格化：创作者工作台变成可控流水线",
				"mainAction":      "建立强钩子。",
				"durationSec":     1,
				"sequenceIndex":   0,
				"recommendedMode": "aigc_video",
			},
			{
				"id":              "SHOT_02_TW_01",
				"shotId":          "SHOT_02_TW_01",
				"sceneSummary":    "HYPERFRAMES | 审核门、本地 runner、MCP、最终渲染串成流程图",
				"mainAction":      "解释系统可追踪。",
				"durationSec":     1,
				"sequenceIndex":   1,
				"recommendedMode": "hyperframes",
			},
		},
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "assets", "data.json"), dataBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TANGYING_FAST_STORYBOARD_RENDER", "1")
	executor := NewHyperFramesRenderExecutor(root, "http://127.0.0.1:19999", 0)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
			"fps":        float64(12),
		},
	})
	if err != nil {
		t.Fatalf("execute fast storyboard render: %v", err)
	}

	outputRef, _ := result.Output["outputRef"].(string)
	if !strings.Contains(outputRef, "/final.mp4") {
		t.Fatalf("outputRef = %q, want final mp4 ref", outputRef)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "project_001", "renders", "final.mp4")); err != nil {
		t.Fatalf("final video should exist: %v", err)
	}
	if result.Output["renderJobId"] != "storyboard_fast_render" {
		t.Fatalf("renderJobId = %#v, want storyboard_fast_render", result.Output["renderJobId"])
	}
	metadataBytes, err := os.ReadFile(filepath.Join(root, "artifacts", "project_001", "final-video", "metadata.json"))
	if err != nil {
		t.Fatalf("read fallback metadata: %v", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("decode fallback metadata: %v", err)
	}
	if metadata["sourceType"] != "fallback_storyboard" || metadata["isFallback"] != true {
		t.Fatalf("fallback metadata should expose storyboard fallback provenance, got %#v", metadata)
	}
	if metadata["executionMode"] != "fallback" || metadata["productionEligible"] != false {
		t.Fatalf("fallback metadata must be production-ineligible, got %#v", metadata)
	}
}

func TestHyperFramesFallbackProvenanceIsProductionIneligible(t *testing.T) {
	provenance := hyperframesRenderProvenance(&hyperFramesRenderResponse{JobID: "storyboard_fast_render"})
	if provenance["executionMode"] != "fallback" || provenance["productionEligible"] != false {
		t.Fatalf("fallback provenance must be production-ineligible: %#v", provenance)
	}
	if provenance["fallbackReason"] != "storyboard_fast_render" {
		t.Fatalf("fallback reason missing: %#v", provenance)
	}
}

func TestHyperFramesRenderExecutorFallsBackToStoryboardWhenServiceFails(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not available")
	}
	if err := exec.Command("python3", "-c", "import PIL").Run(); err != nil {
		t.Skip("python3 Pillow is not available")
	}

	root := t.TempDir()
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(filepath.Join(projectDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := map[string]interface{}{
		"shotList": []map[string]interface{}{
			{
				"id":              "SHOT_01",
				"shotId":          "SHOT_01",
				"sceneSummary":    "影视化场景：一个创作者按下按钮，AI 制片台开始有序工作",
				"mainAction":      "用明确画面表达开源视频流水线的启动。",
				"durationSec":     1,
				"sequenceIndex":   0,
				"recommendedMode": "aigc_video",
			},
		},
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "assets", "data.json"), dataBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(hyperFramesRenderResponse{OK: false, Error: "renderer timed out"})
	}))
	defer server.Close()

	t.Setenv("TANGYING_FAST_STORYBOARD_RENDER", "")
	t.Setenv("TANGYING_STORYBOARD_RENDER_FALLBACK", "1")
	executor := NewHyperFramesRenderExecutor(root, server.URL, 0)
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
			"fps":        float64(12),
		},
	})
	if err != nil {
		t.Fatalf("execute with storyboard fallback: %v", err)
	}
	if result.Output["renderJobId"] != "storyboard_fast_render" {
		t.Fatalf("renderJobId = %#v, want storyboard_fast_render", result.Output["renderJobId"])
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "project_001", "renders", "final.mp4")); err != nil {
		t.Fatalf("fallback final video should exist: %v", err)
	}
}

func TestHyperFramesRenderExecutorFallsBackToStoryboardWhenServiceTimesOut(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not available")
	}
	if err := exec.Command("python3", "-c", "import PIL").Run(); err != nil {
		t.Skip("python3 Pillow is not available")
	}

	root := t.TempDir()
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(filepath.Join(projectDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := map[string]interface{}{
		"shotList": []map[string]interface{}{
			{
				"id":              "SHOT_01",
				"shotId":          "SHOT_01",
				"sceneSummary":    "开场：AI 制片台从混乱变有序，字幕留出安全区",
				"mainAction":      "展示系统开始自动化创作。",
				"durationSec":     1,
				"sequenceIndex":   0,
				"recommendedMode": "hyperframes",
			},
		},
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "assets", "data.json"), dataBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_ = json.NewEncoder(w).Encode(hyperFramesRenderResponse{OK: true, JobID: "late"})
	}))
	defer server.Close()

	t.Setenv("TANGYING_FAST_STORYBOARD_RENDER", "")
	t.Setenv("TANGYING_STORYBOARD_RENDER_FALLBACK", "1")
	executor := NewHyperFramesRenderExecutor(root, server.URL, 0)
	result, err := executor.Execute(context.Background(), Job{
		ID:         "job-1",
		ProjectID:  "project_001",
		Command:    CommandHyperFramesRender,
		TimeoutSec: 8,
		Payload: map[string]interface{}{
			"projectDir": "local://projects/project_001/hyperframes",
			"fps":        float64(12),
			"timeoutSec": float64(1),
		},
	})
	if err != nil {
		t.Fatalf("execute with storyboard fallback after timeout: %v", err)
	}
	if result.Output["renderJobId"] != "storyboard_fast_render" {
		t.Fatalf("renderJobId = %#v, want storyboard_fast_render", result.Output["renderJobId"])
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "project_001", "renders", "final.mp4")); err != nil {
		t.Fatalf("fallback final video should exist: %v", err)
	}
}
