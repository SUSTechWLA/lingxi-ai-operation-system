package artifact

import (
	"context"
	"fmt"

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
	// Compute content hash if not provided
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

	// Handle inline vs MinIO storage
	storageType := req.StorageType
	if storageType == "" {
		storageType = "inline"
	}

	var inlineJSON string
	var storageRef string
	var sizeBytes int64

	if storageType == "inline" && len(req.Data) > 0 {
		inlineJSON = string(req.Data)
		sizeBytes = int64(len(req.Data))
	} else if storageType == "minio" {
		storageRef = req.StorageRef
		sizeBytes = int64(len(req.Data))
	}

	artifact := &Artifact{
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
		Metadata:      req.Metadata,
	}

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

// GetHistory returns all versions of an artifact, newest first.
func (s *Service) GetHistory(ctx context.Context, projectID, stageName, unitID string) ([]*Artifact, error) {
	return s.repo.FindHistory(ctx, projectID, stageName, unitID)
}

// ListByProject returns all current artifacts for a project.
func (s *Service) ListByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	return s.repo.ListByProject(ctx, projectID)
}
