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
	// Compile the unmodified canonical schema first so draft selection and all
	// standard reference resolution fail closed before runtime references are
	// deferred below.
	if _, err := tool.CompileCanonicalSchema(manifest.InputSchema); err != nil {
		return fmt.Errorf("agent step %s input schema invalid for tool %s: %w", step.ID, step.Tool, err)
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
	replacement, changed := neutralizeSchemaNode(root, root, path, map[string]bool{}, 0)
	if !changed {
		return
	}
	replacement, err := cloneSchemaMap(replacement)
	if err != nil {
		return
	}
	for key := range root {
		delete(root, key)
	}
	for key, value := range replacement {
		root[key] = value
	}
}

func neutralizeSchemaNode(root, schema map[string]interface{}, path []interface{}, resolving map[string]bool, depth int) (map[string]interface{}, bool) {
	if len(path) == 0 {
		return map[string]interface{}{}, true
	}
	if schema == nil || depth > 64 {
		return schema, false
	}

	if ref, _ := schema["$ref"].(string); strings.HasPrefix(ref, "#/") && !resolving[ref] {
		resolved := resolveLocalSchemaRef(root, schema)
		if resolved != nil {
			cloned, err := cloneSchemaMap(resolved)
			if err == nil {
				if len(schema) > 1 {
					siblings, siblingErr := cloneSchemaMap(schema)
					if siblingErr == nil {
						delete(siblings, "$ref")
						cloned = map[string]interface{}{"allOf": []interface{}{cloned, siblings}}
					}
				}
				resolving[ref] = true
				replacement, changed := neutralizeSchemaNode(root, cloned, path, resolving, depth+1)
				delete(resolving, ref)
				if changed {
					return replacement, true
				}
			}
		}
	}

	changed := false
	switch token := path[0].(type) {
	case string:
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			if child, ok := properties[token].(map[string]interface{}); ok {
				if replacement, childChanged := neutralizeSchemaNode(root, child, path[1:], resolving, depth+1); childChanged {
					properties[token] = replacement
					changed = true
				}
			}
		}
		if additional, ok := schema["additionalProperties"].(map[string]interface{}); ok {
			if replacement, childChanged := neutralizeSchemaNode(root, additional, path[1:], resolving, depth+1); childChanged {
				schema["additionalProperties"] = replacement
				changed = true
			}
		}
	case int:
		if prefixItems, ok := schema["prefixItems"].([]interface{}); ok && token >= 0 && token < len(prefixItems) {
			if child, ok := prefixItems[token].(map[string]interface{}); ok {
				if replacement, childChanged := neutralizeSchemaNode(root, child, path[1:], resolving, depth+1); childChanged {
					prefixItems[token] = replacement
					changed = true
				}
			}
		} else if tupleItems, ok := schema["items"].([]interface{}); ok && token >= 0 && token < len(tupleItems) {
			if child, ok := tupleItems[token].(map[string]interface{}); ok {
				if replacement, childChanged := neutralizeSchemaNode(root, child, path[1:], resolving, depth+1); childChanged {
					tupleItems[token] = replacement
					changed = true
				}
			}
		} else if child, ok := schema["items"].(map[string]interface{}); ok {
			if replacement, childChanged := neutralizeSchemaNode(root, child, path[1:], resolving, depth+1); childChanged {
				schema["items"] = replacement
				changed = true
			}
		}
	}

	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		branches, ok := schema[keyword].([]interface{})
		if !ok {
			continue
		}
		branchChanged := false
		for index, branch := range branches {
			child, ok := branch.(map[string]interface{})
			if !ok {
				continue
			}
			if replacement, childChanged := neutralizeSchemaNode(root, child, path, resolving, depth+1); childChanged {
				branches[index] = replacement
				branchChanged = true
			}
		}
		if branchChanged {
			changed = true
			// A runtime value can be the discriminator that makes exactly one
			// branch valid. Once that value is deferred, retaining oneOf would
			// create a false static failure when several neutralized branches pass.
			if keyword == "oneOf" {
				delete(schema, "oneOf")
				schema["anyOf"] = branches
			}
		}
	}
	return schema, changed
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
	candidates := schemaPathCandidates(root, schema, path, map[string]bool{}, 0)
	if len(candidates) == 0 {
		return nil, false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	branches := make([]interface{}, len(candidates))
	for index := range candidates {
		branches[index] = candidates[index]
	}
	return map[string]interface{}{"allOf": branches}, true
}

