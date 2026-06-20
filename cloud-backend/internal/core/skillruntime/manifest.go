package skillruntime

// SkillManifest describes a registered skill package.
type SkillManifest struct {
	Name           string            `yaml:"name" json:"name"`
	Version        string            `yaml:"version" json:"version"`
	DisplayName    string            `yaml:"display_name,omitempty" json:"displayName,omitempty"`
	Description    string            `yaml:"description" json:"description"`
	Category       string            `yaml:"category" json:"category"`
	Visibility     SkillVisibility   `yaml:"visibility,omitempty" json:"visibility,omitempty"`
	CanonicalSkill string            `yaml:"canonical_skill,omitempty" json:"canonicalSkill,omitempty"`
	Stages         []StageDefinition `yaml:"stages" json:"stages"`
	Health         HealthStatus      `json:"health"`
	LoadedAt       string            `json:"loadedAt,omitempty"`
	LoadError      string            `json:"loadError,omitempty"`
}

// SkillSummary is the lightweight catalog shape used for routing and gradual loading.
type SkillSummary struct {
	Name                 string       `json:"name"`
	Version              string       `json:"version"`
	DisplayName          string       `json:"displayName,omitempty"`
	Description          string       `json:"description"`
	Category             string       `json:"category"`
	Visibility           string       `json:"visibility"`
	CanonicalSkill       string       `json:"canonicalSkill,omitempty"`
	Health               HealthStatus `json:"health"`
	StageCount           int          `json:"stageCount"`
	RequiresApproval     bool         `json:"requiresApproval"`
	HasLongRunningStages bool         `json:"hasLongRunningStages"`
	LoadError            string       `json:"loadError,omitempty"`
}

// StageDefinition describes one phase of a skill workflow.
type StageDefinition struct {
	Name                string                 `yaml:"name" json:"name"`
	Kind                string                 `yaml:"kind,omitempty" json:"kind,omitempty"`
	Tool                string                 `yaml:"tool,omitempty" json:"tool,omitempty"`
	Instruction         string                 `yaml:"instruction" json:"instruction"`
	InputSchema         string                 `yaml:"input_schema,omitempty" json:"inputSchema,omitempty"`
	OutputSchema        string                 `yaml:"output_schema,omitempty" json:"outputSchema,omitempty"`
	Input               map[string]interface{} `yaml:"input,omitempty" json:"input,omitempty"`
	Optional            bool                   `yaml:"optional" json:"optional"`
	ApprovalReq         bool                   `yaml:"approval_required" json:"approvalRequired"`
	LongRunning         bool                   `yaml:"long_running,omitempty" json:"longRunning,omitempty"`
	HeartbeatTimeoutSec int                    `yaml:"heartbeat_timeout_sec,omitempty" json:"heartbeatTimeoutSec,omitempty"`
}

// HealthStatus indicates whether a skill loaded successfully.
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "HEALTHY"
	HealthUnhealthy HealthStatus = "UNHEALTHY"
	HealthDisabled  HealthStatus = "DISABLED"
)

// SkillVisibility controls whether a skill is offered to routers by default.
type SkillVisibility string

const (
	VisibilityPublic SkillVisibility = "public"
	VisibilityHidden SkillVisibility = "hidden"
)

// SkillRequires lists provider capabilities needed by a skill.
type SkillRequires struct {
	TextToText       bool `yaml:"text_to_text" json:"textToText"`
	ImageToText      bool `yaml:"image_to_text" json:"imageToText"`
	TextToImage      bool `yaml:"text_to_image" json:"textToImage"`
	TextImageToVideo bool `yaml:"text_image_to_video" json:"textImageToVideo"`
}

// Summary returns a routing-safe view of a skill without stage prompts or schemas.
func (s *SkillManifest) Summary() SkillSummary {
	summary := SkillSummary{
		Name:           s.Name,
		Version:        s.Version,
		DisplayName:    s.DisplayName,
		Description:    s.Description,
		Category:       s.Category,
		Visibility:     string(s.EffectiveVisibility()),
		CanonicalSkill: s.CanonicalSkill,
		Health:         s.Health,
		StageCount:     len(s.Stages),
		LoadError:      s.LoadError,
	}
	for _, stage := range s.Stages {
		if stage.ApprovalReq {
			summary.RequiresApproval = true
		}
		if stage.LongRunning {
			summary.HasLongRunningStages = true
		}
	}
	return summary
}

func (s *SkillManifest) EffectiveVisibility() SkillVisibility {
	if s.Visibility == "" {
		return VisibilityPublic
	}
	return s.Visibility
}

func (s *SkillManifest) VisibleInCatalog() bool {
	return s.EffectiveVisibility() != VisibilityHidden
}
