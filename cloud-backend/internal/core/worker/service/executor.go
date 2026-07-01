package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// nodeRefPattern matches {{node_id.output.field}} references in node inputs.
var nodeRefPattern = regexp.MustCompile(`\{\{([^.]+)\.output\.([^}]+)\}\}`)

type NodeExecutor struct {
	toolRegistry    *tool.ToolRegistry
	producer        eventbus.EventPublisher
	cfg             config.WorkerConfig
	directExecutor  *executor.DirectExecutor
	sandboxExecutor *executor.SandboxExecutor
	nodeRepo        repository.NodeRepo
	localDispatcher localJobDispatcher
	renderChecker   RenderDependencyChecker
}

type executorInterface interface {
	Execute(ctx context.Context, req executor.ExecutionRequest) (executor.ExecutionResult, error)
}

type localJobDispatcher interface {
	DispatchLocalJob(ctx context.Context, req localrunner.DispatchLocalJobRequest) (*localrunner.LocalJob, error)
}

type RenderDependencyCheckRequest struct {
	ProjectID string
	TaskID    string
	NodeID    string
	ToolName  string
	Command   string
	Payload   map[string]interface{}
}

type RenderDependencyChecker interface {
	CheckRenderDependencies(ctx context.Context, req RenderDependencyCheckRequest) error
}

type DefaultRenderDependencyChecker struct{}

func (DefaultRenderDependencyChecker) CheckRenderDependencies(_ context.Context, req RenderDependencyCheckRequest) error {
	if !isRenderLocalCommand(req.ToolName, req.Command) {
		return nil
	}
	if req.ToolName == "artifact_packager" || localrunner.NormalizeCommand(req.Command) == localrunner.CommandArtifactPackage {
		// Package guard is handled by RepositoryBackedRenderDependencyChecker.
		// Default checker only handles render commands.
		return nil
	}
	missing := make([]string, 0)
	if !truthy(req.Payload["previewApproved"]) && !truthy(req.Payload["previewHumanApproved"]) {
		missing = append(missing, "PREVIEW_SNAPSHOTS.humanApproved", "preview_review.APPROVED")
	}
	if invalidArtifactState(req.Payload["videoCompositionStatus"]) || invalidArtifactState(req.Payload["compositionStatus"]) {
		missing = append(missing, "VIDEO_COMPOSITION_SPEC.valid")
	}
	if invalidArtifactState(req.Payload["hyperframesProjectStatus"]) || invalidArtifactState(req.Payload["projectStatus"]) {
		missing = append(missing, "HYPERFRAMES_PROJECT.valid")
	}
	if len(missing) > 0 {
		return fmt.Errorf("RENDER_DEPENDENCY_MISSING: 预览尚未确认，禁止开始最终渲染。 missing=%v", missing)
	}
	return nil
}

