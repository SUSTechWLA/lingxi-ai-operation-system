package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
)

// RunService manages WorkflowRun lifecycle — creation, stage tracking, and status aggregation.
type RunService struct {
	repo          workflowTemplateStore
	runRepo       workflowRunStore
	orchService   workflowOrchestrator
	toolSnapshots ToolRegistrySnapshotProvider
	events        observability.EventEmitter
}

type workflowTemplateStore interface {
	FindByID(context.Context, string) (*Template, error)
}

type workflowRunStore interface {
	Create(context.Context, *WorkflowRun) error
	FindByID(context.Context, string) (*WorkflowRun, error)
	FindByProject(context.Context, string) ([]*WorkflowRun, error)
	UpdateStatus(context.Context, string, RunStatus) error
	UpdateStageStatus(context.Context, string, string, StageStatus) error
}

type workflowOrchestrator interface {
	CreateTask(context.Context, string, map[string]interface{}) (*model.Task, error)
	SubmitDAG(context.Context, string, *model.DAGRequest) error
}

type ToolRegistrySnapshot struct {
	ID            string
	SHA256        string
	CanonicalJSON json.RawMessage
}

type ToolRegistrySnapshotProvider interface {
	Snapshot(context.Context) (ToolRegistrySnapshot, error)
}

func (s *RunService) WithObservability(emitter observability.EventEmitter) *RunService {
	s.events = emitter
	return s
}

func (s *RunService) WithToolRegistrySnapshotProvider(provider ToolRegistrySnapshotProvider) *RunService {
	s.toolSnapshots = provider
	return s
}

func NewRunService(repo *Repository, runRepo *RunRepository, orchService *service.OrchestratorService) *RunService {
	return &RunService{repo: repo, runRepo: runRepo, orchService: orchService}
}

