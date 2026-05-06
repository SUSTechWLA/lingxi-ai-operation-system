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
	contextRepo repository.ContextRepo
	nodeRepo    repository.NodeRepo
	taskRepo    repository.TaskRepo
}

func NewContextService(
	contextRepo repository.ContextRepo,
	nodeRepo repository.NodeRepo,
	taskRepo repository.TaskRepo,
) *ContextService {
	return &ContextService{
		contextRepo: contextRepo,
		nodeRepo:    nodeRepo,
		taskRepo:    taskRepo,
	}
}

func (s *ContextService) RecordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) error {
	c := &model.Context{
		ContextType:  ctxType,
		TaskID:       taskID,
		NodeID:       nodeID,
		SourceModule: sourceModule,
		Message:      message,
		Metadata:     metadata,
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
		zap.String("topic", event.Topic),
		zap.String("taskId", event.TaskID),
		zap.String("nodeId", event.NodeID),
		zap.String("status", event.Status),
	)

	var ctxType model.ContextType
	var message string

	switch event.Status {
	case "SUCCESS":
		ctxType = model.ContextNodeSuccess
		message = "Kafka 事件记录：节点执行成功"
	case "FAILED":
		ctxType = model.ContextNodeFailed
		message = fmt.Sprintf("Kafka 事件记录：节点执行失败: %s", event.ErrorMessage)
	case "RUNNING":
		ctxType = model.ContextNodeScheduled
		message = "Kafka 事件记录：节点进入运行状态"
	default:
		return nil
	}

	metadata := buildExecutionMetadata(event.Output)
	c := &model.Context{
		ContextType:  ctxType,
		TaskID:       event.TaskID,
		NodeID:       event.NodeID,
		SourceModule: "ContextService",
		SourceTopic:  event.Topic,
		Message:      message,
		Metadata:     metadata,
	}
	return s.contextRepo.Save(ctx, c)
}

func buildExecutionMetadata(output map[string]interface{}) map[string]interface{} {
	if len(output) == 0 {
		return nil
	}
	meta := make(map[string]interface{})
	for _, key := range []string{"startedAt", "durationMs", "exitCode", "error", "resourceUsage"} {
		if v, ok := output[key]; ok {
			meta[key] = v
		}
	}
	if len(meta) > 0 {
		return meta
	}
	return nil
}
