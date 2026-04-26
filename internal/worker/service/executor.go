package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/eventbus"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type NodeExecutor struct {
	toolRegistry    *tool.ToolRegistry
	producer        *eventbus.Producer
	cfg             config.WorkerConfig
	directExecutor  *executor.DirectExecutor
	sandboxExecutor *executor.SandboxExecutor
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
) *NodeExecutor {
	return &NodeExecutor{
		toolRegistry:    toolRegistry,
		producer:        producer,
		cfg:             cfg,
		directExecutor:  directExec,
		sandboxExecutor: sandboxExec,
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

	toolName := tool.DetermineToolName(event.Type, payload)
	parameters := tool.ExtractParameters(payload)
	toolCtx := tool.ToolContext{
		TaskID:     taskID,
		NodeID:     nodeID,
		RetryCount: 0,
	}

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
				output := fmt.Sprintf("%v", toolResult.Data)
				result = executor.ExecutionResult{
					ExitCode: 0,
					Stdout:   []byte(output),
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

	if result.Error != "" || execErr != nil {
		if result.Error == "" {
			result.Error = execErr.Error()
		}
		ne.publishFailure(taskID, nodeID, traceID, result.Error, idempotencyKey)
		return
	}

	outputData := map[string]interface{}{
		"exitCode": result.ExitCode,
		"stdout":   string(result.Stdout),
		"stderr":   string(result.Stderr),
	}
	if result.ResourceUsage != nil {
		outputData["resourceUsage"] = result.ResourceUsage
	}
	if result.OutputRef != "" {
		outputData["outputRef"] = result.OutputRef
	}

	ne.publishSuccess(taskID, nodeID, traceID, outputData, idempotencyKey)
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
