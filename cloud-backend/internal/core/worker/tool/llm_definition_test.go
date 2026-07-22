package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLLMToolAdaptersPreserveCanonicalSchemaAndProviderShapes(t *testing.T) {
	inputSchema := nestedLLMToolTestSchema()
	definition := LLMToolDefinition{
		Name:        "media_render-video-1",
		Description: "Render a video from structured scenes",
		InputSchema: inputSchema,
		OutputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"videoUrl": map[string]interface{}{"type": "string", "format": "uri"},
			},
			"required":             []interface{}{"videoUrl"},
			"additionalProperties": false,
		},
		Strict: false,
		Annotations: map[string]interface{}{
			"readOnlyHint": false,
		},
	}

	if err := definition.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	responsesTool, err := ToOpenAIResponsesTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIResponsesTool() error = %v", err)
	}
	if responsesTool.Type != "function" || responsesTool.Name != definition.Name || responsesTool.Description != definition.Description || responsesTool.Strict {
		t.Fatalf("unexpected OpenAI Responses shape: %#v", responsesTool)
	}
	assertSchemaEqual(t, responsesTool.Parameters, inputSchema)
	assertJSONKeys(t, responsesTool, "type", "name", "description", "parameters", "strict")

	chatTool, err := ToOpenAIChatCompletionsTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIChatCompletionsTool() error = %v", err)
	}
	if chatTool.Type != "function" || chatTool.Function.Name != definition.Name || chatTool.Function.Description != definition.Description || chatTool.Function.Strict {
		t.Fatalf("unexpected OpenAI Chat Completions shape: %#v", chatTool)
	}
	assertSchemaEqual(t, chatTool.Function.Parameters, inputSchema)
	chatJSON := assertJSONKeys(t, chatTool, "type", "function")
	var chatFunction map[string]json.RawMessage
	if err := json.Unmarshal(chatJSON["function"], &chatFunction); err != nil {
		t.Fatalf("unmarshal OpenAI Chat function: %v", err)
	}
	assertRawJSONKeys(t, chatFunction, "name", "description", "parameters", "strict")

	anthropicTool, err := ToAnthropicTool(definition)
	if err != nil {
		t.Fatalf("ToAnthropicTool() error = %v", err)
	}
	if anthropicTool.Name != definition.Name || anthropicTool.Description != definition.Description || anthropicTool.Strict {
		t.Fatalf("unexpected Anthropic shape: %#v", anthropicTool)
	}
	assertSchemaEqual(t, anthropicTool.InputSchema, inputSchema)
	assertJSONKeys(t, anthropicTool, "name", "description", "input_schema", "strict")

	geminiDeclaration, err := ToGeminiFunctionDeclaration(definition)
	if err != nil {
		t.Fatalf("ToGeminiFunctionDeclaration() error = %v", err)
	}
	if geminiDeclaration.Name != definition.Name || geminiDeclaration.Description != definition.Description {
		t.Fatalf("unexpected Gemini shape: %#v", geminiDeclaration)
	}
	assertSchemaEqual(t, geminiDeclaration.Parameters, inputSchema)
	assertJSONKeys(t, geminiDeclaration, "name", "description", "parameters")

	responsesTool.Parameters["type"] = "string"
	responsesTool.Parameters["properties"].(map[string]interface{})["scenes"].(map[string]interface{})["items"].(map[string]interface{})["$ref"] = "#/$defs/changed"
	chatTool.Function.Parameters["additionalProperties"] = true
	anthropicTool.InputSchema["required"].([]interface{})[0] = "changed"
	geminiDeclaration.Parameters["$defs"].(map[string]interface{})["scene"] = map[string]interface{}{"type": "string"}
	assertSchemaEqual(t, definition.InputSchema, nestedLLMToolTestSchema())
}

