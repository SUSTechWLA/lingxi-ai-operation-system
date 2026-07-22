package localrunner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type RunnerService interface {
	RegisterRunner(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error)
	Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error
	ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error)
	ReportProgress(ctx context.Context, identity JobMutationIdentity, jobID string, req ProgressRequest) error
	CompleteJob(ctx context.Context, identity JobMutationIdentity, jobID string, req CompleteJobRequest) (*LocalJob, error)
	FailJob(ctx context.Context, identity JobMutationIdentity, jobID string, req FailJobRequest) (*LocalJob, error)
	MarkCallbackDelivered(ctx context.Context, identity JobMutationIdentity, jobID string, phase CallbackPhase) error
	GetJob(ctx context.Context, jobID string) (*LocalJob, error)
	ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error
	ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error
}

type NodeResultSink interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
	OnFailure(ctx context.Context, nodeID string, errorMessage string) error
	OnProgress(ctx context.Context, nodeID string, progress float64, step, message string) error
}

type ToolManifestResolver interface {
	GetManifest(name string) *tool.ToolManifest
}

type ScopedLocalJobManifestResolver interface {
	ResolveMCPJobManifest(ctx context.Context, job *LocalJob) (*tool.ToolManifest, error)
}

// ArtifactSyncCallback is invoked after a local job completes successfully,
// allowing the caller to materialize artifact records from the job output.
type ArtifactSyncCallback func(ctx context.Context, job *LocalJob, output map[string]interface{}) error

// JobFailureCallback is invoked after a local job is durably marked failed.
// The full job context lets domain services resolve their own durable task IDs.
type JobFailureCallback func(ctx context.Context, job *LocalJob, reason string) error

