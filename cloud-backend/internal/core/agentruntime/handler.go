package agentruntime

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

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

type Handler struct {
	runner         *Runner
	nodes          ReviewNodeStore
	stateMachine   ReviewStateMachine
	artifactReview ArtifactReviewStore
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

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/agent/runs")
	{
		api.POST("", h.StartRun)
		api.GET("/:runId", h.GetRun)
		api.GET("/:runId/trace", h.GetTrace)
		api.GET("/:runId/reviews", h.ListReviews)
		api.POST("/:runId/reviews/:reviewId/approve", h.ApproveReview)
		api.POST("/:runId/reviews/:reviewId/reject", h.RejectReview)
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
		"approved": true,
		"comment":  req.Comment,
	}); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync artifact_review status to APPROVED.
	h.updateArtifactReviewStatus(c.Request.Context(), node.ID, ArtifactReviewApproved, req.ReviewerID, req.Comment)

	httpx.OK(c, gin.H{"reviewId": node.ID, "status": "APPROVED"})
}

func (h *Handler) RejectReview(c *gin.Context) {
	_, node, err := h.findReviewNode(c.Request.Context(), c.Param("runId"), c.Param("reviewId"))
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
		"approved": false,
		"comment":  reason,
	}, reason); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync artifact_review status to REJECTED.
	h.updateArtifactReviewStatus(c.Request.Context(), node.ID, ArtifactReviewRejected, req.ReviewerID, reason)

	httpx.OK(c, gin.H{"reviewId": node.ID, "status": "REJECTED"})
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
		if node.Type != model.NodeTypeControl {
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
		if node.ID == reviewID && node.Type == model.NodeTypeControl {
			return run, node, nil
		}
	}
	return run, nil, nil
}

type Review struct {
	ID                  string   `json:"id"`
	NodeID              string   `json:"nodeId"`
	Status              string   `json:"status"`
	StepID              string   `json:"stepId,omitempty"`
	Tool                string   `json:"tool,omitempty"`
	ReviewPhase         string   `json:"reviewPhase,omitempty"`
	ReviewReason        string   `json:"reviewReason,omitempty"`
	BlocksDownstream    bool     `json:"blocksDownstream,omitempty"`
	ReviewArtifactKinds []string `json:"reviewArtifactKinds,omitempty"`
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