func TestLLMToolAdaptersAcceptValidStrictSchemaWithoutMutation(t *testing.T) {
	inputSchema := strictLLMToolTestSchema()
	definition := LLMToolDefinition{
		Name:        "strict_tool-1",
		Description: "Strict portable tool",
		InputSchema: inputSchema,
		Strict:      true,
	}

	responsesTool, err := ToOpenAIResponsesTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIResponsesTool() error = %v", err)
	}
	chatTool, err := ToOpenAIChatCompletionsTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIChatCompletionsTool() error = %v", err)
	}
	anthropicTool, err := ToAnthropicTool(definition)
	if err != nil {
		t.Fatalf("ToAnthropicTool() error = %v", err)
	}
	geminiDeclaration, err := ToGeminiFunctionDeclaration(definition)
	if err != nil {
		t.Fatalf("ToGeminiFunctionDeclaration() error = %v", err)
	}

	if !responsesTool.Strict || !chatTool.Function.Strict || !anthropicTool.Strict {
		t.Fatalf("explicit strict mode was not preserved: responses=%t chat=%t anthropic=%t", responsesTool.Strict, chatTool.Function.Strict, anthropicTool.Strict)
	}
	assertSchemaEqual(t, responsesTool.Parameters, inputSchema)
	assertSchemaEqual(t, chatTool.Function.Parameters, inputSchema)
	assertSchemaEqual(t, anthropicTool.InputSchema, inputSchema)
	assertSchemaEqual(t, geminiDeclaration.Parameters, inputSchema)
	assertSchemaEqual(t, definition.InputSchema, strictLLMToolTestSchema())
}

func TestLLMToolDefinitionRejectsNonPortableStrictSchemas(t *testing.T) {
	tests := []struct {
		name        string
		schema      map[string]interface{}
		wantMessage string
	}{
		{
			name: "missing additional properties",
			schema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
				"required":   []interface{}{"name"},
			},
			wantMessage: "additionalProperties=false",
		},
		{
			name: "nested object missing required property",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"child": map[string]interface{}{
						"type":                 "object",
						"properties":           map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
						"required":             []interface{}{},
						"additionalProperties": false,
					},
				},
				"required":             []interface{}{"child"},
				"additionalProperties": false,
			},
			wantMessage: "required",
		},
		{
			name: "unsupported keyword",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
				"oneOf": []interface{}{
					map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "required": []interface{}{}, "additionalProperties": false},
				},
			},
			wantMessage: "oneOf",
		},
		{
			name: "type must be one string",
			schema: map[string]interface{}{
				"type":                 []interface{}{"object", "null"},
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "single JSON Schema type",
		},
		{
			name: "title must be string",
			schema: map[string]interface{}{
				"type":                 "object",
				"title":                42,
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "title",
		},
		{
			name: "description must be string",
			schema: map[string]interface{}{
				"type":                 "object",
				"description":          true,
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "description",
		},
		{
			name: "enum must be array",
			schema: map[string]interface{}{
				"type":                 "object",
				"enum":                 "value",
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "enum",
		},
		{
			name: "enum must not be empty",
			schema: map[string]interface{}{
				"type":                 "object",
				"enum":                 []interface{}{},
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "enum",
		},
		{
			name: "properties must be schema map",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           []interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
			},
			wantMessage: "properties",
		},
		{
			name: "required must be string array",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"required":             "name",
				"additionalProperties": false,
			},
			wantMessage: "required",
		},
		{
			name: "required must not contain duplicates",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
				"required":             []interface{}{"name", "name"},
				"additionalProperties": false,
			},
			wantMessage: "duplicate",
		},
		{
			name: "required items must be strings",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
				"required":             []interface{}{"name", 7},
				"additionalProperties": false,
			},
			wantMessage: "must be a string",
		},
		{
			name: "additional properties must be false boolean",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": "false",
			},
			wantMessage: "additionalProperties=false",
		},
		{
			name: "items must be schema map",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"values": map[string]interface{}{"type": "array", "items": "string"}},
				"required":             []interface{}{"values"},
				"additionalProperties": false,
			},
			wantMessage: "items",
		},
		{
			name: "properties only apply to objects",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"value": map[string]interface{}{"type": "string", "properties": map[string]interface{}{}}},
				"required":             []interface{}{"value"},
				"additionalProperties": false,
			},
			wantMessage: "properties",
		},
		{
			name: "required only applies to objects",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"value": map[string]interface{}{"type": "string", "required": []interface{}{}}},
				"required":             []interface{}{"value"},
				"additionalProperties": false,
			},
			wantMessage: "required",
		},
		{
			name: "additional properties only apply to objects",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"value": map[string]interface{}{"type": "string", "additionalProperties": false}},
				"required":             []interface{}{"value"},
				"additionalProperties": false,
			},
			wantMessage: "additionalProperties",
		},
		{
			name: "items only apply to arrays",
			schema: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{},
				"required":             []interface{}{},
				"additionalProperties": false,
				"items":                map[string]interface{}{"type": "string"},
			},
			wantMessage: "items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := LLMToolDefinition{Name: "strict_tool", InputSchema: tt.schema, Strict: true}
			_, err := ToOpenAIResponsesTool(definition)
			if err == nil {
				t.Fatal("ToOpenAIResponsesTool() error = nil, want strict schema validation error")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %q, want message containing %q", err, tt.wantMessage)
			}
		})
	}
}