// CreateRun creates a new WorkflowRun, compiles stages into a DAG, and submits to the orchestrator.
func (s *RunService) CreateRun(ctx context.Context, userID, projectID, templateID, version string, input map[string]interface{}) (*WorkflowRun, error) {
	if userID == "" {
		return nil, fmt.Errorf("authenticated user is required")
	}
	ctx = observability.EnsureCorrelation(ctx)
	tmpl, err := s.repo.FindByID(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}

	// Parse DAG from template
	var dag model.DAGRequest
	dagData := tmpl.DAG

	if err := jsonUnmarshal(dagData, &dag); err != nil {
		return nil, fmt.Errorf("invalid DAG in template: %w", err)
	}

	snapshot, err := s.freezeToolRegistrySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	applyRunInputToDAG(&dag, input)
	applyToolRegistrySnapshotToDAG(&dag, snapshot)

	// Create orchestrator task
	task, err := s.orchService.CreateTask(ctx, userID, map[string]interface{}{
		"source":                 "video-workflow",
		"template_id":            templateID,
		"project_id":             projectID,
		"toolRegistrySnapshotId": snapshot.ID,
		"toolRegistrySha256":     snapshot.SHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	taskCorrelation := observability.CorrelationFromContext(ctx)
	taskCorrelation.ProjectID = projectID
	taskCorrelation.TaskID = task.ID
	runtime := observability.Runtime{
		WorkflowVersion:        version,
		ToolRegistrySnapshotID: snapshot.ID,
	}
	s.emitRunTransition(ctx, observability.EventTypeTaskCreated, observability.ExecutionStatusCompleted, observability.SeverityInfo, taskCorrelation, runtime, 1, nil)

	// Submit DAG
	if err := s.orchService.SubmitDAG(ctx, task.ID, &dag); err != nil {
		return nil, fmt.Errorf("failed to submit DAG: %w", err)
	}

	// Build stage statuses from DAG nodes
	stageMap := make(map[string]StageStatus)
	for _, node := range dag.Nodes {
		stageMap[node.ID] = StagePending
	}

	now := time.Now()
	run := &WorkflowRun{
		ID:                     "wfr-" + uuid.NewString()[:8],
		ProjectID:              projectID,
		UserID:                 userID,
		TemplateID:             templateID,
		TemplateVersion:        version,
		TaskID:                 task.ID,
		Status:                 RunPending,
		Attempt:                1,
		Input:                  input,
		StageStatuses:          stageMap,
		TraceID:                traceIDForRun(ctx, task.ID),
		ToolRegistrySnapshotID: snapshot.ID,
		CreatedAt:              now,
	}
	run.RunManifest = buildWorkflowRunManifest(run)
	run.RunManifest.ToolRegistrySHA256 = snapshot.SHA256

	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}
	eventCtx := ctx
	correlation := observability.CorrelationFromContext(eventCtx)
	correlation.WorkflowRunID = run.ID
	correlation.ProjectID = run.ProjectID
	correlation.TaskID = run.TaskID
	runtime = workflowRuntime(run)
	s.emitRunTransition(eventCtx, observability.EventTypeWorkflowRunQueued, observability.ExecutionStatusQueued, observability.SeverityInfo, correlation, runtime, int64(run.Attempt), nil)
	for stageName := range run.StageStatuses {
		stageCorrelation := correlation
		stageCorrelation.StageID = stageName
		s.emitRunTransition(eventCtx, observability.EventTypeWorkflowStageQueued, observability.ExecutionStatusQueued, observability.SeverityInfo, stageCorrelation, runtime, int64(run.Attempt), nil)
	}

	zap.L().Info("WorkflowRun created",
		zap.String("id", run.ID),
		zap.String("projectId", projectID),
		zap.String("taskId", task.ID),
	)
	return run, nil
}

func (s *RunService) freezeToolRegistrySnapshot(ctx context.Context) (ToolRegistrySnapshot, error) {
	if s == nil || s.toolSnapshots == nil {
		return ToolRegistrySnapshot{}, fmt.Errorf("tool registry snapshot provider is required")
	}
	snapshot, err := s.toolSnapshots.Snapshot(ctx)
	if err != nil {
		return ToolRegistrySnapshot{}, fmt.Errorf("freeze tool registry snapshot: %w", err)
	}
	hashBytes, err := hex.DecodeString(snapshot.SHA256)
	if err != nil || len(hashBytes) != sha256.Size {
		return ToolRegistrySnapshot{}, fmt.Errorf("freeze tool registry snapshot: invalid SHA-256")
	}
	digest := sha256.Sum256(snapshot.CanonicalJSON)
	if hex.EncodeToString(digest[:]) != snapshot.SHA256 || snapshot.ID != "tool_snapshot_"+snapshot.SHA256 {
		return ToolRegistrySnapshot{}, fmt.Errorf("freeze tool registry snapshot: identity does not match canonical registry")
	}
	snapshot.CanonicalJSON = append(json.RawMessage(nil), snapshot.CanonicalJSON...)
	return snapshot, nil
}

func applyToolRegistrySnapshotToDAG(dag *model.DAGRequest, snapshot ToolRegistrySnapshot) {
	if dag == nil {
		return
	}
	for i := range dag.Nodes {
		if dag.Nodes[i].Input == nil {
			dag.Nodes[i].Input = map[string]interface{}{}
		}
		dag.Nodes[i].Input["toolRegistrySnapshotId"] = snapshot.ID
		dag.Nodes[i].Input["toolRegistrySha256"] = snapshot.SHA256
		if parameters, ok := dag.Nodes[i].Input["parameters"].(map[string]interface{}); ok {
			parameters["toolRegistrySnapshotId"] = snapshot.ID
			parameters["toolRegistrySha256"] = snapshot.SHA256
		}
	}
}

func traceIDForRun(ctx context.Context, fallback string) string {
	if traceID := observability.CorrelationFromContext(ctx).TraceID; traceID != "" {
		return traceID
	}
	return fallback
}

func applyRunInputToDAG(dag *model.DAGRequest, input map[string]interface{}) {
	if dag == nil || len(input) == 0 {
		return
	}
	for i := range dag.Nodes {
		if dag.Nodes[i].Input == nil {
			dag.Nodes[i].Input = make(map[string]interface{})
		}
		for k, v := range input {
			dag.Nodes[i].Input[k] = v
		}

		parameters, ok := dag.Nodes[i].Input["parameters"].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range input {
			parameters[k] = v
		}
	}
}

// GetRun returns a WorkflowRun by ID.
func (s *RunService) GetRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	return s.runRepo.FindByID(ctx, runID)
}

// ListRunsByProject returns all runs for a project.
func (s *RunService) ListRunsByProject(ctx context.Context, projectID string) ([]*WorkflowRun, error) {
	return s.runRepo.FindByProject(ctx, projectID)
}

// UpdateRunStatus updates the status of a WorkflowRun.
func (s *RunService) UpdateRunStatus(ctx context.Context, runID string, status RunStatus) error {
	startedAt := time.Now()
	if err := s.runRepo.UpdateStatus(ctx, runID, status); err != nil {
		return err
	}
	eventType, executionStatus, severity := runStatusEvent(status)
	if eventType == "" {
		return nil
	}
	ctx = observability.EnsureCorrelation(ctx)
	correlation := observability.CorrelationFromContext(ctx)
	correlation.WorkflowRunID = runID
	durationMs := time.Since(startedAt).Milliseconds()
	run, _ := s.runRepo.FindByID(ctx, runID)
	enrichWorkflowCorrelation(&correlation, run)
	s.emitRunTransition(ctx, eventType, executionStatus, severity, correlation, workflowRuntime(run), 1, &durationMs)
	return nil
}

