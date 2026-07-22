package tool

import (
	"encoding/json"
	"reflect"
	"testing"

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

	record := manifestToRecord(&ToolManifest{
		Name:         "canonical_round_trip",
		Description:  "Canonical schema round trip",
		Type:         "builtin",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Parameters:   legacyParameters,
		Output:       legacyOutput,
	})
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
	if !reflect.DeepEqual(restored.Parameters["prompt"].Enum, []string{"short", "long"}) || restored.Parameters["seed"].Default == nil {
		t.Fatalf("legacy parameter projection was not retained: %#v", restored.Parameters)
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
	record := manifestToRecord(&ToolManifest{
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
	record := manifestToRecord(&ToolManifest{
		Name:        "proposal_generator",
		Description: "Generate a proposal",
		Type:        "builtin_prompt_tool",
		ArtifactPolicy: ArtifactPolicy{
			ProduceArtifact: true,
			ArtifactKinds:   []string{"VIDEO_PROPOSAL"},
		},
	})

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
