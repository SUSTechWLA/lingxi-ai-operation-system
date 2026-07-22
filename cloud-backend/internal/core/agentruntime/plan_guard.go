package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// referencePattern matches {{stepID.output.field}} expressions.
var referencePattern = regexp.MustCompile(`^\{\{([^.]+)\.output\.([^}]+)\}\}$`)

type PlanGuard struct {
	tools          ToolCatalog
	localValidator *LocalCapabilityValidator
	directors      DirectorRegistry
}

// NewPlanGuard creates a PlanGuard with the given tool catalog.
// If localProvider is nil, local capability checks are skipped (graceful degradation).
func NewPlanGuard(tools ToolCatalog, localProvider LocalCapabilityProvider) *PlanGuard {
	return &PlanGuard{
		tools:          tools,
		localValidator: NewLocalCapabilityValidator(localProvider),
	}
}

func (g *PlanGuard) WithDirectors(directors DirectorRegistry) *PlanGuard {
	g.directors = directors
	return g
}

func (g *PlanGuard) Validate(plan *AgentPlan) error {
	return g.ValidatePlan(context.Background(), "", plan)
}

// ValidatePlan performs full plan validation. Runtime availability for video
// local tools is deferred until the execution stage so early cloud/review work
// can start before a user's desktop runner is online.
func (g *PlanGuard) ValidatePlan(ctx context.Context, userID string, plan *AgentPlan) error {
	if plan == nil {
		return fmt.Errorf("agent plan is required")
	}
	if len(plan.Steps) == 0 {
		return fmt.Errorf("agent plan has no steps")
	}
	if plan.Budget.MaxSteps > 0 && len(plan.Steps) > plan.Budget.MaxSteps {
		return fmt.Errorf("agent plan has %d steps, exceeds maxSteps %d", len(plan.Steps), plan.Budget.MaxSteps)
	}
	// Collect step info for reference validation.
	stepMap := make(map[string]AgentStep, len(plan.Steps))
	stepManifests := make(map[string]*tool.ToolManifest, len(plan.Steps))
	for _, step := range plan.Steps {
		stepMap[step.ID] = step
		stepManifests[step.ID] = g.manifestFor(step.Tool)
	}
	if err := g.validateKnowledgePolicy(plan, stepManifests); err != nil {
		return err
	}

	seen := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" {
			return fmt.Errorf("agent step id is required")
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate agent step id %s", step.ID)
		}
		seen[step.ID] = true
		if step.Tool == "" {
			return fmt.Errorf("agent step %s has no tool", step.ID)
		}

		manifest := g.manifestFor(step.Tool)
		if manifest == nil {
			return fmt.Errorf("agent step %s references unknown tool %s", step.ID, step.Tool)
		}
		if step.Tool == "__quality_gate__" {
			if err := g.validateQualityGate(step, stepMap); err != nil {
				return err
			}
		}

		// Local capability check: for video plans, local runner availability is
		// an execution-time dependency because local preview/render steps occur
		// after cloud-generated artifacts and human review gates.
		if userID != "" && g.localValidator != nil && !deferLocalCapabilityChecks(plan) {
			if err := g.localValidator.ValidateForLocalExecution(ctx, userID, step, manifest); err != nil {
				return err
			}
		}

		if err := validateCost(step, manifest, plan.Budget.MaxCostLevel); err != nil {
			return err
		}
		if err := validateRiskLevel(step, manifest); err != nil {
			return err
		}
		if err := validateRequiredParameters(step, manifest); err != nil {
			return err
		}
		if err := validateParameterTypes(step, manifest); err != nil {
			return err
		}
		if err := validateToolPolicy(step, manifest, plan); err != nil {
			return err
		}
		if manifest.SideEffect && !manifest.ApprovalPolicy.Required {
			return fmt.Errorf("agent step %s uses side-effect tool %s without approval policy", step.ID, step.Tool)
		}
		for _, dep := range step.DependsOn {
			if dep == step.ID {
				return fmt.Errorf("agent step %s cannot depend on itself", step.ID)
			}
			if !seen[dep] {
				return fmt.Errorf("agent step %s depends on unknown or later step %s", step.ID, dep)
			}
		}
		// Validate reference expressions in arguments (includes output field validation).
		if err := validateReferenceExpressions(step, stepMap, stepManifests); err != nil {
			return err
		}
		if err := validateShotRegenerationScope(step, stepMap); err != nil {
			return err
		}
	}

	if err := g.validateStageGuard(plan, stepMap, stepManifests); err != nil {
		return err
	}

	// MaxToolCalls check.
	if plan.Budget.MaxToolCalls > 0 {
		toolCallCount := countToolCalls(plan.Steps)
		if toolCallCount > plan.Budget.MaxToolCalls {
			return fmt.Errorf("agent plan has %d tool calls, exceeds maxToolCalls %d", toolCallCount, plan.Budget.MaxToolCalls)
		}
	}

	return nil
}

