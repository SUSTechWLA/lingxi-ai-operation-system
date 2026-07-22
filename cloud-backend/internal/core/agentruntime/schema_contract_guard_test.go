package agentruntime

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestPlanGuardUsesCanonicalInputSchemaForNestedLiterals(t *testing.T) {
	catalog := staticToolCatalog{"canonical_consumer": {
		Name: "canonical_consumer",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{"request": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{"type": "string", "enum": []interface{}{"fast", "quality"}},
					"ids":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				},
				"required": []interface{}{"mode", "ids"}, "additionalProperties": false,
			}},
			"required": []interface{}{"request"}, "additionalProperties": false,
		},
		// Deliberately contradictory legacy projection: canonical is authoritative.
		Parameters: map[string]tool.ParamDef{"legacy": {Type: "string", Required: true}},
	}}

	valid := &AgentPlan{Steps: []AgentStep{{ID: "consume", Tool: "canonical_consumer", Arguments: map[string]interface{}{
		"request": map[string]interface{}{"mode": "quality", "ids": []interface{}{"a", "b"}},
	}}}}
	if err := NewPlanGuard(catalog, nil).Validate(valid); err != nil {
		t.Fatalf("canonical-only valid plan rejected: %v", err)
	}

	invalid := &AgentPlan{Steps: []AgentStep{{ID: "consume", Tool: "canonical_consumer", Arguments: map[string]interface{}{
		"request": map[string]interface{}{"mode": "unknown", "ids": []interface{}{"a"}, "extra": true},
	}}}}
	if err := NewPlanGuard(catalog, nil).Validate(invalid); err == nil || !strings.Contains(err.Error(), "input schema") {
		t.Fatalf("invalid canonical nested literal error = %v", err)
	}
}

func TestPlanGuardUsesCanonicalOutputFieldsAndChecksReferenceTypeCompatibility(t *testing.T) {
	catalog := staticToolCatalog{
		"producer": {
			Name: "producer",
			OutputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"assetId": map[string]interface{}{"type": "string"},
					"scores":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "number"}},
				},
			},
			Output: map[string]tool.ParamDef{"legacyOnly": {Type: "string"}},
		},
		"consumer": {
			Name: "consumer",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
				"required":   []interface{}{"assetId"}, "additionalProperties": false,
			},
		},
	}

	valid := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
		{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{"assetId": "{{produce.output.assetId}}"}},
	}}
	if err := NewPlanGuard(catalog, nil).Validate(valid); err != nil {
		t.Fatalf("compatible canonical reference rejected: %v", err)
	}

	for name, ref := range map[string]string{
		"canonical field type mismatch": "{{produce.output.scores}}",
		"legacy field is not canonical": "{{produce.output.legacyOnly}}",
	} {
		t.Run(name, func(t *testing.T) {
			plan := &AgentPlan{Steps: []AgentStep{
				{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
				{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{"assetId": ref}},
			}}
			err := NewPlanGuard(catalog, nil).Validate(plan)
			if err == nil {
				t.Fatalf("reference %s unexpectedly passed", ref)
			}
		})
	}
}

func TestPlanGuardCanonicalSchemaValidatesReferencesInsideArrays(t *testing.T) {
	catalog := staticToolCatalog{
		"producer": {Name: "producer", OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
		}},
		"consumer": {Name: "consumer", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"requests": map[string]interface{}{
				"type": "array", "items": map[string]interface{}{
					"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
					"required": []interface{}{"id"}, "additionalProperties": false,
				},
			}}, "required": []interface{}{"requests"}, "additionalProperties": false,
		}},
	}
	plan := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
		{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{
			"requests": []interface{}{map[string]interface{}{"id": "{{produce.output.assetId}}"}},
		}},
	}}
	if err := NewPlanGuard(catalog, nil).Validate(plan); err != nil {
		t.Fatalf("nested array reference rejected: %v", err)
	}
}

func TestPlanGuardReferenceNeutralizationDoesNotWeakenSharedLocalRef(t *testing.T) {
	catalog := staticToolCatalog{
		"producer": {Name: "producer", OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
		}},
		"consumer": {Name: "consumer", InputSchema: map[string]interface{}{
			"$defs": map[string]interface{}{"identifier": map[string]interface{}{"type": "string"}},
			"type":  "object",
			"properties": map[string]interface{}{
				"primary":   map[string]interface{}{"$ref": "#/$defs/identifier"},
				"secondary": map[string]interface{}{"$ref": "#/$defs/identifier"},
			},
			"required": []interface{}{"primary", "secondary"}, "additionalProperties": false,
		}},
	}
	valid := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
		{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{
			"primary": "{{produce.output.assetId}}", "secondary": "literal",
		}},
	}}
	if err := NewPlanGuard(catalog, nil).Validate(valid); err != nil {
		t.Fatalf("reference through local $ref rejected: %v", err)
	}
	invalid := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
		{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{
			"primary": "{{produce.output.assetId}}", "secondary": float64(42),
		}},
	}}
	if err := NewPlanGuard(catalog, nil).Validate(invalid); err == nil {
		t.Fatal("neutralizing primary reference weakened secondary shared $ref")
	}
}

func TestPlanGuardRejectsSourceTypeUnionWiderThanTarget(t *testing.T) {
	catalog := staticToolCatalog{
		"producer": {Name: "producer", OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"value": map[string]interface{}{"type": []interface{}{"string", "number"}}},
		}},
		"consumer": {Name: "consumer", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"value": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"value"}, "additionalProperties": false,
		}},
	}
	plan := &AgentPlan{Steps: []AgentStep{
		{ID: "produce", Tool: "producer", Arguments: map[string]interface{}{}},
		{ID: "consume", Tool: "consumer", DependsOn: []string{"produce"}, Arguments: map[string]interface{}{"value": "{{produce.output.value}}"}},
	}}
	if err := NewPlanGuard(catalog, nil).Validate(plan); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("wider source union error = %v", err)
	}
}
