package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const maxLLMToolNameLength = 64

var llmToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var portableStrictSchemaKeywords = map[string]struct{}{
	"type":                 {},
	"title":                {},
	"description":          {},
	"enum":                 {},
	"properties":           {},
	"required":             {},
	"additionalProperties": {},
	"items":                {},
}

// LLMToolDefinition is the provider-neutral contract used to expose a tool to
// an LLM. Its schemas remain standard JSON Schema maps; provider wire formats
// are derived through the adapters below.
type LLMToolDefinition struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"inputSchema"`
	OutputSchema map[string]interface{} `json:"outputSchema,omitempty"`
	Strict       bool                   `json:"strict,omitempty"`
	Annotations  map[string]interface{} `json:"annotations,omitempty"`
}

// Validate checks the provider-neutral invariants without narrowing or
// rewriting the canonical JSON Schemas.
func (definition LLMToolDefinition) Validate() error {
	if definition.Name == "" || definition.Name != strings.TrimSpace(definition.Name) {
		return fmt.Errorf("tool name must not be empty or contain surrounding whitespace")
	}
	if len(definition.Name) > maxLLMToolNameLength || !llmToolNamePattern.MatchString(definition.Name) {
		return fmt.Errorf("tool name %q is invalid", definition.Name)
	}
	if rootType, ok := definition.InputSchema["type"].(string); !ok || rootType != "object" {
		return fmt.Errorf("tool input schema root type must be object")
	}
	if _, err := json.Marshal(definition.InputSchema); err != nil {
		return fmt.Errorf("marshal tool input schema: %w", err)
	}
	if definition.OutputSchema != nil {
		if _, err := json.Marshal(definition.OutputSchema); err != nil {
			return fmt.Errorf("marshal tool output schema: %w", err)
		}
	}
	if definition.Annotations != nil {
		if _, err := json.Marshal(definition.Annotations); err != nil {
			return fmt.Errorf("marshal tool annotations: %w", err)
		}
	}
	if definition.Strict {
		if err := validatePortableStrictSchema(definition.InputSchema, "$"); err != nil {
			return fmt.Errorf("tool input schema is not portable in strict mode: %w", err)
		}
	}
	return nil
}

func validatePortableStrictSchema(schema map[string]interface{}, path string) error {
	for keyword := range schema {
		if _, ok := portableStrictSchemaKeywords[keyword]; !ok {
			return fmt.Errorf("keyword %q at %s is unsupported", keyword, path)
		}
	}

	typeName, ok := schema["type"].(string)
	if !ok || typeName == "" {
		return fmt.Errorf("type at %s must be a single JSON Schema type", path)
	}
	switch typeName {
	case "object":
		return validatePortableStrictObject(schema, path)
	case "array":
		items, ok := schema["items"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("array at %s must define one items schema", path)
		}
		return validatePortableStrictSchema(items, path+".items")
	case "string", "number", "integer", "boolean", "null":
		if _, exists := schema["properties"]; exists {
			return fmt.Errorf("properties at %s require type object", path)
		}
		if _, exists := schema["items"]; exists {
			return fmt.Errorf("items at %s require type array", path)
		}
		return nil
	default:
		return fmt.Errorf("type %q at %s is unsupported", typeName, path)
	}
}

func validatePortableStrictObject(schema map[string]interface{}, path string) error {
	if additionalProperties, ok := schema["additionalProperties"].(bool); !ok || additionalProperties {
		return fmt.Errorf("object at %s must set additionalProperties=false", path)
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("object at %s must define properties", path)
	}
	required := schemaRequiredSet(schema["required"])
	if len(properties) > 0 && len(required) == 0 {
		return fmt.Errorf("object at %s must list every property in required", path)
	}
	for name, rawProperty := range properties {
		if !required[name] {
			return fmt.Errorf("property %q at %s must be listed in required", name, path)
		}
		propertySchema, ok := rawProperty.(map[string]interface{})
		if !ok {
			return fmt.Errorf("property %q at %s must contain a schema object", name, path)
		}
		if err := validatePortableStrictSchema(propertySchema, path+".properties."+name); err != nil {
			return err
		}
	}
	for name := range required {
		if _, ok := properties[name]; !ok {
			return fmt.Errorf("required property %q at %s is not defined in properties", name, path)
		}
	}
	return nil
}

type OpenAIResponsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Strict      bool                   `json:"strict"`
}

type OpenAIChatCompletionsTool struct {
	Type     string                       `json:"type"`
	Function OpenAIChatFunctionDefinition `json:"function"`
}

type OpenAIChatFunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Strict      bool                   `json:"strict"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	Strict      bool                   `json:"strict"`
}

type GeminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

func ToOpenAIResponsesTool(definition LLMToolDefinition) (OpenAIResponsesTool, error) {
	parameters, err := validatedInputSchemaCopy(definition)
	if err != nil {
		return OpenAIResponsesTool{}, err
	}
	return OpenAIResponsesTool{
		Type:        "function",
		Name:        definition.Name,
		Description: definition.Description,
		Parameters:  parameters,
		Strict:      definition.Strict,
	}, nil
}

func ToOpenAIChatCompletionsTool(definition LLMToolDefinition) (OpenAIChatCompletionsTool, error) {
	parameters, err := validatedInputSchemaCopy(definition)
	if err != nil {
		return OpenAIChatCompletionsTool{}, err
	}
	return OpenAIChatCompletionsTool{
		Type: "function",
		Function: OpenAIChatFunctionDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  parameters,
			Strict:      definition.Strict,
		},
	}, nil
}

func ToAnthropicTool(definition LLMToolDefinition) (AnthropicTool, error) {
	inputSchema, err := validatedInputSchemaCopy(definition)
	if err != nil {
		return AnthropicTool{}, err
	}
	return AnthropicTool{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: inputSchema,
		Strict:      definition.Strict,
	}, nil
}

func ToGeminiFunctionDeclaration(definition LLMToolDefinition) (GeminiFunctionDeclaration, error) {
	parameters, err := validatedInputSchemaCopy(definition)
	if err != nil {
		return GeminiFunctionDeclaration{}, err
	}
	return GeminiFunctionDeclaration{
		Name:        definition.Name,
		Description: definition.Description,
		Parameters:  parameters,
	}, nil
}

func validatedInputSchemaCopy(definition LLMToolDefinition) (map[string]interface{}, error) {
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	return deepCopyJSONSchema(definition.InputSchema)
}

func deepCopyJSONSchema(schema map[string]interface{}) (map[string]interface{}, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON Schema: %w", err)
	}
	var copied map[string]interface{}
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return nil, fmt.Errorf("unmarshal JSON Schema copy: %w", err)
	}
	return copied, nil
}

func cloneJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(schema))
	for key, value := range schema {
		cloned[key] = cloneJSONSchemaValue(value)
	}
	return cloned
}

func cloneJSONSchemaValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneJSONSchema(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONSchemaValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}
