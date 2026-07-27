package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
)

// RunService manages WorkflowRun lifecycle — creation, stage tracking, and status aggregation.
type RunService struct {
	repo        *Repository
	runRepo     *RunRepository
	orchService *service.OrchestratorService
	events      observability.EventEmitter
}

func (s *RunService) WithObservability(emitter observability.EventEmitter) *RunService {
	s.events = emitter
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

	applyRunInputToDAG(&dag, input)

	// Create orchestrator task
	task, err := s.orchService.CreateTask(ctx, userID, map[string]interface{}{
		"source":      "video-workflow",
		"template_id": templateID,
		"project_id":  projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	taskCorrelation := observability.CorrelationFromContext(ctx)
	taskCorrelation.ProjectID = projectID
	taskCorrelation.TaskID = task.ID
	s.emitRunTransition(ctx, observability.EventTypeTaskCreated, observability.ExecutionStatusCompleted, observability.SeverityInfo, taskCorrelation, 1, nil)

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
		ID:              "wfr-" + uuid.NewString()[:8],
		ProjectID:       projectID,
		UserID:          userID,
		TemplateID:      templateID,
		TemplateVersion: version,
		TaskID:          task.ID,
		Status:          RunPending,
		Attempt:         1,
		Input:           input,
		StageStatuses:   stageMap,
		TraceID:         traceIDForRun(ctx, task.ID),
		CreatedAt:       now,
	}
	run.RunManifest = buildWorkflowRunManifest(run)

	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}
	eventCtx := ctx
	correlation := observability.CorrelationFromContext(eventCtx)
	correlation.WorkflowRunID = run.ID
	correlation.ProjectID = run.ProjectID
	correlation.TaskID = run.TaskID
	s.emitRunTransition(eventCtx, observability.EventTypeWorkflowRunQueued, observability.ExecutionStatusQueued, observability.SeverityInfo, correlation, int64(run.Attempt), nil)
	for stageName := range run.StageStatuses {
		stageCorrelation := correlation
		stageCorrelation.StageID = stageName
		s.emitRunTransition(eventCtx, observability.EventTypeWorkflowStageQueued, observability.ExecutionStatusQueued, observability.SeverityInfo, stageCorrelation, int64(run.Attempt), nil)
	}

	zap.L().Info("WorkflowRun created",
		zap.String("id", run.ID),
		zap.String("projectId", projectID),
		zap.String("taskId", task.ID),
	)
	return run, nil
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
	s.emitRunTransition(ctx, eventType, executionStatus, severity, correlation, 1, &durationMs)
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
	s.emitRunTransition(ctx, eventType, executionStatus, severity, correlation, 1, &durationMs)
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

func (s *RunService) emitRunTransition(ctx context.Context, eventType observability.EventType, status observability.ExecutionStatus, severity observability.Severity, correlation observability.Correlation, attempt int64, durationMs *int64) {
	if s == nil {
		return
	}
	observability.EmitSafely(ctx, s.events, "workflow", observability.Event{
		EventType:   eventType,
		MessageKey:  string(eventType),
		Severity:    severity,
		Correlation: correlation,
		Execution: observability.Execution{
			Status: status, Attempt: attempt, DurationMs: durationMs,
		},
		Privacy: observability.Privacy{
			Classification: observability.PrivacyInternal,
			RedactedFields: []string{"workflow.input", "workflow.output", "workflow.stage.payload"},
		},
	})
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
