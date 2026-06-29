package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
	"github.com/tangying-ai/aios-core/internal/core/translator/service"
)

type TranslatorHandler struct {
	nlService *service.NlToDagService
}

func NewTranslatorHandler(nlService *service.NlToDagService) *TranslatorHandler {
	return &TranslatorHandler{nlService: nlService}
}

func (h *TranslatorHandler) RegisterRoutes(r *gin.Engine, middleware ...gin.HandlerFunc) {
	api := r.Group("/api", middleware...)
	{
		api.POST("/translate", h.Translate)
		api.POST("/translate/submit", h.TranslateAndSubmit)
		api.GET("/task/:taskId/status", h.GetTaskStatus)
	}
}

func (h *TranslatorHandler) Translate(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	dag, err := h.nlService.TranslateToDag(c.Request.Context(), request.Prompt)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.OK(c, dag)
}

func (h *TranslatorHandler) TranslateAndSubmit(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.nlService.TranslateAndSubmit(c.Request.Context(), request.Prompt)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.OK(c, result)
}

func (h *TranslatorHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("taskId")

	result, err := h.nlService.GetTaskStatus(c.Request.Context(), taskID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.OK(c, result)
}

func (h *TranslatorHandler) Health(c *gin.Context) {
	httpx.OK(c, gin.H{
		"status":  "UP",
		"service": "nl-translator",
	})
}