func (g *PlanGuard) validateQualityGate(gate AgentStep, steps map[string]AgentStep) error {
	productionID, _ := gate.Arguments["productionStep"].(string)
	checkerID, _ := gate.Arguments["checkerStep"].(string)
	productionTool, _ := gate.Arguments["productionTool"].(string)
	if productionID == "" || checkerID == "" || productionTool == "" {
		return fmt.Errorf("quality gate %s is missing productionStep, productionTool, or checkerStep", gate.ID)
	}
	production, exists := steps[productionID]
	if !exists {
		return fmt.Errorf("quality gate %s references unknown production step %s", gate.ID, productionID)
	}
	if production.Tool != productionTool {
		return fmt.Errorf("quality gate %s productionTool %s does not match production step tool %s", gate.ID, productionTool, production.Tool)
	}
	productionManifest := g.manifestFor(production.Tool)
	if productionManifest == nil || !productionManifest.QualityPolicy.Required {
		return fmt.Errorf("quality gate %s production step %s has no required quality policy", gate.ID, productionID)
	}
	checkerTool, hasChecker := qualityCheckerFor(production.Tool, productionManifest)
	if !hasChecker {
		return fmt.Errorf("quality gate %s production step %s has no configured checker", gate.ID, productionID)
	}
	checker, exists := steps[checkerID]
	if !exists {
		return fmt.Errorf("quality gate %s references unknown checker step %s", gate.ID, checkerID)
	}
	if checker.Tool != checkerTool {
		return fmt.Errorf("quality gate %s checker tool %s does not match required tool %s", gate.ID, checker.Tool, checkerTool)
	}
	if !containsString(checker.DependsOn, productionID) {
		return fmt.Errorf("quality gate %s checker %s does not depend on production step %s", gate.ID, checkerID, productionID)
	}
	if len(gate.DependsOn) != 1 || gate.DependsOn[0] != checkerID {
		return fmt.Errorf("quality gate %s must depend only on checker step %s", gate.ID, checkerID)
	}
	expectedMinScore := productionManifest.QualityPolicy.MinScore
	if expectedMinScore <= 0 {
		expectedMinScore = 85
	}
	minScore, hasMinScore := gate.Arguments["minScore"]
	if !hasMinScore || intArgument(minScore) != expectedMinScore {
		return fmt.Errorf("quality gate %s minScore does not match manifest policy", gate.ID)
	}
	if autoRepair, ok := gate.Arguments["autoRepair"].(bool); !ok || autoRepair != productionManifest.QualityPolicy.AutoRepair {
		return fmt.Errorf("quality gate %s autoRepair does not match manifest policy", gate.ID)
	}
	maxRepairAttempts, hasMaxRepairAttempts := gate.Arguments["maxRepairAttempts"]
	if !hasMaxRepairAttempts || intArgument(maxRepairAttempts) != productionManifest.QualityPolicy.MaxRepairAttempts {
		return fmt.Errorf("quality gate %s maxRepairAttempts does not match manifest policy", gate.ID)
	}
	if autoApprove, _ := gate.Arguments["autoApproveWhenPassed"].(bool); !autoApprove {
		return fmt.Errorf("quality gate %s must auto-approve only after the checker passes", gate.ID)
	}
	checkerManifest := g.manifestFor(checker.Tool)
	if fmt.Sprint(gate.Arguments["qualityCheckerNode"]) != compiledToolOutputNodeID(checkerID, checkerManifest) {
		return fmt.Errorf("quality gate %s qualityCheckerNode does not match checker step %s", gate.ID, checkerID)
	}
	if fmt.Sprint(gate.Arguments["productionSourceNode"]) != compiledToolSourceNodeID(productionID, productionManifest) {
		return fmt.Errorf("quality gate %s productionSourceNode does not match production step %s", gate.ID, productionID)
	}
	return nil
}

