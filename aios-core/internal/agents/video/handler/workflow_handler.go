package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

// WorkflowHandler serves HTTP endpoints for video workflow run management.
type WorkflowHandler struct {
	runSvc *workflow.RunService
}

func NewWorkflowHandler(runSvc *workflow.RunService) *WorkflowHandler {
	return &WorkflowHandler{runSvc: runSvc}
}

func (h *WorkflowHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects/:pid/workflow-runs")
	{
		api.POST("", h.CreateRun)
		api.GET("/:rid", h.GetRun)
		api.POST("/:rid/pause", h.PauseRun)
		api.POST("/:rid/cancel", h.CancelRun)
	}
}

// POST /api/video-projects/:pid/workflow-runs
func (h *WorkflowHandler) CreateRun(c *gin.Context) {
	projectID := c.Param("pid")
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

// GET /api/video-projects/:pid/workflow-runs/:rid
func (h *WorkflowHandler) GetRun(c *gin.Context) {
	run, err := h.runSvc.GetRun(c.Request.Context(), c.Param("rid"))
	if err != nil {
		fail(c, 404, "run not found")
		return
	}
	ok(c, gin.H{"run": run})
}

// POST /api/video-projects/:pid/workflow-runs/:rid/pause
func (h *WorkflowHandler) PauseRun(c *gin.Context) {
	if err := h.runSvc.PauseRun(c.Request.Context(), c.Param("rid"), "user requested"); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "paused"})
}

// POST /api/video-projects/:pid/workflow-runs/:rid/cancel
func (h *WorkflowHandler) CancelRun(c *gin.Context) {
	if err := h.runSvc.CancelRun(c.Request.Context(), c.Param("rid")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "cancelled"})
}

// Ensure http import is used
var _ = http.StatusOK
