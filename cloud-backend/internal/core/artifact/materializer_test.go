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

func TestBuildArtifactsFromNodeOutputHandlesSingleMapManifest(t *testing.T) {
	// Verify that the materializer handles a single-map (object) artifacts
	// manifest (legacy format from older tool versions) by wrapping it.
	node := &model.Node{
		ID:     "opso_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "opso",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"content": "## OPSO\n口播优化结果。",
				"artifacts": {
					"stage": "opso",
					"skillName": "create-opinion-videos",
					"requiresReview": true
				}
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	// The single-map format should be accepted (wrapped into an array).
	// Entries without unitId and kind are skipped, so we expect 0 valid requests
	// but the function should NOT panic or return nil from the type assertion failure.
	// The legacy map has no unitId/kind, so it's filtered out — that's expected.
	// The key behavior is: no panic, and the function completes normally.
	if requests == nil {
		t.Fatal("expected non-nil result, legacy map should be handled gracefully")
	}
	// No valid artifact entries expected since legacy map lacks unitId/kind
	if len(requests) != 0 {
		t.Fatalf("expected 0 artifact requests from legacy map without unitId/kind, got %d", len(requests))
	}
}

func TestArtifactMaterializerRejectsArtifactWithoutUnitID(t *testing.T) {
	node := &model.Node{
		ID:     "render_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "render",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"artifacts": [
					{
						"kind": "VIDEO",
						"name": "final.mp4",
						"storageRef": "local://projects/vp-1/renders/final.mp4"
					}
				]
			}`,
		},
	}

	_, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err == nil {
		t.Fatal("expected missing unitId to be rejected")
	}
	if !strings.Contains(err.Error(), "ARTIFACT_MANIFEST_INVALID") || !strings.Contains(err.Error(), "unitId is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArtifactMaterializerRejectsArtifactWithoutKind(t *testing.T) {
	node := &model.Node{
		ID:     "render_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "render",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"artifacts": [
					{
						"unitId": "final-video",
						"name": "final.mp4",
						"storageRef": "local://projects/vp-1/renders/final.mp4"
					}
				]
			}`,
		},
	}

	_, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err == nil {
		t.Fatal("expected missing kind to be rejected")
	}
	if !strings.Contains(err.Error(), "ARTIFACT_MANIFEST_INVALID") || !strings.Contains(err.Error(), "kind is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildArtifactsFromNodeOutputHandlesMapManifestWithValidFields(t *testing.T) {
	// Verify that a single-map artifacts manifest WITH valid unitId/kind
	// is correctly materialized into an artifact request.
	node := &model.Node{
		ID:     "publish_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "publish",
		},
		Output: map[string]interface{}{
			"stdout": `{
					"title": "端午节为什么吃粽子",
					"description": "讲清楚端午节和粽子的来源",
					"keywords": ["端午节", "粽子"],
					"artifacts": {
						"unitId": "publish-copy",
						"kind": "JSON",
					"name": "发布文案",
					"mimeType": "application/json",
					"contentHash": "abc123",
					"sizeBytes": 512
				}
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	if len(requests) != 1 {
		t.Fatalf("expected 1 artifact request from single-map with valid fields, got %d", len(requests))
	}
	req := requests[0]
	if req.UnitID != "publish-copy" {
		t.Fatalf("expected unitId 'publish-copy', got %q", req.UnitID)
	}
	if req.Kind != KindJSON {
		t.Fatalf("expected kind JSON, got %q", req.Kind)
	}
	if req.Name != "发布文案" {
		t.Fatalf("expected name '发布文案', got %q", req.Name)
	}
	if req.StorageType != StorageLocal {
		t.Fatalf("expected local storage, got %q", req.StorageType)
	}
}

func TestBuildArtifactsFromCompositionExecMaterializesReviewableVideoCompositionSpec(t *testing.T) {
	node := &model.Node{
		ID:     "composition_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "composition",
			"tool":  "video_composition_builder",
		},
		Output: map[string]interface{}{
			"content": "视频结构已生成。",
			"compositionSpec": map[string]interface{}{
				"totalDurationSec": 30,
				"tracks":           []interface{}{},
			},
			"artifacts": []interface{}{
				map[string]interface{}{
					"unitId":   "composition",
					"kind":     "VIDEO_COMPOSITION_SPEC",
					"name":     "composition.json",
					"mimeType": "application/json",
					"metadata": map[string]interface{}{
						"requiresReview": true,
					},
				},
			},
		},
	}

	requests, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err != nil {
		t.Fatalf("composition artifact should materialize: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one composition artifact request, got %+v", requests)
	}
	req := requests[0]
	if req.StageName != "composition" {
		t.Fatalf("expected composition stage, got %q", req.StageName)
	}
	if req.Kind != ArtifactKind("VIDEO_COMPOSITION_SPEC") {
		t.Fatalf("expected VIDEO_COMPOSITION_SPEC artifact, got %q", req.Kind)
	}
	if req.Metadata["requiresReview"] != true {
		t.Fatalf("composition artifact must be reviewable, got metadata %+v", req.Metadata)
	}
	if req.Metadata["humanApproved"] != false || req.Metadata["status"] != "valid" {
		t.Fatalf("new composition artifact should start valid and not human-approved, got metadata %+v", req.Metadata)
	}
	if len(req.Data) == 0 {
		t.Fatalf("composition artifact should carry inline cloud preview data for review")
	}
}

func TestBuildArtifactsExternalGenerationRequestUsesInlineReviewableProvider(t *testing.T) {
	node := &model.Node{
		ID:     "video_prompt_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "video_prompt",
			"tool":  "video_prompt_generator",
		},
		Output: map[string]interface{}{
			"externalGenerationRequests": []interface{}{
				map[string]interface{}{
					"requestId":           "extgen_123",
					"kind":                "video",
					"shotId":              "shot-1",
					"prompt":              "生成 8 秒视频，保持人物、道具和旧书店场景一致。",
					"promptCharLimit":     2000,
					"referenceImageLimit": 6,
					"references": []interface{}{
						map[string]interface{}{"id": "char-a", "role": "character", "storageRef": "local://projects/vp-1/characters/a.png"},
					},
				},
			},
			"artifacts": []interface{}{
				map[string]interface{}{
					"unitId":   "extgen_123",
					"kind":     "JSON",
					"name":     "external_generation_request.json",
					"mimeType": "application/json",
					"metadata": map[string]interface{}{
						"artifactType":   "external_generation_request",
						"generationKind": "video",
					},
				},
			},
		},
	}

	requests, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err != nil {
		t.Fatalf("external generation request should materialize: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one request artifact, got %+v", requests)
	}
	req := requests[0]
	if req.Provider != "external-generation-request" {
		t.Fatalf("provider = %q, want external-generation-request", req.Provider)
	}
	if req.StorageType != StorageInline {
		t.Fatalf("storage type = %q, want inline", req.StorageType)
	}
	if !strings.Contains(string(req.Data), "生成 8 秒视频") {
		t.Fatalf("request payload should contain prompt, got %s", string(req.Data))
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(req.Data, &decoded); err != nil {
		t.Fatalf("request payload should be JSON: %v", err)
	}
	if decoded["prompt"] == nil || decoded["externalGenerationRequests"] != nil {
		t.Fatalf("request payload should be the individual request, got %+v", decoded)
	}
}

func TestBuildArtifactsExternalGenerationRequestRejectsPromptOverLimit(t *testing.T) {
	node := externalGenerationRequestNode("extgen_too_long", map[string]interface{}{
		"requestId":           "extgen_too_long",
		"kind":                "image",
		"prompt":              strings.Repeat("字", 2001),
		"promptCharLimit":     2000,
		"referenceImageLimit": 6,
	})

	_, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err == nil || !IsArtifactManifestInvalid(err) {
		t.Fatalf("expected manifest validation error for prompt limit, got %v", err)
	}
}

func TestBuildArtifactsExternalGenerationRequestRejectsTooManyReferenceImages(t *testing.T) {
	refs := make([]interface{}, 0, 7)
	for i := 0; i < 7; i++ {
		refs = append(refs, map[string]interface{}{
			"id":         "ref",
			"role":       "keyframe",
			"storageRef": "local://projects/vp-1/ref.png",
		})
	}
	node := externalGenerationRequestNode("extgen_too_many_refs", map[string]interface{}{
		"requestId":           "extgen_too_many_refs",
		"kind":                "video",
		"prompt":              "生成 6 秒视频。",
		"references":          refs,
		"promptCharLimit":     2000,
		"referenceImageLimit": 6,
	})

	_, err := BuildArtifactRequestsFromNodeChecked("vp-1", "run-1", node)
	if err == nil || !IsArtifactManifestInvalid(err) {
		t.Fatalf("expected manifest validation error for reference limit, got %v", err)
	}
}

func externalGenerationRequestNode(unitID string, request map[string]interface{}) *model.Node {
	return &model.Node{
		ID:     "external_generation_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "video_prompt",
			"tool":  "video_prompt_generator",
		},
		Output: map[string]interface{}{
			"externalGenerationRequests": []interface{}{request},
			"artifacts": []interface{}{
				map[string]interface{}{
					"unitId":   unitID,
					"kind":     "JSON",
					"name":     "external_generation_request.json",
					"mimeType": "application/json",
					"metadata": map[string]interface{}{
						"artifactType": "external_generation_request",
					},
				},
			},
		},
	}
}

func TestBuildArtifactsFromNodeOutputSkipsEmptyPublishCopyArtifact(t *testing.T) {
	node := &model.Node{
		ID:     "viewpoint_dossier",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "viewpoint_dossier",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"content": "{\"核心观点\":\"端午不只是吃粽子\"}",
				"title": "",
				"description": "",
				"artifacts": [
					{
						"unitId": "viewpoint_dossier",
						"kind": "MARKDOWN",
						"name": "viewpoint_dossier.md",
						"mimeType": "text/markdown"
					},
					{
						"unitId": "publish-copy",
						"kind": "JSON",
						"name": "发布文案.json",
						"mimeType": "application/json"
					}
				]
			}`,
		},
	}

	requests := BuildArtifactRequestsFromNode("vp-1", "run-1", node)

	for _, req := range requests {
		if req.UnitID == "publish-copy" {
			t.Fatalf("empty publish-copy artifact should be skipped: %+v", req)
		}
	}
}

