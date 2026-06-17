package localrunner

import "time"

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

// LocalRunner represents a registered Electron/local runner instance.
type LocalRunner struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Status        RunnerStatus `json:"status"`
	LastHeartbeat time.Time    `json:"lastHeartbeat"`
	CreatedAt     time.Time    `json:"createdAt"`
}

// LocalJob represents a task dispatched to a local runner.
type LocalJob struct {
	ID             string    `json:"id"`
	RunnerID       string    `json:"runnerId,omitempty"`
	ProjectID      string    `json:"projectId"`
	Command        string    `json:"command"`
	Payload        string    `json:"payload"`
	Status         JobStatus `json:"status"`
	Progress       float64   `json:"progress"`
	CurrentStep    string    `json:"currentStep,omitempty"`
	Output         string    `json:"output,omitempty"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ValidCommands is the whitelist of allowed local job commands.
var ValidCommands = map[string]bool{
	"HYPERGEN_RENDER":  true,
	"FFMPEG_PROBE":     true,
	"FFMPEG_ASSEMBLE":  true,
	"BUNDLE_EXTRACT":   true,
}

// IsValidCommand checks if a command is in the whitelist.
func IsValidCommand(cmd string) bool {
	return ValidCommands[cmd]
}
