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

func TestHyperFramesProjectExecutorPersistsThreeLayerDesignOnEveryShot(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-three-layers",
		ProjectID: "project_three_layers",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic": "一个 Shot，三层协同",
			"shotList": []interface{}{
				map[string]interface{}{
					"shotId": "SHOT_01", "durationSec": float64(6),
					"narrationText": "树懒 IP 连续口播。", "screenText": []interface{}{"三层协同"},
				},
			},
			"shotGenerationPlans": []interface{}{
				map[string]interface{}{
					"shotId": "SHOT_01",
					"visualLayers": map[string]interface{}{
						"schemaVersion": "shot_visual_layers_v1",
						"shotId":        "SHOT_01",
						"ipAroll": map[string]interface{}{
							"layerKey": "ip_aroll", "executionPolicy": "generate", "required": true,
						},
						"hyperframes": map[string]interface{}{
							"layerKey": "hyperframes_text", "executionPolicy": "generate", "required": true,
						},
						"aigc": map[string]interface{}{
							"layerKey": "aigc_enrichment", "executionPolicy": "disabled", "required": false,
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_three_layers", "hyperframes", "assets", "data.json"))
	if err != nil {
		t.Fatalf("read data.json: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode data.json: %v", err)
	}
	if len(interfaceSlice(data["shotGenerationPlans"])) != 1 {
		t.Fatalf("shotGenerationPlans must be preserved: %s", raw)
	}
	shots := interfaceSlice(data["shotList"])
	if len(shots) != 1 {
		t.Fatalf("shotList = %#v", data["shotList"])
	}
	shot := mapFromInterface(shots[0])
	if shot["visualLayerContract"] != "shot_visual_layers_v1" {
		t.Fatalf("shot contract missing: %#v", shot)
	}
	layers := mapFromMap(shot, "visualLayers")
	for _, layerName := range []string{"ipAroll", "hyperframes", "aigc"} {
		layer := mapFromMap(layers, layerName)
		if layer == nil || strings.TrimSpace(stringFromMap(layer, "designSummary")) == "" {
			t.Fatalf("%s layer must have a readable design summary: %#v", layerName, layers)
		}
	}
	policy := mapFromMap(shot, "layerExecutionPolicy")
	if policy["ip_aroll"] != "generate" || policy["hyperframes_text"] != "generate" || policy["aigc_enrichment"] != "disabled" {
		t.Fatalf("shot layer policy = %#v", policy)
	}
	if len(interfaceSlice(shot["requiredLayers"])) != 2 {
		t.Fatalf("required layers = %#v, want IP and HyperFrames", shot["requiredLayers"])
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
	result, err := executor.Execute(context.Background(), Job{
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
	if result.Output["visualLayerContract"] != "shot_visual_layers_v1" {
		t.Fatalf("HyperFrames output must declare the canonical Shot layer contract: %#v", result.Output)
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

func TestHyperFramesProjectExecutorBuildsContinuousIPArollWithSeparateAudioAndBroll(t *testing.T) {
	root := t.TempDir()
	for _, artifactID := range []string{"ip-aroll-main", "broll-shot-01"} {
		artifactDir := filepath.Join(root, "artifacts", "project_001", artifactID)
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			t.Fatalf("mkdir artifact: %v", err)
		}
		if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("fake video bytes"), 0o644); err != nil {
			t.Fatalf("write artifact content: %v", err)
		}
	}

	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-ip-aroll",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "AI 视频创作工作流",
			"script": "先用连续 A-roll 建立信任，再用 B-roll 补充证据。",
			"aRollAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "AROLL_MAIN",
					"durationSec": float64(12),
					"sourceType":  "ip_aroll_video",
					"generationPlan": map[string]interface{}{
						"mode": "ip_aroll_video",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"role":       "a_roll",
								"storageRef": "local://projects/project_001/artifacts/ip-aroll-main/hash/aroll.mp4",
							},
						},
					},
				},
			},
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": float64(4),
					"startSec":    float64(3),
					"visualMode":  "full_screen",
					"generationPlan": map[string]interface{}{
						"mode": "aigc_video",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"role":       "b_roll",
								"storageRef": "local://projects/project_001/artifacts/broll-shot-01/hash/broll.mp4",
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
		`class="aroll-media clip"`,
		`class="aroll-audio clip"`,
		`data-track-index="0"`,
		`data-track-index="10"`,
		`data-duration="12.0"`,
		`class="shot-media clip broll-full"`,
		`data-start="3.0"`,
		`data-track-index="2"`,
		`assets/media/ip-aroll-main-hash-aroll.mp4`,
		`assets/media/broll-shot-01-hash-broll.mp4`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
	compositionStart := strings.Index(html, `data-composition-id="tangying-main"`)
	videoStart := strings.Index(html, `class="aroll-media clip"`)
	sceneStart := strings.Index(html, `<div class="scene-content">`)
	if compositionStart < 0 || videoStart < compositionStart || sceneStart < videoStart {
		t.Fatalf("A-roll media must be a direct composition child before scene overlays")
	}
	if strings.Contains(html[videoStart:sceneStart], `<div`) {
		t.Fatalf("A-roll video/audio must not be nested in a decorative media container")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	if !strings.Contains(string(manifestRaw), `"visualLayerContract": "shot_visual_layers_v1"`) {
		t.Fatalf("manifest must preserve the three-layer Shot contract: %s", manifestRaw)
	}
}

func TestHyperFramesProjectExecutorUsesCleanLayoutForContinuousIPArollWithoutBroll(t *testing.T) {
	root := t.TempDir()
	artifactDir := filepath.Join(root, "artifacts", "project_001", "ip-aroll-main")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "content"), []byte("fake video bytes"), 0o644); err != nil {
		t.Fatalf("write artifact content: %v", err)
	}

	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-ip-aroll-clean",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "正面知识口播",
			"script": "主角持续正面面对镜头。",
			"aRollAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "AROLL_MAIN",
					"durationSec": float64(20),
					"sourceType":  "ip_aroll_video",
					"generationPlan": map[string]interface{}{
						"mode": "ip_aroll_video",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{
								"kind":       "video",
								"role":       "a_roll",
								"storageRef": "local://projects/project_001/artifacts/ip-aroll-main/hash/aroll.mp4",
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

	indexPath := filepath.Join(root, "projects", "project_001", "hyperframes", "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(raw)
	if !strings.Contains(html, `class="has-aroll arroll-clean"`) {
		t.Fatalf("continuous A-roll without B-roll must use clean layout:\n%s", html)
	}
	css, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "assets", "style.css"))
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	if !strings.Contains(string(css), ".arroll-clean .scene-content") {
		t.Fatalf("clean A-roll CSS must hide generic overlays")
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
