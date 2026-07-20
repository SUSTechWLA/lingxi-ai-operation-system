package apispec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type schemaReflectionChild struct {
	Name string `json:"name"`
}

type schemaReflectionFixture struct {
	Arbitrary map[string]interface{}  `json:"arbitrary"`
	Typed     map[string]int          `json:"typed"`
	Raw       json.RawMessage         `json:"raw"`
	Children  []schemaReflectionChild `json:"children"`
	Nullable  *schemaReflectionChild  `json:"nullable"`
	Optional  *schemaReflectionChild  `json:"optional,omitempty"`
}

func TestReflectPreservesJSONWireSemantics(t *testing.T) {
	schema := Reflect(schemaReflectionFixture{})
	if schema == nil {
		t.Fatal("nil schema")
	}
	if !reflect.DeepEqual(schema.Required, []string{"arbitrary", "typed", "raw", "children", "nullable"}) {
		t.Fatalf("required = %v", schema.Required)
	}

	arbitrary := inlineProperty(t, schema, "arbitrary")
	if arbitrary.Type != "object" || arbitrary.AdditionalProperties == nil || arbitrary.AdditionalProperties.Schema == nil || arbitrary.AdditionalProperties.Schema.Schema == nil {
		t.Fatalf("arbitrary map schema = %+v", arbitrary)
	}
	if got := arbitrary.AdditionalProperties.Schema.Schema; got.Type != "" || len(got.Properties) != 0 {
		t.Fatalf("interface{} map value must be arbitrary JSON, got %+v", got)
	}

	typed := inlineProperty(t, schema, "typed")
	if got := typed.AdditionalProperties.Schema.Schema.Type; got != "integer" {
		t.Fatalf("typed map value type = %q", got)
	}

	raw := inlineProperty(t, schema, "raw")
	if raw.Type != "" || raw.Format != "" {
		t.Fatalf("json.RawMessage must be arbitrary JSON, got %+v", raw)
	}

	children := inlineProperty(t, schema, "children")
	if children.Type != "array" || children.Items == nil || children.Items.Schema == nil || children.Items.Schema.Type != "object" {
		t.Fatalf("children schema = %+v", children)
	}

	nullable := inlineProperty(t, schema, "nullable")
	if !nullable.Nullable {
		t.Fatalf("required pointer must remain nullable: %+v", nullable)
	}
	optional := inlineProperty(t, schema, "optional")
	if !optional.Nullable {
		t.Fatalf("optional pointer must remain nullable: %+v", optional)
	}
}

func TestRenderTypeScriptPreservesMapsRefsArraysNullabilityAndRequired(t *testing.T) {
	spec := New("test", "1").
		Schema("Child", &Schema{Type: "object", Properties: map[string]*SchemaRef{
			"name": {Schema: StringSchema()},
		}, Required: []string{"name"}}).
		Schema("Complex", &Schema{Type: "object", Properties: map[string]*SchemaRef{
			"arbitrary":     {Schema: &Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: &Schema{}}}}},
			"typed":         {Schema: &Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: IntegerSchema()}}}},
			"children":      {Schema: &Schema{Type: "array", Items: &SchemaRef{Ref: "#/components/schemas/Child"}}},
			"byId":          {Schema: &Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Ref: "#/components/schemas/Child"}}}},
			"nullableMap":   {Schema: &Schema{Type: "object", Nullable: true, AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: IntegerSchema()}}}},
			"nullableChild": {Schema: &Schema{Ref: "#/components/schemas/Child", Nullable: true}},
			"nested": {Schema: &Schema{Type: "object", Properties: map[string]*SchemaRef{
				"requiredValue": {Schema: StringSchema()}, "optionalValue": {Schema: StringSchema()},
			}, Required: []string{"requiredValue"}}},
		}, Required: []string{"arbitrary", "typed", "children", "byId", "nullableMap", "nullableChild", "nested"}}).
		Build()

	first := string(RenderTypeScript(spec))
	second := string(RenderTypeScript(spec))
	if first != second {
		t.Fatal("TypeScript output is not deterministic")
	}
	for _, want := range []string{
		"arbitrary: Record<string, unknown>;",
		"typed: Record<string, number>;",
		"children: Child[];",
		"byId: Record<string, Child>;",
		"nullableMap: Record<string, number> | null;",
		"nullableChild: Child | null;",
		"nested: { optionalValue?: string; requiredValue: string };",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("generated TypeScript missing %q:\n%s", want, first)
		}
	}
}