func TestLLMToolAdaptersDoNotEnableStrictByDefault(t *testing.T) {
	definition := LLMToolDefinition{
		Name: "safe_tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"optional": map[string]interface{}{"type": "string"},
			},
		},
	}

	responsesTool, err := ToOpenAIResponsesTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIResponsesTool() error = %v", err)
	}
	chatTool, err := ToOpenAIChatCompletionsTool(definition)
	if err != nil {
		t.Fatalf("ToOpenAIChatCompletionsTool() error = %v", err)
	}
	anthropicTool, err := ToAnthropicTool(definition)
	if err != nil {
		t.Fatalf("ToAnthropicTool() error = %v", err)
	}

	if responsesTool.Strict || chatTool.Function.Strict || anthropicTool.Strict {
		t.Fatalf("strict mode must remain disabled unless explicitly requested: responses=%t chat=%t anthropic=%t", responsesTool.Strict, chatTool.Function.Strict, anthropicTool.Strict)
	}
	assertSchemaEqual(t, responsesTool.Parameters, definition.InputSchema)
	assertSchemaEqual(t, chatTool.Function.Parameters, definition.InputSchema)
	assertSchemaEqual(t, anthropicTool.InputSchema, definition.InputSchema)
}

func TestLLMToolDefinitionValidateRejectsInvalidNameAndInputRoot(t *testing.T) {
	tests := []struct {
		name       string
		definition LLMToolDefinition
	}{
		{
			name:       "empty name",
			definition: LLMToolDefinition{InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "invalid name",
			definition: LLMToolDefinition{Name: "bad tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "dot is not portable",
			definition: LLMToolDefinition{Name: "bad.tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "colon is not portable",
			definition: LLMToolDefinition{Name: "bad:tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "digit cannot be first",
			definition: LLMToolDefinition{Name: "1bad_tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "hyphen cannot be first",
			definition: LLMToolDefinition{Name: "-bad_tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "longer than 64 characters",
			definition: LLMToolDefinition{Name: strings.Repeat("a", 65), InputSchema: map[string]interface{}{"type": "object"}},
		},
		{
			name:       "missing root type",
			definition: LLMToolDefinition{Name: "valid_tool", InputSchema: map[string]interface{}{"properties": map[string]interface{}{}}},
		},
		{
			name:       "non-object root",
			definition: LLMToolDefinition{Name: "valid_tool", InputSchema: map[string]interface{}{"type": "string"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.definition.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

func TestLLMToolDefinitionValidateAccepts64CharacterPortableName(t *testing.T) {
	definition := LLMToolDefinition{
		Name:        "_" + strings.Repeat("a", 61) + "-_",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	if err := definition.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func nestedLLMToolTestSchema() map[string]interface{} {
	return map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"properties": map[string]interface{}{
			"scenes": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"$ref": "#/$defs/scene"},
			},
			"callback": map[string]interface{}{
				"type": []interface{}{"string", "null"},
				"enum": []interface{}{"brief", "verbose", nil},
			},
		},
		"required": []interface{}{"scenes"},
		"$defs": map[string]interface{}{
			"scene": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"oneOf": []interface{}{
							map[string]interface{}{"type": "string", "format": "uri"},
							map[string]interface{}{"type": "object", "additionalProperties": true},
						},
					},
				},
				"required":             []interface{}{"source"},
				"additionalProperties": false,
			},
		},
		"additionalProperties": false,
	}
}

func strictLLMToolTestSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"title":       "Strict tool input",
		"description": "Portable strict schema",
		"properties": map[string]interface{}{
			"title": map[string]interface{}{"type": "string", "enum": []interface{}{"short", "long"}},
			"scenes": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "string"},
					},
					"required":             []interface{}{"id"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []interface{}{"title", "scenes"},
		"additionalProperties": false,
	}
}

func assertSchemaEqual(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func assertJSONKeys(t *testing.T, value interface{}, want ...string) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal provider tool: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal provider tool: %v", err)
	}
	assertRawJSONKeys(t, fields, want...)
	return fields
}

func assertRawJSONKeys(t *testing.T, fields map[string]json.RawMessage, want ...string) {
	t.Helper()
	if len(fields) != len(want) {
		t.Fatalf("JSON fields = %v, want exactly %v", reflect.ValueOf(fields).MapKeys(), want)
	}
	for _, name := range want {
		if _, ok := fields[name]; !ok {
			t.Fatalf("JSON field %q missing from %v", name, reflect.ValueOf(fields).MapKeys())
		}
	}
}
