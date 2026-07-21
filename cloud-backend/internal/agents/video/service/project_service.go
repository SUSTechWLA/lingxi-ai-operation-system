package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	CompareAndSwapForUser(ctx context.Context, userID string, p *model.VideoProject, expectedRevision int64) (bool, error)
	SoftDeleteForUser(ctx context.Context, userID string, id string) error
}

func NewProjectService(repo ProjectStore) *ProjectService {
	return &ProjectService{repo: repo}
}

// CreateProject creates a new video project with validated mode and version locking.
func (s *ProjectService) CreateProject(ctx context.Context, userID string, req *model.CreateProjectRequest) (*model.VideoProject, error) {
	if !model.IsValidMode(req.Mode) {
		return nil, fmt.Errorf("invalid mode: %s (must be aigc_shot, voice_visual or cinematic_story)", req.Mode)
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if req.GenerationMode != "" && !model.IsValidGenerationMode(req.GenerationMode) {
		return nil, fmt.Errorf("invalid generation_mode: %s (must be provider_api or manual_import)", req.GenerationMode)
	}

	canonicalProfileID := model.CanonicalProfileForMode(req.Mode)
	persistedMode, _ := model.PersistedModeForProfile(canonicalProfileID)
	// Set defaults
	status := model.StatusDraft
	genMode := req.GenerationMode
	if genMode == "" {
		genMode = model.GenProviderAPI
	}
	skillName := req.SkillName
	if skillName == "" {
		switch req.Mode {
		case model.ModeAIGCShot:
			skillName = "aigc-shot-video"
		case model.ModeCinematicStory:
			skillName = "video-creator"
		default:
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
		UserID:             userID,
		Name:               req.Name,
		Description:        req.Description,
		Mode:               persistedMode,
		CanonicalProfileID: canonicalProfileID,
		Status:             status,
		SkillName:          skillName,
		SkillVersion:       skillVersion,
		WorkflowName:       workflowName,
		WorkflowVersion:    workflowVersion,
		GenerationMode:     genMode,
		AspectRatio:        req.AspectRatio,
		TargetDuration:     req.TargetDuration,
		Language:           language,
		Config:             canonicalProjectConfig(req.Config, canonicalProfileID),
		LocalPathHint:      req.LocalPathHint,
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
	project, err := s.repo.FindByIDForUser(ctx, userID, id)
	if project != nil {
		project.CanonicalProfileID = model.CanonicalProfileForMode(project.Mode)
	}
	return project, err
}

// ListProjects returns projects with optional filters and pagination.
func (s *ProjectService) ListProjects(ctx context.Context, userID string, modeFilter, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	projects, total, err := s.repo.FindAllForUser(ctx, userID, modeFilter, statusFilter, offset, limit)
	for _, project := range projects {
		if project != nil {
			project.CanonicalProfileID = model.CanonicalProfileForMode(project.Mode)
		}
	}
	return projects, total, err
}

func canonicalProjectConfig(raw json.RawMessage, canonicalProfileID string) json.RawMessage {
	config := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &config)
	}
	sanitizeProjectModelProviderConfig(config)
	config["canonicalProfileId"] = canonicalProfileID
	config["profileSchemaVersion"] = model.VideoProfileSchemaVersion
	config["runtimePipelineId"] = model.VideoRuntimePipelineID
	config["runtimePipelineVersion"] = model.VideoRuntimePipelineVersion
	config["runtimePipelineSource"] = model.VideoRuntimePipelineSource
	encoded, err := json.Marshal(config)
	if err != nil {
		return raw
	}
	return encoded
}

func sanitizeProjectModelProviderConfig(config map[string]interface{}) {
	delete(config, "modelProvider")
	delete(config, "modelProviders")
	rawRefs, _ := config["modelProviderRefs"].(map[string]interface{})
	refs := map[string]interface{}{}
	for _, capability := range []string{"text_to_text", "text_to_image", "text_to_video"} {
		raw, _ := rawRefs[capability].(map[string]interface{})
		source, _ := raw["source"].(string)
		baseURL, _ := raw["baseUrl"].(string)
		modelName, _ := raw["model"].(string)
		source = strings.TrimSpace(source)
		baseURL = strings.TrimSpace(baseURL)
		modelName = strings.TrimSpace(modelName)
		if source != "local_agent" || baseURL == "" || modelName == "" {
			continue
		}
		refs[capability] = map[string]interface{}{"source": source, "baseUrl": baseURL, "model": modelName}
	}
	if len(refs) == 0 {
		delete(config, "modelProviderRefs")
		return
	}
	config["modelProviderRefs"] = refs
}

func mergeProjectConfig(existing, update json.RawMessage, canonicalProfileID string) json.RawMessage {
	config := map[string]interface{}{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &config)
	}
	if len(update) > 0 {
		incoming := map[string]interface{}{}
		if json.Unmarshal(update, &incoming) == nil {
			for key, value := range incoming {
				config[key] = value
			}
		}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return canonicalProjectConfig(existing, canonicalProfileID)
	}
	return canonicalProjectConfig(encoded, canonicalProfileID)
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
	return s.saveExpectedRevision(ctx, userID, project)
}

// MarkAgentRunStopped marks a linked project as paused after a user stops the
// active dynamic agent run.
func (s *ProjectService) MarkAgentRunStopped(ctx context.Context, userID, projectID, runID string) error {
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
	project.Status = model.StatusPaused
	project.CurrentRunID = runID
	return s.saveExpectedRevision(ctx, userID, project)
}

// MarkAgentRunCompleted promotes the linked creator project to its durable
// terminal state once the dynamic agent task has completed successfully.
func (s *ProjectService) MarkAgentRunCompleted(ctx context.Context, userID, projectID, runID string) error {
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
	project.Status = model.StatusCompleted
	project.CurrentRunID = runID
	return s.saveExpectedRevision(ctx, userID, project)
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
		project.Config = mergeProjectConfig(project.Config, req.Config, model.CanonicalProfileForMode(project.Mode))
	}
	if req.LocalPathHint != "" {
		project.LocalPathHint = req.LocalPathHint
	}

	if err := s.saveExpectedRevision(ctx, userID, project); err != nil {
		return nil, err
	}

	zap.L().Info("Video project updated", zap.String("id", project.ID))
	return project, nil
}

func (s *ProjectService) saveExpectedRevision(ctx context.Context, userID string, project *model.VideoProject) error {
	expectedRevision := project.ConfigRevision
	swapped, err := s.repo.CompareAndSwapForUser(ctx, userID, project, expectedRevision)
	if err != nil {
		return err
	}
	if !swapped {
		return fmt.Errorf("%w: project %s changed concurrently", errProjectRevisionConflict, project.ID)
	}
	return nil
}

// ArchiveProject soft-deletes a project.
func (s *ProjectService) ArchiveProject(ctx context.Context, userID string, id string) error {
	return s.repo.SoftDeleteForUser(ctx, userID, id)
}

var _ ProjectStore = (*repository.ProjectRepository)(nil)