// UpdateStageStatus updates a single stage's status within a run.
func (s *RunService) UpdateStageStatus(ctx context.Context, runID, stageName string, status StageStatus) error {
	startedAt := time.Now()
	if err := s.runRepo.UpdateStageStatus(ctx, runID, stageName, status); err != nil {
		return err
	}
	eventType, executionStatus, severity := stageStatusEvent(status)
	if eventType == "" {
		return nil
	}
	ctx = observability.EnsureCorrelation(ctx)
	correlation := observability.CorrelationFromContext(ctx)
	correlation.WorkflowRunID = runID
	correlation.StageID = stageName
	durationMs := time.Since(startedAt).Milliseconds()
	run, _ := s.runRepo.FindByID(ctx, runID)
	enrichWorkflowCorrelation(&correlation, run)
	s.emitRunTransition(ctx, eventType, executionStatus, severity, correlation, workflowRuntime(run), 1, &durationMs)
	return nil
}

// PauseRun pauses the run.
func (s *RunService) PauseRun(ctx context.Context, runID, reason string) error {
	return s.runRepo.UpdateStatus(ctx, runID, RunPaused)
}

// CancelRun cancels the run.
func (s *RunService) CancelRun(ctx context.Context, runID string) error {
	return s.UpdateRunStatus(ctx, runID, RunCancelled)
}

func runStatusEvent(status RunStatus) (observability.EventType, observability.ExecutionStatus, observability.Severity) {
	switch status {
	case RunPending:
		return observability.EventTypeWorkflowRunQueued, observability.ExecutionStatusQueued, observability.SeverityInfo
	case RunRunning:
		return observability.EventTypeWorkflowRunStarted, observability.ExecutionStatusStarted, observability.SeverityInfo
	case RunCompleted:
		return observability.EventTypeWorkflowRunCompleted, observability.ExecutionStatusCompleted, observability.SeverityInfo
	case RunFailed:
		return observability.EventTypeWorkflowRunFailed, observability.ExecutionStatusFailed, observability.SeverityError
	case RunCancelled:
		return observability.EventTypeWorkflowRunCancelled, observability.ExecutionStatusCancelled, observability.SeverityWarn
	default:
		return "", "", ""
	}
}

func stageStatusEvent(status StageStatus) (observability.EventType, observability.ExecutionStatus, observability.Severity) {
	switch status {
	case StagePending:
		return observability.EventTypeWorkflowStageQueued, observability.ExecutionStatusQueued, observability.SeverityInfo
	case StageRunning:
		return observability.EventTypeWorkflowStageStarted, observability.ExecutionStatusStarted, observability.SeverityInfo
	case StageSucceeded:
		return observability.EventTypeWorkflowStageCompleted, observability.ExecutionStatusCompleted, observability.SeverityInfo
	case StageFailed:
		return observability.EventTypeWorkflowStageFailed, observability.ExecutionStatusFailed, observability.SeverityError
	case StageCancelled:
		return observability.EventTypeWorkflowStageCancelled, observability.ExecutionStatusCancelled, observability.SeverityWarn
	default:
		return "", "", ""
	}
}

func (s *RunService) emitRunTransition(ctx context.Context, eventType observability.EventType, status observability.ExecutionStatus, severity observability.Severity, correlation observability.Correlation, runtime observability.Runtime, attempt int64, durationMs *int64) {
	if s == nil {
		return
	}
	var eventErr *observability.EventError
	if severity == observability.SeverityError {
		code := "WORKFLOW.RUN.EXECUTION_FAILED"
		if strings.HasPrefix(string(eventType), "workflow.stage.") {
			code = "WORKFLOW.STAGE.EXECUTION_FAILED"
		}
		eventErr = observability.NormalizeError(code, nil, "workflow", "")
	}
	observability.EmitSafely(ctx, s.events, "workflow", observability.Event{
		EventType:   eventType,
		MessageKey:  string(eventType),
		Severity:    severity,
		Correlation: correlation,
		Execution: observability.Execution{
			Status: status, Attempt: attempt, DurationMs: durationMs,
		},
		Runtime: runtime,
		Error:   eventErr,
		Privacy: observability.Privacy{
			Classification: observability.PrivacyInternal,
			RedactedFields: []string{"workflow.input", "workflow.output", "workflow.stage.payload"},
		},
	})
}

func workflowRuntime(run *WorkflowRun) observability.Runtime {
	if run == nil {
		return observability.Runtime{}
	}
	return observability.Runtime{
		WorkflowVersion:        run.TemplateVersion,
		ToolRegistrySnapshotID: run.ToolRegistrySnapshotID,
	}
}

func enrichWorkflowCorrelation(correlation *observability.Correlation, run *WorkflowRun) {
	if correlation == nil || run == nil {
		return
	}
	if run.TraceID != "" {
		correlation.TraceID = run.TraceID
	}
	correlation.WorkflowRunID = run.ID
	correlation.ProjectID = run.ProjectID
	correlation.TaskID = run.TaskID
}

// jsonUnmarshal is a helper to unmarshal json.RawMessage.
func jsonUnmarshal(data interface{}, v interface{}) error {
	switch d := data.(type) {
	case json.RawMessage:
		return json.Unmarshal(d, v)
	case []byte:
		return json.Unmarshal(d, v)
	case string:
		return json.Unmarshal([]byte(d), v)
	default:
		return fmt.Errorf("unexpected DAG data type: %T", data)
	}
}
