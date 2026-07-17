package assets

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

type AssetType string

const (
	AssetTypeImage AssetType = "image"
	AssetTypeAudio AssetType = "audio"
	AssetTypeVideo AssetType = "video"
)

const (
	MaxExternalGenerationReferenceImages = 6
	MaxExternalGenerationPromptChars     = 2000
)

type ReferenceRole string

const (
	ReferenceRoleCharacter  ReferenceRole = "character"
	ReferenceRoleProp       ReferenceRole = "prop"
	ReferenceRoleScene      ReferenceRole = "scene"
	ReferenceRoleStoryboard ReferenceRole = "storyboard"
	ReferenceRoleKeyframe   ReferenceRole = "keyframe"
	ReferenceRoleUploaded   ReferenceRole = "uploaded"
	ReferenceRoleOther      ReferenceRole = "other"
)

type ExternalGenerationStatus string

const (
	ExternalGenerationStatusPendingUpload ExternalGenerationStatus = "pending_upload"
	ExternalGenerationStatusFulfilled     ExternalGenerationStatus = "fulfilled"
)

type ImportRequest struct {
	Type                AssetType `json:"type"`
	StorageType         string    `json:"storageType"`
	StorageRef          string    `json:"storageRef"`
	MimeType            string    `json:"mimeType,omitempty"`
	SizeBytes           int64     `json:"sizeBytes,omitempty"`
	ContentHash         string    `json:"contentHash,omitempty"`
	PromptHash          string    `json:"promptHash,omitempty"`
	DurationSec         int       `json:"durationSec,omitempty"`
	Description         string    `json:"description,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	RelatedShotID       string    `json:"relatedShotId,omitempty"`
	Source              string    `json:"source,omitempty"`
	GenerationRequestID string    `json:"generationRequestId,omitempty"`
	ExternalPlatform    string    `json:"externalPlatform,omitempty"`
	ReferenceAssetIDs   []string  `json:"referenceAssetIds,omitempty"`
}

type ManualAssetManifest struct {
	AssetID             string    `json:"assetId"`
	Type                AssetType `json:"type"`
	StorageType         string    `json:"storageType"`
	StorageRef          string    `json:"storageRef"`
	MimeType            string    `json:"mimeType,omitempty"`
	SizeBytes           int64     `json:"sizeBytes,omitempty"`
	ContentHash         string    `json:"contentHash,omitempty"`
	PromptHash          string    `json:"promptHash,omitempty"`
	DurationSec         int       `json:"durationSec,omitempty"`
	Description         string    `json:"description,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	RelatedShotID       string    `json:"relatedShotId,omitempty"`
	Source              string    `json:"source"`
	GenerationRequestID string    `json:"generationRequestId,omitempty"`
	ExternalPlatform    string    `json:"externalPlatform,omitempty"`
	ReferenceAssetIDs   []string  `json:"referenceAssetIds,omitempty"`
}

