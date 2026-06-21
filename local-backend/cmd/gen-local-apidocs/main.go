// gen-local-apidocs regenerates local-backend/docs/API_REFERENCE.md
// from the local agent's OpenAPI spec.
//
// Usage:
//
//	cd local-backend && go run ./cmd/gen-local-apidocs
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localagent"
)

func main() {
	spec := localagent.BuildLocalSpec()

	md := renderLocalMarkdown(spec)

	outPath := filepath.Join("docs", "API_REFERENCE.md")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		log.Fatalf("Failed to create docs dir: %v", err)
	}
	if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
		log.Fatalf("Failed to write %s: %v", outPath, err)
	}
	log.Printf("Wrote %s (%d bytes)", outPath, len(md))
}

func renderLocalMarkdown(spec *localagent.LocalSpec) string {
	var b strings.Builder
	b.WriteString("<!-- GENERATED — do not edit. Run: cd local-backend && go run ./cmd/gen-local-apidocs -->\n\n")
	b.WriteString(fmt.Sprintf("# %s\n\n", spec.Info.Title))
	b.WriteString(fmt.Sprintf("Version: %s\n\n", spec.Info.Version))
	if len(spec.Servers) > 0 {
		b.WriteString("## Base URL\n\n")
		b.WriteString(fmt.Sprintf("- `%s` — %s\n\n", spec.Servers[0].URL, spec.Servers[0].Description))
	}
	b.WriteString("---\n\n")

	// Sort paths
	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		methods := spec.Paths[path]
		for methodKey, op := range methods {
			method := strings.ToUpper(methodKey)
			b.WriteString(fmt.Sprintf("### %s %s\n\n", method, path))
			if op.Summary != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", op.Summary))
			}

			if len(op.Parameters) > 0 {
				b.WriteString("**Parameters:**\n\n")
				b.WriteString("| Name | In | Type | Required | Description |\n")
				b.WriteString("|------|----|------|----------|-------------|\n")
				for _, p := range op.Parameters {
					req := "No"
					if p.Required {
						req = "Yes"
					}
					typ := "string"
					if p.Schema != nil && p.Schema.Type != "" {
						typ = p.Schema.Type
					}
					b.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | %s | %s |\n",
						p.Name, p.In, typ, req, p.Description))
				}
				b.WriteString("\n")
			}

			if op.RequestBody != nil {
				required := "Optional"
				if op.RequestBody.Required {
					required = "**Required**"
				}
				b.WriteString(fmt.Sprintf("**Request body:** %s (Content-Type: `application/json`)\n\n```json\n{}\n```\n\n",
					required))
			}

			for status, resp := range op.Responses {
				b.WriteString(fmt.Sprintf("- **%s** — %s\n", status, resp.Description))
			}
			if len(op.Responses) > 0 {
				b.WriteString("\n")
			}
			b.WriteString("---\n\n")
		}
	}
	return b.String()
}
