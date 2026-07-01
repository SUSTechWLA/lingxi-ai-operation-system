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
}
