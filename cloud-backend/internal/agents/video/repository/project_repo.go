package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

// ProjectRepository provides data access for video projects.
type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

// Create inserts a new video project.
func (r *ProjectRepository) Create(ctx context.Context, p *model.VideoProject) error {
	if p.ID == "" {
		p.ID = "vp-" + uuid.NewString()[:8]
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO video_projects (id, user_id, name, description, mode, status,
		 skill_name, skill_version, workflow_name, workflow_version,
		 generation_mode, aspect_ratio, target_duration_sec, language,
		 config, current_run_id, local_path_hint, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		p.ID, p.UserID, p.Name, p.Description, string(p.Mode), string(p.Status),
		p.SkillName, p.SkillVersion, p.WorkflowName, p.WorkflowVersion,
		string(p.GenerationMode), p.AspectRatio, p.TargetDuration, p.Language,
		p.Config, p.CurrentRunID, p.LocalPathHint, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	return nil
}

// FindByID returns a project by ID (excludes soft-deleted).
func (r *ProjectRepository) FindByID(ctx context.Context, id string) (*model.VideoProject, error) {
	var p model.VideoProject
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, COALESCE(description, ''), mode, status,
		        skill_name, skill_version, workflow_name, workflow_version,
		        generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
		        COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), deleted_at, created_at, updated_at
		 FROM video_projects
		 WHERE id=$1 AND deleted_at IS NULL`, id,
	).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
		&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
		&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
		&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	return &p, nil
}

func (r *ProjectRepository) FindByIDForUser(ctx context.Context, userID string, id string) (*model.VideoProject, error) {
	var p model.VideoProject
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, COALESCE(description, ''), mode, status,
		        skill_name, skill_version, workflow_name, workflow_version,
		        generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
		        COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), deleted_at, created_at, updated_at
		 FROM video_projects
		 WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, userID,
	).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
		&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
		&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
		&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	return &p, nil
}

// FindAll returns projects with optional filters and pagination.
func (r *ProjectRepository) FindAll(ctx context.Context, modeFilter string, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error) {
	// Build dynamic query
	query := `SELECT id, user_id, name, COALESCE(description, ''), mode, status,
	           skill_name, skill_version, workflow_name, workflow_version,
	           generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
	           COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), deleted_at, created_at, updated_at
	 FROM video_projects WHERE deleted_at IS NULL`
	countQuery := `SELECT COUNT(*) FROM video_projects WHERE deleted_at IS NULL`

	args := []interface{}{}
	argIdx := 1

	if modeFilter != "" {
		query += fmt.Sprintf(" AND mode=$%d", argIdx)
		countQuery += fmt.Sprintf(" AND mode=$%d", argIdx)
		args = append(args, modeFilter)
		argIdx++
	}
	if statusFilter != "" {
		query += fmt.Sprintf(" AND status=$%d", argIdx)
		countQuery += fmt.Sprintf(" AND status=$%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	// Count
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query with pagination
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var projects []*model.VideoProject
	for rows.Next() {
		var p model.VideoProject
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
			&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
			&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
			&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		projects = append(projects, &p)
	}
	return projects, total, nil
}

func (r *ProjectRepository) FindAllForUser(ctx context.Context, userID string, modeFilter string, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error) {
	query := `SELECT id, user_id, name, COALESCE(description, ''), mode, status,
	           skill_name, skill_version, workflow_name, workflow_version,
	           generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
	           COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), deleted_at, created_at, updated_at
	 FROM video_projects WHERE deleted_at IS NULL AND user_id=$1`
	countQuery := `SELECT COUNT(*) FROM video_projects WHERE deleted_at IS NULL AND user_id=$1`

	args := []interface{}{userID}
	argIdx := 2

	if modeFilter != "" {
		query += fmt.Sprintf(" AND mode=$%d", argIdx)
		countQuery += fmt.Sprintf(" AND mode=$%d", argIdx)
		args = append(args, modeFilter)
		argIdx++
	}
	if statusFilter != "" {
		query += fmt.Sprintf(" AND status=$%d", argIdx)
		countQuery += fmt.Sprintf(" AND status=$%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var projects []*model.VideoProject
	for rows.Next() {
		var p model.VideoProject
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
			&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
			&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
			&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		projects = append(projects, &p)
	}
	return projects, total, nil
}

// Update modifies an existing project.
func (r *ProjectRepository) Update(ctx context.Context, p *model.VideoProject) error {
	p.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE video_projects SET name=$2, description=$3, status=$4,
		 generation_mode=$5, aspect_ratio=$6, target_duration_sec=$7,
		 language=$8, config=$9, current_run_id=$10, local_path_hint=$11,
		 updated_at=$12
		 WHERE id=$1 AND deleted_at IS NULL`,
		p.ID, p.Name, p.Description, string(p.Status),
		string(p.GenerationMode), p.AspectRatio, p.TargetDuration,
		p.Language, p.Config, p.CurrentRunID, p.LocalPathHint,
		p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) UpdateForUser(ctx context.Context, userID string, p *model.VideoProject) error {
	p.UpdatedAt = time.Now()
	tag, err := r.pool.Exec(ctx,
		`UPDATE video_projects SET name=$3, description=$4, status=$5,
		 generation_mode=$6, aspect_ratio=$7, target_duration_sec=$8,
		 language=$9, config=$10, current_run_id=$11, local_path_hint=$12,
		 updated_at=$13
		 WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`,
		p.ID, userID, p.Name, p.Description, string(p.Status),
		string(p.GenerationMode), p.AspectRatio, p.TargetDuration,
		p.Language, p.Config, p.CurrentRunID, p.LocalPathHint,
		p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// SoftDelete marks a project as deleted.
func (r *ProjectRepository) SoftDelete(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE video_projects SET deleted_at=$2, updated_at=$2 WHERE id=$1 AND deleted_at IS NULL`,
		id, now,
	)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

func (r *ProjectRepository) SoftDeleteForUser(ctx context.Context, userID string, id string) error {
	now := time.Now()
	tag, err := r.pool.Exec(ctx,
		`UPDATE video_projects SET deleted_at=$3, updated_at=$3 WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`,
		id, userID, now,
	)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}