func schemaPathCandidates(root, schema map[string]interface{}, path []interface{}, resolving map[string]bool, depth int) []map[string]interface{} {
	if schema == nil || depth > 64 {
		return nil
	}
	if ref, _ := schema["$ref"].(string); strings.HasPrefix(ref, "#/") && !resolving[ref] {
		resolved := resolveLocalSchemaRef(root, schema)
		if resolved != nil {
			resolving[ref] = true
			candidates := schemaPathCandidates(root, resolved, path, resolving, depth+1)
			delete(resolving, ref)
			if len(schema) == 1 {
				return candidates
			}
			siblings := make(map[string]interface{}, len(schema)-1)
			for key, value := range schema {
				if key != "$ref" {
					siblings[key] = value
				}
			}
			return append(candidates, schemaPathCandidates(root, siblings, path, resolving, depth+1)...)
		}
	}
	if len(path) == 0 {
		return []map[string]interface{}{schema}
	}

	candidates := make([]map[string]interface{}, 0)
	switch token := path[0].(type) {
	case string:
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			if child, ok := properties[token].(map[string]interface{}); ok {
				candidates = append(candidates, schemaPathCandidates(root, child, path[1:], resolving, depth+1)...)
			}
		}
		if additional, ok := schema["additionalProperties"].(map[string]interface{}); ok {
			candidates = append(candidates, schemaPathCandidates(root, additional, path[1:], resolving, depth+1)...)
		}
	case int:
		if prefixItems, ok := schema["prefixItems"].([]interface{}); ok && token >= 0 && token < len(prefixItems) {
			if child, ok := prefixItems[token].(map[string]interface{}); ok {
				candidates = append(candidates, schemaPathCandidates(root, child, path[1:], resolving, depth+1)...)
			}
		} else if tupleItems, ok := schema["items"].([]interface{}); ok && token >= 0 && token < len(tupleItems) {
			if child, ok := tupleItems[token].(map[string]interface{}); ok {
				candidates = append(candidates, schemaPathCandidates(root, child, path[1:], resolving, depth+1)...)
			}
		} else if child, ok := schema["items"].(map[string]interface{}); ok {
			candidates = append(candidates, schemaPathCandidates(root, child, path[1:], resolving, depth+1)...)
		}
	}

	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		branches, ok := schema[keyword].([]interface{})
		if !ok {
			continue
		}
		alternatives := make([]interface{}, 0, len(branches))
		for _, branch := range branches {
			child, ok := branch.(map[string]interface{})
			if !ok {
				continue
			}
			found := schemaPathCandidates(root, child, path, resolving, depth+1)
			if len(found) == 0 {
				continue
			}
			if keyword == "allOf" {
				candidates = append(candidates, found...)
				continue
			}
			if len(found) == 1 {
				alternatives = append(alternatives, found[0])
			} else {
				combined := make([]interface{}, len(found))
				for index := range found {
					combined[index] = found[index]
				}
				alternatives = append(alternatives, map[string]interface{}{"allOf": combined})
			}
		}
		if len(alternatives) > 0 && keyword != "allOf" {
			candidates = append(candidates, map[string]interface{}{keyword: alternatives})
		}
	}
	return candidates
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
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		branches, ok := schema[keyword].([]interface{})
		if !ok {
			continue
		}
		combined := map[string]bool{}
		haveCombined := false
		for _, branch := range branches {
			child, ok := branch.(map[string]interface{})
			if !ok {
				continue
			}
			childTypes := schemaTypes(child)
			if len(childTypes) == 0 {
				continue
			}
			if keyword == "allOf" && haveCombined {
				for name := range combined {
					if !childTypes[name] {
						delete(combined, name)
					}
				}
			} else {
				for name := range childTypes {
					combined[name] = true
				}
			}
			haveCombined = true
		}
		if haveCombined && len(combined) > 0 {
			if len(types) == 0 {
				types = combined
			} else {
				for name := range types {
					if !combined[name] {
						delete(types, name)
					}
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
