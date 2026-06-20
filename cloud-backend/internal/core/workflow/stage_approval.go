package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

var (
	ErrRunNotFound        = errors.New("workflow run not found")
	ErrRunProjectMismatch = errors.New("workflow run does not belong to project")
	ErrStageNotFound      = errors.New("workflow stage approval node not found")
	ErrStageNotControl    = errors.New("workflow stage is not a control node")
	ErrStageNotReady      = errors.New("workflow stage is not ready for approval")
)

type stageApprovalRunStore interface {
	FindByID(ctx context.Context, id string) (*WorkflowRun, error)
	FindByProject(ctx context.Context, projectID string) ([]*WorkflowRun, error)
}

type stageApprovalNodeStore interface {
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
}

type stageApprovalStateMachine interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
}

// StageApprovalService maps domain-level workflow stages to their underlying
// CONTROL nodes, so clients do not need to know node IDs.
type StageApprovalService struct {
	runs         stageApprovalRunStore
	nodes        stageApprovalNodeStore
	stateMachine stageApprovalStateMachine
}

func NewStageApprovalService(runs stageApprovalRunStore, nodes stageApprovalNodeStore, stateMachine stageApprovalStateMachine) *StageApprovalService {
	return &StageApprovalService{runs: runs, nodes: nodes, stateMachine: stateMachine}
}

func (s *StageApprovalService) ApproveStage(ctx context.Context, projectID, runID, stageName string, output map[string]interface{}) (*model.Node, error) {
	if s == nil || s.runs == nil || s.nodes == nil || s.stateMachine == nil {
		return nil, fmt.Errorf("stage approval service is not configured")
	}

	stageName = strings.TrimSpace(stageName)
	if stageName == "" {
		return nil, ErrStageNotFound
	}

	run, err := s.findRun(ctx, projectID, runID)
	if err != nil {
		return nil, err
	}

	nodes, err := s.nodes.FindByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, fmt.Errorf("find workflow task nodes: %w", err)
	}

	node, err := findApprovalNode(nodes, stageName)
	if err != nil {
		return nil, err
	}
	if node.Status == model.NodeSuccess {
		return node, nil
	}
	if node.Status != model.NodeReady {
		return nil, fmt.Errorf("%w: %s is %s", ErrStageNotReady, node.ID, node.Status)
	}

	if output == nil {
		output = map[string]interface{}{}
	}
	if err := s.stateMachine.OnSuccess(ctx, node.ID, output); err != nil {
		return nil, fmt.Errorf("approve workflow stage: %w", err)
	}
	return node, nil
}

func (s *StageApprovalService) findRun(ctx context.Context, projectID, runID string) (*WorkflowRun, error) {
	if runID != "" {
		run, err := s.runs.FindByID(ctx, runID)
		if err != nil {
			return nil, fmt.Errorf("find workflow run: %w", err)
		}
		if run == nil {
			return nil, ErrRunNotFound
		}
		if run.ProjectID != projectID {
			return nil, ErrRunProjectMismatch
		}
		return run, nil
	}

	runs, err := s.runs.FindByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("find project workflow runs: %w", err)
	}
	if len(runs) == 0 || runs[0] == nil {
		return nil, ErrRunNotFound
	}
	return runs[0], nil
}

func findApprovalNode(nodes []*model.Node, stageName string) (*model.Node, error) {
	for _, node := range nodes {
		if node == nil || node.ID != stageName {
			continue
		}
		if node.Type != model.NodeTypeControl {
			return nil, fmt.Errorf("%w: %s", ErrStageNotControl, stageName)
		}
		return node, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrStageNotFound, stageName)
}
