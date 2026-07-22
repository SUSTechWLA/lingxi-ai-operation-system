package agentruntime

import (
	"reflect"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func canonicalToolSchema(schema map[string]interface{}, legacy map[string]tool.ParamDef) map[string]interface{} {
	if schema != nil {
		return cloneJSONMap(schema)
	}
	properties := make(map[string]interface{}, len(legacy))
	required := make([]string, 0, len(legacy))
	for name, definition := range legacy {
		property := map[string]interface{}{}
		propertyType := strings.TrimSpace(definition.Type)
		if propertyType == "" {
			propertyType = "object"
		}
		property["type"] = propertyType
		if definition.Description != "" {
			property["description"] = definition.Description
		}
		if definition.Default != nil {
			property["default"] = cloneJSONValue(definition.Default)
		}
		if len(definition.Enum) > 0 {
			property["enum"] = append([]string(nil), definition.Enum...)
		}
		properties[name] = property
		if definition.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	out := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func cloneLegacyParamDefs(input map[string]tool.ParamDef) map[string]tool.ParamDef {
	if input == nil {
		return nil
	}
	out := make(map[string]tool.ParamDef, len(input))
	for name, definition := range input {
		definition.Enum = append([]string(nil), definition.Enum...)
		definition.Default = cloneJSONValue(definition.Default)
		out[name] = definition
	}
	return out
}

func cloneJSONMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	cloned, _ := cloneJSONValue(input).(map[string]interface{})
	return cloned
}

func cloneJSONValue(input interface{}) interface{} {
	if input == nil {
		return nil
	}
	return cloneJSONReflect(reflect.ValueOf(input)).Interface()
}

func cloneJSONReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneJSONReflect(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			cloned.SetMapIndex(cloneJSONReflect(iterator.Key()), cloneJSONReflect(iterator.Value()))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneJSONReflect(value.Index(i)))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneJSONReflect(value.Index(i)))
		}
		return cloned
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneJSONReflect(value.Elem()))
		return cloned
	case reflect.Struct:
		cloned := reflect.New(value.Type()).Elem()
		cloned.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath == "" && cloned.Field(i).CanSet() {
				cloned.Field(i).Set(cloneJSONReflect(value.Field(i)))
			}
		}
		return cloned
	default:
		return value
	}
}
