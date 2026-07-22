package tool

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestCanonicalSchemaValidatorEnforcesNestedArraysEnumsRequiredAndClosedObjects(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"properties": map[string]interface{}{"request": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{"type": "string", "enum": []interface{}{"fast", "quality"}},
				"items": map[string]interface{}{"type": "array", "items": map[string]interface{}{
					"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}},
					"required": []interface{}{"id"}, "additionalProperties": false,
				}},
			},
			"required": []interface{}{"mode", "items"}, "additionalProperties": false,
		}},
		"required": []interface{}{"request"}, "additionalProperties": false,
	}
	validator, err := CompileCanonicalSchema(schema)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	valid := map[string]interface{}{"request": map[string]interface{}{"mode": "quality", "items": []interface{}{map[string]interface{}{"id": "asset-1"}}}}
	if err := validator.Validate(valid); err != nil {
		t.Fatalf("valid nested input rejected: %v", err)
	}
	invalid := []map[string]interface{}{
		{"request": map[string]interface{}{"mode": "unknown", "items": []interface{}{}}},
		{"request": map[string]interface{}{"mode": "fast", "items": []interface{}{map[string]interface{}{"id": "asset-1", "extra": true}}}},
		{"request": map[string]interface{}{"mode": "fast", "items": []interface{}{map[string]interface{}{}}}},
	}
	for index, input := range invalid {
		if err := validator.Validate(input); err == nil {
			t.Fatalf("invalid case %d passed: %#v", index, input)
		}
	}
}

func TestCanonicalSchemaValidatorSupportsDraft7RejectsRemoteRefsAndDoesNotAlias(t *testing.T) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#", "type": "object",
		"definitions": map[string]interface{}{"name": map[string]interface{}{"type": "string", "minLength": float64(1)}},
		"properties":  map[string]interface{}{"name": map[string]interface{}{"$ref": "#/definitions/name"}},
		"required":    []interface{}{"name"},
	}
	validator, err := CompileCanonicalSchema(schema)
	if err != nil {
		t.Fatalf("compile draft-07: %v", err)
	}
	schema["required"].([]interface{})[0] = "mutated"
	if err := validator.Validate(map[string]interface{}{"name": "kept"}); err != nil {
		t.Fatalf("validator aliases caller schema: %v", err)
	}
	if err := validator.Validate(map[string]interface{}{"name": ""}); err == nil {
		t.Fatal("draft-07 minLength was not enforced")
	}
	_, err = CompileCanonicalSchema(map[string]interface{}{"type": "object", "properties": map[string]interface{}{
		"remote": map[string]interface{}{"$ref": "https://schemas.example.invalid/remote.json"},
	}})
	if err == nil || !strings.Contains(err.Error(), "remote") {
		t.Fatalf("remote $ref did not fail closed: %v", err)
	}
	if _, err := CompileCanonicalSchema(map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2019-09/schema", "type": "object",
	}); err == nil {
		t.Fatal("unsupported JSON Schema draft unexpectedly compiled")
	}
}

func TestCanonicalSchemaValidatorCacheIsConcurrent(t *testing.T) {
	const count = 16
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			validator, err := CompileCanonicalSchema(map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
				"required": []interface{}{"name"}, "additionalProperties": false,
			})
			if err == nil {
				err = validator.Validate(map[string]interface{}{"name": "ok"})
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCanonicalSchemaValidatorBoundsSchemaBytesAndDepth(t *testing.T) {
	_, err := CompileCanonicalSchema(map[string]interface{}{
		"type": "object", "description": strings.Repeat("x", maxCanonicalSchemaBytes),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized schema error = %v", err)
	}

	root := map[string]interface{}{"type": "object"}
	cursor := root
	for index := 0; index < maxCanonicalSchemaDepth+2; index++ {
		next := map[string]interface{}{"type": "object"}
		cursor["properties"] = map[string]interface{}{"nested": next}
		cursor = next
	}
	if _, err := CompileCanonicalSchema(root); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("over-depth schema error = %v", err)
	}
}

func TestValidateLocalJobOutputUsesMCPStructuredContentAndNativeRoot(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{"assetId": map[string]interface{}{"type": "string"}},
		"required": []interface{}{"assetId"}, "additionalProperties": false,
	}
	mcp := &ToolManifest{Name: "mcp_asset", Boundary: BoundaryMCPProvider, OutputSchema: schema}
	if err := ValidateLocalJobOutput(mcp, map[string]interface{}{
		"content": []interface{}{}, "structuredContent": map[string]interface{}{"assetId": "asset-1"}, "isError": false,
	}); err != nil {
		t.Fatalf("valid MCP output rejected: %v", err)
	}
	if err := ValidateLocalJobOutput(mcp, map[string]interface{}{"assetId": "wrong-root"}); !errors.Is(err, ErrOutputSchemaInvalid) {
		t.Fatalf("MCP wrapper root validated instead of structuredContent: %v", err)
	}
	native := &ToolManifest{Name: "native_asset", Boundary: BoundaryLocalNative, OutputSchema: schema}
	if err := ValidateLocalJobOutput(native, map[string]interface{}{"assetId": "asset-2"}); err != nil {
		t.Fatalf("native output rejected: %v", err)
	}
}

func TestLegacyManifestWithoutCanonicalSchemaRemainsPermissive(t *testing.T) {
	manifest := &ToolManifest{Name: "legacy", Parameters: map[string]ParamDef{"query": {Type: "string", Required: true}}}
	if err := ValidateManifestInput(manifest, map[string]interface{}{"query": "ok", "extra": true}); err != nil {
		t.Fatalf("legacy input narrowed: %v", err)
	}
	if err := ValidateLocalJobOutput(manifest, map[string]interface{}{"extra": true}); err != nil {
		t.Fatalf("legacy output narrowed: %v", err)
	}
}
