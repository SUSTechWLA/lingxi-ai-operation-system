package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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

// ArtifactService is the subset of artifact.Service needed by review handlers.
type ArtifactService interface {
	MarkDownstreamStale(ctx context.Context, projectID string, changedArtifactID string, reason string) ([]string, error)
	MarkDownstreamStaleByStageName(ctx context.Context, projectID string, stageName string, reason string) ([]string, error)
	ApproveArtifact(ctx context.Context, artifactID string, reviewerID string) error
	ApproveCurrentArtifactsByStageAndKinds(ctx context.Context, projectID string, stageName string, artifactKinds []string, reviewerID string) ([]string, error)
	FindCurrentByKind(ctx context.Context, projectID, stageName string) (*artifact.Artifact, error)
	FindCurrentByStageAndKind(ctx context.Context, projectID, stageName, artifactKind string) (*artifact.Artifact, error)
}

// ProjectIDResolver resolves a project ID from a task ID.
type ProjectIDResolver interface {
	ResolveProjectID(ctx context.Context, taskID string) (string, error)
}

// ProjectLifecycleUpdater updates the project shell when an agent run starts.
type ProjectLifecycleUpdater interface {
	MarkAgentRunStarted(ctx context.Context, userID, projectID, runID string) error
	MarkAgentRunStopped(ctx context.Context, userID, projectID, runID string) error
}

type ReviewNodeInputUpdater interface {
	UpdateInputFields(ctx context.Context, id string, fields map[string]interface{}) error
}

type TaskPauser interface {
	PauseTask(ctx context.Context, taskID, reason string) error
}

