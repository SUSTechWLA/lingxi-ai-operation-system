package tool

import "time"

// ToolManifest represents the full specification of a tool, used as the knowledge base
// for AI assistants and external developers to understand how to use or implement tools.
type ToolManifest struct {
	Name                 string                 `json:"name"`
	Description          string                 `json:"description"`
	Version              string                 `json:"version,omitempty"`
	Author               string                 `json:"author,omitempty"`
	Type                 string                 `json:"type"`               // "builtin", "http", "grpc", "executable"
	Boundary             string                 `json:"boundary,omitempty"` // cloud_builtin | local_native | mcp_provider | remote_http | queue | legacy
	Endpoint             string                 `json:"endpoint,omitempty"` // URL for external tools
	Transport            *ToolTransport         `json:"transport,omitempty"`
	Timeout              int                    `json:"timeout,omitempty"`
	InputSchema          map[string]interface{} `json:"inputSchema,omitempty"`
	OutputSchema         map[string]interface{} `json:"outputSchema,omitempty"`
	Parameters           map[string]ParamDef    `json:"parameters"`
	Output               map[string]ParamDef    `json:"output"`
	Sandbox              bool                   `json:"sandbox"`
	Examples             []ToolExample          `json:"examples,omitempty"`
	Capabilities         []string               `json:"capabilities,omitempty"`
	Tags                 []string               `json:"tags,omitempty"`
	WhenToUse            []string               `json:"whenToUse,omitempty"`
	WhenNotToUse         []string               `json:"whenNotToUse,omitempty"`
	CostLevel            string                 `json:"costLevel,omitempty"`
	LatencyLevel         string                 `json:"latencyLevel,omitempty"`
	RiskLevel            string                 `json:"riskLevel,omitempty"`
	SideEffect           bool                   `json:"sideEffect,omitempty"`
	Idempotent           bool                   `json:"idempotent,omitempty"`
	ApprovalPolicy       ApprovalPolicy         `json:"approvalPolicy,omitempty"`
	HumanReview          *HumanReview           `json:"humanReview,omitempty"`
	ArtifactPolicy       ArtifactPolicy         `json:"artifactPolicy,omitempty"`
	QualityPolicy        QualityPolicy          `json:"qualityPolicy,omitempty"`
	ExecutionPlane       string                 `json:"executionPlane,omitempty"`
	RequiresUserDevice   bool                   `json:"requiresUserDevice,omitempty"`
	ArtifactLocation     string                 `json:"artifactLocation,omitempty"`
	LocalCommand         string                 `json:"localCommand,omitempty"`
	LocalRequirements    LocalRequirements      `json:"localRequirements,omitempty"`
	Provider             string                 `json:"provider,omitempty"`
	ProviderBinding      *ProviderBinding       `json:"providerBinding,omitempty"`
	ProviderCapabilities map[string]interface{} `json:"providerCapabilities,omitempty"`
	NextRecommendedTools []string               `json:"nextRecommendedTools,omitempty"`
	FailureModes         []string               `json:"failureModes,omitempty"`
	SkillPackageID       string                 `json:"skillPackageId,omitempty"`
	PromptRef            string                 `json:"promptRef,omitempty"`
	ResourceRefs         []string               `json:"resourceRefs,omitempty"`
	RegisteredAt         time.Time              `json:"registeredAt,omitempty"`
}

