package localrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// RegisterRunner creates a cloud-side session for a local execution runner.
func (s *Service) RegisterRunner(ctx context.Context, req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	now := time.Now()
	runnerID := "runner_" + uuid.NewString()[:8]
	sessionID := "runner_session_" + uuid.NewString()[:12]
	name := req.Platform.Hostname
	if name == "" {
		name = req.DeviceID
	}
	platform, _ := json.Marshal(req.Platform)
	capabilities, _ := json.Marshal(req.Capabilities)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO local_runners
		 (id, device_id, user_id, name, runner_version, platform, workspace_root, capabilities,
		  session_id, status, last_heartbeat, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8::jsonb,$9,$10,$11,$12,$13)`,
		runnerID, req.DeviceID, req.UserID, name, req.RunnerVersion, string(platform),
		req.WorkspaceRoot, string(capabilities), sessionID, string(RunnerOnline), now, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("register runner: %w", err)
	}
	zap.L().Info("Local runner registered", zap.String("id", runnerID), zap.String("deviceId", req.DeviceID))
	return &RegisterRunnerResponse{
		RunnerID:             runnerID,
		SessionID:            sessionID,
		HeartbeatIntervalSec: 15,
		PollIntervalSec:      3,
	}, nil
}

// Heartbeat updates the runner's last heartbeat timestamp.
func (s *Service) Heartbeat(ctx context.Context, runnerID string, req HeartbeatRequest) error {
	status := RunnerOnline
	if req.Status == "offline" {
		status = RunnerOffline
	}
	lastError := ""
	if req.LastError != nil {
		lastError = *req.LastError
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE local_runners
		 SET last_heartbeat=$2, status=$3, running_jobs=$4, disk_free_mb=$5,
		     cpu_load=$6, memory_usage_mb=$7, last_error=$8, updated_at=NOW()
		 WHERE id=$1`,
		runnerID, time.Now(), string(status), req.RunningJobs, req.DiskFreeMb,
		req.CPULoad, req.MemoryUsageMb, lastError,
	)
	return err
}

// CreateJob creates a new local job for a project.
func (s *Service) CreateJob(ctx context.Context, projectID, command, payload string) (*LocalJob, error) {
	var parsed map[string]interface{}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &parsed)
	}
	return s.DispatchLocalJob(ctx, DispatchLocalJobRequest{
		ProjectID: projectID,
		Command:   command,
		Payload:   parsed,
	})
}

func (s *Service) DispatchLocalJob(ctx context.Context, req DispatchLocalJobRequest) (*LocalJob, error) {
	command := NormalizeCommand(req.Command)
	if !IsValidCommand(command) {
		return nil, fmt.Errorf("invalid command: %s", req.Command)
	}
	projectID := req.ProjectID
	if projectID == "" {
		projectID = req.TaskID
	}
	if projectID == "" {
		projectID = "default"
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payloadJSON, _ := json.Marshal(payload)
	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}
	artifactPolicy := req.ArtifactPolicy
	if artifactPolicy.Location == "" {
		artifactPolicy = LocalArtifactPolicy{
			Location:            "local",
			SyncMetadataToCloud: true,
			SyncFileToCloud:     false,
		}
	}
	artifactPolicyJSON, _ := json.Marshal(artifactPolicy)
	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" && req.TaskID != "" && req.NodeID != "" {
		idempotencyKey = req.TaskID + "-" + req.NodeID
	}

	now := time.Now()
	jobID := "local_job_" + uuid.NewString()[:8]
	row := s.pool.QueryRow(ctx,
		localJobSelectPrefix()+`
		 FROM (
		  INSERT INTO local_jobs
		   (id, project_id, task_id, node_id, tool_name, command, payload, status, progress,
		    timeout_sec, artifact_policy, idempotency_key, created_at, updated_at)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10::jsonb,$11,$12,$13)
		  ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
		  DO UPDATE SET updated_at=local_jobs.updated_at
		  RETURNING *
		 ) AS local_jobs`,
		jobID, projectID, req.TaskID, req.NodeID, req.ToolName, command, string(payloadJSON),
		string(JobPending), timeoutSec, string(artifactPolicyJSON), idempotencyKey, now, now,
	)
	job, err := scanJob(row)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return job, nil
}

