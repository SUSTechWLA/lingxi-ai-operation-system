package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestStandardAgentRuntimeContractReportsFiveConnectedLayers(t *testing.T) {
	contract := StandardAgentRuntimeContract()
	if err := contract.Validate(); err != nil {
		t.Fatalf("standard contract Validate returned error: %v", err)
	}
	report := contract.Conformance()
	if !report.Passed || !report.Connected {
		t.Fatalf("standard contract not conformant: %#v", report)
	}
	for _, layer := range []AgentCapabilityLayer{LayerContext, LayerTools, LayerConstrain, LayerVerify, LayerCorrect} {
		status, ok := report.Layers[layer]
		if !ok || !status.Present || !status.Connected || len(status.Components) == 0 {
			t.Fatalf("layer %q is not present and connected to real components: %#v", layer, status)
		}
	}
	if report.DecisionTracePolicy != PublicDecisionTraceOnly {
		t.Fatalf("decision trace policy = %q, want %q", report.DecisionTracePolicy, PublicDecisionTraceOnly)
	}

	// Callers receive copies and cannot mutate the process-wide contract.
	layers := contract.Layers()
	layers[0].Components[0] = "mutated"
	layers[1].DependsOn[0] = LayerCorrect
	fresh := StandardAgentRuntimeContract().Layers()
	if fresh[0].Components[0] == "mutated" || fresh[1].DependsOn[0] == LayerCorrect {
		t.Fatalf("standard contract leaked mutable slices: %#v", fresh)
	}
}

func TestAgentRuntimeContractRejectsMissingDuplicateDisconnectedAndUnimplementedLayers(t *testing.T) {
	base := StandardAgentRuntimeContract().Layers()
	tests := []struct {
		name   string
		layers []AgentRuntimeLayer
		want   string
	}{
		{name: "missing", layers: append([]AgentRuntimeLayer(nil), base[1:]...), want: "missing required layer context"},
		{name: "duplicate", layers: append(append([]AgentRuntimeLayer(nil), base...), base[0]), want: "duplicate layer context"},
		{name: "disconnected", layers: mutateContractLayer(base, LayerTools, func(layer *AgentRuntimeLayer) { layer.DependsOn = []AgentCapabilityLayer{"absent"} }), want: "depends on unknown layer absent"},
		{name: "unimplemented", layers: mutateContractLayer(base, LayerVerify, func(layer *AgentRuntimeLayer) { layer.Components = nil }), want: "layer verify has no implementation components"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := NewAgentRuntimeContract(tt.layers, PublicDecisionTraceOnly)
			if err := contract.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAgentRuntimeContractRejectsRawReasoningTracePolicy(t *testing.T) {
	contract := NewAgentRuntimeContract(StandardAgentRuntimeContract().Layers(), "persist_raw_chain_of_thought")
	if err := contract.Validate(); err == nil || !strings.Contains(err.Error(), "public decision trace") {
		t.Fatalf("Validate error = %v, want public decision trace policy rejection", err)
	}
}

func TestCapabilityLoopUsesRealConstrainAndVerifyComponents(t *testing.T) {
	t.Run("constrain rejects unapproved side effects", func(t *testing.T) {
		catalog := staticToolCatalog{
			"publisher": {Name: "publisher", SideEffect: true, RiskLevel: tool.RiskHigh},
		}
		plan := &AgentPlan{Steps: []AgentStep{{ID: "publish", Tool: "publisher"}}}
		err := NewPlanGuard(catalog, nil).Validate(plan)
		if err == nil || !strings.Contains(err.Error(), "without approval policy") {
			t.Fatalf("PlanGuard.Validate error = %v, want approval rejection", err)
		}
	})

	t.Run("verify inserts checker gate and reconnects downstream", func(t *testing.T) {
		catalog := qualityContractCatalog()
		plan := &AgentPlan{
			Goal: "produce and publish", Domain: "general", Mode: "dynamic_agent",
			Steps: []AgentStep{
				{ID: "produce", Tool: "producer"},
				{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}},
			},
		}
		prepared := NewPlanCompiler(catalog).PreparePlan(plan)
		checker := findPlanStep(prepared, "quality_checker")
		gate := findPlanStep(prepared, "produce_quality_gate")
		consumer := findPlanStep(prepared, "consume")
		if checker.Tool != "quality_checker" || gate.Tool != "__quality_gate__" {
			t.Fatalf("required quality verification was not inserted: %#v", prepared.Steps)
		}
		if len(consumer.DependsOn) != 1 || consumer.DependsOn[0] != gate.ID {
			t.Fatalf("downstream not reconnected through gate: %#v", consumer)
		}
		if gate.Arguments["autoRepair"] != true || gate.Arguments["maxRepairAttempts"] != 2 {
			t.Fatalf("quality correction metadata missing: %#v", gate.Arguments)
		}
	})
}

func TestRunnerCorrectionPreparesAndRevalidatesOneRepair(t *testing.T) {
	t.Run("legal repair continues only after preparation and revalidation", func(t *testing.T) {
		planner := &repairingContractPlanner{
			initial:  &AgentPlan{Goal: "bad", Domain: "general", Steps: []AgentStep{{ID: "bad", Tool: "unknown"}}},
			repaired: &AgentPlan{Goal: "fixed", Domain: "general", Mode: "dynamic_agent", Steps: []AgentStep{{ID: "produce", Tool: "producer"}}},
		}
		catalog := qualityContractCatalog()
		preparedPreview := NewPlanCompiler(catalog).PreparePlan(cloneContractPlan(planner.repaired))
		if err := NewPlanGuard(catalog, nil).Validate(preparedPreview); err != nil {
			t.Fatalf("test legal repair does not satisfy Guard after preparation: %v; plan=%#v", err, preparedPreview.Steps)
		}
		runner := NewRunner(&fakeOrchestrator{taskID: "task-contract"}, newMemoryRunStore(), planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog))

		run, err := runner.Start(context.Background(), StartRunRequest{Message: "produce verified output", Domain: "general"})
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
		if planner.repairCalls != 1 {
			t.Fatalf("repair calls = %d, want exactly 1", planner.repairCalls)
		}
		if findPlanStep(run.Plan, "quality_checker").Tool != "quality_checker" || findPlanStep(run.Plan, "produce_quality_gate").Tool != "__quality_gate__" {
			t.Fatalf("repaired plan skipped PreparePlan: %#v", run.Plan.Steps)
		}
	})

	t.Run("illegal repair remains blocked by revalidation", func(t *testing.T) {
		planner := &repairingContractPlanner{
			initial:  &AgentPlan{Goal: "bad", Domain: "general", Steps: []AgentStep{{ID: "bad", Tool: "unknown"}}},
			repaired: &AgentPlan{Goal: "still bad", Domain: "general", Steps: []AgentStep{{ID: "bad_again", Tool: "still_unknown"}}},
		}
		catalog := qualityContractCatalog()
		runner := NewRunner(&fakeOrchestrator{taskID: "task-contract"}, newMemoryRunStore(), planner, NewPlanGuard(catalog, nil), NewPlanCompiler(catalog))

		if _, err := runner.Start(context.Background(), StartRunRequest{Message: "produce verified output", Domain: "general"}); err == nil {
			t.Fatal("Start succeeded with an illegal repaired plan")
		}
		if planner.repairCalls != 1 {
			t.Fatalf("repair calls = %d, want exactly 1", planner.repairCalls)
		}
	})
}

func TestPlanCompilerAddsQualityGateWhenRequiredCheckerAlreadyExists(t *testing.T) {
	catalog := qualityContractCatalog()
	plan := &AgentPlan{
		Goal: "produce and verify", Domain: "general", Mode: "dynamic_agent",
		Steps: []AgentStep{
			{ID: "produce", Tool: "producer"},
			{ID: "explicit_check", Tool: "quality_checker", DependsOn: []string{"produce"}},
			{ID: "consume", Tool: "consumer", DependsOn: []string{"explicit_check"}},
		},
	}

	prepared := NewPlanCompiler(catalog).PreparePlan(plan)
	gate := findPlanStep(prepared, "produce_quality_gate")
	consumer := findPlanStep(prepared, "consume")
	if gate.Tool != "__quality_gate__" || len(gate.DependsOn) != 1 || gate.DependsOn[0] != "explicit_check" {
		t.Fatalf("required gate not connected to explicit checker: %#v", prepared.Steps)
	}
	if len(consumer.DependsOn) != 1 || consumer.DependsOn[0] != gate.ID {
		t.Fatalf("explicit checker downstream not reconnected through gate: %#v", consumer)
	}
}

func cloneContractPlan(plan *AgentPlan) *AgentPlan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.Steps = append([]AgentStep(nil), plan.Steps...)
	return &cloned
}

