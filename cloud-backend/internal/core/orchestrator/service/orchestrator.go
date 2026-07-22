package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
)

type OrchestratorService struct {
	taskRepo     repository.TaskRepo
	nodeRepo     repository.NodeRepo
	depRepo      repository.DependencyRepo
	dagValidator *DAGValidator
	stateService *StateService
	contextRepo  repository.ContextRepo
}

func NewOrchestratorService(
	taskRepo repository.TaskRepo,
	nodeRepo repository.NodeRepo,
	depRepo repository.DependencyRepo,
	contextRepo repository.ContextRepo,
	stateService *StateService,
) *OrchestratorService {
	return &OrchestratorService{
		taskRepo:     taskRepo,
		nodeRepo:     nodeRepo,
		depRepo:      depRepo,
		contextRepo:  contextRepo,
		stateService: stateService,
		dagValidator: NewDAGValidator(),
	}
}

func (s *OrchestratorService) CreateTask(ctx context.Context, userID string, input map[string]interface{}) (*model.Task, error) {
	task, err := repository.NewTaskForUser(userID, input)
	if err != nil {
		return nil, err
	}
	if err := s.taskRepo.Save(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	s.recordContext(ctx, task.ID, "", model.ContextTaskCreated, "Orchestrator", "任务创建成功，等待 DAG 提交", nil)
	zap.L().Info("Created task", zap.String("taskId", task.ID))

	return task, nil
}

func (s *OrchestratorService) SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to find task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	zap.L().Info("Submitting DAG for task", zap.String("taskId", taskID))

	if err := s.dagValidator.Validate(dagReq); err != nil {
		return fmt.Errorf("DAG validation failed: %w", err)
	}

	// Print DAG structure: all nodes and their dependency edges
	nodeInfos := make([]string, 0, len(dagReq.Nodes))
	for _, n := range dagReq.Nodes {
		nodeInfos = append(nodeInfos, fmt.Sprintf("%s(%s:%s)", n.ID, n.Type, n.Name))
	}
	edges := make([]string, 0, len(dagReq.Edges))
	for _, e := range dagReq.Edges {
		edges = append(edges, fmt.Sprintf("%s → %s", e.From, e.To))
	}
	zap.L().Info("DAG structure",
		zap.String("taskId", taskID),
		zap.Strings("nodes", nodeInfos),
		zap.Strings("edges", edges),
	)

	s.recordContext(ctx, taskID, "", model.ContextDagValidated, "Orchestrator", "DAG 结构校验通过（无环、无重复节点）", map[string]interface{}{
		"nodes": nodeInfos,
		"edges": edges,
	})

	for _, nodeReq := range dagReq.Nodes {
		now := time.Now()
		node := &model.Node{
			ID:             nodeReq.ID,
			TaskID:         taskID,
			Type:           model.NodeType(nodeReq.Type),
			Name:           nodeReq.Name,
			Status:         model.NodeCreated,
			Input:          nodeReq.Input,
			Condition:      nodeReq.Condition,
			IdempotencyKey: taskID + "-" + nodeReq.ID,
			RetryCount:     0,
			MaxRetry:       3,
			Priority:       5,
			WorkerGroup:    "default",
			Version:        0,
			CreatedAt:      now,
		}
		if nodeReq.MaxRetry != nil {
			node.MaxRetry = *nodeReq.MaxRetry
		}
		if nodeReq.Priority != nil {
			node.Priority = *nodeReq.Priority
		}
		if nodeReq.WorkerGroup != "" {
			node.WorkerGroup = nodeReq.WorkerGroup
		}
		if nodeReq.LongRunning {
			node.LongRunning = true
		}
		if nodeReq.HeartbeatTimeoutSec != nil {
			node.HeartbeatTimeoutSec = *nodeReq.HeartbeatTimeoutSec
		}

		if err := s.nodeRepo.Save(ctx, node); err != nil {
			return fmt.Errorf("failed to save node %s: %w", nodeReq.ID, err)
		}
	}

	for _, edge := range dagReq.Edges {
		dep := &model.NodeDependency{
			ParentNodeID: edge.From,
			ChildNodeID:  edge.To,
		}
		if err := s.depRepo.Save(ctx, dep); err != nil {
			return fmt.Errorf("failed to save dependency: %w", err)
		}
	}

	if err := s.stateService.TransitionTask(ctx, taskID, model.TaskRunning); err != nil {
		return fmt.Errorf("failed to transition task to RUNNING: %w", err)
	}

	s.recordContext(ctx, taskID, "", model.ContextDagSubmitted, "Orchestrator", "DAG 已提交，节点写入数据库", map[string]interface{}{
		"nodes": nodeInfos,
		"edges": edges,
	})

	s.initializeReadyNodes(ctx, dagReq)

	zap.L().Info("DAG submitted for task", zap.String("taskId", taskID))
	return nil
}

