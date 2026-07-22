package tool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestToolManifestRecordRoundTripPreservesCanonicalSchemas(t *testing.T) {
	inputSchema := map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
		"$defs": map[string]interface{}{
			"asset": map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"url": map[string]interface{}{"type": "string", "format": "uri"}},
				"required":             []interface{}{"url"},
				"additionalProperties": false,
			},
		},
		"properties": map[string]interface{}{
			"source": map[string]interface{}{"$ref": "#/$defs/asset"},
			"mode": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"const": "fast"},
					map[string]interface{}{"enum": []interface{}{"quality", "balanced"}},
				},
			},
		},
		"required":             []interface{}{"source"},
		"additionalProperties": false,
	}
	outputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"$ref": "#/$defs/asset"},
				},
			},
		},
		"$defs":                inputSchema["$defs"],
		"additionalProperties": false,
	}
	legacyParameters := map[string]ParamDef{
		"source": {Type: "object", Description: "Source asset", Required: true},
	}
	legacyOutput := map[string]ParamDef{
		"result": {Type: "string", Description: "Result reference"},
	}

	record, err := manifestToRecord(&ToolManifest{
		Name:         "canonical_round_trip",
		Description:  "Canonical schema round trip",
		Type:         "builtin",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Parameters:   legacyParameters,
		Output:       legacyOutput,
	})
	if err != nil {
		t.Fatalf("convert manifest: %v", err)
	}
	restored, err := manifestFromRecord(record)
	if err != nil {
		t.Fatalf("restore manifest: %v", err)
	}

	if !reflect.DeepEqual(restored.InputSchema, inputSchema) {
		t.Fatalf("input schema changed during round trip:\n got: %#v\nwant: %#v", restored.InputSchema, inputSchema)
	}
	if !reflect.DeepEqual(restored.OutputSchema, outputSchema) {
		t.Fatalf("output schema changed during round trip:\n got: %#v\nwant: %#v", restored.OutputSchema, outputSchema)
	}
	if !reflect.DeepEqual(restored.Parameters, legacyParameters) || !reflect.DeepEqual(restored.Output, legacyOutput) {
		t.Fatalf("legacy projections changed during round trip: parameters=%#v output=%#v", restored.Parameters, restored.Output)
	}
}

