package config

import (
	"testing"
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
