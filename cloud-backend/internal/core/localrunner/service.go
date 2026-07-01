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

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ErrRunnerNotFound is returned when a runner does not exist.
var ErrRunnerNotFound = fmt.Errorf("runner not found")

// ErrRunnerAccessDenied is returned when runner ownership validation fails.
var ErrRunnerAccessDenied = fmt.Errorf("runner access denied")

// ErrJobAccessDenied is returned when job ownership validation fails.
var ErrJobAccessDenied = fmt.Errorf("job access denied")

// ErrJobAlreadyCompleted is returned when attempting to mutate a completed job.
var ErrJobAlreadyCompleted = fmt.Errorf("job already completed")

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
		`WITH upserted AS (
		  INSERT INTO local_jobs
		   (id, project_id, task_id, node_id, tool_name, command, payload, status, progress,
		    timeout_sec, artifact_policy, idempotency_key, created_at, updated_at)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10::jsonb,$11,$12,$13)
		  ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
		  DO UPDATE SET updated_at=local_jobs.updated_at
		  RETURNING *
		 ) `+localJobSelectPrefix()+` FROM upserted`,
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
		`WITH claimed AS (
		 UPDATE local_jobs
		 SET status='CLAIMED', runner_id=$1, lease_expires_at=$2, claimed_at=NOW(), updated_at=NOW()
		 WHERE id = (
		   SELECT lj.id
		   FROM local_jobs lj
		   WHERE lj.status='PENDING'
		     AND EXISTS (
		       SELECT 1
		       FROM local_runners lr
		       CROSS JOIN LATERAL jsonb_array_elements(COALESCE(lr.capabilities, '[]'::jsonb)) cap
		       WHERE lr.id=$1
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
		 ) `+localJobSelectPrefix()+` FROM claimed`,
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

// CompleteJob marks a job as completed. Idempotent: if the job is already
// completed, it returns the existing job without modifying state.
func (s *Service) CompleteJob(ctx context.Context, jobID string, req CompleteJobRequest) (*LocalJob, error) {
	// Idempotency: if already completed, return existing job
	existing, err := s.GetJob(ctx, jobID)
	if err == nil && existing != nil && (existing.Status == JobCompleted || existing.Status == JobFailed) {
		return existing, nil
	}

	if req.Output == nil {
		req.Output = map[string]interface{}{}
	}
	outputJSON, _ := json.Marshal(req.Output)
	row := s.pool.QueryRow(ctx,
		`WITH completed AS (
		  UPDATE local_jobs
		  SET status='COMPLETED', progress=1.0, output=$2, completed_at=NOW(), updated_at=NOW()
		  WHERE id=$1 AND status NOT IN ('COMPLETED', 'FAILED')
		  RETURNING *
		 ) `+localJobSelectPrefix()+` FROM completed`,
		jobID, string(outputJSON),
	)
	return scanJob(row)
}

// FailJob marks a job as failed. Idempotent: if the job is already
// completed or failed, it returns the existing job without modifying state.
func (s *Service) FailJob(ctx context.Context, jobID string, req FailJobRequest) (*LocalJob, error) {
	// Idempotency: if already completed/failed, return existing job
	existing, err := s.GetJob(ctx, jobID)
	if err == nil && existing != nil && (existing.Status == JobCompleted || existing.Status == JobFailed) {
		return existing, nil
	}

	errorMessage := errorMessageFromMap(req.Error)
	errorJSON, _ := json.Marshal(req.Error)
	diagnosticsJSON, _ := json.Marshal(req.Diagnostics)
	row := s.pool.QueryRow(ctx,
		`WITH failed AS (
		  UPDATE local_jobs
		  SET status='FAILED', error_message=$2, error_json=$3::jsonb, diagnostics=$4::jsonb, retryable=$5,
		      completed_at=NOW(), updated_at=NOW()
		  WHERE id=$1 AND status NOT IN ('COMPLETED', 'FAILED')
		  RETURNING *
		 ) `+localJobSelectPrefix()+` FROM failed`,
		jobID, errorMessage, string(errorJSON), string(diagnosticsJSON), req.Retryable,
	)
	return scanJob(row)
}

// GetJob fetches a local job by ID.
func (s *Service) GetJob(ctx context.Context, jobID string) (*LocalJob, error) {
	row := s.pool.QueryRow(ctx,
		localJobSelectPrefix()+" FROM local_jobs WHERE id=$1", jobID,
	)
	return scanJob(row)
}

