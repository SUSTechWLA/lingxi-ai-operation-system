package artifact

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// Service provides business logic for artifact management.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateArtifact creates a new artifact version. If a version with the same
// content hash already exists for the same scope, it returns the existing one
// (idempotent). Otherwise, it auto-increments the version number and saves.
func (s *Service) CreateArtifact(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error) {
	if req.ContentHash == "" && len(req.Data) > 0 {
		req.ContentHash = HashContent(req.Data)
	}

	// Check for idempotent duplicate (same content hash = same result)
	if req.ContentHash != "" {
		existing, err := s.repo.FindByHash(ctx, req.ProjectID, req.StageName, req.UnitID, req.ContentHash)
		if err == nil && existing != nil {
			zap.L().Debug("Artifact already exists (idempotent)", zap.String("hash", req.ContentHash))
			return existing, nil
		}
	}

	// Determine the next version number
	current, err := s.repo.FindCurrent(ctx, req.ProjectID, req.StageName, req.UnitID)
	nextVersion := 1
	var parentID string
	if err == nil && current != nil {
		nextVersion = current.Version + 1
		parentID = current.ID
	}

	artifact := buildArtifactRecord(req, nextVersion, parentID)

	if err := s.repo.Save(ctx, artifact); err != nil {
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	zap.L().Info("Artifact created",
		zap.String("id", artifact.ID),
		zap.String("stage", artifact.StageName),
		zap.Int("version", artifact.Version),
	)
	return artifact, nil
}

// GetCurrent returns the current version of an artifact for the given scope.
func (s *Service) GetCurrent(ctx context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	return s.repo.FindCurrent(ctx, projectID, stageName, unitID)
}

// GetByID returns an artifact by ID.
func (s *Service) GetByID(ctx context.Context, id string) (*Artifact, error) {
	return s.repo.FindByID(ctx, id)
}

// GetHistory returns all versions of an artifact, newest first.
func (s *Service) GetHistory(ctx context.Context, projectID, stageName, unitID string) ([]*Artifact, error) {
	return s.repo.FindHistory(ctx, projectID, stageName, unitID)
}

// ListByProject returns all current artifacts for a project.
func (s *Service) ListByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return s.repo.ListByProject(ctx, projectID)
}

func buildArtifactRecord(req *CreateArtifactRequest, nextVersion int, parentID string) *Artifact {
	if req.ContentHash == "" && len(req.Data) > 0 {
		req.ContentHash = HashContent(req.Data)
	}

	metadata := cloneMetadata(req.Metadata)
	metadata["cloudPayloadStored"] = false
	metadata["localOnly"] = true
	if _, exists := metadata["requestedStorageType"]; !exists && req.StorageType != "" && req.StorageType != StorageLocal {
		metadata["requestedStorageType"] = req.StorageType
	}

	storageRef := req.StorageRef
	if strings.TrimSpace(storageRef) == "" {
		storageRef = LocalArtifactRef(req.ProjectID, req.StageName, req.UnitID, req.ContentHash, req.Name)
	}

	sizeBytes := req.SizeBytes
	if sizeBytes == 0 && len(req.Data) > 0 {
		sizeBytes = int64(len(req.Data))
	}

	// For artifacts materialized from workflow nodes, store inline content so
	// it can be displayed before the local backend syncs. The storage type is
	// set to "inline" to bypass the StorageLocal placeholder in artifactContent.
	storageType := StorageLocal
	inlineJSON := ""
	if len(req.Data) > 0 && req.Provider == "workflow-node" {
		storageType = "inline"
		inlineJSON = string(req.Data)
		metadata["cloudPayloadStored"] = true
		metadata["localOnly"] = false
		metadata["contentAvailability"] = "cloud-inline"
	}

	return &Artifact{
		ProjectID:     req.ProjectID,
		WorkflowRunID: req.WorkflowRunID,
		StageName:     req.StageName,
		UnitID:        req.UnitID,
		Kind:          req.Kind,
		Name:          req.Name,
		Version:       nextVersion,
		ParentID:      parentID,
		StorageType:   storageType,
		StorageRef:    storageRef,
		InlineJSON:    inlineJSON,
		MimeType:      req.MimeType,
		SizeBytes:     sizeBytes,
		ContentHash:   req.ContentHash,
		PromptHash:    req.PromptHash,
		Provider:      req.Provider,
		Model:         req.Model,
		IsCurrent:     true,
		Metadata:      metadata,
	}
}

func LocalArtifactRef(projectID, stageName, unitID, contentHash, name string) string {
	hash := safeStorageSegment(contentHash)
	if hash == "" {
		hash = "pending"
	}
	fileName := safeStorageSegment(name)
	if fileName == "" {
		fileName = "artifact"
	}
	return "local://projects/" + path.Join(
		safeStorageSegment(projectID),
		"artifacts",
		safeStorageSegment(stageName),
		safeStorageSegment(unitID),
		hash,
		fileName,
	)
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	cloned := map[string]interface{}{}
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

var unsafeStorageSegmentPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeStorageSegment(value string) string {
	cleaned := unsafeStorageSegmentPattern.ReplaceAllString(strings.TrimSpace(value), "-")
	cleaned = strings.Trim(cleaned, ".-_")
	if cleaned == "" {
		return ""
	}
	return cleaned
}
