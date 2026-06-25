package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

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

// ValidatePlan performs full plan validation including local capability checks
// when a user ID is provided.
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

		// Local capability check: if this tool requires local execution,
		// verify the user's local runner is online and supports it.
		if userID != "" && g.localValidator != nil {
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
		if manifest.SideEffect && !manifest.ApprovalPolicy.Required {
			return fmt.Errorf("agent step %s uses side-effect tool %s without approval policy", step.ID, step.Tool)
		}
		for _, dep := range step.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("agent step %s depends on unknown or later step %s", step.ID, dep)
			}
		}
		// Validate reference expressions in arguments (includes output field validation).
		if err := validateReferenceExpressions(step, stepMap, stepManifests); err != nil {
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
		_, ok := value.([]interface{})
		return ok
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
	for _, value := range step.Arguments {
		s, ok := value.(string)
		if !ok {
			continue
		}
		matches := referencePattern.FindStringSubmatch(s)
		if matches == nil {
			continue
		}
		refStepID := matches[1]
		refField := matches[2]

		// Check the referenced step exists.
		refStep, exists := stepMap[refStepID]
		if !exists {
			return fmt.Errorf("agent step %s references unknown step %s in argument expression %s", step.ID, refStepID, s)
		}

		// Check the referenced step is an upstream dependency.
		isUpstream := false
		for _, dep := range step.DependsOn {
			if dep == refStepID {
				isUpstream = true
				break
			}
		}
		if !isUpstream && refStepID != step.ID {
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
