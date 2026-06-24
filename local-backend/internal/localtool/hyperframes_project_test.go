package localtool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHyperFramesProjectExecutorWritesOnlyInsideProjectWorkspace(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)

	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   "HYPERFRAMES_PROJECT_GENERATE",
		Payload: map[string]interface{}{
			"topic":  "端午节的来历",
			"script": "端午节源于纪念屈原。",
			"shotList": []interface{}{
				map[string]interface{}{"id": "s1", "description": "龙舟"},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	projectDir, ok := result.Output["projectDir"].(string)
	if !ok || projectDir != "local://projects/project_001/hyperframes" {
		t.Fatalf("unexpected projectDir: %#v", result.Output)
	}
	for _, rel := range []string{"index.html", "assets/data.json", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(root, "projects", "project_001", "hyperframes", rel)); err != nil {
			t.Fatalf("expected %s to be written: %v", rel, err)
		}
	}
}

func TestHyperFramesProjectExecutorRejectsTraversalProjectID(t *testing.T) {
	executor := NewHyperFramesProjectExecutor(t.TempDir())
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "../escape",
		Command:   "HYPERFRAMES_PROJECT_GENERATE",
	})
	if err == nil {
		t.Fatal("expected traversal project id to be rejected")
	}
}
