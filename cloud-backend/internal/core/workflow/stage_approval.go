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

type stageApprovalArtifactApprover interface {
	ApproveCurrentArtifactsByStageAndKinds(ctx context.Context, projectID string, stageName string, artifactKinds []string, reviewerID string) ([]string, error)
}

// StageApprovalService maps domain-level workflow stages to their underlying
// CONTROL nodes, so clients do not need to know node IDs.
type StageApprovalService struct {
	runs         stageApprovalRunStore
	nodes        stageApprovalNodeStore
	stateMachine stageApprovalStateMachine
	artifacts    stageApprovalArtifactApprover
}

func NewStageApprovalService(runs stageApprovalRunStore, nodes stageApprovalNodeStore, stateMachine stageApprovalStateMachine) *StageApprovalService {
	return &StageApprovalService{runs: runs, nodes: nodes, stateMachine: stateMachine}
}

func (s *StageApprovalService) WithArtifactApprover(approver stageApprovalArtifactApprover) *StageApprovalService {
	if s != nil {
		s.artifacts = approver
	}
	return s
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
		if err := s.approveStageArtifacts(ctx, projectID, node, output); err != nil {
			return nil, err
		}
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
	if err := s.approveStageArtifacts(ctx, projectID, node, output); err != nil {
		return nil, err
	}
	return node, nil
}

func (s *StageApprovalService) approveStageArtifacts(ctx context.Context, projectID string, node *model.Node, output map[string]interface{}) error {
	if s == nil || s.artifacts == nil || node == nil {
		return nil
	}
	stageName := stageNameForApprovalArtifact(node)
	kinds := artifactKindsForApprovalNode(node)
	if stageName == "" || len(kinds) == 0 {
		return nil
	}
	reviewerID := "stage-approval"
	if output != nil {
		if value := strings.TrimSpace(fmt.Sprint(output["reviewerId"])); value != "" && value != "<nil>" {
			reviewerID = value
		}
	}
	approvedIDs, err := s.artifacts.ApproveCurrentArtifactsByStageAndKinds(ctx, projectID, stageName, kinds, reviewerID)
	if err != nil {
		return fmt.Errorf("approve stage artifacts: %w", err)
	}
	if len(approvedIDs) == 0 {
		return fmt.Errorf("approve stage artifacts: no current valid artifacts for stage %s kinds %s", stageName, strings.Join(kinds, ","))
	}
	return nil
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
	requestedStage := normalizeApprovalStageName(stageName)
	if requestedStage == "" {
		return nil, fmt.Errorf("%w: %s", ErrStageNotFound, stageName)
	}

	for _, node := range nodes {
		if node == nil || node.ID != stageName {
			continue
		}
		if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
			return nil, fmt.Errorf("%w: %s", ErrStageNotControl, stageName)
		}
		return node, nil
	}

	for _, node := range nodes {
		if node == nil || !approvalNodeMatchesStage(node, requestedStage) {
			continue
		}
		if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
			continue
		}
		return node, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrStageNotFound, stageName)
}

func approvalNodeMatchesStage(node *model.Node, requestedStage string) bool {
	if node == nil {
		return false
	}
	nodeStage := ""
	if node.Input != nil {
		if stage, ok := node.Input["stage"].(string); ok {
			nodeStage = normalizeApprovalStageName(stage)
		}
		if nodeStage == "" {
			if stage, ok := node.Input["reviewStage"].(string); ok {
				nodeStage = normalizeApprovalStageName(stage)
			}
		}
	}
	if nodeStage == requestedStage {
		return true
	}

	nodeID := normalizeApprovalStageName(node.ID)
	if nodeID == requestedStage ||
		nodeID == requestedStage+"_review" ||
		nodeID == requestedStage+"_review_gate" ||
		nodeID == "review_"+requestedStage {
		return true
	}
	return strings.Contains(nodeID, requestedStage+"_review") ||
		strings.Contains(nodeID, requestedStage+"-review")
}

func stageNameForApprovalArtifact(node *model.Node) string {
	if node == nil || node.Input == nil {
		return ""
	}
	if stage, ok := node.Input["stage"].(string); ok && strings.TrimSpace(stage) != "" {
		return normalizeApprovalStageName(stage)
	}
	if stage, ok := node.Input["reviewStage"].(string); ok && strings.TrimSpace(stage) != "" {
		return normalizeApprovalStageName(stage)
	}
	return ""
}

func artifactKindsForApprovalNode(node *model.Node) []string {
	if node == nil || node.Input == nil {
		return nil
	}
	for _, key := range []string{"requiredOutputs", "reviewArtifactKinds", "artifactKinds"} {
		if kinds := approvalStringSlice(node.Input[key]); len(kinds) > 0 {
			return kinds
		}
	}
	switch stageNameForApprovalArtifact(node) {
	case "composition":
		return []string{"VIDEO_COMPOSITION_SPEC"}
	default:
		return nil
	}
}

func approvalStringSlice(value interface{}) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	case []interface{}:
		for _, item := range typed {
			if str, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
	case string:
		for _, item := range strings.Split(typed, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func normalizeApprovalStageName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "a04", "04", "video_structure", "video_struct", "structure", "timeline", "time_line", "composition":
		return "composition"
	default:
		return normalized
	}
}