type ExternalGenerationTarget struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	DurationSec int    `json:"durationSec,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
}

type ReferenceImageInput struct {
	ID         string        `json:"id,omitempty"`
	Label      string        `json:"label,omitempty"`
	Role       ReferenceRole `json:"role,omitempty"`
	StorageRef string        `json:"storageRef"`
	ArtifactID string        `json:"artifactId,omitempty"`
}

type ReferenceImage struct {
	ID         string        `json:"id"`
	Label      string        `json:"label,omitempty"`
	Role       ReferenceRole `json:"role"`
	StorageRef string        `json:"storageRef"`
	ArtifactID string        `json:"artifactId,omitempty"`
}

type ExternalGenerationInput struct {
	Kind       AssetType             `json:"kind"`
	ProjectID  string                `json:"projectId,omitempty"`
	ShotID     string                `json:"shotId,omitempty"`
	StageName  string                `json:"stageName,omitempty"`
	Prompt     string                `json:"prompt"`
	Negative   string                `json:"negativePrompt,omitempty"`
	References []ReferenceImageInput `json:"references,omitempty"`
	Target     ExternalGenerationTarget
}

type ExternalGenerationRequest struct {
	RequestID           string                   `json:"requestId"`
	Kind                AssetType                `json:"kind"`
	ProjectID           string                   `json:"projectId,omitempty"`
	ShotID              string                   `json:"shotId,omitempty"`
	StageName           string                   `json:"stageName,omitempty"`
	Prompt              string                   `json:"prompt"`
	NegativePrompt      string                   `json:"negativePrompt,omitempty"`
	References          []ReferenceImage         `json:"references,omitempty"`
	Target              ExternalGenerationTarget `json:"target,omitempty"`
	PromptCharLimit     int                      `json:"promptCharLimit"`
	ReferenceImageLimit int                      `json:"referenceImageLimit"`
	Status              ExternalGenerationStatus `json:"status"`
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
		AssetID:             stableAssetID(req.Type, req.StorageRef),
		Type:                req.Type,
		StorageType:         req.StorageType,
		StorageRef:          req.StorageRef,
		MimeType:            strings.TrimSpace(req.MimeType),
		SizeBytes:           req.SizeBytes,
		ContentHash:         strings.TrimSpace(req.ContentHash),
		PromptHash:          strings.TrimSpace(req.PromptHash),
		DurationSec:         req.DurationSec,
		Description:         strings.TrimSpace(req.Description),
		Tags:                cleanTags(req.Tags),
		RelatedShotID:       strings.TrimSpace(req.RelatedShotID),
		Source:              source,
		GenerationRequestID: strings.TrimSpace(req.GenerationRequestID),
		ExternalPlatform:    strings.TrimSpace(req.ExternalPlatform),
		ReferenceAssetIDs:   cleanTags(req.ReferenceAssetIDs),
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

func BuildExternalGenerationRequest(input ExternalGenerationInput) (ExternalGenerationRequest, error) {
	if input.Kind != AssetTypeImage && input.Kind != AssetTypeVideo {
		return ExternalGenerationRequest{}, fmt.Errorf("external generation kind must be image or video")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return ExternalGenerationRequest{}, fmt.Errorf("prompt is required")
	}
	if len([]rune(prompt)) > MaxExternalGenerationPromptChars {
		return ExternalGenerationRequest{}, fmt.Errorf("prompt exceeds %d characters", MaxExternalGenerationPromptChars)
	}
	if len(input.References) > MaxExternalGenerationReferenceImages {
		return ExternalGenerationRequest{}, fmt.Errorf("reference images exceed %d", MaxExternalGenerationReferenceImages)
	}
	references := make([]ReferenceImage, 0, len(input.References))
	for i, ref := range input.References {
		storageRef := strings.TrimSpace(ref.StorageRef)
		if storageRef == "" {
			return ExternalGenerationRequest{}, fmt.Errorf("reference image %d storageRef is required", i+1)
		}
		role := ref.Role
		if role == "" {
			role = ReferenceRoleOther
		}
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			id = stableReferenceID(role, storageRef)
		}
		references = append(references, ReferenceImage{
			ID:         id,
			Label:      strings.TrimSpace(ref.Label),
			Role:       role,
			StorageRef: storageRef,
			ArtifactID: strings.TrimSpace(ref.ArtifactID),
		})
	}
	request := ExternalGenerationRequest{
		Kind:                input.Kind,
		ProjectID:           strings.TrimSpace(input.ProjectID),
		ShotID:              strings.TrimSpace(input.ShotID),
		StageName:           strings.TrimSpace(input.StageName),
		Prompt:              prompt,
		NegativePrompt:      strings.TrimSpace(input.Negative),
		References:          references,
		Target:              cleanExternalGenerationTarget(input.Target),
		PromptCharLimit:     MaxExternalGenerationPromptChars,
		ReferenceImageLimit: MaxExternalGenerationReferenceImages,
		Status:              ExternalGenerationStatusPendingUpload,
	}
	request.RequestID = stableExternalGenerationRequestID(request)
	return request, nil
}

func BuildExternalGenerationArtifactRequest(projectID, workflowRunID, taskID string, request ExternalGenerationRequest) (*artifact.CreateArtifactRequest, error) {
	if strings.TrimSpace(request.RequestID) == "" {
		return nil, fmt.Errorf("requestId is required")
	}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return nil, err
	}
	return &artifact.CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		TaskID:        taskID,
		StageName:     "external_generation_request",
		UnitID:        request.RequestID,
		Kind:          artifact.KindJSON,
		Name:          "external_generation_request.json",
		StorageType:   artifact.StorageInline,
		Data:          data,
		MimeType:      "application/json",
		SizeBytes:     int64(len(data)),
		Provider:      "external-generation-request",
		Metadata: map[string]interface{}{
			"artifactType":        "external_generation_request",
			"generationKind":      string(request.Kind),
			"relatedShotId":       request.ShotID,
			"promptCharLimit":     request.PromptCharLimit,
			"referenceImageLimit": request.ReferenceImageLimit,
			"status":              string(request.Status),
		},
	}, nil
}

func BuildExternalGenerationResultArtifactRequest(projectID, workflowRunID, taskID string, manifest ManualAssetManifest) (*artifact.CreateArtifactRequest, error) {
	kind, name, err := generatedResultArtifactKindAndName(manifest)
	if err != nil {
		return nil, err
	}
	unitID := strings.TrimSpace(manifest.GenerationRequestID)
	if unitID == "" {
		unitID = strings.TrimSpace(manifest.RelatedShotID)
	}
	if unitID == "" {
		unitID = manifest.AssetID
	}
	provenance := manualAssetProvenance(manifest)
	return &artifact.CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		TaskID:        taskID,
		StageName:     "external_generation_result",
		UnitID:        unitID,
		Kind:          kind,
		Name:          name,
		StorageType:   artifact.StorageLocal,
		StorageRef:    manifest.StorageRef,
		MimeType:      manifest.MimeType,
		SizeBytes:     manifest.SizeBytes,
		ContentHash:   manifest.ContentHash,
		PromptHash:    manifest.PromptHash,
		Provider:      "external-generation-upload",
		Metadata: map[string]interface{}{
			"schemaVersion":               1,
			"artifactType":                "external_generation_result",
			"assetId":                     manifest.AssetID,
			"assetType":                   string(manifest.Type),
			"relatedShotId":               manifest.RelatedShotID,
			"externalGenerationRequestId": manifest.GenerationRequestID,
			"externalPlatform":            manifest.ExternalPlatform,
			"source":                      manifest.Source,
			"referenceAssetIds":           manifest.ReferenceAssetIDs,
			"description":                 manifest.Description,
			"tags":                        manifest.Tags,
			"sourceType":                  provenance["sourceType"],
			"providerName":                provenance["providerName"],
			"providerJobId":               provenance["providerJobId"],
			"fallbackReason":              provenance["fallbackReason"],
			"isFallback":                  provenance["isFallback"],
			"generatedAt":                 provenance["generatedAt"],
			"inputPromptHash":             provenance["inputPromptHash"],
			"sourceArtifactIds":           provenance["sourceArtifactIds"],
			"provenance":                  provenance,
		},
	}, nil
}

func manualAssetProvenance(manifest ManualAssetManifest) map[string]interface{} {
	providerName := strings.TrimSpace(manifest.ExternalPlatform)
	if providerName == "" {
		providerName = strings.TrimSpace(manifest.Source)
	}
	if providerName == "" {
		providerName = "external-generation-upload"
	}
	sourceArtifactIDs := make([]interface{}, 0, len(manifest.ReferenceAssetIDs))
	for _, id := range manifest.ReferenceAssetIDs {
		if strings.TrimSpace(id) != "" {
			sourceArtifactIDs = append(sourceArtifactIDs, strings.TrimSpace(id))
		}
	}
	return map[string]interface{}{
		"schemaVersion":     1,
		"sourceType":        "uploaded",
		"providerName":      providerName,
		"providerJobId":     strings.TrimSpace(manifest.GenerationRequestID),
		"fallbackReason":    "",
		"isFallback":        false,
		"generatedAt":       time.Now().UTC().Format(time.RFC3339),
		"inputPromptHash":   strings.TrimSpace(manifest.PromptHash),
		"sourceArtifactIds": sourceArtifactIDs,
	}
}

func validAssetType(value AssetType) bool {
	return value == AssetTypeImage || value == AssetTypeAudio || value == AssetTypeVideo
}

func stableAssetID(assetType AssetType, storageRef string) string {
	sum := sha1.Sum([]byte(string(assetType) + "|" + storageRef))
	return "asset_" + hex.EncodeToString(sum[:])[:16]
}

func stableReferenceID(role ReferenceRole, storageRef string) string {
	sum := sha1.Sum([]byte(string(role) + "|" + storageRef))
	return "ref_" + hex.EncodeToString(sum[:])[:16]
}

func stableExternalGenerationRequestID(request ExternalGenerationRequest) string {
	parts := []string{
		string(request.Kind),
		request.ProjectID,
		request.ShotID,
		request.StageName,
		request.Prompt,
		request.NegativePrompt,
		request.Target.AspectRatio,
		request.Target.Resolution,
		fmt.Sprintf("%d", request.Target.DurationSec),
	}
	for _, ref := range request.References {
		parts = append(parts, ref.ID, string(ref.Role), ref.StorageRef)
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "extgen_" + hex.EncodeToString(sum[:])[:16]
}

func cleanExternalGenerationTarget(target ExternalGenerationTarget) ExternalGenerationTarget {
	return ExternalGenerationTarget{
		AspectRatio: strings.TrimSpace(target.AspectRatio),
		DurationSec: target.DurationSec,
		Resolution:  strings.TrimSpace(target.Resolution),
	}
}

func generatedResultArtifactKindAndName(manifest ManualAssetManifest) (artifact.ArtifactKind, string, error) {
	switch manifest.Type {
	case AssetTypeImage:
		return artifact.KindImage, "external_generation_result.png", nil
	case AssetTypeVideo:
		return artifact.KindVideo, "external_generation_result.mp4", nil
	default:
		return "", "", fmt.Errorf("external generation result type must be image or video")
	}
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
