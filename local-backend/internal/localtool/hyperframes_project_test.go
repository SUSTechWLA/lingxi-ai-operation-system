package localtool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": 6,
					"aigcVideo": map[string]interface{}{
						"artifactKind":    "SHOT_VIDEO_CLIP",
						"concatMode":      "simple_cut",
						"transitionAtEnd": "结尾0.5秒淡出",
					},
				},
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
	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "assets", "data.json"))
	if err != nil {
		t.Fatalf("read data.json: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("data.json should be JSON: %v", err)
	}
	packages, ok := data["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != 1 {
		t.Fatalf("data.json should preserve shotAssetPackages, got %s", string(raw))
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

func TestHyperFramesProjectExecutorUsesShotAssetPackageMedia(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifacts", "project_001", "shot-video-01")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("fake video bytes"), 0o644); err != nil {
		t.Fatalf("write artifact content: %v", err)
	}

	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "端午节的来历",
			"script": "端午节源于纪念屈原。",
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": float64(4),
					"generationPlan": map[string]interface{}{
						"mode": "hybrid_aigc_bg_html_overlay",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"storageRef": "local://projects/project_001/artifacts/shot-video-01/hash/clip.mp4",
							},
							"overlayLayers": []interface{}{
								map[string]interface{}{
									"id":   "title",
									"kind": "html_overlay",
									"role": "title",
									"text": "精确文字",
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(raw)
	for _, expected := range []string{
		`<video`,
		`muted`,
		`playsinline`,
		`data-track-index="0"`,
		`data-duration="4.0"`,
		`data-shot-id="SHOT_01"`,
		`精确文字`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
}

func TestHyperFramesProjectExecutorRejectsMalformedShotAssetPackageMediaRef(t *testing.T) {
	cases := map[string]string{
		"extra segment": "local://projects/project_001/artifacts/shot-video-01/hash/clip.mp4/extra",
		"empty hash":    "local://projects/project_001/artifacts/shot-video-01//clip.mp4",
		"fragment":      "local://projects/project_001/artifacts/shot-video-01/hash/clip.mp4#frag",
		"missing name":  "local://projects/project_001/artifacts/shot-video-01/hash",
		"query":         "local://projects/project_001/artifacts/shot-video-01/hash/clip.mp4?x=1",
	}

	for name, storageRef := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			artifactDir := filepath.Join(root, "artifacts", "project_001", "shot-video-01")
			if err := os.MkdirAll(artifactDir, 0o755); err != nil {
				t.Fatalf("mkdir artifact: %v", err)
			}
			if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("fake video bytes"), 0o644); err != nil {
				t.Fatalf("write artifact content: %v", err)
			}

			executor := NewHyperFramesProjectExecutor(root)
			_, err := executor.Execute(context.Background(), Job{
				ID:        "job-1",
				ProjectID: "project_001",
				Command:   CommandHyperFramesProjectGenerate,
				Payload: map[string]interface{}{
					"topic":  "端午节的来历",
					"script": "端午节源于纪念屈原。",
					"shotAssetPackages": []interface{}{
						map[string]interface{}{
							"shotId":      "SHOT_01",
							"durationSec": float64(4),
							"generationPlan": map[string]interface{}{
								"mode": "hybrid_aigc_bg_html_overlay",
								"fusionPlan": map[string]interface{}{
									"baseLayer": map[string]interface{}{
										"kind":       "video",
										"storageRef": storageRef,
									},
								},
							},
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}

			raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
			if err != nil {
				t.Fatalf("read index.html: %v", err)
			}
			html := string(raw)
			if !strings.Contains(html, `Missing media for SHOT_01`) {
				t.Fatalf("index.html missing media placeholder:\n%s", html)
			}
			if strings.Contains(html, `/api/local/artifacts/shot-video-01`) {
				t.Fatalf("malformed storage ref should not resolve to artifact URL:\n%s", html)
			}
		})
	}
}

func TestHyperFramesProjectExecutorShowsMissingMediaPlaceholder(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "端午节的来历",
			"script": "端午节源于纪念屈原。",
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": float64(4),
					"generationPlan": map[string]interface{}{
						"mode": "hybrid_aigc_bg_html_overlay",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"storageRef": "local://projects/project_001/artifacts/missing-video/hash/clip.mp4",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if html := string(raw); !strings.Contains(html, `Missing media for SHOT_01`) {
		t.Fatalf("index.html missing media placeholder:\n%s", html)
	}
}

func TestHyperFramesProjectExecutorWritesHyperFramesCompositionContract(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic": "一个30秒AI运营短片",
			"compositionSpec": map[string]interface{}{
				"specVersion": "oneclick-video/v1",
				"projectType": "image_text_video",
				"durationSec": float64(6),
				"fps":         float64(30),
				"resolution": map[string]interface{}{
					"width":  float64(1920),
					"height": float64(1080),
				},
				"tracks": []interface{}{
					map[string]interface{}{
						"id":   "overlay-main",
						"type": "overlay",
						"items": []interface{}{
							map[string]interface{}{
								"id":        "card-1",
								"kind":      "title_card",
								"startSec":  float64(0),
								"endSec":    float64(3),
								"title":     "一句话启动",
								"body":      "中间产物可审核，最终视频可播放。",
								"animation": "slide_up",
							},
						},
					},
					map[string]interface{}{
						"id":   "caption-main",
						"type": "caption",
						"items": []interface{}{
							map[string]interface{}{
								"id":       "caption-1",
								"startSec": float64(0),
								"endSec":   float64(6),
								"text":     "默认中间产物全部通过",
							},
						},
					},
				},
				"style": map[string]interface{}{"theme": "editorial"},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(raw)
	for _, expected := range []string{
		`data-composition-id="tangying-main"`,
		`data-width="1920"`,
		`data-height="1080"`,
		`data-duration="6.0"`,
		`window.__timelines["tangying-main"]`,
		`gsap.timeline({ paused: true`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
	if strings.Contains(html, "requestAnimationFrame(") {
		t.Fatalf("index.html must be seekable by HyperFrames instead of frame-loop driven:\n%s", html)
	}
}
