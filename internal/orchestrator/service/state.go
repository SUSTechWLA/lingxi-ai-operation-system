package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
)

type StateService struct {
	nodeRepo    *repository.NodeRepository
	taskRepo    *repository.TaskRepository
	depRepo     *repository.NodeDependencyRepository
	contextRepo *repository.ContextRepository
	producer    *eventbus.Producer
	retryPolicy *RetryPolicy
}

func NewStateService(
	nodeRepo *repository.NodeRepository,
	taskRepo *repository.TaskRepository,
	depRepo *repository.NodeDependencyRepository,
	contextRepo *repository.ContextRepository,
	producer *eventbus.Producer,
) *StateService {
	return &StateService{
		nodeRepo:    nodeRepo,
		taskRepo:    taskRepo,
		depRepo:     depRepo,
		contextRepo: contextRepo,
		producer:    producer,
		retryPolicy: NewRetryPolicy(),
	}
}

func (s *StateService) TransitionNode(ctx context.Context, nodeID string, newStatus model.NodeStatus, output map[string]interface{}, errMsg string) (*model.Node, error) {
	node, err := s.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	oldStatus := node.Status
	zap.L().Info("Node transitioning",
		zap.String("nodeId", nodeID),
		zap.String("oldStatus", string(oldStatus)),
		zap.String("newStatus", string(newStatus)),
	)

	node.Status = newStatus
	if output != nil {
		node.Output = output
	}
	if errMsg != "" {
		node.ErrorMessage = errMsg
	}

	if err := s.nodeRepo.Save(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to save node: %w", err)
	}

	s.recordContextForTransition(ctx, node, newStatus)

	return node, nil
}

func (s *StateService) TransitionTask(ctx context.Context, taskID string, newStatus model.TaskStatus) error {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	oldStatus := task.Status
	zap.L().Info("Task transitioning",
		zap.String("taskId", taskID),
		zap.String("oldStatus", string(oldStatus)),
		zap.String("newStatus", string(newStatus)),
	)

	task.Status = newStatus
	if err := s.taskRepo.Save(ctx, task); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	switch newStatus {
	case model.TaskSuccess:
		s.recordContext(ctx, taskID, "", model.ContextTaskSuccess, "Task completed successfully", nil)
		_ = s.producer.Publish(eventbus.TopicTaskCompleted, taskID, eventbus.Event{
			TaskID: taskID, Status: "SUCCESS",
		})
	case model.TaskFailed:
		s.recordContext(ctx, taskID, "", model.ContextTaskFailed, "Task failed", nil)
		_ = s.producer.Publish(eventbus.TopicTaskFailed, taskID, eventbus.Event{
			TaskID: taskID, Status: "FAILED",
		})
	case model.TaskPaused:
		zap.L().Info("Task paused", zap.String("taskId", taskID))
	case model.TaskRunning:
		if oldStatus == model.TaskPaused {
			zap.L().Info("Task resumed", zap.String("taskId", taskID))
		}
	}

	return nil
}

func (s *StateService) CheckDependenciesMet(ctx context.Context, nodeID string) (bool, error) {
	deps, err := s.depRepo.FindByChildID(ctx, nodeID)
	if err != nil {
		return false, err
	}

	for _, dep := range deps {
		parent, err := s.nodeRepo.FindByID(ctx, dep.ParentNodeID)
		if err != nil || parent == nil || parent.Status != model.NodeSuccess {
			return false, nil
		}
	}

	return true, nil
}

func (s *StateService) CheckTaskCompleted(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status != model.NodeSuccess {
			return false, nil
		}
	}
	return true, nil
}

func (s *StateService) CheckTaskFailed(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status == model.NodeFailed && n.RetryCount >= n.MaxRetry {
			return true, nil
		}
	}
	return false, nil
}

func (s *StateService) HasRetryableNodes(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status == model.NodeFailed && s.retryPolicy.ShouldRetry(n.RetryCount, n.MaxRetry) {
			return true, nil
		}
	}
	return false, nil
}

func (s *StateService) InitializeNodeReady(ctx context.Context, node *model.Node) error {
	if node.Status == model.NodeCreated {
		met, err := s.CheckDependenciesMet(ctx, node.ID)
		if err != nil {
			return err
		}
		if met {
			node.Status = model.NodeReady
			if node.IdempotencyKey == "" {
				node.IdempotencyKey = node.TaskID + "-" + node.ID
			}
			if err := s.nodeRepo.Save(ctx, node); err != nil {
				return err
			}

			s.recordContext(ctx, node.TaskID, node.ID, model.ContextNodeReady, "Node is READY", nil)

			_ = s.producer.Publish(eventbus.TopicNodeReady, node.IdempotencyKey, eventbus.Event{
				TaskID:         node.TaskID,
				NodeID:         node.ID,
				Type:           string(node.Type),
				Payload:        node.Input,
				TraceID:        node.TaskID + "-" + node.ID,
				IdempotencyKey: node.IdempotencyKey,
			})

			zap.L().Info("Node is READY (dependencies met)", zap.String("nodeId", node.ID))
		}
	}
	return nil
}

func (s *StateService) recordContextForTransition(ctx context.Context, node *model.Node, newStatus model.NodeStatus) {
	var ctxType model.ContextType
	var message string

	switch newStatus {
	case model.NodeReady:
		ctxType = model.ContextNodeReady
		message = "Node ready"
	case model.NodeRunning:
		ctxType = model.ContextNodeScheduled
		message = "Node scheduled"
	case model.NodeSuccess:
		ctxType = model.ContextNodeSuccess
		message = "Node succeeded"
	case model.NodeFailed:
		ctxType = model.ContextNodeFailed
		message = "Node failed"
	case model.NodeRetrying:
		ctxType = model.ContextNodeRetry
		message = "Node retrying"
	default:
		return
	}

	s.recordContext(ctx, node.TaskID, node.ID, ctxType, message, nil)
}

func (s *StateService) recordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, message string, metadata map[string]interface{}) {
	c := &model.Context{
		ContextType: ctxType,
		TaskID:      taskID,
		NodeID:      nodeID,
		Message:     message,
		Metadata:    metadata,
	}
	if err := s.contextRepo.Save(ctx, c); err != nil {
		zap.L().Warn("Failed to record context", zap.Error(err))
	}
}
