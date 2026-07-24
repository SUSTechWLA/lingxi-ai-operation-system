package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	videoModel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	coreModel "github.com/tangying-ai/aios-core/internal/core/model"
)

func (s *CreatorViewService) PreviewStepRegeneration(
	ctx context.Context,
	userID, projectID string,
	stepID videoModel.CreatorStepID,
) (videoModel.StepImpact, error) {
	if !knownCreatorStep(stepID) {
		return videoModel.StepImpact{}, ErrCreatorStepInvalid
	}
	project, err := s.projects.GetProject(ctx, userID, projectID)
	if err != nil || project == nil || project.ID != projectID || project.UserID != userID {
		return videoModel.StepImpact{}, ErrCreatorArtifactNotFound
	}
	if _, _, err := s.resolveCreatorStepReview(ctx, project, stepID, "", ""); err != nil {
		return videoModel.StepImpact{}, err
	}
	current, err := s.optionalCurrentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return videoModel.StepImpact{}, err
	}
	return s.stepImpact(ctx, userID, projectID, stepID, current)
}

func (s *CreatorViewService) RegenerateStep(
	ctx context.Context,
	userID, projectID string,
	stepID videoModel.CreatorStepID,
	req videoModel.StepRegenerationRequest,
	idempotencyKey string,
) (*videoModel.StepRegenerationResult, error) {
	if s == nil || s.reviews == nil || s.auditRun == nil || s.auditNodes == nil || s.projectLifecycle == nil {
		return nil, ErrCreatorMutationUnavailable
	}
	if !knownCreatorStep(stepID) {
		return nil, ErrCreatorStepInvalid
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, ErrCreatorInvalidRequest
	}
	project, err := s.projects.GetProject(ctx, userID, projectID)
	if err != nil || project == nil || project.ID != projectID || project.UserID != userID {
		return nil, ErrCreatorArtifactNotFound
	}
	current, err := s.optionalCurrentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	if err := validateRegenerationBase(current, req); err != nil {
		return nil, err
	}
	impact, err := s.stepImpact(ctx, userID, projectID, stepID, current)
	if err != nil {
		return nil, err
	}
	if !sameExactCreatorStepIDs(req.ConfirmedAffectedStepIDs, impact.AffectedStepIDs) {
		return nil, ErrCreatorImpactMismatch
	}
	viewBefore, err := s.GetCreationView(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	attempt := 1
	for _, step := range viewBefore.Steps {
		if step.ID == stepID {
			attempt = maxCreatorAttempt(step.AttemptCount + 1)
			break
		}
	}
	runID, reviewID, err := s.resolveCreatorStepReview(ctx, project, stepID, req.RunID, req.ReviewID)
	if err != nil {
		return nil, err
	}
	hint := strings.TrimSpace(req.Instruction)
	if hint == "" {
		hint = "从已完成步骤重新生成，并保留历史版本"
	}
	if _, err := s.reviews.RegenerateIdempotent(ctx, runID, reviewID, userID, hint, strings.TrimSpace(idempotencyKey)); err != nil {
		return nil, err
	}
	if err := s.projectLifecycle.MarkAgentRunStarted(ctx, userID, projectID, runID); err != nil {
		return nil, err
	}
	view, err := s.GetCreationView(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return &videoModel.StepRegenerationResult{
		RunID: runID, ReviewID: reviewID, Attempt: attempt, Impact: impact, View: view,
	}, nil
}

func validateRegenerationBase(current *artifact.Artifact, req videoModel.StepRegenerationRequest) error {
	baseID := strings.TrimSpace(req.BaseArtifactID)
	if current == nil {
		if baseID != "" || req.BaseVersion != 0 {
			return ErrCreatorVersionConflict
		}
		return nil
	}
	if baseID == "" && req.BaseVersion == 0 {
		return nil
	}
	if baseID != current.ID || req.BaseVersion != current.Version {
		return ErrCreatorVersionConflict
	}
	return nil
}

func (s *CreatorViewService) optionalCurrentArtifactForStep(
	ctx context.Context,
	projectID string,
	stepID videoModel.CreatorStepID,
) (*artifact.Artifact, error) {
	current, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if errors.Is(err, ErrCreatorArtifactNotFound) {
		return nil, nil
	}
	return current, err
}

func (s *CreatorViewService) resolveCreatorStepReview(
	ctx context.Context,
	project *videoModel.VideoProject,
	stepID videoModel.CreatorStepID,
	assertedRunID, assertedReviewID string,
) (string, string, error) {
	if s.auditRun == nil || s.auditNodes == nil || project == nil || strings.TrimSpace(project.CurrentRunID) == "" {
		return "", "", ErrCreatorMutationUnavailable
	}
	run, err := s.auditRun(ctx, project.CurrentRunID)
	if err != nil {
		return "", "", err
	}
	if run == nil || run.ID != project.CurrentRunID || run.UserID != project.UserID || run.TaskID == "" {
		return "", "", ErrCreatorArtifactNotFound
	}
	if assertedRunID != "" && assertedRunID != run.ID {
		return "", "", ErrCreatorVersionConflict
	}
	nodes, err := s.auditNodes.FindByTaskID(ctx, run.TaskID)
	if err != nil {
		return "", "", err
	}
	byID := make(map[string]*coreModel.Node, len(nodes))
	for _, node := range nodes {
		if node != nil {
			byID[node.ID] = node
		}
	}
	candidates := make([]*coreModel.Node, 0)
	for _, node := range nodes {
		if node == nil || (node.Type != coreModel.NodeTypeControl && node.Type != coreModel.NodeTypeReviewGate) || node.Status == coreModel.NodeSkipped {
			continue
		}
		mapped, ok := creatorStepForNode(node)
		if !ok {
			if sourceID, _ := node.Input["sourceNode"].(string); sourceID != "" {
				mapped, ok = creatorStepForNode(byID[sourceID])
			}
		}
		if !ok || mapped != stepID || !creatorReviewCanRegenerate(node) {
			continue
		}
		if assertedReviewID != "" && node.ID != assertedReviewID {
			continue
		}
		candidates = append(candidates, node)
	}
	if len(candidates) == 0 {
		return "", "", ErrCreatorArtifactNotFound
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank, rightRank := creatorReviewRank(candidates[i]), creatorReviewRank(candidates[j])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		leftTime, rightTime := creatorNodeTime(candidates[i]), creatorNodeTime(candidates[j])
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return candidates[i].ID > candidates[j].ID
	})
	return run.ID, candidates[0].ID, nil
}

func creatorReviewCanRegenerate(node *coreModel.Node) bool {
	if node == nil || node.Input == nil {
		return false
	}
	if source, _ := node.Input["sourceNode"].(string); strings.TrimSpace(source) != "" {
		return true
	}
	stepID, _ := node.Input["stepId"].(string)
	return strings.TrimSpace(stepID) != ""
}

func creatorReviewRank(node *coreModel.Node) int {
	if node == nil || node.Input == nil {
		return 0
	}
	phase, _ := node.Input["reviewPhase"].(string)
	switch normalizeCreatorStage(phase) {
	case "quality_gate":
		return 3
	case "after_artifact":
		return 2
	case "before_downstream":
		return 1
	default:
		return 0
	}
}

func sameExactCreatorStepIDs(actual, expected []videoModel.CreatorStepID) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := make(map[videoModel.CreatorStepID]bool, len(actual))
	for _, id := range actual {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range expected {
		if !seen[id] {
			return false
		}
	}
	return true
}
