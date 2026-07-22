package repository

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestToolManifestRepositoryContractUsesFortySharedColumns(t *testing.T) {
	if len(toolManifestColumns) != 40 {
		t.Fatalf("tool manifest columns = %d, want 40: %#v", len(toolManifestColumns), toolManifestColumns)
	}
	if toolManifestColumns[7] != "input_schema" || toolManifestColumns[8] != "output_schema" {
		t.Fatalf("canonical schema columns must be positions 8/9: %#v", toolManifestColumns[7:9])
	}
	if !strings.Contains(toolManifestUpsertSQL, toolManifestSelectColumns) ||
		!strings.Contains(toolManifestFindByNameSQL, toolManifestSelectColumns) ||
		!strings.Contains(toolManifestFindAllSQL, toolManifestSelectColumns) {
		t.Fatal("upsert/find queries do not share the tested tool manifest column contract")
	}

	placeholderPattern := regexp.MustCompile(`\$(\d+)`)
	seen := map[int]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(toolManifestUpsertSQL, -1) {
		position, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		seen[position] = true
	}
	for position := 1; position <= 40; position++ {
		if !seen[position] {
			t.Fatalf("upsert SQL missing placeholder $%d", position)
		}
	}
	if seen[41] {
		t.Fatal("upsert SQL unexpectedly contains placeholder $41")
	}
}

func TestBuildToolManifestUpsertArgsPreservesCanonicalSchemaPositions(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	record := &model.ToolManifestRecord{
		Name:         "contract_tool",
		Description:  "Repository contract",
		Type:         "external",
		Version:      "2.0.0",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}}}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}}}`),
		Parameters:   json.RawMessage(`{"prompt":{"type":"string"}}`),
		Output:       json.RawMessage(`{"url":{"type":"string"}}`),
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}

	args, err := buildToolManifestUpsertArgs(record)
	if err != nil {
		t.Fatalf("build upsert args: %v", err)
	}
	if len(args) != 40 {
		t.Fatalf("upsert args = %d, want 40", len(args))
	}
	if got := string(args[7].([]byte)); got != string(record.InputSchema) {
		t.Fatalf("argument 8 input_schema = %s, want %s", got, record.InputSchema)
	}
	if got := string(args[8].([]byte)); got != string(record.OutputSchema) {
		t.Fatalf("argument 9 output_schema = %s, want %s", got, record.OutputSchema)
	}
	if args[0] != record.Name || args[38] != createdAt || args[39] != updatedAt {
		t.Fatalf("upsert argument ordering drifted: first=%#v created=%#v updated=%#v", args[0], args[38], args[39])
	}
}

func TestBuildToolManifestUpsertArgsReportsJSONFieldErrors(t *testing.T) {
	_, err := buildToolManifestUpsertArgs(&model.ToolManifestRecord{
		Name:        "invalid_record",
		InputSchema: json.RawMessage(`{"type":`),
	})
	if err == nil || !strings.Contains(err.Error(), "input_schema") {
		t.Fatalf("expected explicit input_schema encoding error, got %v", err)
	}
}

func TestScanManifestMapsAllFortyColumns(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 4, 5, 6, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	values := []interface{}{
		"scan_tool", "Scanned tool", "external", stringPointer("3.1.0"), stringPointer("https://example.test/tool"), []byte(`{"type":"streamable_http"}`), 45000,
		[]byte(`{"type":"object","properties":{"input":{"type":"string"}}}`),
		[]byte(`{"type":"object","properties":{"output":{"type":"string"}}}`),
		[]byte(`{"input":{"type":"string"}}`), []byte(`{"output":{"type":"string"}}`), []byte(`[]`), true,
		[]byte(`["generate"]`), []byte(`["media"]`), stringPointer("medium"), stringPointer("low"), stringPointer("high"), true, false,
		[]byte(`{"required":true}`), []byte(`{"produceArtifact":true}`), stringPointer("local"), true,
		stringPointer("local"), stringPointer("LOCAL_MCP_TOOL_CALL"), []byte(`{"commands":["node"]}`), stringPointer("provider-a"),
		[]byte(`{"pagination":true}`), []byte(`["review"]`), []byte(`["timeout"]`), stringPointer("skill-a"), stringPointer("prompt-a"), []byte(`["resource-a"]`),
		stringPointer("mcp_provider"), []byte(`["when"]`), []byte(`["not"]`), []byte(`{"providerId":"provider-a"}`),
		createdAt, updatedAt,
	}
	if len(values) != 40 {
		t.Fatalf("test contract values = %d, want 40", len(values))
	}

	record, err := scanManifest(contractManifestRow{t: t, values: values})
	if err != nil {
		t.Fatalf("scan manifest: %v", err)
	}
	if record.Name != "scan_tool" || record.Version != "3.1.0" || record.TimeoutMs != 45000 {
		t.Fatalf("base fields mapped incorrectly: %#v", record)
	}
	if string(record.InputSchema) != string(values[7].([]byte)) || string(record.OutputSchema) != string(values[8].([]byte)) {
		t.Fatalf("canonical schemas mapped incorrectly: input=%s output=%s", record.InputSchema, record.OutputSchema)
	}
	if string(record.Parameters) != string(values[9].([]byte)) || string(record.Output) != string(values[10].([]byte)) {
		t.Fatalf("legacy projections mapped incorrectly: parameters=%s output=%s", record.Parameters, record.Output)
	}
	if record.Provider != "provider-a" || record.Boundary != "mcp_provider" || record.SkillPackageID != "skill-a" {
		t.Fatalf("provider metadata mapped incorrectly: %#v", record)
	}
	if !record.CreatedAt.Equal(createdAt) || !record.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("timestamps mapped incorrectly: created=%v updated=%v", record.CreatedAt, record.UpdatedAt)
	}
}

type contractManifestRow struct {
	t      *testing.T
	values []interface{}
}

func (r contractManifestRow) Scan(dest ...interface{}) error {
	r.t.Helper()
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destinations = %d, contract values = %d", len(dest), len(r.values))
	}
	for i, value := range r.values {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("destination %d is not a writable pointer", i+1)
		}
		source := reflect.ValueOf(value)
		if !source.IsValid() {
			target.Elem().Set(reflect.Zero(target.Elem().Type()))
			continue
		}
		if !source.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("column %d value type %s cannot assign to %s", i+1, source.Type(), target.Elem().Type())
		}
		target.Elem().Set(source)
	}
	return nil
}

func stringPointer(value string) *string { return &value }
