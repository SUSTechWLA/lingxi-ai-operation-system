package localtool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

func TestIPArollMCPOutputFeedsHyperFramesComposition(t *testing.T) {
	dataDir := t.TempDir()
	sourceVideo := filepath.Join(t.TempDir(), "ip-aroll.mp4")
	if err := os.WriteFile(sourceVideo, []byte("fake continuous A-roll"), 0o644); err != nil {
		t.Fatal(err)
	}

	mcp := newMCPProtocolTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		params := request["params"].(map[string]interface{})
		if params["name"] != "ip_avatar_3d.render_talking_video" {
			t.Fatalf("unexpected MCP tool: %#v", params["name"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"result": map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"status":      "ready",
					"sourceType":  "ip_aroll_video",
					"durationSec": 15,
					"videoPath":   sourceVideo,
				},
			},
		})
	}))
	defer mcp.Close()

	mcpExecutor := NewMCPToolCallExecutorWithDataDir(func() ([]localmcp.ProviderConfig, error) {
		return []localmcp.ProviderConfig{{ID: "ip_avatar_3d", Endpoint: mcp.URL, Enabled: true}}, nil
	}, dataDir)
	mcpResult, err := mcpExecutor.Execute(context.Background(), Job{
		ID:        "render-aroll",
		ProjectID: "project_001",
		Command:   CommandLocalMCPToolCall,
		Payload: map[string]interface{}{
			"providerId": "ip_avatar_3d",
			"mcpTool":    "ip_avatar_3d.render_talking_video",
			"externalGenerationRequests": []interface{}{map[string]interface{}{
				"requestId": "ip_aroll_main",
				"shotId":    "AROLL_MAIN",
				"kind":      "ip_aroll_video",
				"arguments": map[string]interface{}{"script": "用连续 A-roll 建立信任。"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("render A-roll: %v", err)
	}

	projectExecutor := NewHyperFramesProjectExecutor(dataDir)
	_, err = projectExecutor.Execute(context.Background(), Job{
		ID:        "compose-aroll",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":              "AI 视频创作流程",
			"script":             "用连续 A-roll 建立信任。",
			"aRollAssetPackages": mcpResult.Output["aRollAssetPackages"],
		},
	})
	if err != nil {
		t.Fatalf("compose A-roll: %v", err)
	}

	indexPath := filepath.Join(dataDir, "projects", "project_001", "hyperframes", "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	index := string(raw)
	for _, expected := range []string{
		`class="has-aroll arroll-clean"`,
		`class="aroll-media clip"`,
		`class="aroll-audio clip"`,
		`data-duration="15.0"`,
		`data-track-index="10"`,
	} {
		if !strings.Contains(index, expected) {
			t.Fatalf("generated HyperFrames project missing %q", expected)
		}
	}
}

func TestIPArollE2EProjectFromRealMedia(t *testing.T) {
	sourceVideo := strings.TrimSpace(os.Getenv("TANGYING_IP_AROLL_E2E_SOURCE"))
	dataDir := strings.TrimSpace(os.Getenv("TANGYING_IP_AROLL_E2E_DATA_DIR"))
	if sourceVideo == "" || dataDir == "" {
		t.Skip("set TANGYING_IP_AROLL_E2E_SOURCE and TANGYING_IP_AROLL_E2E_DATA_DIR to generate a real-media project")
	}
	projectID := "ip-aroll-e2e"
	if err := mirrorE2EMedia(sourceVideo, filepath.Join(dataDir, "artifacts", projectID, "ip-aroll-main", "content")); err != nil {
		t.Fatalf("mirror A-roll media: %v", err)
	}

	payload := map[string]interface{}{
		"topic":  "一个主题如何变成可发布视频",
		"script": "输入主题后，AIOS 先生成口播稿，再驱动三维角色完成口型、手势和虚拟机位拍摄。B-roll 只在需要证据和演示时出现。",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":        "AROLL_OPEN",
				"startSec":      float64(0),
				"durationSec":   float64(5),
				"sceneSummary":  "树懒正面打招呼并建立主题",
				"mainAction":    "掌心朝镜头挥手，随后回到自然讲解姿态",
				"narrationText": "输入一个主题，系统就能把它变成一条完整视频。",
			},
			map[string]interface{}{
				"shotId":        "BROLL_PROOF",
				"startSec":      float64(5),
				"durationSec":   float64(4),
				"sceneSummary":  "用流程画面证明自动编排能力",
				"mainAction":    "B-roll 全屏覆盖，A-roll 音频连续",
				"narrationText": "脚本、动作、镜头和素材会被编译成同一条时间线。",
			},
			map[string]interface{}{
				"shotId":        "AROLL_CLOSE",
				"startSec":      float64(9),
				"durationSec":   float64(6.333),
				"sceneSummary":  "回到树懒中景总结",
				"mainAction":    "双手解释后点头收束",
				"narrationText": "A-roll 负责信任，B-roll 负责证据，最后输出可审核成片。",
			},
		},
		"aRollAssetPackages": []interface{}{
			map[string]interface{}{
				"shotId":      "AROLL_MAIN",
				"durationSec": float64(15.333),
				"sourceType":  "ip_aroll_video",
				"generationPlan": map[string]interface{}{
					"mode": "ip_aroll_video",
					"fusionPlan": map[string]interface{}{
						"baseLayer": map[string]interface{}{
							"kind":       "video",
							"role":       "a_roll",
							"storageRef": "local://projects/ip-aroll-e2e/artifacts/ip-aroll-main/e2e/aroll.mp4",
						},
					},
				},
			},
		},
	}

	if sourceBroll := strings.TrimSpace(os.Getenv("TANGYING_IP_AROLL_E2E_BROLL_SOURCE")); sourceBroll != "" {
		if err := mirrorE2EMedia(sourceBroll, filepath.Join(dataDir, "artifacts", projectID, "broll-proof", "content")); err != nil {
			t.Fatalf("mirror B-roll media: %v", err)
		}
		payload["shotAssetPackages"] = []interface{}{
			map[string]interface{}{
				"shotId":      "BROLL_PROOF",
				"startSec":    float64(5),
				"durationSec": float64(4),
				"visualMode":  "full_screen",
				"generationPlan": map[string]interface{}{
					"mode": "aigc_video",
					"fusionPlan": map[string]interface{}{
						"baseLayer": map[string]interface{}{
							"kind":       "image",
							"role":       "b_roll",
							"storageRef": "local://projects/ip-aroll-e2e/artifacts/broll-proof/e2e/broll.png",
						},
					},
				},
			},
		}
	}

	executor := NewHyperFramesProjectExecutor(dataDir)
	if _, err := executor.Execute(context.Background(), Job{
		ID:        "compose-real-ip-aroll",
		ProjectID: projectID,
		Command:   CommandHyperFramesProjectGenerate,
		Payload:   payload,
	}); err != nil {
		t.Fatalf("compose real-media project: %v", err)
	}

	projectRoot := filepath.Join(dataDir, "projects", projectID, "hyperframes")
	index, err := os.ReadFile(filepath.Join(projectRoot, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`class="aroll-media clip"`, `class="aroll-audio clip"`, `data-duration="15.3"`} {
		if !strings.Contains(string(index), expected) {
			t.Fatalf("real-media project missing %q", expected)
		}
	}
	t.Logf("real-media HyperFrames project: %s", projectRoot)
}

func mirrorE2EMedia(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		return err
	}
	return destination.Close()
}
