package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

var (
	ErrReviewNotFound          = errors.New("review not found")
	ErrReviewNotPending        = errors.New("review is not pending")
	ErrReviewCannotReopen      = errors.New("review cannot be reopened")
	ErrReviewReferenceMismatch = errors.New("review reference mismatch")
	ErrReviewGateAmbiguous     = errors.New("review gate is ambiguous")
)

// ReviewMutationService is the small creator-facing review state-machine API.
type ReviewMutationService interface {
	Confirm(ctx context.Context, runID, reviewID, reviewerID, comment string) error
	ReopenWithArtifact(ctx context.Context, runID, reviewID, artifactID, reviewerID, reason string) error
}

// ReviewGateResolver derives and verifies the gate identity from durable
// artifact lineage. Supplied IDs are treated only as assertions.
type ReviewGateResolver interface {
	ResolveReviewGate(ctx context.Context, current *artifact.Artifact, runID, reviewID string) (string, string, error)
}

type reviewMutationService struct {
	runner            *Runner
	nodes             ReviewNodeStore
	stateMachine      ReviewStateMachine
	artifactReview    ArtifactReviewStore
	decisionLog       DecisionLogWriter
	artifactService   ArtifactService
	projectIDResolver ProjectIDResolver
	regeneration      RegenerationDispatcher
}

func NewReviewMutationService(runner *Runner, nodes ReviewNodeStore, stateMachine ReviewStateMachine) *reviewMutationService {
	return &reviewMutationService{runner: runner, nodes: nodes, stateMachine: stateMachine}
}

func (s *reviewMutationService) WithArtifactReviewStore(store ArtifactReviewStore) *reviewMutationService {
	s.artifactReview = store
	return s
}

func (s *reviewMutationService) WithDecisionLogWriter(writer DecisionLogWriter) *reviewMutationService {
	s.decisionLog = writer
	return s
}

func (s *reviewMutationService) WithArtifactService(service ArtifactService) *reviewMutationService {
	s.artifactService = service
	return s
}

func (s *reviewMutationService) WithProjectIDResolver(resolver ProjectIDResolver) *reviewMutationService {
	s.projectIDResolver = resolver
	return s
}

func (s *reviewMutationService) WithRegenerationDispatcher(dispatcher RegenerationDispatcher) *reviewMutationService {
	s.regeneration = dispatcher
	return s
}

func (s *reviewMutationService) ResolveReviewGate(ctx context.Context, current *artifact.Artifact, assertedRunID, assertedReviewID string) (string, string, error) {
	if s == nil || s.runner == nil || current == nil || strings.TrimSpace(current.ID) == "" {
		return "", "", ErrReviewReferenceMismatch
	}
	artifactRunRef := strings.TrimSpace(current.WorkflowRunID)
	if artifactRunRef == "" && strings.TrimSpace(current.TaskID) == "" {
		return "", "", ErrReviewReferenceMismatch
	}
	run, _, err := s.runner.Get(ctx, artifactRunRef)
	if err != nil {
		return "", "", err
	}
	if run == nil {
		taskID := strings.TrimSpace(current.TaskID)
		if taskID == "" {
			taskID = artifactRunRef
		}
		run, err = s.runner.findRunByTaskID(ctx, taskID)
		if err != nil {
			return "", "", err
		}
	}
	if run == nil || run.ID == "" {
		return "", "", ErrReviewNotFound
	}
	runID := run.ID
	if assertedRunID != "" && assertedRunID != runID {
		return "", "", ErrReviewReferenceMismatch
	}
	run, nodes, err := s.nodesForRun(ctx, runID)
	if err != nil {
		return "", "", err
	}
	if current.TaskID != "" && current.TaskID != run.TaskID {
		return "", "", ErrReviewReferenceMismatch
	}

	bestScore := 0
	best := make([]*model.Node, 0, 1)
	sourceAliases := reviewSourceAliases(nodes, current.ProducedByNode)
	for _, node := range nodes {
		if !isReviewGate(node) {
			continue
		}
		score := reviewArtifactMatchScore(node, current, sourceAliases)
		if score == 0 || score < bestScore {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = best[:0]
		}
		best = append(best, node)
	}
	if len(best) == 0 {
		return "", "", ErrReviewNotFound
	}
	if len(best) != 1 {
		return "", "", ErrReviewGateAmbiguous
	}
	if assertedReviewID != "" && assertedReviewID != best[0].ID {
		return "", "", ErrReviewReferenceMismatch
	}
	return runID, best[0].ID, nil
}

