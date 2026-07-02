package apispec

import (
	"fmt"
	"sort"
	"strings"
)

// RenderMarkdown produces a human-readable API reference in Markdown format
// from an OpenAPI 3.0 Spec.
//
// The output follows the same sectioned style as the existing API_REFERENCE.md:
// TOC, per-endpoint sections with HTTP method, path, description, parameters,
// and response schemas. It includes a banner warning that the file is generated.
func RenderMarkdown(spec *Spec) []byte {
	var b strings.Builder

	// Header banner
	b.WriteString("<!-- GENERATED — do not edit.\n")
	b.WriteString("     Development branches may regenerate this from the OpenAPI spec.\n")
	b.WriteString("     Source of truth: internal/core/apispec/cloud_spec.go\n")
	b.WriteString("-->\n\n")

	// Title
	b.WriteString(fmt.Sprintf("# %s\n\n", spec.Info.Title))
	b.WriteString(fmt.Sprintf("Version: %s\n\n", spec.Info.Version))

	// Servers
	if len(spec.Servers) > 0 {
		b.WriteString("## Base URLs\n\n")
		for _, s := range spec.Servers {
			b.WriteString(fmt.Sprintf("- `%s` — %s\n", s.URL, s.Description))
		}
		b.WriteString("\n")
	}

	// Group paths by first tag
	groups := groupPathsByTag(spec)

	// Build sorted tag list for TOC
	sortedTags := sortedTags(groups)

	// TOC
	b.WriteString("## Table of Contents\n\n")
	num := 1
	for _, tag := range sortedTags {
		b.WriteString(fmt.Sprintf("%d. [%s](#%d-%s)\n", num, tag, num, anchorID(tag)))
		num++
	}
	b.WriteString("\n---\n\n")

	// Per-tag sections
	num = 1
	for _, tag := range sortedTags {
		entries := groups[tag]
		b.WriteString(fmt.Sprintf("## %d. %s\n\n", num, tag))
		for _, entry := range entries {
			b.WriteString(fmt.Sprintf("### %s %s\n\n", entry.Method, entry.Path))
			if entry.Summary != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", entry.Summary))
			}

			// Parameters
			if len(entry.Params) > 0 {
				b.WriteString("**Parameters:**\n\n")
				b.WriteString("| Name | In | Type | Required | Description |\n")
				b.WriteString("|------|----|------|----------|-------------|\n")
				for _, p := range entry.Params {
					req := "No"
					if p.Required {
						req = "**Yes**"
					}
					typ := "`string`"
					if s := p.Schema; s != nil && s.Schema != nil {
						typ = formatSchemaType(s.Schema)
					}
					b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
						p.Name, p.In, typ, req, p.Description))
				}
				b.WriteString("\n")
			}

			// Request body
			if entry.RequestBody != nil {
				b.WriteString(fmt.Sprintf("**Request body:** %s (Content-Type: `%s`)\n\n",
					labelRequired(entry.RequestBody.Required), contentType(entry.RequestBody)))
				b.WriteString("```json\n")
				b.WriteString(exampleJSON(entry.RequestBody))
				b.WriteString("\n```\n\n")
			}

			// Responses
			if len(entry.Responses) > 0 {
				b.WriteString("**Responses:**\n\n")
				statuses := make([]string, 0, len(entry.Responses))
				for status := range entry.Responses {
					statuses = append(statuses, status)
				}
				sort.Strings(statuses)
				for _, status := range statuses {
					resp := entry.Responses[status]
					b.WriteString(fmt.Sprintf("- **%s** — %s", status, resp.Description))
					if hasBody(resp) {
						b.WriteString(" (JSON)")
					}
					b.WriteString("\n")
				}
				b.WriteString("\n")
			}

			b.WriteString("---\n\n")
		}
		num++
	}

	return []byte(b.String())
}

type pathEntry struct {
	Method      string
	Path        string
	Summary     string
	Params      []Parameter
	RequestBody *RequestBody
	Responses   map[string]*Response
	Tag         string
}

func groupPathsByTag(spec *Spec) map[string][]*pathEntry {
	groups := map[string][]*pathEntry{}
	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	methods := []struct {
		name string
		get  func(*PathItem) *Operation
	}{
		{"GET", func(item *PathItem) *Operation { return item.Get }},
		{"POST", func(item *PathItem) *Operation { return item.Post }},
		{"PUT", func(item *PathItem) *Operation { return item.Put }},
		{"PATCH", func(item *PathItem) *Operation { return item.Patch }},
		{"DELETE", func(item *PathItem) *Operation { return item.Delete }},
	}
	for _, path := range paths {
		item := spec.Paths[path]
		for _, method := range methods {
			op := method.get(item)
			if op == nil {
				continue
			}
			tag := "Other"
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			groups[tag] = append(groups[tag], &pathEntry{
				Method:      method.name,
				Path:        path,
				Summary:     op.Summary,
				Params:      op.Parameters,
				RequestBody: op.RequestBody,
				Responses:   op.Responses,
				Tag:         tag,
			})
		}
	}
	for tag := range groups {
		sort.SliceStable(groups[tag], func(i, j int) bool {
			left := groups[tag][i]
			right := groups[tag][j]
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			return methodRank(left.Method) < methodRank(right.Method)
		})
	}
	return groups
}

