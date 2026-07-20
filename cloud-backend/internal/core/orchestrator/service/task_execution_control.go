package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
)

type TaskExecutionControl struct {
	taskRepo     repository.TaskRepo
	nodeRepo     repository.NodeRepo
	stateService *StateService
}

type idempotentRetryNodeClaimer interface {
	ClaimRetryByIdempotencyKey(context.Context, string, string) (*model.Node, bool, error)
}

func NewTaskExecutionControl(
	taskRepo repository.TaskRepo,
	nodeRepo repository.NodeRepo,
	stateService *StateService,
) *TaskExecutionControl {
	return &TaskExecutionControl{
		taskRepo:     taskRepo,
		nodeRepo:     nodeRepo,
		stateService: stateService,
	}
}

func (tc *TaskExecutionControl) PauseTask(ctx context.Context, taskID, reason string) error {
	task, err := tc.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Status = model.TaskPaused
	task.PauseReason = reason
	if err := tc.taskRepo.Save(ctx, task); err != nil {
		return fmt.Errorf("failed to pause task: %w", err)
	}
	zap.L().Info("Task paused", zap.String("taskId", taskID), zap.String("reason", reason))
	return nil
}

func (tc *TaskExecutionControl) ResumeTask(ctx context.Context, taskID string) error {
	task, err := tc.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Status = model.TaskRunning
	task.PauseReason = ""
	if err := tc.taskRepo.Save(ctx, task); err != nil {
		return fmt.Errorf("failed to resume task: %w", err)
	}
	zap.L().Info("Task resumed", zap.String("taskId", taskID))
	return nil
}

func (tc *TaskExecutionControl) FailTask(ctx context.Context, taskID string) error {
	task, err := tc.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	return tc.stateService.TransitionTask(ctx, taskID, model.TaskFailed)
}

func (tc *TaskExecutionControl) RetryNode(ctx context.Context, nodeID string) error {
	node, err := tc.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	node.Status = model.NodeCreated
	node.RetryCount = 0
	node.ErrorMessage = ""
	if err := tc.nodeRepo.Save(ctx, node); err != nil {
		return fmt.Errorf("failed to save node: %w", err)
	}

	if err := tc.stateService.InitializeNodeReady(ctx, node); err != nil {
		return fmt.Errorf("failed to initialize node as ready: %w", err)
	}

	zap.L().Info("Node retry initiated", zap.String("nodeId", nodeID))
	return nil
}

// RetryNodeIdempotent persists the logical retry key before publishing READY.
// If the caller is replaying after a crash, a CREATED claim is safely resumed
// and any later state is left untouched.
func (tc *TaskExecutionControl) RetryNodeIdempotent(ctx context.Context, nodeID, key string) error {
	claimer, ok := tc.nodeRepo.(idempotentRetryNodeClaimer)
	if !ok {
		return fmt.Errorf("node repository does not support idempotent retry claims")
	}
	node, _, err := claimer.ClaimRetryByIdempotencyKey(ctx, nodeID, key)
	if err != nil {
		return fmt.Errorf("failed to claim idempotent node retry: %w", err)
	}
	if node.Status != model.NodeCreated {
		return nil
	}
	if err := tc.stateService.InitializeNodeReady(ctx, node); err != nil {
		return fmt.Errorf("failed to initialize idempotent node retry: %w", err)
	}
	zap.L().Info("Idempotent node retry initiated", zap.String("nodeId", nodeID), zap.String("idempotencyKey", key))
	return nil
}

func (tc *TaskExecutionControl) GetTaskPauseReason(ctx context.Context, taskID string) string {
	task, err := tc.taskRepo.FindByID(ctx, taskID)
	if err != nil || task == nil {
		return "Task not found"
	}
	if task.Status == model.TaskPaused {
		if task.PauseReason != "" {
			return task.PauseReason
		}
		return "Task was paused"
	}
	return "Task is not paused"
}
