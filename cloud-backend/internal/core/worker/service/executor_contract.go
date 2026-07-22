package service

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

const (
	inputReferenceUnresolvedCode = "INPUT_REFERENCE_UNRESOLVED"
	inputSchemaInvalidCode       = "INPUT_SCHEMA_INVALID"
	outputSchemaInvalidCode      = "OUTPUT_SCHEMA_INVALID"
)

func (ne *NodeExecutor) executionContractManifest(toolName string, parameters map[string]interface{}, fallback *tool.ToolManifest) *tool.ToolManifest {
	if ne == nil || ne.toolRegistry == nil {
		return fallback
	}
	if toolName == "external" {
		for _, key := range []string{"tool", "capabilityTool", "logicalToolName"} {
			if delegated, ok := parameters[key].(string); ok && strings.TrimSpace(delegated) != "" {
				if manifest := ne.toolRegistry.GetManifest(strings.TrimSpace(delegated)); manifest != nil {
					return manifest
				}
			}
		}
	}
	if fallback != nil {
		return fallback
	}
	return ne.toolRegistry.GetManifest(toolName)
}

func executionContractArguments(payload, parameters map[string]interface{}, manifest *tool.ToolManifest) map[string]interface{} {
	if payload != nil {
		if arguments, ok := payload["contractArguments"].(map[string]interface{}); ok {
			return cloneExecutionMap(arguments)
		}
	}
	for _, key := range []string{"arguments", "input"} {
		if arguments, ok := parameters[key].(map[string]interface{}); ok {
			return cloneExecutionMap(arguments)
		}
	}
	arguments := make(map[string]interface{}, len(parameters))
	for key, value := range parameters {
		if routeOnlyExecutionField(key) && !manifestDeclaresInputProperty(manifest, key) {
			continue
		}
		arguments[key] = value
	}
	return arguments
}

func routeOnlyExecutionField(key string) bool {
	switch key {
	case "tool", "capabilityTool", "intent", "expectedOutput", "produceArtifact",
		"localCommand", "providerId", "toolName", "logicalToolName", "timeout", "timeoutSec",
		"artifactPolicy", "skillPackageId", "promptRef", "resourceRefs", "endpoint":
		return true
	default:
		return false
	}
}

func manifestDeclaresInputProperty(manifest *tool.ToolManifest, name string) bool {
	if manifest == nil || manifest.InputSchema == nil {
		return false
	}
	properties, _ := manifest.InputSchema["properties"].(map[string]interface{})
	_, exists := properties[name]
	return exists
}

func cloneExecutionMap(input map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(input))
	for key, value := range input {
		cloned[key] = cloneExecutionValue(value)
	}
	return cloned
}

func cloneExecutionValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneExecutionMap(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneExecutionValue(child)
		}
		return cloned
	default:
		return typed
	}
}

func containsExactNodeReference(value interface{}) bool {
	return walkExactNodeReference(reflect.ValueOf(value))
}

func walkExactNodeReference(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		text := strings.TrimSpace(value.String())
		return nodeRefPattern.FindString(text) == text && strings.HasPrefix(text, "{{") && strings.HasSuffix(text, "}}")
	case reflect.Map:
		for _, key := range value.MapKeys() {
			if walkExactNodeReference(value.MapIndex(key)) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if walkExactNodeReference(value.Index(index)) {
				return true
			}
		}
	}
	return false
}

func validateExecutionInput(manifest *tool.ToolManifest, arguments map[string]interface{}) error {
	if containsExactNodeReference(arguments) {
		return fmt.Errorf("%s: exact output reference remains unresolved", inputReferenceUnresolvedCode)
	}
	if err := tool.ValidateManifestInput(manifest, arguments); err != nil {
		return fmt.Errorf("%s: %w", inputSchemaInvalidCode, err)
	}
	return nil
}
