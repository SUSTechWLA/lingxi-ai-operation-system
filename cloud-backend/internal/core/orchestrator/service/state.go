package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/metadata"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/outbox"
)

// TransitionHook is an optional callback invoked after every node transition.
// It receives the node (post-transition) and its new status.
type TransitionHook func(ctx context.Context, node *model.Node, newStatus model.NodeStatus)

type StateService struct {
	nodeRepo          repository.NodeRepo
	taskRepo          repository.TaskRepo
	depRepo           repository.DependencyRepo
	contextRepo       repository.ContextRepo
	eventSaver        outbox.EventSaver
	retryPolicy       *RetryPolicy
	dependencyChecker *DependencyChecker
	onTransition      TransitionHook
}

func NewStateService(
	nodeRepo repository.NodeRepo,
	taskRepo repository.TaskRepo,
	depRepo repository.DependencyRepo,
	contextRepo repository.ContextRepo,
	eventSaver outbox.EventSaver,
) *StateService {
	return &StateService{
		nodeRepo:    nodeRepo,
		taskRepo:    taskRepo,
		depRepo:     depRepo,
		contextRepo: contextRepo,
		eventSaver:  eventSaver,
		retryPolicy: NewRetryPolicy(),
	}
}

func (s *StateService) SetDependencyChecker(dc *DependencyChecker) {
	s.dependencyChecker = dc
}

// SetTransitionHook registers an optional callback that is invoked after every
// successful node transition. This allows higher-level services (e.g. workflow
// checkpointing) to observe state changes without coupling to the orchestrator.
func (s *StateService) SetTransitionHook(hook TransitionHook) {
	s.onTransition = hook
}

func (s *StateService) TransitionNode(ctx context.Context, nodeID string, newStatus model.NodeStatus, output map[string]interface{}, errMsg string) (*model.Node, error) {
	node, err := s.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find node: %w", err)
	}
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	// Skip if already in target status (idempotency)
	if node.Status == newStatus {
		zap.L().Debug("Node already in target status, skipping transition",
			zap.String("nodeId", nodeID),
			zap.String("status", string(newStatus)),
		)
		return node, nil
	}

	zap.L().Info("Node transitioning",
		zap.String("nodeId", nodeID),
		zap.String("oldStatus", string(node.Status)),
		zap.String("newStatus", string(newStatus)),
	)

	node.Status = newStatus
	if output != nil {
		node.Output = output
	}
	if errMsg != "" {
		node.ErrorMessage = errMsg
	}

	// Record execution timestamps
	now := time.Now()
	if newStatus == model.NodeRunning {
		node.StartedAt = &now
	}
	if newStatus == model.NodeSuccess || newStatus == model.NodeFailed {
		now := time.Now()
		node.CompletedAt = &now
	}

	if err := s.nodeRepo.Save(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to save node: %w", err)
	}

	s.recordContextForTransition(ctx, node, newStatus)

	// Notify higher-level observers (e.g. workflow checkpoints).
	if s.onTransition != nil {
		s.onTransition(ctx, node, newStatus)
	}

	// CONTROL or REVIEW_GATE node: auto-pause task when node becomes READY (human review required)
	if (node.Type == model.NodeTypeControl || node.Type == model.NodeTypeReviewGate) && newStatus == model.NodeReady {
		s.handleControlNodeReady(ctx, node)
	}

	// Event-driven: immediately try to make CREATED nodes ready
	if newStatus == model.NodeCreated {
		s.TryMakeReady(ctx, node)
	}

	return node, nil
}

// TryMakeReady immediately checks dependencies and transitions to READY if met
func (s *StateService) TryMakeReady(ctx context.Context, node *model.Node) {
	if node.Status != model.NodeCreated {
		return
	}
	met, err := s.CheckDependenciesMet(ctx, node.ID)
	if err != nil {
		zap.L().Error("TryMakeReady: failed to check dependencies", zap.Error(err))
		return
	}
	if met {
		if err := s.InitializeNodeReady(ctx, node); err != nil {
			zap.L().Error("TryMakeReady: failed to initialize node as ready", zap.Error(err))
		}
	}
}

