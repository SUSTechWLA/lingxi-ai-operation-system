package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
)

type StateMachine struct {
	stateService *StateService
	nodeRepo     repository.NodeRepo
	taskRepo     repository.TaskRepo
	eventSaver   outbox.EventSaver
	retryPolicy  *RetryPolicy
}

func NewStateMachine(
	stateService *StateService,
	nodeRepo repository.NodeRepo,
	taskRepo repository.TaskRepo,
	eventSaver outbox.EventSaver,
) *StateMachine {
	return &StateMachine{
		stateService: stateService,
		nodeRepo:     nodeRepo,
		taskRepo:     taskRepo,
		eventSaver:   eventSaver,
		retryPolicy:  NewRetryPolicy(),
	}
}

func (sm *StateMachine) OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error {
	node, err := sm.stateService.TransitionNode(ctx, nodeID, model.NodeSuccess, output, "")
	if err != nil {
		return fmt.Errorf("failed to transition node to SUCCESS: %w", err)
	}

	zap.L().Info("Node succeeded", zap.String("nodeId", nodeID))

	_ = sm.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeExecuted, eventbus.Event{
		TaskID: node.TaskID,
		NodeID: node.ID,
		Status: "SUCCESS",
		Output: node.Output,
	})

	task, err := sm.taskRepo.FindByID(ctx, node.TaskID)
	if err != nil || task == nil {
		return nil
	}

	if task.Status == model.TaskPaused {
		hasRetry, _ := sm.stateService.HasRetryableNodes(ctx, node.TaskID)
		if !hasRetry {
			_ = sm.stateService.TransitionTask(ctx, node.TaskID, model.TaskRunning)
		}
	}

	if sm.stateService.dependencyChecker != nil {
		sm.stateService.dependencyChecker.OnNodeExecuted(ctx, node.ID, node.TaskID)
	}

	completed, _ := sm.stateService.CheckTaskCompleted(ctx, node.TaskID)
	if completed {
		_ = sm.stateService.TransitionTask(ctx, node.TaskID, model.TaskSuccess)
	}

	return nil
}

func (sm *StateMachine) OnFailure(ctx context.Context, nodeID string, errorMessage string) error {
	node, err := sm.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	zap.L().Info("Node failed", zap.String("nodeId", nodeID), zap.String("error", errorMessage))

	if sm.retryPolicy.ShouldRetry(node.RetryCount, node.MaxRetry) {
		return sm.retryNode(ctx, node, errorMessage)
	}
	return sm.failNode(ctx, node, errorMessage)
}

func (sm *StateMachine) retryNode(ctx context.Context, node *model.Node, errorMessage string) error {
	zap.L().Info("Retrying node",
		zap.String("nodeId", node.ID),
		zap.Int("attempt", node.RetryCount+1),
		zap.Int("maxRetry", node.MaxRetry),
	)

	node.ErrorMessage = errorMessage
	node.Status = model.NodeRetrying
	node.RetryCount++
	if err := sm.nodeRepo.Save(ctx, node); err != nil {
		return err
	}

	_, err := sm.stateService.TransitionNode(ctx, node.ID, model.NodeCreated, nil, "")
	return err
}

// OnHeartbeatTimeout handles a long-running node whose heartbeat has timed out.
// It transitions the node to HEARTBEAT_TIMEOUT, then either retries or permanently fails.
func (sm *StateMachine) OnHeartbeatTimeout(ctx context.Context, nodeID string) error {
	node, err := sm.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	zap.L().Warn("Heartbeat timeout detected for node",
		zap.String("nodeId", nodeID),
		zap.String("taskId", node.TaskID),
	)

	errMsg := "heartbeat timeout: no heartbeat received within timeout period"
	node.Status = model.NodeHeartbeatTimeout
	node.ErrorMessage = errMsg
	if err := sm.nodeRepo.Save(ctx, node); err != nil {
		return err
	}

	_ = sm.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeFailed, eventbus.Event{
		TaskID:       node.TaskID,
		NodeID:       node.ID,
		Status:       "HEARTBEAT_TIMEOUT",
		ErrorMessage: errMsg,
	})

	// If retries remain, retry from checkpoint
	if sm.retryPolicy.ShouldRetry(node.RetryCount, node.MaxRetry) {
		return sm.retryNode(ctx, node, errMsg)
	}

	// No retries left — permanent failure
	return sm.failNode(ctx, node, errMsg)
}

func (sm *StateMachine) failNode(ctx context.Context, node *model.Node, errorMessage string) error {
	zap.L().Info("Node failed after max retries",
		zap.String("nodeId", node.ID),
		zap.Int("maxRetry", node.MaxRetry),
	)

	_, err := sm.stateService.TransitionNode(ctx, node.ID, model.NodeFailed, nil, errorMessage)
	if err != nil {
		return err
	}

	_ = sm.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeFailed, eventbus.Event{
		TaskID:       node.TaskID,
		NodeID:       node.ID,
		Status:       "FAILED",
		ErrorMessage: node.ErrorMessage,
	})

	_ = sm.stateService.TransitionTask(ctx, node.TaskID, model.TaskPaused)

	hasRetry, _ := sm.stateService.HasRetryableNodes(ctx, node.TaskID)
	if !hasRetry {
		_ = sm.stateService.TransitionTask(ctx, node.TaskID, model.TaskFailed)
	}

	return nil
}
