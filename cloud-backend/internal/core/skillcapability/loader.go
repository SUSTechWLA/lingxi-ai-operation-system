// Package skillcapability provides bundled capability loading for Dynamic Agent tool discovery.
// It reads YAML tool manifests from a directory tree structured as:
//
//	skill-capabilities/<domain>/<skill>/<version>/capability.yaml
//	skill-capabilities/<domain>/<skill>/<version>/tools/*.tool.yaml
//
// On startup, LoadCapabilities traverses this tree and returns ToolManifest slices that
// can be registered with the tool Registry and persisted by the ToolManifestService.
package skillcapability

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// SkillMeta describes a bundled skill capability.
type SkillMeta struct {
	ID          string          `yaml:"id"`
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Domain      string          `yaml:"domain"`
	Version     string          `yaml:"version"`
	Tags        []string        `yaml:"tags"`
	Tools       []SkillMetaTool `yaml:"tools"`
	Recipe      SkillMetaRecipe `yaml:"recipe"`
}

type SkillMetaTool struct {
	ID       string `yaml:"id"`
	Manifest string `yaml:"manifest"`
}

type SkillMetaRecipe struct {
	Mode string `yaml:"mode"`
}

// ToolEntry is a decoded SkillMeta ∪ ToolManifest pair from one .tool.yaml file.
// ToolManifest is deliberately kept as map[string]any here to avoid a circular import
// into the worker/tool package. The caller converts it into a *tool.ToolManifest.
type ToolEntry struct {
	Manifest        map[string]any
	ManifestName    string
	ManifestVersion string
	Skill           SkillMeta
}

// loadError collects errors for one file.
type loadError struct {
	path string
	err  error
}

func (e loadError) Error() string {
	return fmt.Sprintf("capability load error at %s: %v", e.path, e.err)
}

// LoadCapabilities reads all skill capability directories under root and returns
// the parsed tool entries, plus any non-fatal errors (e.g. one corrupt YAML file).
func LoadCapabilities(root string) (entries []ToolEntry, errors []error) {
	if root == "" {
		return nil, nil
	}

	infos, err := os.ReadDir(root)
	if err != nil {
		errors = append(errors, loadError{path: root, err: err})
		return nil, errors
	}

	for _, domainDir := range infos {
		if !domainDir.IsDir() {
			continue
		}
		domainPath := filepath.Join(root, domainDir.Name())

		skillDirs, err := os.ReadDir(domainPath)
		if err != nil {
			errors = append(errors, loadError{path: domainPath, err: err})
			continue
		}

		for _, skillDir := range skillDirs {
			if !skillDir.IsDir() {
				continue
			}
			skillPath := filepath.Join(domainPath, skillDir.Name())

			versionDirs, err := os.ReadDir(skillPath)
			if err != nil {
				errors = append(errors, loadError{path: skillPath, err: err})
				continue
			}

			// Sort version directories by name so we can pick the highest version or the first one.
			sort.Slice(versionDirs, func(i, j int) bool {
				return versionDirs[i].Name() < versionDirs[j].Name()
			})

			for _, versionDir := range versionDirs {
				if !versionDir.IsDir() {
					continue
				}
				versionPath := filepath.Join(skillPath, versionDir.Name())

				skillMeta, err := parseSkillMeta(filepath.Join(versionPath, "capability.yaml"))
				if err != nil {
					errors = append(errors, loadError{path: versionPath, err: err})
					continue
				}
				if skillMeta.Version == "" {
					skillMeta.Version = versionDir.Name()
				}

				toolDir := filepath.Join(versionPath, "tools")
				toolFiles, err := os.ReadDir(toolDir)
				if err != nil {
					errors = append(errors, loadError{path: toolDir, err: err})
					continue
				}

				for _, toolFile := range toolFiles {
					if toolFile.IsDir() || filepath.Ext(toolFile.Name()) != ".yaml" {
						continue
					}
					toolPath := filepath.Join(toolDir, toolFile.Name())
					toolManifest, err := parseSkillToolYAML(toolPath)
					if err != nil {
						errors = append(errors, loadError{path: toolPath, err: err})
						continue
					}

					entries = append(entries, ToolEntry{
						Manifest:        toolManifest,
						ManifestName:    toolManifestName(toolManifest),
						ManifestVersion: skillMeta.Version,
						Skill:           *skillMeta,
					})
				}
			}
		}
	}

	if len(entries) == 0 {
		return nil, errors
	}
	return entries, errors
}

func parseSkillMeta(path string) (*SkillMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read capability.yaml: %w", err)
	}
	var meta SkillMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("invalid capability.yaml: %w", err)
	}
	if meta.ID == "" {
		return nil, fmt.Errorf("capability.yaml is missing required field 'id'")
	}
	return &meta, nil
}

func parseSkillToolYAML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read tool manifest: %w", err)
	}
	var manifest map[string]any
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid tool manifest YAML: %w", err)
	}
	if _, ok := manifest["name"]; !ok {
		return nil, fmt.Errorf("tool manifest is missing required field 'name'")
	}
	return manifest, nil
}

func toolManifestName(manifest map[string]any) string {
	if name, ok := manifest["name"].(string); ok {
		return name
	}
	return ""
}
