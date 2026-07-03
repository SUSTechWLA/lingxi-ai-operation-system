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
		`assets/media/shot-video-01-hash-clip.mp4`,
		`精确文字`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "project_001", "hyperframes", "assets", "media", "shot-video-01-hash-clip.mp4")); err != nil {
		t.Fatalf("expected copied media asset: %v", err)
	}
}

func TestHyperFramesProjectExecutorKeepsFullShotTimelineWhenOnlySomeMediaReady(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifacts", "project_001", "shot-video-03")
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
			"topic":  "躺营 AIOS 正能量开源介绍",
			"script": "让创作者少一点焦虑，多一点稳定产出。",
			"shotList": []interface{}{
				map[string]interface{}{
					"shotId":        "SHOT_01",
					"durationSec":   float64(6),
					"sceneSummary":  "开头钩子",
					"mainAction":    "AI 工具排队上工",
					"narrationText": "AI 工具别再吵架了。",
				},
				map[string]interface{}{
					"shotId":        "SHOT_02",
					"durationSec":   float64(6),
					"sceneSummary":  "流程拆解",
					"mainAction":    "脚本、分镜、素材进入流水线",
					"narrationText": "系统把想法拆成可审核步骤。",
				},
				map[string]interface{}{
					"shotId":        "SHOT_03",
					"durationSec":   float64(6),
					"sceneSummary":  "Dreamina b-roll",
					"mainAction":    "AIGC 素材作为情绪画面",
					"narrationText": "即梦 MCP 生成有趣素材。",
				},
			},
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_03",
					"durationSec": float64(6),
					"generationPlan": map[string]interface{}{
						"mode": "aigc_video",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"storageRef": "local://projects/project_001/artifacts/shot-video-03/hash/clip.mp4",
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
		`data-duration="18.0"`,
		`开头钩子`,
		`流程拆解`,
		`Dreamina b-roll`,
		`03 / 03`,
		`data-shot-id="SHOT_03" data-start="12.0" data-duration="6.0"`,
		`media-safety-mask`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
}

func TestLocalAgentRawArtifactURLUsesConfiguredBase(t *testing.T) {
	t.Setenv("TANGYING_LOCAL_AGENT_BASE_URL", "http://127.0.0.1:19090/")

	got := localAgentRawArtifactURL("project_001", "shot-video-01")
	want := "http://127.0.0.1:19090/api/local/artifacts/shot-video-01?projectId=project_001&raw=1"
	if got != want {
		t.Fatalf("raw artifact url = %q, want %q", got, want)
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

func TestHyperFramesProjectExecutorBuildsShotListCompositionWithoutMediaPackages(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)

	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "躺营 AIOS 开源发布",
			"script": "真正可控的视频生产线来了。",
			"shotList": []interface{}{
				map[string]interface{}{
					"shotId":            "SHOT_01",
					"durationSec":       float64(6),
					"plannedAssetRoute": "aigc_video",
					"sceneSummary":      "开头三秒强反差",
					"mainAction":        "黑箱等待切到可控导演台",
					"narrationText":     "别再把一句话丢给 AI 然后盲等结果。",
				},
				map[string]interface{}{
					"shotId":            "SHOT_02",
					"durationSec":       float64(8),
					"startSec":          float64(0),
					"endSec":            float64(8),
					"plannedAssetRoute": "screen_recording",
					"sceneSummary":      "页面输入启动项目",
					"mainAction":        "展示登录、输入框、审核门",
					"narrationText":     "非技术人员也能把需求拆成可审核步骤。",
				},
				map[string]interface{}{
					"shotId":            "SHOT_03",
					"durationSec":       float64(7),
					"startSec":          float64(0),
					"endSec":            float64(7),
					"plannedAssetRoute": "hyperframes",
					"sceneSummary":      "开源发布 CTA",
					"mainAction":        "README、Wiki、release tag 快速扫过",
					"narrationText":     "关注这个开源项目，一起把 AI 内容生产线跑起来。",
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
		`data-duration="21.0"`,
		`开头三秒强反差`,
		`页面输入启动项目`,
		`开源发布 CTA`,
		`别再把一句话丢给 AI 然后盲等结果。`,
		`03 / 03`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
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
