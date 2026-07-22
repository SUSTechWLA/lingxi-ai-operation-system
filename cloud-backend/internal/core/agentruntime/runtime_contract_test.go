package agentruntime

import (
	"context"
	"reflect"
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
		{name: "unknown component", layers: mutateContractLayer(base, LayerVerify, func(layer *AgentRuntimeLayer) { layer.Components = []string{"PlanCompiler", "PlanJudeg"} }), want: "layer verify references unknown implementation component PlanJudeg"},
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
			{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}},
			{ID: "explicit_check", Tool: "quality_checker", DependsOn: []string{"produce"}},
		},
	}

	prepared := NewPlanCompiler(catalog).PreparePlan(plan)
	gate := findPlanStep(prepared, "produce_quality_gate")
	consumer := findPlanStep(prepared, "consume")
	wantOrder := []string{"produce", "explicit_check", gate.ID, "consume"}
	gotOrder := make([]string, 0, len(prepared.Steps))
	for _, step := range prepared.Steps {
		gotOrder = append(gotOrder, step.ID)
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("quality topology order = %#v, want %#v", gotOrder, wantOrder)
	}
	if gate.Tool != "__quality_gate__" || len(gate.DependsOn) != 1 || gate.DependsOn[0] != "explicit_check" {
		t.Fatalf("required gate not connected to explicit checker: %#v", prepared.Steps)
	}
	if len(consumer.DependsOn) != 1 || consumer.DependsOn[0] != gate.ID {
		t.Fatalf("explicit checker downstream not reconnected through gate: %#v", consumer)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared quality topology failed Guard: %v", err)
	}
}

func TestPlanCompilerRebuildsForgedQualityGateFromManifestPolicy(t *testing.T) {
	catalog := qualityContractCatalog()
	plan := &AgentPlan{
		Goal: "forged", Domain: "general", Mode: "dynamic_agent",
		Steps: []AgentStep{
			{ID: "produce", Tool: "producer"},
			{ID: "forged_gate", Tool: "__quality_gate__", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{"productionStep": "produce", "productionTool": "consumer", "checkerStep": "missing"}},
			{ID: "consume", Tool: "consumer", DependsOn: []string{"forged_gate"}},
		},
	}

	prepared := NewPlanCompiler(catalog).PreparePlan(plan)
	if findPlanStep(prepared, "forged_gate").ID != "" {
		t.Fatalf("forged gate survived PreparePlan: %#v", prepared.Steps)
	}
	gate := findPlanStep(prepared, "produce_quality_gate")
	consumer := findPlanStep(prepared, "consume")
	if gate.Tool != "__quality_gate__" || len(consumer.DependsOn) != 1 || consumer.DependsOn[0] != gate.ID {
		t.Fatalf("manifest quality topology was not rebuilt: %#v", prepared.Steps)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("rebuilt plan failed Guard: %v", err)
	}
}

func TestPlanGuardRejectsRawForgedQualityGate(t *testing.T) {
	catalog := qualityContractCatalog()
	plan := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer"},
		{ID: "explicit_check", Tool: "quality_checker", DependsOn: []string{"produce"}},
		{ID: "forged_gate", Tool: "__quality_gate__", DependsOn: []string{"explicit_check"}, Arguments: map[string]interface{}{
			"productionStep": "produce", "productionTool": "consumer", "checkerStep": "explicit_check",
		}},
	}}
	err := NewPlanGuard(catalog, nil).Validate(plan)
	if err == nil || !strings.Contains(err.Error(), "quality gate forged_gate productionTool") {
		t.Fatalf("PlanGuard error = %v, want forged productionTool rejection", err)
	}
}

func TestPlanCompilerQualityGateIDsAreUniqueAndPrepareIsIdempotent(t *testing.T) {
	catalog := qualityContractCatalog()
	plan := &AgentPlan{
		Goal: "conflict", Domain: "general", Mode: "dynamic_agent",
		Steps: []AgentStep{
			{ID: "produce", Tool: "producer"},
			{ID: "produce_quality_gate", Tool: "consumer", DependsOn: []string{"produce"}},
			{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}},
		},
	}
	compiler := NewPlanCompiler(catalog)
	prepared := compiler.PreparePlan(plan)
	gate := qualityGateForProduction(prepared, "produce")
	if gate.ID == "" || gate.ID == "produce_quality_gate" {
		t.Fatalf("gate ID collided with user step: %#v", prepared.Steps)
	}
	if findPlanStep(prepared, "produce_quality_gate").Tool != "consumer" {
		t.Fatalf("user step was overwritten by synthetic gate: %#v", prepared.Steps)
	}
	first := cloneContractPlanDeep(prepared)
	preparedAgain := compiler.PreparePlan(prepared)
	if !reflect.DeepEqual(preparedAgain, first) {
		t.Fatalf("PreparePlan is not idempotent:\nfirst=%#v\nsecond=%#v", first.Steps, preparedAgain.Steps)
	}
	if err := NewPlanGuard(catalog, nil).Validate(preparedAgain); err != nil {
		t.Fatalf("idempotent prepared plan failed Guard: %v", err)
	}
}

