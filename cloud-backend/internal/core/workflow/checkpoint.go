package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// CheckpointState represents the lifecycle state of a workflow checkpoint.
type CheckpointState string

const (
	CheckpointAwaitingHuman CheckpointState = "AWAITING_HUMAN"
	CheckpointInProgress    CheckpointState = "IN_PROGRESS"
	CheckpointCompleted     CheckpointState = "COMPLETED"
)

// Checkpoint records the system state at a stage boundary so that a
// workflow run can be recovered after restart or failure.
type Checkpoint struct {
	ID            string          `json:"id"`
	WorkflowRunID string          `json:"workflowRunId"`
	TaskID        string          `json:"taskId"`
	StageName     string          `json:"stageName"`
	StageIndex    int             `json:"stageIndex"`
	NodeID        string          `json:"nodeId"`
	State         CheckpointState `json:"state"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	RecoveredAt   *time.Time      `json:"recoveredAt,omitempty"`
}

// CheckpointStore persists and queries workflow checkpoints.
type CheckpointStore interface {
	Save(ctx context.Context, cp *Checkpoint) error
	FindLatest(ctx context.Context, workflowRunID string) (*Checkpoint, error)
	FindByStage(ctx context.Context, workflowRunID, stageName string) (*Checkpoint, error)
	ListByRun(ctx context.Context, workflowRunID string) ([]*Checkpoint, error)
	MarkRecovered(ctx context.Context, id string) error
}

// pgxCheckpointStore is the pgxpool-backed CheckpointStore implementation.
type pgxCheckpointStore struct {
	pool *pgxpool.Pool
}

// NewCheckpointStore creates a CheckpointStore backed by *pgxpool.Pool.
func NewCheckpointStore(pool *pgxpool.Pool) CheckpointStore {
	return &pgxCheckpointStore{pool: pool}
}

// EnsureCheckpointSchema creates the workflow_checkpoints table if it does not exist.
func EnsureCheckpointSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS workflow_checkpoints (
		id VARCHAR(64) PRIMARY KEY,
		workflow_run_id VARCHAR(64) NOT NULL,
		task_id VARCHAR(64) NOT NULL,
		stage_name VARCHAR(128) NOT NULL DEFAULT '',
		stage_index INT NOT NULL DEFAULT 0,
		node_id VARCHAR(64) NOT NULL DEFAULT '',
		state VARCHAR(32) NOT NULL DEFAULT 'AWAITING_HUMAN',
		snapshot JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		recovered_at TIMESTAMPTZ
	)`)
	if err != nil {
		return fmt.Errorf("create workflow_checkpoints table: %w", err)
	}
	// Index for common lookup patterns.
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_checkpoints_run ON workflow_checkpoints(workflow_run_id, created_at DESC)`)
	return nil
}

func (s *pgxCheckpointStore) Save(ctx context.Context, cp *Checkpoint) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_checkpoints (id, workflow_run_id, task_id, stage_name, stage_index, node_id, state, snapshot)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, snapshot=EXCLUDED.snapshot`,
		cp.ID, cp.WorkflowRunID, cp.TaskID, cp.StageName, cp.StageIndex, cp.NodeID, string(cp.State), cp.Snapshot)
	if err != nil {
		zap.L().Error("Failed to save checkpoint", zap.String("id", cp.ID), zap.Error(err))
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

func (s *pgxCheckpointStore) FindLatest(ctx context.Context, workflowRunID string) (*Checkpoint, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, workflow_run_id, task_id, stage_name, stage_index, node_id, state, snapshot, created_at, recovered_at
		 FROM workflow_checkpoints WHERE workflow_run_id=$1 ORDER BY created_at DESC LIMIT 1`, workflowRunID)
	return scanCheckpoint(row)
}

func (s *pgxCheckpointStore) FindByStage(ctx context.Context, workflowRunID, stageName string) (*Checkpoint, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, workflow_run_id, task_id, stage_name, stage_index, node_id, state, snapshot, created_at, recovered_at
		 FROM workflow_checkpoints WHERE workflow_run_id=$1 AND stage_name=$2 ORDER BY created_at DESC LIMIT 1`,
		workflowRunID, stageName)
	return scanCheckpoint(row)
}

func (s *pgxCheckpointStore) ListByRun(ctx context.Context, workflowRunID string) ([]*Checkpoint, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_run_id, task_id, stage_name, stage_index, node_id, state, snapshot, created_at, recovered_at
		 FROM workflow_checkpoints WHERE workflow_run_id=$1 ORDER BY created_at ASC`, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var out []*Checkpoint
	for rows.Next() {
		cp, err := scanCheckpointRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, rows.Err()
}

func (s *pgxCheckpointStore) MarkRecovered(ctx context.Context, id string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE workflow_checkpoints SET recovered_at=$1, state=$2 WHERE id=$3`,
		now, string(CheckpointCompleted), id)
	if err != nil {
		return fmt.Errorf("mark checkpoint recovered: %w", err)
	}
	return nil
}

// scanCheckpoint scans a single checkpoint from pgx.Row.
func scanCheckpoint(row pgx.Row) (*Checkpoint, error) {
	var cp Checkpoint
	var recoveredAt *time.Time
	if err := row.Scan(&cp.ID, &cp.WorkflowRunID, &cp.TaskID, &cp.StageName, &cp.StageIndex, &cp.NodeID, &cp.State, &cp.Snapshot, &cp.CreatedAt, &recoveredAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan checkpoint: %w", err)
	}
	cp.RecoveredAt = recoveredAt
	return &cp, nil
}

// scanCheckpointRows scans a checkpoint from pgx.Rows.
func scanCheckpointRows(rows pgx.Rows) (*Checkpoint, error) {
	var cp Checkpoint
	var recoveredAt *time.Time
	if err := rows.Scan(&cp.ID, &cp.WorkflowRunID, &cp.TaskID, &cp.StageName, &cp.StageIndex, &cp.NodeID, &cp.State, &cp.Snapshot, &cp.CreatedAt, &recoveredAt); err != nil {
		return nil, fmt.Errorf("scan checkpoint row: %w", err)
	}
	cp.RecoveredAt = recoveredAt
	return &cp, nil
}
