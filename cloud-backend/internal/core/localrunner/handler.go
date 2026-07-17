package localrunner

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

type RunnerService interface {
	RegisterRunner(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error)
	Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error
	ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error)
	ReportProgress(ctx context.Context, jobID string, req ProgressRequest) error
	CompleteJob(ctx context.Context, jobID string, req CompleteJobRequest) (*LocalJob, error)
	FailJob(ctx context.Context, jobID string, req FailJobRequest) (*LocalJob, error)
	GetJob(ctx context.Context, jobID string) (*LocalJob, error)
	ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error
	ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error
}

type NodeResultSink interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
	OnFailure(ctx context.Context, nodeID string, errorMessage string) error
	OnProgress(ctx context.Context, nodeID string, progress float64, step, message string) error
}

// ArtifactSyncCallback is invoked after a local job completes successfully,
// allowing the caller to materialize artifact records from the job output.
type ArtifactSyncCallback func(ctx context.Context, projectID, taskID, nodeID, toolName, command string, output map[string]interface{}) error

type Handler struct {
	service              RunnerService
	results              NodeResultSink
	artifactSyncCallback ArtifactSyncCallback
	middleware           []gin.HandlerFunc
}

func NewHandler(service RunnerService, results NodeResultSink, middleware ...gin.HandlerFunc) *Handler {
	return &Handler{service: service, results: results, middleware: middleware}
}

// WithArtifactSyncCallback sets a callback that is invoked after each successful
// local job completion to sync artifacts to the cloud ArtifactIndex.
func (h *Handler) WithArtifactSyncCallback(cb ArtifactSyncCallback) *Handler {
	h.artifactSyncCallback = cb
	return h
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	group := r.Group("/")
	if len(h.middleware) > 0 {
		group.Use(h.middleware...)
	}
	group.POST("/api/local-runners/register", h.registerRunner)
	group.POST("/api/local-runners/:runnerId/heartbeat", h.heartbeat)
	group.GET("/api/local-runners/:runnerId/jobs/claim", h.claimJob)
	group.POST("/api/local-jobs/:jobId/progress", h.reportProgress)
	group.POST("/api/local-jobs/:jobId/complete", h.completeJob)
	group.POST("/api/local-jobs/:jobId/fail", h.failJob)
}

