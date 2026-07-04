package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestValidateForModeRejectsProductionWeakDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setDefaults()

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	err := cfg.ValidateForMode("production")
	if err == nil {
		t.Fatal("expected production validation to reject weak defaults")
	}
	for _, want := range []string{"AUTH_TOKEN_SECRET", "POSTGRES_PASSWORD", "MINIO_SECRET_KEY", "SANDBOX_ENABLED"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("production validation error should mention %s, got %v", want, err)
		}
	}
}

func TestValidateForModeAllowsDevelopmentDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	setDefaults()

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	if err := cfg.ValidateForMode("development"); err != nil {
		t.Fatalf("development defaults should remain usable: %v", err)
	}
}

func TestValidateForModeRejectsProductionWildcardCORSAndSandboxFallback(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{CORSAllowedOrigins: "*"},
		Postgres: PostgresConfig{
			Password: "long-non-default-postgres-password",
		},
		Auth: AuthConfig{
			TokenSecret: "0123456789abcdef0123456789abcdef",
		},
		MinIO: MinIOConfig{
			SecretKey: "long-non-default-minio-secret",
		},
		BashTool: BashToolConfig{
			AllowedCommands: "ls,cat,pwd",
		},
		Sandbox: SandboxConfig{
			Enabled:  true,
			Address:  "127.0.0.1:50051",
			Fallback: true,
		},
	}

	err := cfg.ValidateForMode("release")
	if err == nil {
		t.Fatal("expected production validation to reject wildcard CORS and sandbox fallback")
	}
	for _, want := range []string{"CORS_ALLOWED_ORIGINS", "SANDBOX_FALLBACK"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("production validation error should mention %s, got %v", want, err)
		}
	}
}

func TestValidateForModeRejectsProductionDefaultMinIOAccessKey(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{CORSAllowedOrigins: "http://localhost:3000"},
		Postgres: PostgresConfig{
			Password: "long-non-default-postgres-password",
		},
		Auth: AuthConfig{
			TokenSecret: "0123456789abcdef0123456789abcdef",
		},
		MinIO: MinIOConfig{
			AccessKey: "minioadmin",
			SecretKey: "long-non-default-minio-secret-value",
		},
		BashTool: BashToolConfig{
			AllowedCommands: "ls,cat,pwd",
		},
		Sandbox: SandboxConfig{
			Enabled:  true,
			Address:  "127.0.0.1:50051",
			Fallback: false,
		},
	}

	err := cfg.ValidateForMode("production")
	if err == nil {
		t.Fatal("expected production validation to reject default MinIO access key")
	}
	if !strings.Contains(err.Error(), "MINIO_ACCESS_KEY") {
		t.Fatalf("production validation error should mention MINIO_ACCESS_KEY, got %v", err)
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
