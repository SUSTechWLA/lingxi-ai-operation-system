package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
)

// StageStatusSyncer syncs a single stage's status to the workflow run.
type StageStatusSyncer interface {
	UpdateStageStatus(ctx context.Context, runID, stageName string, status string) error
	FindRunIDByTaskID(ctx context.Context, taskID string) (string, error)
}

type StateMachine struct {
	stateService      *StateService
	nodeRepo          repository.NodeRepo
	taskRepo          repository.TaskRepo
	eventSaver        outbox.EventSaver
	retryPolicy       *RetryPolicy
	stageStatusSyncer StageStatusSyncer
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

// SetStageStatusSyncer injects a syncer to keep the workflow run's
// stage_statuses JSONB in sync with the DAG node states.
func (sm *StateMachine) SetStageStatusSyncer(syncer StageStatusSyncer) {
	sm.stageStatusSyncer = syncer
}

func (sm *StateMachine) syncStageStatus(ctx context.Context, nodeID string, status string) {
	if sm.stageStatusSyncer == nil {
		return
	}
	node, err := sm.nodeRepo.FindByID(ctx, nodeID)
	if err != nil || node == nil {
		return
	}
	runID, err := sm.stageStatusSyncer.FindRunIDByTaskID(ctx, node.TaskID)
	if err != nil || runID == "" {
		return
	}
	stageName := stageStatusKey(node)
	if err := sm.stageStatusSyncer.UpdateStageStatus(ctx, runID, stageName, status); err != nil {
		zap.L().Warn("Failed to sync workflow stage status",
			zap.String("runId", runID),
			zap.String("nodeId", nodeID),
			zap.String("stageName", stageName),
			zap.String("status", status),
			zap.Error(err))
	}
}

func stageStatusKey(node *model.Node) string {
	if node == nil {
		return ""
	}
	if node.Input != nil {
		if original, ok := node.Input["agentOriginalNodeId"].(string); ok && original != "" {
			return original
		}
		if stage, ok := node.Input["stage"].(string); ok && stage != "" {
			if strings.HasSuffix(node.ID, "_exec") {
				return stage + "_exec"
			}
			if strings.HasSuffix(node.ID, "_skip") {
				return stage + "_skip"
			}
			return stage
		}
	}
	if strings.HasPrefix(node.ID, node.TaskID+"-") {
		return strings.TrimPrefix(node.ID, node.TaskID+"-")
	}
	return node.ID
}

// OnProgress updates the ai_node progress, current step, and heartbeat timestamp
// so the frontend can display real-time local execution progress.
func (sm *StateMachine) OnProgress(ctx context.Context, nodeID string, progress float64, step, message string) error {
	if err := sm.nodeRepo.UpdateHeartbeat(ctx, nodeID, progress, step); err != nil {
		return fmt.Errorf("update node heartbeat/progress: %w", err)
	}
	// Also update the stage status to reflect the current step.
	if message != "" {
		sm.syncStageStatus(ctx, nodeID, "RUNNING:"+message)
	} else if step != "" {
		sm.syncStageStatus(ctx, nodeID, "RUNNING:"+step)
	}
	return nil
}

func (sm *StateMachine) OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error {
	node, err := sm.stateService.TransitionNode(ctx, nodeID, model.NodeSuccess, output, "")
	if err != nil {
		return fmt.Errorf("failed to transition node to SUCCESS: %w", err)
	}

	// Sync the node status to the workflow run's stage_statuses so the
	// frontend progress panel shows the correct stage state.
	sm.syncStageStatus(ctx, nodeID, "SUCCEEDED")

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

	// Sync the failed status to the workflow run's stage_statuses.
	sm.syncStageStatus(ctx, node.ID, "FAILED")

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