func intArgument(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func validateShotRegenerationScope(step AgentStep, stepMap map[string]AgentStep) error {
	if strings.TrimSpace(fmt.Sprint(step.Arguments["operation"])) != "shot_regeneration" {
		return nil
	}
	targetShotID := strings.TrimSpace(fmt.Sprint(step.Arguments["targetShotId"]))
	if targetShotID == "" || targetShotID == "<nil>" {
		return fmt.Errorf("shot regeneration plan target shot is required")
	}
	allowed := shotIDList(step.Arguments["allowedShotIds"])
	if len(allowed) != 1 || allowed[0] != targetShotID {
		return shotRegenerationEscapeError(targetShotID)
	}
	for _, field := range []string{"shotId", "targetShotId"} {
		if err := validateScopedShotValue(step.Arguments[field], targetShotID, stepMap); err != nil {
			return err
		}
	}
	for _, value := range shotValues(step.Arguments["shotIds"]) {
		if err := validateScopedShotValue(value, targetShotID, stepMap); err != nil {
			return err
		}
	}
	return nil
}

func validateScopedShotValue(value interface{}, targetShotID string, stepMap map[string]AgentStep) error {
	shotID := strings.TrimSpace(fmt.Sprint(value))
	if shotID == "" || shotID == "<nil>" {
		return nil
	}
	if matches := referencePattern.FindStringSubmatch(shotID); len(matches) == 3 {
		producer, ok := stepMap[matches[1]]
		if !ok || !stepTargetsOnlyShot(producer, targetShotID) {
			return shotRegenerationEscapeError(targetShotID)
		}
		return nil
	}
	if shotID != targetShotID {
		return shotRegenerationEscapeError(targetShotID)
	}
	return nil
}

func stepTargetsOnlyShot(step AgentStep, targetShotID string) bool {
	if strings.TrimSpace(fmt.Sprint(step.Arguments["operation"])) != "shot_regeneration" {
		return false
	}
	if strings.TrimSpace(fmt.Sprint(step.Arguments["targetShotId"])) != targetShotID {
		return false
	}
	allowed := shotIDList(step.Arguments["allowedShotIds"])
	return len(allowed) == 1 && allowed[0] == targetShotID
}

func shotRegenerationEscapeError(targetShotID string) error {
	return fmt.Errorf("shot regeneration plan escapes target shot %s", targetShotID)
}

func shotIDList(value interface{}) []string {
	values := shotValues(value)
	out := make([]string, 0, len(values))
	for _, item := range values {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" && text != "<nil>" {
			out = append(out, text)
		}
	}
	return out
}

func shotValues(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []string:
		out := make([]interface{}, len(typed))
		for i := range typed {
			out[i] = typed[i]
		}
		return out
	case []interface{}:
		return typed
	case string:
		if strings.TrimSpace(typed) != "" {
			return []interface{}{typed}
		}
	}
	return nil
}

func deferLocalCapabilityChecks(plan *AgentPlan) bool {
	return plan != nil && plan.Domain == "video_creation"
}

