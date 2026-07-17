package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

// CheckpointService manages workflow checkpoint lifecycle: creation on review
// gates, listing, and recovery after restart.
type CheckpointService struct {
	store CheckpointStore
	runs  WorkflowRunStore
	nodes NodeStore
}

// WorkflowRunStore abstracts the workflow run persistence layer.
type WorkflowRunStore interface {
	FindByTaskID(ctx context.Context, taskID string) (*WorkflowRun, error)
}

// NodeStore abstracts node lookups needed for checkpointing.
type NodeStore interface {
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
}

// NewCheckpointService creates a CheckpointService.
func NewCheckpointService(store CheckpointStore, runs WorkflowRunStore, nodes NodeStore) *CheckpointService {
	return &CheckpointService{store: store, runs: runs, nodes: nodes}
}

// TransitionHook returns an orchestrator TransitionHook that writes a
// checkpoint whenever a review-gate node transitions to READY.
func (s *CheckpointService) TransitionHook() func(ctx context.Context, node *model.Node, newStatus model.NodeStatus) {
	return func(ctx context.Context, node *model.Node, newStatus model.NodeStatus) {
		if newStatus != model.NodeReady {
			return
		}
		if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
			return
		}

		// Find the workflow run associated with this task.
		run, err := s.runs.FindByTaskID(ctx, node.TaskID)
		if err != nil || run == nil {
			zap.L().Debug("Checkpoint hook: no workflow run for task, skipping",
				zap.String("taskId", node.TaskID), zap.Error(err))
			return
		}

		stageName := resolveStageName(node)
		stageIndex := resolveStageIndex(node)

		// Build a lightweight snapshot of the node's input metadata.
		snapshot := buildNodeSnapshot(node)
		snapshotJSON, _ := json.Marshal(snapshot)

		cp := &Checkpoint{
			ID:            run.ID + "_" + node.ID,
			WorkflowRunID: run.ID,
			TaskID:        node.TaskID,
			StageName:     stageName,
			StageIndex:    stageIndex,
			NodeID:        node.ID,
			State:         CheckpointAwaitingHuman,
			Snapshot:      snapshotJSON,
		}

		if err := s.store.Save(ctx, cp); err != nil {
			zap.L().Error("Failed to save checkpoint",
				zap.String("workflowRunId", run.ID),
				zap.String("stage", stageName),
				zap.Error(err))
			return
		}

		zap.L().Info("Checkpoint written",
			zap.String("workflowRunId", run.ID),
			zap.String("stage", stageName),
			zap.String("state", string(CheckpointAwaitingHuman)))
	}
}

// OnReviewApproved marks the latest checkpoint for the given stage as completed.
func (s *CheckpointService) OnReviewApproved(ctx context.Context, workflowRunID, stageName string) error {
	cp, err := s.store.FindByStage(ctx, workflowRunID, stageName)
	if err != nil || cp == nil {
		return fmt.Errorf("no checkpoint found for stage %s in run %s", stageName, workflowRunID)
	}
	cp.State = CheckpointCompleted
	return s.store.Save(ctx, cp)
}

// ListByRun returns all checkpoints for a workflow run, ordered by creation time.
func (s *CheckpointService) ListByRun(ctx context.Context, workflowRunID string) ([]*Checkpoint, error) {
	return s.store.ListByRun(ctx, workflowRunID)
}

// GetLatest returns the most recent checkpoint for a run.
func (s *CheckpointService) GetLatest(ctx context.Context, workflowRunID string) (*Checkpoint, error) {
	return s.store.FindLatest(ctx, workflowRunID)
}

// RecoverToCheckpoint finds the latest AWAITING_HUMAN checkpoint for the given
// run and returns it so the caller can resume the workflow at that stage.
// Returns nil if no awaiting checkpoint exists.
func (s *CheckpointService) RecoverToCheckpoint(ctx context.Context, workflowRunID string) (*Checkpoint, error) {
	latest, err := s.store.FindLatest(ctx, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("find latest checkpoint: %w", err)
	}
	if latest == nil {
		return nil, nil
	}
	if latest.State != CheckpointAwaitingHuman {
		return nil, nil // already completed or in-progress — nothing to recover
	}
	if err := s.store.MarkRecovered(ctx, latest.ID); err != nil {
		zap.L().Warn("Failed to mark checkpoint as recovered", zap.String("id", latest.ID), zap.Error(err))
	}
	return latest, nil
}

// resolveStageName extracts the workflow stage name from a review gate node's input.
func resolveStageName(node *model.Node) string {
	if node.Input == nil {
		return node.Name
	}
	if s, ok := node.Input["stage"].(string); ok && s != "" {
		return s
	}
	if s, ok := node.Input["stageName"].(string); ok && s != "" {
		return s
	}
	return node.Name
}

// resolveStageIndex tries to extract a stage index from node input metadata.
func resolveStageIndex(node *model.Node) int {
	if node.Input == nil {
		return 0
	}
	if idx, ok := node.Input["stageIndex"].(float64); ok {
		return int(idx)
	}
	return 0
}

// buildNodeSnapshot creates a lightweight metadata map for checkpoint snapshots.
func buildNodeSnapshot(node *model.Node) map[string]interface{} {
	snapshot := map[string]interface{}{
		"nodeId":   node.ID,
		"nodeName": node.Name,
		"nodeType": string(node.Type),
		"status":   string(node.Status),
	}
	if node.Input != nil {
		// Include review-relevant input fields, skip large payloads.
		for _, key := range []string{"stage", "stageName", "reviewPhase", "reviewFocus", "stepId", "tool"} {
			if v, ok := node.Input[key]; ok {
				snapshot[key] = v
			}
		}
	}
	return snapshot
}