func reviewArtifactMatchScore(node *model.Node, current *artifact.Artifact, sourceAliases map[string]bool) int {
	if node == nil || node.Input == nil || current == nil {
		return 0
	}
	if nodeInputString(node, "artifactId") == current.ID {
		return 3
	}
	if producedBy := strings.TrimSpace(current.ProducedByNode); producedBy != "" {
		if sourceAliases[nodeInputString(node, "sourceNode")] || sourceAliases[nodeInputString(node, "productionSourceNode")] {
			return 2
		}
		return 0
	}
	if stage := strings.TrimSpace(nodeInputString(node, "stage")); stage != "" && strings.EqualFold(stage, strings.TrimSpace(current.StageName)) {
		return 1
	}
	return 0
}

func reviewSourceAliases(nodes []*model.Node, producedByNode string) map[string]bool {
	producedByNode = strings.TrimSpace(producedByNode)
	aliases := map[string]bool{}
	if producedByNode == "" {
		return aliases
	}
	aliases[producedByNode] = true
	for _, node := range nodes {
		if node == nil {
			continue
		}
		originalID := nodeInputString(node, "agentOriginalNodeId")
		if node.ID == producedByNode && originalID != "" {
			aliases[originalID] = true
		}
		if originalID == producedByNode {
			aliases[node.ID] = true
		}
	}
	return aliases
}

func (s *reviewMutationService) Confirm(ctx context.Context, runID, reviewID, reviewerID, comment string) error {
	run, node, err := s.findReviewNode(ctx, runID, reviewID)
	if err != nil {
		return err
	}
	if node.Status == model.NodeSuccess {
		return nil
	}
	if node.Status != model.NodeReady {
		return ErrReviewNotPending
	}
	if s.stateMachine == nil {
		return fmt.Errorf("review state machine is unavailable")
	}
	if err := s.stateMachine.OnSuccess(ctx, node.ID, map[string]interface{}{
		"approved": true, "humanApproved": true, "comment": comment,
		"artifactId": nodeInputString(node, "artifactId"), "stage": nodeInputString(node, "stage"),
		"roleAgentId": nodeInputString(node, "roleAgentId"),
	}); err != nil {
		return err
	}
	s.updateArtifactReviewStatus(ctx, node.ID, ArtifactReviewApproved, reviewerID, comment)
	s.writeDecisionLog(ctx, run, node, DecisionStageApproval, "approved", reviewerID, comment, true)
	s.approveReviewedArtifact(ctx, run, node, reviewerID)
	return nil
}

