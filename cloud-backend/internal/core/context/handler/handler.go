package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
	"github.com/tangying-ai/aios-core/internal/core/context/service"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

type ContextHandler struct {
	contextService *service.ContextService
}

func NewContextHandler(contextService *service.ContextService) *ContextHandler {
	return &ContextHandler{contextService: contextService}
}

func (h *ContextHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/context/:taskId", h.GetContextForTask)
		api.GET("/context/:taskId/node/:nodeId/snapshot/latest", h.GetLatestSnapshot)
		api.POST("/context/:taskId/node/:nodeId/restore", h.RestoreFromSnapshot)
		api.POST("/context/record", h.RecordContext)
	}
}

func (h *ContextHandler) GetContextForTask(c *gin.Context) {
	taskID := c.Param("taskId")

	contexts, err := h.contextService.GetContextForTask(c.Request.Context(), taskID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.OK(c, contexts)
}

func (h *ContextHandler) GetLatestSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	snapshot, err := h.contextService.GetLatestSnapshotForNode(c.Request.Context(), nodeID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if snapshot == nil {
		httpx.Fail(c, http.StatusNotFound, "No snapshot found")
		return
	}

	httpx.OK(c, gin.H{"snapshot": snapshot})
}

func (h *ContextHandler) RestoreFromSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	result, err := h.contextService.RestoreNodeFromSnapshot(c.Request.Context(), nodeID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		httpx.Fail(c, http.StatusNotFound, "No snapshot found")
		return
	}

	httpx.OK(c, gin.H{"snapshot": result})
}

func (h *ContextHandler) RecordContext(c *gin.Context) {
	var request struct {
		TaskID  string `json:"taskId"`
		NodeID  string `json:"nodeId"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	err := h.contextService.RecordContext(c.Request.Context(), request.TaskID, request.NodeID,
		model.ContextType(request.Type), "", request.Message, nil)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.OKWith(c, "success", gin.H{"message": "Context recorded"})
}

func (h *ContextHandler) Health(c *gin.Context) {
	httpx.OK(c, gin.H{
		"status":  "UP",
		"service": "ai-context",
	})
}