// ClaimJob claims the oldest pending job for a runner.
func (s *Service) ClaimJob(ctx context.Context, runnerID string) (*LocalJob, error) {
	// Lease expires in 5 minutes
	leaseExpires := time.Now().Add(5 * time.Minute)

	job, err := scanJob(s.pool.QueryRow(ctx,
		localJobSelectPrefix()+`
		 FROM (
		 UPDATE local_jobs
		 SET status='CLAIMED', runner_id=$2, lease_expires_at=$3, claimed_at=NOW(), updated_at=NOW()
		 WHERE id = (
		   SELECT lj.id
		   FROM local_jobs lj
		   WHERE lj.status='PENDING'
		     AND EXISTS (
		       SELECT 1
		       FROM local_runners lr
		       CROSS JOIN LATERAL jsonb_array_elements(COALESCE(lr.capabilities, '[]'::jsonb)) cap
		       WHERE lr.id=$2
		         AND lr.status='ONLINE'
		         AND lr.last_heartbeat > NOW() - INTERVAL '90 seconds'
		         AND cap->>'command' = lj.command
		         AND COALESCE((cap->>'available')::boolean, false) = true
		     )
		   ORDER BY lj.created_at
		   LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING *
		 ) AS local_jobs`,
		runnerID, leaseExpires,
	))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

// ReportProgress updates job progress.
func (s *Service) ReportProgress(ctx context.Context, jobID string, req ProgressRequest) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE local_jobs
		 SET status='RUNNING', progress=$2, current_step=$3, message=$4, updated_at=NOW()
		 WHERE id=$1`,
		jobID, req.Progress, req.Step, req.Message,
	)
	if err != nil {
		return err
	}
	for _, line := range req.Logs {
		if line == "" {
			continue
		}
		_, _ = s.pool.Exec(ctx, `INSERT INTO local_job_logs (job_id, message) VALUES ($1,$2)`, jobID, line)
	}
	return nil
}

// CompleteJob marks a job as completed.
func (s *Service) CompleteJob(ctx context.Context, jobID string, req CompleteJobRequest) (*LocalJob, error) {
	if req.Output == nil {
		req.Output = map[string]interface{}{}
	}
	outputJSON, _ := json.Marshal(req.Output)
	row := s.pool.QueryRow(ctx,
		localJobSelectPrefix()+`
		 FROM (
		  UPDATE local_jobs
		  SET status='COMPLETED', progress=1.0, output=$2, completed_at=NOW(), updated_at=NOW()
		  WHERE id=$1
		  RETURNING *
		 ) AS local_jobs`,
		jobID, string(outputJSON),
	)
	return scanJob(row)
}

// FailJob marks a job as failed.
func (s *Service) FailJob(ctx context.Context, jobID string, req FailJobRequest) (*LocalJob, error) {
	errorMessage := errorMessageFromMap(req.Error)
	errorJSON, _ := json.Marshal(req.Error)
	diagnosticsJSON, _ := json.Marshal(req.Diagnostics)
	row := s.pool.QueryRow(ctx,
		localJobSelectPrefix()+`
		 FROM (
		  UPDATE local_jobs
		  SET status='FAILED', error_message=$2, error_json=$3::jsonb, diagnostics=$4::jsonb, retryable=$5,
		      completed_at=NOW(), updated_at=NOW()
		  WHERE id=$1
		  RETURNING *
		 ) AS local_jobs`,
		jobID, errorMessage, string(errorJSON), string(diagnosticsJSON), req.Retryable,
	)
	return scanJob(row)
}

func localJobSelectPrefix() string {
	return `SELECT id, runner_id, project_id, task_id, node_id, tool_name, command, payload,
	        status, progress, current_step, message, output, error_message, error_json,
	        diagnostics, retryable, timeout_sec, artifact_policy, idempotency_key, attempt,
	        lease_expires_at, created_at, updated_at`
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanJob(row rowScanner) (*LocalJob, error) {
	var job LocalJob
	var runnerID, taskID, nodeID, toolName, payload, currentStep, message, output, errorMessage, idempotencyKey *string
	var status string
	var errorJSON, diagnosticsJSON, artifactPolicyJSON []byte
	err := row.Scan(
		&job.ID, &runnerID, &job.ProjectID, &taskID, &nodeID, &toolName, &job.Command, &payload,
		&status, &job.Progress, &currentStep, &message, &output, &errorMessage, &errorJSON,
		&diagnosticsJSON, &job.Retryable, &job.TimeoutSec, &artifactPolicyJSON, &idempotencyKey,
		&job.Attempt, &job.LeaseExpiresAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	job.Status = JobStatus(status)
	if runnerID != nil {
		job.RunnerID = *runnerID
	}
	if taskID != nil {
		job.TaskID = *taskID
	}
	if nodeID != nil {
		job.NodeID = *nodeID
	}
	if toolName != nil {
		job.ToolName = *toolName
	}
	if currentStep != nil {
		job.CurrentStep = *currentStep
	}
	if message != nil {
		job.Message = *message
	}
	if errorMessage != nil {
		job.ErrorMessage = *errorMessage
	}
	if idempotencyKey != nil {
		job.IdempotencyKey = *idempotencyKey
	}
	job.Payload = decodeJSONStringMap(payload)
	job.Output = decodeJSONStringMap(output)
	if len(errorJSON) > 0 {
		_ = json.Unmarshal(errorJSON, &job.Error)
	}
	if len(diagnosticsJSON) > 0 {
		_ = json.Unmarshal(diagnosticsJSON, &job.Diagnostics)
	}
	if len(artifactPolicyJSON) > 0 {
		_ = json.Unmarshal(artifactPolicyJSON, &job.ArtifactPolicy)
	}
	return &job, nil
}

func decodeJSONStringMap(raw *string) map[string]interface{} {
	result := map[string]interface{}{}
	if raw == nil || *raw == "" {
		return result
	}
	_ = json.Unmarshal([]byte(*raw), &result)
	return result
}

func errorMessageFromMap(m map[string]interface{}) string {
	if msg, ok := m["message"].(string); ok && msg != "" {
		return msg
	}
	if code, ok := m["code"].(string); ok && code != "" {
		return code
	}
	return "local job failed"
}
