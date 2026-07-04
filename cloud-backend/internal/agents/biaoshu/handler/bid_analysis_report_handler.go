package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// BidAnalysisReportHandler handles bid analysis report generation requests.
type BidAnalysisReportHandler struct {
	gw *modelgateway.Gateway
}

// NewBidAnalysisReportHandler creates a BidAnalysisReportHandler.
func NewBidAnalysisReportHandler(gw *modelgateway.Gateway) *BidAnalysisReportHandler {
	return &BidAnalysisReportHandler{gw: gw}
}

// RegisterRoutes registers the bid analysis report endpoint.
func (h *BidAnalysisReportHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/bid-analysis-report")
	api.POST("/generate", h.Generate)
}

// generateRequest is the JSON body for the generate endpoint.
type generateRequest struct {
	RawTextPath string `json:"rawTextPath"`
	ReportPath  string `json:"reportPath"`
	SourceFile  string `json:"sourceFile,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	RunID       string `json:"runId,omitempty"`
}

// Generate handles POST /api/biaoshu/bid-analysis-report/generate.
func (h *BidAnalysisReportHandler) Generate(c *gin.Context) {
	var req generateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	result, err := GenerateBidAnalysisReport(c.Request.Context(), h.gw, GenerateBidAnalysisReportRequest{
		RawTextPath: req.RawTextPath,
		ReportPath:  req.ReportPath,
		SourceFile:  req.SourceFile,
	})
	if err != nil {
		zap.L().Error("bid analysis report generation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"rawTextPath": result.RawTextPath,
			"reportPath":  result.ReportPath,
			"artifact":    result.Artifact,
		},
	})
}
