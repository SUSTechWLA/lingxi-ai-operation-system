package service

import (
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestManifestToRecord_PreservesAgentRuntimePolicyFields(t *testing.T) {
	record := manifestToRecord(&tool.ToolManifest{
		Name:           "video_script_generator",
		Description:    "Generate a reviewable script",
		Type:           "builtin_prompt_tool",
		Version:        "1.0.0",
		Capabilities:   []string{"video_creation", "script_generation"},
		Tags:           []string{"video", "script"},
		CostLevel:      tool.CostLow,
		LatencyLevel:   tool.LatencyMedium,
		RiskLevel:      tool.RiskLow,
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
		ApprovalPolicy: tool.ApprovalPolicy{
			Required:            true,
			Mode:                tool.ApprovalAfterArtifact,
			BlocksDownstream:    true,
			Reason:              "Script drives downstream generation",
			ReviewArtifactKinds: []string{"MARKDOWN"},
		},
		ArtifactPolicy: tool.ArtifactPolicy{
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

	if record.CostLevel != tool.CostLow || record.LatencyLevel != tool.LatencyMedium || record.RiskLevel != tool.RiskLow {
		t.Fatalf("levels not preserved: cost=%q latency=%q risk=%q", record.CostLevel, record.LatencyLevel, record.RiskLevel)
	}
	if !record.Idempotent || record.SideEffect {
		t.Fatalf("side effect/idempotency flags wrong: sideEffect=%v idempotent=%v", record.SideEffect, record.Idempotent)
	}
	if record.SkillPackageID != "codex-video-skill" || record.PromptRef == "" {
		t.Fatalf("skill capability metadata not preserved: %#v", record)
	}

	var approval tool.ApprovalPolicy
	if err := json.Unmarshal(record.ApprovalPolicy, &approval); err != nil {
		t.Fatalf("approval policy not valid JSON: %v", err)
	}
	if !approval.Required || approval.Mode != tool.ApprovalAfterArtifact || !approval.BlocksDownstream {
		t.Fatalf("approval policy not preserved: %#v", approval)
	}
}
