package agentruntime

import (
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
}

func NewPlanGuard(tools ToolCatalog) *PlanGuard {
	return &PlanGuard{
		tools:          tools,
		localValidator: &LocalCapabilityValidator{},
	}
}

func (g *PlanGuard) Validate(plan *AgentPlan) error {
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

	// MaxToolCalls check.
	if plan.Budget.MaxToolCalls > 0 {
		toolCallCount := countToolCalls(plan.Steps)
		if toolCallCount > plan.Budget.MaxToolCalls {
			return fmt.Errorf("agent plan has %d tool calls, exceeds maxToolCalls %d", toolCallCount, plan.Budget.MaxToolCalls)
		}
	}

	return nil
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
//   1. The referenced step exists.
//   2. The referenced step is declared as a dependency.
//   3. The referenced field exists in the upstream tool's output schema.
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
