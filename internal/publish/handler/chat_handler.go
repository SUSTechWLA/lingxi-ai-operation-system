package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/service"
)

type ChatHandler struct {
	chatService *service.ChatService
}

func NewChatHandler(chatService *service.ChatService) *ChatHandler {
	return &ChatHandler{chatService: chatService}
}

func (h *ChatHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/chat/generate", h.ChatGenerate)
		api.POST("/chat/revise", h.ChatRevise)
	}
}

func (h *ChatHandler) ChatGenerate(c *gin.Context) {
	if h.chatService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	var req service.ChatGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required", "data": nil})
		return
	}

	result, err := h.chatService.Generate(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}

func (h *ChatHandler) ChatRevise(c *gin.Context) {
	if h.chatService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	var req service.ChatReviseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required", "data": nil})
		return
	}

	result, err := h.chatService.Revise(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}