func TestContentFromMatchingNodeArtifactHydratesLocalMarkdown(t *testing.T) {
	artifact := &Artifact{
		ProjectID:     "vp-1",
		WorkflowRunID: "run-1",
		StageName:     "viewpoint_dossier",
		UnitID:        "viewpoint_dossier",
		Kind:          KindMarkdown,
		Name:          "viewpoint_dossier.md",
		StorageType:   StorageLocal,
		MimeType:      "text/markdown",
	}
	node := &model.Node{
		ID:     "viewpoint_dossier_exec",
		Status: model.NodeSuccess,
		Input: map[string]interface{}{
			"stage": "viewpoint_dossier",
		},
		Output: map[string]interface{}{
			"stdout": `{
				"content": "# 观点档案\n端午节和粽子的来源。",
				"artifacts": [
					{
						"unitId": "viewpoint_dossier",
						"kind": "MARKDOWN",
						"name": "viewpoint_dossier.md",
						"mimeType": "text/markdown",
						"storageRef": "local://projects/vp-1/artifacts/viewpoint_dossier/viewpoint_dossier/hash/viewpoint_dossier.md"
					}
				]
			}`,
		},
	}

	content, ok := contentFromMatchingNodeArtifact("vp-1", "run-1", artifact, node)

	if !ok {
		t.Fatal("expected local markdown artifact to hydrate from matching node output")
	}
	if string(content) != "# 观点档案\n端午节和粽子的来源。" {
		t.Fatalf("unexpected hydrated content: %q", string(content))
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
	if req.StorageType != StorageInline {
		t.Fatalf("revision should be stored inline for immediate review, got %q", req.StorageType)
	}
	if req.StorageRef == "" {
		t.Fatalf("revision should include a local storage ref")
	}
	if string(req.Data) != "## 新稿\n开头更犀利。" {
		t.Fatalf("revision request should carry reviewable revised content, got %q", string(req.Data))
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
