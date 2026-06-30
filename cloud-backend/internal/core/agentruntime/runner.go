package agentruntime

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

type RunStatus string

const (
	RunStatusCreated RunStatus = "CREATED"
	RunStatusRunning RunStatus = "RUNNING"
	RunStatusFailed  RunStatus = "FAILED"
)

type StartRunRequest struct {
	UserID       string                 `json:"userId,omitempty"`
	Message      string                 `json:"message"`
	Domain       string                 `json:"domain,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Mode         string                 `json:"mode,omitempty"`
	MaxCostLevel string                 `json:"maxCostLevel,omitempty"`
	MaxRiskLevel string                 `json:"maxRiskLevel,omitempty"`
}

type Run struct {
	ID        string                 `json:"id"`
	TaskID    string                 `json:"taskId,omitempty"`
	UserID    string                 `json:"userId,omitempty"`
	Domain    string                 `json:"domain,omitempty"`
	Message   string                 `json:"message"`
	Plan      *AgentPlan             `json:"plan,omitempty"`
	Status    RunStatus              `json:"status"`
	Budget    AgentBudget            `json:"budget,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type Planner interface {
	GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error)
}

// PlanRepairer is an optional interface that planners can implement
// to attempt plan repair after guard validation fails. The planner
// is responsible for obtaining the tool manifests it needs internally.
type PlanRepairer interface {
	RepairPlan(ctx context.Context, plan *AgentPlan, guardError string) (*AgentPlan, error)
}

type Orchestrator interface {
	CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error)
	SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
	GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
}

type RunStore interface {
	SaveRun(ctx context.Context, run *Run) error
	FindRun(ctx context.Context, id string) (*Run, error)
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

func (r *Runner) Start(ctx context.Context, req StartRunRequest) (*Run, error) {
	if err := r.validateStartRequest(req); err != nil {
		return nil, err
	}
	return r.completeStart(ctx, req, newRunShell(req))
}

func (r *Runner) StartAsync(ctx context.Context, req StartRunRequest) (*Run, error) {
	if err := r.validateStartRequest(req); err != nil {
		return nil, err
	}
	run := newRunShell(req)
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store agent run: %w", err)
	}
	go r.completeStartInBackground(req, run)
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
	return &Run{
		ID:        "agent_run_" + uuid.NewString(),
		UserID:    req.UserID,
		Domain:    domain,
		Message:   req.Message,
		Status:    RunStatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]interface{}{"mode": mode, "startPhase": "planning"},
	}
}

func (r *Runner) completeStartInBackground(req StartRunRequest, run *Run) {
	ctx, cancel := context.WithTimeout(context.Background(), asyncRunStartTimeout)
	defer cancel()
	if _, err := r.completeStart(ctx, req, run); err != nil {
		failed := *run
		failed.Status = RunStatusFailed
		failed.UpdatedAt = time.Now()
		if failed.Metadata == nil {
			failed.Metadata = map[string]interface{}{}
		}
		failed.Metadata["startPhase"] = "failed"
		failed.Metadata["error"] = err.Error()
		if saveErr := r.store.SaveRun(context.Background(), &failed); saveErr != nil {
			zap.L().Warn("failed to persist async agent run failure",
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
	plan, err := r.planner.GeneratePlan(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate agent plan: %w", err)
	}
	applyRequestPlanDefaults(plan, req)
	plan = r.compiler.PreparePlan(plan)
	if err := r.guard.ValidatePlan(ctx, req.UserID, plan); err != nil {
		// Attempt plan repair if the planner supports it.
		if repairer, ok := r.planner.(PlanRepairer); ok {
			zap.L().Warn("agent plan guard validation failed, attempting repair",
				zap.Error(err),
			)
			repaired, repairErr := repairer.RepairPlan(ctx, plan, err.Error())
			if repairErr == nil && repaired != nil {
				applyRequestPlanDefaults(repaired, req)
				repaired = r.compiler.PreparePlan(repaired)
				revalidateErr := r.guard.ValidatePlan(ctx, req.UserID, repaired)
				if revalidateErr == nil {
					plan = repaired
					zap.L().Info("agent plan repaired successfully")
					goto planOK
				}
				zap.L().Warn("agent plan repair did not pass revalidation", zap.Error(revalidateErr))
			} else if repairErr != nil {
				zap.L().Warn("agent plan repair failed", zap.Error(repairErr))
			}
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

	dag, err := r.compiler.Compile(plan)
	if err != nil {
		return nil, fmt.Errorf("compile agent plan: %w", err)
	}
	if providers := clientModelProvidersFromContext(req.Context); len(providers) > 0 {
		injectClientModelProviders(dag, providers)
	}

	run := &Run{
		ID:        "agent_run_" + uuid.NewString(),
		UserID:    req.UserID,
		Domain:    plan.Domain,
		Message:   req.Message,
		Plan:      plan,
		Status:    RunStatusCreated,
		Budget:    plan.Budget,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  map[string]interface{}{"mode": req.Mode, "agentToolTrace": agentToolTrace},
	}
	if len(judgeReport.Warnings) > 0 {
		run.Metadata["planJudgeWarnings"] = judgeReport.Warnings
		run.Metadata["planJudgePassed"] = judgeReport.Passed
	}

	taskInput := map[string]interface{}{
		"source":         "agentruntime",
		"agentRunId":     run.ID,
		"userId":         req.UserID,
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
	task, err := r.orchestrator.CreateTask(ctx, taskInput)
	if err != nil {
		return nil, fmt.Errorf("create agent task: %w", err)
	}
	run.TaskID = task.ID

	scoped := scopeDAGToTask(task.ID, dag)
	if err := r.orchestrator.SubmitDAG(ctx, task.ID, scoped); err != nil {
		run.Status = RunStatusFailed
		run.UpdatedAt = time.Now()
		_ = r.store.SaveRun(ctx, run)
		return nil, fmt.Errorf("submit agent DAG: %w", err)
	}

	run.Status = RunStatusRunning
	run.UpdatedAt = time.Now()
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store agent run: %w", err)
	}
	return run, nil
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
	if plan.Mode == "" {
		plan.Mode = "dynamic_agent"
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
		capability := modelProviderCapabilityForTool(params)
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

func modelProviderCapabilityForTool(params map[string]interface{}) string {
	toolName := strings.TrimSpace(fmt.Sprint(params["tool"]))
	if toolName == "" || toolName == "<nil>" {
		return ""
	}
	if capability, ok := clientModelProviderToolCapabilities[toolName]; ok {
		return capability
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
	return run, task, err
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
