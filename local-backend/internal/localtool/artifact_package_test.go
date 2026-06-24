package localtool

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactPackageExecutorEmptyInclude(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload:   map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for empty include list")
	}
}

func TestArtifactPackageExecutorTraversalInclude(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload: map[string]interface{}{
			"include": []interface{}{"local://projects/../escape/secret.txt"},
		},
	})
	if err == nil {
		t.Fatal("expected error for traversal include path")
	}
}

func TestArtifactPackageExecutorTraversalOutput(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	// Create a valid include file
	projDir := filepath.Join(root, "projects", "project_001", "renders")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "final.mp4"), []byte("fake video"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload: map[string]interface{}{
			"include": []interface{}{"local://projects/project_001/renders/final.mp4"},
			"output":  "local://projects/../escape/package.zip",
		},
	})
	if err == nil {
		t.Fatal("expected error for traversal output path")
	}
}

func TestArtifactPackageExecutorPackagesFiles(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	// Create test files
	rendersDir := filepath.Join(root, "projects", "project_001", "renders")
	if err := os.MkdirAll(rendersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rendersDir, "final.mp4"), []byte("fake video content"), 0o644); err != nil {
		t.Fatal(err)
	}

	hyperframesDir := filepath.Join(root, "projects", "project_001", "hyperframes")
	if err := os.MkdirAll(hyperframesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hyperframesDir, "manifest.json"), []byte(`{"entry":"index.html"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload: map[string]interface{}{
			"include": []interface{}{
				"local://projects/project_001/renders/final.mp4",
				"local://projects/project_001/hyperframes/manifest.json",
			},
			"output": "local://projects/project_001/packages/project_package.zip",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output
	outputRef, ok := result.Output["outputRef"].(string)
	if !ok {
		t.Fatalf("expected outputRef in result, got %#v", result.Output)
	}
	if outputRef != "local://projects/project_001/packages/project_package.zip" {
		t.Fatalf("unexpected outputRef: %q", outputRef)
	}

	success, ok := result.Output["success"].(bool)
	if !ok || !success {
		t.Fatalf("expected success=true, got %v", result.Output["success"])
	}

	// Verify zip file exists and is valid
	zipPath := filepath.Join(root, "projects", "project_001", "packages", "project_package.zip")
	info, err := os.Stat(zipPath)
	if err != nil {
		t.Fatalf("zip file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("zip file is empty")
	}

	// Verify zip contents
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	if len(r.File) != 2 {
		t.Fatalf("expected 2 files in zip, got %d: %v", len(r.File), r.File)
	}
}

func TestArtifactPackageExecutorPackagesDirectory(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	// Create a directory with files
	hyperframesDir := filepath.Join(root, "projects", "project_001", "hyperframes", "assets")
	if err := os.MkdirAll(hyperframesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hyperframesDir, "data.json"), []byte(`{"key":"value"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hyperframesDir, "style.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Package the directory
	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload: map[string]interface{}{
			"include": []interface{}{
				"local://projects/project_001/hyperframes/assets",
			},
			"output": "local://projects/project_001/packages/assets_package.zip",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	files, ok := result.Output["files"].([]string)
	if !ok || len(files) != 2 {
		t.Fatalf("expected 2 packaged files, got %#v", result.Output["files"])
	}

	// Verify zip
	zipPath := filepath.Join(root, "projects", "project_001", "packages", "assets_package.zip")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	if len(r.File) != 2 {
		t.Fatalf("expected 2 files in zip, got %d", len(r.File))
	}
}

func TestArtifactPackageExecutorMissingFile(t *testing.T) {
	root := t.TempDir()
	executor := NewArtifactPackageExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandArtifactPackage,
		Payload: map[string]interface{}{
			"include": []interface{}{"local://projects/project_001/nonexistent/file.txt"},
			"output":  "local://projects/project_001/packages/test.zip",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing include file")
	}
}
