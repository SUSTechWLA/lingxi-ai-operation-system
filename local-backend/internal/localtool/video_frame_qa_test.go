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

func TestBuildVisualQAShotSummariesAggregatesMetricsAndRepairGuidance(t *testing.T) {
	frames := []visualQAFrameResult{
		{
			ShotID:  "SHOT_01",
			TimeSec: 0,
			Passed:  true,
			Metrics: map[string]float64{
				"topLeftTextZoneEdgeDensity": 0.02,
				"lowerThirdEdgeDensity":      0.01,
				"fullFrameEdgeDensity":       0.03,
			},
		},
		{
			ShotID:  "SHOT_02",
			TimeSec: 4,
			Passed:  false,
			Metrics: map[string]float64{
				"topLeftTextZoneEdgeDensity": 0.12,
				"lowerThirdEdgeDensity":      0.02,
				"fullFrameEdgeDensity":       0.04,
			},
			Issues: []visualQAIssue{{
				Code:       "top_left_text_zone_crowded",
				Severity:   "blocking",
				Zone:       "top_left",
				Suggestion: "减少左上角叠字。",
			}},
		},
		{
			ShotID:  "SHOT_02",
			TimeSec: 8,
			Passed:  true,
			Metrics: map[string]float64{
				"topLeftTextZoneEdgeDensity": 0.04,
				"lowerThirdEdgeDensity":      0.01,
				"fullFrameEdgeDensity":       0.02,
			},
		},
	}

	summaries := buildVisualQAShotSummaries(frames)
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	if summaries[0].ShotID != "SHOT_01" || !summaries[0].Passed || summaries[0].NeedsRegeneration {
		t.Fatalf("unexpected SHOT_01 summary: %#v", summaries[0])
	}
	failing := summaries[1]
	if failing.ShotID != "SHOT_02" {
		t.Fatalf("second shot id = %q, want SHOT_02", failing.ShotID)
	}
	if failing.Passed || !failing.NeedsRegeneration {
		t.Fatalf("SHOT_02 should require regeneration: %#v", failing)
	}
	if failing.Score != 82 {
		t.Fatalf("SHOT_02 score = %d, want 82", failing.Score)
	}
	if failing.MetricSummary["maxTopLeftTextZoneEdgeDensity"] != 0.12 {
		t.Fatalf("max top-left density = %#v, want 0.12", failing.MetricSummary)
	}
	if len(failing.Recommendations) == 0 || failing.Conclusion == "" {
		t.Fatalf("expected repair guidance, got conclusion=%q recommendations=%#v", failing.Conclusion, failing.Recommendations)
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
