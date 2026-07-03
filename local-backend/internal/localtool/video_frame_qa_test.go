package localtool

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeFrameVisualQualityFlagsCrowdedTextSafetyZone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crowded.png")
	writeVisualQATestFrame(t, path, true)

	result, err := analyzeFrameVisualQuality(path, visualQAThresholds{})
	if err != nil {
		t.Fatalf("analyze frame: %v", err)
	}
	if result.Passed {
		t.Fatalf("crowded frame passed; metrics=%#v issues=%#v", result.Metrics, result.Issues)
	}
	if !hasVisualQAIssue(result.Issues, "top_left_text_zone_crowded") {
		t.Fatalf("issues = %#v, want top_left_text_zone_crowded", result.Issues)
	}
}

func TestAnalyzeFrameVisualQualityPassesSparseFrame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sparse.png")
	writeVisualQATestFrame(t, path, false)

	result, err := analyzeFrameVisualQuality(path, visualQAThresholds{})
	if err != nil {
		t.Fatalf("analyze frame: %v", err)
	}
	if !result.Passed {
		t.Fatalf("sparse frame failed; metrics=%#v issues=%#v", result.Metrics, result.Issues)
	}
}

func TestVideoFrameQAExecutorRejectsUnsafeProjectID(t *testing.T) {
	_, err := NewVideoFrameQAExecutor(t.TempDir()).Execute(t.Context(), Job{
		ProjectID: "../bad",
		Payload: map[string]interface{}{
			"input": "local://projects/test/renders/final.mp4",
		},
	})
	if err == nil {
		t.Fatal("expected unsafe project id error")
	}
}

func TestVisualQAContactSheetTileMinimizesEmptyCells(t *testing.T) {
	cases := map[int]string{
		1:  "1x1",
		3:  "3x1",
		5:  "3x2",
		8:  "4x2",
		16: "4x4",
		20: "4x4",
	}
	for frameCount, want := range cases {
		if got := visualQAContactSheetTile(frameCount); got != want {
			t.Fatalf("visualQAContactSheetTile(%d) = %q, want %q", frameCount, got, want)
		}
	}
}

func hasVisualQAIssue(issues []visualQAIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func writeVisualQATestFrame(t *testing.T, path string, crowded bool) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 18, G: 28, B: 38, A: 255})
		}
	}
	line := color.RGBA{R: 242, G: 242, B: 235, A: 255}
	rows := []int{86, 122}
	if crowded {
		rows = []int{72, 96, 120, 144, 168, 192, 216, 240, 264}
	}
	for _, y := range rows {
		for x := 80; x < 760; x++ {
			for h := 0; h < 5; h++ {
				img.SetRGBA(x, y+h, line)
			}
		}
		for x := 80; x < 760; x += 42 {
			for h := 0; h < 22; h++ {
				img.SetRGBA(x, y+h, line)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create frame: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
}