func (s *reviewMutationService) ReopenWithArtifact(ctx context.Context, runID, reviewID, artifactID, reviewerID, reason string) error {
	run, node, err := s.findReviewNode(ctx, runID, reviewID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(artifactID) == "" {
		return ErrReviewReferenceMismatch
	}
	switch node.Status {
	case model.NodeReady, model.NodeSuccess, model.NodeFailed:
	default:
		return ErrReviewCannotReopen
	}
	updater, ok := s.nodes.(ReviewNodeInputUpdater)
	if !ok {
		return fmt.Errorf("review input updater is unavailable")
	}
	if err := updater.UpdateInputFields(ctx, node.ID, map[string]interface{}{"artifactId": artifactID}); err != nil {
		return err
	}
	if node.Input == nil {
		node.Input = map[string]interface{}{}
	}
	node.Input["artifactId"] = artifactID
	if err := s.nodes.UpdateStatus(ctx, node.ID, model.NodeReady, map[string]interface{}{
		"approved": false, "humanApproved": false, "artifactId": artifactID, "comment": reason,
	}, ""); err != nil {
		return err
	}
	s.updateArtifactReviewStatus(ctx, node.ID, ArtifactReviewPending, reviewerID, reason)
	s.writeDecisionLog(ctx, run, node, DecisionStageEdit, artifactID, reviewerID, reason, false)
	return nil
}

func (s *reviewMutationService) SubmitEdited(ctx context.Context, runID, reviewID, reviewerID, comment string, content interface{}) error {
	run, node, err := s.findReviewNode(ctx, runID, reviewID)
	if err != nil {
		return err
	}
	if node.Status != model.NodeReady {
		return ErrReviewNotPending
	}
	if s.stateMachine == nil {
		return fmt.Errorf("review state machine is unavailable")
	}
	if err := s.stateMachine.OnSuccess(ctx, node.ID, map[string]interface{}{
		"approved": true, "humanApproved": true, "edited": true, "editContent": content,
		"comment": comment, "stage": nodeInputString(node, "stage"), "roleAgentId": nodeInputString(node, "roleAgentId"),
		"staleTrackingRequired": true, "staleTracker": "stale_tracker",
		"staleArtifacts": downstreamStaleArtifactsForReview(node),
	}); err != nil {
		return err
	}
	s.updateArtifactReviewStatus(ctx, node.ID, ArtifactReviewApproved, reviewerID, comment)
	s.writeDecisionLog(ctx, run, node, DecisionStageEdit, "approved", reviewerID, comment, false)
	s.approveReviewedArtifact(ctx, run, node, reviewerID)
	s.triggerDownstreamStale(ctx, run, node, "用户修改上游产物")
	return nil
}

func (s *reviewMutationService) Regenerate(ctx context.Context, runID, reviewID, reviewerID, hint string) ([]string, error) {
	run, node, err := s.findReviewNode(ctx, runID, reviewID)
	if err != nil {
		return nil, err
	}
	sourceID := nodeInputString(node, "sourceNode")
	if sourceID == "" {
		if stepID := nodeInputString(node, "stepId"); stepID != "" {
			sourceID = stepID + "_exec"
		}
	}
	if sourceID == "" {
		return nil, fmt.Errorf("source exec node not found for review gate %s", node.ID)
	}
	resolvedSourceID, err := s.resolveSourceNodeID(ctx, node, sourceID)
	if err != nil {
		return nil, err
	}
	if err := s.nodes.UpdateStatus(ctx, resolvedSourceID, model.NodeCreated, nil, ""); err != nil {
		return nil, err
	}
	if err := s.nodes.UpdateStatus(ctx, node.ID, model.NodeCreated, nil, ""); err != nil {
		return nil, err
	}
	if s.regeneration != nil {
		if err := s.regeneration.ResumeTask(ctx, node.TaskID); err != nil {
			return nil, err
		}
		if err := s.regeneration.RetryNode(ctx, resolvedSourceID); err != nil {
			return nil, err
		}
	}
	s.writeDecisionLog(ctx, run, node, DecisionStageRegeneration, "approved", reviewerID, hint, false)
	s.triggerDownstreamStale(ctx, run, node, "用户重新生成阶段")
	return downstreamStaleArtifactsForReview(node), nil
}

func (s *reviewMutationService) resolveSourceNodeID(ctx context.Context, gate *model.Node, sourceID string) (string, error) {
	if gate == nil || gate.TaskID == "" {
		return "", ErrReviewNotFound
	}
	nodes, err := s.nodes.FindByTaskID(ctx, gate.TaskID)
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		if node.ID == sourceID {
			return node.ID, nil
		}
	}
	for _, node := range nodes {
		if nodeInputString(node, "agentOriginalNodeId") == sourceID {
			return node.ID, nil
		}
	}
	return "", fmt.Errorf("source exec node %q not found for review gate %s", sourceID, gate.ID)
}

