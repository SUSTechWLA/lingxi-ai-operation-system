package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model/repository"
)

type OrchestratorService struct {
	taskRepo      repository.TaskRepo
	nodeRepo      repository.NodeRepo
	depRepo       repository.DependencyRepo
	dagValidator  *DAGValidator
	stateService  *StateService
	contextRepo   repository.ContextRepo
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

func (s *OrchestratorService) CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error) {
	task := repository.NewTaskFromMap(input)
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

	s.recordContext(ctx, taskID, "", model.ContextDagValidated, "Orchestrator", "DAG 结构校验通过（无环、无重复节点）", nil)

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

	s.recordContext(ctx, taskID, "", model.ContextDagSubmitted, "Orchestrator", "DAG 已提交，节点写入数据库", nil)

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

	result := map[string]interface{}{
		"taskId":   task.ID,
		"status":   task.Status,
		"input":    task.Input,
		"output":   task.Output,
		"createdAt": task.CreatedAt,
		"nodes":    nodes,
	}

	return result, nil
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

func (s *OrchestratorService) recordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) {
	c := &model.Context{
		ContextType: ctxType,
		TaskID:      taskID,
		NodeID:      nodeID,
		SourceModule: sourceModule,
		Message:     message,
		Metadata:    metadata,
	}
	if err := s.contextRepo.Save(ctx, c); err != nil {
		zap.L().Warn("Failed to record context", zap.Error(err))
	}
}
