package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/translator/service"
)

type TranslatorHandler struct {
	nlService *service.NlToDagService
}

func NewTranslatorHandler(nlService *service.NlToDagService) *TranslatorHandler {
	return &TranslatorHandler{nlService: nlService}
}

func (h *TranslatorHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/translate", h.Translate)
		api.POST("/translate/submit", h.TranslateAndSubmit)
		api.GET("/task/:taskId/status", h.GetTaskStatus)
		api.GET("/health", h.Health)
	}
}

func (h *TranslatorHandler) Translate(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dag, err := h.nlService.TranslateToDag(c.Request.Context(), request.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dag)
}

func (h *TranslatorHandler) TranslateAndSubmit(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.nlService.TranslateAndSubmit(c.Request.Context(), request.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *TranslatorHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("taskId")

	result, err := h.nlService.GetTaskStatus(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *TranslatorHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "UP",
		"service": "nl-translator",
	})
}
