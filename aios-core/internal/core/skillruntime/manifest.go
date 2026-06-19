package skillruntime

// SkillManifest describes a registered skill package.
type SkillManifest struct {
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version" json:"version"`
	Description string            `yaml:"description" json:"description"`
	Category    string            `yaml:"category" json:"category"`
	Stages      []StageDefinition `yaml:"stages" json:"stages"`
	Health      HealthStatus      `json:"health"`
	LoadedAt    string            `json:"loadedAt,omitempty"`
	LoadError   string            `json:"loadError,omitempty"`
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

// SkillRequires lists provider capabilities needed by a skill.
type SkillRequires struct {
	TextToText       bool `yaml:"text_to_text" json:"textToText"`
	ImageToText      bool `yaml:"image_to_text" json:"imageToText"`
	TextToImage      bool `yaml:"text_to_image" json:"textToImage"`
	TextImageToVideo bool `yaml:"text_image_to_video" json:"textImageToVideo"`
}
