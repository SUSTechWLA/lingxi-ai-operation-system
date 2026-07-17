// Skill2Workflow converts a skill.yaml directory into a workflow DAG.
//
// Usage:
//
//	skill2workflow --skill skills/my-skill/1.0.0              # print DAG to stdout
//	skill2workflow --skill skills/my-skill/1.0.0 --output d.json  # write to file
//	skill2workflow --skill-root skills/ --output out/         # batch convert
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tangying-ai/aios-core/internal/core/skillruntime"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

func main() {
	skillDir := flag.String("skill", "", "Path to skill directory (e.g., skills/my-skill/1.0.0)")
	skillRoot := flag.String("skill-root", "", "Path to skills root directory (batch mode)")
	output := flag.String("output", "", "Output JSON file (or directory in batch mode)")
	flag.Parse()

	if *skillDir != "" {
		compileOne(*skillDir, *output)
	} else if *skillRoot != "" {
		compileAll(*skillRoot, *output)
	} else {
		fmt.Fprintln(os.Stderr, "Usage: skill2workflow --skill <dir> [--output <file>]")
		fmt.Fprintln(os.Stderr, "       skill2workflow --skill-root <dir> [--output <dir>]")
		os.Exit(1)
	}
}

func compileOne(dir, output string) {
	absDir, _ := filepath.Abs(dir)
	parent := filepath.Dir(absDir)
	version := filepath.Base(absDir)
	name := filepath.Base(parent)

	skill, err := loadSkillYAML(absDir, name, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if skill.Health == skillruntime.HealthUnhealthy {
		fmt.Fprintf(os.Stderr, "Error: skill is unhealthy: %s\n", skill.LoadError)
		os.Exit(1)
	}

	dag, err := workflow.CompileSkillToDAG(skill)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	pretty, _ := json.MarshalIndent(json.RawMessage(dag), "", "  ")
	if output != "" {
		os.WriteFile(output, pretty, 0644)
		fmt.Printf("✅ DAG (%d stages) → %s\n", len(skill.Stages), output)
		fmt.Printf("   Register: curl -X POST localhost:8080/api/workflows -H 'Content-Type: application/json' -d '{\"name\":\"%s-workflow\",\"category\":\"%s\",\"dag\":%s}'\n",
			skill.Name, skill.Category, string(dag))
	} else {
		fmt.Println(string(pretty))
	}
}

func compileAll(root, output string) {
	reg, errs := skillruntime.LoadSkills(root)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", e)
		}
	}

	for _, skill := range reg.List() {
		dag, err := workflow.CompileSkillToDAG(skill)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %s@%s: %v\n", skill.Name, skill.Version, err)
			continue
		}

		if output != "" {
			os.MkdirAll(output, 0755)
			outFile := filepath.Join(output, fmt.Sprintf("%s-%s.json", skill.Name, skill.Version))
			pretty, _ := json.MarshalIndent(json.RawMessage(dag), "", "  ")
			os.WriteFile(outFile, pretty, 0644)
		}
		fmt.Printf("✅ %s@%s: %d stages → DAG ready\n", skill.Name, skill.Version, len(skill.Stages))
	}
}

// loadSkillYAML reads and validates a skill.yaml from disk.
func loadSkillYAML(dir, name, version string) (*skillruntime.SkillManifest, error) {
	manifestPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", manifestPath, err)
	}

	var raw struct {
		Description string `yaml:"description"`
		Category    string `yaml:"category"`
		Stages      []struct {
			Name                string                 `yaml:"name"`
			Kind                string                 `yaml:"kind"`
			Tool                string                 `yaml:"tool"`
			Instruction         string                 `yaml:"instruction"`
			InputSchema         string                 `yaml:"input_schema"`
			OutputSchema        string                 `yaml:"output_schema"`
			Input               map[string]interface{} `yaml:"input"`
			Optional            bool                   `yaml:"optional"`
			ApprovalReq         bool                   `yaml:"approval_required"`
			LongRunning         bool                   `yaml:"long_running"`
			HeartbeatTimeoutSec int                    `yaml:"heartbeat_timeout_sec"`
		} `yaml:"stages"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	skill := &skillruntime.SkillManifest{
		Name:        name,
		Version:     version,
		Description: raw.Description,
		Category:    raw.Category,
		Stages:      make([]skillruntime.StageDefinition, 0, len(raw.Stages)),
		Health:      skillruntime.HealthHealthy,
	}

	for _, s := range raw.Stages {
		skill.Stages = append(skill.Stages, skillruntime.StageDefinition{
			Name:                s.Name,
			Kind:                s.Kind,
			Tool:                s.Tool,
			Instruction:         s.Instruction,
			InputSchema:         s.InputSchema,
			OutputSchema:        s.OutputSchema,
			Input:               s.Input,
			Optional:            s.Optional,
			ApprovalReq:         s.ApprovalReq,
			LongRunning:         s.LongRunning,
			HeartbeatTimeoutSec: s.HeartbeatTimeoutSec,
		})
	}

	// Validate stage instruction files exist
	for _, stage := range skill.Stages {
		if stage.Instruction != "" {
			instrPath := filepath.Join(dir, stage.Instruction)
			if _, err := os.Stat(instrPath); err != nil {
				skill.Health = skillruntime.HealthUnhealthy
				skill.LoadError = fmt.Sprintf("stage %s: instruction file not found: %s", stage.Name, stage.Instruction)
			}
		}
	}

	return skill, nil
}
