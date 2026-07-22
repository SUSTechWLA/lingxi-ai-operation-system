package agentruntime

import (
	"fmt"
	"sort"
	"strings"
)

// AgentCapabilityLayer names one capability in the agent execution loop.
type AgentCapabilityLayer string

const (
	LayerContext   AgentCapabilityLayer = "context"
	LayerTools     AgentCapabilityLayer = "tools"
	LayerConstrain AgentCapabilityLayer = "constrain"
	LayerVerify    AgentCapabilityLayer = "verify"
	LayerCorrect   AgentCapabilityLayer = "correct"

	// PublicDecisionTraceOnly means the runtime may expose tool-selection reasons,
	// guard decisions, and verification outcomes, but never raw chain-of-thought.
	PublicDecisionTraceOnly = "public_decision_trace_only"
)

var requiredCapabilityLayers = []AgentCapabilityLayer{
	LayerContext,
	LayerTools,
	LayerConstrain,
	LayerVerify,
	LayerCorrect,
}

// AgentRuntimeLayer describes the public contract of one capability layer.
// It identifies existing runtime components rather than introducing a parallel
// execution framework.
type AgentRuntimeLayer struct {
	Name           AgentCapabilityLayer   `json:"name"`
	Responsibility string                 `json:"responsibility"`
	Inputs         []string               `json:"inputs"`
	Outputs        []string               `json:"outputs"`
	Components     []string               `json:"components"`
	DependsOn      []AgentCapabilityLayer `json:"dependsOn,omitempty"`
}

// AgentRuntimeContract is immutable to callers: construction and reads clone
// every slice, so process-wide conformance data cannot be mutated accidentally.
type AgentRuntimeContract struct {
	layers              []AgentRuntimeLayer
	decisionTracePolicy string
}

type AgentLayerConformance struct {
	Present    bool     `json:"present"`
	Connected  bool     `json:"connected"`
	Components []string `json:"components,omitempty"`
}

type AgentRuntimeConformanceReport struct {
	Passed              bool                                           `json:"passed"`
	Connected           bool                                           `json:"connected"`
	DecisionTracePolicy string                                         `json:"decisionTracePolicy"`
	Layers              map[AgentCapabilityLayer]AgentLayerConformance `json:"layers"`
	DependencyFlow      []string                                       `json:"dependencyFlow,omitempty"`
	Errors              []string                                       `json:"errors,omitempty"`
}

func NewAgentRuntimeContract(layers []AgentRuntimeLayer, decisionTracePolicy string) AgentRuntimeContract {
	return AgentRuntimeContract{
		layers:              cloneRuntimeLayers(layers),
		decisionTracePolicy: strings.TrimSpace(decisionTracePolicy),
	}
}

// StandardAgentRuntimeContract returns the contract implemented by the current
// agentruntime package. Component identifiers are stable public audit labels.
func StandardAgentRuntimeContract() AgentRuntimeContract {
	return NewAgentRuntimeContract([]AgentRuntimeLayer{
		{
			Name:           LayerContext,
			Responsibility: "Provide the user request, retrieved knowledge, compacted project state, and role memory to planning.",
			Inputs:         []string{"StartRunRequest.message", "StartRunRequest.domain", "StartRunRequest.mode", "StartRunRequest.context", "CompactedContext", "RoleMemory"},
			Outputs:        []string{"planner public request context"},
			Components:     []string{"StartRunRequest", "plannerUserPrompt", "CompactedContext", "RoleMemory"},
		},
		{
			Name:           LayerTools,
			Responsibility: "Discover and select actionable tools using canonical JSON Schema and provider metadata.",
			Inputs:         []string{"planner public request context", "ToolManifest.inputSchema", "ToolManifest.outputSchema"},
			Outputs:        []string{"ToolCandidate", "AgentPlan.steps"},
			Components:     []string{"ToolManifest", "HybridToolRetriever", "LLMPlanner"},
			DependsOn:      []AgentCapabilityLayer{LayerContext},
		},
		{
			Name:           LayerConstrain,
			Responsibility: "Enforce budgets, tool policy, cost, risk, approvals, side effects, and local capabilities before execution.",
			Inputs:         []string{"AgentPlan", "ToolManifest policy", "local capabilities"},
			Outputs:        []string{"GuardDecisionTrace", "validation error"},
			Components:     []string{"PlanGuard", "LocalCapabilityValidator"},
			DependsOn:      []AgentCapabilityLayer{LayerTools},
		},
		{
			Name:           LayerVerify,
			Responsibility: "Insert policy-required automated checkers and quality gates, then judge the prepared plan.",
			Inputs:         []string{"prepared AgentPlan", "ToolManifest.qualityPolicy"},
			Outputs:        []string{"checker step", "quality gate", "PlanJudgeReport"},
			Components:     []string{"PlanCompiler", "PlanJudge"},
			DependsOn:      []AgentCapabilityLayer{LayerTools},
		},
		{
			Name:           LayerCorrect,
			Responsibility: "Repair a rejected plan once, prepare it again, and require complete revalidation before execution; preserve quality auto-repair metadata.",
			Inputs:         []string{"guard error", "rejected AgentPlan", "quality gate result"},
			Outputs:        []string{"prepared repaired plan", "revalidation result", "quality autoRepair policy"},
			Components:     []string{"Runner", "PlanRepairer", "PlanCompiler.PreparePlan", "PlanGuard.ValidatePlan", "QualityPolicy.AutoRepair"},
			DependsOn:      []AgentCapabilityLayer{LayerConstrain, LayerVerify},
		},
	}, PublicDecisionTraceOnly)
}

