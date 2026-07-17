package artifact

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

type ArtifactCreator interface {
	CreateArtifact(ctx context.Context, req *CreateArtifactRequest) (*Artifact, error)
}

type ArtifactSyncService struct {
	artifactService ArtifactCreator
	logger          *zap.Logger
}

func NewArtifactSyncService(artifactService ArtifactCreator, logger *zap.Logger) *ArtifactSyncService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ArtifactSyncService{
		artifactService: artifactService,
		logger:          logger,
	}
}

func (s *ArtifactSyncService) SyncFromNodeOutput(
	ctx context.Context,
	projectID string,
	workflowRunID string,
	taskID string,
	node *model.Node,
) ([]*Artifact, error) {
	if s == nil || s.artifactService == nil || node == nil {
		return nil, nil
	}

	requests, err := BuildArtifactRequestsFromNodeChecked(projectID, workflowRunID, node)
	if err != nil {
		return nil, err
	}

	createdArtifacts := make([]*Artifact, 0, len(requests))
	for _, req := range requests {
		req.ProjectID = projectID
		req.WorkflowRunID = workflowRunID
		req.TaskID = taskID
		if req.Metadata == nil {
			req.Metadata = map[string]interface{}{}
		}
		req.Metadata["taskId"] = taskID
		if node.ID != "" {
			req.Metadata["producedByNode"] = node.ID
		}

		created, err := s.artifactService.CreateArtifact(ctx, req)
		if err != nil {
			s.logger.Warn("artifact sync: failed to create artifact",
				zap.String("projectID", projectID),
				zap.String("workflowRunID", workflowRunID),
				zap.String("taskID", taskID),
				zap.String("nodeID", node.ID),
				zap.String("kind", string(req.Kind)),
				zap.String("unitID", req.UnitID),
				zap.String("storageRef", req.StorageRef),
				zap.Error(err),
			)
			if IsCriticalArtifactKind(req.Kind) {
				return createdArtifacts, fmt.Errorf(
					"CRITICAL_ARTIFACT_SYNC_FAILED: critical artifact sync failed: projectID=%s workflowRunID=%s taskID=%s nodeID=%s kind=%s unitID=%s: %w",
					projectID,
					workflowRunID,
					taskID,
					node.ID,
					req.Kind,
					req.UnitID,
					err,
				)
			}
			continue
		}

		s.logger.Info("artifact sync: created artifact",
			zap.String("projectID", projectID),
			zap.String("workflowRunID", workflowRunID),
			zap.String("taskID", taskID),
			zap.String("nodeID", node.ID),
			zap.String("artifactID", created.ID),
			zap.String("kind", string(created.Kind)),
			zap.String("unitID", created.UnitID),
		)
		createdArtifacts = append(createdArtifacts, created)
	}
	return createdArtifacts, nil
}