func (g *PlanGuard) validateStageGuard(
	plan *AgentPlan,
	stepMap map[string]AgentStep,
	stepManifests map[string]*tool.ToolManifest,
) error {
	if g == nil || g.directors == nil || plan == nil || plan.Domain != "video_creation" {
		return nil
	}

	stageByStep := make(map[string]string, len(plan.Steps))
	for _, step := range plan.Steps {
		stageByStep[step.ID] = resolveStepStage(step)
	}

	callCount := map[string]int{}
	hasReview := map[string]bool{}
	outputsByStage := map[string]map[string]bool{}
	stepsByStage := map[string][]AgentStep{}

	for _, step := range plan.Steps {
		stage := stageByStep[step.ID]
		if stage == "" {
			continue
		}
		director := g.directors.Get(stage)
		if director == nil {
			continue
		}

		forbidden := stringSet(director.ForbiddenTools())
		if forbidden[step.Tool] {
			return fmt.Errorf("stage guard: role %s stage %s forbidden tool %s", roleLabel(director), stage, step.Tool)
		}
		allowed := stringSet(director.AllowedTools())
		if len(allowed) > 0 && !allowed[step.Tool] && !isQualityCheckerTool(step.Tool) {
			return fmt.Errorf("stage guard: role %s stage %s does not allow tool %s", roleLabel(director), stage, step.Tool)
		}

		callCount[stage]++
		if maxCalls := director.MaxToolCalls(); maxCalls > 0 && callCount[stage] > maxCalls {
			return fmt.Errorf("stage guard: role %s stage %s exceeds max tool calls %d", roleLabel(director), stage, maxCalls)
		}

		stepsByStage[stage] = append(stepsByStage[stage], step)
		if _, ok := outputsByStage[stage]; !ok {
			outputsByStage[stage] = map[string]bool{}
		}
		manifest := stepManifests[step.ID]
		for _, out := range step.ExpectedOutput {
			outputsByStage[stage][out] = true
		}
		if manifest != nil {
			for out := range manifest.Output {
				outputsByStage[stage][out] = true
			}
			for _, kind := range manifest.ArtifactPolicy.ArtifactKinds {
				outputsByStage[stage][kind] = true
			}
			if manifest.ApprovalPolicy.Required || (manifest.HumanReview != nil && manifest.HumanReview.Required) {
				hasReview[stage] = true
			}
		}

		if isRenderStage(stage, director) && !hasApprovedPreviewDependency(step, stepMap, stageByStep) {
			return fmt.Errorf("stage guard: role %s stage %s requires an approved preview dependency before render", roleLabel(director), stage)
		}
	}

	for stage, steps := range stepsByStage {
		if len(steps) == 0 {
			continue
		}
		director := g.directors.Get(stage)
		if director == nil {
			continue
		}
		if director.RequiresApproval() && !hasReview[stage] {
			return fmt.Errorf("stage guard: role %s stage %s requires human review but no reviewable tool was planned", roleLabel(director), stage)
		}
		role, ok := director.(RoleAgentDirector)
		if !ok {
			continue
		}
		for _, required := range role.RequiredOutputs() {
			if !outputsByStage[stage][required] {
				return fmt.Errorf("stage guard: role %s stage %s missing required output %s", roleLabel(director), stage, required)
			}
		}
		if len(role.RequiredInputs()) > 0 && !stageHasRequiredInputs(steps, role.RequiredInputs()) {
			return fmt.Errorf("stage guard: role %s stage %s missing required inputs %v", roleLabel(director), stage, role.RequiredInputs())
		}
	}

	return nil
}

func (g *PlanGuard) validateKnowledgePolicy(plan *AgentPlan, stepManifests map[string]*tool.ToolManifest) error {
	if plan == nil || plan.KnowledgePolicy == nil {
		return nil
	}
	policy := plan.KnowledgePolicy
	if policy.RetrievalPolicy == RetrievalNone || policy.RetrievalPolicy == RetrievalForbidden {
		for _, step := range plan.Steps {
			manifest := stepManifests[step.ID]
			if hasFreshKnowledgeCapability(manifest) {
				return fmt.Errorf("knowledge policy forbids fresh knowledge tool %s when retrievalPolicy=%s: %s", step.Tool, policy.RetrievalPolicy, policy.Reason)
			}
		}
	}
	if policy.FreshnessLevel == FreshnessHigh && policy.RetrievalPolicy != RetrievalRequired {
		return fmt.Errorf("high freshness task requires retrievalPolicy=required")
	}
	for _, step := range plan.Steps {
		manifest := stepManifests[step.ID]
		if isFreshKnowledgeIntent(step) && !hasFreshKnowledgeCapability(manifest) {
			return fmt.Errorf("capability mismatch: step %s intent requires fresh knowledge but tool %s lacks required capability", step.ID, step.Tool)
		}
		if hasFreshKnowledgeCapability(manifest) && manifest.SideEffect {
			return fmt.Errorf("fresh knowledge tool %s must be sideEffect=false", step.Tool)
		}
	}
	if policy.RetrievalPolicy != RetrievalRequired {
		return nil
	}
	if len(nonEmptyStrings(policy.SearchQueries)) == 0 {
		return fmt.Errorf("required retrieval must include searchQueries")
	}
	if !policy.BlockOnEmptyFacts {
		return fmt.Errorf("required retrieval must set blockOnEmptyFacts=true")
	}
	if !planHasFreshKnowledgeTool(plan, stepManifests) {
		return fmt.Errorf("required retrieval requires a registered tool with fresh knowledge capability")
	}
	return nil
}