func (s *StateService) TransitionTask(ctx context.Context, taskID string, newStatus model.TaskStatus) error {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status == newStatus {
		zap.L().Debug("Task already in target status, skipping transition",
			zap.String("taskId", taskID),
			zap.String("status", string(newStatus)),
		)
		return nil
	}

	oldStatus := task.Status
	zap.L().Info("Task transitioning",
		zap.String("taskId", taskID),
		zap.String("oldStatus", string(oldStatus)),
		zap.String("newStatus", string(newStatus)),
	)

	task.Status = newStatus
	if newStatus != model.TaskPaused {
		task.PauseReason = ""
	}
	if err := s.taskRepo.Save(ctx, task); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	switch newStatus {
	case model.TaskSuccess:
		s.recordContext(ctx, taskID, "", model.ContextTaskSuccess, "StateMachine", "任务全部节点执行完毕，任务完成", nil)
		_ = s.eventSaver.SaveEvent(ctx, "task", taskID, eventbus.TopicTaskCompleted, eventbus.Event{
			TaskID: taskID, Status: "SUCCESS",
		})
	case model.TaskFailed:
		s.recordContext(ctx, taskID, "", model.ContextTaskFailed, "StateMachine", "任务执行失败", nil)
		_ = s.eventSaver.SaveEvent(ctx, "task", taskID, eventbus.TopicTaskFailed, eventbus.Event{
			TaskID: taskID, Status: "FAILED",
		})
	case model.TaskPaused:
		zap.L().Info("Task paused", zap.String("taskId", taskID))
	case model.TaskRunning:
		if oldStatus == model.TaskPaused {
			zap.L().Info("Task resumed", zap.String("taskId", taskID))
		}
	}

	return nil
}

// handleControlNodeReady pauses the task when a CONTROL node becomes READY,
// signaling that human review is required before the workflow proceeds.
// For quality gates with autoApproveWhenPassed=true, it auto-approves when the
// quality checker passes, so the workflow continues without human intervention.
func (s *StateService) handleControlNodeReady(ctx context.Context, node *model.Node) {
	// Auto-approve quality gates when the checker score meets the threshold.
	if s.tryAutoApproveQualityGate(ctx, node) {
		return
	}

	task, err := s.taskRepo.FindByID(ctx, node.TaskID)
	if err != nil || task == nil {
		zap.L().Error("Failed to find task for CONTROL node pause", zap.Error(err))
		return
	}

	// Only pause if task is currently RUNNING
	if task.Status != model.TaskRunning {
		zap.L().Debug("Task not in RUNNING state, skipping CONTROL node pause",
			zap.String("taskId", node.TaskID),
			zap.String("taskStatus", string(task.Status)),
		)
		return
	}

	task.Status = model.TaskPaused
	task.PauseReason = fmt.Sprintf("Control node '%s' (%s) requires review", node.Name, node.ID)
	if err := s.taskRepo.Save(ctx, task); err != nil {
		zap.L().Error("Failed to pause task for CONTROL node", zap.Error(err))
		return
	}

	s.recordContext(ctx, node.TaskID, node.ID,
		model.ContextNodeReviewRequired,
		"StateMachine",
		fmt.Sprintf("Control node '%s' ready for review — task paused", node.Name),
		nil,
	)

	zap.L().Info("Task paused for CONTROL node review",
		zap.String("taskId", node.TaskID),
		zap.String("nodeId", node.ID),
		zap.String("nodeName", node.Name),
	)
}