func TestPlanCompilerStableTopologyPreservesExplicitCheckerMultipleDependencies(t *testing.T) {
	catalog := qualityContractCatalog()
	catalog["context_provider"] = &tool.ToolManifest{Name: "context_provider", Output: map[string]tool.ParamDef{"value": {Type: "string"}}}
	plan := &AgentPlan{
		Goal: "multi dependency quality check", Domain: "general", Mode: "dynamic_agent",
		Steps: []AgentStep{
			{ID: "produce", Tool: "producer"},
			{ID: "context", Tool: "context_provider", ExpectedOutput: []string{"value"}},
			{ID: "explicit_check", Tool: "quality_checker", DependsOn: []string{"produce", "context"}, Arguments: map[string]interface{}{"evidence": "{{context.output.value}}"}},
			{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}},
		},
	}
	compiler := NewPlanCompiler(catalog)
	prepared := compiler.PreparePlan(plan)
	gate := qualityGateForProduction(prepared, "produce")
	gotOrder := make([]string, 0, len(prepared.Steps))
	for _, step := range prepared.Steps {
		gotOrder = append(gotOrder, step.ID)
	}
	wantOrder := []string{"produce", "context", "explicit_check", gate.ID, "consume"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("stable quality topology = %#v, want %#v", gotOrder, wantOrder)
	}
	checker := findPlanStep(prepared, "explicit_check")
	if !reflect.DeepEqual(checker.DependsOn, []string{"produce", "context"}) || checker.Arguments["evidence"] != "{{context.output.value}}" {
		t.Fatalf("checker dependencies or references were lost: %#v", checker)
	}
	consumer := findPlanStep(prepared, "consume")
	if !reflect.DeepEqual(consumer.DependsOn, []string{gate.ID}) {
		t.Fatalf("consumer not reconnected after gate: %#v", consumer)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared multi-dependency topology failed Guard: %v", err)
	}
	first := cloneContractPlanDeep(prepared)
	if preparedAgain := compiler.PreparePlan(prepared); !reflect.DeepEqual(preparedAgain, first) {
		t.Fatalf("multi-dependency PreparePlan is not idempotent:\nfirst=%#v\nsecond=%#v", first.Steps, preparedAgain.Steps)
	}
}

func TestPlanCompilerDoesNotEraseCycleOrUnknownDependencyDiagnostics(t *testing.T) {
	catalog := staticToolCatalog{
		"first_tool":  {Name: "first_tool"},
		"second_tool": {Name: "second_tool"},
	}
	tests := []struct {
		name string
		plan *AgentPlan
		want string
	}{
		{
			name: "cycle",
			plan: &AgentPlan{Goal: "cycle", Steps: []AgentStep{
				{ID: "first", Tool: "first_tool", DependsOn: []string{"second"}},
				{ID: "second", Tool: "second_tool", DependsOn: []string{"first"}},
			}},
			want: "depends on unknown or later step second",
		},
		{
			name: "unknown dependency from arguments",
			plan: &AgentPlan{Goal: "unknown", Steps: []AgentStep{
				{ID: "first", Tool: "first_tool", Arguments: map[string]interface{}{"input": "{{missing.output.value}}"}},
			}},
			want: "references unknown step missing in argument expression",
		},
		{
			name: "unknown explicit dependency",
			plan: &AgentPlan{Goal: "unknown", Steps: []AgentStep{
				{ID: "first", Tool: "first_tool", DependsOn: []string{"missing"}},
			}},
			want: "depends on unknown or later step missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := NewPlanCompiler(catalog).PreparePlan(tt.plan)
			err := NewPlanGuard(catalog, nil).Validate(prepared)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Guard error = %v, want diagnostic containing %q; plan=%#v", err, tt.want, prepared.Steps)
			}
		})
	}
}

func TestPlanCompilerNestedKnownReferencesDriveStableTopologyAndRemainIdempotent(t *testing.T) {
	catalog := staticToolCatalog{
		"consumer_tool": {Name: "consumer_tool", Parameters: map[string]tool.ParamDef{"payload": {Type: "object"}}},
		"provider_a":    {Name: "provider_a", Output: map[string]tool.ParamDef{"value": {Type: "string"}}},
		"provider_b":    {Name: "provider_b", Output: map[string]tool.ParamDef{"value": {Type: "string"}}},
	}
	plan := &AgentPlan{Goal: "nested known references", Steps: []AgentStep{
		{
			ID: "consume", Tool: "consumer_tool",
			Arguments: map[string]interface{}{
				"payload": map[string]interface{}{
					"layers": []map[string]interface{}{
						{"value": "{{provide_a.output.value}}"},
						{"nested": []interface{}{map[string]interface{}{"value": "{{provide_b.output.value}}"}}},
					},
				},
			},
		},
		{ID: "provide_a", Tool: "provider_a"},
		{ID: "provide_b", Tool: "provider_b"},
	}}
	compiler := NewPlanCompiler(catalog)
	prepared := compiler.PreparePlan(plan)
	gotOrder := make([]string, 0, len(prepared.Steps))
	for _, step := range prepared.Steps {
		gotOrder = append(gotOrder, step.ID)
	}
	if want := []string{"provide_a", "provide_b", "consume"}; !reflect.DeepEqual(gotOrder, want) {
		t.Fatalf("nested reference topology = %#v, want %#v", gotOrder, want)
	}
	consumer := findPlanStep(prepared, "consume")
	if !reflect.DeepEqual(consumer.DependsOn, []string{"provide_a", "provide_b"}) {
		t.Fatalf("nested reference dependencies = %#v, want both providers", consumer.DependsOn)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("nested known reference plan failed Guard: %v", err)
	}
	first := cloneContractPlanDeep(prepared)
	if preparedAgain := compiler.PreparePlan(prepared); !reflect.DeepEqual(preparedAgain, first) {
		t.Fatalf("nested reference PreparePlan is not idempotent:\nfirst=%#v\nsecond=%#v", first.Steps, preparedAgain.Steps)
	}
}

