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
capabilities:
  - video_creation
  - script_generation
costLevel: low
approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: Script must be reviewed before shots are created.
artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - MARKDOWN
  defaultReviewRequired: true
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
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
