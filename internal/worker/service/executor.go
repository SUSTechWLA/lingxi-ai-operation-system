package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type NodeExecutor struct {
	toolRegistry    *tool.ToolRegistry
	producer        *eventbus.Producer
	cfg             config.WorkerConfig
	directExecutor  *executor.DirectExecutor
	sandboxExecutor *executor.SandboxExecutor
	nodeRepo        repository.NodeRepo
}

type executorInterface interface {
	Execute(ctx context.Context, req executor.ExecutionRequest) (executor.ExecutionResult, error)
}

func NewNodeExecutor(
	toolRegistry *tool.ToolRegistry,
	producer *eventbus.Producer,
	cfg config.WorkerConfig,
	directExec *executor.DirectExecutor,
	sandboxExec *executor.SandboxExecutor,
	nodeRepo repository.NodeRepo,
) *NodeExecutor {
	return &NodeExecutor{
		toolRegistry:    toolRegistry,
		producer:        producer,
		cfg:             cfg,
		directExecutor:  directExec,
		sandboxExecutor: sandboxExec,
		nodeRepo:        nodeRepo,
	}
}

func (ne *NodeExecutor) selectExecutor(t tool.Tool) executorInterface {
	if ne.cfg.Sandbox.Enabled && ne.sandboxExecutor != nil {
		if _, ok := t.(tool.BuildableTool); ok {
			return ne.sandboxExecutor
		}
	}
	return ne.directExecutor
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

	// image_urls are stripped from Kafka events (too large) — read them from DB
	if _, hasImageURLs := payload["image_urls"]; !hasImageURLs && ne.nodeRepo != nil {
		if node, err := ne.nodeRepo.FindByID(ctx, nodeID); err == nil && node != nil {
			if urls, ok := node.Input["image_urls"]; ok {
				payload["image_urls"] = urls
			}
		}
	}

	toolName := tool.DetermineToolName(event.Type, payload)
	parameters := tool.ExtractParameters(payload)
	toolCtx := tool.ToolContext{
		TaskID:     taskID,
		NodeID:     nodeID,
		RetryCount: 0,
	}

	startTime := time.Now()
	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "RUNNING",
		Output: map[string]interface{}{
			"startedAt": startTime.Format(time.RFC3339Nano),
		},
	})

	t, found := ne.toolRegistry.Get(toolName)
	if !found {
		errMsg := fmt.Sprintf("Tool not found: %s", toolName)
		ne.publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey, nil)
		return
	}

	if !t.ValidateParameters(parameters) {
		errMsg := fmt.Sprintf("Invalid parameters for tool: %s", toolName)
		ne.publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey, nil)
		return
	}

	execImpl := ne.selectExecutor(t)

	var result executor.ExecutionResult
	var execErr error

	if bt, ok := t.(tool.BuildableTool); ok {
		execReq, err := bt.BuildExecutionRequest(parameters)
		if err != nil {
			result.Error = err.Error()
		} else {
			execReq.TaskID = taskID
			execReq.NodeID = nodeID
			timeout := time.Duration(execReq.TimeoutSec) * time.Second
			if timeout == 0 {
				timeout = time.Duration(ne.cfg.ToolTimeoutSeconds) * time.Second
			}
			execCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			result, execErr = execImpl.Execute(execCtx, *execReq)
		}
	} else if et, ok := t.(tool.ExecutableTool); ok {
		resultCh := make(chan tool.ToolResult, 1)
		go func() {
			resultCh <- et.Execute(ctx, parameters, toolCtx)
		}()

		timeout := time.Duration(ne.cfg.ToolTimeoutSeconds) * time.Second
		select {
		case toolResult := <-resultCh:
			if toolResult.Success {
				output, _ := json.Marshal(toolResult.Data)
				result = executor.ExecutionResult{
					ExitCode: 0,
					Stdout:   output,
				}
			} else {
				result = executor.ExecutionResult{
					ExitCode: 1,
					Error:    toolResult.Error,
				}
			}
		case <-time.After(timeout):
			result = executor.ExecutionResult{
				TimedOut: true,
				Error:    fmt.Sprintf("Tool execution timed out after %d seconds", ne.cfg.ToolTimeoutSeconds),
			}
		}
	} else {
		result.Error = "tool does not implement any executable interface"
	}

	durationMs := time.Since(startTime).Milliseconds()

	if result.Error != "" || execErr != nil {
		if result.Error == "" {
			result.Error = execErr.Error()
		}
		failureData := map[string]interface{}{
			"exitCode":   result.ExitCode,
			"durationMs": durationMs,
			"error":      result.Error,
		}
		if result.ResourceUsage != nil {
			failureData["resourceUsage"] = result.ResourceUsage
		}
		ne.publishFailure(taskID, nodeID, traceID, result.Error, idempotencyKey, failureData)
		return
	}

	data := map[string]interface{}{
		"exitCode":   result.ExitCode,
		"stdout":     string(result.Stdout),
		"stderr":     string(result.Stderr),
		"durationMs": durationMs,
	}
	if result.ResourceUsage != nil {
		data["resourceUsage"] = result.ResourceUsage
	}
	if result.OutputRef != "" {
		data["outputRef"] = result.OutputRef
	}

	ne.publishSuccess(taskID, nodeID, traceID, data, idempotencyKey)
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

func (ne *NodeExecutor) publishFailure(taskID, nodeID, traceID, errMsg, idempotencyKey string, data map[string]interface{}) {
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
		Output:         data,
		TraceID:        result.TraceID,
		ErrorMessage:   result.ErrorMessage,
		IdempotencyKey: result.IdempotencyKey,
	}

	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, event)
	zap.L().Info("Node execution failed",
		zap.String("nodeId", nodeID),
		zap.String("error", errMsg),
	)
}
