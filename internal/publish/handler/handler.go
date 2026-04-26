package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/service"
)

type PublishHandler struct {
	publishService *service.PublishService
}

func NewPublishHandler(publishService *service.PublishService) *PublishHandler {
	return &PublishHandler{publishService: publishService}
}

func (h *PublishHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/publish", h.PublishContent)
		api.POST("/ai/generate", h.AIGenerateContent)
		api.POST("/ai/polish", h.AIPolishText)
	}
}

func (h *PublishHandler) PublishContent(c *gin.Context) {
	var req service.PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	result, err := h.publishService.PublishContent(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}

func (h *PublishHandler) AIGenerateContent(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	result, err := h.publishService.AIGenerateContent(c.Request.Context(), request.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}

func (h *PublishHandler) AIPolishText(c *gin.Context) {
	var request struct {
		Text string `json:"text" binding:"required"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if request.Type == "" {
		request.Type = "description"
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	content, err := h.publishService.AIPolishText(c.Request.Context(), request.Text, request.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"content": content}})
}
