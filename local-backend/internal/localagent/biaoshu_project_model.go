package localagent

import (
	"errors"
	"fmt"
	"strings"
)

const BiaoshuProjectSchemaVersion = "biaoshu.project.v1"

type BiaoshuProjectStage string

const (
	StageCreated        BiaoshuProjectStage = "created"
	StageRawParsed      BiaoshuProjectStage = "raw_parsed"
	StageAnalysisReady  BiaoshuProjectStage = "analysis_ready"
	StageContextReady   BiaoshuProjectStage = "context_ready"
	StageScoringReady   BiaoshuProjectStage = "scoring_ready"
	StageOutlineReady   BiaoshuProjectStage = "outline_ready"
	StageChaptersReady  BiaoshuProjectStage = "chapters_ready"
	StageWordcheckReady BiaoshuProjectStage = "wordcheck_ready"
	StageDraftMerged    BiaoshuProjectStage = "draft_merged"
	StageWordExported   BiaoshuProjectStage = "word_exported"
	StageFailed         BiaoshuProjectStage = "failed"
)

type BiaoshuArtifactKind string

const (
	ArtifactKindRawText          BiaoshuArtifactKind = "BID_RAW_TEXT"
	ArtifactKindBidAnalysis      BiaoshuArtifactKind = "BID_ANALYSIS"
	ArtifactKindProjectContext   BiaoshuArtifactKind = "BID_PROJECT_CONTEXT"
	ArtifactKindScoringBreakdown BiaoshuArtifactKind = "BID_SCORING_BREAKDOWN"
	ArtifactKindOutline          BiaoshuArtifactKind = "BID_OUTLINE"
	ArtifactKindChapters         BiaoshuArtifactKind = "BID_CHAPTERS"
	ArtifactKindWordCount        BiaoshuArtifactKind = "WORD_COUNT_REPORT"
	ArtifactKindMergedDraft      BiaoshuArtifactKind = "MERGED_DRAFT"
	ArtifactKindDocx             BiaoshuArtifactKind = "TECHNICAL_BID_DOCX"
)

type BiaoshuProjectManifest struct {
	SchemaVersion string                     `json:"schemaVersion"`
	ProjectID     string                     `json:"projectId"`
	ProjectName   string                     `json:"projectName"`
	Status        string                     `json:"status"`
	CurrentStage  BiaoshuProjectStage        `json:"currentStage"`
	CreatedAt     string                     `json:"createdAt"`
	UpdatedAt     string                     `json:"updatedAt"`
	SourceFiles   []BiaoshuProjectSourceFile `json:"sourceFiles"`
	OutputDir     string                     `json:"outputDir"`
	Runs          []BiaoshuProjectRun        `json:"runs"`
	Artifacts     []BiaoshuProjectArtifact   `json:"artifacts"`
	StageEvents   []BiaoshuProjectStageEvent `json:"stageEvents"`
}

type BiaoshuProjectSourceFile struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	OriginalName string `json:"originalName"`
	SHA256       string `json:"sha256,omitempty"`
	AddedAt      string `json:"addedAt"`
}

type BiaoshuProjectRun struct {
	RunID          string `json:"runId"`
	CloudTaskID    string `json:"cloudTaskId,omitempty"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	EndedAt        string `json:"endedAt,omitempty"`
	CloudAvailable bool   `json:"cloudAvailable,omitempty"`
}

type BiaoshuProjectArtifact struct {
	ID           string                 `json:"id"`
	Kind         BiaoshuArtifactKind    `json:"kind"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	StorageRef   string                 `json:"storageRef"`
	MimeType     string                 `json:"mimeType"`
	SourceFileID string                 `json:"sourceFileId,omitempty"`
	DependsOn    []string               `json:"dependsOn"`
	CreatedAt    string                 `json:"createdAt"`
	UpdatedAt    string                 `json:"updatedAt"`
	Metadata     map[string]interface{} `json:"metadata"`
}

type BiaoshuProjectStageEvent struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	At         string `json:"at"`
	Message    string `json:"message"`
	ArtifactID string `json:"artifactId,omitempty"`
	RunID      string `json:"runId,omitempty"`
}

func validateBiaoshuProjectManifest(manifest BiaoshuProjectManifest) error {
	if manifest.SchemaVersion != BiaoshuProjectSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.ProjectID) == "" {
		return errors.New("projectId is required")
	}
	if strings.TrimSpace(manifest.ProjectName) == "" {
		return errors.New("projectName is required")
	}
	if strings.TrimSpace(manifest.OutputDir) == "" {
		return errors.New("outputDir is required")
	}
	return nil
}

func biaoshuStageFromArtifacts(artifacts []BiaoshuProjectArtifact, status string) BiaoshuProjectStage {
	if status == "FAILED" {
		return StageFailed
	}
	valid := map[BiaoshuArtifactKind]bool{}
	for _, artifact := range artifacts {
		if artifact.Status == "valid" && strings.TrimSpace(artifact.StorageRef) != "" {
			valid[artifact.Kind] = true
		}
	}
	switch {
	case valid[ArtifactKindDocx]:
		return StageWordExported
	case valid[ArtifactKindMergedDraft]:
		return StageDraftMerged
	case valid[ArtifactKindWordCount]:
		return StageWordcheckReady
	case valid[ArtifactKindChapters]:
		return StageChaptersReady
	case valid[ArtifactKindOutline]:
		return StageOutlineReady
	case valid[ArtifactKindScoringBreakdown]:
		return StageScoringReady
	case valid[ArtifactKindProjectContext]:
		return StageContextReady
	case valid[ArtifactKindBidAnalysis]:
		return StageAnalysisReady
	case valid[ArtifactKindRawText]:
		return StageRawParsed
	default:
		return StageCreated
	}
}
