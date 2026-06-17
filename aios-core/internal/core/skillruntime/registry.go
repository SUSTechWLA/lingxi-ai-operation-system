package skillruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Registry holds loaded skills and provides thread-safe access.
type Registry struct {
	mu      sync.RWMutex
	skills  map[string]*SkillManifest // key: name@version
}

func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*SkillManifest),
	}
}

// Register adds a skill to the registry.
func (r *Registry) Register(skill *SkillManifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := skill.Name + "@" + skill.Version
	if existing, ok := r.skills[key]; ok {
		return fmt.Errorf("skill %s already registered (loaded from %s)", key, existing.LoadedAt)
	}
	r.skills[key] = skill
	return nil
}

// Get retrieves a skill by name and version.
func (r *Registry) Get(name, version string) (*SkillManifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := name + "@" + version
	skill, ok := r.skills[key]
	if !ok {
		return nil, fmt.Errorf("skill %s not found", key)
	}
	return skill, nil
}

// List returns all registered skills.
func (r *Registry) List() []*SkillManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []*SkillManifest
	for _, s := range r.skills {
		skills = append(skills, s)
	}
	return skills
}

// Health returns a map of skill key -> health status.
func (r *Registry) Health() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string)
	for k, s := range r.skills {
		result[k] = string(s.Health)
	}
	return result
}

// LoadSkills scans the root directory for skill packages.
func LoadSkills(root string) (*Registry, []error) {
	reg := NewRegistry()
	var errs []error

	entries, err := os.ReadDir(root)
	if err != nil {
		errs = append(errs, fmt.Errorf("cannot read skill root %s: %w", root, err))
		return reg, errs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(root, entry.Name())
		versions, err := os.ReadDir(skillDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot read skill %s: %w", entry.Name(), err))
			continue
		}

		for _, ver := range versions {
			if !ver.IsDir() {
				continue
			}
			skill, err := loadSkill(filepath.Join(skillDir, ver.Name()), entry.Name(), ver.Name())
			if err != nil {
				skill = &SkillManifest{
					Name:      entry.Name(),
					Version:   ver.Name(),
					Health:    HealthUnhealthy,
					LoadError: err.Error(),
				}
				errs = append(errs, fmt.Errorf("skill %s@%s: %w", entry.Name(), ver.Name(), err))
			}
			if regErr := reg.Register(skill); regErr != nil {
				errs = append(errs, regErr)
			}
			zap.L().Info("Skill loaded",
				zap.String("name", skill.Name),
				zap.String("version", skill.Version),
				zap.String("health", string(skill.Health)),
			)
		}
	}
	return reg, errs
}

func loadSkill(dir, name, version string) (*SkillManifest, error) {
	manifestPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read skill.yaml: %w", err)
	}

	var skill SkillManifest
	if err := yaml.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("invalid skill.yaml: %w", err)
	}

	skill.Name = name
	skill.Version = version
	skill.Health = HealthHealthy

	// Validate stage files exist
	for i, stage := range skill.Stages {
		if stage.Instruction != "" {
			instrPath := filepath.Join(dir, stage.Instruction)
			if _, err := os.Stat(instrPath); err != nil {
				return nil, fmt.Errorf("stage %s: instruction file not found: %s", stage.Name, stage.Instruction)
			}
		}
		// Validate schema files
		if stage.InputSchema != "" {
			schemaPath := filepath.Join(dir, stage.InputSchema)
			if _, err := os.Stat(schemaPath); err != nil {
				return nil, fmt.Errorf("stage %s: input schema not found: %s", stage.Name, stage.InputSchema)
			}
		}
		if stage.OutputSchema != "" {
			schemaPath := filepath.Join(dir, stage.OutputSchema)
			if _, err := os.Stat(schemaPath); err != nil {
				return nil, fmt.Errorf("stage %s: output schema not found: %s", stage.Name, stage.OutputSchema)
			}
		}
		// Reset instruction to full path for later loading
		if stage.Instruction != "" {
			skill.Stages[i].Instruction = filepath.Join(dir, stage.Instruction)
		}
	}

	return &skill, nil
}