func (c AgentRuntimeContract) Layers() []AgentRuntimeLayer {
	return cloneRuntimeLayers(c.layers)
}

func (c AgentRuntimeContract) DecisionTracePolicy() string {
	return c.decisionTracePolicy
}

func (c AgentRuntimeContract) Validate() error {
	if c.decisionTracePolicy != PublicDecisionTraceOnly {
		return fmt.Errorf("agent runtime contract requires public decision trace only; raw chain-of-thought must not be persisted or exposed")
	}
	byName := make(map[AgentCapabilityLayer]AgentRuntimeLayer, len(c.layers))
	for _, layer := range c.layers {
		if _, exists := byName[layer.Name]; exists {
			return fmt.Errorf("duplicate layer %s", layer.Name)
		}
		byName[layer.Name] = layer
	}
	for _, required := range requiredCapabilityLayers {
		if _, exists := byName[required]; !exists {
			return fmt.Errorf("missing required layer %s", required)
		}
	}
	for _, layer := range c.layers {
		if strings.TrimSpace(layer.Responsibility) == "" {
			return fmt.Errorf("layer %s has no responsibility", layer.Name)
		}
		if len(layer.Inputs) == 0 || len(layer.Outputs) == 0 {
			return fmt.Errorf("layer %s must declare inputs and outputs", layer.Name)
		}
		if len(layer.Components) == 0 {
			return fmt.Errorf("layer %s has no implementation components", layer.Name)
		}
		for _, component := range layer.Components {
			if !knownAgentRuntimeComponent(component) {
				return fmt.Errorf("layer %s references unknown implementation component %s", layer.Name, component)
			}
		}
		for _, dependency := range layer.DependsOn {
			if _, exists := byName[dependency]; !exists {
				return fmt.Errorf("layer %s depends on unknown layer %s", layer.Name, dependency)
			}
		}
	}
	if err := validateRuntimeDependencyGraph(byName); err != nil {
		return err
	}
	return nil
}

func knownAgentRuntimeComponent(component string) bool {
	switch component {
	case "StartRunRequest", "plannerUserPrompt", "CompactedContext", "RoleMemory",
		"ToolManifest", "HybridToolRetriever", "LLMPlanner",
		"PlanGuard", "LocalCapabilityValidator",
		"PlanCompiler", "PlanJudge",
		"Runner", "PlanRepairer", "PlanCompiler.PreparePlan", "PlanGuard.ValidatePlan", "QualityPolicy.AutoRepair":
		return true
	default:
		return false
	}
}

func (c AgentRuntimeContract) Conformance() AgentRuntimeConformanceReport {
	report := AgentRuntimeConformanceReport{
		DecisionTracePolicy: c.decisionTracePolicy,
		Layers:              make(map[AgentCapabilityLayer]AgentLayerConformance, len(requiredCapabilityLayers)),
	}
	byName := make(map[AgentCapabilityLayer]AgentRuntimeLayer, len(c.layers))
	for _, layer := range c.layers {
		byName[layer.Name] = layer
	}
	for _, required := range requiredCapabilityLayers {
		layer, present := byName[required]
		connected := present && runtimeLayerReachesContext(required, byName, map[AgentCapabilityLayer]bool{})
		report.Layers[required] = AgentLayerConformance{
			Present: present, Connected: connected, Components: append([]string(nil), layer.Components...),
		}
		if present {
			for _, dependency := range layer.DependsOn {
				report.DependencyFlow = append(report.DependencyFlow, fmt.Sprintf("%s->%s", dependency, required))
			}
		}
	}
	sort.Strings(report.DependencyFlow)
	if err := c.Validate(); err != nil {
		report.Errors = []string{err.Error()}
		return report
	}
	report.Connected = true
	for _, status := range report.Layers {
		if !status.Connected {
			report.Connected = false
			break
		}
	}
	report.Passed = report.Connected
	return report
}

func validateRuntimeDependencyGraph(layers map[AgentCapabilityLayer]AgentRuntimeLayer) error {
	state := map[AgentCapabilityLayer]int{}
	var visit func(AgentCapabilityLayer) error
	visit = func(name AgentCapabilityLayer) error {
		switch state[name] {
		case 1:
			return fmt.Errorf("agent runtime dependency cycle includes layer %s", name)
		case 2:
			return nil
		}
		state[name] = 1
		for _, dependency := range layers[name].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for name := range layers {
		if err := visit(name); err != nil {
			return err
		}
		if !runtimeLayerReachesContext(name, layers, map[AgentCapabilityLayer]bool{}) {
			return fmt.Errorf("layer %s is disconnected from context", name)
		}
	}
	return nil
}

func runtimeLayerReachesContext(name AgentCapabilityLayer, layers map[AgentCapabilityLayer]AgentRuntimeLayer, visiting map[AgentCapabilityLayer]bool) bool {
	if name == LayerContext {
		return true
	}
	if visiting[name] {
		return false
	}
	visiting[name] = true
	defer delete(visiting, name)
	for _, dependency := range layers[name].DependsOn {
		if runtimeLayerReachesContext(dependency, layers, visiting) {
			return true
		}
	}
	return false
}

func cloneRuntimeLayers(input []AgentRuntimeLayer) []AgentRuntimeLayer {
	if input == nil {
		return nil
	}
	out := make([]AgentRuntimeLayer, len(input))
	for i := range input {
		out[i] = input[i]
		out[i].Inputs = append([]string(nil), input[i].Inputs...)
		out[i].Outputs = append([]string(nil), input[i].Outputs...)
		out[i].Components = append([]string(nil), input[i].Components...)
		out[i].DependsOn = append([]AgentCapabilityLayer(nil), input[i].DependsOn...)
	}
	return out
}
