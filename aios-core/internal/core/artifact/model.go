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

// Artifact represents a versioned intermediate or final output of a workflow stage.
type Artifact struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"projectId"`
	WorkflowRunID string                 `json:"workflowRunId,omitempty"`
	StageName     string                 `json:"stageName"`
	UnitID        string                 `json:"unitId,omitempty"`
	Kind          ArtifactKind           `json:"kind"`
	Name          string                 `json:"name"`
	Version       int                    `json:"version"`
	ParentID      string                 `json:"parentId,omitempty"`
	StorageType   string                 `json:"storageType"` // "minio" | "inline"
	StorageRef    string                 `json:"storageRef,omitempty"`
	InlineJSON    string                 `json:"inlineJson,omitempty"`
	MimeType      string                 `json:"mimeType,omitempty"`
	SizeBytes     int64                  `json:"sizeBytes"`
	ContentHash   string                 `json:"contentHash"`
	PromptHash    string                 `json:"promptHash,omitempty"`
	Provider      string                 `json:"provider,omitempty"`
	Model         string                 `json:"model,omitempty"`
	IsCurrent     bool                   `json:"isCurrent"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// CreateArtifactRequest is the input for creating a new artifact version.
type CreateArtifactRequest struct {
	ProjectID     string
	WorkflowRunID string
	StageName     string
	UnitID        string
	Kind          ArtifactKind
	Name          string
	StorageType   string // "minio" | "inline"
	StorageRef    string
	Data          []byte
	MimeType      string
	ContentHash   string
	PromptHash    string
	Provider      string
	Model         string
	Metadata      map[string]interface{}
}