type ToolTransport struct {
	Type     string            `json:"type,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// ProviderBinding maps a logical ToolManifest entry to one remote MCP provider
// tool. Planner and retriever use LogicalToolName; local execution routes the
// call through ProviderID and RemoteToolName via LOCAL_MCP_TOOL_CALL.
type ProviderBinding struct {
	ProviderID      string            `json:"providerId,omitempty"`
	RemoteToolName  string            `json:"remoteToolName,omitempty"`
	LogicalToolName string            `json:"logicalToolName,omitempty"`
	TargetRunnerID  string            `json:"targetRunnerId,omitempty"`
	CatalogRevision string            `json:"catalogRevision,omitempty"`
	DeviceID        string            `json:"deviceId,omitempty"`
	ToolPrefix      string            `json:"toolPrefix,omitempty"`
	ToolNameMap     map[string]string `json:"toolNameMap,omitempty"`
}

const (
	BoundaryCloudBuiltin = "cloud_builtin"
	BoundaryLocalNative  = "local_native"
	BoundaryMCPProvider  = "mcp_provider"
	BoundaryRemoteHTTP   = "remote_http"
	BoundaryQueue        = "queue"
	BoundaryLegacy       = "legacy"

	CostLow    = "low"
	CostMedium = "medium"
	CostHigh   = "high"

	LatencyLow    = "low"
	LatencyMedium = "medium"
	LatencyHigh   = "high"

	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"

	ApprovalNone             = "none"
	ApprovalBeforeExecute    = "before_execute"
	ApprovalAfterArtifact    = "after_artifact"
	ApprovalBeforeDownstream = "before_downstream"
	ApprovalBeforeSideEffect = "before_side_effect"
	ApprovalAlways           = "always"

	ExecutionPlaneCloud      = "cloud"
	ExecutionPlaneLocal      = "local"
	ExecutionPlaneRemoteHTTP = "remote_http"
	ExecutionPlaneHybrid     = "hybrid"

	ArtifactLocationCloud = "cloud"
	ArtifactLocationLocal = "local"
	ArtifactLocationBoth  = "both"
)

type ApprovalPolicy struct {
	Required            bool     `json:"required"`
	Mode                string   `json:"mode,omitempty"`
	BlocksDownstream    bool     `json:"blocksDownstream,omitempty"`
	Reason              string   `json:"reason,omitempty"`
	ReviewArtifactKinds []string `json:"reviewArtifactKinds,omitempty"`
}

type ArtifactPolicy struct {
	ProduceArtifact       bool     `json:"produceArtifact"`
	ArtifactKinds         []string `json:"artifactKinds,omitempty"`
	DefaultReviewRequired bool     `json:"defaultReviewRequired,omitempty"`
	Storage               string   `json:"storage,omitempty"`             // "local" | "cloud" | "both"
	SyncMetadataToCloud   bool     `json:"syncMetadataToCloud,omitempty"` // sync artifact metadata to cloud
	SyncFileToCloud       bool     `json:"syncFileToCloud,omitempty"`     // sync artifact file to cloud
}

// QualityPolicy defines automated quality checking for a tool.
// When Required is true, the PlanCompiler can auto-insert a quality checker
// node after this tool's execution node, followed by a quality gate CONTROL node
// that auto-approves when the checker passes or blocks when it fails.
type QualityPolicy struct {
	Required          bool   `json:"required" yaml:"required"`
	CheckerTool       string `json:"checkerTool,omitempty" yaml:"checkerTool,omitempty"`
	MinScore          int    `json:"minScore,omitempty" yaml:"minScore,omitempty"`
	AutoRepair        bool   `json:"autoRepair,omitempty" yaml:"autoRepair,omitempty"`
	MaxRepairAttempts int    `json:"maxRepairAttempts,omitempty" yaml:"maxRepairAttempts,omitempty"`
	RepairTool        string `json:"repairTool,omitempty" yaml:"repairTool,omitempty"`
}

// HumanReview describes how human-in-the-loop review should be presented
// to the user. It supplements ApprovalPolicy with UI-facing metadata:
// title, review focus points, and available user actions.
type HumanReview struct {
	Required    bool     `json:"required" yaml:"required"`
	Gate        string   `json:"gate,omitempty" yaml:"gate,omitempty"`
	Title       string   `json:"title,omitempty" yaml:"title,omitempty"`
	ReviewFocus []string `json:"reviewFocus,omitempty" yaml:"reviewFocus,omitempty"`
	UserActions []string `json:"userActions,omitempty" yaml:"userActions,omitempty"`
}

type LocalRequirements struct {
	OS              []string `json:"os,omitempty"`
	Commands        []string `json:"commands,omitempty"`
	MinDiskMb       int      `json:"minDiskMb,omitempty"`
	RequiresNetwork bool     `json:"requiresNetwork,omitempty"`
}

type ParamDef struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
}

type ToolExample struct {
	Input  map[string]interface{} `json:"input"`
	Output map[string]interface{} `json:"output"`
}

// ManifestForTool generates a ToolManifest from a Tool by extracting its metadata.
// If the tool implements ManifestProvider, its custom Manifest() is used directly.
// Otherwise, a minimal manifest with just Name and Description is generated.
func ManifestForTool(t Tool) ToolManifest {
	if mp, ok := t.(ManifestProvider); ok {
		return mp.Manifest()
	}

	return ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
	}
}
