// gen-apidocs regenerates cloud-backend/docs/API_REFERENCE.md and
// frontend/src/utils/api-types.generated.ts from the authoritative
// OpenAPI spec (internal/core/apispec/cloud_spec.go).
//
// Usage:
//
//	cd cloud-backend && go run ./cmd/gen-apidocs
//	make gen-docs
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tangying-ai/aios-core/internal/core/apispec"
)

func main() {
	spec := apispec.BuildCloudSpec()

	// Root of the repository (cloud-backend is one level down)
	root, err := findRepoRoot()
	if err != nil {
		log.Fatalf("Cannot find repo root: %v", err)
	}

	// 1. Markdown docs
	mdPath := filepath.Join(root, "cloud-backend", "docs", "API_REFERENCE.md")
	md := apispec.RenderMarkdown(spec)
	if err := os.WriteFile(mdPath, md, 0o644); err != nil {
		log.Fatalf("Failed to write %s: %v", mdPath, err)
	}
	log.Printf("Wrote %s (%d bytes)", mdPath, len(md))

	// 2. TypeScript types
	tsPath := filepath.Join(root, "frontend", "src", "utils", "api-types.generated.ts")
	ts := apispec.RenderTypeScript(spec)
	if err := os.MkdirAll(filepath.Dir(tsPath), 0o755); err != nil {
		log.Fatalf("Failed to create frontend dir: %v", err)
	}
	if err := os.WriteFile(tsPath, ts, 0o644); err != nil {
		log.Fatalf("Failed to write %s: %v", tsPath, err)
	}
	log.Printf("Wrote %s (%d bytes)", tsPath, len(ts))

	log.Println("Done. Verify with: make api-docs-check")
}

func findRepoRoot() (string, error) {
	// Look for AGENTS.md as the marker
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