func (s *OrchestratorService) GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error) {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil
	}

	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	edges, err := s.depRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	edgeStrs := make([]string, 0, len(edges))
	for _, e := range edges {
		edgeStrs = append(edgeStrs, fmt.Sprintf("%s → %s", e.ParentNodeID, e.ChildNodeID))
	}

	// Strip large binary fields (e.g. image_urls) from node input for display
	sanitizedNodes := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		sanitized := map[string]interface{}{
			"id":           n.ID,
			"taskId":       n.TaskID,
			"type":         string(n.Type),
			"name":         n.Name,
			"status":       string(n.Status),
			"output":       sanitizeNodeOutput(n.Output),
			"errorMessage": n.ErrorMessage,
			"condition":    n.Condition,
			"retryCount":   n.RetryCount,
			"maxRetry":     n.MaxRetry,
			"priority":     n.Priority,
			"createdAt":    n.CreatedAt,
			"startedAt":    n.StartedAt,
			"completedAt":  n.CompletedAt,
		}
		if n.Input != nil {
			cleanInput := make(map[string]interface{}, len(n.Input))
			for k, v := range n.Input {
				if k == "image_urls" {
					urls, ok := v.([]interface{})
					if !ok {
						continue
					}
					cleanInput["image_count"] = len(urls)
					continue
				}
				cleanInput[k] = v
			}
			sanitized["input"] = cleanInput
		}
		sanitizedNodes = append(sanitizedNodes, sanitized)
	}

	result := map[string]interface{}{
		"taskId":    task.ID,
		"status":    string(task.Status),
		"input":     task.Input,
		"output":    task.Output,
		"createdAt": task.CreatedAt,
		"nodes":     sanitizedNodes,
		"edges":     edgeStrs,
	}

	return result, nil
}

// GetTaskProgress returns aggregated progress information for a task.
func (s *OrchestratorService) GetTaskProgress(ctx context.Context, taskID string) (*model.TaskProgressResponse, error) {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	nodes, err := s.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	totalNodes := len(nodes)
	completedNodes := 0
	var totalProgress float64
	nodeInfos := make([]model.NodeProgressInfo, 0, len(nodes))

	for _, n := range nodes {
		switch n.Status {
		case model.NodeSuccess, model.NodeSkipped:
			completedNodes++
			n.Progress = 1.0
			totalProgress += 1.0
		case model.NodeFailed:
			// Failed nodes count as 0 progress
			totalProgress += 0
		case model.NodeRunning:
			totalProgress += n.Progress
		}

		info := model.NodeProgressInfo{
			NodeID:      n.ID,
			Name:        n.Name,
			Type:        string(n.Type),
			Status:      string(n.Status),
			Progress:    n.Progress,
			CurrentStep: n.CurrentStep,
			HeartbeatAt: n.HeartbeatAt,
			StartedAt:   n.StartedAt,
			Error:       n.ErrorMessage,
		}
		nodeInfos = append(nodeInfos, info)
	}

	var overallProgress float64
	if totalNodes > 0 {
		overallProgress = totalProgress / float64(totalNodes)
	}

	return &model.TaskProgressResponse{
		TaskID:         taskID,
		Status:         string(task.Status),
		Progress:       overallProgress,
		TotalNodes:     totalNodes,
		CompletedNodes: completedNodes,
		Nodes:          nodeInfos,
	}, nil
}

func (s *OrchestratorService) GetRecentTask(ctx context.Context) (*model.Task, error) {
	return s.taskRepo.FindRecent(ctx)
}

func (s *OrchestratorService) initializeReadyNodes(ctx context.Context, dagReq *model.DAGRequest) {
	nodesWithDeps := make(map[string]bool)
	for _, edge := range dagReq.Edges {
		nodesWithDeps[edge.To] = true
	}

	for _, nodeReq := range dagReq.Nodes {
		if !nodesWithDeps[nodeReq.ID] {
			node, err := s.nodeRepo.FindByID(ctx, nodeReq.ID)
			if err != nil || node == nil {
				continue
			}
			if node.Status == model.NodeCreated {
				_ = s.stateService.InitializeNodeReady(ctx, node)
			}
		}
	}
}

// sanitizeNodeOutput replaces large base64 data in node output with readable summaries.
func sanitizeNodeOutput(output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return nil
	}
	cleaned := make(map[string]interface{}, len(output))
	for k, v := range output {
		if k == "stdout" {
			if s, ok := v.(string); ok {
				cleaned[k] = summarizeStdout(s)
				continue
			}
		}
		cleaned[k] = v
	}
	return cleaned
}

func summarizeStdout(stdout string) string {
	// Count base64 data URLs (typical pattern: "data:image/jpeg;base64,...")
	base64Count := 0
	for {
		idx := strings.Index(stdout, `"data:image/`)
		if idx < 0 {
			break
		}
		base64Count++
		stdout = stdout[:idx] + stdout[idx+1:] // remove opening quote so we find the next
	}
	if base64Count > 0 {
		return fmt.Sprintf("[%d keyframe images (base64 data URLs omitted for readability)]", base64Count)
	}
	return stdout
}

func (s *OrchestratorService) recordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) {
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
