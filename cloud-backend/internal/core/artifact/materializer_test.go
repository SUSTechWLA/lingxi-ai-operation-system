package artifact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestBuildArtifactsFromNodeOutputCreatesDisplayableArtifactsFromLocalManifest(t *testing.T) {
	node := &model.Node{
		ID:     "render_review_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "render_review",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"artifacts":[
					{
						"unitId":"content",
						"kind":"MARKDOWN",
						"name":"render_review.md",
						"mimeType":"text/markdown; charset=utf-8",
						"storageRef":"local://projects/vp-1/artifacts/render_review/content/hash/render_review.md",
						"contentHash":"hash",
						"sizeBytes":128,
						"metadata":{"displayable":true}
					},
					{
						"unitId":"final-video",
						"kind":"VIDEO",
						"name":"final.mp4",
						"mimeType":"video/mp4",
						"storageRef":"local://projects/vp-1/artifacts/render_review/final-video/hash/final.mp4",
						"contentHash":"video-hash",
						"sizeBytes":4096
					}
				]
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	kinds := map[ArtifactKind]bool{}
	for _, req := range requests {
		kinds[req.Kind] = true
	}
	for _, kind := range []ArtifactKind{KindMarkdown, KindVideo} {
		if !kinds[kind] {
			t.Fatalf("expected artifact kind %s in requests: %+v", kind, requests)
		}
	}
	for _, req := range requests {
		if req.StorageType != StorageLocal {
			t.Fatalf("artifact request should use local storage, got %q for %+v", req.StorageType, req)
		}
		if req.StorageRef == "" {
			t.Fatalf("artifact request should include a local storage ref: %+v", req)
		}
		if !strings.HasPrefix(req.StorageRef, "local://projects/vp-1/artifacts/") {
			t.Fatalf("artifact request should point to a project local ref, got %q", req.StorageRef)
		}
		if len(req.Data) != 0 {
			t.Fatalf("cloud artifact materializer should not carry user payload data: %+v", req)
		}
	}
	if requests[0].SizeBytes != 128 {
		t.Fatalf("size should come from local manifest, got %d", requests[0].SizeBytes)
	}
}

func TestBuildArtifactsFromNodeOutputIgnoresLegacyPayloadFields(t *testing.T) {
	node := &model.Node{
		ID:     "render_review_exec",
		Status: model.NodeSuccess,
		Input:  map[string]interface{}{"stage": "render_review"},
		Output: map[string]interface{}{
			"stdout": `{
				"content":"## 成片审核\n这是一段 markdown。",
				"imageRequests":[{"prompt":"生成封面图","url":"data:image/png;base64,AAA"}],
				"videoImportPackage":{"videoUrl":"https://example.com/video.mp4","videoPrompt":"生成视频"}
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	if len(requests) != 0 {
		raw, _ := json.Marshal(requests)
		t.Fatalf("legacy payload fields should not be materialized through cloud: %s", raw)
	}
}

func TestReviseInlineArtifactCreatesNewVersionPayload(t *testing.T) {
	base := &Artifact{
		ID:          "art-1",
		ProjectID:   "vp-1",
		StageName:   "recording_script",
		UnitID:      "content",
		Kind:        KindMarkdown,
		Name:        "recording_script.md",
		Version:     1,
		StorageType: "inline",
		InlineJSON:  "## 旧稿\n开头太弱。",
	}

	req := BuildRevisionRequest(base, "开头更犀利一点", []byte("## 新稿\n开头更犀利。"))

	if req.ProjectID != base.ProjectID || req.StageName != base.StageName || req.UnitID != base.UnitID {
		t.Fatalf("revision should preserve artifact scope: %+v", req)
	}
	if req.Metadata["revisionInstruction"] != "开头更犀利一点" {
		t.Fatalf("revision instruction missing from metadata: %+v", req.Metadata)
	}
	if req.StorageType != StorageLocal {
		t.Fatalf("revision should be stored locally, got %q", req.StorageType)
	}
	if req.StorageRef == "" {
		t.Fatalf("revision should include a local storage ref")
	}
	if len(req.Data) != 0 {
		t.Fatalf("revision request should not carry user payload through cloud: %s", string(req.Data))
	}
}

func TestArtifactContentReturnsAllMediaURLs(t *testing.T) {
	artifact := &Artifact{
		Kind:        KindVideo,
		StorageType: "inline",
		MimeType:    "application/json",
		InlineJSON: `{
			"coverUrl": "https://example.com/cover.png",
			"videoUrl": "https://example.com/final.mp4",
			"clips": [
				{"url": "https://example.com/clip-a.mp4"},
				{"src": "data:video/mp4;base64,AAAA"}
			],
			"audioPackage": {"audioUrl": "https://example.com/voice.mp3"}
		}`,
	}

	_, mediaURL, mediaURLs := artifactContent(artifact)

	if mediaURL != "https://example.com/final.mp4" {
		t.Fatalf("expected first media URL, got %q", mediaURL)
	}
	expected := []string{
		"https://example.com/final.mp4",
		"https://example.com/cover.png",
		"https://example.com/clip-a.mp4",
		"data:video/mp4;base64,AAAA",
		"https://example.com/voice.mp3",
	}
	if len(mediaURLs) != len(expected) {
		t.Fatalf("expected %d media URLs, got %d: %#v", len(expected), len(mediaURLs), mediaURLs)
	}
	for i, want := range expected {
		if mediaURLs[i] != want {
			t.Fatalf("mediaURLs[%d] = %q, want %q", i, mediaURLs[i], want)
		}
	}
}
