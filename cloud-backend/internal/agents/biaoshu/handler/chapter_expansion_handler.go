package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ExpansionHandler handles chapter expansion and task book generation.
type ExpansionHandler struct {
	gw *modelgateway.Gateway
}

// NewExpansionHandler creates an ExpansionHandler.
func NewExpansionHandler(gw *modelgateway.Gateway) *ExpansionHandler {
	return &ExpansionHandler{gw: gw}
}

// RegisterRoutes registers expansion endpoints.
func (h *ExpansionHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/chapters/expansion-task-book/generate", h.GenerateTaskBook)
	api.POST("/chapters/expand", h.Expand)
	api.POST("/chapters/expansion-qa", h.RunQA)
}

// generateTaskBookRequest is the JSON body for task book generation.
type generateTaskBookRequest struct {
	OutputDir      string          `json:"outputDir"`
	TaskBookPath   string          `json:"taskBookPath"`
	WordCountItems []WordCountItem `json:"items"`
}

// GenerateTaskBook handles POST /api/biaoshu/chapters/expansion-task-book/generate.
func (h *ExpansionHandler) GenerateTaskBook(c *gin.Context) {
	var req generateTaskBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if req.OutputDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "outputDir is required"})
		return
	}

	if len(req.WordCountItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "items is empty — run word count check first"})
		return
	}

	taskBookContent := ""
	if req.TaskBookPath != "" {
		b, err := os.ReadFile(req.TaskBookPath)
		if err == nil {
			taskBookContent = string(b)
		}
	}

	result, err := GenerateExpansionTaskBook(req.OutputDir, req.WordCountItems, taskBookContent)
	if err != nil {
		zap.L().Error("failed to generate expansion task book", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// qaRequest is the JSON body for expansion QA.
type qaRequest struct {
	OutputDir     string              `json:"outputDir"`
	ExpandedItems []ExpandChapterItem `json:"expandedItems"`
}

// RunQA handles POST /api/biaoshu/chapters/expansion-qa.
func (h *ExpansionHandler) RunQA(c *gin.Context) {
	var req qaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := RunExpansionQA(req.OutputDir, req.ExpandedItems)
	if err != nil {
		zap.L().Error("expansion QA failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// expandRequest is the JSON body for batch chapter expansion.
type expandRequest struct {
	Items              []WordCountItem `json:"items"`
	TaskBookPath       string          `json:"taskBookPath"`
	OutlinePath        string          `json:"outlinePath"`
	ScoringReportPath  string          `json:"scoringReportPath"`
	AnalysisReportPath string          `json:"analysisReportPath"`
	ContextReportPath  string          `json:"contextReportPath,omitempty"`
	OutputDir          string          `json:"outputDir"`
}

// Expand handles POST /api/biaoshu/chapters/expand.
func (h *ExpansionHandler) Expand(c *gin.Context) {
	var req expandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := ExpandChapters(c.Request.Context(), h.gw, ExpandChaptersRequest{
		Items:        req.Items,
		TaskBookPath: req.TaskBookPath,
		OutlinePath:  req.OutlinePath,
		ScoringPath:  req.ScoringReportPath,
		AnalysisPath: req.AnalysisReportPath,
		ContextPath:  req.ContextReportPath,
		OutputDir:    req.OutputDir,
	})
	if err != nil {
		zap.L().Error("chapter expansion failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
