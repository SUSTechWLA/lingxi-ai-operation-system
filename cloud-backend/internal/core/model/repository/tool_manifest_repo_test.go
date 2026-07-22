package repository

import (
	"context"
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
	expectedColumns := []string{
		"name", "description", "type", "version", "endpoint", "transport", "timeout_ms",
		"input_schema", "output_schema", "parameters", "output", "examples", "sandbox", "capabilities", "tags",
		"cost_level", "latency_level", "risk_level", "side_effect", "idempotent", "approval_policy", "artifact_policy",
		"execution_plane", "requires_user_device", "artifact_location", "local_command", "local_requirements",
		"provider", "provider_capabilities", "next_recommended_tools", "failure_modes", "skill_package_id", "prompt_ref", "resource_refs",
		"boundary", "when_to_use", "when_not_to_use", "provider_binding", "created_at", "updated_at",
	}
	if !reflect.DeepEqual(toolManifestColumns, expectedColumns) {
		t.Fatalf("tool manifest column contract drifted:\n got: %#v\nwant: %#v", toolManifestColumns, expectedColumns)
	}
	if got := strings.Split(toolManifestSelectColumns, ", "); !reflect.DeepEqual(got, expectedColumns) {
		t.Fatalf("select column list does not match scan contract:\n got: %#v\nwant: %#v", got, expectedColumns)
	}
	if !strings.Contains(toolManifestUpsertSQL, toolManifestSelectColumns) ||
		!strings.Contains(toolManifestFindByNameSQL, toolManifestSelectColumns) ||
		!strings.Contains(toolManifestFindAllSQL, toolManifestSelectColumns) {
		t.Fatal("upsert/find queries do not share the tested tool manifest column contract")
	}

	valuesPattern := regexp.MustCompile(`(?s)VALUES\s*\((.*?)\)\s*ON CONFLICT`)
	valuesMatch := valuesPattern.FindStringSubmatch(toolManifestUpsertSQL)
	if len(valuesMatch) != 2 {
		t.Fatalf("could not isolate VALUES segment in upsert SQL: %s", toolManifestUpsertSQL)
	}
	placeholderPattern := regexp.MustCompile(`\$(\d+)`)
	placeholders := placeholderPattern.FindAllStringSubmatch(valuesMatch[1], -1)
	if len(placeholders) != 40 {
		t.Fatalf("VALUES placeholders = %d, want exactly 40: %q", len(placeholders), valuesMatch[1])
	}
	for index, match := range placeholders {
		position, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		want := index + 1
		if position != want {
			t.Fatalf("VALUES placeholder %d = $%d, want $%d", index+1, position, want)
		}
	}
}

