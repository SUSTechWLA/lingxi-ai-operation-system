package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
)

type ContextService struct {
	contextRepo *repository.ContextRepository
	nodeRepo    *repository.NodeRepository
	taskRepo    *repository.TaskRepository
}

func NewContextService(
	contextRepo *repository.ContextRepository,
	nodeRepo *repository.NodeRepository,
	taskRepo *repository.TaskRepository,
) *ContextService {
	return &ContextService{
		contextRepo: contextRepo,
		nodeRepo:    nodeRepo,
		taskRepo:    taskRepo,
	}
}

func (s *ContextService) RecordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, message string, metadata map[string]interface{}) error {
	c := &model.Context{
		ContextType: ctxType,
		TaskID:      taskID,
		NodeID:      nodeID,
		Message:     message,
		Metadata:    metadata,
	}
	return s.contextRepo.Save(ctx, c)
}

func (s *ContextService) GetContextForTask(ctx context.Context, taskID string) ([]*model.Context, error) {
	return s.contextRepo.FindByTaskID(ctx, taskID)
}

func (s *ContextService) GetLatestSnapshotForNode(ctx context.Context, nodeID string) (map[string]interface{}, error) {
	c, err := s.contextRepo.FindLatestSnapshotByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	return c.SnapshotData, nil
}

func (s *ContextService) RestoreNodeFromSnapshot(ctx context.Context, nodeID string) (map[string]interface{}, error) {
	return s.GetLatestSnapshotForNode(ctx, nodeID)
}

func (s *ContextService) HandleEvent(ctx context.Context, event eventbus.Event) error {
	zap.L().Info("ContextService handling event",
		zap.String("taskId", event.TaskID),
		zap.String("nodeId", event.NodeID),
		zap.String("status", event.Status),
	)

	var ctxType model.ContextType
	var message string

	switch event.Status {
	case "SUCCESS":
		ctxType = model.ContextNodeSuccess
		message = "Node succeeded"
	case "FAILED":
		ctxType = model.ContextNodeFailed
		message = fmt.Sprintf("Node failed: %s", event.ErrorMessage)
	case "RUNNING":
		ctxType = model.ContextNodeScheduled
		message = "Node running"
	default:
		return nil
	}

	return s.RecordContext(ctx, event.TaskID, event.NodeID, ctxType, message, nil)
}
