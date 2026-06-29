package assets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

func TestBuildManualAssetManifest(t *testing.T) {
	got, err := BuildManualAssetManifest(ImportRequest{
		Type:          AssetTypeVideo,
		StorageType:   "local",
		StorageRef:    "local://projects/p1/imports/clip.mp4",
		DurationSec:   12,
		Description:   "手动导入的参考视频",
		Tags:          []string{"reference", "opening"},
		RelatedShotID: "shot-1",
		Source:        "manual_upload",
	})
	if err != nil {
		t.Fatalf("BuildManualAssetManifest returned error: %v", err)
	}
	if got.AssetID == "" || got.Type != AssetTypeVideo || got.StorageRef == "" {
		t.Fatalf("manifest missing key fields: %+v", got)
	}
	if got.RelatedShotID != "shot-1" || got.Source != "manual_upload" {
		t.Fatalf("manifest relation/source mismatch: %+v", got)
	}
}

func TestBuildManualAssetManifestRejectsUnsupportedType(t *testing.T) {
	_, err := BuildManualAssetManifest(ImportRequest{Type: "pdf", StorageRef: "local://x"})
	if err == nil {
		t.Fatalf("expected unsupported asset type error")
	}
}

func TestBuildExternalGenerationRequestBuildsInspectablePromptPackage(t *testing.T) {
	request, err := BuildExternalGenerationRequest(ExternalGenerationInput{
		Kind:      AssetTypeVideo,
		ProjectID: "project-1",
		ShotID:    "shot-1",
		StageName: "video_prompt",
		Prompt:    "生成 8 秒视频，角色 A 从左向右走入旧书店，保持人物服装和场景一致。",
		References: []ReferenceImageInput{
			{ID: "char-a", Label: "人物 A", Role: ReferenceRoleCharacter, StorageRef: "local://projects/project-1/artifacts/character/a.png"},
			{ID: "scene-bookstore", Label: "旧书店", Role: ReferenceRoleScene, StorageRef: "local://projects/project-1/artifacts/scene/bookstore.png"},
		},
		Target: ExternalGenerationTarget{
			AspectRatio: "16:9",
			DurationSec: 8,
			Resolution:  "1920x1080",
		},
	})
	if err != nil {
		t.Fatalf("BuildExternalGenerationRequest returned error: %v", err)
	}
	if request.RequestID == "" {
		t.Fatalf("request id should be stable and non-empty: %+v", request)
	}
	if request.PromptCharLimit != 2000 || request.ReferenceImageLimit != 6 {
		t.Fatalf("limits should be explicit for external tools: %+v", request)
	}
	if request.Status != ExternalGenerationStatusPendingUpload {
		t.Fatalf("status = %q, want pending upload", request.Status)
	}
	if len(request.References) != 2 || request.References[0].Role != ReferenceRoleCharacter {
		t.Fatalf("references not preserved: %+v", request.References)
	}
}

func TestBuildExternalGenerationRequestRejectsPromptOverLimit(t *testing.T) {
	_, err := BuildExternalGenerationRequest(ExternalGenerationInput{
		Kind:   AssetTypeImage,
		Prompt: strings.Repeat("字", 2001),
	})
	if err == nil {
		t.Fatalf("expected prompt length validation error")
	}
}

func TestBuildExternalGenerationRequestRejectsMoreThanSixReferenceImages(t *testing.T) {
	refs := make([]ReferenceImageInput, 0, 7)
	for i := 0; i < 7; i++ {
		refs = append(refs, ReferenceImageInput{
			ID:         "ref-" + string(rune('a'+i)),
			Label:      "参考图",
			Role:       ReferenceRoleStoryboard,
			StorageRef: "local://projects/project-1/ref.png",
		})
	}
	_, err := BuildExternalGenerationRequest(ExternalGenerationInput{
		Kind:       AssetTypeVideo,
		Prompt:     "生成一段 6 秒视频。",
		References: refs,
	})
	if err == nil {
		t.Fatalf("expected reference image count validation error")
	}
}

func TestBuildExternalGenerationArtifactRequestIsInlineReviewable(t *testing.T) {
	request, err := BuildExternalGenerationRequest(ExternalGenerationInput{
		Kind:      AssetTypeImage,
		ProjectID: "project-1",
		ShotID:    "shot-2",
		Prompt:    "生成一张关键帧，保持人物和道具一致。",
		References: []ReferenceImageInput{
			{ID: "prop-1", Role: ReferenceRoleProp, StorageRef: "local://projects/project-1/artifacts/props/mic.png"},
		},
	})
	if err != nil {
		t.Fatalf("BuildExternalGenerationRequest returned error: %v", err)
	}
	req, err := BuildExternalGenerationArtifactRequest("project-1", "run-1", "task-1", request)
	if err != nil {
		t.Fatalf("BuildExternalGenerationArtifactRequest returned error: %v", err)
	}
	if req.Kind != artifact.KindJSON || req.StorageType != artifact.StorageInline {
		t.Fatalf("prompt package should be inline JSON, got kind=%s storage=%s", req.Kind, req.StorageType)
	}
	if req.Provider != "external-generation-request" {
		t.Fatalf("provider = %q, want external-generation-request", req.Provider)
	}
	if req.Metadata["artifactType"] != "external_generation_request" || req.Metadata["generationKind"] != string(AssetTypeImage) {
		t.Fatalf("metadata should index external request: %+v", req.Metadata)
	}
	var decoded ExternalGenerationRequest
	if err := json.Unmarshal(req.Data, &decoded); err != nil {
		t.Fatalf("request data is not request JSON: %v", err)
	}
	if decoded.RequestID != request.RequestID {
		t.Fatalf("decoded request id = %q, want %q", decoded.RequestID, request.RequestID)
	}
}

