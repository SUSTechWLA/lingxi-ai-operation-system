package handler

import (
	"context"
	"errors"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

type StageApprover interface {
	ApproveStage(ctx context.Context, projectID, runID, stageName string, output map[string]interface{}) (*model.Node, error)
}

// WorkflowHandler serves HTTP endpoints for video workflow run management.
type WorkflowHandler struct {
	runSvc        *workflow.RunService
	stageApprover StageApprover
}

func NewWorkflowHandler(runSvc *workflow.RunService, stageApprover StageApprover) *WorkflowHandler {
	return &WorkflowHandler{runSvc: runSvc, stageApprover: stageApprover}
}

func (h *WorkflowHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects/:id/workflow-runs")
	{
		api.POST("", h.CreateRun)
		api.GET("/:rid", h.GetRun)
		api.POST("/:rid/pause", h.PauseRun)
		api.POST("/:rid/cancel", h.CancelRun)
	}

	stages := r.Group("/api/video-projects/:id/stages")
	{
		stages.POST("/:stage/approve", h.ApproveStage)
	}
}

// POST /api/video-projects/:id/workflow-runs
func (h *WorkflowHandler) CreateRun(c *gin.Context) {
	projectID := c.Param("id")
	var req struct {
		TemplateID      string                 `json:"templateId"`
		TemplateVersion string                 `json:"templateVersion"`
		Input           map[string]interface{} `json:"input"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	run, err := h.runSvc.CreateRun(c.Request.Context(), projectID, req.TemplateID, req.TemplateVersion, req.Input)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"run": run})
}

// GET /api/video-projects/:id/workflow-runs/:rid
func (h *WorkflowHandler) GetRun(c *gin.Context) {
	run, err := h.runSvc.GetRun(c.Request.Context(), c.Param("rid"))
	if err != nil {
		fail(c, 404, "run not found")
		return
	}
	ok(c, gin.H{"run": run})
}

// POST /api/video-projects/:id/workflow-runs/:rid/pause
func (h *WorkflowHandler) PauseRun(c *gin.Context) {
	if err := h.runSvc.PauseRun(c.Request.Context(), c.Param("rid"), "user requested"); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "paused"})
}

// POST /api/video-projects/:id/workflow-runs/:rid/cancel
func (h *WorkflowHandler) CancelRun(c *gin.Context) {
	if err := h.runSvc.CancelRun(c.Request.Context(), c.Param("rid")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "cancelled"})
}

// POST /api/video-projects/:id/stages/:stage/approve
func (h *WorkflowHandler) ApproveStage(c *gin.Context) {
	if h.stageApprover == nil {
		fail(c, 503, "stage approval service is not configured")
		return
	}

	var req struct {
		RunID   string                 `json:"runId"`
		Output  map[string]interface{} `json:"output"`
		Comment string                 `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	if req.Output == nil {
		req.Output = map[string]interface{}{}
	}
	req.Output["approved"] = true
	req.Output["stage"] = c.Param("stage")
	req.Output["projectId"] = c.Param("id")
	if req.RunID != "" {
		req.Output["runId"] = req.RunID
	}
	if req.Comment != "" {
		req.Output["comment"] = req.Comment
	}

	node, err := h.stageApprover.ApproveStage(c.Request.Context(), c.Param("id"), req.RunID, c.Param("stage"), req.Output)
	if err != nil {
		fail(c, approvalErrorStatus(err), err.Error())
		return
	}
	ok(c, gin.H{"nodeId": node.ID, "stage": c.Param("stage"), "message": "approved"})
}

func approvalErrorStatus(err error) int {
	switch {
	case errors.Is(err, workflow.ErrRunNotFound), errors.Is(err, workflow.ErrStageNotFound), errors.Is(err, workflow.ErrRunProjectMismatch):
		return 404
	case errors.Is(err, workflow.ErrStageNotReady):
		return 409
	case errors.Is(err, workflow.ErrStageNotControl):
		return 400
	default:
		return 500
	}
}