func validateToolPolicy(step AgentStep, manifest *tool.ToolManifest, plan *AgentPlan) error {
	if manifest == nil {
		return nil
	}
	if plan != nil && plan.KnowledgePolicy != nil && toolForbiddenByKnowledgePolicy(manifest, plan.KnowledgePolicy) {
		return fmt.Errorf("knowledge policy forbids tool %s by forbidden capability/tool: %s", step.Tool, plan.KnowledgePolicy.Reason)
	}
	if requiresApprovalByNameOrCapability(manifest) && !manifest.ApprovalPolicy.Required {
		return fmt.Errorf("agent step %s uses %s without required approval policy", step.ID, step.Tool)
	}
	if isHighCostAIGCTool(manifest) && !manifest.ApprovalPolicy.Required {
		return fmt.Errorf("agent step %s uses high-cost AIGC tool %s without approval or budget gate", step.ID, step.Tool)
	}
	if isFileWriteTool(manifest) && !hasFileWriteArtifactPolicy(manifest) {
		return fmt.Errorf("agent step %s uses file_write tool %s without pathguard-backed artifact policy", step.ID, step.Tool)
	}
	if isMCPProviderTool(manifest) {
		if err := validateMCPProviderPolicy(step, manifest); err != nil {
			return err
		}
	}
	if strings.TrimSpace(manifest.LocalCommand) != "" && isArbitraryShellCommand(manifest.LocalCommand) {
		return fmt.Errorf("agent step %s uses forbidden local command %s; CLI providers must be registered MCP tools", step.ID, manifest.LocalCommand)
	}
	return nil
}

func requiresApprovalByNameOrCapability(manifest *tool.ToolManifest) bool {
	name := strings.ToLower(strings.TrimSpace(manifest.Name))
	if strings.HasPrefix(name, "publish.") || strings.HasPrefix(name, "delete.") {
		return true
	}
	for _, capability := range manifest.Capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability == "platform_publish" || capability == "artifact_delete" || capability == "delete" {
			return true
		}
	}
	return false
}

func isHighCostAIGCTool(manifest *tool.ToolManifest) bool {
	if manifest == nil || manifest.CostLevel != tool.CostHigh {
		return false
	}
	return manifestHasAnyCapability(manifest, []string{
		"aigc_generation",
		"video_generation",
		"image_generation",
		"text_to_video",
		"text_to_image",
	})
}

func isFileWriteTool(manifest *tool.ToolManifest) bool {
	return manifestHasAnyCapability(manifest, []string{"file_write", "artifact_write", "local_file_write"})
}

func hasFileWriteArtifactPolicy(manifest *tool.ToolManifest) bool {
	if manifest == nil {
		return false
	}
	if manifest.ArtifactPolicy.ProduceArtifact {
		return true
	}
	if strings.TrimSpace(manifest.ArtifactPolicy.Storage) != "" {
		return true
	}
	return strings.TrimSpace(manifest.ArtifactLocation) != ""
}

func isMCPProviderTool(manifest *tool.ToolManifest) bool {
	if manifest == nil {
		return false
	}
	return manifest.Boundary == tool.BoundaryMCPProvider || manifest.ProviderBinding != nil || strings.EqualFold(manifest.Type, "mcp")
}

