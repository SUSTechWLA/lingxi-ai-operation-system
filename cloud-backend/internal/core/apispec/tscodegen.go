package apispec

import (
	"fmt"
	"sort"
	"strings"
)

// RenderTypeScript generates TypeScript type definitions from an OpenAPI Spec's
// component schemas. Object schemas are emitted as interfaces; composed schemas
// are emitted as aliases so OpenAPI unions and intersections survive generation.
func RenderTypeScript(spec *Spec) []byte {
	var b strings.Builder

	b.WriteString("// GENERATED from OpenAPI spec — do not edit.\n")
	b.WriteString("// Regenerate: cd cloud-backend && make gen-ts\n")
	b.WriteString("// Source of truth: internal/core/apispec/cloud_spec.go\n")
	b.WriteString("//\n")
	b.WriteString("// This file contains the response/request body types derived from\n")
	b.WriteString("// the OpenAPI components/schemas. The generic ApiResponse<T> envelope\n")
	b.WriteString("// lives in types.ts alongside frontend-only UI types.\n\n")

	if spec.Components == nil || spec.Components.Schemas == nil {
		b.WriteString("// No schemas to export.\n")
		return []byte(b.String())
	}

	// Sort schemas for stable output
	names := make([]string, 0, len(spec.Components.Schemas))
	for name := range spec.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			continue
		}
		if len(schema.OneOf) > 0 || len(schema.AllOf) > 0 {
			b.WriteString(fmt.Sprintf("/** %s */\n", schema.Description))
			if schema.Description == "" {
				b.WriteString(fmt.Sprintf("// %s\n", name))
			}
			b.WriteString(fmt.Sprintf("export type %s = %s;\n\n", toPascalCase(name), schemaToTSType(schema)))
			continue
		}
		// Only emit object schemas as interfaces
		if schema.Type != "object" && schema.Type != "" {
			continue
		}
		// Skip request-body-only schemas (DAGRequest, etc.) — keep response types
		// Actually emit all objects for completeness; TS codegen consumers can filter.

		b.WriteString(fmt.Sprintf("/** %s */\n", schema.Description))
		if schema.Description == "" {
			b.WriteString(fmt.Sprintf("// %s\n", name))
		}
		b.WriteString(fmt.Sprintf("export interface %s {\n", toPascalCase(name)))

		// Emit properties sorted by name for stability
		propNames := make([]string, 0, len(schema.Properties))
		for p := range schema.Properties {
			propNames = append(propNames, p)
		}
		sort.Strings(propNames)

		required := make(map[string]bool, len(schema.Required))
		for _, r := range schema.Required {
			required[r] = true
		}

		for _, prop := range propNames {
			propSchema := schema.Properties[prop]
			if propSchema == nil || propSchema.Schema == nil {
				// $ref case — resolve and continue; for now emit as any
				if propSchema != nil && propSchema.Ref != "" {
					refName := strings.TrimPrefix(propSchema.Ref, "#/components/schemas/")
					b.WriteString(fmt.Sprintf("  %s%s: %s;\n",
						prop, optionalSuffix(required, prop), toPascalCase(refName)))
				} else {
					b.WriteString(fmt.Sprintf("  %s%s: unknown;\n", prop, optionalSuffix(required, prop)))
				}
				continue
			}

			tsType := schemaToTSType(propSchema.Schema)
			opt := optionalSuffix(required, prop)
			b.WriteString(fmt.Sprintf("  %s%s: %s;\n", prop, opt, tsType))
		}

		b.WriteString("}\n\n")
	}

	return []byte(strings.TrimRight(b.String(), "\n") + "\n")
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func optionalSuffix(required map[string]bool, prop string) string {
	if required[prop] {
		return ""
	}
	return "?"
}

func schemaToTSType(s *Schema) string {
	if s == nil {
		return "unknown"
	}
	if s.Nullable {
		innerSchema := *s
		innerSchema.Nullable = false
		inner := schemaToTSType(&innerSchema)
		return inner + " | null"
	}
	if s.Ref != "" {
		return toPascalCase(strings.TrimPrefix(s.Ref, "#/components/schemas/"))
	}
	if len(s.OneOf) > 0 {
		parts := make([]string, 0, len(s.OneOf))
		for _, branch := range s.OneOf {
			parts = append(parts, schemaRefToTSType(branch))
		}
		return strings.Join(parts, " | ")
	}
	if len(s.AllOf) > 0 {
		parts := make([]string, 0, len(s.AllOf))
		for _, branch := range s.AllOf {
			parts = append(parts, schemaRefToTSType(branch))
		}
		return strings.Join(parts, " & ")
	}
	switch s.Type {
	case "string":
		if len(s.Enum) > 0 {
			quoted := make([]string, len(s.Enum))
			for i, e := range s.Enum {
				quoted[i] = fmt.Sprintf("'%v'", e)
			}
			return strings.Join(quoted, " | ")
		}
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		if s.Items != nil {
			return arrayItemType(s.Items) + "[]"
		}
		return "unknown[]"
	case "object":
		if s.AdditionalProperties != nil {
			return "Record<string, " + schemaRefToTSType(s.AdditionalProperties) + ">"
		}
		if len(s.Properties) > 0 {
			var b strings.Builder
			b.WriteString("{ ")
			propNames := make([]string, 0, len(s.Properties))
			for p := range s.Properties {
				propNames = append(propNames, p)
			}
			sort.Strings(propNames)
			required := map[string]bool{}
			for _, r := range s.Required {
				required[r] = true
			}
			for i, p := range propNames {
				if i > 0 {
					b.WriteString("; ")
				}
				ts := "unknown"
				if s.Properties[p] != nil && s.Properties[p].Schema != nil {
					ts = schemaToTSType(s.Properties[p].Schema)
				} else if s.Properties[p] != nil && s.Properties[p].Ref != "" {
					ts = toPascalCase(strings.TrimPrefix(s.Properties[p].Ref, "#/components/schemas/"))
				}
				b.WriteString(p)
				b.WriteString(optionalSuffix(required, p))
				b.WriteString(": ")
				b.WriteString(ts)
			}
			b.WriteString(" }")
			return b.String()
		}
		return "Record<string, unknown>"
	default:
		return "unknown"
	}
}

func schemaRefToTSType(ref *SchemaRef) string {
	if ref == nil {
		return "unknown"
	}
	if ref.Ref != "" {
		return toPascalCase(strings.TrimPrefix(ref.Ref, "#/components/schemas/"))
	}
	return schemaToTSType(ref.Schema)
}

func arrayItemType(item *SchemaRef) string {
	typeName := schemaRefToTSType(item)
	if strings.Contains(typeName, " | ") || strings.Contains(typeName, " & ") {
		return "(" + typeName + ")"
	}
	return typeName
}
