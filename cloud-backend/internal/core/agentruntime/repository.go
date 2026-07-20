package agentruntime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

const ackTerminalEventSQL = `UPDATE agent_runs
	SET terminal_event_delivered_at=NOW(), terminal_event_lease_until=NULL, terminal_event_claim_token=NULL
	WHERE id=$1 AND terminal_event_id=$2 AND terminal_event_claim_token=$3 AND terminal_event_delivered_at IS NULL`

const releaseTerminalEventSQL = `UPDATE agent_runs
	SET terminal_event_lease_until=NULL, terminal_event_claim_token=NULL
	WHERE id=$1 AND terminal_event_id=$2 AND terminal_event_claim_token=$3 AND terminal_event_delivered_at IS NULL`

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateRun(ctx context.Context, run *Run) (bool, error) {
	planJSON, budgetJSON, metadataJSON := marshalRunFields(run)
	result, err := r.pool.Exec(ctx,
		`INSERT INTO agent_runs (id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (id) DO NOTHING`,
		run.ID, run.TaskID, run.UserID, run.Domain, run.Message, planJSON,
		string(run.Status), budgetJSON, metadataJSON, run.CreatedAt, run.UpdatedAt,
	)
	return err == nil && result.RowsAffected() == 1, err
}

func (r *Repository) SaveRun(ctx context.Context, run *Run) error {
	planJSON, budgetJSON, metadataJSON := marshalRunFields(run)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_runs (id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (id) DO UPDATE SET
		   task_id=$2, user_id=$3, domain=$4, message=$5, plan_json=$6,
		   status=$7, budget_json=$8, metadata_json=$9, updated_at=$11`,
		run.ID, run.TaskID, run.UserID, run.Domain, run.Message, planJSON,
		string(run.Status), budgetJSON, metadataJSON, run.CreatedAt, run.UpdatedAt,
	)
	return err
}

func (r *Repository) SaveRunTerminal(ctx context.Context, run *Run, event RunTerminalEvent) error {
	planJSON, budgetJSON, metadataJSON := marshalRunFields(run)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO agent_runs (id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at,
		 terminal_event_json, terminal_event_id, terminal_event_delivered_at, terminal_event_attempts, terminal_event_lease_until, terminal_event_claim_token)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULL,0,NULL,NULL)
		 ON CONFLICT (id) DO UPDATE SET
		   task_id=$2, user_id=$3, domain=$4, message=$5, plan_json=$6,
		   status=$7, budget_json=$8, metadata_json=$9, updated_at=$11,
		   terminal_event_json=$12, terminal_event_id=$13, terminal_event_delivered_at=NULL,
		   terminal_event_attempts=0, terminal_event_lease_until=NULL, terminal_event_claim_token=NULL`,
		run.ID, run.TaskID, run.UserID, run.Domain, run.Message, planJSON,
		string(run.Status), budgetJSON, metadataJSON, run.CreatedAt, run.UpdatedAt, eventJSON, event.EventID,
	)
	return err
}

func (r *Repository) ClaimTerminalEvents(ctx context.Context, limit int, leaseUntil time.Time, claimToken string) ([]TerminalEventDelivery, error) {
	rows, err := r.pool.Query(ctx,
		`WITH candidates AS (
		 SELECT id FROM agent_runs
		 WHERE terminal_event_json IS NOT NULL AND terminal_event_delivered_at IS NULL
		   AND (terminal_event_lease_until IS NULL OR terminal_event_lease_until <= NOW())
		 ORDER BY updated_at ASC
		 FOR UPDATE SKIP LOCKED
		 LIMIT $1
		)
		UPDATE agent_runs AS runs
		SET terminal_event_lease_until=$2, terminal_event_attempts=terminal_event_attempts+1, terminal_event_claim_token=$3
		FROM candidates
		WHERE runs.id=candidates.id
		RETURNING runs.id, runs.terminal_event_id, runs.terminal_event_claim_token, runs.terminal_event_json`, limit, leaseUntil, claimToken,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deliveries := make([]TerminalEventDelivery, 0, limit)
	for rows.Next() {
		var delivery TerminalEventDelivery
		var eventJSON []byte
		if err := rows.Scan(&delivery.RunID, &delivery.EventID, &delivery.ClaimToken, &eventJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(eventJSON, &delivery.Event); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (r *Repository) AckTerminalEvent(ctx context.Context, delivery TerminalEventDelivery) (bool, error) {
	tag, err := r.pool.Exec(ctx, ackTerminalEventSQL, delivery.RunID, delivery.EventID, delivery.ClaimToken)
	return err == nil && tag.RowsAffected() == 1, err
}

func (r *Repository) ReleaseTerminalEvent(ctx context.Context, delivery TerminalEventDelivery) (bool, error) {
	tag, err := r.pool.Exec(ctx, releaseTerminalEventSQL, delivery.RunID, delivery.EventID, delivery.ClaimToken)
	return err == nil && tag.RowsAffected() == 1, err
}

func marshalRunFields(run *Run) ([]byte, []byte, []byte) {
	planJSON, _ := json.Marshal(run.Plan)
	budgetJSON, _ := json.Marshal(run.Budget)
	metadataJSON, _ := json.Marshal(run.Metadata)
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt = time.Now()
	return planJSON, budgetJSON, metadataJSON
}

func (r *Repository) FindRun(ctx context.Context, id string) (*Run, error) {
	return scanRun(r.pool.QueryRow(ctx,
		`SELECT id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at
		 FROM agent_runs WHERE id=$1`, id,
	))
}

func (r *Repository) FindRunByTaskID(ctx context.Context, taskID string) (*Run, error) {
	return scanRun(r.pool.QueryRow(ctx,
		`SELECT id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at
		 FROM agent_runs
		 WHERE task_id=$1
		   AND (SELECT COUNT(*) FROM agent_runs WHERE task_id=$1)=1`, taskID,
	))
}

func scanRun(row pgx.Row) (*Run, error) {
	var run Run
	var planJSON, budgetJSON, metadataJSON []byte
	var taskID, userID, domain *string
	var status string
	if err := row.Scan(&run.ID, &taskID, &userID, &domain, &run.Message, &planJSON,
		&status, &budgetJSON, &metadataJSON, &run.CreatedAt, &run.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if taskID != nil {
		run.TaskID = *taskID
	}
	if userID != nil {
		run.UserID = *userID
	}
	if domain != nil {
		run.Domain = *domain
	}
	run.Status = RunStatus(status)
	if len(planJSON) > 0 {
		var plan AgentPlan
		if err := json.Unmarshal(planJSON, &plan); err == nil {
			run.Plan = &plan
		}
	}
	if len(budgetJSON) > 0 {
		_ = json.Unmarshal(budgetJSON, &run.Budget)
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &run.Metadata)
	}
	return &run, nil
}
