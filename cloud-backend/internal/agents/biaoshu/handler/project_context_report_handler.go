package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ProjectContextReportHandler handles project context report generation.
type ProjectContextReportHandler struct {
	gw *modelgateway.Gateway
}

// NewProjectContextReportHandler creates a ProjectContextReportHandler.
func NewProjectContextReportHandler(gw *modelgateway.Gateway) *ProjectContextReportHandler {
	return &ProjectContextReportHandler{gw: gw}
}

// RegisterRoutes registers the project context report endpoint.
func (h *ProjectContextReportHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/project-context")
	api.POST("/report/generate", h.GenerateReport)
}

// generateReportRequest is the JSON body for the report generation endpoint.
type generateReportRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	ContextAnswers     string `json:"contextAnswers"`
	ContextReportPath  string `json:"contextReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
}

// GenerateReport handles POST /api/biaoshu/project-context/report/generate.
func (h *ProjectContextReportHandler) GenerateReport(c *gin.Context) {
	var req generateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateProjectContextReport(c.Request.Context(), h.gw, GenerateProjectContextReportRequest{
		AnalysisReportPath: req.AnalysisReportPath,
		ContextAnswers:     req.ContextAnswers,
		ContextReportPath:  req.ContextReportPath,
		SourceFile:         req.SourceFile,
	})
	if err != nil {
		zap.L().Error("project context report generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"analysisReportPath": result.AnalysisReportPath,
			"contextReportPath":  result.ContextReportPath,
			"artifact":           result.Artifact,
		},
	})
}