func validateMCPProviderPolicy(step AgentStep, manifest *tool.ToolManifest) error {
	if manifest.ExecutionPlane != "" && manifest.ExecutionPlane != tool.ExecutionPlaneLocal && manifest.ExecutionPlane != tool.ExecutionPlaneHybrid {
		return fmt.Errorf("agent step %s uses MCP provider tool %s on invalid execution plane %s", step.ID, step.Tool, manifest.ExecutionPlane)
	}
	if manifest.LocalCommand != "" && manifest.LocalCommand != "LOCAL_MCP_TOOL_CALL" {
		return fmt.Errorf("agent step %s uses MCP provider tool %s with non-standard local command %s", step.ID, step.Tool, manifest.LocalCommand)
	}
	if enabled, ok := providerBoolCapability(manifest, "enabled"); ok && !enabled {
		return fmt.Errorf("agent step %s uses disabled MCP provider %s for tool %s", step.ID, providerIDForManifest(manifest), step.Tool)
	}
	if enabledTools := providerStringListCapability(manifest, "enabledTools"); len(enabledTools) > 0 &&
		!toolNameInProviderList(manifest, enabledTools) {
		return fmt.Errorf("agent step %s uses MCP tool %s not present in provider enabledTools", step.ID, step.Tool)
	}
	if disabledTools := providerStringListCapability(manifest, "disabledTools"); len(disabledTools) > 0 &&
		toolNameInProviderList(manifest, disabledTools) {
		return fmt.Errorf("agent step %s uses disabled MCP tool %s", step.ID, step.Tool)
	}
	return nil
}

func providerIDForManifest(manifest *tool.ToolManifest) string {
	if manifest == nil {
		return ""
	}
	if manifest.ProviderBinding != nil && manifest.ProviderBinding.ProviderID != "" {
		return manifest.ProviderBinding.ProviderID
	}
	return manifest.Provider
}

func toolNameInProviderList(manifest *tool.ToolManifest, names []string) bool {
	if manifest == nil {
		return false
	}
	candidates := []string{manifest.Name}
	if manifest.ProviderBinding != nil {
		candidates = append(candidates, manifest.ProviderBinding.LogicalToolName, manifest.ProviderBinding.RemoteToolName)
	}
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		for _, name := range names {
			if candidate == strings.ToLower(strings.TrimSpace(name)) {
				return true
			}
		}
	}
	return false
}

