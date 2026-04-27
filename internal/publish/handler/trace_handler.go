package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	contextSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/context/service"
	orchSvc "github.com/lingxi-ai/lingxi-ai-operation-system/internal/orchestrator/service"
)

type TraceHandler struct {
	orchestratorService *orchSvc.OrchestratorService
	contextService      *contextSvc.ContextService
}

func NewTraceHandler(orchestratorService *orchSvc.OrchestratorService, contextService *contextSvc.ContextService) *TraceHandler {
	return &TraceHandler{orchestratorService: orchestratorService, contextService: contextService}
}

func (h *TraceHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/trace/recent", h.GetRecentTrace)
	r.GET("/api/trace/:taskId", h.GetTrace)
}

func (h *TraceHandler) GetRecentTrace(c *gin.Context) {
	task, err := h.orchestratorService.GetRecentTask(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "no tasks found", "data": nil})
		return
	}
	h.buildAndReturnTrace(c, task.ID)
}

func (h *TraceHandler) GetTrace(c *gin.Context) {
	taskID := c.Param("taskId")
	h.buildAndReturnTrace(c, taskID)
}

func (h *TraceHandler) buildAndReturnTrace(c *gin.Context, taskID string) {
	taskDetails, err := h.orchestratorService.GetTaskWithDetails(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if taskDetails == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "task not found", "data": nil})
		return
	}

	contexts, err := h.contextService.GetContextForTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"task":     taskDetails,
			"contexts": contexts,
		},
	})
}
