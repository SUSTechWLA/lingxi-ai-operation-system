package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ChapterGenerationHandler handles concurrent chapter generation requests.
type ChapterGenerationHandler struct {
	gw *modelgateway.Gateway
}

// NewChapterGenerationHandler creates a ChapterGenerationHandler.
func NewChapterGenerationHandler(gw *modelgateway.Gateway) *ChapterGenerationHandler {
	return &ChapterGenerationHandler{gw: gw}
}

// RegisterRoutes registers the chapter generation endpoint.
func (h *ChapterGenerationHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/chapters/generate", h.Generate)
}

// generateChaptersRequest is the JSON body for batch chapter generation.
type generateChaptersRequest struct {
	TaskBookPath       string `json:"taskBookPath"`
	OutlinePath        string `json:"outlinePath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextReportPath  string `json:"contextReportPath,omitempty"`
	OutputDir          string `json:"outputDir"`
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
}

// Generate handles POST /api/biaoshu/chapters/generate.
func (h *ChapterGenerationHandler) Generate(c *gin.Context) {
	var req generateChaptersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateChapters(c.Request.Context(), h.gw, GenerateChaptersRequest{
		TaskBookPath:       req.TaskBookPath,
		OutlinePath:        req.OutlinePath,
		ScoringReportPath:  req.ScoringReportPath,
		AnalysisReportPath: req.AnalysisReportPath,
		ContextReportPath:  req.ContextReportPath,
		OutputDir:          req.OutputDir,
	})
	if err != nil {
		zap.L().Error("chapter generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"chapters":       result.Chapters,
			"success":        result.Success,
			"failed":         result.Failed,
			"warnings":       result.Warnings,
			"totalWordCount": result.TotalWordCount,
			"totalScore":     result.TotalScore,
		},
	})
}
