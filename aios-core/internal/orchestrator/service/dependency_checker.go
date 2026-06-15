package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/eventbus"
	"github.com/tangying-ai/aios-core/internal/model"
	"github.com/tangying-ai/aios-core/internal/model/repository"
	"github.com/tangying-ai/aios-core/internal/outbox"
)

type DependencyChecker struct {
	nodeRepo     repository.NodeRepo
	stateService *StateService
	eventSaver   outbox.EventSaver
}

func NewDependencyChecker(
	nodeRepo repository.NodeRepo,
	stateService *StateService,
	eventSaver outbox.EventSaver,
) *DependencyChecker {
	return &DependencyChecker{
		nodeRepo:     nodeRepo,
		stateService: stateService,
		eventSaver:   eventSaver,
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
		if child.Status != model.NodeCreated && child.Status != model.NodeRetrying {
			continue
		}

		met, err := dc.stateService.CheckDependenciesMet(ctx, child.ID)
		if err != nil {
			zap.L().Error("Failed to check dependencies", zap.Error(err))
			continue
		}
		if !met {
			continue
		}

		if child.Condition != "" && !evaluateCondition(child.Condition, child.TaskID, ctx, dc.nodeRepo) {
			_, _ = dc.stateService.TransitionNode(ctx, child.ID, model.NodeSkipped, nil, "Condition not met")
			zap.L().Info("Child node SKIPPED (condition not met)", zap.String("nodeId", child.ID))
			continue
		}

		if err := dc.stateService.InitializeNodeReady(ctx, child); err != nil {
			zap.L().Error("Failed to initialize node as ready", zap.Error(err))
			continue
		}

		childPayload := buildEventPayload(child.Input, child.Name)

		_ = dc.eventSaver.SaveEvent(ctx, "node", child.ID, eventbus.TopicNodeReady, eventbus.Event{
			TaskID:         child.TaskID,
			NodeID:         child.ID,
			Type:           string(child.Type),
			Payload:        childPayload,
			TraceID:        child.TaskID + "-" + child.ID,
			IdempotencyKey: child.IdempotencyKey,
		})
		zap.L().Info("Child node is now READY and published", zap.String("nodeId", child.ID))
	}

	completed, _ := dc.stateService.CheckTaskCompleted(ctx, taskID)
	if completed {
		_ = dc.stateService.TransitionTask(ctx, taskID, model.TaskSuccess)
		zap.L().Info("Task completed successfully", zap.String("taskId", taskID))
	}
}

// evaluateCondition checks a simple condition string against parent node outputs
// Supported format: "NODE_ID.status == success" or "NODE_ID.status == failed"
func evaluateCondition(condition, taskID string, ctx context.Context, nodeRepo repository.NodeRepo) bool {
	condition = strings.TrimSpace(condition)

	// Simple equality check: "nodeId.status == success"
	parts := strings.SplitN(condition, "==", 2)
	if len(parts) != 2 {
		zap.L().Warn("Invalid condition format, treating as true", zap.String("condition", condition))
		return true
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	// Parse "nodeId.field"
	fieldParts := strings.SplitN(left, ".", 2)
	if len(fieldParts) != 2 {
		return true
	}

	refNodeID := fieldParts[0]
	field := fieldParts[1]

	node, err := nodeRepo.FindByID(ctx, refNodeID)
	if err != nil || node == nil {
		zap.L().Warn("Condition references unknown node, treating as false", zap.String("nodeId", refNodeID))
		return false
	}

	var actual string
	switch field {
	case "status":
		actual = string(node.Status)
	default:
		if node.Output != nil {
			if v, ok := node.Output[field]; ok {
				actual = strings.ToLower(fmt.Sprintf("%v", v))
			}
		}
	}

	return strings.EqualFold(actual, right)
}
