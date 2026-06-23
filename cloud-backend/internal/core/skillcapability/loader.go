package skillcapability

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
	"gopkg.in/yaml.v3"
)

func LoadCapabilities(root string) (*Registry, []*tool.ToolManifest, []error) {
	reg := NewRegistry()
	var manifests []*tool.ToolManifest
	var errs []error

	if root == "" {
		return reg, manifests, []error{fmt.Errorf("skill capability root is empty")}
	}
	if _, err := os.Stat(root); err != nil {
		return reg, manifests, []error{fmt.Errorf("cannot read skill capability root %s: %w", root, err)}
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if entry.IsDir() || entry.Name() != "skillcap.yaml" {
			return nil
		}

		capability, toolManifests, loadErr := loadCapability(filepath.Dir(path))
		if loadErr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, loadErr))
			return nil
		}
		if err := reg.Register(capability); err != nil {
			errs = append(errs, err)
			return nil
		}
		manifests = append(manifests, toolManifests...)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	return reg, manifests, errs
}

func loadCapability(dir string) (*Manifest, []*tool.ToolManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "skillcap.yaml"))
	if err != nil {
		return nil, nil, err
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, nil, fmt.Errorf("invalid skillcap.yaml: %w", err)
	}
	manifest.RootPath = dir
	manifest.Status = StatusHealthy

	var toolManifests []*tool.ToolManifest
	for _, decl := range manifest.Tools {
		loaded, err := loadToolManifest(dir, manifest.ID, decl)
		if err != nil {
			return nil, nil, err
		}
		toolManifests = append(toolManifests, loaded)
	}

	return &manifest, toolManifests, nil
}

func loadToolManifest(root, skillPackageID string, decl ToolDeclaration) (*tool.ToolManifest, error) {
	if decl.Manifest == "" {
		return nil, fmt.Errorf("tool %s manifest path is required", decl.ID)
	}
	data, err := os.ReadFile(filepath.Join(root, decl.Manifest))
	if err != nil {
		return nil, fmt.Errorf("cannot read tool manifest %s: %w", decl.Manifest, err)
	}

	var file toolManifestFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("invalid tool manifest %s: %w", decl.Manifest, err)
	}
	manifest := file.toManifest()
	if manifest.Name == "" {
		manifest.Name = decl.ID
	}
	if manifest.SkillPackageID == "" {
		manifest.SkillPackageID = skillPackageID
	}
	if manifest.PromptRef == "" {
		manifest.PromptRef = decl.Prompt
	}
	if manifest.ResourceRefs == nil {
		manifest.ResourceRefs = []string{}
	}
	return manifest, nil
}

type toolManifestFile struct {
	Name                 string                   `yaml:"name"`
	Description          string                   `yaml:"description"`
	Version              string                   `yaml:"version"`
	Author               string                   `yaml:"author"`
	Type                 string                   `yaml:"type"`
	Endpoint             string                   `yaml:"endpoint"`
	Timeout              int                      `yaml:"timeout"`
	Parameters           map[string]tool.ParamDef `yaml:"parameters"`
	Output               map[string]tool.ParamDef `yaml:"output"`
	Sandbox              bool                     `yaml:"sandbox"`
	Examples             []tool.ToolExample       `yaml:"examples"`
	Capabilities         []string                 `yaml:"capabilities"`
	Tags                 []string                 `yaml:"tags"`
	CostLevel            string                   `yaml:"costLevel"`
	LatencyLevel         string                   `yaml:"latencyLevel"`
	RiskLevel            string                   `yaml:"riskLevel"`
	SideEffect           bool                     `yaml:"sideEffect"`
	Idempotent           bool                     `yaml:"idempotent"`
	ApprovalPolicy       approvalPolicyFile       `yaml:"approvalPolicy"`
	ArtifactPolicy       artifactPolicyFile       `yaml:"artifactPolicy"`
	NextRecommendedTools []string                 `yaml:"nextRecommendedTools"`
	FailureModes         []string                 `yaml:"failureModes"`
	SkillPackageID       string                   `yaml:"skillPackageId"`
	PromptRef            string                   `yaml:"promptRef"`
	ResourceRefs         []string                 `yaml:"resourceRefs"`
}

func (f toolManifestFile) toManifest() *tool.ToolManifest {
	idempotent := f.Idempotent
	if !f.SideEffect && !idempotent {
		idempotent = true
	}
	return &tool.ToolManifest{
		Name:                 f.Name,
		Description:          f.Description,
		Version:              f.Version,
		Author:               f.Author,
		Type:                 f.Type,
		Endpoint:             f.Endpoint,
		Timeout:              f.Timeout,
		Parameters:           f.Parameters,
		Output:               f.Output,
		Sandbox:              f.Sandbox,
		Examples:             f.Examples,
		Capabilities:         f.Capabilities,
		Tags:                 f.Tags,
		CostLevel:            f.CostLevel,
		LatencyLevel:         f.LatencyLevel,
		RiskLevel:            f.RiskLevel,
		SideEffect:           f.SideEffect,
		Idempotent:           idempotent,
		ApprovalPolicy:       f.ApprovalPolicy.toPolicy(),
		ArtifactPolicy:       f.ArtifactPolicy.toPolicy(),
		NextRecommendedTools: f.NextRecommendedTools,
		FailureModes:         f.FailureModes,
		SkillPackageID:       f.SkillPackageID,
		PromptRef:            f.PromptRef,
		ResourceRefs:         f.ResourceRefs,
	}
}

type approvalPolicyFile struct {
	Required            bool     `yaml:"required"`
	Mode                string   `yaml:"mode"`
	BlocksDownstream    bool     `yaml:"blocksDownstream"`
	Reason              string   `yaml:"reason"`
	ReviewArtifactKinds []string `yaml:"reviewArtifactKinds"`
}

func (p approvalPolicyFile) toPolicy() tool.ApprovalPolicy {
	return tool.ApprovalPolicy{
		Required:            p.Required,
		Mode:                p.Mode,
		BlocksDownstream:    p.BlocksDownstream,
		Reason:              p.Reason,
		ReviewArtifactKinds: p.ReviewArtifactKinds,
	}
}

type artifactPolicyFile struct {
	ProduceArtifact       bool     `yaml:"produceArtifact"`
	ArtifactKinds         []string `yaml:"artifactKinds"`
	DefaultReviewRequired bool     `yaml:"defaultReviewRequired"`
}

func (p artifactPolicyFile) toPolicy() tool.ArtifactPolicy {
	return tool.ArtifactPolicy{
		ProduceArtifact:       p.ProduceArtifact,
		ArtifactKinds:         p.ArtifactKinds,
		DefaultReviewRequired: p.DefaultReviewRequired,
	}
}
