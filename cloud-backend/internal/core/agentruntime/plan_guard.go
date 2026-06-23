package agentruntime

import (
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type PlanGuard struct {
	tools ToolCatalog
}

func NewPlanGuard(tools ToolCatalog) *PlanGuard {
	return &PlanGuard{tools: tools}
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
		if err := validateRequiredParameters(step, manifest); err != nil {
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
	}
	return nil
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
