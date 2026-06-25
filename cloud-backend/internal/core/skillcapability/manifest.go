package skillcapability

import "github.com/tangying-ai/aios-core/internal/core/worker/tool"

type CapabilityStatus string

const (
	StatusHealthy   CapabilityStatus = "HEALTHY"
	StatusUnhealthy CapabilityStatus = "UNHEALTHY"
)

type Manifest struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Version     string            `json:"version" yaml:"version"`
	Domain      string            `json:"domain" yaml:"domain"`
	Description string            `json:"description" yaml:"description"`
	Activation  Activation        `json:"activation" yaml:"activation"`
	Resources   []Resource        `json:"resources" yaml:"resources"`
	Tools       []ToolDeclaration `json:"tools" yaml:"tools"`
	RoleAgents  []RoleAgent       `json:"roleAgents,omitempty" yaml:"-"`
	Recipe      RecipeRef         `json:"recipe" yaml:"recipe"`
	RootPath    string            `json:"rootPath,omitempty" yaml:"-"`
	Status      CapabilityStatus  `json:"status" yaml:"-"`
	LoadError   string            `json:"loadError,omitempty" yaml:"-"`
}

type Activation struct {
	Intents  []string `json:"intents,omitempty" yaml:"intents"`
	Keywords []string `json:"keywords,omitempty" yaml:"keywords"`
}

type Resource struct {
	ID           string `json:"id" yaml:"id"`
	Type         string `json:"type" yaml:"type"`
	Path         string `json:"path" yaml:"path"`
	Scope        string `json:"scope,omitempty" yaml:"scope"`
	LoadStrategy string `json:"loadStrategy,omitempty" yaml:"load_strategy"`
	Priority     int    `json:"priority,omitempty" yaml:"priority"`
}

type ToolDeclaration struct {
	ID       string `json:"id" yaml:"id"`
	Manifest string `json:"manifest" yaml:"manifest"`
	Prompt   string `json:"prompt,omitempty" yaml:"prompt"`
}

type RecipeRef struct {
	Path string `json:"path,omitempty" yaml:"path"`
	Mode string `json:"mode,omitempty" yaml:"mode"`
}

type RoleAgent struct {
	ID              string              `json:"id" yaml:"id"`
	Name            string              `json:"name" yaml:"name"`
	DisplayName     string              `json:"displayName" yaml:"displayName"`
	Stage           string              `json:"stage" yaml:"stage"`
	Goal            string              `json:"goal" yaml:"goal"`
	RequiredInputs  []string            `json:"requiredInputs,omitempty" yaml:"requiredInputs"`
	RequiredOutputs []string            `json:"requiredOutputs,omitempty" yaml:"requiredOutputs"`
	AllowedTools    []string            `json:"allowedTools,omitempty" yaml:"allowedTools"`
	ForbiddenTools  []string            `json:"forbiddenTools,omitempty" yaml:"forbiddenTools"`
	HumanReview     *tool.HumanReview   `json:"humanReview,omitempty" yaml:"humanReview"`
	QualityPolicy   *tool.QualityPolicy `json:"qualityPolicy,omitempty" yaml:"qualityPolicy"`
	MaxToolCalls    int                 `json:"maxToolCalls,omitempty" yaml:"maxToolCalls"`
}
