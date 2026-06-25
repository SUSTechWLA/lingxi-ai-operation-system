package agentruntime

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

type ReviewNodeStore interface {
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
	UpdateStatus(ctx context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error
}

type ReviewStateMachine interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
	OnFailure(ctx context.Context, nodeID string, errorMessage string) error
}

// Decision type constants for audit-trail entries.
const (
	DecisionStageApproval     = "stage_approval"
	DecisionStageRejection    = "stage_rejection"
	DecisionStageEdit         = "stage_edited"
	DecisionStageRegeneration = "stage_regenerated"
)

// DecisionLogWriter abstracts the storage needed to write audit-trail entries.
type DecisionLogWriter interface {
	Save(ctx context.Context, record *DecisionLogRecord) error
}

// DecisionLogRecord is a simplified decision-log entry used by the agent runtime handler.
type DecisionLogRecord struct {
	WorkflowRunID  string `json:"workflowRunId"`
	TaskID         string `json:"taskId"`
	StageName      string `json:"stageName"`
	DecisionType   string `json:"decisionType"`
	Selected       string `json:"selected"`
	ApprovedByUser bool   `json:"approvedByUser"`
	ReviewerID     string `json:"reviewerId,omitempty"`
	Comment        string `json:"comment,omitempty"`
}

type Handler struct {
	runner         *Runner
	nodes          ReviewNodeStore
	stateMachine   ReviewStateMachine
	artifactReview ArtifactReviewStore
	decisionLog    DecisionLogWriter
}

func NewHandler(runner *Runner, nodes ReviewNodeStore, stateMachine ReviewStateMachine) *Handler {
	return &Handler{runner: runner, nodes: nodes, stateMachine: stateMachine}
}

// WithArtifactReviewStore sets the artifact review store for persisting
// PENDING/APPROVED/REJECTED artifact review records.
func (h *Handler) WithArtifactReviewStore(store ArtifactReviewStore) *Handler {
	h.artifactReview = store
	return h
}

// WithDecisionLogWriter sets the decision log writer for audit-trail persistence.
func (h *Handler) WithDecisionLogWriter(w DecisionLogWriter) *Handler {
	h.decisionLog = w
	return h
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/agent/runs")
	{
		api.POST("", h.StartRun)
		api.GET("/:runId", h.GetRun)
		api.GET("/:runId/trace", h.GetTrace)
		api.GET("/:runId/reviews", h.ListReviews)
		api.POST("/:runId/reviews/:reviewId/approve", h.ApproveReview)
		api.POST("/:runId/reviews/:reviewId/reject", h.RejectReview)
		api.POST("/:runId/reviews/:reviewId/submit-edited", h.SubmitEdited)
		api.POST("/:runId/reviews/:reviewId/regenerate", h.RegenerateStage)
	}
}

