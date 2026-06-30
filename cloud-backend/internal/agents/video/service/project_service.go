package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/agents/video/repository"
)

// ProjectService provides business logic for video projects.
type ProjectService struct {
	repo ProjectStore
}

type ProjectStore interface {
	Create(ctx context.Context, p *model.VideoProject) error
	FindByIDForUser(ctx context.Context, userID string, id string) (*model.VideoProject, error)
	FindAllForUser(ctx context.Context, userID string, modeFilter string, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error)
	UpdateForUser(ctx context.Context, userID string, p *model.VideoProject) error
	SoftDeleteForUser(ctx context.Context, userID string, id string) error
}

func NewProjectService(repo ProjectStore) *ProjectService {
	return &ProjectService{repo: repo}
}

// CreateProject creates a new video project with validated mode and version locking.
func (s *ProjectService) CreateProject(ctx context.Context, userID string, req *model.CreateProjectRequest) (*model.VideoProject, error) {
	if !model.IsValidMode(req.Mode) {
		return nil, fmt.Errorf("invalid mode: %s (must be aigc_shot or voice_visual)", req.Mode)
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if req.GenerationMode != "" && !model.IsValidGenerationMode(req.GenerationMode) {
		return nil, fmt.Errorf("invalid generation_mode: %s (must be provider_api or manual_import)", req.GenerationMode)
	}

	// Set defaults
	status := model.StatusDraft
	genMode := req.GenerationMode
	if genMode == "" {
		genMode = model.GenProviderAPI
	}
	skillName := req.SkillName
	if skillName == "" {
		if req.Mode == model.ModeAIGCShot {
			skillName = "aigc-shot-video"
		} else {
			skillName = "voice-visual-video"
		}
	}
	skillVersion := req.SkillVersion
	if skillVersion == "" {
		skillVersion = "1.0.0"
	}
	workflowName := req.WorkflowName
	if workflowName == "" {
		workflowName = skillName + "-workflow"
	}
	workflowVersion := req.WorkflowVersion
	if workflowVersion == "" {
		workflowVersion = "1.0.0"
	}
	language := req.Language
	if language == "" {
		language = "zh-CN"
	}

	project := &model.VideoProject{
		UserID:          userID,
		Name:            req.Name,
		Description:     req.Description,
		Mode:            req.Mode,
		Status:          status,
		SkillName:       skillName,
		SkillVersion:    skillVersion,
		WorkflowName:    workflowName,
		WorkflowVersion: workflowVersion,
		GenerationMode:  genMode,
		AspectRatio:     req.AspectRatio,
		TargetDuration:  req.TargetDuration,
		Language:        language,
		Config:          req.Config,
		LocalPathHint:   req.LocalPathHint,
	}

	if err := s.repo.Create(ctx, project); err != nil {
		return nil, err
	}

	zap.L().Info("Video project created",
		zap.String("id", project.ID),
		zap.String("name", project.Name),
		zap.String("mode", string(project.Mode)),
	)
	return project, nil
}

// GetProject returns a project by ID.
func (s *ProjectService) GetProject(ctx context.Context, userID string, id string) (*model.VideoProject, error) {
	return s.repo.FindByIDForUser(ctx, userID, id)
}

// ListProjects returns projects with optional filters and pagination.
func (s *ProjectService) ListProjects(ctx context.Context, userID string, modeFilter, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.FindAllForUser(ctx, userID, modeFilter, statusFilter, offset, limit)
}

// MarkAgentRunStarted links an agent run to a project and marks it running.
func (s *ProjectService) MarkAgentRunStarted(ctx context.Context, userID, projectID, runID string) error {
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}
	if projectID == "" {
		return fmt.Errorf("project_id is required")
	}
	project, err := s.repo.FindByIDForUser(ctx, userID, projectID)
	if err != nil {
		return err
	}
	project.Status = model.StatusRunning
	project.CurrentRunID = runID
	return s.repo.UpdateForUser(ctx, userID, project)
}

// UpdateProject updates a project. Mode and version fields cannot be changed.
func (s *ProjectService) UpdateProject(ctx context.Context, userID string, id string, req *model.UpdateProjectRequest) (*model.VideoProject, error) {
	project, err := s.repo.FindByIDForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = req.Description
	}
	if req.Status != "" {
		project.Status = req.Status
	}
	if req.GenerationMode != "" {
		if !model.IsValidGenerationMode(req.GenerationMode) {
			return nil, fmt.Errorf("invalid generation_mode: %s", req.GenerationMode)
		}
		project.GenerationMode = req.GenerationMode
	}
	if req.AspectRatio != "" {
		project.AspectRatio = req.AspectRatio
	}
	if req.TargetDuration != nil {
		project.TargetDuration = *req.TargetDuration
	}
	if req.Language != "" {
		project.Language = req.Language
	}
	if req.Config != nil {
		project.Config = req.Config
	}
	if req.LocalPathHint != "" {
		project.LocalPathHint = req.LocalPathHint
	}

	if err := s.repo.UpdateForUser(ctx, userID, project); err != nil {
		return nil, err
	}

	zap.L().Info("Video project updated", zap.String("id", project.ID))
	return project, nil
}

// ArchiveProject soft-deletes a project.
func (s *ProjectService) ArchiveProject(ctx context.Context, userID string, id string) error {
	return s.repo.SoftDeleteForUser(ctx, userID, id)
}

var _ ProjectStore = (*repository.ProjectRepository)(nil)
