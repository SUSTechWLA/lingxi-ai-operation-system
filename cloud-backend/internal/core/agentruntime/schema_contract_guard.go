package agentruntime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type contractArgumentReference struct {
	argumentReference
	Path []interface{}
}

func contractArgumentReferences(value interface{}) []contractArgumentReference {
	references := make([]contractArgumentReference, 0)
	walkContractArgumentReferences(reflect.ValueOf(value), nil, &references)
	return references
}

func walkContractArgumentReferences(value reflect.Value, path []interface{}, references *[]contractArgumentReference) {
	if !value.IsValid() {
		return
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		expression := value.String()
		stepID, field, ok := outputReference(expression)
		if ok {
			*references = append(*references, contractArgumentReference{
				argumentReference: argumentReference{StepID: stepID, Field: field, Expression: expression},
				Path:              append([]interface{}(nil), path...),
			})
		}
	case reflect.Map:
		keys := value.MapKeys()
		sort.SliceStable(keys, func(i, j int) bool { return argumentMapKey(keys[i]) < argumentMapKey(keys[j]) })
		for _, key := range keys {
			walkContractArgumentReferences(value.MapIndex(key), append(path, argumentMapKey(key)), references)
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			walkContractArgumentReferences(value.Index(index), append(path, index), references)
		}
	}
}

func validateCanonicalStepInput(step AgentStep, manifest *tool.ToolManifest) error {
	if manifest == nil || manifest.InputSchema == nil {
		return nil
	}
	schema, err := cloneSchemaMap(manifest.InputSchema)
	if err != nil {
		return fmt.Errorf("agent step %s input schema invalid for tool %s: %w", step.ID, step.Tool, err)
	}
	arguments := cloneArgumentValue(step.Arguments)
	for _, reference := range contractArgumentReferences(step.Arguments) {
		neutralizeSchemaPath(schema, reference.Path)
		arguments = replaceArgumentPath(arguments, reference.Path, nil)
	}
	copyManifest := *manifest
	copyManifest.InputSchema = schema
	if err := tool.ValidateManifestInput(&copyManifest, arguments); err != nil {
		return fmt.Errorf("agent step %s input schema invalid for tool %s: %w", step.ID, step.Tool, err)
	}
	return nil
}

func cloneSchemaMap(schema map[string]interface{}) (map[string]interface{}, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func cloneArgumentValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			cloned[key] = cloneArgumentValue(child)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneArgumentValue(child)
		}
		return cloned
	default:
		value := reflect.ValueOf(value)
		if value.IsValid() && (value.Kind() == reflect.Slice || value.Kind() == reflect.Array) {
			cloned := make([]interface{}, value.Len())
			for index := range cloned {
				cloned[index] = cloneArgumentValue(value.Index(index).Interface())
			}
			return cloned
		}
		return typed
	}
}

func replaceArgumentPath(value interface{}, path []interface{}, replacement interface{}) interface{} {
	if len(path) == 0 {
		return replacement
	}
	switch token := path[0].(type) {
	case string:
		object, ok := value.(map[string]interface{})
		if ok {
			object[token] = replaceArgumentPath(object[token], path[1:], replacement)
		}
	case int:
		array, ok := value.([]interface{})
		if ok && token >= 0 && token < len(array) {
			array[token] = replaceArgumentPath(array[token], path[1:], replacement)
		}
	}
	return value
}

func neutralizeSchemaPath(root map[string]interface{}, path []interface{}) {
	if len(path) == 0 {
		return
	}
	current := root
	for index, token := range path {
		last := index == len(path)-1
		switch token := token.(type) {
		case string:
			properties, _ := current["properties"].(map[string]interface{})
			if properties == nil {
				return
			}
			if last {
				properties[token] = map[string]interface{}{}
				return
			}
			next, _ := properties[token].(map[string]interface{})
			if next == nil {
				return
			}
			current = next
		case int:
			if last {
				current["items"] = map[string]interface{}{}
				return
			}
			next, _ := current["items"].(map[string]interface{})
			if next == nil {
				return
			}
			current = next
		}
	}
}

func canonicalOutputFieldSchema(manifest *tool.ToolManifest, field string) (map[string]interface{}, bool) {
	if manifest == nil {
		return nil, false
	}
	if manifest.OutputSchema != nil {
		path := make([]interface{}, 0)
		for _, segment := range strings.Split(field, ".") {
			path = append(path, segment)
		}
		return schemaAtPath(manifest.OutputSchema, manifest.OutputSchema, path)
	}
	legacy, ok := manifest.Output[field]
	if !ok {
		return nil, false
	}
	return map[string]interface{}{"type": legacy.Type}, true
}

func canonicalInputPathSchema(manifest *tool.ToolManifest, path []interface{}) (map[string]interface{}, bool) {
	if manifest == nil || manifest.InputSchema == nil {
		return nil, false
	}
	return schemaAtPath(manifest.InputSchema, manifest.InputSchema, path)
}

func schemaAtPath(root, schema map[string]interface{}, path []interface{}) (map[string]interface{}, bool) {
	current := resolveLocalSchemaRef(root, schema)
	if current == nil {
		return nil, false
	}
	for _, token := range path {
		current = resolveLocalSchemaRef(root, current)
		switch token := token.(type) {
		case string:
			properties, _ := current["properties"].(map[string]interface{})
			next, _ := properties[token].(map[string]interface{})
			if next == nil {
				return nil, false
			}
			current = next
		case int:
			next, _ := current["items"].(map[string]interface{})
			if next == nil {
				return nil, false
			}
			current = next
		}
	}
	current = resolveLocalSchemaRef(root, current)
	return current, current != nil
}

func resolveLocalSchemaRef(root, schema map[string]interface{}) map[string]interface{} {
	ref, _ := schema["$ref"].(string)
	if !strings.HasPrefix(ref, "#/") {
		return schema
	}
	var current interface{} = root
	for _, raw := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = object[segment]
	}
	resolved, _ := current.(map[string]interface{})
	return resolved
}

func schemaTypes(schema map[string]interface{}) map[string]bool {
	types := map[string]bool{}
	if schema == nil {
		return types
	}
	switch value := schema["type"].(type) {
	case string:
		types[value] = true
	case []interface{}:
		for _, item := range value {
			if name, ok := item.(string); ok {
				types[name] = true
			}
		}
	case []string:
		for _, name := range value {
			types[name] = true
		}
	}
	if len(types) == 0 {
		if value, exists := schema["const"]; exists {
			if inferred := jsonValueSchemaType(value); inferred != "" {
				types[inferred] = true
			}
		}
		if enum, ok := schema["enum"].([]interface{}); ok {
			for _, value := range enum {
				if inferred := jsonValueSchemaType(value); inferred != "" {
					types[inferred] = true
				}
			}
		}
	}
	return types
}

func jsonValueSchemaType(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if typed == float64(int64(typed)) {
			return "integer"
		}
		return "number"
	case json.Number:
		if _, err := typed.Int64(); err == nil {
			return "integer"
		}
		return "number"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return ""
	}
}

func referenceTypesCompatible(source, target map[string]interface{}) bool {
	sourceTypes := schemaTypes(source)
	targetTypes := schemaTypes(target)
	if len(sourceTypes) == 0 || len(targetTypes) == 0 {
		return true
	}
	for sourceType := range sourceTypes {
		if !targetTypes[sourceType] && !(sourceType == "integer" && targetTypes["number"]) {
			return false
		}
	}
	return true
}
