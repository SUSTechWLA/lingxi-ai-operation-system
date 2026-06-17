package localrunner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Service manages local runner registration and job lifecycle.
type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// RegisterRunner creates a new local runner.
func (s *Service) RegisterRunner(ctx context.Context, name string) (*LocalRunner, error) {
	runner := &LocalRunner{
		ID:            "lr-" + uuid.NewString()[:8],
		Name:          name,
		Status:        RunnerOnline,
		LastHeartbeat: time.Now(),
		CreatedAt:     time.Now(),
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO local_runners (id, name, status, last_heartbeat, created_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		runner.ID, runner.Name, string(runner.Status), runner.LastHeartbeat, runner.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("register runner: %w", err)
	}
	zap.L().Info("Local runner registered", zap.String("id", runner.ID), zap.String("name", name))
	return runner, nil
}

// Heartbeat updates the runner's last heartbeat timestamp.
func (s *Service) Heartbeat(ctx context.Context, runnerID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE local_runners SET last_heartbeat=$2, status='ONLINE' WHERE id=$1`,
		runnerID, time.Now(),
	)
	return err
}

// CreateJob creates a new local job for a project.
func (s *Service) CreateJob(ctx context.Context, projectID, command, payload string) (*LocalJob, error) {
	if !IsValidCommand(command) {
		return nil, fmt.Errorf("invalid command: %s", command)
	}
	now := time.Now()
	job := &LocalJob{
		ID:        "lj-" + uuid.NewString()[:8],
		ProjectID: projectID,
		Command:   command,
		Payload:   payload,
		Status:    JobPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO local_jobs (id, project_id, command, payload, status, progress, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,0,$6,$7)`,
		job.ID, job.ProjectID, job.Command, job.Payload, string(job.Status), job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return job, nil
}

// ClaimJob claims the oldest pending job for a runner.
func (s *Service) ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error) {
	// Lease expires in 5 minutes
	leaseExpires := time.Now().Add(5 * time.Minute)

	var job LocalJob
	err := s.pool.QueryRow(ctx,
		`UPDATE local_jobs SET status='CLAIMED', runner_id=$2, lease_expires_at=$3, updated_at=NOW()
		 WHERE id = (SELECT id FROM local_jobs WHERE status='PENDING' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		 RETURNING id, runner_id, project_id, command, payload, status, progress, current_step, output, error_message, lease_expires_at, created_at, updated_at`,
		runnerID, leaseExpires,
	).Scan(&job.ID, &job.RunnerID, &job.ProjectID, &job.Command, &job.Payload,
		&job.Status, &job.Progress, &job.CurrentStep, &job.Output, &job.ErrorMessage,
		&job.LeaseExpiresAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("no pending jobs: %w", err)
	}
	job.Status = JobClaimed
	return &job, nil
}

// ReportProgress updates job progress.
func (s *Service) ReportProgress(ctx context.Context, jobID string, progress float64, step string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE local_jobs SET status='RUNNING', progress=$2, current_step=$3, updated_at=NOW() WHERE id=$1`,
		jobID, progress, step,
	)
	return err
}

// CompleteJob marks a job as completed.
func (s *Service) CompleteJob(ctx context.Context, jobID, output string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE local_jobs SET status='COMPLETED', progress=1.0, output=$2, updated_at=NOW() WHERE id=$1`,
		jobID, output,
	)
	return err
}

// FailJob marks a job as failed.
func (s *Service) FailJob(ctx context.Context, jobID, errorMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE local_jobs SET status='FAILED', error_message=$2, updated_at=NOW() WHERE id=$1`,
		jobID, errorMsg,
	)
	return err
}