func TestRenderTypeScriptEmitsNeverSiblingsForClosedOneOf(t *testing.T) {
	closed := false
	direct := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"mode":          {Schema: &Schema{Type: "string", Enum: []any{"direct"}}},
		"directContent": {Schema: StringSchema()},
	}, Required: []string{"mode", "directContent"}, AdditionalProperties: &AdditionalProperties{Allowed: &closed}}
	instruction := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"mode":        {Schema: &Schema{Type: "string", Enum: []any{"instruction"}}},
		"instruction": {Schema: StringSchema()},
	}, Required: []string{"mode", "instruction"}, AdditionalProperties: &AdditionalProperties{Allowed: &closed}}
	spec := New("test", "1").Schema("Mutation", &Schema{OneOf: []*SchemaRef{
		{Schema: direct}, {Schema: instruction},
	}}).Build()

	generated := string(RenderTypeScript(spec))
	want := "export type Mutation = { directContent: string; instruction?: never; mode: 'direct' } | { directContent?: never; instruction: string; mode: 'instruction' };"
	if !strings.Contains(generated, want) {
		t.Fatalf("generated TypeScript missing discriminated union %q:\n%s", want, generated)
	}
}

func TestRenderTypeScriptDoesNotStrengthenOpenOneOf(t *testing.T) {
	left := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"kind": {Schema: &Schema{Type: "string", Enum: []any{"left"}}}, "left": {Schema: StringSchema()},
	}, Required: []string{"kind", "left"}}
	right := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"kind": {Schema: &Schema{Type: "string", Enum: []any{"right"}}}, "right": {Schema: StringSchema()},
	}, Required: []string{"kind", "right"}}
	spec := New("test", "1").Schema("OpenUnion", &Schema{OneOf: []*SchemaRef{
		{Schema: left}, {Schema: right},
	}}).Build()

	generated := string(RenderTypeScript(spec))
	want := "export type OpenUnion = { kind: 'left'; left: string } | { kind: 'right'; right: string };"
	if !strings.Contains(generated, want) || strings.Contains(generated, "never") {
		t.Fatalf("open oneOf was strengthened beyond OpenAPI %q:\n%s", want, generated)
	}
}

func TestRenderTypeScriptAppliesNeverOnlyToClosedBranches(t *testing.T) {
	closed := false
	closedLeft := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"kind": {Schema: &Schema{Type: "string", Enum: []any{"left"}}}, "left": {Schema: StringSchema()},
	}, Required: []string{"kind", "left"}, AdditionalProperties: &AdditionalProperties{Allowed: &closed}}
	openRight := &Schema{Type: "object", Properties: map[string]*SchemaRef{
		"kind": {Schema: &Schema{Type: "string", Enum: []any{"right"}}}, "right": {Schema: StringSchema()},
	}, Required: []string{"kind", "right"}}
	spec := New("test", "1").Schema("MixedUnion", &Schema{OneOf: []*SchemaRef{
		{Schema: closedLeft}, {Schema: openRight},
	}}).Build()

	generated := string(RenderTypeScript(spec))
	want := "export type MixedUnion = { kind: 'left'; left: string; right?: never } | { kind: 'right'; right: string };"
	if !strings.Contains(generated, want) {
		t.Fatalf("closed/open oneOf exactness mismatch %q:\n%s", want, generated)
	}
}

func TestSchemaAdditionalPropertiesSupportsClosedAndTypedObjects(t *testing.T) {
	var closed Schema
	if err := json.Unmarshal([]byte(`{"type":"object","additionalProperties":false}`), &closed); err != nil {
		t.Fatalf("unmarshal closed object: %v", err)
	}
	closedJSON, err := json.Marshal(closed)
	if err != nil {
		t.Fatalf("marshal closed object: %v", err)
	}
	if !strings.Contains(string(closedJSON), `"additionalProperties":false`) {
		t.Fatalf("closed schema JSON = %s", closedJSON)
	}

	typedJSON, err := json.Marshal(&Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Schema: IntegerSchema()}}})
	if err != nil {
		t.Fatalf("marshal typed map: %v", err)
	}
	if !strings.Contains(string(typedJSON), `"additionalProperties":{"type":"integer"}`) {
		t.Fatalf("typed map schema JSON = %s", typedJSON)
	}
	refJSON, err := json.Marshal(&Schema{Type: "object", AdditionalProperties: &AdditionalProperties{Schema: &SchemaRef{Ref: "#/components/schemas/Child"}}})
	if err != nil {
		t.Fatalf("marshal ref map: %v", err)
	}
	if !strings.Contains(string(refJSON), `"additionalProperties":{"$ref":"#/components/schemas/Child"}`) {
		t.Fatalf("ref map schema JSON = %s", refJSON)
	}
}

func inlineProperty(t *testing.T, schema *Schema, name string) *Schema {
	t.Helper()
	ref := schema.Properties[name]
	if ref == nil || ref.Schema == nil {
		t.Fatalf("inline property %q missing", name)
	}
	return ref.Schema
}
