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

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) SaveRun(ctx context.Context, run *Run) error {
	planJSON, _ := json.Marshal(run.Plan)
	budgetJSON, _ := json.Marshal(run.Budget)
	metadataJSON, _ := json.Marshal(run.Metadata)
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}
	run.UpdatedAt = time.Now()

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

func (r *Repository) FindRun(ctx context.Context, id string) (*Run, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, task_id, user_id, domain, message, plan_json, status, budget_json, metadata_json, created_at, updated_at
		 FROM agent_runs WHERE id=$1`, id,
	)

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
