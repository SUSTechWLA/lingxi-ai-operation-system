package tool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const maxLLMToolNameLength = 128

var llmToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:-]*$`)

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
