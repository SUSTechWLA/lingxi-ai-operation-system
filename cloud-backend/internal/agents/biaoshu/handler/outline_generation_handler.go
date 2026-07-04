package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// OutlineGenerationHandler handles outline generation requests.
type OutlineGenerationHandler struct {
	gw *modelgateway.Gateway
}

// NewOutlineGenerationHandler creates an OutlineGenerationHandler.
func NewOutlineGenerationHandler(gw *modelgateway.Gateway) *OutlineGenerationHandler {
	return &OutlineGenerationHandler{gw: gw}
}

// RegisterRoutes registers the outline generation endpoint.
func (h *OutlineGenerationHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/outline/generate", h.Generate)
}

// generateOutlineRequest is the JSON body for outline generation.
type generateOutlineRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextReportPath  string `json:"contextReportPath"`
	ScoringReportPath  string `json:"scoringReportPath,omitempty"`
	OutlinePath        string `json:"outlinePath"`
	SourceFile         string `json:"sourceFile,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
}

// Generate handles POST /api/biaoshu/outline/generate.
func (h *OutlineGenerationHandler) Generate(c *gin.Context) {
	var req generateOutlineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateOutline(c.Request.Context(), h.gw, GenerateOutlineRequest{
		AnalysisReportPath: req.AnalysisReportPath,
		ContextReportPath:  req.ContextReportPath,
		ScoringReportPath:  req.ScoringReportPath,
		OutlinePath:        req.OutlinePath,
		SourceFile:         req.SourceFile,
	})
	if err != nil {
		zap.L().Error("outline generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"analysisReportPath": result.AnalysisReportPath,
			"contextReportPath":  result.ContextReportPath,
			"outlinePath":        result.OutlinePath,
			"artifact":           result.Artifact,
		},
	})
}
