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
	ToolName  string `json:"toolName"`
	Command   string `json:"command"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
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
	ID             string                 `json:"jobId"`
	RunnerID       string                 `json:"runnerId,omitempty"`
	ProjectID      string                 `json:"projectId"`
	TaskID         string                 `json:"taskId,omitempty"`
	NodeID         string                 `json:"nodeId,omitempty"`
	ToolName       string                 `json:"toolName,omitempty"`
	Command        string                 `json:"command"`
	Payload        map[string]interface{} `json:"payload"`
	Status         JobStatus              `json:"status,omitempty"`
	Progress       float64                `json:"progress,omitempty"`
	CurrentStep    string                 `json:"currentStep,omitempty"`
	Message        string                 `json:"message,omitempty"`
	Output         map[string]interface{} `json:"output,omitempty"`
	Error          map[string]interface{} `json:"error,omitempty"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
	Diagnostics    map[string]interface{} `json:"diagnostics,omitempty"`
	Retryable      bool                   `json:"retryable,omitempty"`
	TimeoutSec     int                    `json:"timeoutSec,omitempty"`
	ArtifactPolicy LocalArtifactPolicy    `json:"artifactPolicy,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
	Attempt        int                    `json:"attempt,omitempty"`
	LeaseExpiresAt *time.Time             `json:"leaseExpiresAt,omitempty"`
	CreatedAt      time.Time              `json:"createdAt,omitempty"`
	UpdatedAt      time.Time              `json:"updatedAt,omitempty"`
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
	SessionID     string  `json:"sessionId"`
	Status        string  `json:"status"`
	RunningJobs   int     `json:"runningJobs"`
	DiskFreeMb    int64   `json:"diskFreeMb"`
	CPULoad       float64 `json:"cpuLoad"`
	MemoryUsageMb int64   `json:"memoryUsageMb"`
	LastError     *string `json:"lastError"`
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

type DispatchLocalJobRequest struct {
	ProjectID      string                 `json:"projectId"`
	TaskID         string                 `json:"taskId,omitempty"`
	NodeID         string                 `json:"nodeId,omitempty"`
	ToolName       string                 `json:"toolName,omitempty"`
	Command        string                 `json:"command"`
	Payload        map[string]interface{} `json:"payload"`
	TimeoutSec     int                    `json:"timeoutSec,omitempty"`
	ArtifactPolicy LocalArtifactPolicy    `json:"artifactPolicy,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
}

// ValidCommands is the whitelist of allowed local job commands.
var ValidCommands = map[string]bool{
	"HYPERFRAMES_PROJECT_GENERATE": true,
	"HYPERFRAMES_RENDER":           true,
	"HYPERFRAMES_LINT":             true,
	"HYPERFRAMES_SNAPSHOT":         true,
	"HYPERGEN_RENDER":              true,
	"FFMPEG_PROBE":                 true,
	"FFMPEG_CLIP_EXTRACT":          true,
	"FFMPEG_ASSEMBLE":              true,
	"AUDIO_EXTRACT":                true,
	"AUDIO_NORMALIZE":              true,
	"ASR_TRANSCRIBE":               true,
	"ARTIFACT_PACKAGE":             true,
	"LOCAL_FILE_IMPORT":            true,
	"LOCAL_MEDIA_INDEX":            true,
	"BUNDLE_EXTRACT":               true,
}

// IsValidCommand checks if a command is in the whitelist.
func IsValidCommand(cmd string) bool {
	return ValidCommands[NormalizeCommand(cmd)]
}

func NormalizeCommand(cmd string) string {
	return strings.ToUpper(strings.TrimSpace(cmd))
}
