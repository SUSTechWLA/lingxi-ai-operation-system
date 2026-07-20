package model

import (
	"time"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

type CreatorStepID string
type CreatorStepState string

const (
	CreatorStepRequirements CreatorStepID = "requirements"
	CreatorStepDirection    CreatorStepID = "direction"
	CreatorStepScript       CreatorStepID = "script"
	CreatorStepShots        CreatorStepID = "shots"
	CreatorStepPreview      CreatorStepID = "preview"
	CreatorStepDelivery     CreatorStepID = "delivery"

	CreatorStepNotStarted     CreatorStepState = "not_started"
	CreatorStepGenerating     CreatorStepState = "generating"
	CreatorStepNeedsReview    CreatorStepState = "needs_review"
	CreatorStepConfirmed      CreatorStepState = "confirmed"
	CreatorStepNeedsAttention CreatorStepState = "needs_attention"
	CreatorStepFailed         CreatorStepState = "failed"
)

// CreatorStep is the stable, creator-facing summary of a production step.
// Internal stages are intentionally collapsed into the six IDs above.
type CreatorStep struct {
	ID                CreatorStepID    `json:"id"`
	Label             string           `json:"label"`
	State             CreatorStepState `json:"state"`
	CurrentArtifactID string           `json:"currentArtifactId,omitempty"`
	CurrentVersion    int              `json:"currentVersion,omitempty"`
	ReviewID          string           `json:"reviewId,omitempty"`
	RunID             string           `json:"runId,omitempty"`
	AllowedActions    []string         `json:"allowedActions"`
}

// CreationView is the backend-authoritative state for the creator workspace.
type CreationView struct {
	Project       *VideoProject `json:"project"`
	ActiveStep    CreatorStepID `json:"activeStep"`
	Steps         []CreatorStep `json:"steps"`
	ShotSummary   ShotSummary   `json:"shotSummary"`
	ActiveTasks   []CreatorTask `json:"activeTasks"`
	AssemblyDirty bool          `json:"assemblyDirty"`
}

// CreatorTask only exposes durable work that a creator can safely resume after reconnecting.
type CreatorTask struct {
	ID     string `json:"id"`
	Scope  string `json:"scope"`
	ShotID string `json:"shotId,omitempty"`
	Status string `json:"status"`
	Label  string `json:"label"`
}

// ShotPageQuery scopes a creator-facing Shot review list without returning full media payloads.
type ShotPageQuery struct {
	Cursor  string
	Limit   int
	Status  string
	Chapter string
	Query   string
}

type ShotListItem struct {
	ID                  string `json:"id"`
	SequenceIndex       int    `json:"sequenceIndex"`
	Title               string `json:"title"`
	Chapter             string `json:"chapter,omitempty"`
	DurationSec         int    `json:"durationSec"`
	Version             int    `json:"version"`
	ReviewStatus        string `json:"reviewStatus"`
	QAStatus            string `json:"qaStatus,omitempty"`
	GenerationStatus    string `json:"generationStatus"`
	AcceptedCandidateID string `json:"acceptedCandidateId,omitempty"`
	ThumbnailRef        string `json:"thumbnailRef,omitempty"`
}

type ShotPage struct {
	Items      []ShotListItem `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Total      int            `json:"total"`
}

type ShotSummary struct {
	Total          int `json:"total"`
	Confirmed      int `json:"confirmed"`
	AwaitingReview int `json:"awaitingReview"`
	Generating     int `json:"generating"`
	NeedsAction    int `json:"needsAction"`
}

type ShotImpact struct {
	ShotID                   string   `json:"shotId"`
	AffectedShotIDs          []string `json:"affectedShotIds"`
	InvalidatesFinalAssembly bool     `json:"invalidatesFinalAssembly"`
	RegeneratesOtherShots    bool     `json:"regeneratesOtherShots"`
	EstimatedDurationSec     int      `json:"estimatedDurationSec"`
	RequiresConfirmation     bool     `json:"requiresConfirmation"`
}

// ShotWorkspace keeps one Shot's durable review state together for the creator workspace.
type ShotWorkspace struct {
	Shot    ShotUnit       `json:"shot"`
	History []ShotRevision `json:"history"`
	Impact  ShotImpact     `json:"impact"`
}

type ArtifactSelection struct {
	Kind    string   `json:"kind"`
	X       *float64 `json:"x,omitempty"`
	Y       *float64 `json:"y,omitempty"`
	Width   *float64 `json:"width,omitempty"`
	Height  *float64 `json:"height,omitempty"`
	StartMs *int64   `json:"startMs,omitempty"`
	EndMs   *int64   `json:"endMs,omitempty"`
}

type StepRevisionRequest struct {
	IdempotencyKey           string                 `json:"-"`
	ArtifactID               string                 `json:"artifactId"`
	BaseVersion              int                    `json:"baseVersion"`
	Mode                     string                 `json:"mode"`
	Instruction              string                 `json:"instruction,omitempty"`
	DirectContent            string                 `json:"directContent,omitempty"`
	ModelProviders           map[string]interface{} `json:"modelProviders,omitempty"`
	RunID                    string                 `json:"runId,omitempty"`
	ReviewID                 string                 `json:"reviewId,omitempty"`
	ConfirmedAffectedShotIDs []string               `json:"confirmedAffectedShotIds,omitempty"`
	Selection                *ArtifactSelection     `json:"selection,omitempty"`
}

type StepRestoreRequest struct {
	IdempotencyKey           string   `json:"-"`
	BaseVersion              int      `json:"baseVersion"`
	RunID                    string   `json:"runId,omitempty"`
	ReviewID                 string   `json:"reviewId,omitempty"`
	Reason                   string   `json:"reason,omitempty"`
	ConfirmedAffectedShotIDs []string `json:"confirmedAffectedShotIds,omitempty"`
}

type StepConfirmRequest struct {
	ArtifactID string `json:"artifactId"`
	RunID      string `json:"runId,omitempty"`
	ReviewID   string `json:"reviewId,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

type StepImpact struct {
	AffectedStepIDs      []CreatorStepID `json:"affectedStepIds"`
	AffectedShotIDs      []string        `json:"affectedShotIds,omitempty"`
	RequiresConfirmation bool            `json:"requiresConfirmation"`
}

type StepMutationResult struct {
	Artifact *artifact.Artifact `json:"artifact"`
	Impact   StepImpact         `json:"impact"`
	View     *CreationView      `json:"view"`
}

type CreatorArtifactVersion struct {
	ArtifactID string    `json:"artifactId"`
	Version    int       `json:"version"`
	IsCurrent  bool      `json:"isCurrent"`
	CreatedAt  time.Time `json:"createdAt"`
}

type StepVersions struct {
	Versions []CreatorArtifactVersion `json:"versions"`
}