func NewNodeExecutor(
	toolRegistry *tool.ToolRegistry,
	producer eventbus.EventPublisher,
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

func (ne *NodeExecutor) SetLocalJobDispatcher(dispatcher localJobDispatcher) {
	ne.localDispatcher = dispatcher
}

func (ne *NodeExecutor) SetRenderDependencyChecker(checker RenderDependencyChecker) {
	ne.renderChecker = checker
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

	// Hydrate payload from DB (image URLs, long-running metadata).
	payload, isLongRunning, heartbeatTimeoutSec := ne.hydratePayloadFromDB(ctx, nodeID, payload)

	// REVIEW_GATE and CONTROL nodes are synchronization points, not executable tools.
	// They pause execution and wait for human approval via the review API.
	if event.Type == string(model.NodeTypeReviewGate) || event.Type == string(model.NodeTypeControl) {
		ne.publishSuccess(taskID, nodeID, traceID, payload, idempotencyKey)
		return
	}

	toolName := tool.DetermineToolName(event.Type, payload)
	parameters := tool.ExtractParameters(payload)
	manifest := ne.toolRegistry.GetManifest(toolName)

	// Resolve {{node_id.output.field}} references.
	parameters = ne.resolveParameters(ctx, taskID, parameters)

	// Build tool context with checkpoint data for retries.
	toolCtx := ne.buildToolContext(ctx, nodeID, taskID, isLongRunning)

	// Local execution plane: dispatch to local runner and return.
	if manifest != nil && manifest.ExecutionPlane == tool.ExecutionPlaneLocal {
		if err := ne.dispatchLocalNode(ctx, event, manifest, parameters, idempotencyKey); err != nil {
			ne.publishFailure(taskID, nodeID, traceID, err.Error(), idempotencyKey, nil)
		}
		return
	}

	startTime := time.Now()

	// Set up long-running heartbeat + progress support.
	progressCb, hbCancel := ne.setupLongRunningHeartbeat(ctx, taskID, nodeID, idempotencyKey,
		isLongRunning, heartbeatTimeoutSec)
	defer hbCancel()

	ne.publishEvent(eventbus.TopicNodeResult, idempotencyKey, eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Status: "RUNNING",
		Output: map[string]interface{}{
			"startedAt":           startTime.Format(time.RFC3339Nano),
			"longRunning":         isLongRunning,
			"heartbeatTimeoutSec": heartbeatTimeoutSec,
		},
	})

	// Execute the tool on the cloud plane.
	result, execErr := ne.executeTool(ctx, toolName, parameters, toolCtx, isLongRunning, progressCb)

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

// hydratePayloadFromDB reads image_urls and long-running metadata from the node
// record (these are stripped from Kafka events). Also checks for payload-level
// long_running override.
func (ne *NodeExecutor) hydratePayloadFromDB(ctx context.Context, nodeID string, payload map[string]interface{}) (map[string]interface{}, bool, int) {
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
	// Payload-level override.
	if lr, ok := payload["long_running"].(bool); ok && lr {
		isLongRunning = true
	}
	return payload, isLongRunning, heartbeatTimeoutSec
}

// resolveParameters resolves {{node_id.output.field}} references using completed
// parent node outputs. Returns the original parameters on resolution failure.
func (ne *NodeExecutor) resolveParameters(ctx context.Context, taskID string, parameters map[string]interface{}) map[string]interface{} {
	if ne.nodeRepo == nil {
		return parameters
	}
	resolved, err := ne.resolveNodeReferences(ctx, taskID, parameters)
	if err != nil {
		zap.L().Warn("Failed to resolve node references, using original parameters", zap.Error(err))
		return parameters
	}
	return resolved
}

// buildToolContext builds a ToolContext with retry count from the node record.
func (ne *NodeExecutor) buildToolContext(ctx context.Context, nodeID, taskID string, isLongRunning bool) tool.ToolContext {
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
	return toolCtx
}

// setupLongRunningHeartbeat wires up progress callbacks and a periodic heartbeat
// goroutine for long-running nodes. Returns a progress callback (nil for
// non-long-running) and a cancel function to stop the heartbeat goroutine.
func (ne *NodeExecutor) setupLongRunningHeartbeat(
	ctx context.Context,
	taskID, nodeID, idempotencyKey string,
	isLongRunning bool,
	heartbeatTimeoutSec int,
) (tool.ProgressCallback, context.CancelFunc) {
	hbInterval := ne.cfg.HeartbeatIntervalSec
	if hbInterval <= 0 {
		hbInterval = 30
	}
	hbTimeout := heartbeatTimeoutSec
	if hbTimeout <= 0 {
		hbTimeout = ne.cfg.HeartbeatTimeoutSec
	}
	if hbTimeout <= 0 {
		hbTimeout = 300
	}

	hbCtx, hbCancel := context.WithCancel(ctx)

	if !isLongRunning {
		return nil, hbCancel
	}

	zap.L().Info("Starting long-running node execution",
		zap.String("nodeId", nodeID),
		zap.Int("heartbeatInterval", hbInterval),
		zap.Int("heartbeatTimeout", hbTimeout),
	)

	// First heartbeat immediately.
	ne.publishProgress(taskID, nodeID, 0, "started")
	ne.publishHeartbeat(taskID, nodeID, idempotencyKey)

	// Progress callback for the tool.
	progressCb := func(_ context.Context, update tool.ProgressUpdate) {
		if update.Progress > 0 {
			ne.publishProgress(taskID, nodeID, update.Progress, update.Step)
		}
		if update.Checkpoint != nil {
			ne.publishCheckpoint(taskID, nodeID, update.Progress, update.Step, update.Checkpoint)
		}
	}

	// Periodic heartbeat goroutine.
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

	return progressCb, hbCancel
}

// executeTool looks up the tool, validates parameters, wires up progress
// reporting, selects the executor, and runs the tool.
func (ne *NodeExecutor) executeTool(
	ctx context.Context,
	toolName string,
	parameters map[string]interface{},
	toolCtx tool.ToolContext,
	isLongRunning bool,
	progressCb tool.ProgressCallback,
) (executor.ExecutionResult, error) {
	t, found := ne.toolRegistry.Get(toolName)
	if !found {
		return executor.ExecutionResult{Error: fmt.Sprintf("Tool not found: %s", toolName)}, nil
	}

	if !t.ValidateParameters(parameters) {
		return executor.ExecutionResult{Error: fmt.Sprintf("Invalid parameters for tool: %s", toolName)}, nil
	}
	manifest := ne.toolRegistry.GetManifest(toolName)

	// Wire progress reporter for long-running tasks.
	if isLongRunning && progressCb != nil {
		if pr, ok := t.(tool.ProgressReporter); ok {
			pr.SetProgressCallback(progressCb)
			zap.L().Info("ProgressReporter wired for tool", zap.String("tool", toolName))
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

		timeout := ne.executableToolTimeout(toolName, parameters, manifest)
		select {
		case toolResult := <-resultCh:
			if toolResult.Success {
				output, _ := json.Marshal(toolResult.Data)
				result = executor.ExecutionResult{ExitCode: 0, Stdout: output}
			} else {
				result = executor.ExecutionResult{ExitCode: 1, Error: toolResult.Error}
			}
		case <-time.After(timeout):
			result = executor.ExecutionResult{
				TimedOut: true,
				Error:    fmt.Sprintf("Tool execution timed out after %d seconds", int(timeout/time.Second)),
			}
		}
	} else {
		result.Error = "tool does not implement any executable interface"
	}

	return result, execErr
}

func (ne *NodeExecutor) executableToolTimeout(toolName string, parameters map[string]interface{}, manifest *tool.ToolManifest) time.Duration {
	timeoutSec := 0
	if toolName == "external" {
		if delegatedTool := firstString(parameters, nil, "tool", "capabilityTool"); delegatedTool != "" {
			if delegatedManifest := ne.toolRegistry.GetManifest(delegatedTool); delegatedManifest != nil {
				timeoutSec = normalizedManifestTimeoutSec(delegatedManifest.Timeout)
			}
		}
	}
	if timeoutSec <= 0 && manifest != nil {
		timeoutSec = normalizedManifestTimeoutSec(manifest.Timeout)
	}
	if timeoutSec <= 0 {
		timeoutSec = ne.cfg.ToolTimeoutSeconds
	}
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}
	return time.Duration(timeoutSec) * time.Second
}

func normalizedManifestTimeoutSec(timeoutSec int) int {
	const maxReasonableTimeoutSec = 86400 // 24 hours
	if timeoutSec > maxReasonableTimeoutSec {
		return timeoutSec / 1000
	}
	return timeoutSec
}

func (ne *NodeExecutor) dispatchLocalNode(
	ctx context.Context,
	event eventbus.Event,
	manifest *tool.ToolManifest,
	parameters map[string]interface{},
	idempotencyKey string,
) error {
	if ne.localDispatcher == nil {
		return fmt.Errorf("local execution requested for %s but local job dispatcher is not configured", manifest.Name)
	}
	command := manifest.LocalCommand
	if command == "" {
		return fmt.Errorf("local execution requested for %s but localCommand is empty", manifest.Name)
	}
	timeoutSec := manifest.Timeout
	timeoutSec = normalizedManifestTimeoutSec(timeoutSec)
	if timeoutSec <= 0 {
		timeoutSec = ne.cfg.ToolTimeoutSeconds
	}
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	projectID := firstString(parameters, event.Payload, "projectId", "project_id", "videoProjectId")
	if isRenderLocalCommand(manifest.Name, command) {
		checker := ne.renderChecker
		if checker == nil {
			checker = DefaultRenderDependencyChecker{}
		}
		if err := checker.CheckRenderDependencies(ctx, RenderDependencyCheckRequest{
			ProjectID: projectID,
			TaskID:    event.TaskID,
			NodeID:    event.NodeID,
			ToolName:  manifest.Name,
			Command:   command,
			Payload:   parameters,
		}); err != nil {
			return err
		}
	}

	job, err := ne.localDispatcher.DispatchLocalJob(ctx, localrunner.DispatchLocalJobRequest{
		ProjectID:      projectID,
		TaskID:         event.TaskID,
		NodeID:         event.NodeID,
		ToolName:       manifest.Name,
		Command:        command,
		Payload:        parameters,
		TimeoutSec:     timeoutSec,
		ArtifactPolicy: localArtifactPolicyForManifest(manifest),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return err
	}
	if ne.nodeRepo != nil && job != nil {
		output := map[string]interface{}{
			"executionPlane": tool.ExecutionPlaneLocal,
			"localJobId":     job.ID,
			"localCommand":   command,
			"toolName":       manifest.Name,
			"queuedAt":       time.Now().Format(time.RFC3339Nano),
		}
		if err := ne.nodeRepo.UpdateStatus(ctx, event.NodeID, model.NodeWaitingLocal, output, ""); err != nil {
			return fmt.Errorf("mark node waiting local: %w", err)
		}
	}
	zap.L().Info("Node dispatched to local runner",
		zap.String("taskId", event.TaskID),
		zap.String("nodeId", event.NodeID),
		zap.String("tool", manifest.Name),
		zap.String("command", command),
	)
	return nil
}

func isRenderLocalCommand(toolName, command string) bool {
	normalized := localrunner.NormalizeCommand(command)
	return toolName == "hyperframes_renderer" ||
		toolName == "artifact_packager" ||
		normalized == localrunner.CommandHyperFramesRender ||
		normalized == localrunner.CommandArtifactPackage
}

func truthy(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") ||
			strings.EqualFold(strings.TrimSpace(typed), "approved") ||
			strings.EqualFold(strings.TrimSpace(typed), "valid")
	default:
		return false
	}
}

func invalidArtifactState(value interface{}) bool {
	if value == nil {
		return false
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(text))
	return normalized != "" && normalized != "valid" && normalized != "approved"
}

func localArtifactPolicyForManifest(manifest *tool.ToolManifest) localrunner.LocalArtifactPolicy {
	location := manifest.ArtifactLocation
	if location == "" {
		location = tool.ArtifactLocationLocal
	}
	return localrunner.LocalArtifactPolicy{
		Location:            location,
		SyncMetadataToCloud: true,
		SyncFileToCloud:     location == tool.ArtifactLocationCloud || location == tool.ArtifactLocationBoth,
	}
}

func firstString(primary map[string]interface{}, secondary map[string]interface{}, keys ...string) string {
	for _, source := range []map[string]interface{}{primary, secondary} {
		for _, key := range keys {
			if source == nil {
				continue
			}
			if value, ok := source[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
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

	ne.publishEvent(eventbus.TopicNodeResult, idempotencyKey, event)
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

	ne.publishEvent(eventbus.TopicNodeResult, idempotencyKey, event)
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
	ne.publishEvent(eventbus.TopicProgress, hbKey, event)
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
	ne.publishEvent(eventbus.TopicProgress, taskID+"-"+nodeID+"-progress", event)
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
	ne.publishEvent(eventbus.TopicProgress, taskID+"-"+nodeID+"-checkpoint", event)
	zap.L().Info("Checkpoint saved",
		zap.String("nodeId", nodeID),
		zap.Float64("progress", progress),
	)
}

func (ne *NodeExecutor) publishEvent(topic, key string, event eventbus.Event) {
	if ne == nil || ne.producer == nil {
		return
	}
	_ = ne.producer.Publish(topic, key, event)
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

	node, val, ok := findNodeOutputField(ctx, nodeRepo, taskID, refNodeID, field)
	if !ok {
		zap.L().Warn("Cannot resolve node reference: referenced node output not found",
			zap.String("taskId", taskID),
			zap.String("refNodeID", refNodeID),
			zap.String("field", field),
			zap.String("ref", ref))
		return ref, false
	}

	if node != nil {
		zap.L().Debug("Resolved node reference",
			zap.String("taskId", taskID),
			zap.String("refNodeID", refNodeID),
			zap.String("resolvedNodeID", node.ID),
			zap.String("field", field))
	}
	return val, true
}

// findNodeOutput looks up a node and returns its output map, trying both the raw reference ID
// and the taskID-scoped version (taskID-refNodeID) for compatibility with both code paths.
// When exact match fails, it falls back to fuzzy-matching by taskID + refNodeID substring.
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

	// Fallback: fuzzy-match within the same task. PlanCompiler references use
	// plain step IDs (e.g., "knowledge_researcher") but the orchestrator creates
	// nodes with prefixed IDs (e.g., "td34b45e16e-knowledge_researcher_exec").
	nodes, err := nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, nil
	}
	for _, node := range nodes {
		if node == nil || node.Output == nil {
			continue
		}
		// Match if the node ID contains the refNodeID as a substring. This handles
		// both exec nodes (suffix _exec) and review nodes (suffix _review).
		if strings.Contains(node.ID, refNodeID) {
			return node, node.Output
		}
	}
	return nil, nil
}

func findNodeOutputField(ctx context.Context, nodeRepo repository.NodeRepo, taskID, refNodeID, field string) (*model.Node, interface{}, bool) {
	candidates := findNodeOutputCandidates(ctx, nodeRepo, taskID, refNodeID)
	for _, node := range candidates {
		if node == nil || node.Output == nil {
			continue
		}
		if val, ok := lookupOutputField(node.Output, field); ok {
			return node, val, true
		}
	}
	return nil, nil, false
}

func findNodeOutputCandidates(ctx context.Context, nodeRepo repository.NodeRepo, taskID, refNodeID string) []*model.Node {
	seen := map[string]bool{}
	candidates := make([]*model.Node, 0, 4)
	add := func(node *model.Node) {
		if node == nil || node.ID == "" || seen[node.ID] {
			return
		}
		seen[node.ID] = true
		candidates = append(candidates, node)
	}

	for _, candidate := range []string{refNodeID, taskID + "-" + refNodeID} {
		node, err := nodeRepo.FindByID(ctx, candidate)
		if err == nil {
			add(node)
		}
	}

	nodes, err := nodeRepo.FindByTaskID(ctx, taskID)
	if err == nil {
		for _, node := range nodes {
			if node != nil && strings.Contains(node.ID, refNodeID) {
				add(node)
			}
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := nodeRefCandidatePriority(candidates[i].ID, taskID, refNodeID)
		right := nodeRefCandidatePriority(candidates[j].ID, taskID, refNodeID)
		if left != right {
			return left < right
		}
		return candidates[i].ID < candidates[j].ID
	})

	return candidates
}

func nodeRefCandidatePriority(nodeID, taskID, refNodeID string) int {
	switch {
	case nodeID == refNodeID || nodeID == taskID+"-"+refNodeID:
		return 0
	case isExecNodeForRef(nodeID, refNodeID):
		return 1
	default:
		return 2
	}
}

func isExecNodeForRef(nodeID, refNodeID string) bool {
	return strings.HasSuffix(nodeID, refNodeID+"_exec") ||
		strings.HasSuffix(nodeID, refNodeID+"-exec") ||
		strings.Contains(nodeID, refNodeID+"_exec_") ||
		strings.Contains(nodeID, refNodeID+"-exec-")
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

		_, val, ok := findNodeOutputField(ctx, nodeRepo, taskID, refNodeID, field)
		if !ok {
			zap.L().Warn("Cannot resolve node reference: node not found",
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

func lookupOutputField(output map[string]interface{}, field string) (interface{}, bool) {
	if output == nil || strings.TrimSpace(field) == "" {
		return nil, false
	}
	if val, ok := output[field]; ok {
		return val, true
	}
	for _, payload := range structuredOutputPayloads(output) {
		if val, ok := payload[field]; ok {
			return val, true
		}
	}
	return nil, false
}

func structuredOutputPayloads(output map[string]interface{}) []map[string]interface{} {
	payloads := make([]map[string]interface{}, 0, 4)
	if parsed := parseObjectPayload(output["stdout"]); parsed != nil {
		payloads = append(payloads, parsed)
		if embedded := parseObjectPayload(parsed["content"]); embedded != nil {
			payloads = append(payloads, embedded)
		}
		if pkg := parseObjectPayload(parsed["package"]); pkg != nil {
			payloads = append(payloads, pkg)
		}
	}
	if embedded := parseObjectPayload(output["content"]); embedded != nil {
		payloads = append(payloads, embedded)
	}
	if pkg := parseObjectPayload(output["package"]); pkg != nil {
		payloads = append(payloads, pkg)
	}
	return payloads
}

func parseObjectPayload(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &parsed); err != nil {
			return nil
		}
		return parsed
	default:
		return nil
	}
}