// tryAutoApproveQualityGate checks if a REVIEW_GATE node is a quality gate
// with autoApproveWhenPassed, and if the quality checker passed with a
// sufficient score, auto-approves the gate and resumes the task.
// Returns true if the gate was auto-approved (caller should not pause).
func (s *StateService) tryAutoApproveQualityGate(ctx context.Context, gateNode *model.Node) bool {
	if gateNode == nil || gateNode.Input == nil {
		return false
	}
	reviewPhase, _ := gateNode.Input["reviewPhase"].(string)
	if reviewPhase != "quality_gate" {
		return false
	}
	autoApprove, _ := gateNode.Input["autoApproveWhenPassed"].(bool)
	if !autoApprove {
		return false
	}

	// Find the quality checker node.
	checkerNodeID, _ := gateNode.Input["qualityCheckerNode"].(string)
	if checkerNodeID == "" {
		checkerNodeID, _ = gateNode.Input["checkerStep"].(string)
	}
	if checkerNodeID == "" {
		zap.L().Warn("Quality gate has autoApproveWhenPassed but no checkerNodeID",
			zap.String("gateNodeId", gateNode.ID))
		return false
	}
	checker, err := s.nodeRepo.FindByID(ctx, checkerNodeID)
	if err != nil || checker == nil {
		// Try finding by original node ID or substring match.
		allNodes, findErr := s.nodeRepo.FindByTaskID(ctx, gateNode.TaskID)
		if findErr == nil {
			for _, n := range allNodes {
				if n == nil {
					continue
				}
				// Match by scoped ID (e.g. "tXXXX-script_quality_checker")
				if strings.HasSuffix(n.ID, checkerNodeID) || strings.HasSuffix(checkerNodeID, n.ID) {
					checker = n
					break
				}
			}
		}
	}
	if checker == nil {
		zap.L().Warn("Quality gate auto-approve: checker node not found",
			zap.String("gateNodeId", gateNode.ID),
			zap.String("checkerNodeId", checkerNodeID))
		return false
	}

	if checker.Status != model.NodeSuccess {
		zap.L().Info("Quality gate auto-approve: checker not yet succeeded",
			zap.String("gateNodeId", gateNode.ID),
			zap.String("checkerNodeId", checker.ID),
			zap.String("checkerStatus", string(checker.Status)))
		return false
	}

	// Read the checker output.
	score := 0.0
	passed := false
	if checker.Output != nil {
		if s, ok := checker.Output["score"].(float64); ok {
			score = s
		}
		if p, ok := checker.Output["passed"].(bool); ok {
			passed = p
		}
	}

	minScore := 85.0
	if ms, ok := gateNode.Input["minScore"].(float64); ok {
		minScore = ms
	}
	if gateScore, ok := gateNode.Input["minScore"].(int); ok {
		minScore = float64(gateScore)
	}

	gateResult := map[string]interface{}{
		"autoApproved": true,
		"score":        score,
		"minScore":     minScore,
		"passed":       passed,
		"phase":        "quality_gate",
	}

	if passed && score >= minScore {
		gateResult["gateResult"] = "auto_approved"
		_, err := s.TransitionNode(ctx, gateNode.ID, model.NodeSuccess, gateResult, "")
		if err != nil {
			zap.L().Error("Quality gate auto-approve: transition failed", zap.Error(err))
			return false
		}

		// Resume the task if it was paused by a prior review gate.
		task, taskErr := s.taskRepo.FindByID(ctx, gateNode.TaskID)
		if taskErr == nil && task != nil && task.Status == model.TaskPaused {
			_ = s.TransitionTask(ctx, gateNode.TaskID, model.TaskRunning)
		}

		zap.L().Info("Quality gate auto-approved",
			zap.String("gateNodeId", gateNode.ID),
			zap.Float64("score", score),
			zap.Float64("minScore", minScore))
		return true
	}

	// Quality check failed — gate remains open for human review.
	gateResult["gateResult"] = "failed"
	_, _ = s.TransitionNode(ctx, gateNode.ID, model.NodeReady, gateResult, "")
	zap.L().Warn("Quality gate did not pass, waiting for human review",
		zap.String("gateNodeId", gateNode.ID),
		zap.Float64("score", score),
		zap.Float64("minScore", minScore))
	return false
}

func (s *StateService) CheckDependenciesMet(ctx context.Context, nodeID string) (bool, error) {
	deps, err := s.depRepo.FindByChildID(ctx, nodeID)
	if err != nil {
		return false, err
	}

	for _, dep := range deps {
		parent, err := s.nodeRepo.FindByID(ctx, dep.ParentNodeID)
		if err != nil || parent == nil {
			return false, nil
		}
		// SUCCESS or SKIPPED satisfies dependency
		if parent.Status != model.NodeSuccess && parent.Status != model.NodeSkipped {
			return false, nil
		}
	}

	return true, nil
}

func (s *StateService) CheckTaskCompleted(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status != model.NodeSuccess && n.Status != model.NodeSkipped {
			return false, nil
		}
	}
	return true, nil
}

func (s *StateService) CheckTaskFailed(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status == model.NodeFailed && n.RetryCount >= n.MaxRetry {
			return true, nil
		}
	}
	return false, nil
}

func (s *StateService) HasRetryableNodes(ctx context.Context, taskID string) (bool, error) {
	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}

	for _, n := range nodes {
		if n.Status == model.NodeFailed && s.retryPolicy.ShouldRetry(n.RetryCount, n.MaxRetry) {
			return true, nil
		}
	}
	return false, nil
}