type Handler struct {
	service              RunnerService
	results              NodeResultSink
	artifactSyncCallback ArtifactSyncCallback
	jobFailureCallback   JobFailureCallback
	manifestResolver     ToolManifestResolver
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

// WithJobFailureCallback registers a domain failure callback.
func (h *Handler) WithJobFailureCallback(cb JobFailureCallback) *Handler {
	h.jobFailureCallback = cb
	return h
}

// WithToolManifestResolver enables canonical output validation before a local
// completion is durably marked COMPLETED or published to the DAG state machine.
func (h *Handler) WithToolManifestResolver(resolver ToolManifestResolver) *Handler {
	h.manifestResolver = resolver
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
	identity, err := h.validateJobFromRequest(c)
	if err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	if err := h.service.ReportProgress(c.Request.Context(), identity, c.Param("jobId"), req); err != nil {
		writeJobMutationError(c, err)
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
	identity, err := h.validateJobFromRequest(c)
	if err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	jobContext, err := h.service.GetJob(c.Request.Context(), c.Param("jobId"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if jobContext != nil && (jobContext.Status == JobCompleted || jobContext.Status == JobFailed) {
		if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, jobContext); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
		// The persisted terminal status is authoritative. A late complete report
		// may replay pending callbacks, but can never turn FAILED into success.
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	req.Output = normalizeCompleteJobOutput(jobContext, req.Output)
	if isMCPErrorCompletion(jobContext, req.Output) {
		h.failInvalidCompletion(c, identity, jobContext, fmt.Errorf("MCP tool returned isError=true"))
		return
	}
	manifest, manifestErr := h.manifestForLocalJob(c.Request.Context(), jobContext)
	if manifestErr != nil {
		h.failInvalidCompletion(c, identity, jobContext, manifestErr)
		return
	}
	if manifest != nil {
		if err := tool.ValidateLocalJobOutput(manifest, req.Output); err != nil {
			h.failInvalidCompletion(c, identity, jobContext, err)
			return
		}
	}
	job, err := h.service.CompleteJob(c.Request.Context(), identity, c.Param("jobId"), req)
	if err != nil {
		writeJobMutationError(c, err)
		return
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, job); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) manifestForLocalJob(ctx context.Context, job *LocalJob) (*tool.ToolManifest, error) {
	if h == nil || job == nil {
		return nil, nil
	}
	if NormalizeCommand(job.Command) == CommandLocalMCPToolCall {
		if resolver, ok := h.service.(ScopedLocalJobManifestResolver); ok {
			manifest, err := resolver.ResolveMCPJobManifest(ctx, job)
			if err != nil || manifest != nil {
				return manifest, err
			}
		}
	}
	if h.manifestResolver == nil {
		return nil, nil
	}
	for _, name := range []string{job.MCPLogicalToolName, job.ToolName} {
		if name != "" {
			if manifest := h.manifestResolver.GetManifest(name); manifest != nil {
				return manifest, nil
			}
		}
	}
	return nil, nil
}

func isMCPErrorCompletion(job *LocalJob, output map[string]interface{}) bool {
	if job == nil || NormalizeCommand(job.Command) != CommandLocalMCPToolCall {
		return false
	}
	isError, _ := output["isError"].(bool)
	return isError
}

func (h *Handler) failInvalidCompletion(c *gin.Context, identity JobMutationIdentity, job *LocalJob, validationErr error) {
	code := "OUTPUT_SCHEMA_INVALID"
	if errors.Is(validationErr, tool.ErrMCPToolResult) {
		code = "MCP_TOOL_ERROR"
	}
	message := code + ": " + validationErr.Error()
	failed, err := h.service.FailJob(c.Request.Context(), identity, c.Param("jobId"), FailJobRequest{
		Success: false,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
		Retryable: true,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, failed); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeError(c, http.StatusUnprocessableEntity, message)
}

func (h *Handler) deliverTerminalCallbacks(ctx context.Context, identity JobMutationIdentity, job *LocalJob) error {
	if job == nil || (job.Status != JobCompleted && job.Status != JobFailed) {
		return nil
	}
	if job.ResultCallbackState != CallbackDelivered {
		if job.Status == JobCompleted {
			if h.results != nil && job.NodeID != "" {
				if err := h.results.OnSuccess(ctx, job.NodeID, job.Output); err != nil {
					return err
				}
			}
		} else if h.results != nil && job.NodeID != "" {
			if err := h.results.OnFailure(ctx, job.NodeID, terminalFailureMessage(job)); err != nil {
				return err
			}
		}
		if err := h.service.MarkCallbackDelivered(ctx, identity, job.ID, CallbackPhaseResult); err != nil {
			return err
		}
		job.ResultCallbackState = CallbackDelivered
	}
	if job.FollowupCallbackState != CallbackDelivered {
		if job.Status == JobCompleted {
			if h.artifactSyncCallback != nil {
				if err := h.artifactSyncCallback(ctx, job, job.Output); err != nil {
					return err
				}
			}
		} else if h.jobFailureCallback != nil {
			if err := h.jobFailureCallback(ctx, job, terminalFailureMessage(job)); err != nil {
				return err
			}
		}
		if err := h.service.MarkCallbackDelivered(ctx, identity, job.ID, CallbackPhaseFollowup); err != nil {
			return err
		}
		job.FollowupCallbackState = CallbackDelivered
	}
	return nil
}

func terminalFailureMessage(job *LocalJob) string {
	if job == nil {
		return "local job failed"
	}
	if strings.TrimSpace(job.ErrorMessage) != "" {
		return job.ErrorMessage
	}
	return errorMessageFromMap(job.Error)
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
	identity, err := h.validateJobFromRequest(c)
	if err != nil {
		writeError(c, http.StatusForbidden, err.Error())
		return
	}
	existing, err := h.service.GetJob(c.Request.Context(), c.Param("jobId"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil && (existing.Status == JobCompleted || existing.Status == JobFailed) {
		if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, existing); err != nil {
			writeError(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	job, err := h.service.FailJob(c.Request.Context(), identity, c.Param("jobId"), req)
	if err != nil {
		writeJobMutationError(c, err)
		return
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, job); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func writeJobMutationError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, ErrJobAccessDenied) || errors.Is(err, ErrRunnerAccessDenied) {
		status = http.StatusForbidden
	} else if errors.Is(err, ErrJobAlreadyCompleted) {
		status = http.StatusConflict
	}
	writeError(c, status, err.Error())
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
func (h *Handler) validateJobFromRequest(c *gin.Context) (JobMutationIdentity, error) {
	jobID := c.Param("jobId")
	userID, _ := auth.UserIDFromContext(c.Request.Context())
	deviceID, _ := auth.DeviceIDFromContext(c.Request.Context())
	runnerID := c.GetHeader("X-Runner-ID")
	if runnerID == "" {
		runnerID = c.GetHeader("X-Runner-Id")
	}
	if runnerID == "" {
		return JobMutationIdentity{}, fmt.Errorf("X-Runner-ID header required")
	}
	sessionID := c.GetHeader("X-Runner-Session-ID")
	if sessionID == "" {
		sessionID = c.GetHeader("X-Runner-Session-Id")
	}
	if err := h.service.ValidateRunnerAccess(c.Request.Context(), userID, deviceID, runnerID, sessionID); err != nil {
		return JobMutationIdentity{}, err
	}
	if err := h.service.ValidateJobAccess(c.Request.Context(), userID, runnerID, jobID); err != nil {
		return JobMutationIdentity{}, err
	}
	return JobMutationIdentity{UserID: userID, DeviceID: deviceID, RunnerID: runnerID, SessionID: sessionID}, nil
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"code":    status,
		"message": message,
	})
}
