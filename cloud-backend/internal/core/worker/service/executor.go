package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// nodeRefPattern matches {{node_id.output.field}} references in node inputs.
var nodeRefPattern = regexp.MustCompile(`\{\{([^.]+)\.output\.([^}]+)\}\}`)

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
	// Also read long-running metadata for the node
	var isLongRunning bool
	var heartbeatTimeoutSec int
	if ne.nodeRepo != nil {
		if node, err := ne.nodeRepo.FindByID(ctx, nodeID); err == nil && node != nil {
			if urls, ok := node.Input["image_urls"]; ok {
				payload["image_urls"] = urls
			}
			isLongRunning = node.LongRunning
			if node.HeartbeatTimeoutSec > 0 {
				heartbeatTimeoutSec = node.HeartbeatTimeoutSec
			}
		}
	}
	// Also check payload-level override
	if lr, ok := payload["long_running"].(bool); ok && lr {
		isLongRunning = true
	}

	toolName := tool.DetermineToolName(event.Type, payload)
	parameters := tool.ExtractParameters(payload)

	// Resolve {{node_id.output.field}} references using completed parent node outputs
	if ne.nodeRepo != nil {
		resolved, err := ne.resolveNodeReferences(ctx, taskID, parameters)
		if err != nil {
			zap.L().Warn("Failed to resolve node references, using original parameters", zap.Error(err))
		} else {
			parameters = resolved
		}
	}

	// Build tool context with checkpoint data for retries
	toolCtx := tool.ToolContext{
		TaskID:     taskID,
		NodeID:     nodeID,
		RetryCount: 0,
	}
	if isLongRunning && ne.nodeRepo != nil {
		if node, err := ne.nodeRepo.FindByID(ctx, nodeID); err == nil && node != nil {
			toolCtx.RetryCount = node.RetryCount
		}
	}

	startTime := time.Now()

	// --- Long-running task: heartbeat + progress support ---
	hbInterval := ne.cfg.HeartbeatIntervalSec
	if hbInterval <= 0 {
		hbInterval = 30
	}
	hbTimeout := heartbeatTimeoutSec
	if hbTimeout <= 0 {
		hbTimeout = ne.cfg.HeartbeatTimeoutSec
	}
	if hbTimeout <= 0 {
		hbTimeout = 300 // default 5 minutes
	}

	var progressCb tool.ProgressCallback
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()

	if isLongRunning {
		zap.L().Info("Starting long-running node execution",
			zap.String("nodeId", nodeID),
			zap.Int("heartbeatInterval", hbInterval),
			zap.Int("heartbeatTimeout", hbTimeout),
		)

		// First heartbeat immediately
		ne.publishProgress(taskID, nodeID, 0, "started")
		ne.publishHeartbeat(taskID, nodeID, idempotencyKey)

		// Progress callback for the tool
		progressCb = func(cbCtx context.Context, update tool.ProgressUpdate) {
			if update.Progress > 0 {
				ne.publishProgress(taskID, nodeID, update.Progress, update.Step)
			}
			// If checkpoint data provided, persist it via progress event
			if update.Checkpoint != nil {
				ne.publishCheckpoint(taskID, nodeID, update.Progress, update.Step, update.Checkpoint)
			}
		}

		// Periodic heartbeat goroutine
		go func() {
			ticker := time.NewTicker(time.Duration(hbInterval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-ticker.C:
					ne.publishHeartbeat(taskID, nodeID, idempotencyKey)
				}
			}
		}()
	}

	_ = ne.producer.Publish(eventbus.TopicNodeResult, idempotencyKey, eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "RUNNING",
		Output: map[string]interface{}{
			"startedAt":           startTime.Format(time.RFC3339Nano),
			"longRunning":         isLongRunning,
			"heartbeatTimeoutSec": hbTimeout,
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

	// Wire progress reporter for long-running tasks
	if isLongRunning && progressCb != nil {
		if pr, ok := t.(tool.ProgressReporter); ok {
			pr.SetProgressCallback(progressCb)
			zap.L().Info("ProgressReporter wired for tool", zap.String("tool", toolName), zap.String("nodeId", nodeID))
		} else {
			zap.L().Debug("Tool does not implement ProgressReporter, only heartbeat will be sent",
				zap.String("tool", toolName), zap.String("nodeId", nodeID))
		}
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
	sanitizedData := repository.SanitizeOutputForPersistence(data)
	result := model.NodeResultEvent{
		TaskID:         taskID,
		NodeID:         nodeID,
		Status:         model.NodeSuccess,
		Data:           sanitizedData,
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
	sanitizedData := repository.SanitizeOutputForPersistence(data)
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
		Output:         sanitizedData,
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

// ── Long-running task helpers ──

// publishHeartbeat sends a heartbeat event for a long-running node.
func (ne *NodeExecutor) publishHeartbeat(taskID, nodeID, idempotencyKey string) {
	hbKey := idempotencyKey + "-hb"
	event := eventbus.Event{
		TaskID:         taskID,
		NodeID:         nodeID,
		Status:         "HEARTBEAT",
		IdempotencyKey: hbKey,
	}
	_ = ne.producer.Publish(eventbus.TopicProgress, hbKey, event)
}

// publishProgress sends a progress update event for a long-running node.
func (ne *NodeExecutor) publishProgress(taskID, nodeID string, progress float64, step string) {
	event := eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "PROGRESS",
		Output: map[string]interface{}{
			"progress": progress,
			"step":     step,
		},
	}
	_ = ne.producer.Publish(eventbus.TopicProgress, taskID+"-"+nodeID+"-progress", event)
}

// publishCheckpoint sends a checkpoint event for a long-running node.
func (ne *NodeExecutor) publishCheckpoint(taskID, nodeID string, progress float64, step string, checkpoint map[string]interface{}) {
	event := eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "CHECKPOINT",
		Output: map[string]interface{}{
			"progress":   progress,
			"step":       step,
			"checkpoint": checkpoint,
		},
	}
	_ = ne.producer.Publish(eventbus.TopicProgress, taskID+"-"+nodeID+"-checkpoint", event)
	zap.L().Info("Checkpoint saved",
		zap.String("nodeId", nodeID),
		zap.Float64("progress", progress),
	)
}

// resolveNodeReferences scans parameters for {{node_id.output.field}} references,
// looks up the completed parent nodes, and replaces references with actual values.
func (ne *NodeExecutor) resolveNodeReferences(ctx context.Context, taskID string, params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(params))
	for k, v := range params {
		resolved, err := resolveValue(ctx, ne.nodeRepo, taskID, v)
		if err != nil {
			return nil, err
		}
		result[k] = resolved
	}
	return result, nil
}

func resolveValue(ctx context.Context, nodeRepo repository.NodeRepo, taskID string, v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case string:
		// If the entire string is a single {{ref}}, resolve to the typed value
		if nodeRefPattern.MatchString(val) && strings.TrimSpace(val) == val &&
			strings.HasPrefix(val, "{{") && strings.HasSuffix(val, "}}") &&
			strings.Count(val, "{{") == 1 {
			typed, isRef := resolveSingleRef(ctx, nodeRepo, taskID, val)
			if isRef {
				return typed, nil
			}
		}
		return resolveString(ctx, nodeRepo, taskID, val), nil
	case map[string]interface{}:
		resolved := make(map[string]interface{}, len(val))
		for mk, mv := range val {
			rv, err := resolveValue(ctx, nodeRepo, taskID, mv)
			if err != nil {
				return nil, err
			}
			resolved[mk] = rv
		}
		return resolved, nil
	case []interface{}:
		resolved := make([]interface{}, len(val))
		for i, item := range val {
			rv, err := resolveValue(ctx, nodeRepo, taskID, item)
			if err != nil {
				return nil, err
			}
			resolved[i] = rv
		}
		return resolved, nil
	default:
		return v, nil
	}
}

