package localtool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHyperFramesSnapshotExecutorRejectsTraversalProjectID(t *testing.T) {
	executor := NewHyperFramesSnapshotExecutor(t.TempDir(), "http://127.0.0.1:8787", 0)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "../escape",
		Command:   "HYPERFRAMES_SNAPSHOT",
	})
	if err == nil {
		t.Fatal("expected traversal project id to be rejected")
	}
}

func TestHyperFramesSnapshotExecutorRejectsTraversalInProjectDir(t *testing.T) {
	executor := NewHyperFramesSnapshotExecutor(t.TempDir(), "http://127.0.0.1:8787", 0)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   "HYPERFRAMES_SNAPSHOT",
		Payload: map[string]interface{}{
			"projectDir": "local://../escape",
		},
	})
	if err == nil {
		t.Fatal("expected traversal projectDir to be rejected")
	}
}

func TestHyperFramesSnapshotExecutorRequiresValidProjectID(t *testing.T) {
	executor := NewHyperFramesSnapshotExecutor(t.TempDir(), "http://127.0.0.1:8787", 0)
	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: "HYPERFRAMES_SNAPSHOT",
	})
	if err == nil {
		t.Fatal("expected missing projectID to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid project id") {
		t.Fatalf("expected 'invalid project id' error, got: %v", err)
	}
}

func TestHyperFramesSnapshotExecutorRejectsMissingEntryFile(t *testing.T) {
	root := t.TempDir()
	// Create project dir but no entry file
	projectDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	executor := NewHyperFramesSnapshotExecutor(root, "http://127.0.0.1:8787", 0)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   "HYPERFRAMES_SNAPSHOT",
	})
	if err == nil {
		t.Fatal("expected missing entry file error")
	}
	if !strings.Contains(err.Error(), "entry file not found") {
		t.Fatalf("expected 'entry file not found' error, got: %v", err)
	}
}

func TestHyperFramesSnapshotExecutorRegisteredInDefaultBootstrap(t *testing.T) {
	reg := NewRegistry()
	cfg := ExecutorConfig{
		DataDir:               t.TempDir(),
		HyperFramesServiceURL: "http://127.0.0.1:8787",
	}
	if err := RegisterDefaultExecutors(reg, cfg); err != nil {
		t.Fatal(err)
	}
	commands := reg.RegisteredCommands()
	found := false
	for _, cmd := range commands {
		if cmd == CommandHyperFramesSnapshot {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("HYPERFRAMES_SNAPSHOT not registered; commands: %v", commands)
	}
	if !reg.CanExecute(CommandHyperFramesSnapshot) {
		t.Fatal("HYPERFRAMES_SNAPSHOT not executable after registration")
	}
}
