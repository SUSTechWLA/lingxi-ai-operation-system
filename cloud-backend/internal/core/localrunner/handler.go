package localrunner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type RunnerService interface {
	RegisterRunner(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error)
	Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error
	ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error)
	ReportProgress(ctx context.Context, identity JobMutationIdentity, jobID string, req ProgressRequest) error
	CompleteJob(ctx context.Context, identity JobMutationIdentity, jobID string, req CompleteJobRequest) (*LocalJob, error)
	FailJob(ctx context.Context, identity JobMutationIdentity, jobID string, req FailJobRequest) (*LocalJob, error)
	ClaimTerminalCallback(ctx context.Context, identity JobMutationIdentity, jobID string, phase CallbackPhase) (*TerminalCallbackClaim, error)
	AcknowledgeTerminalCallback(ctx context.Context, jobID string, phase CallbackPhase, token string) error
	ReleaseTerminalCallback(ctx context.Context, jobID string, phase CallbackPhase, token string) error
	GetJob(ctx context.Context, jobID string) (*LocalJob, error)
	ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error
	ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error
}

type NodeResultSink interface {
	OnSuccess(ctx context.Context, nodeID string, output map[string]interface{}) error
	OnFailure(ctx context.Context, nodeID string, errorMessage string) error
	OnProgress(ctx context.Context, nodeID string, progress float64, step, message string) error
}

type callbackIdempotencyContextKey struct{}

// CallbackIdempotencyKeyFromContext returns the stable delivery key attached to
// terminal callback invocations. Sinks that support idempotent writes should
// persist this key with their side effect.
func CallbackIdempotencyKeyFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(callbackIdempotencyContextKey{}).(string)
	return value, ok && value != ""
}

type ToolManifestResolver interface {
	GetManifest(name string) *tool.ToolManifest
}

type ScopedLocalJobManifestResolver interface {
	ResolveMCPJobManifest(ctx context.Context, job *LocalJob) (*tool.ToolManifest, error)
}

