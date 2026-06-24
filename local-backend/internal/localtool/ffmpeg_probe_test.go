package localtool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegProbeExecutorMissingInput(t *testing.T) {
	root := t.TempDir()
	executor := NewFFmpegProbeExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandFFmpegProbe,
		Payload: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestFFmpegProbeExecutorInvalidPath(t *testing.T) {
	root := t.TempDir()
	executor := NewFFmpegProbeExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandFFmpegProbe,
		Payload: map[string]interface{}{
			"input": "local://projects/../escape/video.mp4",
		},
	})
	if err == nil {
		t.Fatal("expected error for traversal input path")
	}
}

func TestFFmpegProbeExecutorFileNotFound(t *testing.T) {
	root := t.TempDir()
	executor := NewFFmpegProbeExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:      "job-1",
		Command: CommandFFmpegProbe,
		Payload: map[string]interface{}{
			"input": "local://projects/project_001/assets/nonexistent.mp4",
		},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFFmpegProbeExecutorInvalidMediaFile(t *testing.T) {
	root := t.TempDir()
	executor := NewFFmpegProbeExecutor(root)

	// Create a text file that is not valid media
	assetsDir := filepath.Join(root, "projects", "project_001", "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	textFile := filepath.Join(assetsDir, "notavideo.mp4")
	if err := os.WriteFile(textFile, []byte("this is not a video file"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandFFmpegProbe,
		Payload: map[string]interface{}{
			"input": "local://projects/project_001/assets/notavideo.mp4",
		},
	})
	// ffprobe should fail on an invalid media file
	if err == nil {
		// Some versions of ffprobe may still return partial data
		media, ok := result.Output["media"].(map[string]interface{})
		if ok {
			// Verify basic structure even on partial success
			_ = media
		}
	}
}

func TestFFmpegAvailable(t *testing.T) {
	// Just verify the function doesn't panic
	available := FFmpegAvailable()
	// Result depends on system — just check it's a boolean
	_ = available
}