func (s *reviewMutationService) triggerDownstreamStale(ctx context.Context, run *Run, node *model.Node, reason string) {
	if s.artifactService == nil || s.projectIDResolver == nil || run == nil || node == nil {
		return
	}
	projectID, err := s.projectIDResolver.ResolveProjectID(ctx, run.TaskID)
	if err != nil || projectID == "" {
		return
	}
	if artifactID := nodeInputString(node, "artifactId"); artifactID != "" {
		_, _ = s.artifactService.MarkDownstreamStale(ctx, projectID, artifactID, reason)
		return
	}
	if stage := nodeInputString(node, "stage"); stage != "" {
		_, _ = s.artifactService.MarkDownstreamStaleByStageName(ctx, projectID, stage, reason)
	}
}

func (s *reviewMutationService) findReviewNode(ctx context.Context, runID, reviewID string) (*Run, *model.Node, error) {
	run, nodes, err := s.nodesForRun(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range nodes {
		if node.ID == reviewID && isReviewGate(node) {
			return run, node, nil
		}
	}
	return run, nil, ErrReviewNotFound
}

func (s *reviewMutationService) nodesForRun(ctx context.Context, runID string) (*Run, []*model.Node, error) {
	if s == nil || s.runner == nil || s.nodes == nil || strings.TrimSpace(runID) == "" {
		return nil, nil, ErrReviewNotFound
	}
	run, _, err := s.runner.Get(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	if run == nil || run.TaskID == "" {
		return nil, nil, ErrReviewNotFound
	}
	nodes, err := s.nodes.FindByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, nil, err
	}
	return run, nodes, nil
}

func isReviewGate(node *model.Node) bool {
	return node != nil && (node.Type == model.NodeTypeControl || node.Type == model.NodeTypeReviewGate)
}

func (s *reviewMutationService) updateArtifactReviewStatus(ctx context.Context, nodeID string, status ArtifactReviewStatus, reviewerID, comment string) {
	if s.artifactReview == nil {
		return
	}
	review, err := s.artifactReview.FindByNodeID(ctx, nodeID)
	if err == nil && review != nil {
		_ = s.artifactReview.UpdateStatus(ctx, review.ID, status, reviewerID, comment)
	}
}

func (s *reviewMutationService) writeDecisionLog(ctx context.Context, run *Run, node *model.Node, decisionType, selected, reviewerID, comment string, approved bool) {
	if s.decisionLog == nil || run == nil || node == nil {
		return
	}
	_ = s.decisionLog.Save(ctx, &DecisionLogRecord{
		WorkflowRunID: run.ID, TaskID: run.TaskID, StageName: nodeInputString(node, "stage"),
		DecisionType: decisionType, Selected: selected, ApprovedByUser: approved,
		ReviewerID: reviewerID, Comment: comment,
	})
}

func (s *reviewMutationService) approveReviewedArtifact(ctx context.Context, run *Run, node *model.Node, reviewerID string) {
	if s.artifactService == nil || node == nil {
		return
	}
	artifactID := nodeInputString(node, "artifactId")
	if artifactID == "" && s.artifactReview != nil {
		if review, err := s.artifactReview.FindByNodeID(ctx, node.ID); err == nil && review != nil {
			artifactID = review.ArtifactID
		}
	}
	if artifactID != "" {
		if err := s.artifactService.ApproveArtifact(ctx, artifactID, reviewerID); err != nil {
			zap.L().Warn("approve artifact by id failed", zap.String("artifactId", artifactID), zap.Error(err))
		}
		return
	}
	if s.projectIDResolver == nil || run == nil {
		return
	}
	projectID, err := s.projectIDResolver.ResolveProjectID(ctx, run.TaskID)
	if err != nil || projectID == "" {
		return
	}
	stage, kinds := nodeInputString(node, "stage"), reviewArtifactKindsFromNode(node)
	if stage != "" && len(kinds) > 0 {
		_, _ = s.artifactService.ApproveCurrentArtifactsByStageAndKinds(ctx, projectID, stage, kinds, reviewerID)
	}
}

var _ ReviewMutationService = (*reviewMutationService)(nil)
var _ ReviewGateResolver = (*reviewMutationService)(nil)
