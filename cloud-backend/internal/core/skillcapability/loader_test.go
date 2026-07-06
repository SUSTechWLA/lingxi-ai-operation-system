package skillcapability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCapabilities_HappyPath(t *testing.T) {
	root := scaffoldCapabilityTree(t)
	entries, errs := LoadCapabilities(root)
	if len(errs) > 0 {
		t.Fatalf("unexpected load errors: %v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 tool entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ManifestName != "parse_bid_files" {
		t.Fatalf("unexpected manifest name: %q", e.ManifestName)
	}
	if e.ManifestVersion != "1.0.0" {
		t.Fatalf("unexpected manifest version: %q", e.ManifestVersion)
	}
	if e.Skill.ID != "biaoshu-writer" {
		t.Fatalf("unexpected skill ID: %q", e.Skill.ID)
	}
	// Verify that the manifest can be serialized to JSON and back
	raw, err := json.Marshal(e.Manifest)
	if err != nil {
		t.Fatalf("cannot marshal manifest: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("cannot unmarshal manifest: %v", err)
	}
	if back["name"] != "parse_bid_files" {
		t.Fatalf("manifest name mismatch after round-trip: %v", back["name"])
	}
}

func TestLoadCapabilities_InvalidYAML(t *testing.T) {
	root := t.TempDir()
	capDir := filepath.Join(root, "biaoshu", "test-writer", "1.0.0", "tools")
	if err := os.MkdirAll(capDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create capability.yaml valid but tool manifest invalid
	capYAML := "id: test-writer\nname: Test Writer\ndomain: bid_writing\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(capDir), "capability.yaml"), []byte(capYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(capDir, "broken.yaml"), []byte("{{not valid yaml]]"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, errs := LoadCapabilities(root)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for broken YAML, got %d", len(entries))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestLoadCapabilities_MissingName(t *testing.T) {
	root := t.TempDir()
	capDir := filepath.Join(root, "biaoshu", "test-writer", "1.0.0")
	toolsDir := filepath.Join(capDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(capDir, "capability.yaml"), []byte("id: test-writer\nname: Test Writer\ndomain: bid_writing\nversion: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "no_name.yaml"), []byte("description: A tool without a name\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errs := LoadCapabilities(root)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for missing name, got %d", len(errs))
	}
}

func TestLoadCapabilities_EmptyRoot(t *testing.T) {
	entries, errs := LoadCapabilities("")
	if entries != nil || errs != nil {
		t.Fatalf("empty root should return nil, nil, got entries=%v errs=%v", entries, errs)
	}
}

func TestLoadCapabilities_SkipNonYAMLFiles(t *testing.T) {
	root := scaffoldCapabilityTree(t)
	toolsDir := filepath.Join(root, "biaoshu", "biaoshu-writer", "1.0.0", "tools")
	if err := os.WriteFile(filepath.Join(toolsDir, "README.md"), []byte("# docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only 1 YAML tool; .md files should be skipped.
	entries, _ := LoadCapabilities(root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after skipping .md, got %d", len(entries))
	}
}

func scaffoldCapabilityTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	capDir := filepath.Join(root, "biaoshu", "biaoshu-writer", "1.0.0")
	toolsDir := filepath.Join(capDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	capYAML := `id: biaoshu-writer
name: Biaoshu Writer
description: AI-powered bid/tender document writing toolkit
domain: bid_writing
version: 1.0.0
tags:
  - bid_writing
  - bid
  - biaoshu
tools:
  - id: parse_bid_files
    manifest: tools/parse_bid_files.tool.yaml
`
	if err := os.WriteFile(filepath.Join(capDir, "capability.yaml"), []byte(capYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	toolYAML := `name: parse_bid_files
description: Parse tender bid files
version: 1.0.0
type: http
endpoint: http://127.0.0.1:9001/tools/parse_bid_files
timeout: 300
capabilities:
  - bid_writing
  - bid_parsing
  - document_parsing
parameters:
  file_path:
    type: string
    required: true
output:
  report_path:
    type: string
`
	if err := os.WriteFile(filepath.Join(toolsDir, "parse_bid_files.tool.yaml"), []byte(toolYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}