func (h *Handler) StartRun(c *gin.Context) {
	var req StartRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	run, err := h.runner.Start(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(c, gin.H{
		"runId":  run.ID,
		"taskId": run.TaskID,
		"status": run.Status,
		"plan":   run.Plan,
	})
}

func (h *Handler) GetRun(c *gin.Context) {
	run, task, err := h.runner.Get(c.Request.Context(), c.Param("runId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		httpx.Fail(c, http.StatusNotFound, "agent run not found")
		return
	}
	httpx.OK(c, gin.H{"run": run, "task": task})
}

func (h *Handler) GetTrace(c *gin.Context) {
	run, task, err := h.runner.Get(c.Request.Context(), c.Param("runId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		httpx.Fail(c, http.StatusNotFound, "agent run not found")
		return
	}
	httpx.OK(c, task)
}

func (h *Handler) ListReviews(c *gin.Context) {
	run, reviews, err := h.reviewsForRun(c.Request.Context(), c.Param("runId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		httpx.Fail(c, http.StatusNotFound, "agent run not found")
		return
	}

	// Sync artifact_review records for any CONTROL nodes that don't have one yet.
	if h.artifactReview != nil && run.TaskID != "" {
		for _, r := range reviews {
			h.ensureArtifactReview(c.Request.Context(), run.TaskID, r)
		}
	}

	httpx.OK(c, gin.H{"runId": run.ID, "reviews": reviews})
}

func (h *Handler) ApproveReview(c *gin.Context) {
	run, node, err := h.findReviewNode(c.Request.Context(), c.Param("runId"), c.Param("reviewId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil || node == nil {
		httpx.Fail(c, http.StatusNotFound, "review not found")
		return
	}
	if node.Status == model.NodeSuccess {
		httpx.OK(c, gin.H{"reviewId": node.ID, "status": "APPROVED"})
		return
	}
	if node.Status != model.NodeReady {
		httpx.Fail(c, http.StatusConflict, "review is not pending")
		return
	}

	var req struct {
		Comment    string `json:"comment,omitempty"`
		ReviewerID string `json:"reviewerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.stateMachine.OnSuccess(c.Request.Context(), node.ID, map[string]interface{}{
		"approved":      true,
		"humanApproved": true,
		"comment":       req.Comment,
		"stage":         nodeInputString(node, "stage"),
		"roleAgentId":   nodeInputString(node, "roleAgentId"),
	}); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync artifact_review status to APPROVED.
	h.updateArtifactReviewStatus(c.Request.Context(), node.ID, ArtifactReviewApproved, req.ReviewerID, req.Comment)

	// Write decision log entry for audit trail.
	h.writeDecisionLog(c.Request.Context(), run, node, DecisionStageApproval, req.ReviewerID, req.Comment)

	httpx.OK(c, gin.H{"reviewId": node.ID, "status": "APPROVED"})
}

func (h *Handler) RejectReview(c *gin.Context) {
	run, node, err := h.findReviewNode(c.Request.Context(), c.Param("runId"), c.Param("reviewId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		httpx.Fail(c, http.StatusNotFound, "review not found")
		return
	}
	if node.Status != model.NodeReady {
		httpx.Fail(c, http.StatusConflict, "review is not pending")
		return
	}

	var req struct {
		Comment    string `json:"comment,omitempty"`
		ReviewerID string `json:"reviewerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)
	reason := req.Comment
	if reason == "" {
		reason = "review rejected"
	}
	if err := h.nodes.UpdateStatus(c.Request.Context(), node.ID, model.NodeFailed, map[string]interface{}{
		"approved":              false,
		"comment":               reason,
		"stage":                 nodeInputString(node, "stage"),
		"roleAgentId":           nodeInputString(node, "roleAgentId"),
		"staleTrackingRequired": true,
		"staleTracker":          "stale_tracker",
		"staleArtifacts":        downstreamStaleArtifactsForReview(node),
	}, reason); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync artifact_review status to REJECTED.
	h.updateArtifactReviewStatus(c.Request.Context(), node.ID, ArtifactReviewRejected, req.ReviewerID, reason)

	// Write decision log entry for audit trail.
	h.writeDecisionLog(c.Request.Context(), run, node, DecisionStageRejection, req.ReviewerID, reason)

	httpx.OK(c, gin.H{"reviewId": node.ID, "status": "REJECTED"})
}

func (h *Handler) writeDecisionLog(ctx context.Context, run *Run, node *model.Node, decisionType, reviewerID, comment string) {
	if h.decisionLog == nil {
		return
	}
	stageName := ""
	if node.Input != nil {
		if s, ok := node.Input["stage"].(string); ok {
			stageName = s
		} else if s, ok := node.Input["stepId"].(string); ok {
			stageName = s
		}
	}
	_ = h.decisionLog.Save(ctx, &DecisionLogRecord{
		TaskID:         run.TaskID,
		StageName:      stageName,
		DecisionType:   decisionType,
		Selected:       "approved",
		ApprovedByUser: decisionType == DecisionStageApproval,
		ReviewerID:     reviewerID,
		Comment:        comment,
	})
}

func (h *Handler) reviewsForRun(ctx context.Context, runID string) (*Run, []Review, error) {
	run, _, err := h.runner.Get(ctx, runID)
	if err != nil || run == nil || run.TaskID == "" {
		return run, nil, err
	}
	nodes, err := h.nodes.FindByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, nil, err
	}
	reviews := make([]Review, 0)
	for _, node := range nodes {
		if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
			continue
		}
		reviews = append(reviews, reviewFromNode(node))
	}
	return run, reviews, nil
}

func (h *Handler) findReviewNode(ctx context.Context, runID, reviewID string) (*Run, *model.Node, error) {
	run, _, err := h.runner.Get(ctx, runID)
	if err != nil || run == nil || run.TaskID == "" {
		return run, nil, err
	}
	nodes, err := h.nodes.FindByTaskID(ctx, run.TaskID)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range nodes {
		if node.ID == reviewID && (node.Type == model.NodeTypeControl || node.Type == model.NodeTypeReviewGate) {
			return run, node, nil
		}
	}
	return run, nil, nil
}

type Review struct {
	ID                  string                 `json:"id"`
	NodeID              string                 `json:"nodeId"`
	Status              string                 `json:"status"`
	StepID              string                 `json:"stepId,omitempty"`
	Tool                string                 `json:"tool,omitempty"`
	Stage               string                 `json:"stage,omitempty"`
	RoleAgentID         string                 `json:"roleAgentId,omitempty"`
	RoleAgent           map[string]interface{} `json:"roleAgent,omitempty"`
	HumanReview         map[string]interface{} `json:"humanReview,omitempty"`
	RequiredInputs      []string               `json:"requiredInputs,omitempty"`
	RequiredOutputs     []string               `json:"requiredOutputs,omitempty"`
	ReviewPhase         string                 `json:"reviewPhase,omitempty"`
	ReviewReason        string                 `json:"reviewReason,omitempty"`
	BlocksDownstream    bool                   `json:"blocksDownstream,omitempty"`
	ReviewArtifactKinds []string               `json:"reviewArtifactKinds,omitempty"`
}

func reviewFromNode(node *model.Node) Review {
	review := Review{
		ID:     node.ID,
		NodeID: node.ID,
		Status: reviewStatus(node.Status),
	}
	if node.Input != nil {
		review.StepID, _ = node.Input["stepId"].(string)
		review.Tool, _ = node.Input["tool"].(string)
		review.Stage, _ = node.Input["stage"].(string)
		review.RoleAgentID, _ = node.Input["roleAgentId"].(string)
		review.RoleAgent = stringInterfaceMap(node.Input["roleAgent"])
		review.HumanReview = stringInterfaceMap(node.Input["humanReview"])
		review.RequiredInputs = stringSlice(node.Input["requiredInputs"])
		review.RequiredOutputs = stringSlice(node.Input["requiredOutputs"])
		review.ReviewPhase, _ = node.Input["reviewPhase"].(string)
		review.ReviewReason, _ = node.Input["reviewReason"].(string)
		review.BlocksDownstream, _ = node.Input["blocksDownstream"].(bool)
		review.ReviewArtifactKinds = stringSlice(node.Input["reviewArtifactKinds"])
	}
	return review
}

func reviewStatus(status model.NodeStatus) string {
	switch status {
	case model.NodeReady:
		return "PENDING"
	case model.NodeSuccess:
		return "APPROVED"
	case model.NodeFailed:
		return "REJECTED"
	default:
		return string(status)
	}
}

func stringInterfaceMap(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	default:
		return nil
	}
}

func nodeInputString(node *model.Node, key string) string {
	if node == nil || node.Input == nil {
		return ""
	}
	value, _ := node.Input[key].(string)
	return value
}

func stringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// ensureArtifactReview creates a PENDING artifact_review record for a CONTROL node
// if one does not already exist. This bridges the DAG CONTROL node with the
// artifact_reviews persistence table.
func (h *Handler) ensureArtifactReview(ctx context.Context, taskID string, r Review) {
	if h.artifactReview == nil {
		return
	}

	// Check if a record already exists for this node.
	existing, err := h.artifactReview.FindByNodeID(ctx, r.NodeID)
	if err == nil && existing != nil {
		return
	}

	review := &ArtifactReview{
		ID:           r.NodeID,
		TaskID:       taskID,
		NodeID:       r.NodeID,
		Status:       ArtifactReviewPending,
		ReviewReason: r.ReviewReason,
	}
	_ = h.artifactReview.Save(ctx, review)
}

// updateArtifactReviewStatus syncs the artifact_review table when a review
// is approved or rejected.
func (h *Handler) updateArtifactReviewStatus(ctx context.Context, nodeID string, status ArtifactReviewStatus, reviewerID, comment string) {
	if h.artifactReview == nil {
		return
	}

	// Try to find by node ID first.
	review, err := h.artifactReview.FindByNodeID(ctx, nodeID)
	if err != nil || review == nil {
		return
	}

	_ = h.artifactReview.UpdateStatus(ctx, review.ID, status, reviewerID, comment)
}

// SubmitEdited allows the user to submit an edited version of the artifact
// that triggered the review. The review gate is approved with the edited content.
func (h *Handler) SubmitEdited(c *gin.Context) {
	run, node, err := h.findReviewNode(c.Request.Context(), c.Param("runId"), c.Param("reviewId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		httpx.Fail(c, http.StatusNotFound, "review not found")
		return
	}
	if node.Status != model.NodeReady {
		httpx.Fail(c, http.StatusConflict, "review is not pending")
		return
	}

	var req struct {
		Content    interface{} `json:"content"`
		Comment    string      `json:"comment,omitempty"`
		ReviewerID string      `json:"reviewerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)

	output := map[string]interface{}{
		"approved":              true,
		"humanApproved":         true,
		"edited":                true,
		"editContent":           req.Content,
		"comment":               req.Comment,
		"stage":                 nodeInputString(node, "stage"),
		"roleAgentId":           nodeInputString(node, "roleAgentId"),
		"staleTrackingRequired": true,
		"staleTracker":          "stale_tracker",
		"staleArtifacts":        downstreamStaleArtifactsForReview(node),
	}
	if err := h.stateMachine.OnSuccess(c.Request.Context(), node.ID, output); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.updateArtifactReviewStatus(c.Request.Context(), node.ID, ArtifactReviewApproved, req.ReviewerID, req.Comment)
	h.writeDecisionLog(c.Request.Context(), run, node, DecisionStageEdit, req.ReviewerID, req.Comment)
	httpx.OK(c, gin.H{"reviewId": node.ID, "status": "APPROVED_EDITED"})
}

// RegenerateStage resets the execution node and review gate for a stage,
// allowing the LLM to re-run the stage with an optional regeneration hint.
func (h *Handler) RegenerateStage(c *gin.Context) {
	run, node, err := h.findReviewNode(c.Request.Context(), c.Param("runId"), c.Param("reviewId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if node == nil {
		httpx.Fail(c, http.StatusNotFound, "review not found")
		return
	}
	// REVIEW_GATE nodes must be READY (pending review), FAILED, or SUCCESS.
	// SUCCESS means re-requested after approval.

	var req struct {
		Hint       string `json:"hint,omitempty"`
		ReviewerID string `json:"reviewerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)

	// Reset the exec node that feeds into this review gate.
	if err := h.regenerateSourceNode(c.Request.Context(), node); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "failed to regenerate source: "+err.Error())
		return
	}

	// Reset the review gate itself to CREATED so it re-enters the ready cycle.
	if err := h.nodes.UpdateStatus(c.Request.Context(), node.ID, model.NodeCreated, nil, ""); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "failed to reset review gate: "+err.Error())
		return
	}

	// Write audit trail.
	h.writeDecisionLog(c.Request.Context(), run, node, DecisionStageRegeneration, req.ReviewerID, req.Hint)
	httpx.OK(c, gin.H{
		"reviewId":              node.ID,
		"status":                "REGENERATING",
		"staleTrackingRequired": true,
		"staleTracker":          "stale_tracker",
		"staleArtifacts":        downstreamStaleArtifactsForReview(node),
	})
}

// regenerateSourceNode finds the upstream execution node that feeds into this
// review gate and resets it to CREATED so it gets re-dispatched.
func (h *Handler) regenerateSourceNode(ctx context.Context, gateNode *model.Node) error {
	// The source exec node is typically named <step>_exec and connected to
	// the review gate via an edge. We look for it in the gate node's input.
	sourceID := ""
	if gateNode.Input != nil {
		if id, ok := gateNode.Input["sourceNode"].(string); ok && id != "" {
			sourceID = id
		}
	}
	if sourceID == "" {
		// Fallback: try to derive from stepId.
		if stepID, ok := gateNode.Input["stepId"].(string); ok && stepID != "" {
			sourceID = stepID + "_exec"
		}
	}
	if sourceID == "" {
		return fmt.Errorf("source exec node not found for review gate %s", gateNode.ID)
	}
	return h.nodes.UpdateStatus(ctx, sourceID, model.NodeCreated, nil, "")
}

func downstreamStaleArtifactsForReview(node *model.Node) []string {
	if node == nil || node.Input == nil {
		return nil
	}
	outputs := stringSlice(node.Input["requiredOutputs"])
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, output := range outputs {
		for _, downstream := range artifact.DownstreamStaleArtifactKinds(output) {
			if !seen[downstream] {
				seen[downstream] = true
				result = append(result, downstream)
			}
		}
	}
	return result
}