// ValidateRunnerAccess verifies that a runner exists, belongs to the given user and device,
// has a matching session ID, and is not revoked.
func (s *Service) ValidateRunnerAccess(ctx context.Context, userID, deviceID, runnerID, sessionID string) error {
	var dbUserID, dbDeviceID, dbSessionID, dbStatus string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, device_id, session_id, status FROM local_runners WHERE id=$1`, runnerID,
	).Scan(&dbUserID, &dbDeviceID, &dbSessionID, &dbStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrRunnerNotFound
		}
		return fmt.Errorf("validate runner: %w", err)
	}
	if dbStatus == string(RunnerRevoked) {
		return fmt.Errorf("%w: runner is revoked", ErrRunnerAccessDenied)
	}
	if dbUserID != "" && dbUserID != userID {
		return fmt.Errorf("%w: runner belongs to user %s, not %s", ErrRunnerAccessDenied, dbUserID, userID)
	}
	if deviceID != "" && dbDeviceID != "" && dbDeviceID != deviceID {
		return fmt.Errorf("%w: runner device mismatch", ErrRunnerAccessDenied)
	}
	if sessionID != "" && dbSessionID != "" && dbSessionID != sessionID {
		return fmt.Errorf("%w: runner session mismatch", ErrRunnerAccessDenied)
	}
	return nil
}

// ValidateJobAccess verifies that a job exists, belongs to the given user,
// and is claimed by the given runner (if already claimed).
func (s *Service) ValidateJobAccess(ctx context.Context, userID, runnerID, jobID string) error {
	var dbRunnerID, dbStatus string
	err := s.pool.QueryRow(ctx,
		`SELECT runner_id, status FROM local_jobs WHERE id=$1`, jobID,
	).Scan(&dbRunnerID, &dbStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("job not found: %s", jobID)
		}
		return fmt.Errorf("validate job: %w", err)
	}
	// Once a job is claimed, only the claiming runner can complete/fail it
	if dbRunnerID != "" && dbRunnerID != runnerID {
		return fmt.Errorf("%w: job %s claimed by runner %s, not %s", ErrJobAccessDenied, jobID, dbRunnerID, runnerID)
	}
	// Completed/failed jobs are immutable (idempotent check)
	if dbStatus == string(JobCompleted) || dbStatus == string(JobFailed) {
		return ErrJobAlreadyCompleted
	}
	return nil
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

// ReapExpiredLeases finds jobs stuck in CLAIMED status with expired leases
// and resets them to PENDING so they can be reclaimed. Increments the attempt
// counter on each reaped job. Returns the number of jobs reaped.
func (s *Service) ReapExpiredLeases(ctx context.Context) (int, error) {
	result, err := s.pool.Exec(ctx,
		`UPDATE local_jobs
		 SET status='PENDING', runner_id=NULL, lease_expires_at=NULL,
		     attempt=attempt+1, updated_at=NOW()
		 WHERE status='CLAIMED'
		   AND lease_expires_at IS NOT NULL
		   AND lease_expires_at < NOW()`,
	)
	if err != nil {
		return 0, fmt.Errorf("reap expired leases: %w", err)
	}
	return int(result.RowsAffected()), nil
}

// StartLeaseReaper runs a periodic goroutine that reaps expired job leases.
// It stops when the context is cancelled.
func (s *Service) StartLeaseReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.ReapExpiredLeases(ctx)
				if err != nil {
					zap.L().Warn("lease reaper error", zap.Error(err))
				} else if n > 0 {
					zap.L().Info("lease reaper reset expired jobs",
						zap.Int("count", n))
				}
			}
		}
	}()
}

// HasOnlineRunner checks whether the given user has at least one online
// local runner that can accept jobs.
func (s *Service) HasOnlineRunner(ctx context.Context, userID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM local_runners
		 WHERE user_id=$1 AND status='ONLINE'
		   AND last_heartbeat > NOW() - INTERVAL '90 seconds'`,
		userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check online runner: %w", err)
	}
	return count > 0, nil
}

// SupportsCommand checks whether any online runner for the given user
// has registered the specified local command as available.
func (s *Service) SupportsCommand(ctx context.Context, userID string, command string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM local_runners lr
		 CROSS JOIN LATERAL jsonb_array_elements(COALESCE(lr.capabilities, '[]'::jsonb)) cap
		 WHERE lr.user_id=$1
		   AND lr.status='ONLINE'
		   AND lr.last_heartbeat > NOW() - INTERVAL '90 seconds'
		   AND cap->>'command' = $2
		   AND COALESCE((cap->>'available')::boolean, false) = true`,
		userID, command,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check command support: %w", err)
	}
	return count > 0, nil
}

// SupportsCommandForAnyUser checks whether any online runner (regardless of user)
// has registered the specified local command as available. This is used by the
// render dependency guard to check infrastructure readiness without a user context.
func (s *Service) SupportsCommandForAnyUser(ctx context.Context, command string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM local_runners lr
			 CROSS JOIN LATERAL jsonb_array_elements(COALESCE(lr.capabilities, '[]'::jsonb)) cap
			 WHERE lr.status='ONLINE'
			   AND lr.last_heartbeat > NOW() - INTERVAL '90 seconds'
			   AND cap->>'command' = $1
			   AND COALESCE((cap->>'available')::boolean, false) = true`,
		command,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check command support for any user: %w", err)
	}
	return count > 0, nil
}

// SatisfiesRequirements checks whether the user's online runners satisfy
// the given local requirements. Returns a list of human-readable descriptions
// of unsatisfied requirements.
func (s *Service) SatisfiesRequirements(ctx context.Context, userID string, req *tool.LocalRequirements) (bool, []string, error) {
	if req == nil {
		return true, nil, nil
	}

	var missing []string

	// Check required commands
	for _, cmd := range req.Commands {
		supported, err := s.SupportsCommand(ctx, userID, cmd)
		if err != nil {
			return false, nil, fmt.Errorf("check requirement command %q: %w", cmd, err)
		}
		if !supported {
			missing = append(missing, fmt.Sprintf("命令 %s 在本地环境中不可用", cmd))
		}
	}

	// OS checks
	if len(req.OS) > 0 {
		var count int
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM local_runners
			 WHERE user_id=$1 AND status='ONLINE'
			   AND last_heartbeat > NOW() - INTERVAL '90 seconds'
			   AND platform->>'os' = ANY($2)`,
			userID, req.OS,
		).Scan(&count)
		if err != nil {
			return false, nil, fmt.Errorf("check OS requirement: %w", err)
		}
		if count == 0 {
			missing = append(missing, fmt.Sprintf("需要操作系统 %v，但未找到匹配的在线 runner", req.OS))
		}
	}

	if len(missing) > 0 {
		return false, missing, nil
	}
	return true, nil, nil
}
