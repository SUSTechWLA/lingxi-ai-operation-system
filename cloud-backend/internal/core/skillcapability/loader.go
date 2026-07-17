package skillcapability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	roleAgents, err := loadRoleAgents(filepath.Join(dir, "agents"))
	if err != nil {
		return nil, nil, err
	}
	manifest.RoleAgents = roleAgents

	return &manifest, toolManifests, nil
}

func loadRoleAgents(dir string) ([]RoleAgent, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read role agents directory %s: %w", dir, err)
	}

	var agents []RoleAgent
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".agent.yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read role agent %s: %w", path, err)
		}
		var agent RoleAgent
		if err := yaml.Unmarshal(data, &agent); err != nil {
			return fmt.Errorf("invalid role agent %s: %w", path, err)
		}
		if agent.ID == "" {
			return fmt.Errorf("role agent %s id is required", path)
		}
		if agent.Stage == "" {
			return fmt.Errorf("role agent %s stage is required", agent.ID)
		}
		if agent.Name == "" {
			agent.Name = agent.ID
		}
		agents = append(agents, agent)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return agents, nil
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
	Boundary             string                   `yaml:"boundary"`
	Endpoint             string                   `yaml:"endpoint"`
	Timeout              int                      `yaml:"timeout"`
	Parameters           map[string]tool.ParamDef `yaml:"parameters"`
	Output               map[string]tool.ParamDef `yaml:"output"`
	Sandbox              bool                     `yaml:"sandbox"`
	Examples             []tool.ToolExample       `yaml:"examples"`
	Capabilities         []string                 `yaml:"capabilities"`
	Tags                 []string                 `yaml:"tags"`
	WhenToUse            []string                 `yaml:"whenToUse"`
	WhenNotToUse         []string                 `yaml:"whenNotToUse"`
	CostLevel            string                   `yaml:"costLevel"`
	LatencyLevel         string                   `yaml:"latencyLevel"`
	RiskLevel            string                   `yaml:"riskLevel"`
	SideEffect           bool                     `yaml:"sideEffect"`
	Idempotent           bool                     `yaml:"idempotent"`
	ApprovalPolicy       approvalPolicyFile       `yaml:"approvalPolicy"`
	ArtifactPolicy       artifactPolicyFile       `yaml:"artifactPolicy"`
	HumanReview          humanReviewFile          `yaml:"humanReview"`
	QualityPolicy        qualityPolicyFile        `yaml:"qualityPolicy"`
	ExecutionPlane       string                   `yaml:"executionPlane"`
	RequiresUserDevice   bool                     `yaml:"requiresUserDevice"`
	ArtifactLocation     string                   `yaml:"artifactLocation"`
	LocalCommand         string                   `yaml:"localCommand"`
	LocalRequirements    localRequirementsFile    `yaml:"localRequirements"`
	Provider             string                   `yaml:"provider"`
	ProviderBinding      *tool.ProviderBinding    `yaml:"providerBinding"`
	ProviderCapabilities map[string]interface{}   `yaml:"providerCapabilities"`
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
		Boundary:             f.Boundary,
		Endpoint:             f.Endpoint,
		Timeout:              f.Timeout,
		Parameters:           f.Parameters,
		Output:               f.Output,
		Sandbox:              f.Sandbox,
		Examples:             f.Examples,
		Capabilities:         f.Capabilities,
		Tags:                 f.Tags,
		WhenToUse:            f.WhenToUse,
		WhenNotToUse:         f.WhenNotToUse,
		CostLevel:            f.CostLevel,
		LatencyLevel:         f.LatencyLevel,
		RiskLevel:            f.RiskLevel,
		SideEffect:           f.SideEffect,
		Idempotent:           idempotent,
		ApprovalPolicy:       f.ApprovalPolicy.toPolicy(),
		ArtifactPolicy:       f.ArtifactPolicy.toPolicy(),
		HumanReview:          f.HumanReview.toHumanReview(),
		QualityPolicy:        f.QualityPolicy.toPolicy(),
		ExecutionPlane:       f.ExecutionPlane,
		RequiresUserDevice:   f.RequiresUserDevice,
		ArtifactLocation:     f.ArtifactLocation,
		LocalCommand:         f.LocalCommand,
		LocalRequirements:    f.LocalRequirements.toRequirements(),
		Provider:             f.Provider,
		ProviderBinding:      f.ProviderBinding,
		ProviderCapabilities: f.ProviderCapabilities,
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
	Storage               string   `yaml:"storage"`
	SyncMetadataToCloud   bool     `yaml:"syncMetadataToCloud"`
	SyncFileToCloud       bool     `yaml:"syncFileToCloud"`
}

func (p artifactPolicyFile) toPolicy() tool.ArtifactPolicy {
	return tool.ArtifactPolicy{
		ProduceArtifact:       p.ProduceArtifact,
		ArtifactKinds:         p.ArtifactKinds,
		DefaultReviewRequired: p.DefaultReviewRequired,
		Storage:               p.Storage,
		SyncMetadataToCloud:   p.SyncMetadataToCloud,
		SyncFileToCloud:       p.SyncFileToCloud,
	}
}

type qualityPolicyFile struct {
	Required          bool   `yaml:"required"`
	CheckerTool       string `yaml:"checkerTool"`
	MinScore          int    `yaml:"minScore"`
	AutoRepair        bool   `yaml:"autoRepair"`
	MaxRepairAttempts int    `yaml:"maxRepairAttempts"`
	RepairTool        string `yaml:"repairTool"`
}

func (p qualityPolicyFile) toPolicy() tool.QualityPolicy {
	return tool.QualityPolicy{
		Required:          p.Required,
		CheckerTool:       p.CheckerTool,
		MinScore:          p.MinScore,
		AutoRepair:        p.AutoRepair,
		MaxRepairAttempts: p.MaxRepairAttempts,
		RepairTool:        p.RepairTool,
	}
}

type localRequirementsFile struct {
	OS              []string `yaml:"os"`
	Commands        []string `yaml:"commands"`
	MinDiskMb       int      `yaml:"minDiskMb"`
	RequiresNetwork bool     `yaml:"requiresNetwork"`
}

func (r localRequirementsFile) toRequirements() tool.LocalRequirements {
	return tool.LocalRequirements{
		OS:              r.OS,
		Commands:        r.Commands,
		MinDiskMb:       r.MinDiskMb,
		RequiresNetwork: r.RequiresNetwork,
	}
}

type humanReviewFile struct {
	Required    bool     `yaml:"required"`
	Gate        string   `yaml:"gate"`
	Title       string   `yaml:"title"`
	ReviewFocus []string `yaml:"reviewFocus"`
	UserActions []string `yaml:"userActions"`
}

func (h humanReviewFile) toHumanReview() *tool.HumanReview {
	if !h.Required && h.Title == "" {
		return nil
	}
	return &tool.HumanReview{
		Required:    h.Required,
		Gate:        h.Gate,
		Title:       h.Title,
		ReviewFocus: h.ReviewFocus,
		UserActions: h.UserActions,
	}
}
