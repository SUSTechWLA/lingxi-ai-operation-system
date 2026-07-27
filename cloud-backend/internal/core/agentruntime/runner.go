package agentruntime

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type RunStatus string

const (
	RunStatusCreated   RunStatus = "CREATED"
	RunStatusRunning   RunStatus = "RUNNING"
	RunStatusSuccess   RunStatus = "SUCCESS"
	RunStatusFailed    RunStatus = "FAILED"
	RunStatusCancelled RunStatus = "CANCELLED"
)

var (
	errRunCancelled        = errors.New("agent run cancelled")
	ErrIdempotencyConflict = errors.New("agent run idempotency key conflicts with a different request")
)

type StartRunRequest struct {
	RunID                  string                 `json:"-"`
	UserID                 string                 `json:"userId,omitempty"`
	Message                string                 `json:"message"`
	Domain                 string                 `json:"domain,omitempty"`
	Context                map[string]interface{} `json:"context,omitempty"`
	Mode                   string                 `json:"mode,omitempty"`
	MaxCostLevel           string                 `json:"maxCostLevel,omitempty"`
	MaxRiskLevel           string                 `json:"maxRiskLevel,omitempty"`
	IdempotencyFingerprint string                 `json:"-"`
	DeviceID               string                 `json:"-"`
	TargetRunnerID         string                 `json:"-"`
	ParentRunID            *string                `json:"parentRunId,omitempty"`
	ReplayFromStageID      *string                `json:"replayFromStageId,omitempty"`
	requestToolSnapshot    *RequestToolSnapshot
}

type Run struct {
	ID                     string                 `json:"id"`
	TaskID                 string                 `json:"taskId,omitempty"`
	UserID                 string                 `json:"userId,omitempty"`
	Domain                 string                 `json:"domain,omitempty"`
	Message                string                 `json:"message"`
	Plan                   *AgentPlan             `json:"plan,omitempty"`
	Status                 RunStatus              `json:"status"`
	Budget                 AgentBudget            `json:"budget,omitempty"`
	TraceID                string                 `json:"traceId,omitempty"`
	ToolRegistrySnapshotID string                 `json:"toolRegistrySnapshotId,omitempty"`
	RunManifest            *RunManifest           `json:"runManifest,omitempty"`
	ParentRunID            *string                `json:"parentRunId,omitempty"`
	ReplayFromStageID      *string                `json:"replayFromStageId,omitempty"`
	CreatedAt              time.Time              `json:"createdAt"`
	UpdatedAt              time.Time              `json:"updatedAt"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	toolSnapshot           ToolSnapshot
}

type RunTerminalEvent struct {
	EventID string                 `json:"eventId"`
	RunID   string                 `json:"runId"`
	UserID  string                 `json:"userId,omitempty"`
	Status  RunStatus              `json:"status"`
	Context map[string]interface{} `json:"context,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

type RunTerminalCallback func(ctx context.Context, event RunTerminalEvent) error

type Planner interface {
	GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error)
}

// PlanRepairer is an optional interface that planners can implement
// to attempt plan repair after guard validation fails. The planner
// is responsible for obtaining the tool manifests it needs internally.
type PlanRepairer interface {
	RepairPlan(ctx context.Context, plan *AgentPlan, guardError string) (*AgentPlan, error)
}

// RequestScopedPlanRepairer guarantees that a repair turn sees the exact same
// immutable tool snapshot as the original planning/guard/compile sequence.
type RequestScopedPlanRepairer interface {
	RepairPlanForRequest(ctx context.Context, req StartRunRequest, plan *AgentPlan, guardError string) (*AgentPlan, error)
}

type Orchestrator interface {
	CreateTask(ctx context.Context, userID string, input map[string]interface{}) (*model.Task, error)
	SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
	GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
}

type RunStore interface {
	CreateRun(ctx context.Context, run *Run) (bool, error)
	SaveRun(ctx context.Context, run *Run) error
	SaveRunTerminal(ctx context.Context, run *Run, event RunTerminalEvent) error
	FindRun(ctx context.Context, id string) (*Run, error)
	ClaimTerminalEvents(ctx context.Context, limit int, leaseUntil time.Time, claimToken string) ([]TerminalEventDelivery, error)
	AckTerminalEvent(ctx context.Context, delivery TerminalEventDelivery) (bool, error)
	ReleaseTerminalEvent(ctx context.Context, delivery TerminalEventDelivery) (bool, error)
}

type runByTaskStore interface {
	FindRunByTaskID(ctx context.Context, taskID string) (*Run, error)
}

type TerminalEventDelivery struct {
	RunID      string
	EventID    string
	ClaimToken string
	Event      RunTerminalEvent
}

type PlanJudge interface {
	Evaluate(plan *AgentPlan) PlanJudgeReport
}

