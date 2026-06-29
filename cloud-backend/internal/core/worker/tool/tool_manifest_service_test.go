package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

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

func TestRestorePersistedManifests_ReloadsExternalToolsIntoRegistry(t *testing.T) {
	record := manifestToRecord(&ToolManifest{
		Name:         "parse_bid_files",
		Description:  "Parse bid files",
		Type:         "http",
		Endpoint:     "http://127.0.0.1:9001/tools/parse_bid_files",
		Capabilities: []string{"bid_writing", "bid_parsing"},
		Parameters: map[string]ParamDef{
			"file_path": {Type: "string", Required: true},
		},
		Output: map[string]ParamDef{
			"stdout": {Type: "string"},
		},
	})
	builtinRecord := manifestToRecord(&ToolManifest{
		Name:        "llm_api",
		Description: "built in",
		Type:        "builtin",
	})
	registry := NewToolRegistry()
	service := NewToolManifestService(&fakeManifestRepo{records: []*model.ToolManifestRecord{record, builtinRecord}}, nil, registry)

	if err := service.RestorePersistedManifests(context.Background()); err != nil {
		t.Fatalf("RestorePersistedManifests returned error: %v", err)
	}
	restored := registry.GetManifest("parse_bid_files")
	if restored == nil {
		t.Fatalf("parse_bid_files should be restored")
	}
	if restored.Endpoint != "http://127.0.0.1:9001/tools/parse_bid_files" {
		t.Fatalf("endpoint not restored: %#v", restored)
	}
	if len(restored.Capabilities) != 2 || restored.Capabilities[0] != "bid_writing" {
		t.Fatalf("capabilities not restored: %#v", restored.Capabilities)
	}
	if registry.GetExternalManifest("llm_api") != nil {
		t.Fatalf("builtin records should not be restored as external manifests")
	}
}

type fakeManifestRepo struct {
	records []*model.ToolManifestRecord
}

func (r *fakeManifestRepo) Upsert(context.Context, *model.ToolManifestRecord) error { return nil }

func (r *fakeManifestRepo) FindByName(_ context.Context, name string) (*model.ToolManifestRecord, error) {
	for _, record := range r.records {
		if record.Name == name {
			return record, nil
		}
	}
	return nil, nil
}

func (r *fakeManifestRepo) FindAll(context.Context) ([]*model.ToolManifestRecord, error) {
	return r.records, nil
}

func (r *fakeManifestRepo) Delete(context.Context, string) error { return nil }
