package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ProjectContextQuestionHandler handles project context question generation.
type ProjectContextQuestionHandler struct {
	gw *modelgateway.Gateway
}

// NewProjectContextQuestionHandler creates a ProjectContextQuestionHandler.
func NewProjectContextQuestionHandler(gw *modelgateway.Gateway) *ProjectContextQuestionHandler {
	return &ProjectContextQuestionHandler{gw: gw}
}

// RegisterRoutes registers the project context questions endpoint.
func (h *ProjectContextQuestionHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/project-context")
	api.POST("/questions", h.GenerateQuestions)
}

// generateQuestionsRequest is the JSON body for the questions endpoint.
type generateQuestionsRequest struct {
	AnalysisReportPath string `json:"analysisReportPath"`
	SourceFile         string `json:"sourceFile,omitempty"`
}

// GenerateQuestions handles POST /api/biaoshu/project-context/questions.
func (h *ProjectContextQuestionHandler) GenerateQuestions(c *gin.Context) {
	var req generateQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateProjectContextQuestions(c.Request.Context(), h.gw, GenerateProjectContextQuestionsRequest{
		AnalysisReportPath: req.AnalysisReportPath,
		SourceFile:         req.SourceFile,
	})
	if err != nil {
		zap.L().Error("project context questions generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"analysisReportPath": result.AnalysisReportPath,
			"questions":          result.Questions,
			"markdown":           result.Markdown,
		},
	})
}
