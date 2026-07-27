package workflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/database"
)

// RunRepository provides data access for WorkflowRun and StageRun.
type RunRepository struct {
	db workflowRunDB
}

type workflowRunRows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close()
	Err() error
}

type workflowRunDB interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) workflowRunScanner
	Query(ctx context.Context, sql string, args ...interface{}) (workflowRunRows, error)
}

type pgxWorkflowRunDB struct{ pool *pgxpool.Pool }

var _ workflowRunDB = pgxWorkflowRunDB{}

func (db pgxWorkflowRunDB) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, arguments...)
}
func (db pgxWorkflowRunDB) QueryRow(ctx context.Context, sql string, args ...interface{}) workflowRunScanner {
	return db.pool.QueryRow(ctx, sql, args...)
}
func (db pgxWorkflowRunDB) Query(ctx context.Context, sql string, args ...interface{}) (workflowRunRows, error) {
	return db.pool.Query(ctx, sql, args...)
}

func NewRunRepository(pool *pgxpool.Pool) *RunRepository {
	return newRunRepositoryWithDB(pgxWorkflowRunDB{pool: pool})
}

func newRunRepositoryWithDB(db workflowRunDB) *RunRepository {
	return &RunRepository{db: db}
}

// Create inserts a new WorkflowRun.
func (r *RunRepository) Create(ctx context.Context, run *WorkflowRun) error {
	if err := validatePersistedWorkflowRun(run); err != nil {
		return err
	}
	if run.ID == "" {
		run.ID = "wfr-" + uuid.NewString()[:8]
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO workflow_runs (id, project_id, user_id, template_id, template_version,
		 task_id, status, attempt, input, output, stage_statuses, trace_id, tool_registry_snapshot_id,
		 run_manifest, parent_run_id, replay_from_stage_id, started_at, finished_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		run.ID, run.ProjectID, run.UserID, run.TemplateID, run.TemplateVersion,
		run.TaskID, string(run.Status), run.Attempt, run.Input, run.Output,
		run.StageStatuses, run.TraceID, run.ToolRegistrySnapshotID, run.RunManifest,
		run.ParentRunID, run.ReplayFromStageID, run.StartedAt, run.FinishedAt, run.CreatedAt,
	)
	return err
}

func validatePersistedWorkflowRun(run *WorkflowRun) error {
	if err := database.ValidateRunIdentityColumnBounds(
		run.TraceID,
		run.ToolRegistrySnapshotID,
		run.ParentRunID,
		run.ReplayFromStageID,
	); err != nil {
		return err
	}
	return validateWorkflowRunManifest(run.RunManifest)
}

// FindByID returns a WorkflowRun by ID.
func (r *RunRepository) FindByID(ctx context.Context, id string) (*WorkflowRun, error) {
	return scanWorkflowRun(r.db.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        COALESCE(trace_id, ''), COALESCE(tool_registry_snapshot_id, ''), run_manifest,
		        parent_run_id, replay_from_stage_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE id=$1`, id,
	))
}

// FindByTaskID returns the WorkflowRun associated with the given orchestrator task.
func (r *RunRepository) FindByTaskID(ctx context.Context, taskID string) (*WorkflowRun, error) {
	return scanWorkflowRun(r.db.QueryRow(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        COALESCE(trace_id, ''), COALESCE(tool_registry_snapshot_id, ''), run_manifest,
		        parent_run_id, replay_from_stage_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE task_id=$1`, taskID,
	))
}

// FindByProject returns all runs for a project, newest first.
func (r *RunRepository) FindByProject(ctx context.Context, projectID string) ([]*WorkflowRun, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, project_id, COALESCE(user_id, 'default'), template_id, template_version,
		        task_id, status, attempt, input, output, stage_statuses,
		        COALESCE(trace_id, ''), COALESCE(tool_registry_snapshot_id, ''), run_manifest,
		        parent_run_id, replay_from_stage_id, started_at, finished_at, created_at
		 FROM workflow_runs WHERE project_id=$1 ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*WorkflowRun
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

type workflowRunScanner interface {
	Scan(dest ...interface{}) error
}

func scanWorkflowRun(row workflowRunScanner) (*WorkflowRun, error) {
	var run WorkflowRun
	var runManifestJSON []byte
	if err := row.Scan(
		&run.ID, &run.ProjectID, &run.UserID, &run.TemplateID, &run.TemplateVersion,
		&run.TaskID, &run.Status, &run.Attempt, &run.Input, &run.Output,
		&run.StageStatuses, &run.TraceID, &run.ToolRegistrySnapshotID, &runManifestJSON,
		&run.ParentRunID, &run.ReplayFromStageID, &run.StartedAt, &run.FinishedAt, &run.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(runManifestJSON) > 0 && string(runManifestJSON) != "null" {
		var manifest RunManifest
		if err := json.Unmarshal(runManifestJSON, &manifest); err != nil {
			return nil, err
		}
		run.RunManifest = &manifest
	}
	return &run, nil
}

// UpdateStatus updates the run's status.
func (r *RunRepository) UpdateStatus(ctx context.Context, runID string, status RunStatus) error {
	now := time.Now()
	_, err := r.db.Exec(ctx,
		`UPDATE workflow_runs SET status=$2, started_at=COALESCE(started_at,$3),
		 finished_at=CASE WHEN $2 IN ('COMPLETED','FAILED','CANCELLED') THEN $3 ELSE finished_at END
		 WHERE id=$1`, runID, string(status), now,
	)
	return err
}

// FindRunIDByTaskID returns the run ID associated with a given task ID.
func (r *RunRepository) FindRunIDByTaskID(ctx context.Context, taskID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM workflow_runs WHERE task_id=$1`, taskID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateStageStatus updates a single stage's status in the run's stage_statuses JSONB.
func (r *RunRepository) UpdateStageStatus(ctx context.Context, runID, stageName string, status StageStatus) error {
	_, err := r.db.Exec(ctx,
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
	_, err := r.db.Exec(ctx,
		`INSERT INTO workflow_attempts (id, stage_run_id, number, trigger_type, node_ids, status, error_message, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		attempt.ID, attempt.StageRunID, attempt.Number, attempt.TriggerType,
		attempt.NodeIDs, attempt.Status, attempt.ErrorMessage, attempt.CreatedAt,
	)
	return err
}