type PlanJudgeReport struct {
	Passed   bool               `json:"passed"`
	Warnings []PlanJudgeWarning `json:"warnings,omitempty"`
}

type PlanJudgeWarning struct {
	Code     string `json:"code"`
	StepID   string `json:"stepId,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type Runner struct {
	orchestrator Orchestrator
	store        RunStore
	planner      Planner
	guard        *PlanGuard
	compiler     *PlanCompiler
	planJudge    PlanJudge
	terminal     RunTerminalCallback
	toolResolver RequestToolSnapshotResolver
}

const asyncRunStartTimeout = 10 * time.Minute

func NewRunner(orchestrator Orchestrator, store RunStore, planner Planner, guard *PlanGuard, compiler *PlanCompiler) *Runner {
	return &Runner{
		orchestrator: orchestrator,
		store:        store,
		planner:      planner,
		guard:        guard,
		compiler:     compiler,
	}
}

func (r *Runner) WithPlanJudge(judge PlanJudge) *Runner {
	r.planJudge = judge
	return r
}

func (r *Runner) WithTerminalCallback(callback RunTerminalCallback) *Runner {
	r.terminal = callback
	return r
}

func (r *Runner) WithRequestToolResolver(resolver RequestToolSnapshotResolver) *Runner {
	r.toolResolver = resolver
	return r
}

func (r *Runner) Start(ctx context.Context, req StartRunRequest) (*Run, error) {
	if err := r.validateStartRequest(req); err != nil {
		return nil, err
	}
	run := newRunShell(req)
	if err := r.attachToolSnapshot(ctx, &req, run); err != nil {
		return nil, err
	}
	return r.completeStart(ctx, req, run)
}

func (r *Runner) StartAsync(ctx context.Context, req StartRunRequest) (*Run, error) {
	if err := r.validateStartRequest(req); err != nil {
		return nil, err
	}
	run := newRunShell(req)
	if err := r.attachToolSnapshot(ctx, &req, run); err != nil {
		return nil, err
	}
	created, err := r.store.CreateRun(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("store agent run: %w", err)
	}
	if !created {
		existing, findErr := r.store.FindRun(ctx, run.ID)
		if findErr != nil {
			return nil, fmt.Errorf("find existing agent run: %w", findErr)
		}
		if existing == nil {
			return nil, fmt.Errorf("agent run %s already exists but cannot be loaded", run.ID)
		}
		if req.IdempotencyFingerprint != "" && existingIdempotencyFingerprint(existing) != req.IdempotencyFingerprint {
			return nil, ErrIdempotencyConflict
		}
		if deliverErr := r.DeliverPendingTerminalEventsOnce(ctx, 1); deliverErr != nil {
			zap.L().Warn("existing agent run terminal reconciliation failed", zap.String("runId", run.ID), zap.Error(deliverErr))
		}
		return existing, nil
	}
	backgroundRun := *run
	go r.completeStartInBackground(req, &backgroundRun)
	return run, nil
}

func (r *Runner) validateStartRequest(req StartRunRequest) error {
	if req.Message == "" {
		return fmt.Errorf("message is required")
	}
	if r == nil || r.orchestrator == nil || r.store == nil || r.planner == nil || r.guard == nil || r.compiler == nil {
		return fmt.Errorf("agent runner is not configured")
	}
	return nil
}

func newRunShell(req StartRunRequest) *Run {
	domain := req.Domain
	if domain == "" {
		domain = inferDomain(req.Message)
	}
	mode := req.Mode
	if mode == "" {
		mode = "dynamic_agent"
	}
	now := time.Now()
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		runID = "agent_run_" + uuid.NewString()
	}
	metadata := map[string]interface{}{
		"mode": mode, "startPhase": "planning", "requestContext": sanitizedRunContext(req.Context),
	}
	if req.IdempotencyFingerprint != "" {
		metadata["idempotencyFingerprint"] = req.IdempotencyFingerprint
	}
	return &Run{
		ID:                runID,
		UserID:            req.UserID,
		Domain:            domain,
		Message:           req.Message,
		Status:            RunStatusCreated,
		CreatedAt:         now,
		UpdatedAt:         now,
		Metadata:          metadata,
		ParentRunID:       cloneStringPointer(req.ParentRunID),
		ReplayFromStageID: cloneStringPointer(req.ReplayFromStageID),
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (r *Runner) attachToolSnapshot(ctx context.Context, req *StartRunRequest, run *Run) error {
	if run == nil || req == nil || run.ToolRegistrySnapshotID != "" {
		return nil
	}
	if req.requestToolSnapshot == nil && r.toolResolver != nil {
		snapshot, err := r.toolResolver.Resolve(ctx, req.UserID, req.DeviceID, req.TargetRunnerID)
		if err != nil {
			return fmt.Errorf("resolve request tool snapshot: %w", err)
		}
		req.requestToolSnapshot = snapshot
	}

	var manifests []*tool.ToolManifest
	if req.requestToolSnapshot != nil {
		manifests = req.requestToolSnapshot.ListManifests()
	} else if r != nil && r.guard != nil {
		if provider, ok := r.guard.tools.(ToolListProvider); ok {
			manifests = provider.ListManifests()
		} else if r.guard.tools != nil {
			return fmt.Errorf("build tool registry snapshot: catalog does not support immutable listing")
		}
		req.requestToolSnapshot = newRequestToolSnapshot(manifests, nil)
		manifests = req.requestToolSnapshot.ListManifests()
	} else {
		req.requestToolSnapshot = newRequestToolSnapshot(nil, nil)
	}
	snapshot, err := BuildToolSnapshot(manifests)
	if err != nil {
		return fmt.Errorf("build tool registry snapshot: %w", err)
	}
	correlation := observability.CorrelationFromContext(ctx)
	run.TraceID = correlation.TraceID
	run.ToolRegistrySnapshotID = snapshot.ID
	run.toolSnapshot = snapshot
	if run.Metadata == nil {
		run.Metadata = map[string]interface{}{}
	}
	run.Metadata["toolRegistrySnapshotId"] = snapshot.ID
	run.RunManifest = &RunManifest{
		SchemaVersion:          runManifestSchemaVersion,
		Runtime:                "cloud-agent",
		RunID:                  run.ID,
		TraceID:                run.TraceID,
		ToolRegistrySnapshotID: snapshot.ID,
		ToolRegistrySHA256:     snapshot.SHA256,
		ParentRunID:            cloneStringPointer(run.ParentRunID),
		ReplayFromStageID:      cloneStringPointer(run.ReplayFromStageID),
		CreatedAt:              run.CreatedAt,
	}
	if req.requestToolSnapshot != nil {
		run.RunManifest.MCPRunnerRevisions = req.requestToolSnapshot.RunnerRevisions()
	}
	if err := validateAgentRunManifest(run.RunManifest); err != nil {
		return fmt.Errorf("validate run manifest: %w", err)
	}
	return nil
}

func existingIdempotencyFingerprint(run *Run) string {
	if run == nil || run.Metadata == nil {
		return ""
	}
	fingerprint, _ := run.Metadata["idempotencyFingerprint"].(string)
	return fingerprint
}

func (r *Runner) completeStartInBackground(req StartRunRequest, run *Run) {
	ctx, cancel := context.WithTimeout(context.Background(), asyncRunStartTimeout)
	defer cancel()
	if _, err := r.completeStart(ctx, req, run); err != nil {
		if errors.Is(err, errRunCancelled) {
			zap.L().Info("async agent run start aborted after cancellation",
				zap.String("runId", run.ID),
			)
			return
		}
		failed := *run
		failed.Status = RunStatusFailed
		failed.UpdatedAt = time.Now()
		if failed.Metadata == nil {
			failed.Metadata = map[string]interface{}{}
		}
		failed.Metadata["startPhase"] = "failed"
		failed.Metadata["error"] = err.Error()
		event := RunTerminalEvent{
			RunID: failed.ID, UserID: failed.UserID, Status: RunStatusFailed,
			Context: sanitizedRunContext(req.Context), Error: err.Error(),
		}
		if saveErr := r.persistAndDeliverTerminal(context.Background(), &failed, event); saveErr != nil {
			zap.L().Warn("failed to persist or deliver async agent run failure",
				zap.String("runId", run.ID),
				zap.Error(saveErr),
			)
		}
		zap.L().Warn("async agent run start failed",
			zap.String("runId", run.ID),
			zap.Error(err),
		)
	}
}

func (r *Runner) completeStart(ctx context.Context, req StartRunRequest, run *Run) (*Run, error) {
	if err := r.attachToolSnapshot(ctx, &req, run); err != nil {
		return nil, err
	}
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}
	guard := r.guard
	compiler := r.compiler
	if req.requestToolSnapshot != nil {
		guard = r.guard.withToolCatalog(req.requestToolSnapshot)
		compiler = r.compiler.withToolCatalog(req.requestToolSnapshot)
	}
	plan, err := r.planner.GeneratePlan(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate agent plan: %w", err)
	}
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}
	applyRequestPlanDefaults(plan, req)
	plan = compiler.PreparePlan(plan)
	applyShotRegenerationPlanScope(plan, req.Context)
	if err := guard.ValidatePlan(ctx, req.UserID, plan); err != nil {
		// Attempt plan repair with the same request-scoped tool snapshot. An old
		// unscoped repairer is never used for a dynamic snapshot run because it
		// could silently drop or replace runner-local tools.
		var repaired *AgentPlan
		var repairErr error
		canRepair := false
		if repairer, ok := r.planner.(RequestScopedPlanRepairer); ok {
			canRepair = true
			repaired, repairErr = repairer.RepairPlanForRequest(ctx, req, plan, err.Error())
		} else if req.requestToolSnapshot == nil {
			if repairer, ok := r.planner.(PlanRepairer); ok {
				canRepair = true
				repaired, repairErr = repairer.RepairPlan(ctx, plan, err.Error())
			}
		}
		if canRepair {
			zap.L().Warn("agent plan guard validation failed, attempting repair",
				zap.Error(err),
			)
			if repairErr == nil && repaired != nil {
				applyRequestPlanDefaults(repaired, req)
				repaired = compiler.PreparePlan(repaired)
				applyShotRegenerationPlanScope(repaired, req.Context)
				revalidateErr := guard.ValidatePlan(ctx, req.UserID, repaired)
				if revalidateErr == nil {
					plan = repaired
					zap.L().Info("agent plan repaired successfully")
					goto planOK
				}
				zap.L().Warn("agent plan repair did not pass revalidation", zap.Error(revalidateErr))
			} else if repairErr != nil {
				zap.L().Warn("agent plan repair failed", zap.Error(repairErr))
			}
		} else if req.requestToolSnapshot != nil {
			zap.L().Warn("agent plan repair skipped because planner has no request-scoped repair contract", zap.Error(err))
		}
		return nil, fmt.Errorf("guard agent plan: %w", err)
	}
planOK:
	agentToolTrace := buildAgentToolTrace(plan, GuardDecisionTrace{Passed: true})
	logAgentToolTrace(req, plan, agentToolTrace)
	judgeReport := PlanJudgeReport{Passed: true}
	if r.planJudge != nil {
		judgeReport = r.planJudge.Evaluate(plan)
	}
	if !judgeReport.Passed {
		return nil, fmt.Errorf("agent plan failed video beta validation: %s", summarizePlanJudgeWarnings(judgeReport.Warnings))
	}

	dag, err := compiler.Compile(plan)
	if err != nil {
		return nil, fmt.Errorf("compile agent plan: %w", err)
	}
	if providers := clientModelProvidersFromContext(req.Context); len(providers) > 0 {
		injectClientModelProviders(dag, providers)
	}
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}

	if run == nil {
		run = newRunShell(req)
	}
	now := time.Now()
	if run.ID == "" {
		run.ID = "agent_run_" + uuid.NewString()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UserID = req.UserID
	run.Domain = plan.Domain
	run.Message = req.Message
	run.Plan = plan
	run.Status = RunStatusCreated
	run.Budget = plan.Budget
	run.UpdatedAt = now
	run.Metadata = map[string]interface{}{
		"mode": plan.Mode, "agentToolTrace": agentToolTrace, "requestContext": sanitizedRunContext(req.Context),
		"toolRegistrySnapshotId": run.ToolRegistrySnapshotID,
	}
	if req.requestToolSnapshot != nil && len(req.requestToolSnapshot.runners) > 0 {
		run.Metadata["mcpCatalogSnapshot"] = req.requestToolSnapshot.RunnerRevisions()
	}
	if len(judgeReport.Warnings) > 0 {
		run.Metadata["planJudgeWarnings"] = judgeReport.Warnings
		run.Metadata["planJudgePassed"] = judgeReport.Passed
	}

	taskInput := map[string]interface{}{
		"source":         "agentruntime",
		"agentRunId":     run.ID,
		"message":        req.Message,
		"domain":         plan.Domain,
		"context":        sanitizedRunContext(req.Context),
		"plan":           plan,
		"agentToolTrace": agentToolTrace,
	}
	if len(judgeReport.Warnings) > 0 {
		taskInput["planJudgeWarnings"] = judgeReport.Warnings
		taskInput["planJudgePassed"] = judgeReport.Passed
	}
	task, err := r.orchestrator.CreateTask(ctx, req.UserID, taskInput)
	if err != nil {
		return nil, fmt.Errorf("create agent task: %w", err)
	}
	run.TaskID = task.ID
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store agent run task link: %w", err)
	}
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}

	scoped := scopeDAGToTask(task.ID, dag)
	if err := r.orchestrator.SubmitDAG(ctx, task.ID, scoped); err != nil {
		run.Status = RunStatusFailed
		run.UpdatedAt = time.Now()
		_ = r.store.SaveRun(ctx, run)
		return nil, fmt.Errorf("submit agent DAG: %w", err)
	}
	if err := r.abortIfCancelled(ctx, run); err != nil {
		return nil, err
	}

	run.Status = RunStatusRunning
	run.UpdatedAt = time.Now()
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store agent run: %w", err)
	}
	return run, nil
}

func (r *Runner) abortIfCancelled(ctx context.Context, run *Run) error {
	if r == nil || r.store == nil || run == nil || run.ID == "" {
		return nil
	}
	current, err := r.store.FindRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if current != nil && current.Status == RunStatusCancelled {
		return errRunCancelled
	}
	return nil
}

func applyRequestPlanDefaults(plan *AgentPlan, req StartRunRequest) {
	if plan == nil {
		return
	}
	if req.Domain != "" {
		plan.Domain = req.Domain
	} else if plan.Domain == "" {
		plan.Domain = req.Domain
	}
	if plan.Domain == "video_creation" && strings.TrimSpace(req.Message) != "" {
		plan.Goal = req.Message
	}
	if plan.Mode == "" {
		plan.Mode = "dynamic_agent"
	}
	applyRequestSafeContextDefaults(plan, req.Context)
	applyShotRegenerationPlanScope(plan, req.Context)
}

func applyShotRegenerationPlanScope(plan *AgentPlan, ctx map[string]interface{}) {
	if plan == nil || len(plan.Steps) == 0 || strings.TrimSpace(fmt.Sprint(ctx["operation"])) != "shot_regeneration" {
		return
	}
	targetShotID := strings.TrimSpace(fmt.Sprint(ctx["targetShotId"]))
	regenerationTaskID := strings.TrimSpace(fmt.Sprint(ctx["shotRegenerationTaskId"]))
	regenerationRunID := strings.TrimSpace(fmt.Sprint(ctx["shotRegenerationRunId"]))
	for i := range plan.Steps {
		if plan.Steps[i].Arguments == nil {
			plan.Steps[i].Arguments = map[string]interface{}{}
		}
		plan.Steps[i].Arguments["operation"] = "shot_regeneration"
		plan.Steps[i].Arguments["targetShotId"] = targetShotID
		plan.Steps[i].Arguments["allowedShotIds"] = []string{targetShotID}
		if regenerationTaskID != "" && regenerationTaskID != "<nil>" {
			plan.Steps[i].Arguments["shotRegenerationTaskId"] = regenerationTaskID
		}
		if regenerationRunID != "" && regenerationRunID != "<nil>" {
			plan.Steps[i].Arguments["shotRegenerationRunId"] = regenerationRunID
		}
	}
}

func applyRequestSafeContextDefaults(plan *AgentPlan, ctx map[string]interface{}) {
	if plan == nil || len(plan.Steps) == 0 || ctx == nil {
		return
	}
	if plan.Steps[0].Arguments == nil {
		plan.Steps[0].Arguments = map[string]interface{}{}
	}
	for _, key := range []string{
		"projectId",
		"videoProjectId",
		"topic",
		"durationSec",
		"targetDurationSec",
		"videoType",
		"profileId",
		"projectMode",
		"aigcProvider",
		"aigcEnabled",
		"aigcPolicy",
		"ipRenderMode",
		"productionRoute",
		"canonicalProfileId",
		"requiredLayers",
		"visualLayerContract",
		"designedLayers",
		"layerExecutionPolicy",
		"generationMode",
		"preflightPipeline",
		"aspectRatio",
		"language",
		"script",
		"characterProfilePath",
		"presentationMode",
		"cameraPreset",
		"actionSequence",
		"brollWindows",
		"renderTimeoutSec",
		"hyperframesRenderTimeoutSec",
	} {
		if _, exists := plan.Steps[0].Arguments[key]; exists {
			continue
		}
		if value, ok := ctx[key]; ok {
			plan.Steps[0].Arguments[key] = value
		}
	}
}

func clientModelProvidersFromContext(ctx map[string]interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	if provider, ok := normalizeClientModelProvider(ctx["modelProvider"]); ok {
		out["text_to_text"] = provider
	}
	providers, ok := ctx["modelProviders"]
	if !ok {
		return out
	}
	switch typed := providers.(type) {
	case map[string]interface{}:
		for _, capability := range []string{"text_to_text", "text_to_image", "text_to_video"} {
			if provider, ok := normalizeClientModelProvider(typed[capability]); ok {
				out[capability] = provider
			}
		}
	case map[string]map[string]interface{}:
		for _, capability := range []string{"text_to_text", "text_to_image", "text_to_video"} {
			if provider, ok := normalizeClientModelProvider(typed[capability]); ok {
				out[capability] = provider
			}
		}
	}
	return out
}

func normalizeClientModelProvider(value interface{}) (map[string]interface{}, bool) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	provider := map[string]interface{}{}
	for _, key := range []string{"baseUrl", "apiKey", "model"} {
		text := strings.TrimSpace(fmt.Sprint(raw[key]))
		if text != "" && text != "<nil>" {
			provider[key] = text
		}
	}
	if provider["apiKey"] == nil {
		return nil, false
	}
	return provider, true
}

func injectClientModelProviders(dag *model.DAGRequest, providers map[string]map[string]interface{}) {
	if dag == nil || len(providers) == 0 {
		return
	}
	for i := range dag.Nodes {
		input := dag.Nodes[i].Input
		if input == nil {
			continue
		}
		params, ok := input["parameters"].(map[string]interface{})
		if !ok {
			continue
		}
		if _, exists := params["modelProvider"]; exists {
			continue
		}
		capability := modelProviderCapabilityForTool(input, params)
		if capability == "" {
			continue
		}
		provider := providers[capability]
		if len(provider) == 0 && capability != "text_to_text" {
			provider = providers["text_to_text"]
		}
		if len(provider) == 0 {
			continue
		}
		params["modelProvider"] = copyMap(provider)
	}
}

func modelProviderCapabilityForTool(input, params map[string]interface{}) string {
	for _, value := range []interface{}{params["tool"], params["capabilityTool"], input["capabilityTool"], input["tool"]} {
		toolName := strings.TrimSpace(fmt.Sprint(value))
		if toolName == "" || toolName == "<nil>" {
			continue
		}
		if capability, ok := clientModelProviderToolCapabilities[toolName]; ok {
			return capability
		}
	}
	return ""
}

var clientModelProviderToolCapabilities = map[string]string{
	"image_asset_generator":         "text_to_image",
	"text_image_to_video_generator": "text_to_video",

	"skill_stage_agent":             "text_to_text",
	"proposal_generator":            "text_to_text",
	"visual_feasibility_analyzer":   "text_to_text",
	"render_strategy_planner":       "text_to_text",
	"card_plan_generator":           "text_to_text",
	"caption_splitter":              "text_to_text",
	"composition_quality_checker":   "text_to_text",
	"reference_asset_planner":       "text_to_text",
	"asset_decision_agent":          "text_to_text",
	"asset_policy_generator":        "text_to_text",
	"continuity_checker":            "text_to_text",
	"style_profile_builder":         "text_to_text",
	"preview_quality_checker":       "text_to_text",
	"knowledge_researcher":          "text_to_text",
	"fact_checker":                  "text_to_text",
	"video_script_generator":        "text_to_text",
	"shot_splitter":                 "text_to_text",
	"keyframe_prompt_generator":     "text_to_text",
	"video_prompt_generator":        "text_to_text",
	"script_quality_checker":        "text_to_text",
	"shot_quality_checker":          "text_to_text",
	"video_prompt_quality_checker":  "text_to_text",
	"publish_copy_generator":        "text_to_text",
	"package_quality_checker":       "text_to_text",
	"hyperframes_project_generator": "text_to_text",
}

func sanitizedRunContext(ctx map[string]interface{}) map[string]interface{} {
	if len(ctx) == 0 {
		return ctx
	}
	out := make(map[string]interface{}, len(ctx))
	for k, v := range ctx {
		if isSensitiveModelProviderContextKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveModelProviderContextKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "modelprovider" || normalized == "modelproviders"
}

func logAgentToolTrace(req StartRunRequest, plan *AgentPlan, trace map[string]interface{}) {
	planned, _ := trace["plannedTools"].([]string)
	candidates, _ := trace["candidateTools"].([]ToolCandidateTrace)
	knowledgeInfo, _ := trace["knowledgeContext"].(map[string]interface{})
	zap.L().Info("agent runtime tool trace",
		zap.String("userInput", req.Message),
		zap.String("domain", plan.Domain),
		zap.Int("candidateToolCount", len(candidates)),
		zap.Strings("plannedTools", planned),
		zap.Any("candidateTools", candidates),
		zap.Any("knowledgeContext", knowledgeInfo),
		zap.Bool("guardPassed", true),
	)
}

func buildAgentToolTrace(plan *AgentPlan, guard GuardDecisionTrace) map[string]interface{} {
	trace := map[string]interface{}{
		"plannedTools":     plannedTools(plan),
		"guardDecision":    map[string]interface{}{"passed": guard.Passed, "warnings": guard.Warnings},
		"knowledgeContext": plannedKnowledgeContextInfo(plan),
	}
	if plan != nil && plan.ToolTrace != nil {
		trace["candidateTools"] = plan.ToolTrace.CandidateTools
		plan.ToolTrace.PlannedTools = plannedTools(plan)
		plan.ToolTrace.GuardDecision = &guard
		info := plannedKnowledgeContextStruct(plan)
		plan.ToolTrace.KnowledgeContext = &info
	} else {
		trace["candidateTools"] = []ToolCandidateTrace{}
	}
	return trace
}

func plannedTools(plan *AgentPlan) []string {
	if plan == nil {
		return nil
	}
	out := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		out = append(out, step.Tool)
	}
	return out
}

func plannedKnowledgeContextInfo(plan *AgentPlan) map[string]interface{} {
	info := plannedKnowledgeContextStruct(plan)
	return map[string]interface{}{
		"itemCount":   info.ItemCount,
		"sourceCount": info.SourceCount,
		"generatedBy": info.GeneratedBy,
	}
}

func plannedKnowledgeContextStruct(plan *AgentPlan) KnowledgeContextInfo {
	info := KnowledgeContextInfo{}
	if plan == nil {
		return info
	}
	generated := map[string]bool{}
	for _, step := range plan.Steps {
		kc, ok := step.Arguments["knowledgeContext"].(map[string]interface{})
		if !ok {
			continue
		}
		info.ItemCount += len(interfaceSlice(kc["items"]))
		info.SourceCount += len(interfaceSlice(kc["sources"]))
		for _, item := range interfaceSlice(kc["generatedBy"]) {
			if name, ok := item.(string); ok && name != "" && !generated[name] {
				generated[name] = true
				info.GeneratedBy = append(info.GeneratedBy, name)
			}
		}
	}
	return info
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func (r *Runner) Get(ctx context.Context, id string) (*Run, map[string]interface{}, error) {
	if r == nil || r.store == nil {
		return nil, nil, fmt.Errorf("agent runner is not configured")
	}
	run, err := r.store.FindRun(ctx, id)
	if err != nil || run == nil {
		return run, nil, err
	}
	if run.TaskID == "" || r.orchestrator == nil {
		return run, nil, nil
	}
	task, err := r.orchestrator.GetTaskWithDetails(ctx, run.TaskID)
	if err != nil {
		return run, task, err
	}
	taskStatus := taskStatusString(task)
	if taskStatus == string(model.TaskSuccess) && run.Status != RunStatusSuccess && run.Status != RunStatusCancelled {
		run.Status = RunStatusSuccess
		run.UpdatedAt = time.Now()
		if saveErr := r.persistAndDeliverTerminal(ctx, run, terminalEventFromRun(run, "")); saveErr != nil {
			return run, task, saveErr
		}
	}
	if taskStatus == string(model.TaskFailed) && run.Status != RunStatusFailed && run.Status != RunStatusCancelled {
		run.Status = RunStatusFailed
		run.UpdatedAt = time.Now()
		if saveErr := r.persistAndDeliverTerminal(ctx, run, terminalEventFromRun(run, taskErrorString(task))); saveErr != nil {
			return run, task, saveErr
		}
	}
	return run, task, nil
}

func (r *Runner) findRunByTaskID(ctx context.Context, taskID string) (*Run, error) {
	if r == nil || r.store == nil || strings.TrimSpace(taskID) == "" {
		return nil, nil
	}
	store, ok := r.store.(runByTaskStore)
	if !ok {
		return nil, nil
	}
	return store.FindRunByTaskID(ctx, taskID)
}

func taskStatusString(task map[string]interface{}) string {
	if len(task) == 0 {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(fmt.Sprint(task["status"])))
}

func (r *Runner) Cancel(ctx context.Context, id string) (*Run, error) {
	if r == nil || r.store == nil {
		return nil, fmt.Errorf("agent runner is not configured")
	}
	run, err := r.store.FindRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, nil
	}
	run.Status = RunStatusCancelled
	run.UpdatedAt = time.Now()
	if run.Metadata == nil {
		run.Metadata = map[string]interface{}{}
	}
	run.Metadata["cancelledBy"] = "user"
	if err := r.persistAndDeliverTerminal(ctx, run, terminalEventFromRun(run, "agent run cancelled by user")); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *Runner) persistAndDeliverTerminal(ctx context.Context, run *Run, event RunTerminalEvent) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("agent runner is not configured")
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = "agent_terminal_" + uuid.NewString()
	}
	if err := r.store.SaveRunTerminal(ctx, run, event); err != nil {
		return fmt.Errorf("persist agent terminal event: %w", err)
	}
	return r.DeliverPendingTerminalEventsOnce(ctx, 1)
}

func (r *Runner) DeliverPendingTerminalEventsOnce(ctx context.Context, limit int) error {
	if r == nil || r.store == nil || r.terminal == nil || limit <= 0 {
		return nil
	}
	claimToken := "agent_terminal_claim_" + uuid.NewString()
	deliveries, err := r.store.ClaimTerminalEvents(ctx, limit, time.Now().Add(30*time.Second), claimToken)
	if err != nil {
		return fmt.Errorf("claim agent terminal events: %w", err)
	}
	var deliveryErrors []error
	for _, delivery := range deliveries {
		if callbackErr := r.terminal(ctx, delivery.Event); callbackErr != nil {
			_, _ = r.store.ReleaseTerminalEvent(ctx, delivery)
			deliveryErrors = append(deliveryErrors, fmt.Errorf("deliver terminal event for %s: %w", delivery.RunID, callbackErr))
			continue
		}
		if _, ackErr := r.store.AckTerminalEvent(ctx, delivery); ackErr != nil {
			deliveryErrors = append(deliveryErrors, fmt.Errorf("ack terminal event for %s: %w", delivery.RunID, ackErr))
		}
	}
	return errors.Join(deliveryErrors...)
}

func (r *Runner) RunTerminalDelivery(ctx context.Context, interval time.Duration, batchSize int) {
	if interval <= 0 || batchSize <= 0 {
		return
	}
	_ = r.DeliverPendingTerminalEventsOnce(ctx, batchSize)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.DeliverPendingTerminalEventsOnce(ctx, batchSize); err != nil {
				zap.L().Warn("agent terminal event retry failed", zap.Error(err))
			}
		}
	}
}

func terminalEventFromRun(run *Run, errorMessage string) RunTerminalEvent {
	event := RunTerminalEvent{Error: errorMessage}
	if run == nil {
		return event
	}
	event.RunID, event.UserID, event.Status = run.ID, run.UserID, run.Status
	if run.Metadata != nil {
		event.Context, _ = run.Metadata["requestContext"].(map[string]interface{})
	}
	return event
}

func taskErrorString(task map[string]interface{}) string {
	for _, key := range []string{"error", "errorMessage", "message"} {
		if value := strings.TrimSpace(fmt.Sprint(task[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return "agent task failed"
}

func scopeDAGToTask(taskID string, dag *model.DAGRequest) *model.DAGRequest {
	if dag == nil {
		return nil
	}
	idMap := make(map[string]string, len(dag.Nodes))
	scoped := &model.DAGRequest{
		Nodes: make([]model.NodeRequest, len(dag.Nodes)),
		Edges: make([]model.Edge, len(dag.Edges)),
	}
	for i, node := range dag.Nodes {
		oldID := node.ID
		node.ID = scopedNodeID(taskID, oldID)
		idMap[oldID] = node.ID
		if node.Input != nil {
			node.Input = copyMap(node.Input)
			node.Input["agentOriginalNodeId"] = oldID
		}
		scoped.Nodes[i] = node
	}
	for i, edge := range dag.Edges {
		if mapped, ok := idMap[edge.From]; ok {
			edge.From = mapped
		}
		if mapped, ok := idMap[edge.To]; ok {
			edge.To = mapped
		}
		scoped.Edges[i] = edge
	}
	for i := range scoped.Nodes {
		if scoped.Nodes[i].Input != nil {
			if rewritten, ok := rewriteNodeReferences(scoped.Nodes[i].Input, idMap).(map[string]interface{}); ok {
				scoped.Nodes[i].Input = rewritten
			}
		}
	}
	return scoped
}

func scopedNodeID(taskID, nodeID string) string {
	const maxNodeIDLength = 64
	prefix := "t" + shortHash(taskID, 10) + "-"
	candidate := prefix + nodeID
	if len(candidate) <= maxNodeIDLength {
		return candidate
	}
	nodeHash := shortHash(nodeID, 8)
	maxBase := maxNodeIDLength - len(prefix) - len(nodeHash) - 1
	if maxBase < 1 {
		return prefix + nodeHash
	}
	return prefix + nodeID[:maxBase] + "-" + nodeHash
}

func shortHash(value string, length int) string {
	sum := sha1.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if length > len(encoded) {
		length = len(encoded)
	}
	return encoded[:length]
}

func rewriteNodeReferences(value interface{}, idMap map[string]string) interface{} {
	switch v := value.(type) {
	case string:
		out := v
		for oldID, newID := range idMap {
			out = strings.ReplaceAll(out, "{{"+oldID+".output.", "{{"+newID+".output.")
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			if s, ok := item.(string); ok && isNodeIDReferenceField(key) {
				if mapped, found := idMap[s]; found {
					out[key] = mapped
					continue
				}
			}
			out[key] = rewriteNodeReferences(item, idMap)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = rewriteNodeReferences(item, idMap)
		}
		return out
	default:
		return value
	}
}

func isNodeIDReferenceField(key string) bool {
	switch key {
	case "sourceNode", "productionSourceNode", "qualityCheckerNode":
		return true
	default:
		return false
	}
}

func summarizePlanJudgeWarnings(warnings []PlanJudgeWarning) string {
	if len(warnings) == 0 {
		return "plan judge did not pass"
	}
	parts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning.Message != "" {
			parts = append(parts, warning.Message)
			continue
		}
		if warning.Code != "" {
			parts = append(parts, warning.Code)
		}
	}
	if len(parts) == 0 {
		return "plan judge did not pass"
	}
	return strings.Join(parts, "; ")
}