func TestPlanGuardRejectsNestedUnknownOrSelfReferenceInsideTypedArray(t *testing.T) {
	catalog := staticToolCatalog{
		"consumer_tool": {
			Name:       "consumer_tool",
			Parameters: map[string]tool.ParamDef{"payload": {Type: "object"}},
			Output:     map[string]tool.ParamDef{"value": {Type: "string"}},
		},
	}
	tests := []struct {
		name      string
		reference string
		want      string
	}{
		{name: "unknown", reference: "{{missing.output.value}}", want: "references unknown step missing in argument expression"},
		{name: "self", reference: "{{consume.output.value}}", want: "cannot depend on itself"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := NewPlanCompiler(catalog).PreparePlan(&AgentPlan{Goal: "nested invalid", Steps: []AgentStep{
				{
					ID: "consume", Tool: "consumer_tool",
					Arguments: map[string]interface{}{
						"payload": map[string]interface{}{
							"layers": []map[string]interface{}{{
								"items": []interface{}{map[string]interface{}{"value": tt.reference}},
							}},
						},
					},
				},
			}})
			err := NewPlanGuard(catalog, nil).Validate(plan)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("nested %s reference Guard error = %v, want diagnostic containing %q", tt.name, err, tt.want)
			}
		})
	}
}

func TestRemoveReferencesToRemovedStepsPrunesNestedObjectsAndArraysPrecisely(t *testing.T) {
	plan := &AgentPlan{Goal: "nested removed", Steps: []AgentStep{
		{ID: "keep", Tool: "provider_a", Arguments: map[string]interface{}{}},
		{
			ID: "consume", Tool: "consumer_tool", DependsOn: []string{"gone", "keep"},
			Arguments: map[string]interface{}{
				"payload": map[string]interface{}{
					"drop": "{{gone.output.value}}",
					"layers": []interface{}{
						"literal",
						"{{gone.output.value}}",
						map[string]interface{}{
							"drop":    "{{gone.output.value}}",
							"keep":    "{{keep.output.value}}",
							"unknown": "{{missing.output.value}}",
						},
					},
				},
			},
		},
	}}
	removeReferencesToRemovedSteps(plan, map[string]bool{"gone": true})
	consumer := findPlanStep(plan, "consume")
	if !reflect.DeepEqual(consumer.DependsOn, []string{"keep"}) {
		t.Fatalf("removed-step dependencies = %#v, want only keep", consumer.DependsOn)
	}
	payload := consumer.Arguments["payload"].(map[string]interface{})
	if _, exists := payload["drop"]; exists {
		t.Fatalf("nested removed map reference survived: %#v", payload)
	}
	layers := payload["layers"].([]interface{})
	if len(layers) != 2 || layers[0] != "literal" {
		t.Fatalf("nested removed array reference was not pruned precisely: %#v", layers)
	}
	nested := layers[1].(map[string]interface{})
	if _, exists := nested["drop"]; exists || nested["keep"] != "{{keep.output.value}}" || nested["unknown"] != "{{missing.output.value}}" {
		t.Fatalf("nested removed reference cleanup damaged legal references: %#v", nested)
	}
	first := cloneContractPlanDeep(plan)
	removeReferencesToRemovedSteps(plan, map[string]bool{"gone": true})
	if !reflect.DeepEqual(plan, first) {
		t.Fatalf("nested removed reference cleanup is not idempotent:\nfirst=%#v\nsecond=%#v", first.Steps, plan.Steps)
	}
}

func qualityGateForProduction(plan *AgentPlan, productionID string) AgentStep {
	if plan == nil {
		return AgentStep{}
	}
	for _, step := range plan.Steps {
		if step.Tool == "__quality_gate__" && step.Arguments["productionStep"] == productionID {
			return step
		}
	}
	return AgentStep{}
}

func cloneContractPlanDeep(plan *AgentPlan) *AgentPlan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.Steps = make([]AgentStep, len(plan.Steps))
	for i, step := range plan.Steps {
		cloned.Steps[i] = step
		if step.DependsOn != nil {
			cloned.Steps[i].DependsOn = append([]string{}, step.DependsOn...)
		}
		cloned.Steps[i].Arguments = copyMap(step.Arguments)
	}
	return &cloned
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
