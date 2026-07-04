package model

import (
	"encoding/json"
	"time"
)

// VideoMode represents the production mode.
type VideoMode string

const (
	ModeAIGCShot       VideoMode = "aigc_shot"
	ModeVoiceVisual    VideoMode = "voice_visual"
	ModeCinematicStory VideoMode = "cinematic_story"
)

// ProjectStatus represents the lifecycle status.
type ProjectStatus string

const (
	StatusDraft     ProjectStatus = "DRAFT"
	StatusRunning   ProjectStatus = "RUNNING"
	StatusPaused    ProjectStatus = "PAUSED"
	StatusCompleted ProjectStatus = "COMPLETED"
	StatusArchived  ProjectStatus = "ARCHIVED"
)

// GenerationMode determines how video content is produced.
type GenerationMode string

const (
	GenProviderAPI  GenerationMode = "provider_api"
	GenManualImport GenerationMode = "manual_import"
)

// VideoProject represents a video creation project.
type VideoProject struct {
	ID              string          `json:"id"`
	UserID          string          `json:"userId"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Mode            VideoMode       `json:"mode"`
	Status          ProjectStatus   `json:"status"`
	SkillName       string          `json:"skillName"`
	SkillVersion    string          `json:"skillVersion"`
	WorkflowName    string          `json:"workflowName"`
	WorkflowVersion string          `json:"workflowVersion"`
	GenerationMode  GenerationMode  `json:"generationMode"`
	AspectRatio     string          `json:"aspectRatio,omitempty"`
	TargetDuration  int             `json:"targetDurationSec,omitempty"`
	Language        string          `json:"language,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CurrentRunID    string          `json:"currentRunId,omitempty"`
	LocalPathHint   string          `json:"localPathHint,omitempty"`
	DeletedAt       *time.Time      `json:"deletedAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// CreateProjectRequest is the input for creating a new video project.
type CreateProjectRequest struct {
	Name            string         `json:"name" binding:"required"`
	Description     string         `json:"description,omitempty"`
	Mode            VideoMode      `json:"mode" binding:"required"`
	SkillName       string         `json:"skillName"`
	SkillVersion    string         `json:"skillVersion"`
	WorkflowName    string         `json:"workflowName"`
	WorkflowVersion string         `json:"workflowVersion"`
	GenerationMode  GenerationMode `json:"generationMode"`
	AspectRatio     string         `json:"aspectRatio,omitempty"`
	TargetDuration  int            `json:"targetDurationSec,omitempty"`
	Language        string         `json:"language,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	LocalPathHint   string         `json:"localPathHint,omitempty"`
}

// UpdateProjectRequest is the input for updating a project. Mode and version
// fields cannot be changed after creation.
type UpdateProjectRequest struct {
	Name           string          `json:"name,omitempty"`
	Description    string          `json:"description,omitempty"`
	Status         ProjectStatus   `json:"status,omitempty"`
	GenerationMode GenerationMode  `json:"generationMode,omitempty"`
	AspectRatio    string          `json:"aspectRatio,omitempty"`
	TargetDuration *int            `json:"targetDurationSec,omitempty"`
	Language       string          `json:"language,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	LocalPathHint  string          `json:"localPathHint,omitempty"`
}

// IsValidMode checks if a mode string is a valid video production mode.
func IsValidMode(mode VideoMode) bool {
	return mode == ModeAIGCShot || mode == ModeVoiceVisual || mode == ModeCinematicStory
}

// IsValidGenerationMode checks if a generation mode is valid.
func IsValidGenerationMode(mode GenerationMode) bool {
	return mode == GenProviderAPI || mode == GenManualImport
}
