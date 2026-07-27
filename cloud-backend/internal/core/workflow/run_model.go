package workflow

import "time"

// RunStatus represents the lifecycle of a WorkflowRun.
type RunStatus string

const (
	RunPending   RunStatus = "PENDING"
	RunRunning   RunStatus = "RUNNING"
	RunPaused    RunStatus = "PAUSED"
	RunCompleted RunStatus = "COMPLETED"
	RunFailed    RunStatus = "FAILED"
	RunCancelled RunStatus = "CANCELLED"
)

// StageStatus represents the execution state of a single stage.
type StageStatus string

const (
	StagePending         StageStatus = "PENDING"
	StageRunning         StageStatus = "RUNNING"
	StageWaitingApproval StageStatus = "WAITING_APPROVAL"
	StageSucceeded       StageStatus = "SUCCEEDED"
	StageFailed          StageStatus = "FAILED"
	StageCancelled       StageStatus = "CANCELLED"
	StageInvalidated     StageStatus = "INVALIDATED"
)

// RunManifest intentionally admits only versioned identifiers and hashes.
// Prompts, arguments, credentials, and arbitrary request metadata are excluded.
type RunManifest struct {
	SchemaVersion          string    `json:"schemaVersion"`
	Runtime                string    `json:"runtime"`
	RunID                  string    `json:"runId"`
	TraceID                string    `json:"traceId,omitempty"`
	ToolRegistrySnapshotID string    `json:"toolRegistrySnapshotId,omitempty"`
	ToolRegistrySHA256     string    `json:"toolRegistrySha256,omitempty"`
	ParentRunID            *string   `json:"parentRunId,omitempty"`
	ReplayFromStageID      *string   `json:"replayFromStageId,omitempty"`
	CreatedAt              time.Time `json:"createdAt"`
}

func buildWorkflowRunManifest(run *WorkflowRun) *RunManifest {
	if run == nil {
		return nil
	}
	return &RunManifest{
		SchemaVersion:          "1",
		Runtime:                "cloud-workflow",
		RunID:                  run.ID,
		TraceID:                run.TraceID,
		ToolRegistrySnapshotID: run.ToolRegistrySnapshotID,
		ParentRunID:            cloneWorkflowStringPointer(run.ParentRunID),
		ReplayFromStageID:      cloneWorkflowStringPointer(run.ReplayFromStageID),
		CreatedAt:              run.CreatedAt,
	}
}

func cloneWorkflowStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// WorkflowRun links a video project to an orchestrator task and tracks stage-level progress.
type WorkflowRun struct {
	ID                     string                 `json:"id"`
	ProjectID              string                 `json:"projectId"`
	UserID                 string                 `json:"userId"`
	TemplateID             string                 `json:"templateId"`
	TemplateVersion        string                 `json:"templateVersion"`
	TaskID                 string                 `json:"taskId"`
	Status                 RunStatus              `json:"status"`
	Attempt                int                    `json:"attempt"`
	Input                  map[string]interface{} `json:"input,omitempty"`
	Output                 map[string]interface{} `json:"output,omitempty"`
	StageStatuses          map[string]StageStatus `json:"stageStatuses"`
	TraceID                string                 `json:"traceId"`
	ToolRegistrySnapshotID string                 `json:"toolRegistrySnapshotId,omitempty"`
	RunManifest            *RunManifest           `json:"runManifest,omitempty"`
	ParentRunID            *string                `json:"parentRunId,omitempty"`
	ReplayFromStageID      *string                `json:"replayFromStageId,omitempty"`
	StartedAt              *time.Time             `json:"startedAt,omitempty"`
	FinishedAt             *time.Time             `json:"finishedAt,omitempty"`
	CreatedAt              time.Time              `json:"createdAt"`
}

// StageRun tracks the execution of a single stage within a workflow run.
type StageRun struct {
	ID            string                 `json:"id"`
	WorkflowRunID string                 `json:"workflowRunId"`
	StageName     string                 `json:"stageName"`
	UnitID        string                 `json:"unitId,omitempty"`
	Status        StageStatus            `json:"status"`
	Attempt       int                    `json:"attempt"`
	NodeIDs       []string               `json:"nodeIds"`
	Input         map[string]interface{} `json:"input,omitempty"`
	Output        map[string]interface{} `json:"output,omitempty"`
	ErrorCode     string                 `json:"errorCode,omitempty"`
	ErrorMessage  string                 `json:"errorMessage,omitempty"`
	StartedAt     *time.Time             `json:"startedAt,omitempty"`
	FinishedAt    *time.Time             `json:"finishedAt,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// Attempt records a single execution attempt of a stage (for rerun tracking).
type Attempt struct {
	ID           string    `json:"id"`
	StageRunID   string    `json:"stageRunId"`
	Number       int       `json:"number"`
	TriggerType  string    `json:"triggerType"` // INITIAL, RERUN, RETRY
	NodeIDs      []string  `json:"nodeIds"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}
