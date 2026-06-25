package skillcapability

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestLoadCapabilities_LoadsSkillCapabilityAndToolManifests(t *testing.T) {
	root := t.TempDir()
	capDir := filepath.Join(root, "video", "codex-video-skill", "1.0.0")
	if err := os.MkdirAll(filepath.Join(capDir, "tools"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(capDir, "skillcap.yaml"), `
id: codex-video-skill
name: Codex Video Skill
version: 1.0.0
domain: video_creation
description: Reviewable video creation capability package.
activation:
  intents:
    - video_creation
resources:
  - id: script_rules
    type: rule
    path: rules/script_rules.md
    scope: video_script_generator
    priority: 90
tools:
  - id: video_script_generator
    manifest: tools/video_script_generator.tool.yaml
    prompt: prompts/script_generator.prompt.md
`)
	writeFile(t, filepath.Join(capDir, "tools", "video_script_generator.tool.yaml"), `
name: video_script_generator
description: Generate a short video script.
type: builtin_prompt_tool
endpoint: builtin://video-creation/video_script_generator
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_RENDER
localRequirements:
  os:
    - darwin
    - linux
  commands:
    - node
    - ffmpeg
  minDiskMb: 2048
  requiresNetwork: false
capabilities:
  - video_creation
  - script_generation
costLevel: low
approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: Script must be reviewed before shots are created.
humanReview:
  required: true
  gate: after_artifact
  title: 审核口播脚本
  reviewFocus:
    - 开头是否有吸引力
    - 口播是否自然流畅
  userActions:
    - approve
    - edit
    - regenerate
artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - MARKDOWN
  defaultReviewRequired: true
qualityPolicy:
  required: true
  checkerTool: script_quality_checker
  minScore: 85
  autoRepair: true
  maxRepairAttempts: 2
nextRecommendedTools:
  - shot_splitter
`)

	reg, manifests, errs := LoadCapabilities(root)
	if len(errs) != 0 {
		t.Fatalf("LoadCapabilities returned errors: %v", errs)
	}
	caps := reg.List()
	if len(caps) != 1 {
		t.Fatalf("expected one capability, got %d", len(caps))
	}
	if caps[0].ID != "codex-video-skill" || caps[0].Domain != "video_creation" {
		t.Fatalf("capability not loaded: %#v", caps[0])
	}
	if len(manifests) != 1 {
		t.Fatalf("expected one tool manifest, got %d", len(manifests))
	}
	manifest := manifests[0]
	if manifest.SkillPackageID != "codex-video-skill" {
		t.Fatalf("tool manifest not linked to skill package: %#v", manifest)
	}
	if manifest.PromptRef != "prompts/script_generator.prompt.md" {
		t.Fatalf("prompt ref not copied from tool declaration: %#v", manifest)
	}
	if manifest.ApprovalPolicy.Mode != tool.ApprovalAfterArtifact || !manifest.ArtifactPolicy.DefaultReviewRequired {
		t.Fatalf("review policy not loaded: %#v", manifest)
	}
	if !manifest.QualityPolicy.Required || manifest.QualityPolicy.CheckerTool != "script_quality_checker" || manifest.QualityPolicy.MinScore != 85 {
		t.Fatalf("quality policy not loaded: %#v", manifest.QualityPolicy)
	}
	if !manifest.QualityPolicy.AutoRepair || manifest.QualityPolicy.MaxRepairAttempts != 2 {
		t.Fatalf("quality repair policy not loaded: %#v", manifest.QualityPolicy)
	}
	if manifest.ExecutionPlane != tool.ExecutionPlaneLocal || !manifest.RequiresUserDevice || manifest.ArtifactLocation != tool.ArtifactLocationLocal {
		t.Fatalf("execution plane policy not loaded: %#v", manifest)
	}
	if manifest.LocalCommand != "HYPERFRAMES_RENDER" || manifest.LocalRequirements.MinDiskMb != 2048 {
		t.Fatalf("local execution requirements not loaded: %#v", manifest.LocalRequirements)
	}
	if len(manifest.LocalRequirements.Commands) != 2 || manifest.LocalRequirements.Commands[1] != "ffmpeg" {
		t.Fatalf("local command requirements not loaded: %#v", manifest.LocalRequirements.Commands)
	}
	if manifest.HumanReview == nil {
		t.Fatal("humanReview not loaded")
	}
	if !manifest.HumanReview.Required || manifest.HumanReview.Title != "审核口播脚本" {
		t.Fatalf("humanReview not loaded correctly: %#v", manifest.HumanReview)
	}
	if len(manifest.HumanReview.ReviewFocus) != 2 || len(manifest.HumanReview.UserActions) != 3 {
		t.Fatalf("humanReview reviewFocus/userActions not loaded: %#v", manifest.HumanReview)
	}
}

func TestBundledVideoCapabilityMarksHyperFramesProjectGeneratorLocal(t *testing.T) {
	root := filepath.Join("..", "..", "..", "skill-capabilities")
	_, manifests, errs := LoadCapabilities(root)
	if len(errs) != 0 {
		t.Fatalf("LoadCapabilities returned errors: %v", errs)
	}
	for _, manifest := range manifests {
		if manifest.Name != "hyperframes_project_generator" {
			continue
		}
		if manifest.ExecutionPlane != tool.ExecutionPlaneLocal || manifest.LocalCommand != "HYPERFRAMES_PROJECT_GENERATE" {
			t.Fatalf("hyperframes_project_generator must run locally: %#v", manifest)
		}
		if !manifest.RequiresUserDevice || manifest.ArtifactLocation != tool.ArtifactLocationLocal {
			t.Fatalf("hyperframes_project_generator local metadata incomplete: %#v", manifest)
		}
		return
	}
	t.Fatal("hyperframes_project_generator manifest not found")
}

func TestBundledVideoHumanReviewLoaded(t *testing.T) {
	root := filepath.Join("..", "..", "..", "skill-capabilities")
	_, manifests, errs := LoadCapabilities(root)
	if len(errs) != 0 {
		t.Fatalf("LoadCapabilities returned errors: %v", errs)
	}
	for _, manifest := range manifests {
		switch manifest.Name {
		case "hyperframes_renderer", "hyperframes_snapshot":
			if manifest.HumanReview == nil {
				t.Fatalf("%s: humanReview not loaded", manifest.Name)
			}
			if !manifest.HumanReview.Required {
				t.Fatalf("%s: humanReview.required should be true", manifest.Name)
			}
			if manifest.HumanReview.Title == "" {
				t.Fatalf("%s: humanReview.title is empty", manifest.Name)
			}
			if len(manifest.HumanReview.ReviewFocus) == 0 {
				t.Fatalf("%s: humanReview.reviewFocus is empty", manifest.Name)
			}
			if len(manifest.HumanReview.UserActions) == 0 {
				t.Fatalf("%s: humanReview.userActions is empty", manifest.Name)
			}
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