func mutateContractLayer(input []AgentRuntimeLayer, name AgentCapabilityLayer, mutate func(*AgentRuntimeLayer)) []AgentRuntimeLayer {
	out := make([]AgentRuntimeLayer, len(input))
	for i := range input {
		out[i] = input[i]
		out[i].Inputs = append([]string(nil), input[i].Inputs...)
		out[i].Outputs = append([]string(nil), input[i].Outputs...)
		out[i].Components = append([]string(nil), input[i].Components...)
		out[i].DependsOn = append([]AgentCapabilityLayer(nil), input[i].DependsOn...)
		if out[i].Name == name {
			mutate(&out[i])
		}
	}
	return out
}

func qualityContractCatalog() staticToolCatalog {
	return staticToolCatalog{
		"producer": {
			Name: "producer", Output: map[string]tool.ParamDef{"artifact": {Type: "string"}},
			QualityPolicy: tool.QualityPolicy{Required: true, CheckerTool: "quality_checker", MinScore: 90, AutoRepair: true, MaxRepairAttempts: 2},
		},
		"quality_checker": {Name: "quality_checker", Output: map[string]tool.ParamDef{"passed": {Type: "boolean"}, "score": {Type: "number"}}},
		"consumer":        {Name: "consumer"},
	}
}

type repairingContractPlanner struct {
	initial     *AgentPlan
	repaired    *AgentPlan
	repairCalls int
}

func (p *repairingContractPlanner) GeneratePlan(context.Context, StartRunRequest) (*AgentPlan, error) {
	return p.initial, nil
}

func (p *repairingContractPlanner) RepairPlan(context.Context, *AgentPlan, string) (*AgentPlan, error) {
	p.repairCalls++
	return p.repaired, nil
}