type Handler struct {
	runner            *Runner
	nodes             ReviewNodeStore
	stateMachine      ReviewStateMachine
	artifactReview    ArtifactReviewStore
	decisionLog       DecisionLogWriter
	artifactService   ArtifactService
	projectIDResolver ProjectIDResolver
	projectLifecycle  ProjectLifecycleUpdater
	taskPauser        TaskPauser
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

// WithArtifactService sets the artifact service for stale tracking on review actions.
func (h *Handler) WithArtifactService(s ArtifactService) *Handler {
	h.artifactService = s
	return h
}

// WithProjectIDResolver sets the resolver to look up project ID from task ID.
func (h *Handler) WithProjectIDResolver(r ProjectIDResolver) *Handler {
	h.projectIDResolver = r
	return h
}

// WithProjectLifecycleUpdater sets the updater used to reflect agent run
// lifecycle changes on the parent video project.
func (h *Handler) WithProjectLifecycleUpdater(updater ProjectLifecycleUpdater) *Handler {
	h.projectLifecycle = updater
	return h
}

func (h *Handler) WithTaskPauser(pauser TaskPauser) *Handler {
	h.taskPauser = pauser
	return h
}

func (h *Handler) RegisterRoutes(r *gin.Engine, middleware ...gin.HandlerFunc) {
	api := r.Group("/api/agent/runs", middleware...)
	{
		api.POST("", h.StartRun)
		api.GET("/:runId", h.GetRun)
		api.POST("/:runId/cancel", h.CancelRun)
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
	if req.UserID == "" {
		req.UserID = ginUserID(c)
	}
	run, err := h.runner.StartAsync(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	h.markProjectRunStarted(c.Request.Context(), req, run)
	httpx.OK(c, gin.H{
		"runId":  run.ID,
		"taskId": run.TaskID,
		"status": run.Status,
		"plan":   run.Plan,
	})
}

func (h *Handler) CancelRun(c *gin.Context) {
	var req struct {
		ProjectID string `json:"projectId"`
		Reason    string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "user requested"
	}
	run, err := h.runner.Cancel(c.Request.Context(), c.Param("runId"))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		httpx.Fail(c, http.StatusNotFound, "agent run not found")
		return
	}
	if h.taskPauser != nil && run.TaskID != "" {
		if err := h.taskPauser.PauseTask(c.Request.Context(), run.TaskID, req.Reason); err != nil {
			zap.L().Warn("agent run cancelled but task pause failed",
				zap.String("runId", run.ID),
				zap.String("taskId", run.TaskID),
				zap.Error(err),
			)
		}
	}
	if h.projectLifecycle != nil && req.ProjectID != "" && run.UserID != "" {
		if err := h.projectLifecycle.MarkAgentRunStopped(c.Request.Context(), run.UserID, req.ProjectID, run.ID); err != nil {
			zap.L().Warn("agent run cancelled but project status update failed",
				zap.String("projectId", req.ProjectID),
				zap.String("runId", run.ID),
				zap.Error(err),
			)
		}
	}
	httpx.OK(c, gin.H{
		"runId":  run.ID,
		"taskId": run.TaskID,
		"status": run.Status,
	})
}

func ginUserID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get("userID"); ok {
		if userID, ok := value.(string); ok {
			return userID
		}
	}
	return ""
}

func (h *Handler) markProjectRunStarted(ctx context.Context, req StartRunRequest, run *Run) {
	if h == nil || h.projectLifecycle == nil || run == nil {
		return
	}
	projectID, _ := req.Context["projectId"].(string)
	if projectID == "" || req.UserID == "" {
		return
	}
	if err := h.projectLifecycle.MarkAgentRunStarted(ctx, req.UserID, projectID, run.ID); err != nil {
		zap.L().Warn("agent run started but project status update failed",
			zap.String("projectId", projectID),
			zap.String("runId", run.ID),
			zap.Error(err),
		)
	}
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
	nodes := []*model.Node{}
	if h.nodes != nil && run.TaskID != "" {
		nodes, err = h.nodes.FindByTaskID(c.Request.Context(), run.TaskID)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	httpx.OK(c, gin.H{"task": task, "nodes": nodes})
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
	h.ensureReviewNodeArtifactID(c.Request.Context(), run, node)

	var req struct {
		Comment    string `json:"comment,omitempty"`
		ReviewerID string `json:"reviewerId,omitempty"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.stateMachine.OnSuccess(c.Request.Context(), node.ID, map[string]interface{}{
		"approved":      true,
		"humanApproved": true,
		"comment":       req.Comment,
		"artifactId":    nodeInputString(node, "artifactId"),
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

	// Mark the reviewed artifact as approved.
	h.approveReviewedArtifact(c.Request.Context(), run, node, req.ReviewerID)

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
	h.ensureReviewNodeArtifactID(c.Request.Context(), run, node)

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

	// Force downstream stale: rejecting an artifact invalidates everything downstream.
	h.triggerDownstreamStale(c.Request.Context(), run, node, "用户驳回审核")

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
	nodesByID := make(map[string]*model.Node, len(nodes))
	nodesByOriginalID := make(map[string]*model.Node, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
		if originalID := nodeInputString(node, "agentOriginalNodeId"); originalID != "" {
			nodesByOriginalID[originalID] = node
		}
	}
	reviews := make([]Review, 0)
	for _, node := range nodes {
		if node.Type != model.NodeTypeControl && node.Type != model.NodeTypeReviewGate {
			continue
		}
		if !visibleReviewNodeStatus(node.Status) {
			continue
		}
		if !reviewNodeSourceComplete(node, nodesByID, nodesByOriginalID) {
			continue
		}
		h.ensureReviewNodeArtifactID(ctx, run, node)
		review := reviewFromNode(node)
		enrichReviewFromSourceNode(&review, node, nodesByID, nodesByOriginalID)
		if !reviewReadyForDecision(review, node) {
			continue
		}
		reviews = append(reviews, review)
	}
	return run, reviews, nil
}

func visibleReviewNodeStatus(status model.NodeStatus) bool {
	return status == model.NodeReady || status == model.NodeSuccess || status == model.NodeFailed
}

func reviewNodeSourceComplete(reviewNode *model.Node, nodesByID, nodesByOriginalID map[string]*model.Node) bool {
	if reviewNode == nil || reviewNode.Status != model.NodeReady || reviewNode.Input == nil {
		return true
	}
	phase, _ := reviewNode.Input["reviewPhase"].(string)
	if phase != "after_artifact" && phase != "quality_gate" && phase != "before_downstream" {
		return true
	}
	sourceNodeID, _ := reviewNode.Input["sourceNode"].(string)
	if sourceNodeID == "" {
		sourceNodeID, _ = reviewNode.Input["productionSourceNode"].(string)
	}
	if sourceNodeID == "" {
		return false
	}
	source := nodesByID[sourceNodeID]
	if source == nil {
		source = nodesByOriginalID[sourceNodeID]
	}
	return source != nil && source.Status == model.NodeSuccess
}

func reviewReadyForDecision(review Review, node *model.Node) bool {
	if review.Status != "PENDING" || node == nil || node.Input == nil {
		return true
	}
	phase, _ := node.Input["reviewPhase"].(string)
	if phase != "after_artifact" && phase != "quality_gate" && phase != "before_downstream" {
		return true
	}
	if strings.TrimSpace(review.ReviewContent) != "" {
		return true
	}
	if len(review.ReviewArtifacts) > 0 {
		return true
	}
	return reviewOutputReadyForDecision(review.ReviewOutput)
}

func reviewOutputReadyForDecision(output map[string]interface{}) bool {
	if len(output) == 0 {
		return false
	}
	if strings.TrimSpace(reviewContentFromPayload(output)) != "" {
		return true
	}
	if len(reviewArtifactList(output["artifacts"])) > 0 {
		return true
	}
	for _, key := range []string{"proposalPacket", "qualityReport", "package", "cardPlan", "shotList", "compositionSpec", "preview", "renderReport", "publishCopy", "finalReview"} {
		if reviewValuePresent(output[key]) {
			return true
		}
	}
	if stdout, ok := output["stdout"].(string); ok && strings.TrimSpace(stdout) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
			return reviewOutputReadyForDecision(parsed)
		}
	}
	return false
}

func reviewValuePresent(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return value != nil
	}
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
	ID                  string                   `json:"id"`
	NodeID              string                   `json:"nodeId"`
	Status              string                   `json:"status"`
	StepID              string                   `json:"stepId,omitempty"`
	Tool                string                   `json:"tool,omitempty"`
	Stage               string                   `json:"stage,omitempty"`
	RoleAgentID         string                   `json:"roleAgentId,omitempty"`
	RoleAgent           map[string]interface{}   `json:"roleAgent,omitempty"`
	HumanReview         map[string]interface{}   `json:"humanReview,omitempty"`
	RequiredInputs      []string                 `json:"requiredInputs,omitempty"`
	RequiredOutputs     []string                 `json:"requiredOutputs,omitempty"`
	ReviewPhase         string                   `json:"reviewPhase,omitempty"`
	ReviewReason        string                   `json:"reviewReason,omitempty"`
	BlocksDownstream    bool                     `json:"blocksDownstream,omitempty"`
	ReviewArtifactKinds []string                 `json:"reviewArtifactKinds,omitempty"`
	ArtifactID          string                   `json:"artifactId,omitempty"`
	SourceNodeID        string                   `json:"sourceNodeId,omitempty"`
	ReviewContent       string                   `json:"reviewContent,omitempty"`
	ReviewArtifacts     []map[string]interface{} `json:"reviewArtifacts,omitempty"`
	ReviewOutput        map[string]interface{}   `json:"reviewOutput,omitempty"`
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
		review.ArtifactID, _ = node.Input["artifactId"].(string)
		review.SourceNodeID, _ = node.Input["sourceNode"].(string)
		if review.ReviewPhase == "quality_gate" {
			if reviewTool, _ := node.Input["reviewTool"].(string); reviewTool != "" {
				review.Tool = reviewTool
			}
		}
	}
	return review
}

func enrichReviewFromSourceNode(review *Review, reviewNode *model.Node, nodesByID, nodesByOriginalID map[string]*model.Node) {
	if review == nil || reviewNode == nil || reviewNode.Input == nil {
		return
	}
	sourceNodeID, _ := reviewNode.Input["sourceNode"].(string)
	// For quality gate nodes, the production source is stored under a different key.
	if sourceNodeID == "" {
		sourceNodeID, _ = reviewNode.Input["productionSourceNode"].(string)
	}
	if sourceNodeID == "" {
		return
	}
	source := nodesByID[sourceNodeID]
	if source == nil {
		source = nodesByOriginalID[sourceNodeID]
	}
	if source == nil {
		return
	}
	review.SourceNodeID = source.ID
	if len(review.RequiredOutputs) == 0 && source.Input != nil {
		review.RequiredOutputs = stringSlice(source.Input["requiredOutputs"])
		if len(review.RequiredOutputs) == 0 {
			review.RequiredOutputs = stringSlice(source.Input["expectedOutput"])
		}
	}
	payload := parseReviewOutputPayload(source.Output)
	if len(payload) == 0 {
		return
	}
	if content := reviewContentFromPayload(payload); content != "" {
		review.ReviewContent = content
	}
	review.ReviewArtifacts = reviewArtifactList(payload["artifacts"])
	review.ReviewOutput = payload
	enrichQualityReport(review, reviewNode, nodesByID, nodesByOriginalID)
}

func enrichQualityReport(review *Review, reviewNode *model.Node, nodesByID, nodesByOriginalID map[string]*model.Node) {
	if review == nil || reviewNode == nil || reviewNode.Input == nil {
		return
	}
	if phase, _ := reviewNode.Input["reviewPhase"].(string); phase != "quality_gate" {
		return
	}
	checkerNodeID, _ := reviewNode.Input["qualityCheckerNode"].(string)
	if checkerNodeID == "" {
		checkerNodeID, _ = reviewNode.Input["checkerStep"].(string)
	}
	if checkerNodeID == "" {
		return
	}
	// Try exact match first, then original ID, then fuzzy substring match.
	checker := nodesByID[checkerNodeID]
	if checker == nil {
		checker = nodesByOriginalID[checkerNodeID]
	}
	// The compiled node ID may include a suffix (_review, _exec) that doesn't
	// match the actual node ID. Try substring matching within the same task.
	if checker == nil {
		// Strip common suffixes for a broader match.
		for _, suffix := range []string{"_review", "_exec"} {
			if base, ok := strings.CutSuffix(checkerNodeID, suffix); ok {
				checker = nodesByOriginalID[base]
				if checker == nil {
					checker = nodesByID[base]
				}
				if checker != nil {
					break
				}
			}
		}
		// Last resort: iterate and match by substring.
		if checker == nil {
			for id, node := range nodesByID {
				if strings.Contains(id, checkerNodeID) || strings.Contains(checkerNodeID, id) {
					checker = node
					break
				}
			}
		}
	}
	if checker == nil {
		return
	}
	report := parseReviewOutputPayload(checker.Output)
	if len(report) == 0 {
		return
	}
	if review.ReviewOutput == nil {
		review.ReviewOutput = map[string]interface{}{}
	}
	review.ReviewOutput["qualityReport"] = report
}

func parseReviewOutputPayload(output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return nil
	}
	if stdout, ok := output["stdout"].(string); ok && stdout != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
			return parsed
		}
	}
	return output
}

func reviewContentFromPayload(payload map[string]interface{}) string {
	for _, key := range []string{"content", "script", "text", "markdown", "summary"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func reviewArtifactList(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if artifact, ok := item.(map[string]interface{}); ok {
				result = append(result, artifact)
			}
		}
		return result
	case map[string]interface{}:
		return []map[string]interface{}{typed}
	default:
		return nil
	}
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
		ArtifactID:   r.ArtifactID,
		Status:       ArtifactReviewPending,
		ReviewReason: r.ReviewReason,
	}
	_ = h.artifactReview.Save(ctx, review)
}

func (h *Handler) ensureReviewNodeArtifactID(ctx context.Context, run *Run, node *model.Node) {
	if h.artifactService == nil || h.projectIDResolver == nil || run == nil || node == nil {
		return
	}
	if nodeInputString(node, "artifactId") != "" {
		return
	}
	stageName := nodeInputString(node, "stage")
	if stageName == "" {
		return
	}
	projectID, err := h.projectIDResolver.ResolveProjectID(ctx, run.TaskID)
	if err != nil || projectID == "" {
		return
	}
	current := (*artifact.Artifact)(nil)
	artifactKinds := reviewArtifactKindsFromNode(node)
	for _, kind := range artifactKinds {
		candidate, err := h.artifactService.FindCurrentByStageAndKind(ctx, projectID, stageName, kind)
		if err == nil && candidate != nil && candidate.ID != "" {
			current = candidate
			break
		}
	}
	if current == nil {
		candidate, err := h.artifactService.FindCurrentByKind(ctx, projectID, stageName)
		if err != nil || candidate == nil || candidate.ID == "" {
			return
		}
		current = candidate
	}
	if node.Input == nil {
		node.Input = map[string]interface{}{}
	}
	node.Input["artifactId"] = current.ID
	node.Input["stage"] = stageName
	if len(stringSlice(node.Input["requiredOutputs"])) == 0 {
		node.Input["requiredOutputs"] = []string{string(current.Kind)}
	}
	if len(stringSlice(node.Input["artifactKinds"])) == 0 {
		node.Input["artifactKinds"] = []string{string(current.Kind)}
	}
	if updater, ok := h.nodes.(ReviewNodeInputUpdater); ok {
		_ = updater.UpdateInputFields(ctx, node.ID, map[string]interface{}{
			"artifactId":      current.ID,
			"stage":           stageName,
			"requiredOutputs": stringSlice(node.Input["requiredOutputs"]),
			"artifactKinds":   stringSlice(node.Input["artifactKinds"]),
		})
	}
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
	h.ensureReviewNodeArtifactID(c.Request.Context(), run, node)

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

	// Mark reviewed artifact as approved.
	h.approveReviewedArtifact(c.Request.Context(), run, node, req.ReviewerID)

	// Force downstream stale: editing an artifact invalidates everything downstream.
	h.triggerDownstreamStale(c.Request.Context(), run, node, "用户修改上游产物")

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

	// Force downstream stale: regenerating a stage invalidates everything downstream.
	h.triggerDownstreamStale(c.Request.Context(), run, node, "用户重新生成阶段")

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
	resolvedID, err := h.resolveSourceNodeID(ctx, gateNode, sourceID)
	if err != nil {
		return err
	}
	return h.nodes.UpdateStatus(ctx, resolvedID, model.NodeCreated, nil, "")
}

func (h *Handler) resolveSourceNodeID(ctx context.Context, gateNode *model.Node, sourceID string) (string, error) {
	if h == nil || h.nodes == nil || gateNode == nil || gateNode.TaskID == "" {
		return sourceID, nil
	}
	nodes, err := h.nodes.FindByTaskID(ctx, gateNode.TaskID)
	if err != nil {
		return sourceID, nil
	}
	for _, node := range nodes {
		if node.ID == sourceID {
			return node.ID, nil
		}
	}
	for _, node := range nodes {
		if originalID, _ := node.Input["agentOriginalNodeId"].(string); originalID == sourceID {
			return node.ID, nil
		}
	}
	return "", fmt.Errorf("source exec node %q not found for review gate %s", sourceID, gateNode.ID)
}

// triggerDownstreamStale marks all downstream artifacts as stale in the database
// when an upstream artifact is modified, rejected, or regenerated.
func (h *Handler) triggerDownstreamStale(ctx context.Context, run *Run, node *model.Node, reason string) {
	if h.artifactService == nil || h.projectIDResolver == nil {
		return
	}
	projectID, err := h.projectIDResolver.ResolveProjectID(ctx, run.TaskID)
	if err != nil || projectID == "" {
		zap.L().Warn("cannot resolve project ID for stale tracking",
			zap.String("taskId", run.TaskID),
			zap.Error(err),
		)
		return
	}

	artifactID := nodeInputString(node, "artifactId")
	if artifactID != "" {
		if _, err := h.artifactService.MarkDownstreamStale(ctx, projectID, artifactID, reason); err != nil {
			zap.L().Warn("failed to mark downstream stale by artifact id",
				zap.String("projectId", projectID),
				zap.String("artifactId", artifactID),
				zap.Error(err),
			)
		}
		return
	}

	stageName := nodeInputString(node, "stage")
	if stageName == "" {
		return
	}
	if _, err := h.artifactService.MarkDownstreamStaleByStageName(ctx, projectID, stageName, reason); err != nil {
		zap.L().Warn("failed to mark downstream stale",
			zap.String("projectId", projectID),
			zap.String("stageName", stageName),
			zap.Error(err),
		)
	}
}

// approveReviewedArtifact marks the artifact associated with a review as human-approved.
func (h *Handler) approveReviewedArtifact(ctx context.Context, run *Run, node *model.Node, reviewerID string) {
	if h.artifactService == nil || node == nil {
		return
	}
	// The artifact ID can sometimes be found in the node's input.
	artifactID := nodeInputString(node, "artifactId")
	if artifactID == "" && h.artifactReview != nil {
		if review, err := h.artifactReview.FindByNodeID(ctx, node.ID); err == nil && review != nil {
			artifactID = review.ArtifactID
		}
	}
	if artifactID == "" {
		projectID := h.resolveProjectID(ctx, run)
		stageName := nodeInputString(node, "stage")
		artifactKinds := reviewArtifactKindsFromNode(node)
		if projectID == "" || stageName == "" || len(artifactKinds) == 0 {
			zap.L().Warn("approve artifact skipped: missing fallback fields",
				zap.String("projectId", projectID),
				zap.String("stageName", stageName),
				zap.Strings("artifactKinds", artifactKinds),
				zap.String("nodeId", node.ID),
			)
			return
		}
		approvedIDs, err := h.artifactService.ApproveCurrentArtifactsByStageAndKinds(ctx, projectID, stageName, artifactKinds, reviewerID)
		if err != nil {
			zap.L().Warn("approve artifacts by stage and kinds failed",
				zap.String("projectId", projectID),
				zap.String("stageName", stageName),
				zap.Strings("artifactKinds", artifactKinds),
				zap.Error(err),
			)
			return
		}
		zap.L().Info("approved artifacts by stage and kinds",
			zap.String("projectId", projectID),
			zap.String("stageName", stageName),
			zap.Strings("artifactKinds", artifactKinds),
			zap.Strings("approvedIds", approvedIDs),
		)
		return
	}
	if err := h.artifactService.ApproveArtifact(ctx, artifactID, reviewerID); err != nil {
		zap.L().Warn("approve artifact by id failed", zap.String("artifactId", artifactID), zap.Error(err))
	}
}

func (h *Handler) resolveProjectID(ctx context.Context, run *Run) string {
	if h.projectIDResolver == nil || run == nil || run.TaskID == "" {
		return ""
	}
	projectID, err := h.projectIDResolver.ResolveProjectID(ctx, run.TaskID)
	if err != nil {
		zap.L().Warn("cannot resolve project ID", zap.String("taskId", run.TaskID), zap.Error(err))
		return ""
	}
	return projectID
}

func reviewArtifactKindsFromNode(node *model.Node) []string {
	if node == nil || node.Input == nil {
		return nil
	}
	for _, key := range []string{"artifactKinds", "reviewArtifactKinds", "requiredOutputs"} {
		if values := stringSlice(node.Input[key]); len(values) > 0 {
			return values
		}
	}
	return nil
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
