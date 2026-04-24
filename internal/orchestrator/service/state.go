package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/outbox"
)

type StateService struct {
	nodeRepo          repository.NodeRepo
	taskRepo          repository.TaskRepo
	depRepo           repository.DependencyRepo
	contextRepo       repository.ContextRepo
	eventSaver        outbox.EventSaver
	retryPolicy       *RetryPolicy
	dependencyChecker *DependencyChecker
}

func NewStateService(
	nodeRepo repository.NodeRepo,
	taskRepo repository.TaskRepo,
	depRepo repository.DependencyRepo,
	contextRepo repository.ContextRepo,
	eventSaver outbox.EventSaver,
) *StateService {
	return &StateService{
		nodeRepo:    nodeRepo,
		taskRepo:    taskRepo,
		depRepo:     depRepo,
		contextRepo: contextRepo,
		eventSaver:  eventSaver,
		retryPolicy: NewRetryPolicy(),
	}
}

func (s *StateService) SetDependencyChecker(dc *DependencyChecker) {
	s.dependencyChecker = dc
}

func (s *StateService) TransitionNode(ctx context.Context, nodeID string, newStatus model.NodeStatus, output map[string]interface{}, errMsg string) (*model.Node, error) {
	node, err := s.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	zap.L().Info("Node transitioning",
		zap.String("nodeId", nodeID),
		zap.String("oldStatus", string(node.Status)),
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

	// Event-driven: immediately try to make CREATED nodes ready
	if newStatus == model.NodeCreated {
		s.TryMakeReady(ctx, node)
	}

	return node, nil
}

// TryMakeReady immediately checks dependencies and transitions to READY if met
func (s *StateService) TryMakeReady(ctx context.Context, node *model.Node) {
	if node.Status != model.NodeCreated {
		return
	}
	met, err := s.CheckDependenciesMet(ctx, node.ID)
	if err != nil {
		zap.L().Error("TryMakeReady: failed to check dependencies", zap.Error(err))
		return
	}
	if met {
		if err := s.InitializeNodeReady(ctx, node); err != nil {
			zap.L().Error("TryMakeReady: failed to initialize node as ready", zap.Error(err))
		}
	}
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
		_ = s.eventSaver.SaveEvent(ctx, "task", taskID, eventbus.TopicTaskCompleted, eventbus.Event{
			TaskID: taskID, Status: "SUCCESS",
		})
	case model.TaskFailed:
		s.recordContext(ctx, taskID, "", model.ContextTaskFailed, "Task failed", nil)
		_ = s.eventSaver.SaveEvent(ctx, "task", taskID, eventbus.TopicTaskFailed, eventbus.Event{
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
		if err != nil || parent == nil {
			return false, nil
		}
		// SUCCESS or SKIPPED satisfies dependency
		if parent.Status != model.NodeSuccess && parent.Status != model.NodeSkipped {
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
		if n.Status != model.NodeSuccess && n.Status != model.NodeSkipped {
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

			// Merge node name into payload so worker can determine the correct tool
			payload := make(map[string]interface{})
			for k, v := range node.Input {
				payload[k] = v
			}
			payload["name"] = node.Name

			_ = s.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeReady, eventbus.Event{
				TaskID:         node.TaskID,
				NodeID:         node.ID,
				Type:           string(node.Type),
				Payload:        payload,
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
	case model.NodeSkipped:
		ctxType = model.ContextType("NODE_SKIPPED")
		message = "Node skipped (condition not met)"
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
