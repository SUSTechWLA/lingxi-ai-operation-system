package repository

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeOutputForPersistenceRedactsUserPayloads(t *testing.T) {
	output := map[string]interface{}{
		"stdout": `{
			"content":"## 用户脚本\n这段正文不能进入云端数据库。",
			"imageRequests":[{"url":"data:image/png;base64,AAAA","prompt":"封面图 prompt"}],
			"videoImportPackage":{"videoUrl":"https://provider.example.com/final.mp4"},
			"artifacts":[{
				"unitId":"content",
				"kind":"MARKDOWN",
				"storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
				"contentHash":"hash",
				"sizeBytes":42
			}],
			"exitCode":0
		}`,
		"content":     "短正文也不能直接进云端。",
		"storageRef":  "local://projects/vp-1/artifacts/script/content/hash/script.md",
		"contentHash": "hash",
		"sizeBytes":   42,
	}

	cleaned := SanitizeOutputForPersistence(output)
	raw, err := json.Marshal(cleaned)
	if err != nil {
		t.Fatalf("sanitized output should marshal: %v", err)
	}
	text := string(raw)

	for _, forbidden := range []string{
		"这段正文不能进入云端数据库",
		"短正文也不能直接进云端",
		"data:image/png;base64",
		"https://provider.example.com/final.mp4",
		"封面图 prompt",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("sanitized output still contains user payload %q: %s", forbidden, text)
		}
	}
	for _, expected := range []string{
		"local://projects/vp-1/artifacts/script/content/hash/script.md",
		"contentHash",
		"sizeBytes",
		"USER_ASSET_REDACTED",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("sanitized output missing expected marker %q: %s", expected, text)
		}
	}
}

func TestSanitizeOutputForPersistencePreservesArtifactManifest(t *testing.T) {
	output := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{
				"unitId":      "final-video",
				"kind":        "VIDEO",
				"name":        "final.mp4",
				"storageRef":  "local://projects/vp-1/artifacts/render/final-video/hash/final.mp4",
				"contentHash": "video-hash",
				"sizeBytes":   float64(4096),
			},
		},
	}

	cleaned := SanitizeOutputForPersistence(output)
	artifacts, ok := cleaned["artifacts"].([]interface{})
	if !ok || len(artifacts) != 1 {
		t.Fatalf("artifact manifest should be preserved: %+v", cleaned)
	}
	first, ok := artifacts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("artifact manifest entry should remain an object: %+v", artifacts[0])
	}
	if first["storageRef"] != "local://projects/vp-1/artifacts/render/final-video/hash/final.mp4" {
		t.Fatalf("local storage ref changed: %+v", first)
	}
	if first["sizeBytes"] != float64(4096) {
		t.Fatalf("size should be preserved from local manifest: %+v", first)
	}
}
