package model

import (
	"encoding/json"
	"time"
)

// BidStatus represents the lifecycle stage of a bid project.
type BidStatus string

const (
	BidDraft      BidStatus = "DRAFT"
	BidParsing    BidStatus = "PARSING"
	BidPlanning   BidStatus = "PLANNING"
	BidGenerating BidStatus = "GENERATING"
	BidReviewing  BidStatus = "REVIEWING"
	BidExporting  BidStatus = "EXPORTING"
	BidCompleted  BidStatus = "COMPLETED"
	BidFailed     BidStatus = "FAILED"
)

// ChapterStatus represents the workflow state of a bid chapter.
type ChapterStatus string

const (
	ChapterPending     ChapterStatus = "PENDING"
	ChapterGenerating  ChapterStatus = "GENERATING"
	ChapterReviewing   ChapterStatus = "REVIEWING"
	ChapterApproved    ChapterStatus = "APPROVED"
	ChapterRejected    ChapterStatus = "REJECTED"
)

// BidProject represents a single bid/tender generation project.
type BidProject struct {
	ID              string          `json:"id"`
	UserID          string          `json:"userId,omitempty"`
	Name            string          `json:"name"`
	Status          BidStatus       `json:"status"`
	TaskID          string          `json:"taskId,omitempty"`
	TemplateID      string          `json:"templateId,omitempty"`
	Industry        string          `json:"industry,omitempty"`
	TenderFilePath  string          `json:"tenderFilePath,omitempty"`
	TenderFileName  string          `json:"tenderFileName,omitempty"`
	TenderAnalysis  json.RawMessage `json:"tenderAnalysis,omitempty"`
	Structure       json.RawMessage `json:"structure,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	Progress        float64         `json:"progress"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// BidChapter represents a section within a bid project.
type BidChapter struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"projectId"`
	NodeID        string          `json:"nodeId,omitempty"`
	Title         string          `json:"title"`
	Content       string          `json:"content,omitempty"`
	Status        ChapterStatus   `json:"status"`
	ReviewComment string          `json:"reviewComment,omitempty"`
	ScoreItems    json.RawMessage `json:"scoreItems,omitempty"`
	SortOrder     int             `json:"sortOrder"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// BidTemplate is a reusable bid structure preset.
type BidTemplate struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Category    string          `json:"category,omitempty"`
	Industry    string          `json:"industry,omitempty"`
	Structure   json.RawMessage `json:"structure"`
	WorkflowDAG json.RawMessage `json:"workflowDag,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// ── API request/response types ──

// CreateProjectRequest is the request body for creating a bid project.
type CreateProjectRequest struct {
	Name       string `json:"name" binding:"required"`
	TemplateID string `json:"templateId,omitempty"`
	Industry   string `json:"industry,omitempty"`
	UserID     string `json:"userId,omitempty"`
}

// UpdateProjectRequest is the request body for updating a bid project.
type UpdateProjectRequest struct {
	Name      string          `json:"name,omitempty"`
	Structure json.RawMessage `json:"structure,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

// ApproveChapterRequest is the request body for approving a chapter.
type ApproveChapterRequest struct {
	Comment string `json:"comment,omitempty"`
}

// RejectChapterRequest is the request body for rejecting a chapter.
type RejectChapterRequest struct {
	Comment string `json:"comment" binding:"required"`
}

// ExportRequest is the request body for triggering document export.
type ExportRequest struct {
	Format    string          `json:"format"` // "docx" or "pdf"
	CoverInfo json.RawMessage `json:"coverInfo,omitempty"`
}

// ProgressResponse reports the current progress of a bid project.
type ProgressResponse struct {
	Stage       string  `json:"stage"`
	Progress    float64 `json:"progress"`
	CurrentStep string  `json:"currentStep"`
	TotalSteps  int     `json:"totalSteps"`
	TaskID      string  `json:"taskId,omitempty"`
}

// ExportStatusResponse reports the export job status.
type ExportStatusResponse struct {
	Status      string `json:"status"` // "EXPORTING", "COMPLETED", "FAILED"
	DownloadURL string `json:"downloadUrl,omitempty"`
	Error       string `json:"error,omitempty"`
}