func TestManifestFromRecordDerivesCanonicalSchemasForLegacyRows(t *testing.T) {
	parameters, err := json.Marshal(map[string]ParamDef{
		"prompt": {Type: "string", Description: "Generation prompt", Required: true, Enum: []string{"short", "long"}},
		"seed":   {Type: "integer", Description: "Optional seed", Default: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := json.Marshal(map[string]ParamDef{
		"url": {Type: "string", Description: "Generated asset URL"},
	})
	if err != nil {
		t.Fatal(err)
	}

	restored, err := manifestFromRecord(&model.ToolManifestRecord{
		Name:         "legacy_tool",
		Description:  "Legacy record",
		Type:         "builtin",
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
		Parameters:   parameters,
		Output:       output,
	})
	if err != nil {
		t.Fatalf("restore legacy manifest: %v", err)
	}

	if restored.InputSchema["type"] != "object" || restored.InputSchema["additionalProperties"] != false {
		t.Fatalf("legacy input was not converted to a closed object schema: %#v", restored.InputSchema)
	}
	properties, ok := restored.InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("legacy input properties missing: %#v", restored.InputSchema)
	}
	if properties["prompt"].(map[string]interface{})["type"] != "string" {
		t.Fatalf("legacy prompt property was not preserved: %#v", properties["prompt"])
	}
	required, ok := restored.InputSchema["required"].([]interface{})
	if !ok || len(required) != 1 || required[0] != "prompt" {
		t.Fatalf("legacy required flags were not promoted: %#v", restored.InputSchema["required"])
	}
	if restored.OutputSchema["type"] != "object" || restored.OutputSchema["additionalProperties"] != false {
		t.Fatalf("legacy output was not converted to a closed object schema: %#v", restored.OutputSchema)
	}
	outputProperties, ok := restored.OutputSchema["properties"].(map[string]interface{})
	if !ok || outputProperties["url"].(map[string]interface{})["type"] != "string" ||
		outputProperties["url"].(map[string]interface{})["description"] != "Generated asset URL" {
		t.Fatalf("legacy output properties were not preserved: %#v", restored.OutputSchema["properties"])
	}
	outputRequired, ok := restored.OutputSchema["required"].([]interface{})
	if !ok || len(outputRequired) != 0 {
		t.Fatalf("legacy output without required fields must expose an empty required array: %#v", restored.OutputSchema["required"])
	}
	if !reflect.DeepEqual(restored.Parameters["prompt"].Enum, []string{"short", "long"}) || restored.Parameters["seed"].Default == nil {
		t.Fatalf("legacy parameter projection was not retained: %#v", restored.Parameters)
	}
}

func TestManifestFromRecordPromotesRequiredLegacyOutputFields(t *testing.T) {
	output := json.RawMessage(`{
		"assetUrl":{"type":"string","description":"Generated asset URL","required":true},
		"duration":{"type":"number","description":"Duration in seconds","required":false}
	}`)

	restored, err := manifestFromRecord(&model.ToolManifestRecord{
		Name:         "legacy_required_output",
		Description:  "Legacy output required flags",
		Type:         "builtin",
		OutputSchema: json.RawMessage(`{}`),
		Output:       output,
	})
	if err != nil {
		t.Fatalf("restore legacy manifest: %v", err)
	}

	if restored.OutputSchema["type"] != "object" || restored.OutputSchema["additionalProperties"] != false {
		t.Fatalf("legacy output was not converted to a closed object schema: %#v", restored.OutputSchema)
	}
	properties, ok := restored.OutputSchema["properties"].(map[string]interface{})
	if !ok || len(properties) != 2 || properties["duration"].(map[string]interface{})["type"] != "number" {
		t.Fatalf("legacy output properties were not preserved: %#v", restored.OutputSchema["properties"])
	}
	required, ok := restored.OutputSchema["required"].([]interface{})
	if !ok || !reflect.DeepEqual(required, []interface{}{"assetUrl"}) {
		t.Fatalf("legacy output required flags were not promoted: %#v", restored.OutputSchema["required"])
	}
}

func TestManifestFromRecordPrefersCanonicalSchemasAndRetainsLegacyProjections(t *testing.T) {
	canonicalInput := json.RawMessage(`{"type":"object","properties":{"canonical":{"type":"boolean"}},"additionalProperties":true}`)
	canonicalOutput := json.RawMessage(`{"type":"object","properties":{"status":{"enum":["ready","pending"]}}}`)
	parameters := json.RawMessage(`{"legacy":{"type":"string","description":"Legacy input","required":true}}`)
	output := json.RawMessage(`{"legacyResult":{"type":"number","description":"Legacy output","required":false}}`)

	restored, err := manifestFromRecord(&model.ToolManifestRecord{
		Name:         "canonical_priority",
		Description:  "Canonical wins",
		Type:         "builtin",
		InputSchema:  canonicalInput,
		OutputSchema: canonicalOutput,
		Parameters:   parameters,
		Output:       output,
	})
	if err != nil {
		t.Fatalf("restore manifest: %v", err)
	}

	properties := restored.InputSchema["properties"].(map[string]interface{})
	if _, ok := properties["canonical"]; !ok {
		t.Fatalf("canonical input schema was not preferred: %#v", restored.InputSchema)
	}
	if _, ok := properties["legacy"]; ok {
		t.Fatalf("legacy projection replaced canonical input schema: %#v", restored.InputSchema)
	}
	if restored.Parameters["legacy"].Description != "Legacy input" || restored.Output["legacyResult"].Type != "number" {
		t.Fatalf("legacy projections were not retained: parameters=%#v output=%#v", restored.Parameters, restored.Output)
	}
}

func TestManifestToRecord_PreservesAgentRuntimePolicyFields(t *testing.T) {
	record, err := manifestToRecord(&ToolManifest{
		Name:               "video_script_generator",
		Description:        "Generate a reviewable script",
		Type:               "builtin_prompt_tool",
		Version:            "1.0.0",
		ExecutionPlane:     ExecutionPlaneLocal,
		RequiresUserDevice: true,
		ArtifactLocation:   ArtifactLocationLocal,
		LocalCommand:       "HYPERFRAMES_RENDER",
		LocalRequirements: LocalRequirements{
			OS:              []string{"darwin", "linux"},
			Commands:        []string{"node", "ffmpeg"},
			MinDiskMb:       2048,
			RequiresNetwork: false,
		},
		Capabilities:   []string{"video_creation", "script_generation"},
		Tags:           []string{"video", "script"},
		CostLevel:      CostLow,
		LatencyLevel:   LatencyMedium,
		RiskLevel:      RiskLow,
		SideEffect:     false,
		Idempotent:     true,
		SkillPackageID: "codex-video-skill",
		PromptRef:      "prompts/script_generator.prompt.md",
		ResourceRefs:   []string{"script_rules", "style_reference"},
		FailureModes:   []string{"llm_timeout"},
		NextRecommendedTools: []string{
			"shot_splitter",
			"publish_copy_generator",
		},
		ApprovalPolicy: ApprovalPolicy{
			Required:            true,
			Mode:                ApprovalAfterArtifact,
			BlocksDownstream:    true,
			Reason:              "Script drives downstream generation",
			ReviewArtifactKinds: []string{"MARKDOWN"},
		},
		ArtifactPolicy: ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"MARKDOWN"},
			DefaultReviewRequired: true,
		},
	})
	if err != nil {
		t.Fatalf("convert manifest: %v", err)
	}

	var capabilities []string
	if err := json.Unmarshal(record.Capabilities, &capabilities); err != nil {
		t.Fatalf("capabilities not valid JSON: %v", err)
	}
	if len(capabilities) != 2 || capabilities[1] != "script_generation" {
		t.Fatalf("capabilities not preserved: %#v", capabilities)
	}

	if record.CostLevel != CostLow || record.LatencyLevel != LatencyMedium || record.RiskLevel != RiskLow {
		t.Fatalf("levels not preserved: cost=%q latency=%q risk=%q", record.CostLevel, record.LatencyLevel, record.RiskLevel)
	}
	if !record.Idempotent || record.SideEffect {
		t.Fatalf("side effect/idempotency flags wrong: sideEffect=%v idempotent=%v", record.SideEffect, record.Idempotent)
	}
	if record.SkillPackageID != "codex-video-skill" || record.PromptRef == "" {
		t.Fatalf("skill capability metadata not preserved: %#v", record)
	}
	if record.ExecutionPlane != ExecutionPlaneLocal || !record.RequiresUserDevice || record.ArtifactLocation != ArtifactLocationLocal {
		t.Fatalf("edge execution metadata not preserved: %#v", record)
	}
	if record.LocalCommand != "HYPERFRAMES_RENDER" {
		t.Fatalf("local command not preserved: %#v", record)
	}

	var localRequirements LocalRequirements
	if err := json.Unmarshal(record.LocalRequirements, &localRequirements); err != nil {
		t.Fatalf("local requirements not valid JSON: %v", err)
	}
	if localRequirements.MinDiskMb != 2048 || len(localRequirements.Commands) != 2 || localRequirements.Commands[1] != "ffmpeg" {
		t.Fatalf("local requirements not preserved: %#v", localRequirements)
	}

	var approval ApprovalPolicy
	if err := json.Unmarshal(record.ApprovalPolicy, &approval); err != nil {
		t.Fatalf("approval policy not valid JSON: %v", err)
	}
	if !approval.Required || approval.Mode != ApprovalAfterArtifact || !approval.BlocksDownstream {
		t.Fatalf("approval policy not preserved: %#v", approval)
	}
}

