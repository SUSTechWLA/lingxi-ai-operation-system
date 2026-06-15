package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/common/metadata"
	"github.com/tangying-ai/aios-core/internal/eventbus"
	"github.com/tangying-ai/aios-core/internal/model"
	"github.com/tangying-ai/aios-core/internal/model/repository"
	"github.com/tangying-ai/aios-core/internal/outbox"
)

type StateService struct {
	nodeRepo          repository.NodeRepo
	taskRepo          repository.TaskRepo
	depRepo           repository.DependencyRepo
	contextRepo       repository.ContextRepo
	eventSaver        outbox.EventSaver
	retryPolicy       *RetryPolicy
	dependencyChecker *DependencyChecker
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

	node.Status = model.NodeReady
	if node.IdempotencyKey == "" {
		node.IdempotencyKey = node.TaskID + "-" + node.ID
	}
	if err := s.nodeRepo.Save(ctx, node); err != nil {
		return err
	}

	s.recordContext(ctx, node.TaskID, node.ID, model.ContextNodeReady, "StateMachine", "初始节点就绪（无依赖），进入 READY 状态", buildNodeMetadata(node, model.NodeReady))

	payload := buildEventPayload(node.Input, node.Name)

	_ = s.eventSaver.SaveEvent(ctx, "node", node.ID, eventbus.TopicNodeReady, eventbus.Event{
		TaskID:         node.TaskID,
		NodeID:         node.ID,
		Type:           string(node.Type),
		Payload:        payload,
		TraceID:        node.TaskID + "-" + node.ID,
		IdempotencyKey: node.IdempotencyKey,
	})

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
		ctxType = model.ContextType("NODE_SKIPPED")
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
