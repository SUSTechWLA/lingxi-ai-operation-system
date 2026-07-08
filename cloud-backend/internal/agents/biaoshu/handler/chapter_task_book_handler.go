package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ChapterTaskBookHandler handles chapter task book generation requests.
type ChapterTaskBookHandler struct {
	gw *modelgateway.Gateway
}

// NewChapterTaskBookHandler creates a ChapterTaskBookHandler.
func NewChapterTaskBookHandler(gw *modelgateway.Gateway) *ChapterTaskBookHandler {
	return &ChapterTaskBookHandler{gw: gw}
}

// RegisterRoutes registers the chapter task book generation endpoint.
func (h *ChapterTaskBookHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/chapter-task-book/generate", h.Generate)
}

// Generate handles POST /api/biaoshu/chapter-task-book/generate.
func (h *ChapterTaskBookHandler) Generate(c *gin.Context) {
	var req GenerateChapterTaskBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateChapterTaskBook(c.Request.Context(), h.gw, req)
	if err != nil {
		zap.L().Error("chapter task book generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"taskBookPath": result.TaskBookPath,
			"content":      result.Content,
			"artifact":     result.Artifact,
			"warnings":     result.Warnings,
		},
	})
}
