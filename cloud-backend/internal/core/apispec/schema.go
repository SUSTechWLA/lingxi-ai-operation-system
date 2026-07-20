package apispec

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// Reflect derives an OpenAPI 3.0 JSON Schema from a Go type T.
// T should be a struct type or a pointer-to-struct. Only exported fields
// with json tags are included.
//
// Supported types:
//   - string, int, int64, float64, bool → primitive schemas
//   - time.Time → string format: date-time
//   - json.RawMessage → unconstrained JSON value
//   - map[string]interface{} → object (free-form)
//   - map[string]T → object with additionalProperties: T
//   - []T → array with items: T
//   - *T → nullable (schema + nullable: true)
//   - struct → object with properties derived from json tags
//
// Fields tagged with `json:"-"` are skipped. `omitempty` json tags make a
// field optional (not in required[]).
func Reflect(typ any) *Schema {
	return reflectType(reflect.TypeOf(typ), map[reflect.Type]bool{})
}

func reflectType(t reflect.Type, seen map[reflect.Type]bool) *Schema {
	// Dereference pointers
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t == nil {
		return &Schema{Description: "arbitrary JSON value"}
	}

	// json.RawMessage is a named []byte. Detect it before the slice branch so
	// the wire value remains honest JSON instead of becoming a base64 string.
	if t == reflect.TypeOf(json.RawMessage{}) {
		return &Schema{Description: "arbitrary JSON value"}
	}

	// Prevent infinite recursion
	if seen[t] {
		return &Schema{Type: "object", Description: "recursive reference"}
	}
	seen[t] = true
	defer func() { delete(seen, t) }()

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}

	case reflect.Int, reflect.Int32:
		return &Schema{Type: "integer"}

	case reflect.Int64:
		return &Schema{Type: "integer", Format: "int64"}

	case reflect.Uint, reflect.Uint32:
		return &Schema{Type: "integer"}

	case reflect.Uint64:
		return &Schema{Type: "integer", Format: "int64"}

	case reflect.Float32:
		return &Schema{Type: "number", Format: "float"}

	case reflect.Float64:
		return &Schema{Type: "number", Format: "double"}

	case reflect.Bool:
		return &Schema{Type: "boolean"}

	case reflect.Interface:
		return &Schema{Description: "arbitrary JSON value"}

	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			valSchema := reflectType(t.Elem(), seen)
			return &Schema{Type: "object", AdditionalProperties: &SchemaRef{Schema: valSchema}}
		}
		return &Schema{Type: "object", Description: "map"}

	case reflect.Slice, reflect.Array:
		// Special case: []byte = string, format: byte
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string", Format: "byte"}
		}
		itemSchema := reflectType(t.Elem(), seen)
		return &Schema{Type: "array", Items: &SchemaRef{Schema: itemSchema}}

	case reflect.Struct:
		// time.Time special case
		if t == reflect.TypeOf(time.Time{}) {
			return &Schema{Type: "string", Format: "date-time"}
		}
		return reflectStruct(t, seen)

	default:
		return &Schema{Type: "string", Description: t.String()}
	}
}

func reflectStruct(t reflect.Type, seen map[reflect.Type]bool) *Schema {
	schema := &Schema{Type: "object", Properties: map[string]*SchemaRef{}}
	required := []string{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		// Parse json tag: "name,omitempty" → name, optional
		parts := strings.Split(jsonTag, ",")
		name := parts[0]
		if name == "" {
			name = field.Name
		}
		omitempty := false
		for _, opt := range parts[1:] {
			if opt == "omitempty" {
				omitempty = true
			}
		}

		fieldType := field.Type
		nullable := false
		if fieldType.Kind() == reflect.Ptr {
			nullable = true
		}

		fieldSchema := reflectType(fieldType, seen)
		if nullable && fieldSchema != nil {
			fieldSchema.Nullable = true
		}

		schema.Properties[name] = &SchemaRef{Schema: fieldSchema}
		if !omitempty {
			required = append(required, name)
		}
	}

	if len(required) > 0 {
		schema.Required = required
	}
	return schema
}

// PrimitiveSchema is a shorthand for common primitive schemas used in query/path params.
func PrimitiveSchema(typ, format string) *Schema {
	return &Schema{Type: typ, Format: format}
}

// StringSchema is a shorthand for string type.
func StringSchema() *Schema {
	return &Schema{Type: "string"}
}

// IntegerSchema is a shorthand for integer type.
func IntegerSchema() *Schema {
	return &Schema{Type: "integer"}
}

// BoolSchema is a shorthand for boolean type.
func BoolSchema() *Schema {
	return &Schema{Type: "boolean"}
}

// ArraySchema is a shorthand for an array of items.
func ArraySchema(items *Schema) *Schema {
	return &Schema{Type: "array", Items: &SchemaRef{Schema: items}}
}

// ObjectSchema is a shorthand for a plain object (free-form).
func ObjectSchema() *Schema {
	return &Schema{Type: "object"}
}

// RefSchema creates a $ref schema reference.
func RefSchema(name string) *Schema {
	return &Schema{Ref: "#/components/schemas/" + name}
}

// Optional wraps a schema as optional (removes from required — used by ParamSchema).
// For reflect-based schemas, omitempty/pointer handles this automatically.
func Optional(s *Schema) *Schema {
	return s
}
