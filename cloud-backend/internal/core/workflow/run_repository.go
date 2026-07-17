package workflow

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunRepository provides data access for WorkflowRun and StageRun.
type RunRepository struct {
	pool *pgxpool.Pool
}

func NewRunRepository(pool *pgxpool.Pool) *RunRepository {
	return &RunRepository{pool: pool}
}

// Create inserts a new WorkflowRun.
func (r *RunRepository) Create(ctx context.Context, run *WorkflowRun) error {
	if run.ID == "" {
		run.ID = "wfr-" + uuid.NewString()[:8]
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO workflow_runs (id, project_id, user_id, template_id, template_version,
		 task_id, status, attempt, input, output, stage_statuses, trace_id, started_at, finished_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		run.ID, run.ProjectID, run.UserID, run.TemplateID, run.TemplateVersion,
		run.TaskID, string(run.Status), run.Attempt, run.Input, run.Output,
		run.StageStatuses, run.TraceID, run.StartedAt, run.FinishedAt, run.CreatedAt,
	)
	return err
}

// FindByID returns a WorkflowRun by ID.
func (r *RunRepository) FindByID(ctx context.Context, id string) (*WorkflowRun, error) {
	var run WorkflowRun
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        trace_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE id=$1`, id,
	).Scan(
		&run.ID, &run.ProjectID, &run.UserID, &run.TemplateID, &run.TemplateVersion,
		&run.TaskID, &run.Status, &run.Attempt, &run.Input, &run.Output,
		&run.StageStatuses, &run.TraceID, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// FindByTaskID returns the WorkflowRun associated with the given orchestrator task.
func (r *RunRepository) FindByTaskID(ctx context.Context, taskID string) (*WorkflowRun, error) {
	var run WorkflowRun
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        trace_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE task_id=$1`, taskID,
	).Scan(
		&run.ID, &run.ProjectID, &run.UserID, &run.TemplateID, &run.TemplateVersion,
		&run.TaskID, &run.Status, &run.Attempt, &run.Input, &run.Output,
		&run.StageStatuses, &run.TraceID, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// FindByProject returns all runs for a project, newest first.
func (r *RunRepository) FindByProject(ctx context.Context, projectID string) ([]*WorkflowRun, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        trace_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE project_id=$1 ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		if err := rows.Scan(
			&run.ID, &run.ProjectID, &run.UserID, &run.TemplateID, &run.TemplateVersion,
			&run.TaskID, &run.Status, &run.Attempt, &run.Input, &run.Output,
			&run.StageStatuses, &run.TraceID, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
		); err != nil {
			return nil, err
		}
		runs = append(runs, &run)
	}
	return runs, nil
}

// UpdateStatus updates the run's status.
func (r *RunRepository) UpdateStatus(ctx context.Context, runID string, status RunStatus) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_runs SET status=$2, started_at=COALESCE(started_at,$3),
		 finished_at=CASE WHEN $2 IN ('COMPLETED','FAILED','CANCELLED') THEN $3 ELSE finished_at END
		 WHERE id=$1`, runID, string(status), now,
	)
	return err
}

// FindRunIDByTaskID returns the run ID associated with a given task ID.
func (r *RunRepository) FindRunIDByTaskID(ctx context.Context, taskID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM workflow_runs WHERE task_id=$1`, taskID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateStageStatus updates a single stage's status in the run's stage_statuses JSONB.
func (r *RunRepository) UpdateStageStatus(ctx context.Context, runID, stageName string, status StageStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE workflow_runs SET stage_statuses = jsonb_set(stage_statuses, $2::text[], to_jsonb($3::text), true)
		 WHERE id=$1`, runID, stageStatusJSONBPath(stageName), string(status),
	)
	return err
}

func stageStatusJSONBPath(stageName string) []string {
	return []string{stageName}
}

// SaveAttempt records a new execution attempt for a stage.
func (r *RunRepository) SaveAttempt(ctx context.Context, attempt *Attempt) error {
	if attempt.ID == "" {
		attempt.ID = "att-" + uuid.NewString()[:8]
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO workflow_attempts (id, stage_run_id, number, trigger_type, node_ids, status, error_message, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		attempt.ID, attempt.StageRunID, attempt.Number, attempt.TriggerType,
		attempt.NodeIDs, attempt.Status, attempt.ErrorMessage, attempt.CreatedAt,
	)
	return err
}
