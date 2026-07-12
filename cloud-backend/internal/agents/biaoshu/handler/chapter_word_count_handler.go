package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WordCountHandler handles word count check requests.
type WordCountHandler struct{}

// NewWordCountHandler creates a WordCountHandler.
func NewWordCountHandler() *WordCountHandler {
	return &WordCountHandler{}
}

// RegisterRoutes registers the word count check endpoint.
func (h *WordCountHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu")
	api.POST("/chapters/word-count/check", h.Check)
}

// checkRequest is the JSON body for word count check.
type checkRequest struct {
	ChapterPaths      []string `json:"chapterPaths"`
	TaskBookPath      string   `json:"taskBookPath"`
	ScoringReportPath string   `json:"scoringReportPath"`
	OutputReportPath  string   `json:"outputReportPath,omitempty"`
	Mode              string   `json:"mode,omitempty"`
}

// Check handles POST /api/biaoshu/chapters/word-count/check.
func (h *WordCountHandler) Check(c *gin.Context) {
	var req checkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := CheckWordCount(CheckWordCountRequest{
		ChapterPaths:      req.ChapterPaths,
		TaskBookPath:      req.TaskBookPath,
		ScoringReportPath: req.ScoringReportPath,
		OutputReportPath:  req.OutputReportPath,
		Mode:              req.Mode,
	})
	if err != nil {
		zap.L().Error("word count check failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
