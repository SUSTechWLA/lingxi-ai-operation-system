package localrunner

import (
	"fmt"
	"strings"
)

const (
	maxMCPToolSchemaDepth = 32
	maxMCPToolSchemaNodes = 2048
)

func validateMCPJSONSchema(label string, schema map[string]interface{}) error {
	if schema == nil {
		return fmt.Errorf("%s is required", label)
	}
	if schema["type"] != "object" {
		return fmt.Errorf("%s root type must be object", label)
	}
	nodes := 0
	if err := validateSchemaValue(schema, 0, &nodes); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func validateSchemaValue(value interface{}, depth int, nodes *int) error {
	if depth > maxMCPToolSchemaDepth {
		return fmt.Errorf("schema depth exceeds %d", maxMCPToolSchemaDepth)
	}
	*nodes++
	if *nodes > maxMCPToolSchemaNodes {
		return fmt.Errorf("schema node count exceeds %d", maxMCPToolSchemaNodes)
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, refKeyword := range []string{"$ref", "$dynamicRef"} {
			if ref, exists := typed[refKeyword]; exists {
				refString, ok := ref.(string)
				if !ok || !strings.HasPrefix(refString, "#") {
					return fmt.Errorf("remote, empty, or non-string %s is not allowed", refKeyword)
				}
			}
		}
		if err := validateSchemaKeywords(typed); err != nil {
			return err
		}
		for _, child := range typed {
			if err := validateSchemaValue(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range typed {
			if err := validateSchemaValue(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case string, float64, bool, nil:
	default:
		return fmt.Errorf("schema contains non-JSON value %T", value)
	}
	return nil
}

func validateSchemaKeywords(schema map[string]interface{}) error {
	if value, exists := schema["type"]; exists {
		switch typed := value.(type) {
		case string:
			if !validJSONSchemaType(typed) {
				return fmt.Errorf("invalid schema type %q", typed)
			}
		case []interface{}:
			if len(typed) == 0 {
				return fmt.Errorf("schema type array is empty")
			}
			for _, item := range typed {
				name, ok := item.(string)
				if !ok || !validJSONSchemaType(name) {
					return fmt.Errorf("schema type array is invalid")
				}
			}
		default:
			return fmt.Errorf("schema type must be a string or string array")
		}
	}
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		if value, exists := schema[key]; exists {
			if _, ok := value.(map[string]interface{}); !ok {
				return fmt.Errorf("%s must be an object", key)
			}
		}
	}
	if value, exists := schema["required"]; exists {
		items, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("required must be a string array")
		}
		seen := map[string]struct{}{}
		for _, item := range items {
			name, ok := item.(string)
			if !ok || name == "" {
				return fmt.Errorf("required must be a string array")
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("required contains duplicate %q", name)
			}
			seen[name] = struct{}{}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if value, exists := schema[key]; exists {
			if _, ok := value.([]interface{}); !ok {
				return fmt.Errorf("%s must be an array", key)
			}
		}
	}
	for _, key := range []string{"additionalProperties", "unevaluatedProperties"} {
		if value, exists := schema[key]; exists {
			switch value.(type) {
			case bool, map[string]interface{}:
			default:
				return fmt.Errorf("%s must be a boolean or schema object", key)
			}
		}
	}
	return nil
}

func validJSONSchemaType(value string) bool {
	switch value {
	case "null", "boolean", "object", "array", "number", "integer", "string":
		return true
	default:
		return false
	}
}
