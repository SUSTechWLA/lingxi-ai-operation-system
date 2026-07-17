package artifact

import (
	"time"
)

// ArtifactKind classifies the type of artifact.
type ArtifactKind string

const (
	KindJSON     ArtifactKind = "JSON"
	KindMarkdown ArtifactKind = "MARKDOWN"
	KindImage    ArtifactKind = "IMAGE"
	KindAudio    ArtifactKind = "AUDIO"
	KindVideo    ArtifactKind = "VIDEO"
	KindBundle   ArtifactKind = "BUNDLE"
	KindLog      ArtifactKind = "LOG"
)

const (
	KindFFmpegProbeReport ArtifactKind = "FFMPEG_PROBE_REPORT"
	KindFinalReview       ArtifactKind = "FINAL_REVIEW"
	KindProjectPackage    ArtifactKind = "PROJECT_PACKAGE"
)

func IsCriticalArtifactKind(kind ArtifactKind) bool {
	switch kind {
	case KindVideo, KindFFmpegProbeReport, KindFinalReview, KindProjectPackage:
		return true
	default:
		return false
	}
}

const (
	// StorageLocal means the user payload is stored by the local desktop agent.
	StorageLocal = "local"
	// StorageInline is retained for reading legacy rows created before local-only storage.
	StorageInline = "inline"
	// StorageMinIO is retained for reading legacy rows created before local-only storage.
	StorageMinIO = "minio"
)

// Artifact represents a versioned intermediate or final output of a workflow stage.
type Artifact struct {
	ID             string                 `json:"id"`
	ProjectID      string                 `json:"projectId"`
	WorkflowRunID  string                 `json:"workflowRunId,omitempty"`
	TaskID         string                 `json:"taskId,omitempty"`
	StageName      string                 `json:"stageName"`
	RoleAgentID    string                 `json:"roleAgentId,omitempty"`
	UnitID         string                 `json:"unitId,omitempty"`
	Kind           ArtifactKind           `json:"kind"`
	Name           string                 `json:"name"`
	Version        int                    `json:"version"`
	ParentID       string                 `json:"parentId,omitempty"`
	StorageType    string                 `json:"storageType"` // "local"; legacy reads may be "minio" | "inline"
	StorageRef     string                 `json:"storageRef,omitempty"`
	InlineJSON     string                 `json:"inlineJson,omitempty"`
	MimeType       string                 `json:"mimeType,omitempty"`
	SizeBytes      int64                  `json:"sizeBytes"`
	ContentHash    string                 `json:"contentHash"`
	PromptHash     string                 `json:"promptHash,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	IsCurrent      bool                   `json:"isCurrent"`
	Status         string                 `json:"status"`
	HumanApproved  bool                   `json:"humanApproved"`
	DependsOn      []string               `json:"dependsOn,omitempty"`
	ProducedByNode string                 `json:"producedByNode,omitempty"`
	ProducedByTool string                 `json:"producedByTool,omitempty"`
	ProducedByRole string                 `json:"producedByRole,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
}

// CreateArtifactRequest is the input for creating a new artifact version.
type CreateArtifactRequest struct {
	ProjectID     string
	WorkflowRunID string
	TaskID        string
	StageName     string
	RoleAgentID   string
	UnitID        string
	Kind          ArtifactKind
	Name          string
	StorageType   string // new artifacts are normalized to "local"
	StorageRef    string
	Data          []byte
	MimeType      string
	SizeBytes     int64
	ContentHash   string
	PromptHash    string
	Provider      string
	Model         string
	Metadata      map[string]interface{}
}
