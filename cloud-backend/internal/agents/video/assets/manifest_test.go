package assets

import (
	"context"
	"encoding/json"
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