func (h *Handler) registerRunner(c *gin.Context) {
	var req RegisterRunnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if userID, ok := auth.UserIDFromContext(c.Request.Context()); ok {
		req.UserID = userID
	}
	if deviceID, ok := auth.DeviceIDFromContext(c.Request.Context()); ok {
		req.DeviceID = deviceID
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
	if err := h.validateRunnerFromRequest(c); err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	if err := h.service.Heartbeat(c.Request.Context(), c.Param("runnerId"), req); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) claimJob(c *gin.Context) {
	if err := h.validateRunnerFromRequest(c); err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
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
	if err := h.validateJobFromRequest(c); err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	if err := h.service.ReportProgress(c.Request.Context(), c.Param("jobId"), req); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync progress to the ai_node so the frontend can display real-time
	// local execution progress instead of just WAITING_LOCAL.
	if h.results != nil {
		job, err := h.service.GetJob(c.Request.Context(), c.Param("jobId"))
		if err == nil && job != nil && job.NodeID != "" {
			_ = h.results.OnProgress(c.Request.Context(), job.NodeID, req.Progress, req.Step, req.Message)
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) completeJob(c *gin.Context) {
	var req CompleteJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateJobFromRequest(c); err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	jobContext, _ := h.service.GetJob(c.Request.Context(), c.Param("jobId"))
	req.Output = normalizeCompleteJobOutput(jobContext, req.Output)
	job, err := h.service.CompleteJob(c.Request.Context(), c.Param("jobId"), req)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if jobContext == nil {
		jobContext = job
	}
	req.Output = normalizeCompleteJobOutput(jobContext, req.Output)
	if h.results != nil && job != nil && job.NodeID != "" {
		if err := h.results.OnSuccess(c.Request.Context(), job.NodeID, req.Output); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	// Sync artifact metadata to cloud ArtifactIndex after local job completion.
	if h.artifactSyncCallback != nil && job != nil {
		if err := h.artifactSyncCallback(c.Request.Context(), job.ProjectID, job.TaskID, job.NodeID, job.ToolName, string(job.Command), req.Output); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func normalizeCompleteJobOutput(job *LocalJob, output map[string]interface{}) map[string]interface{} {
	if output == nil {
		output = map[string]interface{}{}
	}
	if job == nil || NormalizeCommand(string(job.Command)) != CommandHyperFramesRender {
		return output
	}

	artifacts, _ := output["artifacts"].([]interface{})
	video := firstVideoArtifact(artifacts)
	if video == nil {
		video = map[string]interface{}{}
		artifacts = append(artifacts, video)
	}

	name := stringFromOutput(output, "name")
	if name == "" {
		name = stringFromOutput(output, "outputName")
	}
	if name == "" {
		if v, ok := job.Payload["outputName"].(string); ok {
			name = v
		}
	}
	if name == "" {
		name = "final.mp4"
	}

	storageRef := stringFromOutput(output, "storageRef")
	if storageRef == "" {
		storageRef = stringFromOutput(video, "storageRef")
	}
	if storageRef == "" {
		storageRef = "local://projects/" + job.ProjectID + "/renders/" + name
	}

	sizeBytes := output["sizeBytes"]
	if sizeBytes == nil {
		sizeBytes = video["sizeBytes"]
	}

	metadata, _ := video["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if value, ok := output["renderTimeMs"]; ok {
		metadata["renderTimeMs"] = value
	}
	if value, ok := output["fps"]; ok {
		metadata["fps"] = value
	}
	if value, ok := output["width"]; ok {
		metadata["width"] = value
	}
	if value, ok := output["height"]; ok {
		metadata["height"] = value
	}
	if metrics, ok := output["metrics"].(map[string]interface{}); ok {
		if value, ok := metrics["renderTimeMs"]; ok {
			metadata["renderTimeMs"] = value
		}
		if value, ok := metrics["fps"]; ok {
			metadata["fps"] = value
		}
		if value, ok := metrics["width"]; ok {
			metadata["width"] = value
		}
		if value, ok := metrics["height"]; ok {
			metadata["height"] = value
		}
	}

	video["unitId"] = "final-video"
	video["kind"] = "VIDEO"
	video["name"] = name
	video["storageType"] = "local"
	video["storageRef"] = storageRef
	video["mimeType"] = "video/mp4"
	if sizeBytes != nil {
		video["sizeBytes"] = sizeBytes
	}
	video["status"] = "valid"
	video["humanApproved"] = false
	video["dependsOn"] = []interface{}{"PREVIEW_SNAPSHOTS", "HYPERFRAMES_PROJECT"}
	video["producedByTool"] = "hyperframes_renderer"
	video["producedByRole"] = "渲染制片"
	video["metadata"] = metadata
	output["artifacts"] = artifacts
	return output
}

func firstVideoArtifact(artifacts []interface{}) map[string]interface{} {
	for _, item := range artifacts {
		artifact, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if kind, _ := artifact["kind"].(string); kind == "VIDEO" {
			return artifact
		}
	}
	return nil
}

func stringFromOutput(values map[string]interface{}, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func (h *Handler) failJob(c *gin.Context) {
	var req FailJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateJobFromRequest(c); err != nil {
		writeError(c, http.StatusForbidden, err.Error())
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

// validateRunnerFromRequest checks that the runner ID from the URL matches
// the authenticated user and the X-Runner-ID / X-Runner-Session-ID headers.
func (h *Handler) validateRunnerFromRequest(c *gin.Context) error {
	runnerID := c.Param("runnerId")
	userID, _ := auth.UserIDFromContext(c.Request.Context())
	deviceID, _ := auth.DeviceIDFromContext(c.Request.Context())
	headerRunnerID := c.GetHeader("X-Runner-ID")
	sessionID := c.GetHeader("X-Runner-Session-ID")
	if sessionID == "" {
		sessionID = c.GetHeader("X-Runner-Session-Id")
	}

	// If X-Runner-ID is present, it must match the URL param
	if headerRunnerID != "" && headerRunnerID != runnerID {
		return ErrRunnerAccessDenied
	}

	return h.service.ValidateRunnerAccess(c.Request.Context(), userID, deviceID, runnerID, sessionID)
}

// validateJobFromRequest checks that the requesting runner (from X-Runner-ID header)
// has access to the job.
func (h *Handler) validateJobFromRequest(c *gin.Context) error {
	jobID := c.Param("jobId")
	userID, _ := auth.UserIDFromContext(c.Request.Context())
	runnerID := c.GetHeader("X-Runner-ID")
	if runnerID == "" {
		runnerID = c.GetHeader("X-Runner-Id")
	}
	if runnerID == "" {
		return fmt.Errorf("X-Runner-ID header required")
	}
	return h.service.ValidateJobAccess(c.Request.Context(), userID, runnerID, jobID)
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"code":    status,
		"message": message,
	})
}