// StaleMCPJobRetirer atomically retires MCP jobs whose immutable catalog
// binding is no longer advertised by the authenticated target runner. The
// returned terminal jobs include any pending callback replay work.
type StaleMCPJobRetirer interface {
	RetireStaleMCPJobs(ctx context.Context, identity JobMutationIdentity) ([]*LocalJob, error)
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
	events               observability.EventEmitter
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

func (h *Handler) WithObservability(emitter observability.EventEmitter) *Handler {
	h.events = emitter
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
	if err := h.retireStaleMCPJobs(c.Request.Context(), h.runnerIdentityFromRequest(c)); err != nil {
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
	if err := h.retireStaleMCPJobs(c.Request.Context(), h.runnerIdentityFromRequest(c)); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := h.service.ClaimJob(c.Request.Context(), c.Param("runnerId"))
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if job != nil {
		ctx := observability.EnsureCorrelation(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		correlation, attempt := localJobCorrelation(ctx, job)
		h.emitLocalJob(ctx, observability.EventTypeLocalJobStarted, observability.ExecutionStatusStarted,
			observability.SeverityInfo, correlation, attempt, nil, nil, nil)
	}
	c.JSON(http.StatusOK, ClaimJobResponse{Job: job})
}

func (h *Handler) retireStaleMCPJobs(ctx context.Context, identity JobMutationIdentity) error {
	retirer, ok := h.service.(StaleMCPJobRetirer)
	if !ok {
		return nil
	}
	jobs, err := retirer.RetireStaleMCPJobs(ctx, identity)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := h.deliverTerminalCallbacks(ctx, identity, job); err != nil {
			return err
		}
	}
	return nil
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
	ctx, finishLifecycle := h.prepareLocalJobTerminal(c.Request.Context(), jobContext)
	c.Request = c.Request.WithContext(ctx)
	defer finishLifecycle(c)
	if jobContext != nil && (jobContext.Status == JobCompleted || jobContext.Status == JobFailed) {
		if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, jobContext); err != nil {
			writeCallbackError(c, err)
			return
		}
		// The persisted terminal status is authoritative. A late complete report
		// may replay pending callbacks, but can never turn FAILED into success.
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	req.Output = normalizeCompleteJobOutput(jobContext, req.Output)
	if isMCPErrorCompletion(jobContext, req.Output) {
		validationErr := tool.ValidateLocalJobOutput(&tool.ToolManifest{Boundary: tool.BoundaryMCPProvider}, req.Output)
		h.failInvalidCompletion(c, identity, jobContext, validationErr)
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
		if h.recoverTerminalMutation(c, identity, err) {
			return
		}
		writeJobMutationError(c, err)
		return
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, job); err != nil {
		writeCallbackError(c, err)
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
		writeCallbackError(c, err)
		return
	}
	writeError(c, http.StatusUnprocessableEntity, message)
}

func (h *Handler) deliverTerminalCallbacks(ctx context.Context, identity JobMutationIdentity, job *LocalJob) error {
	if job == nil || (job.Status != JobCompleted && job.Status != JobFailed) {
		return nil
	}
	if err := h.deliverTerminalCallbackPhase(ctx, identity, job, CallbackPhaseResult); err != nil {
		return err
	}
	return h.deliverTerminalCallbackPhase(ctx, identity, job, CallbackPhaseFollowup)
}

func (h *Handler) deliverTerminalCallbackPhase(ctx context.Context, identity JobMutationIdentity, job *LocalJob, phase CallbackPhase) error {
	claim, err := h.service.ClaimTerminalCallback(ctx, identity, job.ID, phase)
	if err != nil {
		return err
	}
	if claim == nil || claim.Delivered {
		return nil
	}
	callbackCtx := context.WithValue(ctx, callbackIdempotencyContextKey{}, claim.IdempotencyKey)
	deliveryErr := h.invokeTerminalCallback(callbackCtx, job, phase)
	if deliveryErr != nil {
		if releaseErr := h.service.ReleaseTerminalCallback(ctx, job.ID, phase, claim.Token); releaseErr != nil {
			return fmt.Errorf("terminal callback failed: %v; release claim: %w", deliveryErr, releaseErr)
		}
		return deliveryErr
	}
	return h.service.AcknowledgeTerminalCallback(ctx, job.ID, phase, claim.Token)
}

func (h *Handler) invokeTerminalCallback(ctx context.Context, job *LocalJob, phase CallbackPhase) error {
	switch phase {
	case CallbackPhaseResult:
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
		return nil
	case CallbackPhaseFollowup:
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
		return nil
	default:
		return fmt.Errorf("unknown callback phase %q", phase)
	}
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
	ctx, finishLifecycle := h.prepareLocalJobTerminal(c.Request.Context(), existing)
	c.Request = c.Request.WithContext(ctx)
	defer finishLifecycle(c)
	if existing != nil && (existing.Status == JobCompleted || existing.Status == JobFailed) {
		if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, existing); err != nil {
			writeCallbackError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	job, err := h.service.FailJob(c.Request.Context(), identity, c.Param("jobId"), req)
	if err != nil {
		if h.recoverTerminalMutation(c, identity, err) {
			return
		}
		writeJobMutationError(c, err)
		return
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, job); err != nil {
		writeCallbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) prepareLocalJobTerminal(ctx context.Context, job *LocalJob) (context.Context, func(*gin.Context)) {
	ctx = observability.EnsureCorrelation(ctx)
	correlation, attempt := localJobCorrelation(ctx, job)
	return ctx, func(c *gin.Context) {
		var durationMs *int64
		if job != nil && !job.CreatedAt.IsZero() {
			value := time.Since(job.CreatedAt).Milliseconds()
			if value >= 0 {
				durationMs = &value
			}
		}
		eventType := observability.EventTypeLocalJobCompleted
		status := observability.ExecutionStatusCompleted
		severity := observability.SeverityInfo
		recovered := recover()
		if recovered != nil || c == nil || c.Writer.Status() >= http.StatusBadRequest {
			eventType = observability.EventTypeLocalJobFailed
			status = observability.ExecutionStatusFailed
			severity = observability.SeverityError
		}
		h.emitLocalJob(ctx, eventType, status, severity, correlation, attempt, durationMs, nil, nil)
		if recovered != nil {
			panic(recovered)
		}
	}
}

func localJobCorrelation(ctx context.Context, job *LocalJob) (observability.Correlation, int64) {
	correlation := observability.CorrelationFromContext(ctx)
	attempt := int64(1)
	if job == nil {
		return correlation, attempt
	}
	correlation.ProviderJobID = job.ID
	correlation.ProjectID = job.ProjectID
	correlation.TaskID = job.TaskID
	correlation.StageID = job.NodeID
	if job.Attempt > 0 {
		attempt = int64(job.Attempt)
	}
	return correlation, attempt
}

func (h *Handler) emitLocalJob(ctx context.Context, eventType observability.EventType, status observability.ExecutionStatus, severity observability.Severity, correlation observability.Correlation, attempt int64, durationMs *int64, eventErr *observability.EventError, sizeBytes *int64) {
	if h == nil {
		return
	}
	observability.EmitSafely(ctx, h.events, "local-runner", observability.Event{
		EventType:   eventType,
		MessageKey:  string(eventType),
		Severity:    severity,
		Correlation: correlation,
		Execution: observability.Execution{
			Status: status, Attempt: attempt, DurationMs: durationMs,
		},
		Evidence: observability.Evidence{SizeBytes: sizeBytes},
		Error:    eventErr,
		Privacy: observability.Privacy{
			Classification: observability.PrivacyInternal,
			RedactedFields: []string{"localJob.payload", "localJob.output", "localJob.error", "localJob.diagnostics"},
		},
	})
}

// recoverTerminalMutation handles the loser of a concurrent complete/fail CAS.
// The terminal row is authoritative and its callback outbox remains replayable;
// returning 403 here would incorrectly make the local runner discard the report.
func (h *Handler) recoverTerminalMutation(c *gin.Context, identity JobMutationIdentity, mutationErr error) bool {
	if !errors.Is(mutationErr, ErrJobAccessDenied) {
		return false
	}
	job, err := h.service.GetJob(c.Request.Context(), c.Param("jobId"))
	if err != nil || job == nil || (job.Status != JobCompleted && job.Status != JobFailed) {
		return false
	}
	if err := h.deliverTerminalCallbacks(c.Request.Context(), identity, job); err != nil {
		writeCallbackError(c, err)
		return true
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
	return true
}

func writeCallbackError(c *gin.Context, err error) {
	if errors.Is(err, ErrCallbackBusy) {
		writeError(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeError(c, http.StatusInternalServerError, err.Error())
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
	identity := h.runnerIdentityFromRequest(c)
	runnerID := identity.RunnerID
	headerRunnerID := c.GetHeader("X-Runner-ID")

	// If X-Runner-ID is present, it must match the URL param
	if headerRunnerID != "" && headerRunnerID != runnerID {
		return ErrRunnerAccessDenied
	}

	return h.service.ValidateRunnerAccess(c.Request.Context(), identity.UserID, identity.DeviceID, runnerID, identity.SessionID)
}

func (h *Handler) runnerIdentityFromRequest(c *gin.Context) JobMutationIdentity {
	userID, _ := auth.UserIDFromContext(c.Request.Context())
	deviceID, _ := auth.DeviceIDFromContext(c.Request.Context())
	sessionID := c.GetHeader("X-Runner-Session-ID")
	if sessionID == "" {
		sessionID = c.GetHeader("X-Runner-Session-Id")
	}
	return JobMutationIdentity{
		UserID: userID, DeviceID: deviceID, RunnerID: c.Param("runnerId"), SessionID: sessionID,
	}
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
