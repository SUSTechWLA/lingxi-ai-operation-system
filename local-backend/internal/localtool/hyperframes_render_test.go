package localtool

import (
	"context"
	"os"
	"path/filepath"
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
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesRender,
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
