package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	bidmodel "github.com/tangying-ai/aios-core/internal/agents/bid/model"
	bidrepo "github.com/tangying-ai/aios-core/internal/agents/bid/repository"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
)

// BidService orchestrates bid project lifecycle on top of AIOS Core.
type BidService struct {
	repo         *bidrepo.BidRepository
	orchService  *service.OrchestratorService
	taskControl  *service.TaskExecutionControl
	stateService *service.StateService
	taskRepo     repository.TaskRepo
	nodeRepo     repository.NodeRepo
}

// NewBidService creates a new BidService.
func NewBidService(
	repo *bidrepo.BidRepository,
	orchService *service.OrchestratorService,
	taskControl *service.TaskExecutionControl,
	stateService *service.StateService,
	taskRepo repository.TaskRepo,
	nodeRepo repository.NodeRepo,
) *BidService {
	return &BidService{
		repo:         repo,
		orchService:  orchService,
		taskControl:  taskControl,
		stateService: stateService,
		taskRepo:     taskRepo,
		nodeRepo:     nodeRepo,
	}
}

// ── Project CRUD ──

// CreateProject creates a new bid project.
func (s *BidService) CreateProject(ctx context.Context, userID string, req *bidmodel.CreateProjectRequest) (*bidmodel.BidProject, error) {
	now := time.Now()
	project := &bidmodel.BidProject{
		ID:         "bid-" + uuid.NewString()[:8],
		UserID:     userID,
		Name:       req.Name,
		Status:     bidmodel.BidDraft,
		TemplateID: req.TemplateID,
		Industry:   req.Industry,
		Config:     json.RawMessage("{}"),
		Progress:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.SaveProject(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to save project: %w", err)
	}

	zap.L().Info("Bid project created", zap.String("projectId", project.ID), zap.String("name", project.Name))
	return project, nil
}

// GetProject retrieves a bid project with its chapters.
func (s *BidService) GetProject(ctx context.Context, userID string, projectID string) (*bidmodel.BidProject, []*bidmodel.BidChapter, error) {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("project not found: %w", err)
	}

	chapters, err := s.repo.FindChaptersByProject(ctx, projectID)
	if err != nil {
		zap.L().Warn("Failed to load chapters", zap.String("projectId", projectID), zap.Error(err))
		chapters = []*bidmodel.BidChapter{}
	}

	return project, chapters, nil
}

// ListProjects lists bid projects with pagination and filters.
func (s *BidService) ListProjects(ctx context.Context, status, userID string, offset, limit int) ([]*bidmodel.BidProject, int, error) {
	return s.repo.FindProjects(ctx, status, userID, offset, limit)
}

// UpdateProject updates a bid project's structure or config.
func (s *BidService) UpdateProject(ctx context.Context, userID string, projectID string, req *bidmodel.UpdateProjectRequest) (*bidmodel.BidProject, error) {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Structure != nil {
		project.Structure = req.Structure
	}
	if req.Config != nil {
		project.Config = req.Config
	}
	project.UpdatedAt = time.Now()

	if err := s.repo.SaveProject(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}
	return project, nil
}

// DeleteProject removes a bid project.
func (s *BidService) DeleteProject(ctx context.Context, userID string, projectID string) error {
	return s.repo.DeleteProjectForUser(ctx, userID, projectID)
}

// ── Tender Upload ──

// SetTenderFile records the uploaded tender file path on the project.
func (s *BidService) SetTenderFile(ctx context.Context, userID string, projectID, filePath, fileName string) error {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	project.TenderFilePath = filePath
	project.TenderFileName = fileName
	project.Status = bidmodel.BidParsing
	project.UpdatedAt = time.Now()

	return s.repo.SaveProject(ctx, project)
}

// ── Generation Lifecycle ──

// StartGeneration creates an orchestrator task and submits the bid DAG.
func (s *BidService) StartGeneration(ctx context.Context, userID string, projectID string) (string, error) {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return "", fmt.Errorf("project not found: %w", err)
	}

	// Create orchestrator task
	task, err := s.orchService.CreateTask(ctx, map[string]interface{}{
		"projectId": projectID,
		"source":    "bid-generator",
		"user_id":   userID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}
	taskID := task.ID

	// Build the bid generation DAG
	dag := BuildBidDAG(project)

	// Submit DAG to orchestrator
	if err := s.orchService.SubmitDAG(ctx, taskID, dag); err != nil {
		return "", fmt.Errorf("failed to submit DAG: %w", err)
	}

	// Update project with task ID
	project.TaskID = taskID
	project.Status = bidmodel.BidGenerating
	project.UpdatedAt = time.Now()
	if err := s.repo.SaveProject(ctx, project); err != nil {
		zap.L().Warn("Failed to update project task_id", zap.Error(err))
	}

	// Sync chapters with DAG nodes
	if err := s.syncChaptersFromDAG(ctx, projectID, dag); err != nil {
		zap.L().Warn("Failed to sync chapters", zap.Error(err))
	}

	zap.L().Info("Bid generation started",
		zap.String("projectId", projectID),
		zap.String("taskId", taskID),
		zap.Int("nodes", len(dag.Nodes)),
	)
	return taskID, nil
}

// PauseGeneration pauses the bid generation task.
func (s *BidService) PauseGeneration(ctx context.Context, userID string, projectID string) error {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return err
	}
	if project.TaskID == "" {
		return fmt.Errorf("no active task")
	}
	return s.taskControl.PauseTask(ctx, project.TaskID, "Paused by user")
}

