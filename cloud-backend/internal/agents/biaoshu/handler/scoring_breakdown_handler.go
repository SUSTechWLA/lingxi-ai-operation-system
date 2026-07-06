package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

type ScoringBreakdownHandler struct {
	gw *modelgateway.Gateway
}

func NewScoringBreakdownHandler(gw *modelgateway.Gateway) *ScoringBreakdownHandler {
	return &ScoringBreakdownHandler{gw: gw}
}

func (h *ScoringBreakdownHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/scoring-breakdown")
	api.POST("/generate", h.Generate)
}

type generateScoringBreakdownRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ScoringReportPath  string `json:"scoringReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
}

func (h *ScoringBreakdownHandler) Generate(c *gin.Context) {
	var req generateScoringBreakdownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateScoringBreakdown(c.Request.Context(), h.gw, GenerateScoringBreakdownRequest{
		AnalysisReportPath: req.AnalysisReportPath,
		ScoringReportPath:  req.ScoringReportPath,
		SourceFile:         req.SourceFile,
	})
	if err != nil {
		zap.L().Error("scoring breakdown generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"analysisReportPath": result.AnalysisReportPath,
			"scoringReportPath":  result.ScoringReportPath,
			"artifact":           result.Artifact,
		},
	})
}
