package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/context/service"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
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
		api.GET("/health", h.Health)
	}
}

func (h *ContextHandler) GetContextForTask(c *gin.Context) {
	taskID := c.Param("taskId")

	contexts, err := h.contextService.GetContextForTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, contexts)
}

func (h *ContextHandler) GetLatestSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	snapshot, err := h.contextService.GetLatestSnapshotForNode(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if snapshot == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No snapshot found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"snapshot": snapshot})
}

func (h *ContextHandler) RestoreFromSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	result, err := h.contextService.RestoreNodeFromSnapshot(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No snapshot found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"snapshot": result})
}

func (h *ContextHandler) RecordContext(c *gin.Context) {
	var request struct {
		TaskID  string `json:"taskId"`
		NodeID  string `json:"nodeId"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.contextService.RecordContext(c.Request.Context(), request.TaskID, request.NodeID,
		model.ContextType(request.Type), request.Message, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Context recorded"})
}

func (h *ContextHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "UP",
		"service": "ai-context",
	})
}