// resolveSingleRef resolves an entire string that is one {{node.output.field}} reference.
// Returns the typed value directly (not JSON-stringified) for non-string types like arrays.
func resolveSingleRef(ctx context.Context, nodeRepo repository.NodeRepo, taskID string, ref string) (interface{}, bool) {
	matches := nodeRefPattern.FindStringSubmatch(ref)
	if len(matches) != 3 {
		return ref, false
	}

	refNodeID := strings.TrimSpace(matches[1])
	field := strings.TrimSpace(matches[2])

	_, output := findNodeOutput(ctx, nodeRepo, taskID, refNodeID)
	if output == nil {
		return ref, false
	}

	val, ok := output[field]
	if !ok {
		if stdout, sOk := output["stdout"].(string); sOk && stdout != "" {
			var parsed map[string]interface{}
			if json.Unmarshal([]byte(stdout), &parsed) == nil {
				val, ok = parsed[field]
			}
		}
	}
	if !ok {
		return ref, false
	}

	return val, true
}

// findNodeOutput looks up a node and returns its output map, trying both the raw reference ID
// and the taskID-scoped version (taskID-refNodeID) for compatibility with both code paths.
func findNodeOutput(ctx context.Context, nodeRepo repository.NodeRepo, taskID, refNodeID string) (*model.Node, map[string]interface{}) {
	// Try raw ID first (used by /api/node direct submission path)
	for _, candidate := range []string{
		refNodeID,
		taskID + "-" + refNodeID,
	} {
		node, err := nodeRepo.FindByID(ctx, candidate)
		if err != nil || node == nil || node.Output == nil {
			continue
		}
		return node, node.Output
	}
	return nil, nil
}

func resolveString(ctx context.Context, nodeRepo repository.NodeRepo, taskID string, s string) string {
	matches := nodeRefPattern.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return s
	}

	result := s
	for _, m := range matches {
		refNodeID := strings.TrimSpace(m[1])
		field := strings.TrimSpace(m[2])

		_, output := findNodeOutput(ctx, nodeRepo, taskID, refNodeID)
		if output == nil {
			zap.L().Warn("Cannot resolve node reference: node not found",
				zap.String("refNodeId", refNodeID),
				zap.String("field", field),
			)
			continue
		}

		val, ok := output[field]
		if !ok {
			// Field not at top level — try parsing stdout (tool output is JSON-marshaled there)
			if stdout, sOk := output["stdout"].(string); sOk && stdout != "" {
				var parsed map[string]interface{}
				if json.Unmarshal([]byte(stdout), &parsed) == nil {
					val, ok = parsed[field]
				}
			}
		}
		if !ok {
			zap.L().Warn("Cannot resolve node reference: field not found in output",
				zap.String("refNodeId", refNodeID),
				zap.String("field", field),
			)
			continue
		}

		var replacement string
		switch v := val.(type) {
		case string:
			replacement = v
		default:
			b, err := json.Marshal(v)
			if err != nil {
				replacement = fmt.Sprintf("%v", v)
			} else {
				replacement = string(b)
			}
		}

		result = strings.Replace(result, m[0], replacement, 1)
	}

	return result
}