func methodRank(method string) int {
	switch method {
	case "GET":
		return 0
	case "POST":
		return 1
	case "PUT":
		return 2
	case "PATCH":
		return 3
	case "DELETE":
		return 4
	default:
		return 99
	}
}

func sortedTags(groups map[string][]*pathEntry) []string {
	tags := make([]string, 0, len(groups))
	for t := range groups {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	// Move "Other" to end
	others := make([]string, 0)
	rest := make([]string, 0, len(tags))
	for _, t := range tags {
		if t == "Other" {
			others = append(others, t)
		} else {
			rest = append(rest, t)
		}
	}
	rest = append(rest, others...)
	return rest
}

func anchorID(tag string) string {
	return strings.ReplaceAll(strings.ToLower(tag), " ", "-")
}

func formatSchemaType(s *Schema) string {
	if s.Ref != "" {
		return "`" + strings.TrimPrefix(s.Ref, "#/components/schemas/") + "`"
	}
	if s.Type == "array" && s.Items != nil && s.Items.Schema != nil {
		return "`[" + formatSchemaType(s.Items.Schema) + "]`"
	}
	if s.Type != "" {
		if s.Format != "" {
			return fmt.Sprintf("`%s (%s)`", s.Type, s.Format)
		}
		return fmt.Sprintf("`%s`", s.Type)
	}
	return "`object`"
}

func labelRequired(required bool) string {
	if required {
		return "**Required**"
	}
	return "Optional"
}

func contentType(body *RequestBody) string {
	contentTypes := make([]string, 0, len(body.Content))
	for ct := range body.Content {
		contentTypes = append(contentTypes, ct)
	}
	sort.Strings(contentTypes)
	if len(contentTypes) > 0 {
		return contentTypes[0]
	}
	return "application/json"
}

func hasBody(resp *Response) bool {
	return resp.Content != nil && len(resp.Content) > 0
}

func exampleJSON(body *RequestBody) string {
	contentTypes := make([]string, 0, len(body.Content))
	for ct := range body.Content {
		contentTypes = append(contentTypes, ct)
	}
	sort.Strings(contentTypes)
	for _, ct := range contentTypes {
		mt := body.Content[ct]
		if mt.Schema != nil {
			return schemaExample(mt.Schema)
		}
	}
	return "{}"
}

func schemaExample(ref *SchemaRef) string {
	if ref.Ref != "" {
		return fmt.Sprintf("{ \"$ref\": \"%s\" }", ref.Ref)
	}
	if ref.Schema == nil {
		return "{}"
	}
	return schemaToExample(ref.Schema, 0)
}

func schemaToExample(s *Schema, depth int) string {
	if depth > 3 {
		return "{ ... }"
	}
	switch s.Type {
	case "string":
		if s.Format == "date-time" {
			return `"2026-01-01T00:00:00Z"`
		}
		return `"string"`
	case "integer":
		return "0"
	case "number":
		return "0.0"
	case "boolean":
		return "false"
	case "array":
		if s.Items != nil && s.Items.Schema != nil {
			return "[" + schemaToExample(s.Items.Schema, depth+1) + "]"
		}
		return "[]"
	case "object":
		if s.Ref != "" {
			return fmt.Sprintf("{ \"$ref\": \"%s\" }", s.Ref)
		}
		if s.Properties == nil || len(s.Properties) == 0 {
			return "{}"
		}
		var b strings.Builder
		b.WriteString("{\n")
		propNames := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			propNames = append(propNames, name)
		}
		sort.Strings(propNames)
		for count, name := range propNames {
			prop := s.Properties[name]
			if count > 5 {
				b.WriteString("  ...\n")
				break
			}
			if prop == nil || prop.Schema == nil {
				b.WriteString(fmt.Sprintf("  \"%s\": null,\n", name))
			} else {
				b.WriteString(fmt.Sprintf("  \"%s\": %s,\n", name, schemaToExample(prop.Schema, depth+1)))
			}
			count++
		}
		b.WriteString("}")
		return b.String()
	default:
		return "null"
	}
}
