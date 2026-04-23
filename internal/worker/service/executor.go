package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type NodeExecutor struct {
	toolRegistry *tool.ToolRegistry
	producer     *eventbus.Producer
	cfg          config.WorkerConfig
}

func NewNodeExecutor(
	toolRegistry *tool.ToolRegistry,
	producer *eventbus.Producer,
	cfg config.WorkerConfig,
) *NodeExecutor {
	return &NodeExecutor{
		toolRegistry: toolRegistry,
		producer:     producer,
		cfg:          cfg,
	}
}

func (ne *NodeExecutor) ExecuteNode(ctx context.Context, event eventbus.Event) {
	taskID := event.TaskID
	nodeID := event.NodeID
	traceID := event.TraceID
	if traceID == "" {
		traceID = taskID + "-" + nodeID
	}

	idempotencyKey := event.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = taskID + "-" + nodeID
	}

	zap.L().Info("Starting node execution",
		zap.String("taskId", taskID),
		zap.String("nodeId", nodeID),
		zap.String("type", event.Type),
		zap.String("traceId", traceID),
		zap.String("idempotencyKey", idempotencyKey),
	)

	payload := event.Payload
	if payload == nil {
		payload = make(map[string]interface{})
	}

	toolName := tool.DetermineToolName(event.Type, payload)
	parameters := tool.ExtractParameters(payload)
	toolCtx := tool.ToolContext{
		TaskID:     taskID,
		NodeID:     nodeID,
		RetryCount: 0,
	}

	// Publish node running event
	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "RUNNING",
	})

	t, found := ne.toolRegistry.Get(toolName)
	if !found {
		errMsg := fmt.Sprintf("Tool not found: %s", toolName)
		ne.publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey)
		return
	}

	if !t.ValidateParameters(parameters) {
		errMsg := fmt.Sprintf("Invalid parameters for tool: %s", toolName)
		ne.publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey)
		return
	}

	resultCh := make(chan tool.ToolResult, 1)
	go func() {
		resultCh <- t.Execute(ctx, parameters, toolCtx)
	}()

	timeout := time.Duration(ne.cfg.ToolTimeoutSeconds) * time.Second
	select {
	case result := <-resultCh:
		if result.Success {
			ne.publishSuccess(taskID, nodeID, traceID, result.Data, idempotencyKey)
		} else {
			ne.publishFailure(taskID, nodeID, traceID, result.Error, idempotencyKey)
		}
	case <-time.After(timeout):
		errMsg := fmt.Sprintf("Tool execution timed out after %d seconds", ne.cfg.ToolTimeoutSeconds)
		ne.publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey)
	}
}

func (ne *NodeExecutor) publishSuccess(taskID, nodeID, traceID string, data map[string]interface{}, idempotencyKey string) {
	result := model.NodeResultEvent{
		TaskID:         taskID,
		NodeID:         nodeID,
		Status:         model.NodeSuccess,
		Data:           data,
		TraceID:        traceID,
		IdempotencyKey: idempotencyKey,
	}

	event := eventbus.Event{
		TaskID:         result.TaskID,
		NodeID:         result.NodeID,
		Status:         string(result.Status),
		Output:         result.Data,
		TraceID:        result.TraceID,
		IdempotencyKey: result.IdempotencyKey,
	}

	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, event)
	zap.L().Info("Node execution succeeded", zap.String("nodeId", nodeID))
}

func (ne *NodeExecutor) publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey string) {
	result := model.NodeResultEvent{
		TaskID:         taskID,
		NodeID:         nodeID,
		Status:         model.NodeFailed,
		TraceID:        traceID,
		ErrorMessage:   errMsg,
		IdempotencyKey: idempotencyKey,
	}

	event := eventbus.Event{
		TaskID:         result.TaskID,
		NodeID:         result.NodeID,
		Status:         string(result.Status),
		TraceID:        result.TraceID,
		ErrorMessage:   result.ErrorMessage,
		IdempotencyKey: result.IdempotencyKey,
	}

	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, event)
	zap.L().Info("Node execution failed", zap.String("nodeId", nodeID), zap.String("error", errMsg))
}
