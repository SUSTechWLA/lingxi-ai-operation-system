package artifact

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestBuildArtifactsFromNodeOutputCreatesDisplayableMediaArtifacts(t *testing.T) {
	node := &model.Node{
		ID:     "render_review_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "render_review",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"content":"## 成片审核\n这是一段 markdown。",
				"imageRequests":[{"prompt":"生成封面图","url":"data:image/png;base64,AAA"}],
				"videoImportPackage":{"videoUrl":"https://example.com/video.mp4","videoPrompt":"生成视频"},
				"audioPackage":{"audioUrl":"https://example.com/audio.mp3","transcript":"旁白文本"}
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	kinds := map[ArtifactKind]bool{}
	for _, req := range requests {
		kinds[req.Kind] = true
	}
	for _, kind := range []ArtifactKind{KindMarkdown, KindImage, KindVideo, KindAudio} {
		if !kinds[kind] {
			t.Fatalf("expected artifact kind %s in requests: %+v", kind, requests)
		}
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
	if string(req.Data) != "## 新稿\n开头更犀利。" {
		t.Fatalf("revision payload mismatch: %s", string(req.Data))
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
