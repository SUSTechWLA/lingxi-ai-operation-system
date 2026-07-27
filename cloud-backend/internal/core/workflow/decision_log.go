package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DecisionLogRecord captures a single auditable decision during a workflow run.
// Every human approval, rejection, stage edit, and runtime selection is recorded.
type DecisionLogRecord struct {
	ID                string          `json:"id"`
	WorkflowRunID     string          `json:"workflowRunId"`
	TaskID            string          `json:"taskId"`
	StageName         string          `json:"stageName"`
	DecisionType      string          `json:"decisionType"`
	Selected          string          `json:"selected"`
	OptionsConsidered json.RawMessage `json:"optionsConsidered,omitempty"`
	ApprovedByUser    bool            `json:"approvedByUser"`
	ReviewerID        string          `json:"reviewerId,omitempty"`
	Comment           string          `json:"comment,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// Decision type constants.
const (
	DecisionStageApproval       = "stage_approval"
	DecisionStageRejection      = "stage_rejection"
	DecisionStageEdit           = "stage_edited"
	DecisionStageRegeneration   = "stage_regenerated"
	DecisionRenderRuntimeSelect = "render_runtime_selection"
	DecisionPipelineSelection   = "pipeline_selection"
	DecisionProposalSelection   = "proposal_selection"
	DecisionPreviewApproval     = "preview_approval"
	DecisionFinalRenderApproval = "final_render_approval"
)

// DecisionLogStore persists decision log records.
type DecisionLogStore interface {
	Save(ctx context.Context, d *DecisionLogRecord) error
	FindByRun(ctx context.Context, workflowRunID string) ([]*DecisionLogRecord, error)
	FindByStage(ctx context.Context, workflowRunID, stageName string) ([]*DecisionLogRecord, error)
}

type DecisionWorkflowRunResolver interface {
	FindRunIDByTaskID(ctx context.Context, taskID string) (string, error)
}

// SaveSimple implements a simplified save that accepts id, taskID, stageName,
// decisionType, selected, approved, reviewerID, and comment. It is suitable for
// use as an agentruntime.DecisionLogWriter adapter.
func (s *pgxDecisionLogStore) SaveSimple(ctx context.Context, taskID, stageName, decisionType, selected, reviewerID, comment string, approved bool) error {
	workflowRunID, err := s.resolveWorkflowRunID(ctx, taskID)
	if err != nil {
		return err
	}
	return s.Save(ctx, &DecisionLogRecord{
		TaskID:         taskID,
		StageName:      stageName,
		DecisionType:   decisionType,
		Selected:       selected,
		ApprovedByUser: approved,
		ReviewerID:     reviewerID,
		Comment:        comment,
		WorkflowRunID:  workflowRunID,
	})
}

func (s *pgxDecisionLogStore) resolveWorkflowRunID(ctx context.Context, taskID string) (string, error) {
	if s == nil || s.resolver == nil {
		return "", fmt.Errorf("workflowRunId resolver is required")
	}
	workflowRunID, err := s.resolver.FindRunIDByTaskID(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("resolve workflowRunId: %w", err)
	}
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return "", fmt.Errorf("workflowRunId is required")
	}
	return workflowRunID, nil
}

type pgxDecisionLogStore struct {
	pool     *pgxpool.Pool
	resolver DecisionWorkflowRunResolver
}

// NewDecisionLogStore creates a DecisionLogStore backed by pgxpool.Pool.
func NewDecisionLogStore(pool *pgxpool.Pool) DecisionLogStore {
	return &pgxDecisionLogStore{pool: pool}
}

func NewDecisionLogStoreWithResolver(pool *pgxpool.Pool, resolver DecisionWorkflowRunResolver) DecisionLogStore {
	return &pgxDecisionLogStore{pool: pool, resolver: resolver}
}

// EnsureDecisionLogSchema creates the decision_logs table if it does not exist.
func EnsureDecisionLogSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS decision_logs (
		id VARCHAR(64) PRIMARY KEY,
		workflow_run_id VARCHAR(64) NOT NULL,
		task_id VARCHAR(64) NOT NULL DEFAULT '',
		stage_name VARCHAR(128) NOT NULL DEFAULT '',
		decision_type VARCHAR(64) NOT NULL,
		selected VARCHAR(512) NOT NULL DEFAULT '',
		options_considered JSONB,
		approved_by_user BOOLEAN NOT NULL DEFAULT false,
		reviewer_id VARCHAR(128) DEFAULT '',
		comment TEXT DEFAULT '',
		created_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("create decision_logs table: %w", err)
	}
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_decision_logs_run ON decision_logs(workflow_run_id, created_at DESC)`)
	return nil
}

func (s *pgxDecisionLogStore) Save(ctx context.Context, d *DecisionLogRecord) error {
	if d == nil {
		return fmt.Errorf("decision log record is required")
	}
	if d.WorkflowRunID == "" {
		return fmt.Errorf("workflowRunId is required")
	}
	if d.ID == "" {
		d.ID = "dl-" + uuid.NewString()[:8]
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO decision_logs (id, workflow_run_id, task_id, stage_name, decision_type, selected, options_considered, approved_by_user, reviewer_id, comment)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		d.ID, d.WorkflowRunID, d.TaskID, d.StageName, d.DecisionType, d.Selected,
		d.OptionsConsidered, d.ApprovedByUser, d.ReviewerID, d.Comment,
	)
	if err != nil {
		zap.L().Error("Failed to save decision log", zap.String("id", d.ID), zap.Error(err))
		return fmt.Errorf("save decision log: %w", err)
	}
	return nil
}

func (s *pgxDecisionLogStore) FindByRun(ctx context.Context, workflowRunID string) ([]*DecisionLogRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_run_id, task_id, stage_name, decision_type, selected, options_considered, approved_by_user, reviewer_id, comment, created_at
		 FROM decision_logs WHERE workflow_run_id=$1 ORDER BY created_at ASC`, workflowRunID)
	if err != nil {
		return nil, fmt.Errorf("find decision logs by run: %w", err)
	}
	defer rows.Close()
	return scanDecisionLogs(rows)
}

func (s *pgxDecisionLogStore) FindByStage(ctx context.Context, workflowRunID, stageName string) ([]*DecisionLogRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_run_id, task_id, stage_name, decision_type, selected, options_considered, approved_by_user, reviewer_id, comment, created_at
		 FROM decision_logs WHERE workflow_run_id=$1 AND stage_name=$2 ORDER BY created_at ASC`, workflowRunID, stageName)
	if err != nil {
		return nil, fmt.Errorf("find decision logs by stage: %w", err)
	}
	defer rows.Close()
	return scanDecisionLogs(rows)
}

func scanDecisionLogs(rows pgx.Rows) ([]*DecisionLogRecord, error) {
	var out []*DecisionLogRecord
	for rows.Next() {
		var d DecisionLogRecord
		if err := rows.Scan(&d.ID, &d.WorkflowRunID, &d.TaskID, &d.StageName, &d.DecisionType,
			&d.Selected, &d.OptionsConsidered, &d.ApprovedByUser, &d.ReviewerID, &d.Comment, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan decision log: %w", err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}
