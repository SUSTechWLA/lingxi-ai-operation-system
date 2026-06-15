package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/orchestrator/service"
)

// Service manages workflow templates and instantiation.
type Service struct {
	repo         *Repository
	orchService  *service.OrchestratorService
}

func NewService(repo *Repository, orchService *service.OrchestratorService) *Service {
	return &Service{repo: repo, orchService: orchService}
}

// List returns all workflow templates.
func (s *Service) List(ctx context.Context) ([]*Template, error) {
	return s.repo.FindAll(ctx)
}

// Get returns a single template by ID.
func (s *Service) Get(ctx context.Context, id string) (*Template, error) {
	return s.repo.FindByID(ctx, id)
}

// Create saves a new workflow template.
func (s *Service) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	now := time.Now()
	t := &Template{
		ID:          "wf-" + uuid.NewString()[:8],
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		DAG:         req.DAG,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}
	zap.L().Info("Workflow template created", zap.String("id", t.ID), zap.String("name", t.Name))
	return t, nil
}

// Update modifies an existing template.
func (s *Service) Update(ctx context.Context, id string, req *UpdateTemplateRequest) (*Template, error) {
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Description != "" {
		t.Description = req.Description
	}
	if req.Category != "" {
		t.Category = req.Category
	}
	if req.DAG != nil {
		t.DAG = req.DAG
	}
	t.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, fmt.Errorf("failed to update: %w", err)
	}
	return t, nil
}

// Delete removes a template.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// Instantiate creates a Task and submits the template's DAG to the orchestrator.
// This is the core "instantiate a workflow" operation.
func (s *Service) Instantiate(ctx context.Context, templateID string, overrides map[string]interface{}) (string, error) {
	t, err := s.repo.FindByID(ctx, templateID)
	if err != nil {
		return "", fmt.Errorf("template not found: %w", err)
	}

	// Parse the DAG from JSONB
	var dag model.DAGRequest
	if err := json.Unmarshal(t.DAG, &dag); err != nil {
		return "", fmt.Errorf("invalid DAG in template: %w", err)
	}

	// Apply overrides to node inputs (e.g. custom file paths, prompts)
	if overrides != nil {
		for i := range dag.Nodes {
			for k, v := range overrides {
				if dag.Nodes[i].Input == nil {
					dag.Nodes[i].Input = make(map[string]interface{})
				}
				dag.Nodes[i].Input[k] = v
			}
		}
	}

	// Create task
	task, err := s.orchService.CreateTask(ctx, map[string]interface{}{
		"source":       "workflow-template",
		"template_id":  templateID,
		"template_name": t.Name,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	// Submit DAG
	if err := s.orchService.SubmitDAG(ctx, task.ID, &dag); err != nil {
		return "", fmt.Errorf("failed to submit DAG: %w", err)
	}

	zap.L().Info("Workflow instantiated",
		zap.String("templateId", templateID),
		zap.String("taskId", task.ID),
		zap.Int("nodes", len(dag.Nodes)),
	)
	return task.ID, nil
}