func providerBoolCapability(manifest *tool.ToolManifest, key string) (bool, bool) {
	if manifest == nil || manifest.ProviderCapabilities == nil {
		return false, false
	}
	value, ok := manifest.ProviderCapabilities[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func providerStringListCapability(manifest *tool.ToolManifest, key string) []string {
	if manifest == nil || manifest.ProviderCapabilities == nil {
		return nil
	}
	value := manifest.ProviderCapabilities[key]
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func manifestHasAnyCapability(manifest *tool.ToolManifest, capabilities []string) bool {
	if manifest == nil {
		return false
	}
	allowed := toSetLower(capabilities)
	for _, capability := range manifest.Capabilities {
		if allowed[strings.ToLower(strings.TrimSpace(capability))] {
			return true
		}
	}
	return false
}

func isArbitraryShellCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "bash", "sh", "zsh", "cmd", "powershell", "python", "python3", "node", "curl":
		return true
	default:
		return false
	}
}

func planHasFreshKnowledgeTool(plan *AgentPlan, stepManifests map[string]*tool.ToolManifest) bool {
	for _, step := range plan.Steps {
		if hasFreshKnowledgeCapability(stepManifests[step.ID]) {
			return true
		}
	}
	return false
}

func isFreshKnowledgeIntent(step AgentStep) bool {
	text := strings.ToLower(strings.Join([]string{step.ID, step.Intent, step.Reason}, " "))
	for _, keyword := range []string{
		"搜索", "检索", "获取最新", "最新事实", "实时", "新闻", "当前事件",
		"search", "fresh facts", "current event retrieval", "retrieve facts",
	} {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func nonEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func resolveStepStage(step AgentStep) string {
	if step.Arguments == nil {
		return ""
	}
	stage, _ := step.Arguments["stage"].(string)
	return stage
}

func roleLabel(director StageDirector) string {
	if role, ok := director.(RoleAgentDirector); ok && role.RoleID() != "" {
		return role.RoleID()
	}
	return director.Name()
}

func stageHasRequiredInputs(steps []AgentStep, required []string) bool {
	needsArtifactInput := false
	needsUserRequest := false
	for _, input := range required {
		if input == "USER_REQUEST" {
			needsUserRequest = true
			continue
		}
		needsArtifactInput = true
	}
	if needsUserRequest {
		hasUserRequest := false
		for _, step := range steps {
			if step.Arguments == nil {
				continue
			}
			if value, ok := step.Arguments["brief"].(string); ok && value != "" {
				hasUserRequest = true
			}
			if value, ok := step.Arguments["topic"].(string); ok && value != "" {
				hasUserRequest = true
			}
		}
		if !hasUserRequest {
			return false
		}
	}
	if !needsArtifactInput {
		return true
	}
	for _, step := range steps {
		if len(step.DependsOn) > 0 {
			return true
		}
		if step.Arguments == nil {
			continue
		}
		if artifacts, ok := step.Arguments["inputArtifacts"]; ok && artifacts != nil {
			return true
		}
	}
	return false
}

func isRenderStage(stage string, director StageDirector) bool {
	if stage == "render" {
		return true
	}
	if role, ok := director.(RoleAgentDirector); ok {
		return role.RoleID() == "render_producer"
	}
	return false
}

func hasApprovedPreviewDependency(step AgentStep, stepMap map[string]AgentStep, stageByStep map[string]string) bool {
	return hasApprovedPreviewDependencyRecursive(step, stepMap, stageByStep, map[string]bool{})
}

func hasApprovedPreviewDependencyRecursive(
	step AgentStep,
	stepMap map[string]AgentStep,
	stageByStep map[string]string,
	visited map[string]bool,
) bool {
	if visited[step.ID] {
		return false
	}
	visited[step.ID] = true
	for _, dep := range step.DependsOn {
		if stageByStep[dep] == "preview" {
			return true
		}
		upstream, ok := stepMap[dep]
		if !ok {
			continue
		}
		switch upstream.Tool {
		case "hyperframes_snapshot", "preview_quality_checker":
			return true
		}
		if hasApprovedPreviewDependencyRecursive(upstream, stepMap, stageByStep, visited) {
			return true
		}
	}
	return false
}

// ValidateWithWarnings is like Validate but returns a list of non-fatal warnings
// (e.g., missing quality checkers after key production tools).
func (g *PlanGuard) ValidateWithWarnings(plan *AgentPlan) ([]string, error) {
	if err := g.Validate(plan); err != nil {
		return nil, err
	}

	var warnings []string

	// Quality gate insertion warnings — use the canonical QualityCheckerFor.
	// Build set of tool names present in the plan.
	toolSet := make(map[string]bool, len(plan.Steps))
	stepTools := make(map[string]string, len(plan.Steps))
	for _, step := range plan.Steps {
		toolSet[step.Tool] = true
		stepTools[step.ID] = step.Tool
	}

	for _, step := range plan.Steps {
		checkerTool := QualityCheckerFor(step.Tool)
		if checkerTool != "" && !toolSet[checkerTool] {
			warnings = append(warnings, fmt.Sprintf(
				"建议在 %s 之后加入 %s 进行质量检查", step.Tool, checkerTool))
		}
	}

	return warnings, nil
}

func (g *PlanGuard) manifestFor(name string) *tool.ToolManifest {
	if name == "__quality_gate__" {
		return &tool.ToolManifest{
			Name:      "__quality_gate__",
			Type:      "control",
			Boundary:  tool.BoundaryCloudBuiltin,
			CostLevel: tool.CostLow,
			RiskLevel: tool.RiskLow,
		}
	}
	if g == nil || g.tools == nil {
		return nil
	}
	return g.tools.GetManifest(name)
}

func validateCost(step AgentStep, manifest *tool.ToolManifest, maxCost string) error {
	if maxCost == "" || manifest.CostLevel == "" {
		return nil
	}
	if costRank(manifest.CostLevel) > costRank(maxCost) {
		return fmt.Errorf("agent step %s uses cost level %s above budget %s", step.ID, manifest.CostLevel, maxCost)
	}
	return nil
}

func costRank(level string) int {
	switch level {
	case tool.CostHigh:
		return 3
	case tool.CostMedium:
		return 2
	default:
		return 1
	}
}

func validateRequiredParameters(step AgentStep, manifest *tool.ToolManifest) error {
	for name, param := range manifest.Parameters {
		if !param.Required {
			continue
		}
		if _, ok := step.Arguments[name]; !ok {
			return fmt.Errorf("agent step %s missing required parameter %s for tool %s", step.ID, name, step.Tool)
		}
	}
	return nil
}

func validateRiskLevel(step AgentStep, manifest *tool.ToolManifest) error {
	if manifest.RiskLevel == "" {
		return nil
	}
	// Risk levels: low, medium, high.
	// Only enforce that high-risk tools must have approval.
	if riskRank(manifest.RiskLevel) >= 3 && !manifest.ApprovalPolicy.Required {
		return fmt.Errorf("agent step %s uses high-risk tool %s without approval policy", step.ID, step.Tool)
	}
	return nil
}

func riskRank(level string) int {
	switch level {
	case tool.RiskHigh:
		return 3
	case tool.RiskMedium:
		return 2
	default:
		return 1
	}
}

// validateParameterTypes checks that argument values match the expected type
// from the tool manifest parameter definitions.
func validateParameterTypes(step AgentStep, manifest *tool.ToolManifest) error {
	for name, value := range step.Arguments {
		param, ok := manifest.Parameters[name]
		if !ok {
			continue
		}
		// Skip reference expressions — they are resolved at runtime.
		if isReferenceExpression(value) {
			continue
		}
		if !matchesParamType(value, param.Type) {
			return fmt.Errorf("agent step %s parameter %s type mismatch: want %s", step.ID, name, param.Type)
		}
	}
	return nil
}

// isReferenceExpression checks if a value is a {{step.output.field}} reference.
func isReferenceExpression(value interface{}) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	return referencePattern.MatchString(s)
}

// matchesParamType checks if a value matches the expected parameter type.
func matchesParamType(value interface{}, expectedType string) bool {
	if value == nil {
		return true
	}
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32, json.Number:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		switch value.(type) {
		case []interface{}, []string, []map[string]interface{}:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	default:
		return true
	}
}

// validateReferenceExpressions checks that {{step.output.field}} references
// point to existing upstream steps with the declared output fields.
// It validates:
//  1. The referenced step exists.
//  2. The referenced step is declared as a dependency.
//  3. The referenced field exists in the upstream tool's output schema.
func validateReferenceExpressions(
	step AgentStep,
	stepMap map[string]AgentStep,
	stepManifests map[string]*tool.ToolManifest,
) error {
	for _, reference := range argumentReferences(step.Arguments) {
		refStepID := reference.StepID
		refField := reference.Field
		expression := reference.Expression

		// Check the referenced step exists.
		refStep, exists := stepMap[refStepID]
		if !exists {
			return fmt.Errorf("agent step %s references unknown step %s in argument expression %s", step.ID, refStepID, expression)
		}
		if refStepID == step.ID {
			return fmt.Errorf("agent step %s cannot reference its own output in argument expression %s", step.ID, expression)
		}

		// Check the referenced step is an upstream dependency.
		isUpstream := false
		for _, dep := range step.DependsOn {
			if dep == refStepID {
				isUpstream = true
				break
			}
		}
		if !isUpstream {
			return fmt.Errorf("agent step %s references step %s which is not declared as a dependency", step.ID, refStepID)
		}

		// Validate the referenced field exists in the upstream tool's output schema.
		if stepManifests != nil {
			refManifest := stepManifests[refStep.ID]
			if refManifest == nil {
				return fmt.Errorf(
					"agent step %s references step %s without manifest, cannot validate output field %s",
					step.ID, refStepID, refField,
				)
			}
			if len(refManifest.Output) > 0 {
				if _, ok := refManifest.Output[refField]; !ok {
					return fmt.Errorf(
						"agent step %s references output field %s of step %s, but tool %s does not declare this output field",
						step.ID, refField, refStepID, refStep.Tool,
					)
				}
			}
		}
	}
	return nil
}

// countToolCalls counts the expected number of LLM/tool calls, accounting for
// approval policies that add CONTROL nodes.
func countToolCalls(steps []AgentStep) int {
	count := len(steps)
	for _, step := range steps {
		// If a tool has approval, add 1 for the CONTROL node.
		// This is estimated; the PlanCompiler has the exact count.
		_ = step
	}
	return count
}

// QualityCheckerFor returns the recommended quality checker tool name for a
// production tool, or empty string if no checker is defined.
func QualityCheckerFor(prodTool string) string {
	switch prodTool {
	case "video_script_generator":
		return "script_quality_checker"
	case "shot_splitter":
		return "shot_quality_checker"
	case "video_prompt_generator":
		return "video_prompt_quality_checker"
	case "video_package_exporter":
		return "package_quality_checker"
	default:
		return ""
	}
}
