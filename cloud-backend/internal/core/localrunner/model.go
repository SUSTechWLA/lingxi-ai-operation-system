package localrunner

import (
	"strings"
	"time"
)

// RunnerStatus indicates whether a local runner is online.
type RunnerStatus string

const (
	RunnerOnline  RunnerStatus = "ONLINE"
	RunnerOffline RunnerStatus = "OFFLINE"
	RunnerRevoked RunnerStatus = "REVOKED"
)

type CallbackState string

const (
	CallbackPending    CallbackState = "PENDING"
	CallbackProcessing CallbackState = "PROCESSING"
	CallbackDelivered  CallbackState = "DELIVERED"
)

type CallbackPhase string

const (
	CallbackPhaseResult   CallbackPhase = "RESULT"
	CallbackPhaseFollowup CallbackPhase = "FOLLOWUP"
)

// JobStatus tracks the lifecycle of a local job.
type JobStatus string

const (
	JobPending   JobStatus = "PENDING"
	JobClaimed   JobStatus = "CLAIMED"
	JobRunning   JobStatus = "RUNNING"
	JobCompleted JobStatus = "COMPLETED"
	JobFailed    JobStatus = "FAILED"
)

type PlatformInfo struct {
	OS       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type RunnerCapability struct {
	ToolName        string                 `json:"toolName"`
	Command         string                 `json:"command"`
	Available       bool                   `json:"available"`
	Version         string                 `json:"version,omitempty"`
	CatalogRevision string                 `json:"catalogRevision,omitempty"`
	MCPTools        []MCPToolAdvertisement `json:"mcpTools,omitempty"`
}

type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

type MCPToolAdvertisement struct {
	ProviderID      string                 `json:"providerId"`
	LogicalToolName string                 `json:"logicalToolName"`
	RemoteToolName  string                 `json:"remoteToolName"`
	Description     string                 `json:"description,omitempty"`
	InputSchema     map[string]interface{} `json:"inputSchema"`
	OutputSchema    map[string]interface{} `json:"outputSchema,omitempty"`
	Annotations     MCPToolAnnotations     `json:"annotations,omitempty"`
	ApprovalMode    string                 `json:"approvalMode,omitempty"`
	TimeoutSec      int                    `json:"timeoutSec,omitempty"`
}

type RunnerMCPToolCatalog struct {
	RunnerID      string                 `json:"runnerId"`
	DeviceID      string                 `json:"deviceId,omitempty"`
	UserID        string                 `json:"userId"`
	Revision      string                 `json:"revision"`
	Tools         []MCPToolAdvertisement `json:"tools"`
	LastHeartbeat time.Time              `json:"lastHeartbeat"`
}

// LocalRunner represents a registered Electron/local runner instance.
type LocalRunner struct {
	ID            string             `json:"id"`
	DeviceID      string             `json:"deviceId,omitempty"`
	UserID        string             `json:"userId,omitempty"`
	Name          string             `json:"name,omitempty"`
	RunnerVersion string             `json:"runnerVersion,omitempty"`
	Platform      PlatformInfo       `json:"platform,omitempty"`
	WorkspaceRoot string             `json:"workspaceRoot,omitempty"`
	Capabilities  []RunnerCapability `json:"capabilities,omitempty"`
	SessionID     string             `json:"sessionId,omitempty"`
	Status        RunnerStatus       `json:"status"`
	LastHeartbeat time.Time          `json:"lastHeartbeat"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}

type LocalArtifactPolicy struct {
	Location            string `json:"location"`
	SyncMetadataToCloud bool   `json:"syncMetadataToCloud"`
	SyncFileToCloud     bool   `json:"syncFileToCloud"`
}

// LocalJob represents a task dispatched to a local runner.
type LocalJob struct {
	ID                         string                 `json:"jobId"`
	RunnerID                   string                 `json:"runnerId,omitempty"`
	UserID                     string                 `json:"userId,omitempty"`
	TargetRunnerID             string                 `json:"targetRunnerId,omitempty"`
	CatalogRevision            string                 `json:"catalogRevision,omitempty"`
	MCPProviderID              string                 `json:"mcpProviderId,omitempty"`
	MCPLogicalToolName         string                 `json:"mcpLogicalToolName,omitempty"`
	MCPRemoteToolName          string                 `json:"mcpRemoteToolName,omitempty"`
	ProjectID                  string                 `json:"projectId"`
	TaskID                     string                 `json:"taskId,omitempty"`
	NodeID                     string                 `json:"nodeId,omitempty"`
	ToolName                   string                 `json:"toolName,omitempty"`
	Command                    string                 `json:"command"`
	Payload                    map[string]interface{} `json:"payload"`
	Status                     JobStatus              `json:"status,omitempty"`
	Progress                   float64                `json:"progress,omitempty"`
	CurrentStep                string                 `json:"currentStep,omitempty"`
	Message                    string                 `json:"message,omitempty"`
	Output                     map[string]interface{} `json:"output,omitempty"`
	Error                      map[string]interface{} `json:"error,omitempty"`
	ErrorMessage               string                 `json:"errorMessage,omitempty"`
	Diagnostics                map[string]interface{} `json:"diagnostics,omitempty"`
	Retryable                  bool                   `json:"retryable,omitempty"`
	TimeoutSec                 int                    `json:"timeoutSec,omitempty"`
	ArtifactPolicy             LocalArtifactPolicy    `json:"artifactPolicy,omitempty"`
	IdempotencyKey             string                 `json:"idempotencyKey,omitempty"`
	Attempt                    int                    `json:"attempt,omitempty"`
	TraceID                    string                 `json:"-"`
	SpanID                     string                 `json:"-"`
	ParentSpanID               string                 `json:"-"`
	ResultCallbackState        CallbackState          `json:"resultCallbackState,omitempty"`
	FollowupCallbackState      CallbackState          `json:"followupCallbackState,omitempty"`
	ObservabilityCallbackState CallbackState          `json:"-"`
	LeaseExpiresAt             *time.Time             `json:"leaseExpiresAt,omitempty"`
	CompletedAt                *time.Time             `json:"completedAt,omitempty"`
	CreatedAt                  time.Time              `json:"createdAt,omitempty"`
	UpdatedAt                  time.Time              `json:"updatedAt,omitempty"`
}

type RegisterRunnerRequest struct {
	DeviceID      string             `json:"deviceId"`
	UserID        string             `json:"userId"`
	RunnerVersion string             `json:"runnerVersion"`
	Platform      PlatformInfo       `json:"platform"`
	WorkspaceRoot string             `json:"workspaceRoot"`
	Capabilities  []RunnerCapability `json:"capabilities"`
}

type RegisterRunnerResponse struct {
	RunnerID             string `json:"runnerId"`
	SessionID            string `json:"sessionId"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
	PollIntervalSec      int    `json:"pollIntervalSec"`
}

type HeartbeatRequest struct {
	SessionID     string              `json:"sessionId"`
	Status        string              `json:"status"`
	RunningJobs   int                 `json:"runningJobs"`
	DiskFreeMb    int64               `json:"diskFreeMb"`
	CPULoad       float64             `json:"cpuLoad"`
	MemoryUsageMb int64               `json:"memoryUsageMb"`
	LastError     *string             `json:"lastError"`
	Capabilities  *[]RunnerCapability `json:"capabilities,omitempty"`
}

type ClaimJobResponse struct {
	Job *LocalJob `json:"job"`
}

type ProgressRequest struct {
	Status   string   `json:"status"`
	Progress float64  `json:"progress"`
	Step     string   `json:"step"`
	Message  string   `json:"message"`
	Logs     []string `json:"logs,omitempty"`
}

type CompleteJobRequest struct {
	Success bool                   `json:"success"`
	Output  map[string]interface{} `json:"output"`
}

type FailJobRequest struct {
	Success     bool                   `json:"success"`
	Error       map[string]interface{} `json:"error"`
	Diagnostics map[string]interface{} `json:"diagnostics,omitempty"`
	Retryable   bool                   `json:"retryable"`
}

// JobMutationIdentity is populated only from authenticated request context and
// runner session headers. It is never decoded from a job mutation body.
type JobMutationIdentity struct {
	UserID    string
	DeviceID  string
	RunnerID  string
	SessionID string
}

// TerminalCallbackClaim is a durable lease for one callback phase. The stable
// idempotency key lets downstream sinks suppress a replay if delivery succeeds
// but the database acknowledgement is interrupted.
type TerminalCallbackClaim struct {
	Token          string
	IdempotencyKey string
	Delivered      bool
}

type DispatchLocalJobRequest struct {
	UserID             string                 `json:"userId,omitempty"`
	TargetRunnerID     string                 `json:"targetRunnerId,omitempty"`
	CatalogRevision    string                 `json:"catalogRevision,omitempty"`
	MCPProviderID      string                 `json:"mcpProviderId,omitempty"`
	MCPLogicalToolName string                 `json:"mcpLogicalToolName,omitempty"`
	MCPRemoteToolName  string                 `json:"mcpRemoteToolName,omitempty"`
	ProjectID          string                 `json:"projectId"`
	TaskID             string                 `json:"taskId,omitempty"`
	NodeID             string                 `json:"nodeId,omitempty"`
	ToolName           string                 `json:"toolName,omitempty"`
	Command            string                 `json:"command"`
	Payload            map[string]interface{} `json:"payload"`
	TimeoutSec         int                    `json:"timeoutSec,omitempty"`
	ArtifactPolicy     LocalArtifactPolicy    `json:"artifactPolicy,omitempty"`
	IdempotencyKey     string                 `json:"idempotencyKey,omitempty"`
	TraceID            string                 `json:"-"`
	SpanID             string                 `json:"-"`
	ParentSpanID       string                 `json:"-"`
}

// Local command constants — single source of truth for all local job commands.
const (
	CommandHyperFramesProjectGenerate = "HYPERFRAMES_PROJECT_GENERATE"
	CommandHyperFramesRender          = "HYPERFRAMES_RENDER"
	CommandHyperFramesLint            = "HYPERFRAMES_LINT"
	CommandFinalReview                = "FINAL_REVIEW"
	CommandHyperFramesSnapshot        = "HYPERFRAMES_SNAPSHOT"
	CommandHyperGenRender             = "HYPERGEN_RENDER"
	CommandFFmpegProbe                = "FFMPEG_PROBE"
	CommandFFmpegClipExtract          = "FFMPEG_CLIP_EXTRACT"
	CommandFFmpegAssemble             = "FFMPEG_ASSEMBLE"
	CommandAudioExtract               = "AUDIO_EXTRACT"
	CommandAudioNormalize             = "AUDIO_NORMALIZE"
	CommandASRTranscribe              = "ASR_TRANSCRIBE"
	CommandArtifactPackage            = "ARTIFACT_PACKAGE"
	CommandLocalFileImport            = "LOCAL_FILE_IMPORT"
	CommandLocalMediaIndex            = "LOCAL_MEDIA_INDEX"
	CommandLocalMCPToolCall           = "LOCAL_MCP_TOOL_CALL"
	CommandLocalIpTalkingAvatarRender = "LOCAL_IP_TALKING_AVATAR_RENDER"
	CommandVideoFrameQA               = "VIDEO_FRAME_QA"
	CommandBundleExtract              = "BUNDLE_EXTRACT"
)

// ValidCommands is the whitelist of allowed local job commands.
var ValidCommands = map[string]bool{
	CommandHyperFramesProjectGenerate: true,
	CommandHyperFramesRender:          true,
	CommandHyperFramesLint:            true,
	CommandHyperFramesSnapshot:        true,
	CommandHyperGenRender:             true,
	CommandFFmpegProbe:                true,
	CommandFFmpegClipExtract:          true,
	CommandFFmpegAssemble:             true,
	CommandAudioExtract:               true,
	CommandAudioNormalize:             true,
	CommandASRTranscribe:              true,
	CommandArtifactPackage:            true,
	CommandLocalFileImport:            true,
	CommandLocalMediaIndex:            true,
	CommandLocalMCPToolCall:           true,
	CommandLocalIpTalkingAvatarRender: true,
	CommandVideoFrameQA:               true,
	CommandBundleExtract:              true,
	CommandFinalReview:                true,
}

// IsValidCommand checks if a command is in the whitelist.
func IsValidCommand(cmd string) bool {
	return ValidCommands[NormalizeCommand(cmd)]
}

func NormalizeCommand(cmd string) string {
	return strings.ToUpper(strings.TrimSpace(cmd))
}