func TestBuildExternalGenerationResultArtifactRequestKeepsMediaLocal(t *testing.T) {
	manifest, err := BuildManualAssetManifest(ImportRequest{
		Type:                AssetTypeVideo,
		StorageRef:          "local://projects/project-1/artifacts/external/shot-1.mp4",
		MimeType:            "video/mp4",
		SizeBytes:           42,
		ContentHash:         "sha256:abc123",
		RelatedShotID:       "shot-1",
		Source:              "external_generation_upload",
		GenerationRequestID: "extgen_123",
		ExternalPlatform:    "seedance-web",
		PromptHash:          "prompt-sha",
		ReferenceAssetIDs:   []string{"char-a", "scene-bookstore"},
	})
	if err != nil {
		t.Fatalf("BuildManualAssetManifest returned error: %v", err)
	}
	req, err := BuildExternalGenerationResultArtifactRequest("project-1", "run-1", "task-1", manifest)
	if err != nil {
		t.Fatalf("BuildExternalGenerationResultArtifactRequest returned error: %v", err)
	}
	if req.Kind != artifact.KindVideo || req.StorageType != artifact.StorageLocal {
		t.Fatalf("external result should be a local video artifact, got kind=%s storage=%s", req.Kind, req.StorageType)
	}
	if len(req.Data) != 0 || req.StorageRef != manifest.StorageRef {
		t.Fatalf("media payload must stay local: data=%d storageRef=%q", len(req.Data), req.StorageRef)
	}
	if req.ContentHash != "sha256:abc123" || req.PromptHash != "prompt-sha" {
		t.Fatalf("hash metadata not carried: content=%q prompt=%q", req.ContentHash, req.PromptHash)
	}
	if req.Metadata["externalGenerationRequestId"] != "extgen_123" {
		t.Fatalf("missing request link metadata: %+v", req.Metadata)
	}
}

func TestBuildAssetManifestArtifactRequest(t *testing.T) {
	manifest, err := BuildManualAssetManifest(ImportRequest{
		Type:        AssetTypeImage,
		StorageType: "local",
		StorageRef:  "local://projects/p1/imports/ref.png",
		Source:      "manual_upload",
	})
	if err != nil {
		t.Fatalf("BuildManualAssetManifest returned error: %v", err)
	}
	req, err := BuildArtifactRequest("project-1", "run-1", "task-1", manifest)
	if err != nil {
		t.Fatalf("BuildArtifactRequest returned error: %v", err)
	}
	if req.StageName != "asset_manifest" || req.UnitID != "asset_manifest" {
		t.Fatalf("unexpected artifact scope: %+v", req)
	}
	if req.Kind != artifact.KindJSON || req.Name != "asset_manifest.json" {
		t.Fatalf("unexpected artifact kind/name: %+v", req)
	}
	var decoded ManualAssetManifest
	if err := json.Unmarshal(req.Data, &decoded); err != nil {
		t.Fatalf("artifact data is not manifest JSON: %v", err)
	}
	if decoded.AssetID != manifest.AssetID {
		t.Fatalf("decoded assetId = %q, want %q", decoded.AssetID, manifest.AssetID)
	}
}

func TestServiceSaveManifestWritesAssetManifestArtifact(t *testing.T) {
	sink := &fakeArtifactSink{}
	svc := NewService(sink)
	manifest, artifactRecord, err := svc.SaveManifest(context.Background(), SaveRequest{
		ProjectID: "project-1",
		TaskID:    "task-1",
		Import: ImportRequest{
			Type:       AssetTypeVideo,
			StorageRef: "local://projects/p1/imports/clip.mp4",
		},
	})
	if err != nil {
		t.Fatalf("SaveManifest returned error: %v", err)
	}
	if manifest.AssetID == "" {
		t.Fatalf("manifest missing assetId: %+v", manifest)
	}
	if artifactRecord == nil || artifactRecord.StageName != "asset_manifest" {
		t.Fatalf("unexpected artifact record: %+v", artifactRecord)
	}
	if sink.lastReq == nil || sink.lastReq.UnitID != "asset_manifest" {
		t.Fatalf("artifact sink was not called with asset_manifest: %+v", sink.lastReq)
	}
}

type fakeArtifactSink struct {
	lastReq *artifact.CreateArtifactRequest
}

func (s *fakeArtifactSink) CreateArtifact(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	s.lastReq = req
	return &artifact.Artifact{
		ID:          "artifact-1",
		ProjectID:   req.ProjectID,
		StageName:   req.StageName,
		UnitID:      req.UnitID,
		Kind:        req.Kind,
		Name:        req.Name,
		StorageType: req.StorageType,
	}, nil
}