// ResumeGeneration resumes a paused bid generation task.
func (s *BidService) ResumeGeneration(ctx context.Context, userID string, projectID string) error {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return err
	}
	if project.TaskID == "" {
		return fmt.Errorf("no active task")
	}
	return s.taskControl.ResumeTask(ctx, project.TaskID)
}

// ── Chapter Review ──

// ApproveChapter approves a chapter and triggers the CONTROL node to succeed.
func (s *BidService) ApproveChapter(ctx context.Context, userID string, projectID, chapterID string) (*bidmodel.BidChapter, error) {
	if _, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	chapter, err := s.repo.FindChapterByID(ctx, chapterID)
	if err != nil {
		return nil, fmt.Errorf("chapter not found: %w", err)
	}

	// Mark CONTROL node as SUCCESS (this resumes the DAG)
	if chapter.NodeID != "" {
		if _, err := s.stateService.TransitionNode(ctx, chapter.NodeID, model.NodeSuccess, nil, ""); err != nil {
			zap.L().Warn("Failed to transition node to SUCCESS", zap.Error(err))
		}
	}

	chapter.Status = bidmodel.ChapterApproved
	chapter.UpdatedAt = time.Now()
	if err := s.repo.SaveChapter(ctx, chapter); err != nil {
		return nil, err
	}

	return chapter, nil
}

// RejectChapter rejects a chapter and triggers the CONTROL node to fail (retry).
func (s *BidService) RejectChapter(ctx context.Context, userID string, projectID, chapterID, comment string) (*bidmodel.BidChapter, error) {
	if _, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	chapter, err := s.repo.FindChapterByID(ctx, chapterID)
	if err != nil {
		return nil, fmt.Errorf("chapter not found: %w", err)
	}

	// Mark CONTROL node as FAILED (this triggers retry of the generation node)
	if chapter.NodeID != "" {
		if _, err := s.stateService.TransitionNode(ctx, chapter.NodeID, model.NodeFailed, nil, comment); err != nil {
			zap.L().Warn("Failed to transition node to FAILED", zap.Error(err))
		}
	}

	chapter.Status = bidmodel.ChapterRejected
	chapter.ReviewComment = comment
	chapter.UpdatedAt = time.Now()
	if err := s.repo.SaveChapter(ctx, chapter); err != nil {
		return nil, err
	}

	return chapter, nil
}

// ── Progress ──

// GetProgress returns the current progress of a bid project.
func (s *BidService) GetProgress(ctx context.Context, userID string, projectID string) (*bidmodel.ProgressResponse, error) {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	chapters, _ := s.repo.FindChaptersByProject(ctx, projectID)
	totalSteps := len(chapters)
	currentStep := 0
	for _, ch := range chapters {
		if ch.Status == bidmodel.ChapterApproved {
			currentStep++
		}
	}

	progress := float64(0)
	if totalSteps > 0 {
		progress = float64(currentStep) / float64(totalSteps)
	}

	return &bidmodel.ProgressResponse{
		Stage:       string(project.Status),
		Progress:    progress,
		CurrentStep: fmt.Sprintf("已完成 %d/%d 章", currentStep, totalSteps),
		TotalSteps:  totalSteps,
		TaskID:      project.TaskID,
	}, nil
}

// ── Templates ──

// ListTemplates returns all available bid templates.
func (s *BidService) ListTemplates(ctx context.Context) ([]*bidmodel.BidTemplate, error) {
	return s.repo.FindTemplates(ctx)
}

// ── Export ──

// GetExportStatus returns the export node status and download URL.
func (s *BidService) GetExportStatus(ctx context.Context, userID string, projectID string) (*bidmodel.ExportStatusResponse, error) {
	project, err := s.repo.FindProjectByIDForUser(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	// Find the export node in the task
	if project.TaskID == "" {
		return &bidmodel.ExportStatusResponse{Status: "NOT_STARTED"}, nil
	}

	nodes, err := s.nodeRepo.FindByTaskID(ctx, project.TaskID)
	if err != nil {
		return &bidmodel.ExportStatusResponse{Status: "EXPORTING"}, nil
	}

	for _, n := range nodes {
		if n.Name == "doc_exporter" {
			if n.Status == model.NodeSuccess {
				downloadURL, _ := n.Output["download_url"].(string)
				return &bidmodel.ExportStatusResponse{
					Status:      "COMPLETED",
					DownloadURL: downloadURL,
				}, nil
			}
			if n.Status == model.NodeFailed {
				return &bidmodel.ExportStatusResponse{
					Status: "FAILED",
					Error:  n.ErrorMessage,
				}, nil
			}
			return &bidmodel.ExportStatusResponse{Status: "EXPORTING"}, nil
		}
	}

	return &bidmodel.ExportStatusResponse{Status: "EXPORTING"}, nil
}

// ── Helpers ──

// syncChaptersFromDAG creates bid_chapter records for each chapter generation node in the DAG.
func (s *BidService) syncChaptersFromDAG(ctx context.Context, projectID string, dag *model.DAGRequest) error {
	for i, node := range dag.Nodes {
		if node.Type != "TOOL" || node.Name != "chapter_generator" {
			continue
		}

		title := ""
		if chapterData, ok := node.Input["chapter"].(map[string]interface{}); ok {
			if t, ok := chapterData["title"].(string); ok {
				title = t
			}
		}
		if title == "" {
			title = node.Name
		}

		chapter := &bidmodel.BidChapter{
			ID:        fmt.Sprintf("%s-ch-%d", projectID, i),
			ProjectID: projectID,
			NodeID:    node.ID,
			Title:     title,
			Status:    bidmodel.ChapterPending,
			SortOrder: i,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.repo.SaveChapter(ctx, chapter); err != nil {
			return err
		}
	}
	return nil
}
