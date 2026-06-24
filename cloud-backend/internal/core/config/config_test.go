package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestVideoConfigDefaults(t *testing.T) {
	cfg := &Config{
		Video: VideoConfig{},
	}

	if cfg.Video.VideoCreationEnabled {
		t.Error("VIDEO_CREATION_ENABLED should default to false")
	}
	if cfg.Video.LocalRunnerEnabled {
		t.Error("LOCAL_RUNNER_ENABLED should default to false")
	}
	if cfg.Video.ModelProviderMode != "" && cfg.Video.ModelProviderMode != "fake" {
		t.Errorf("MODEL_PROVIDER_MODE should default to 'fake', got %q", cfg.Video.ModelProviderMode)
	}
}

func TestVideoConfigStruct(t *testing.T) {
	cfg := &Config{
		Video: VideoConfig{
			VideoCreationEnabled: true,
			LocalRunnerEnabled:   true,
			ModelProviderMode:    "fake",
			SkillRoot:            "skills",
		},
	}

	if !cfg.Video.VideoCreationEnabled {
		t.Error("VIDEO_CREATION_ENABLED should be true")
	}
	if cfg.Video.ModelProviderMode != "fake" {
		t.Errorf("MODEL_PROVIDER_MODE should be 'fake', got %q", cfg.Video.ModelProviderMode)
	}
	if cfg.Video.SkillRoot != "skills" {
		t.Errorf("SKILL_ROOT should be 'skills', got %q", cfg.Video.SkillRoot)
	}
}

func TestSetDefaultsEnablesCloudVideoCreation(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	if !viper.GetBool("VIDEO_CREATION_ENABLED") {
		t.Fatal("VIDEO_CREATION_ENABLED should default to true for the cloud backend")
	}
	if viper.GetBool("LOCAL_RUNNER_ENABLED") {
		t.Fatal("LOCAL_RUNNER_ENABLED should remain false by default")
	}
	if got := viper.GetString("AGENT_PLANNER_MODE"); got != "hybrid" {
		t.Fatalf("AGENT_PLANNER_MODE default = %q, want hybrid", got)
	}
	if got := viper.GetInt("AGENT_PLANNER_MAX_TOOLS"); got != 8 {
		t.Fatalf("AGENT_PLANNER_MAX_TOOLS default = %d, want 8", got)
	}
}

func TestConfigZeroValueBehavior(t *testing.T) {
	// When VideoCreationEnabled is false, old routes should behave normally.
	// This test validates the zero-value behavior of the feature flag.
	cfg := &Config{}

	if cfg.Video.VideoCreationEnabled {
		t.Error("Zero-value Config should have VideoCreationEnabled=false")
	}
	if cfg.Video.LocalRunnerEnabled {
		t.Error("Zero-value Config should have LocalRunnerEnabled=false")
	}
}

func TestResolveSkillRootFindsMigratedCloudBackendSkillsFromRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	cloudSkills := filepath.Join(repoRoot, "cloud-backend", "skills")
	if err := os.MkdirAll(cloudSkills, 0o755); err != nil {
		t.Fatalf("create cloud skills dir: %v", err)
	}

	resolved := resolveSkillRoot("skills", repoRoot)
	if resolved != cloudSkills {
		t.Fatalf("resolved skill root = %q, want %q", resolved, cloudSkills)
	}
}

func TestResolveSkillRootKeepsExistingRelativePath(t *testing.T) {
	cloudRoot := t.TempDir()
	skills := filepath.Join(cloudRoot, "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatalf("create skills dir: %v", err)
	}

	resolved := resolveSkillRoot("skills", cloudRoot)
	if resolved != skills {
		t.Fatalf("resolved skill root = %q, want %q", resolved, skills)
	}
}

func TestResolveSkillRootKeepsEmptyValue(t *testing.T) {
	if resolved := resolveSkillRoot("", t.TempDir()); resolved != "" {
		t.Fatalf("resolved empty skill root = %q, want empty", resolved)
	}
}