func (s *StateService) InitializeNodeReady(ctx context.Context, node *model.Node) error {
	if node.Status != model.NodeCreated {
		return nil
	}

	met, err := s.CheckDependenciesMet(ctx, node.ID)
	if err != nil {
		return err
	}
	if !met {
		return nil
	}

	if node.IdempotencyKey == "" {
		node.IdempotencyKey = node.TaskID + "-" + node.ID
	}

	payload := buildEventPayload(node.Input, node.Name)
	readyEvent := eventbus.Event{
		TaskID:         node.TaskID,
		NodeID:         node.ID,
		Type:           string(node.Type),
		Payload:        payload,
		TraceID:        node.TaskID + "-" + node.ID,
		IdempotencyKey: node.IdempotencyKey,
	}
	if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
		if atomicSaver, ok := s.eventSaver.(outbox.AtomicNodeReadySaver); ok {
			applied, atomicErr := atomicSaver.SaveNodeReadyEvent(ctx, node.ID, node.IdempotencyKey, readyEvent)
			if atomicErr != nil {
				return atomicErr
			}
			if !applied {
				return nil
			}
			node.Status = model.NodeReady
			s.recordContext(ctx, node.TaskID, node.ID, model.ContextNodeReady, "StateMachine", "初始节点就绪（无依赖），进入 READY 状态", buildNodeMetadata(node, model.NodeReady))
			zap.L().Info("Node is READY (dependencies met)", zap.String("nodeId", node.ID))
			return nil
		}
	}

	node.Status = model.NodeReady
	if err := s.nodeRepo.Save(ctx, node); err != nil {
		return err
	}

	s.recordContext(ctx, node.TaskID, node.ID, model.ContextNodeReady, "StateMachine", "初始节点就绪（无依赖），进入 READY 状态", buildNodeMetadata(node, model.NodeReady))

	if node.Type == model.NodeTypeControl || node.Type == model.NodeTypeReviewGate {
		s.handleControlNodeReady(ctx, node)
		return nil
	}

	if err := s.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeReady, readyEvent); err != nil {
		return err
	}

	zap.L().Info("Node is READY (dependencies met)", zap.String("nodeId", node.ID))
	return nil
}

func (s *StateService) recordContextForTransition(ctx context.Context, node *model.Node, newStatus model.NodeStatus) {
	var ctxType model.ContextType
	var message string

	switch newStatus {
	case model.NodeReady:
		ctxType = model.ContextNodeReady
		message = "节点状态就绪（CREATED → READY），等待 Worker 调度"
	case model.NodeRunning:
		ctxType = model.ContextNodeScheduled
		message = "节点开始调度（READY → RUNNING），Worker 已收到事件"
	case model.NodeSuccess:
		ctxType = model.ContextNodeSuccess
		message = "节点执行成功（RUNNING → SUCCESS）"
	case model.NodeFailed:
		ctxType = model.ContextNodeFailed
		message = "节点执行失败（RUNNING → FAILED）"
	case model.NodeRetrying:
		ctxType = model.ContextNodeRetry
		message = "节点即将重试"
	case model.NodeSkipped:
		ctxType = model.ContextNodeSkipped
		message = "节点已跳过（条件未满足）"
	default:
		return
	}

	s.recordContext(ctx, node.TaskID, node.ID, ctxType, "StateMachine", message, buildNodeMetadata(node, newStatus))
}

func buildNodeMetadata(node *model.Node, status model.NodeStatus) map[string]interface{} {
	meta := make(map[string]interface{})

	// Tool name for context
	if node.Name != "" {
		meta["tool"] = node.Name
	}

	// Summarize input parameters
	if inputSummary := metadata.BuildInputSummary(node.Input); len(inputSummary) > 0 {
		meta["input"] = inputSummary
	}

	// Summarize output for terminal states
	if node.Output != nil && (status == model.NodeSuccess || status == model.NodeFailed) {
		if stdout, ok := node.Output["stdout"].(string); ok {
			if outputSummary := metadata.BuildOutputSummary(stdout); len(outputSummary) > 0 {
				meta["output"] = outputSummary
			}
		}
		// Always include execution metrics for observability
		if v, ok := node.Output["durationMs"]; ok {
			meta["durationMs"] = v
		}
		if v, ok := node.Output["exitCode"]; ok {
			meta["exitCode"] = v
		}
		// Include error from output if present
		if errStr, ok := node.Output["error"].(string); ok && errStr != "" {
			meta["error"] = errStr
		}
	}

	if len(meta) == 0 {
		return nil
	}
	return meta
}

// buildEventPayload copies node input into an event payload, skipping image_urls
// since base64-encoded images are too large for Kafka messages.
func buildEventPayload(input map[string]interface{}, name string) map[string]interface{} {
	payload := make(map[string]interface{}, len(input)+1)
	for k, v := range input {
		if k == "image_urls" {
			continue
		}
		payload[k] = v
	}
	payload["name"] = name
	return payload
}

func (s *StateService) recordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) {
	c := &model.Context{
		ContextType:  ctxType,
		TaskID:       taskID,
		NodeID:       nodeID,
		SourceModule: sourceModule,
		Message:      message,
		Metadata:     metadata,
	}
	if err := s.contextRepo.Save(ctx, c); err != nil {
		zap.L().Warn("Failed to record context", zap.Error(err))
	}
}
