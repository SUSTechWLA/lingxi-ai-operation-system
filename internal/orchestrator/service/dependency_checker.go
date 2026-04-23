package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
)

type DependencyChecker struct {
	nodeRepo     *repository.NodeRepository
	stateService *StateService
	producer     *eventbus.Producer
}

func NewDependencyChecker(
	nodeRepo *repository.NodeRepository,
	stateService *StateService,
	producer *eventbus.Producer,
) *DependencyChecker {
	return &DependencyChecker{
		nodeRepo:     nodeRepo,
		stateService: stateService,
		producer:     producer,
	}
}

func (dc *DependencyChecker) OnNodeExecuted(ctx context.Context, nodeID, taskID string) {
	zap.L().Info("DependencyChecker: processing node executed event",
		zap.String("nodeId", nodeID),
		zap.String("taskId", taskID),
	)

	childNodes, err := dc.nodeRepo.FindChildNodes(ctx, nodeID)
	if err != nil {
		zap.L().Error("Failed to find child nodes", zap.Error(err))
		return
	}

	zap.L().Info("Found child nodes", zap.Int("count", len(childNodes)), zap.String("parent", nodeID))

	for _, child := range childNodes {
		if child.Status == model.NodeCreated || child.Status == model.NodeRetrying {
			met, err := dc.stateService.CheckDependenciesMet(ctx, child.ID)
			if err != nil {
				zap.L().Error("Failed to check dependencies", zap.Error(err))
				continue
			}
			if met {
				if err := dc.stateService.InitializeNodeReady(ctx, child); err != nil {
					zap.L().Error("Failed to initialize node as ready", zap.Error(err))
					continue
				}
				_ = dc.producer.Publish(eventbus.TopicNodeReady, child.IdempotencyKey, eventbus.Event{
					TaskID:         child.TaskID,
					NodeID:         child.ID,
					Type:           string(child.Type),
					Payload:        child.Input,
					TraceID:        child.TaskID + "-" + child.ID,
					IdempotencyKey: child.IdempotencyKey,
				})
				zap.L().Info("Child node is now READY and published", zap.String("nodeId", child.ID))
			}
		}
	}

	completed, _ := dc.stateService.CheckTaskCompleted(ctx, taskID)
	if completed {
		_ = dc.stateService.TransitionTask(ctx, taskID, model.TaskSuccess)
		zap.L().Info("Task completed successfully", zap.String("taskId", taskID))
	}
}
