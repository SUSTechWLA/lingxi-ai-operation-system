package assets

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

type AssetType string

const (
	AssetTypeImage AssetType = "image"
	AssetTypeAudio AssetType = "audio"
	AssetTypeVideo AssetType = "video"
)

type ImportRequest struct {
	Type          AssetType `json:"type"`
	StorageType   string    `json:"storageType"`
	StorageRef    string    `json:"storageRef"`
	DurationSec   int       `json:"durationSec,omitempty"`
	Description   string    `json:"description,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	RelatedShotID string    `json:"relatedShotId,omitempty"`
	Source        string    `json:"source,omitempty"`
}

type ManualAssetManifest struct {
	AssetID       string    `json:"assetId"`
	Type          AssetType `json:"type"`
	StorageType   string    `json:"storageType"`
	StorageRef    string    `json:"storageRef"`
	DurationSec   int       `json:"durationSec,omitempty"`
	Description   string    `json:"description,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	RelatedShotID string    `json:"relatedShotId,omitempty"`
	Source        string    `json:"source"`
}

type ArtifactSink interface {
	CreateArtifact(ctx context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error)
}

type Service struct {
	sink ArtifactSink
}

type SaveRequest struct {
	ProjectID     string        `json:"projectId,omitempty"`
	WorkflowRunID string        `json:"workflowRunId,omitempty"`
	TaskID        string        `json:"taskId,omitempty"`
	Import        ImportRequest `json:"import"`
}

func NewService(sink ArtifactSink) *Service {
	return &Service{sink: sink}
}

func (s *Service) SaveManifest(ctx context.Context, req SaveRequest) (ManualAssetManifest, *artifact.Artifact, error) {
	if s == nil || s.sink == nil {
		return ManualAssetManifest{}, nil, fmt.Errorf("artifact sink is required")
	}
	manifest, err := BuildManualAssetManifest(req.Import)
	if err != nil {
		return ManualAssetManifest{}, nil, err
	}
	artifactReq, err := BuildArtifactRequest(req.ProjectID, req.WorkflowRunID, req.TaskID, manifest)
	if err != nil {
		return ManualAssetManifest{}, nil, err
	}
	record, err := s.sink.CreateArtifact(ctx, artifactReq)
	if err != nil {
		return ManualAssetManifest{}, nil, err
	}
	return manifest, record, nil
}

func BuildManualAssetManifest(req ImportRequest) (ManualAssetManifest, error) {
	if !validAssetType(req.Type) {
		return ManualAssetManifest{}, fmt.Errorf("unsupported asset type %q", req.Type)
	}
	req.StorageType = strings.TrimSpace(req.StorageType)
	if req.StorageType == "" {
		req.StorageType = artifact.StorageLocal
	}
	req.StorageRef = strings.TrimSpace(req.StorageRef)
	if req.StorageRef == "" {
		return ManualAssetManifest{}, fmt.Errorf("storageRef is required")
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "manual_upload"
	}
	return ManualAssetManifest{
		AssetID:       stableAssetID(req.Type, req.StorageRef),
		Type:          req.Type,
		StorageType:   req.StorageType,
		StorageRef:    req.StorageRef,
		DurationSec:   req.DurationSec,
		Description:   strings.TrimSpace(req.Description),
		Tags:          cleanTags(req.Tags),
		RelatedShotID: strings.TrimSpace(req.RelatedShotID),
		Source:        source,
	}, nil
}

func BuildArtifactRequest(projectID, workflowRunID, taskID string, manifest ManualAssetManifest) (*artifact.CreateArtifactRequest, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return &artifact.CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		TaskID:        taskID,
		StageName:     "asset_manifest",
		UnitID:        "asset_manifest",
		Kind:          artifact.KindJSON,
		Name:          "asset_manifest.json",
		StorageType:   artifact.StorageLocal,
		Data:          data,
		MimeType:      "application/json",
		SizeBytes:     int64(len(data)),
		Provider:      "manual-asset-manifest",
		Metadata: map[string]interface{}{
			"artifactType": "asset_manifest",
			"assetId":      manifest.AssetID,
			"assetType":    string(manifest.Type),
		},
	}, nil
}

func validAssetType(value AssetType) bool {
	return value == AssetTypeImage || value == AssetTypeAudio || value == AssetTypeVideo
}

func stableAssetID(assetType AssetType, storageRef string) string {
	sum := sha1.Sum([]byte(string(assetType) + "|" + storageRef))
	return "asset_" + hex.EncodeToString(sum[:])[:16]
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}
