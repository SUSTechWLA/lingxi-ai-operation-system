package localrunner

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RunnerService interface {
	RegisterRunner(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error)
	Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error
	ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error)
	ReportProgress(ctx context.Context, jobID string, req ProgressRequest) error
	CompleteJob(ctx context.Context, jobID string, req CompleteJobRequest) (*LocalJob, error)
	FailJob(ctx context.Context, jobID string, req FailJobRequest) (*LocalJob, error)
}

type NodeResultSink interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
	OnFailure(ctx context.Context, nodeID string, errorMessage string) error
}

type Handler struct {
	service RunnerService
	results NodeResultSink
}

func NewHandler(service RunnerService, results NodeResultSink) *Handler {
	return &Handler{service: service, results: results}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/api/local-runners/register", h.registerRunner)
	r.POST("/api/local-runners/:runnerId/heartbeat", h.heartbeat)
	r.GET("/api/local-runners/:runnerId/jobs/claim", h.claimJob)
	r.POST("/api/local-jobs/:jobId/progress", h.reportProgress)
	r.POST("/api/local-jobs/:jobId/complete", h.completeJob)
	r.POST("/api/local-jobs/:jobId/fail", h.failJob)
}

func (h *Handler) registerRunner(c *gin.Context) {
	var req RegisterRunnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.service.RegisterRunner(c.Request.Context(), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) heartbeat(c *gin.Context) {
	var req HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Heartbeat(c.Request.Context(), c.Param("runnerId"), req); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) claimJob(c *gin.Context) {
	job, err := h.service.ClaimJob(c.Request.Context(), c.Param("runnerId"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, ClaimJobResponse{Job: job})
}

func (h *Handler) reportProgress(c *gin.Context) {
	var req ProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.ReportProgress(c.Request.Context(), c.Param("jobId"), req); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) completeJob(c *gin.Context) {
	var req CompleteJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	job, err := h.service.CompleteJob(c.Request.Context(), c.Param("jobId"), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if h.results != nil && job != nil && job.NodeID != "" {
		if err := h.results.OnSuccess(c.Request.Context(), job.NodeID, req.Output); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) failJob(c *gin.Context) {
	var req FailJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	job, err := h.service.FailJob(c.Request.Context(), c.Param("jobId"), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if h.results != nil && job != nil && job.NodeID != "" {
		if err := h.results.OnFailure(c.Request.Context(), job.NodeID, errorMessageFromMap(req.Error)); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"code":    status,
		"message": message,
	})
}