func TestManifestToRecordDefaultsArtifactLocationToLocalOnly(t *testing.T) {
	record, err := manifestToRecord(&ToolManifest{
		Name:        "proposal_generator",
		Description: "Generate a proposal",
		Type:        "builtin_prompt_tool",
		ArtifactPolicy: ArtifactPolicy{
			ProduceArtifact: true,
			ArtifactKinds:   []string{"VIDEO_PROPOSAL"},
		},
	})
	if err != nil {
		t.Fatalf("convert manifest: %v", err)
	}

	if record.ArtifactLocation != ArtifactLocationLocal {
		t.Fatalf("artifact location default = %q, want %q", record.ArtifactLocation, ArtifactLocationLocal)
	}
	var policy ArtifactPolicy
	if err := json.Unmarshal(record.ArtifactPolicy, &policy); err != nil {
		t.Fatalf("artifact policy not valid JSON: %v", err)
	}
	if policy.Storage != ArtifactLocationLocal {
		t.Fatalf("artifact policy storage = %q, want %q", policy.Storage, ArtifactLocationLocal)
	}
	if policy.SyncFileToCloud {
		t.Fatalf("local-only artifact policy must not sync files to cloud: %#v", policy)
	}
}

func TestManifestToRecordRejectsUnserializableCanonicalSchema(t *testing.T) {
	_, err := manifestToRecord(&ToolManifest{
		Name:        "invalid_schema",
		Description: "Invalid canonical schema",
		Type:        "external",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"invalidKey": func() {},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "input_schema") {
		t.Fatalf("expected explicit input_schema serialization error, got %v", err)
	}
}

func TestRegisterExternalRejectsInvalidSchemaWithoutCallingRepository(t *testing.T) {
	repo := &stubToolManifestRepo{}
	registry := NewToolRegistry()
	service := newTestToolManifestService(repo, &stubToolManifestCache{getErr: redis.Nil}, registry)

	err := service.RegisterExternal(context.Background(), &ToolManifest{
		Name:        "invalid_external",
		Description: "Invalid external schema",
		Type:        "external",
		InputSchema: map[string]interface{}{"type": "object", "invalidKey": make(chan string)},
	})
	if err == nil || !strings.Contains(err.Error(), "input_schema") {
		t.Fatalf("expected explicit input_schema registration error, got %v", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("repository called for invalid manifest: %d", repo.upsertCalls)
	}
	if registry.GetExternalManifest("invalid_external") != nil {
		t.Fatal("invalid manifest was registered in memory")
	}
}

func TestSyncBuiltinToolsReturnsObservableSchemaErrors(t *testing.T) {
	repo := &stubToolManifestRepo{}
	registry := NewToolRegistry()
	registry.RegisterExternal(&ToolManifest{
		Name:         "invalid_sync_tool",
		Description:  "Invalid schema in sync",
		Type:         "external",
		OutputSchema: map[string]interface{}{"type": "object", "invalidKey": make(chan int)},
	})
	service := newTestToolManifestService(repo, &stubToolManifestCache{getErr: redis.Nil}, registry)

	err := service.SyncBuiltinTools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid_sync_tool") || !strings.Contains(err.Error(), "output_schema") {
		t.Fatalf("expected observable per-tool schema error, got %v", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("repository called for invalid synced manifest: %d", repo.upsertCalls)
	}
}

func TestListAllDerivesCanonicalSchemasForLegacyRepositoryRows(t *testing.T) {
	legacy := legacyToolManifestRecord(t, "legacy_repo_tool")
	repo := &stubToolManifestRepo{records: []*model.ToolManifestRecord{legacy}}
	cache := &stubToolManifestCache{getErr: redis.Nil}
	service := newTestToolManifestService(repo, cache, NewToolRegistry())

	records, err := service.ListAll(context.Background())
	if err != nil {
		t.Fatalf("list manifests: %v", err)
	}
	if repo.findAllCalls != 1 {
		t.Fatalf("repository find calls = %d, want 1", repo.findAllCalls)
	}
	assertRecordHasDerivedCanonicalSchemas(t, records[0])
	if records[0].Description != legacy.Description || records[0].Boundary != legacy.Boundary {
		t.Fatalf("normalization lost record metadata: %#v", records[0])
	}
}

func TestListAllDerivesCanonicalSchemasForLegacyCacheRows(t *testing.T) {
	legacy := legacyToolManifestRecord(t, "legacy_cache_tool")
	cached, err := json.Marshal([]*model.ToolManifestRecord{legacy})
	if err != nil {
		t.Fatal(err)
	}
	repo := &stubToolManifestRepo{}
	cache := &stubToolManifestCache{data: cached}
	service := newTestToolManifestService(repo, cache, NewToolRegistry())

	records, err := service.ListAll(context.Background())
	if err != nil {
		t.Fatalf("list cached manifests: %v", err)
	}
	if repo.findAllCalls != 0 {
		t.Fatalf("valid cache should not query repository: %d", repo.findAllCalls)
	}
	assertRecordHasDerivedCanonicalSchemas(t, records[0])
}

func TestListAllFallsBackToRepositoryWhenCachedRecordIsInvalid(t *testing.T) {
	repoRecord := legacyToolManifestRecord(t, "repo_fallback_tool")
	repo := &stubToolManifestRepo{records: []*model.ToolManifestRecord{repoRecord}}
	cache := &stubToolManifestCache{
		data: []byte(`[{"name":"invalid_cache_tool","description":"Invalid cached record","type":"external","input_schema":"not-an-object"}]`),
	}
	service := newTestToolManifestService(repo, cache, NewToolRegistry())

	records, err := service.ListAll(context.Background())
	if err != nil {
		t.Fatalf("list manifests with invalid cache fallback: %v", err)
	}
	if repo.findAllCalls != 1 {
		t.Fatalf("invalid cache did not fall back to repository: find calls=%d", repo.findAllCalls)
	}
	if len(records) != 1 || records[0].Name != "repo_fallback_tool" {
		t.Fatalf("repository fallback records not returned: %#v", records)
	}
	assertRecordHasDerivedCanonicalSchemas(t, records[0])
}

type stubToolManifestRepo struct {
	records      []*model.ToolManifestRecord
	upsertCalls  int
	findAllCalls int
}

func (r *stubToolManifestRepo) Upsert(context.Context, *model.ToolManifestRecord) error {
	r.upsertCalls++
	return nil
}

func (r *stubToolManifestRepo) FindByName(context.Context, string) (*model.ToolManifestRecord, error) {
	return nil, nil
}

func (r *stubToolManifestRepo) FindAll(context.Context) ([]*model.ToolManifestRecord, error) {
	r.findAllCalls++
	return r.records, nil
}

func (r *stubToolManifestRepo) Delete(context.Context, string) error { return nil }

type stubToolManifestCache struct {
	data   []byte
	getErr error
}

func newTestToolManifestService(repo *stubToolManifestRepo, cache toolManifestCache, registry *ToolRegistry) *ToolManifestService {
	return &ToolManifestService{repo: repo, rdb: cache, registry: registry}
}

func (c *stubToolManifestCache) Get(context.Context, string) *redis.StringCmd {
	if c.getErr != nil {
		return redis.NewStringResult("", c.getErr)
	}
	return redis.NewStringResult(string(c.data), nil)
}

func (c *stubToolManifestCache) Set(_ context.Context, _ string, value interface{}, _ time.Duration) *redis.StatusCmd {
	switch typed := value.(type) {
	case []byte:
		c.data = append([]byte(nil), typed...)
	case string:
		c.data = []byte(typed)
	default:
		return redis.NewStatusResult("", errors.New("unsupported cache value"))
	}
	return redis.NewStatusResult("OK", nil)
}

func (c *stubToolManifestCache) Del(context.Context, ...string) *redis.IntCmd {
	c.data = nil
	return redis.NewIntResult(1, nil)
}

func legacyToolManifestRecord(t *testing.T, name string) *model.ToolManifestRecord {
	t.Helper()
	parameters, err := json.Marshal(map[string]ParamDef{
		"prompt": {Type: "string", Description: "Prompt", Required: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := json.Marshal(map[string]ParamDef{
		"assetUrl": {Type: "string", Description: "Asset URL", Required: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &model.ToolManifestRecord{
		Name:         name,
		Description:  "Legacy manifest metadata",
		Type:         "external",
		Boundary:     BoundaryLegacy,
		InputSchema:  json.RawMessage(`{}`),
		OutputSchema: json.RawMessage(`{}`),
		Parameters:   parameters,
		Output:       output,
	}
}

func assertRecordHasDerivedCanonicalSchemas(t *testing.T, record *model.ToolManifestRecord) {
	t.Helper()
	var inputSchema map[string]interface{}
	if err := json.Unmarshal(record.InputSchema, &inputSchema); err != nil {
		t.Fatalf("decode derived input schema: %v", err)
	}
	var outputSchema map[string]interface{}
	if err := json.Unmarshal(record.OutputSchema, &outputSchema); err != nil {
		t.Fatalf("decode derived output schema: %v", err)
	}
	if inputSchema["type"] != "object" || outputSchema["type"] != "object" {
		t.Fatalf("canonical schemas were not derived: input=%#v output=%#v", inputSchema, outputSchema)
	}
	if !reflect.DeepEqual(inputSchema["required"], []interface{}{"prompt"}) ||
		!reflect.DeepEqual(outputSchema["required"], []interface{}{"assetUrl"}) {
		t.Fatalf("derived schema required arrays are wrong: input=%#v output=%#v", inputSchema, outputSchema)
	}
}
