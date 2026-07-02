package localtool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFileImportExecutorCopiesWorkspaceFileIntoProjectArtifacts(t *testing.T) {
	root := t.TempDir()
	sourceURI := "local://cache/uploads/reference.png"
	sourcePath := filepath.Join(root, "cache", "uploads", "reference.png")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x89, 'P', 'N', 'G', '\r', '\n'}
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	if err := RegisterDefaultExecutors(reg, ExecutorConfig{DataDir: root}); err != nil {
		t.Fatal(err)
	}
	if !reg.CanExecute(CommandLocalFileImport) {
		t.Fatalf("%s should be registered by default; commands=%v", CommandLocalFileImport, reg.RegisteredCommands())
	}

	result, err := reg.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "vp_1",
		Command:   CommandLocalFileImport,
		Payload: map[string]interface{}{
			"source":       sourceURI,
			"id":           "ref_1",
			"artifactKind": "REFERENCE_IMAGE",
			"mimeType":     "image/png",
			"metadata": map[string]interface{}{
				"role": "scene_reference",
			},
		},
	})
	if err != nil {
		t.Fatalf("execute local import: %v", err)
	}

	storageRef, _ := result.Output["storageRef"].(string)
	if !strings.HasPrefix(storageRef, "local://projects/vp_1/artifacts/ref_1/") {
		t.Fatalf("unexpected storageRef: %q", storageRef)
	}
	contentHash, _ := result.Output["contentHash"].(string)
	if !strings.HasPrefix(contentHash, "sha256:") {
		t.Fatalf("expected sha256 content hash, got %q", contentHash)
	}
	sizeBytes, _ := result.Output["sizeBytes"].(int64)
	if sizeBytes != int64(len(payload)) {
		t.Fatalf("sizeBytes = %d, want %d", sizeBytes, len(payload))
	}

	importedPath, err := NewPathGuard(root).ResolveLocalURI(storageRef)
	if err != nil {
		t.Fatalf("resolve imported storageRef: %v", err)
	}
	imported, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("read imported file: %v", err)
	}
	if !bytes.Equal(imported, payload) {
		t.Fatalf("imported payload mismatch: %#v", imported)
	}

	artifacts, ok := result.Output["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) != 1 {
		t.Fatalf("expected one artifact entry, got %#v", result.Output["artifacts"])
	}
	if artifacts[0]["kind"] != "REFERENCE_IMAGE" || artifacts[0]["storageRef"] != storageRef {
		t.Fatalf("unexpected artifact metadata: %#v", artifacts[0])
	}
	metadata, _ := artifacts[0]["metadata"].(map[string]interface{})
	if metadata["role"] != "scene_reference" || metadata["sourceRef"] != sourceURI {
		t.Fatalf("metadata should preserve source details: %#v", metadata)
	}
}

func TestLocalFileImportExecutorRejectsMissingProjectID(t *testing.T) {
	executor := NewLocalFileImportExecutor(t.TempDir())
	_, err := executor.Execute(context.Background(), Job{
		Command: CommandLocalFileImport,
		Payload: map[string]interface{}{
			"source": "local://cache/uploads/reference.png",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid project id") {
		t.Fatalf("expected invalid project id error, got %v", err)
	}
}