func TestBuildToolManifestUpsertArgsPreservesCanonicalSchemaPositions(t *testing.T) {
	createdAt := time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	record := &model.ToolManifestRecord{
		Name:                 "sentinel-01-name",
		Description:          "sentinel-02-description",
		Type:                 "sentinel-03-type",
		Version:              "sentinel-04-version",
		Endpoint:             "sentinel-05-endpoint",
		Transport:            json.RawMessage(`{"sentinel":6}`),
		TimeoutMs:            7007,
		InputSchema:          json.RawMessage(`{"sentinel":8}`),
		OutputSchema:         json.RawMessage(`{"sentinel":9}`),
		Parameters:           json.RawMessage(`{"sentinel":10}`),
		Output:               json.RawMessage(`{"sentinel":11}`),
		Examples:             json.RawMessage(`[{"sentinel":12}]`),
		Sandbox:              true,
		Capabilities:         json.RawMessage(`[{"sentinel":14}]`),
		Tags:                 json.RawMessage(`[{"sentinel":15}]`),
		CostLevel:            "sentinel-16-cost",
		LatencyLevel:         "sentinel-17-latency",
		RiskLevel:            "sentinel-18-risk",
		SideEffect:           false,
		Idempotent:           true,
		ApprovalPolicy:       json.RawMessage(`{"sentinel":21}`),
		ArtifactPolicy:       json.RawMessage(`{"sentinel":22}`),
		ExecutionPlane:       "sentinel-23-plane",
		RequiresUserDevice:   true,
		ArtifactLocation:     "sentinel-25-location",
		LocalCommand:         "sentinel-26-command",
		LocalRequirements:    json.RawMessage(`{"sentinel":27}`),
		Provider:             "sentinel-28-provider",
		ProviderCapabilities: json.RawMessage(`{"sentinel":29}`),
		NextRecommendedTools: json.RawMessage(`[{"sentinel":30}]`),
		FailureModes:         json.RawMessage(`[{"sentinel":31}]`),
		SkillPackageID:       "sentinel-32-skill",
		PromptRef:            "sentinel-33-prompt",
		ResourceRefs:         json.RawMessage(`[{"sentinel":34}]`),
		Boundary:             "sentinel-35-boundary",
		WhenToUse:            json.RawMessage(`[{"sentinel":36}]`),
		WhenNotToUse:         json.RawMessage(`[{"sentinel":37}]`),
		ProviderBinding:      json.RawMessage(`{"sentinel":38}`),
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}

	args, err := buildToolManifestUpsertArgs(record)
	if err != nil {
		t.Fatalf("build upsert args: %v", err)
	}
	if len(args) != 40 {
		t.Fatalf("upsert args = %d, want 40", len(args))
	}
	expected := []interface{}{
		record.Name, record.Description, record.Type, record.Version, record.Endpoint, []byte(record.Transport), record.TimeoutMs,
		[]byte(record.InputSchema), []byte(record.OutputSchema), []byte(record.Parameters), []byte(record.Output), []byte(record.Examples), record.Sandbox,
		[]byte(record.Capabilities), []byte(record.Tags), record.CostLevel, record.LatencyLevel, record.RiskLevel,
		record.SideEffect, record.Idempotent, []byte(record.ApprovalPolicy), []byte(record.ArtifactPolicy),
		record.ExecutionPlane, record.RequiresUserDevice, record.ArtifactLocation, record.LocalCommand, []byte(record.LocalRequirements),
		record.Provider, []byte(record.ProviderCapabilities), []byte(record.NextRecommendedTools), []byte(record.FailureModes),
		record.SkillPackageID, record.PromptRef, []byte(record.ResourceRefs), record.Boundary, []byte(record.WhenToUse),
		[]byte(record.WhenNotToUse), []byte(record.ProviderBinding), record.CreatedAt, record.UpdatedAt,
	}
	for index := range expected {
		if !reflect.DeepEqual(args[index], expected[index]) {
			t.Fatalf("upsert argument $%d = %#v, want sentinel %#v", index+1, args[index], expected[index])
		}
	}
}

func TestToolManifestRepositoryUpsertRejectsNilRecord(t *testing.T) {
	repository := &ToolManifestRepository{}
	err := repository.Upsert(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected explicit nil record error, got %v", err)
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
	actual := []interface{}{
		record.Name, record.Description, record.Type, record.Version, record.Endpoint, []byte(record.Transport), record.TimeoutMs,
		[]byte(record.InputSchema), []byte(record.OutputSchema), []byte(record.Parameters), []byte(record.Output), []byte(record.Examples), record.Sandbox,
		[]byte(record.Capabilities), []byte(record.Tags), record.CostLevel, record.LatencyLevel, record.RiskLevel,
		record.SideEffect, record.Idempotent, []byte(record.ApprovalPolicy), []byte(record.ArtifactPolicy),
		record.ExecutionPlane, record.RequiresUserDevice, record.ArtifactLocation, record.LocalCommand, []byte(record.LocalRequirements),
		record.Provider, []byte(record.ProviderCapabilities), []byte(record.NextRecommendedTools), []byte(record.FailureModes),
		record.SkillPackageID, record.PromptRef, []byte(record.ResourceRefs), record.Boundary, []byte(record.WhenToUse),
		[]byte(record.WhenNotToUse), []byte(record.ProviderBinding), record.CreatedAt, record.UpdatedAt,
	}
	expected := []interface{}{
		values[0], values[1], values[2], *values[3].(*string), *values[4].(*string), values[5], values[6],
		values[7], values[8], values[9], values[10], values[11], values[12], values[13], values[14],
		*values[15].(*string), *values[16].(*string), *values[17].(*string), values[18], values[19], values[20], values[21],
		*values[22].(*string), values[23], *values[24].(*string), *values[25].(*string), values[26], *values[27].(*string),
		values[28], values[29], values[30], *values[31].(*string), *values[32].(*string), values[33], *values[34].(*string),
		values[35], values[36], values[37], values[38], values[39],
	}
	if len(actual) != 40 || len(expected) != 40 {
		t.Fatalf("scan assertion contract lengths: actual=%d expected=%d", len(actual), len(expected))
	}
	for index := range expected {
		if !reflect.DeepEqual(actual[index], expected[index]) {
			t.Fatalf("scanned column %d (%s) = %#v, want %#v", index+1, toolManifestColumns[index], actual[index], expected[index])
		}
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
