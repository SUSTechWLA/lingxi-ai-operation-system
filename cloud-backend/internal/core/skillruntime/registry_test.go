package skillruntime

import (
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	skill := &SkillManifest{
		Name:    "test-skill",
		Version: "1.0.0",
		Stages:  []StageDefinition{{Name: "stage1", Instruction: "test.md"}},
		Health:  HealthHealthy,
	}

	if err := reg.Register(skill); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Get("test-skill", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test-skill" {
		t.Errorf("expected test-skill, got %s", got.Name)
	}
}

func TestRegistryDuplicate(t *testing.T) {
	reg := NewRegistry()
	skill := &SkillManifest{Name: "dup", Version: "1.0.0", Health: HealthHealthy}
	reg.Register(skill)
	err := reg.Register(skill)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get("nonexistent", "1.0.0")
	if err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&SkillManifest{Name: "a", Version: "1.0.0", Health: HealthHealthy})
	reg.Register(&SkillManifest{Name: "b", Version: "1.0.0", Health: HealthHealthy})

	skills := reg.List()
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestRegistryHealth(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&SkillManifest{Name: "healthy", Version: "1.0.0", Health: HealthHealthy})
	reg.Register(&SkillManifest{Name: "broken", Version: "1.0.0", Health: HealthUnhealthy})

	h := reg.Health()
	if h["healthy@1.0.0"] != "HEALTHY" {
		t.Error("expected HEALTHY")
	}
	if h["broken@1.0.0"] != "UNHEALTHY" {
		t.Error("expected UNHEALTHY")
	}
}

func TestLoadSkillsMissingDir(t *testing.T) {
	reg, errs := LoadSkills("/nonexistent/dir")
	if len(errs) == 0 {
		t.Error("expected errors for missing directory")
	}
	if len(reg.List()) != 0 {
		t.Error("expected empty registry for missing directory")
	}
}
