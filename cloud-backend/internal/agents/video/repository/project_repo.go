package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

// ProjectRepository provides data access for video projects.
type ProjectRepository struct {
	db projectDB
}

type projectDB interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

const pendingShotRegenerationQuery = `
	SELECT projects.user_id, projects.id, tasks.key
	FROM video_projects AS projects
	CROSS JOIN LATERAL jsonb_each(COALESCE(projects.config->'shotDrivenState'->'regenerationTasks', '{}'::jsonb)) AS tasks(key, value)
	WHERE projects.deleted_at IS NULL
	  AND (
		tasks.value->>'status' = 'queued'
		OR (
			tasks.value->>'status' = 'dispatching'
			AND COALESCE(NULLIF(tasks.value->>'dispatchLeaseUntil', '')::timestamptz, '-infinity'::timestamptz) <= NOW()
		)
	  )
	ORDER BY projects.updated_at ASC, tasks.key ASC
	LIMIT $1`

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{db: pool}
}

func (r *ProjectRepository) FindPendingShotRegenerations(ctx context.Context, limit int) ([]model.PendingShotRegeneration, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, pendingShotRegenerationQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("find pending shot regenerations: %w", err)
	}
	defer rows.Close()
	pending := make([]model.PendingShotRegeneration, 0, limit)
	for rows.Next() {
		var item model.PendingShotRegeneration
		if err := rows.Scan(&item.UserID, &item.ProjectID, &item.TaskID); err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	return pending, rows.Err()
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

	_, err := r.db.Exec(ctx,
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
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, name, COALESCE(description, ''), mode, status,
		        skill_name, skill_version, workflow_name, workflow_version,
		        generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
		        COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), COALESCE(config_revision, 0), deleted_at, created_at, updated_at
		 FROM video_projects
		 WHERE id=$1 AND deleted_at IS NULL`, id,
	).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
		&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
		&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
		&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.ConfigRevision, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	return &p, nil
}

func (r *ProjectRepository) FindByIDForUser(ctx context.Context, userID string, id string) (*model.VideoProject, error) {
	var p model.VideoProject
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, name, COALESCE(description, ''), mode, status,
		        skill_name, skill_version, workflow_name, workflow_version,
		        generation_mode, COALESCE(aspect_ratio, ''), COALESCE(target_duration_sec, 0), COALESCE(language, ''),
		        COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), COALESCE(config_revision, 0), deleted_at, created_at, updated_at
		 FROM video_projects
		 WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, id, userID,
	).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Description, &p.Mode, &p.Status,
		&p.SkillName, &p.SkillVersion, &p.WorkflowName, &p.WorkflowVersion,
		&p.GenerationMode, &p.AspectRatio, &p.TargetDuration, &p.Language,
		&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.ConfigRevision, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
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
	           COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), COALESCE(config_revision, 0), deleted_at, created_at, updated_at
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
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Query with pagination
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
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
			&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.ConfigRevision, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
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
	           COALESCE(config, '{}'::jsonb), COALESCE(current_run_id, ''), COALESCE(local_path_hint, ''), COALESCE(config_revision, 0), deleted_at, created_at, updated_at
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
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
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
			&p.Config, &p.CurrentRunID, &p.LocalPathHint, &p.ConfigRevision, &p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		projects = append(projects, &p)
	}
	return projects, total, nil
}

// Update modifies an existing project.
func (r *ProjectRepository) Update(ctx context.Context, p *model.VideoProject) error {
	return fmt.Errorf("unconditional project update is disabled; use compare-and-swap with an expected config revision")
}

func (r *ProjectRepository) UpdateForUser(ctx context.Context, userID string, p *model.VideoProject) error {
	return fmt.Errorf("unconditional project update is disabled; use compare-and-swap with an expected config revision")
}

func (r *ProjectRepository) CompareAndSwapForUser(ctx context.Context, userID string, p *model.VideoProject, expectedRevision int64) (bool, error) {
	p.UpdatedAt = time.Now()
	nextRevision := expectedRevision + 1
	tag, err := r.db.Exec(ctx,
		`WITH revision_guard AS (
		 SELECT set_config('app.video_project_expected_revision', $15::text, true) AS expected_revision
		)
		UPDATE video_projects SET name=$3, description=$4, status=$5,
		 generation_mode=$6, aspect_ratio=$7, target_duration_sec=$8,
		 language=$9, config=$10, current_run_id=$11, local_path_hint=$12,
		 updated_at=$13, config_revision=$14
		 FROM revision_guard
		 WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL AND config_revision=$15
		 AND revision_guard.expected_revision=$15::text`,
		p.ID, userID, p.Name, p.Description, string(p.Status),
		string(p.GenerationMode), p.AspectRatio, p.TargetDuration,
		p.Language, p.Config, p.CurrentRunID, p.LocalPathHint,
		p.UpdatedAt, nextRevision, expectedRevision,
	)
	if err != nil {
		return false, fmt.Errorf("failed to compare and swap project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	p.ConfigRevision = nextRevision
	return true, nil
}

// SoftDelete marks a project as deleted.
func (r *ProjectRepository) SoftDelete(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.db.Exec(ctx,
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
	tag, err := r.db.Exec(ctx,
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
