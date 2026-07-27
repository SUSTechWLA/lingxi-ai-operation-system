package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/config"
)

const (
	// Run-manifest limits bound durable metadata independently of prompt/tool
	// payload size. Agent and workflow builders share this persistence contract.
	RunManifestLimitExceededCode = "RUN_MANIFEST_LIMIT_EXCEEDED"
	RunManifestMaxBytes          = 32 * 1024
	RunManifestMaxRunnerCatalogs = 128

	// RunManifestMaxIdentifierBytes bounds identifiers inside run_manifest
	// JSON. Persisted top-level columns use their exact DDL widths below.
	RunManifestMaxIdentifierBytes = 256
	RunManifestMaxRunnerIDBytes   = 128
	RunManifestMaxVersionBytes    = 128
	RunManifestMaxHashBytes       = 128

	RunTraceIDMaxBytes                = 128
	RunToolRegistrySnapshotIDMaxBytes = 160
	RunParentRunIDMaxBytes            = 64
	RunReplayFromStageIDMaxBytes      = 128
)

// ValidateRunIdentityColumnBounds enforces the exact VARCHAR widths shared by
// agent_runs and workflow_runs before either repository reaches PostgreSQL.
func ValidateRunIdentityColumnBounds(
	traceID string,
	toolRegistrySnapshotID string,
	parentRunID *string,
	replayFromStageID *string,
) error {
	if err := validateRunIdentityColumn("trace ID", traceID, RunTraceIDMaxBytes); err != nil {
		return err
	}
	if err := validateRunIdentityColumn(
		"tool snapshot ID",
		toolRegistrySnapshotID,
		RunToolRegistrySnapshotIDMaxBytes,
	); err != nil {
		return err
	}
	if parentRunID != nil {
		if err := validateRunIdentityColumn("parent run ID", *parentRunID, RunParentRunIDMaxBytes); err != nil {
			return err
		}
	}
	if replayFromStageID != nil {
		if err := validateRunIdentityColumn("replay stage ID", *replayFromStageID, RunReplayFromStageIDMaxBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateRunIdentityColumn(field, value string, maximum int) error {
	if len(value) > maximum {
		return fmt.Errorf(
			"%s: %s bytes %d exceeds %d",
			RunManifestLimitExceededCode,
			field,
			len(value),
			maximum,
		)
	}
	return nil
}

const localMCPReplanMigrationSQL = `UPDATE local_jobs
SET status='FAILED',
    error_message='MCP_CATALOG_REPLAN_REQUIRED',
    error_json=jsonb_build_object(
      'code', 'MCP_CATALOG_REPLAN_REQUIRED',
      'message', 'Legacy MCP job has no immutable runner catalog binding; create a new plan and dispatch again'
    ),
    diagnostics=jsonb_build_object('migration', 'mcp_catalog_binding_v1'),
    retryable=false,
    runner_id=NULL,
    lease_expires_at=NULL,
    completed_at=NOW(),
    updated_at=NOW()
WHERE command='LOCAL_MCP_TOOL_CALL'
  AND status IN ('PENDING','CLAIMED','RUNNING')
  AND (
    BTRIM(COALESCE(catalog_revision,''))='' OR
    BTRIM(COALESCE(mcp_provider_id,''))='' OR
    BTRIM(COALESCE(mcp_logical_tool_name,''))='' OR
    BTRIM(COALESCE(mcp_remote_tool_name,''))=''
  )`

const localOrphanJobMigrationSQL = `UPDATE local_jobs
SET status='FAILED',
    error_message='TASK_OWNER_UNRESOLVED',
    error_json=jsonb_build_object(
      'code', 'TASK_OWNER_UNRESOLVED',
      'message', 'Job owner could not be resolved from durable task state; dispatch again from an authenticated workflow'
    ),
    retryable=false,
    runner_id=NULL,
    lease_expires_at=NULL,
    completed_at=NOW(),
    updated_at=NOW()
WHERE status IN ('PENDING','CLAIMED','RUNNING')
  AND BTRIM(COALESCE(user_id,''))=''`

const videoProjectConfigRevisionMigration = `
	ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS config_revision BIGINT NOT NULL DEFAULT 0;
	CREATE OR REPLACE FUNCTION enforce_video_project_config_revision_guard() RETURNS trigger AS $$
	DECLARE expected_revision TEXT;
	BEGIN
		IF NEW.config IS DISTINCT FROM OLD.config THEN
			expected_revision := current_setting('app.video_project_expected_revision', true);
			IF expected_revision IS NULL OR expected_revision = '' OR expected_revision::BIGINT <> OLD.config_revision THEN
				RAISE EXCEPTION 'video project config update requires expected revision %', OLD.config_revision;
			END IF;
			IF NEW.config_revision <> OLD.config_revision + 1 THEN
				RAISE EXCEPTION 'video project config revision must advance exactly once';
			END IF;
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;
	DROP TRIGGER IF EXISTS video_project_config_revision_guard ON video_projects;
	CREATE TRIGGER video_project_config_revision_guard
		BEFORE UPDATE OF config ON video_projects
		FOR EACH ROW EXECUTE FUNCTION enforce_video_project_config_revision_guard();
`

const agentTerminalOutboxMigration = `
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_json JSONB;
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_id VARCHAR(96);
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_delivered_at TIMESTAMPTZ;
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_attempts INT NOT NULL DEFAULT 0;
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_lease_until TIMESTAMPTZ;
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS terminal_event_claim_token VARCHAR(96);
	UPDATE agent_runs SET terminal_event_id='legacy_terminal_' || id || '_' || COALESCE(terminal_event_attempts, 0)::text
		WHERE terminal_event_json IS NOT NULL AND terminal_event_id IS NULL;
	CREATE INDEX IF NOT EXISTS idx_agent_runs_terminal_pending
		ON agent_runs(updated_at) WHERE terminal_event_json IS NOT NULL AND terminal_event_delivered_at IS NULL;
`

const localJobCallbackOutboxMigration = `
	ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS result_callback_claim_token VARCHAR(96);
	ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS result_callback_lease_until TIMESTAMPTZ;
	ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS followup_callback_claim_token VARCHAR(96);
	ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS followup_callback_lease_until TIMESTAMPTZ;
`

const runManifestMigration = `
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS trace_id VARCHAR(128);
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS tool_registry_snapshot_id VARCHAR(160);
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS run_manifest JSONB;
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS parent_run_id VARCHAR(64);
	ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS replay_from_stage_id VARCHAR(128);
	ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS trace_id VARCHAR(128);
	ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS tool_registry_snapshot_id VARCHAR(160);
	ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS run_manifest JSONB;
	ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS parent_run_id VARCHAR(64);
	ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS replay_from_stage_id VARCHAR(128);
`

const observabilityRelayMigration = `
	CREATE TABLE IF NOT EXISTS observability_event_outbox (
		event_id VARCHAR(128) PRIMARY KEY,
		user_id VARCHAR(64) NOT NULL,
		run_id VARCHAR(128) NOT NULL,
		trace_id VARCHAR(128) NOT NULL,
		occurred_at TIMESTAMPTZ NOT NULL,
		redacted_payload JSONB NOT NULL,
		ingested_at TIMESTAMPTZ NOT NULL,
		delivered_at TIMESTAMPTZ,
		expires_at TIMESTAMPTZ NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_observability_event_outbox_user
		ON observability_event_outbox(user_id, occurred_at, event_id)
		WHERE delivered_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_observability_event_outbox_correlation
		ON observability_event_outbox(user_id, run_id, trace_id);
	CREATE INDEX IF NOT EXISTS idx_observability_event_outbox_expiry
		ON observability_event_outbox(expires_at);

	CREATE TABLE IF NOT EXISTS observability_run_summaries (
		user_id VARCHAR(64) NOT NULL,
		run_id VARCHAR(128) NOT NULL,
		status VARCHAR(32) NOT NULL,
		duration_ms BIGINT,
		error_fingerprints TEXT[] NOT NULL DEFAULT '{}',
		correlation JSONB NOT NULL,
		versions JSONB NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
		last_event_id VARCHAR(128) NOT NULL,
		CONSTRAINT observability_run_summary_fingerprint_limit
			CHECK (cardinality(error_fingerprints) <= 128),
		PRIMARY KEY (user_id, run_id)
	);
	ALTER TABLE observability_run_summaries
		ADD COLUMN IF NOT EXISTS last_event_id VARCHAR(128) NOT NULL DEFAULT '';
	UPDATE observability_run_summaries AS summaries
	SET error_fingerprints=ARRAY(
		SELECT DISTINCT fingerprint
		FROM unnest(COALESCE(summaries.error_fingerprints, ARRAY[]::TEXT[])) AS fingerprint
		WHERE fingerprint ~ '^[a-f0-9]{64}$'
		ORDER BY fingerprint
		LIMIT 128
	);
	DO $$ BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname='observability_run_summary_fingerprint_limit'
			  AND conrelid = 'observability_run_summaries'::regclass
		) THEN
			ALTER TABLE observability_run_summaries
				ADD CONSTRAINT observability_run_summary_fingerprint_limit
				CHECK (cardinality(error_fingerprints) <= 128) NOT VALID;
		END IF;
	END $$;
	ALTER TABLE observability_run_summaries
		VALIDATE CONSTRAINT observability_run_summary_fingerprint_limit;
	CREATE INDEX IF NOT EXISTS idx_observability_run_summaries_user_updated
		ON observability_run_summaries(user_id, updated_at DESC);
`

type migrationExecer interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

func ensureVideoProjectConfigRevision(ctx context.Context, execer migrationExecer) error {
	if _, err := execer.Exec(ctx, videoProjectConfigRevisionMigration); err != nil {
		return fmt.Errorf("required video project config revision schema: %w", err)
	}
	return nil
}

func ensureRunManifestSchema(ctx context.Context, execer migrationExecer) error {
	if _, err := execer.Exec(ctx, runManifestMigration); err != nil {
		return fmt.Errorf("required run manifest schema: %w", err)
	}
	return nil
}

func ensureObservabilityRelaySchema(ctx context.Context, execer migrationExecer) error {
	if _, err := execer.Exec(ctx, observabilityRelayMigration); err != nil {
		return fmt.Errorf("required observability relay schema: %w", err)
	}
	return nil
}

func NewPool(ctx context.Context, cfg config.PostgresConfig) *pgxpool.Pool {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		zap.L().Fatal("Failed to parse database config", zap.Error(err))
	}

	poolCfg.MinConns = 2
	poolCfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		zap.L().Fatal("Failed to create connection pool", zap.Error(err))
	}

	if err := pool.Ping(ctx); err != nil {
		zap.L().Fatal("Failed to ping database", zap.Error(err))
	}

	zap.L().Info("Database connected", zap.String("host", cfg.Host), zap.String("db", cfg.DB))
	return pool
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
	    id VARCHAR(64) PRIMARY KEY,
	    email VARCHAR(255) NOT NULL UNIQUE,
	    password_hash TEXT NOT NULL,
	    nickname VARCHAR(128),
	    avatar_url TEXT,
	    status VARCHAR(32) NOT NULL DEFAULT 'active',
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW(),
	    last_login_at TIMESTAMPTZ
	);

	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

	CREATE TABLE IF NOT EXISTS refresh_tokens (
	    id VARCHAR(64) PRIMARY KEY,
	    user_id VARCHAR(64) NOT NULL REFERENCES users(id),
	    token_hash TEXT NOT NULL UNIQUE,
	    device_id VARCHAR(128),
	    user_agent TEXT,
	    ip_address VARCHAR(64),
	    expires_at TIMESTAMPTZ NOT NULL,
	    revoked_at TIMESTAMPTZ,
	    replaced_by_token_id VARCHAR(64),
	    created_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);

	CREATE TABLE IF NOT EXISTS devices (
	    id VARCHAR(128) PRIMARY KEY,
	    user_id VARCHAR(64) NOT NULL REFERENCES users(id),
	    device_name VARCHAR(128),
	    device_type VARCHAR(64),
	    platform VARCHAR(64),
	    last_seen_at TIMESTAMPTZ,
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

	CREATE TABLE IF NOT EXISTS ai_task (
	    id VARCHAR(64) PRIMARY KEY,
	    user_id VARCHAR(64),
	    status VARCHAR(20),
	    input JSONB,
	    output JSONB,
	    pause_reason TEXT,
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS ai_node (
	    id VARCHAR(64) PRIMARY KEY,
	    task_id VARCHAR(64),
	    type VARCHAR(20),
	    name VARCHAR(100),
	    status VARCHAR(20),
	    input JSONB,
	    output JSONB,
	    error_message TEXT,
	    condition TEXT,
	    retry_count INT DEFAULT 0,
	    max_retry INT DEFAULT 3,
	    priority INT DEFAULT 5,
	    worker_group VARCHAR(50) DEFAULT 'default',
	    version INT DEFAULT 0,
	    idempotency_key VARCHAR(128),
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_node_task ON ai_node(task_id);
	CREATE INDEX IF NOT EXISTS idx_node_status ON ai_node(status);

	CREATE TABLE IF NOT EXISTS ai_node_dependency (
	    parent_node_id VARCHAR(64),
	    child_node_id VARCHAR(64),
	    PRIMARY KEY (parent_node_id, child_node_id)
	);

	CREATE TABLE IF NOT EXISTS ai_context (
	    id BIGSERIAL PRIMARY KEY,
	    context_type VARCHAR(50),
	    task_id VARCHAR(64),
	    node_id VARCHAR(64),
	    metadata JSONB,
	    message VARCHAR(1000),
	    snapshot_data JSONB,
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_context_task ON ai_context(task_id);
	CREATE INDEX IF NOT EXISTS idx_context_node ON ai_context(node_id);

		CREATE TABLE IF NOT EXISTS outbox (
		    id BIGSERIAL PRIMARY KEY,
		    aggregate_type VARCHAR(50) NOT NULL,
		    aggregate_id VARCHAR(100) NOT NULL,
		    event_type VARCHAR(100) NOT NULL,
		    payload JSONB NOT NULL,
		    retry_count INTEGER NOT NULL DEFAULT 0,
		    last_error TEXT,
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_outbox_created ON outbox(created_at);

		-- Backward-compatible migration for existing outbox tables
		ALTER TABLE outbox ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE outbox ADD COLUMN IF NOT EXISTS last_error TEXT;

		CREATE TABLE IF NOT EXISTS outbox_dlq (
		    id BIGSERIAL PRIMARY KEY,
		    aggregate_type VARCHAR(50) NOT NULL,
		    aggregate_id VARCHAR(100) NOT NULL,
		    event_type VARCHAR(100) NOT NULL,
		    payload JSONB NOT NULL,
		    retry_count INTEGER NOT NULL DEFAULT 0,
		    last_error TEXT,
		    original_id BIGINT,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    dead_at TIMESTAMPTZ DEFAULT NOW()
		);

	CREATE INDEX IF NOT EXISTS idx_outbox_created ON outbox(created_at);

	CREATE TABLE IF NOT EXISTS media_assets (
	    id VARCHAR(64) PRIMARY KEY,
	    user_id VARCHAR(64) NOT NULL DEFAULT 'default',
	    original_name TEXT NOT NULL,
	    mime_type VARCHAR(50),
	    size BIGINT,
	    minio_path TEXT,
	    tags JSONB DEFAULT '[]',
	    embedding_id VARCHAR(64),
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_media_assets_user ON media_assets(user_id);

		CREATE TABLE IF NOT EXISTS tool_manifests (
		    name VARCHAR(255) PRIMARY KEY,
		    description TEXT NOT NULL,
		    type VARCHAR(50) NOT NULL DEFAULT 'builtin',
		    version VARCHAR(50) DEFAULT '1.0',
		    endpoint TEXT,
		    timeout_ms INT DEFAULT 30000,
		    input_schema JSONB DEFAULT '{}',
		    output_schema JSONB DEFAULT '{}',
		    parameters JSONB DEFAULT '{}',
		    output JSONB DEFAULT '{}',
		    examples JSONB DEFAULT '[]',
		    sandbox BOOLEAN DEFAULT false,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS capabilities JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS input_schema JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS output_schema JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS boundary VARCHAR(32);
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS when_to_use JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS when_not_to_use JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS transport JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS cost_level VARCHAR(16) DEFAULT 'low';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS latency_level VARCHAR(16) DEFAULT 'medium';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS risk_level VARCHAR(16) DEFAULT 'low';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS side_effect BOOLEAN DEFAULT false;
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS idempotent BOOLEAN DEFAULT true;
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS approval_policy JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS artifact_policy JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS execution_plane VARCHAR(32) DEFAULT 'cloud';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS requires_user_device BOOLEAN DEFAULT false;
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS artifact_location VARCHAR(32) DEFAULT 'cloud';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS local_command VARCHAR(64);
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS local_requirements JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS provider VARCHAR(128);
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS provider_binding JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS provider_capabilities JSONB DEFAULT '{}';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS next_recommended_tools JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS failure_modes JSONB DEFAULT '[]';
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS skill_package_id VARCHAR(255);
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS prompt_ref TEXT;
		ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS resource_refs JSONB DEFAULT '[]';

		CREATE TABLE IF NOT EXISTS agent_runs (
		    id VARCHAR(64) PRIMARY KEY,
		    task_id VARCHAR(64),
		    user_id VARCHAR(64),
		    domain VARCHAR(128),
		    message TEXT,
		    plan_json JSONB,
		    status VARCHAR(32) NOT NULL,
		    budget_json JSONB DEFAULT '{}',
		    metadata_json JSONB DEFAULT '{}',
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_task ON agent_runs(task_id);
		CREATE INDEX IF NOT EXISTS idx_agent_runs_domain ON agent_runs(domain);

		CREATE TABLE IF NOT EXISTS agent_steps (
		    id VARCHAR(64) PRIMARY KEY,
		    run_id VARCHAR(64) NOT NULL,
		    step_id VARCHAR(128) NOT NULL,
		    tool_name VARCHAR(255),
		    status VARCHAR(32) NOT NULL,
		    input_json JSONB DEFAULT '{}',
		    output_json JSONB DEFAULT '{}',
		    artifact_ids JSONB DEFAULT '[]',
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_agent_steps_run ON agent_steps(run_id);

		CREATE TABLE IF NOT EXISTS skill_capabilities (
		    id VARCHAR(255) PRIMARY KEY,
		    name VARCHAR(255) NOT NULL,
		    version VARCHAR(64) NOT NULL,
		    domain VARCHAR(128),
		    root_path TEXT,
		    manifest_json JSONB,
		    status VARCHAR(32) NOT NULL,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_skill_capabilities_domain ON skill_capabilities(domain);

		CREATE TABLE IF NOT EXISTS artifact_reviews (
		    id VARCHAR(64) PRIMARY KEY,
		    task_id VARCHAR(64) NOT NULL,
		    node_id VARCHAR(64) NOT NULL,
		    artifact_id VARCHAR(64),
		    storage_ref TEXT,
		    status VARCHAR(32) NOT NULL,
		    review_reason TEXT,
		    reviewer_id VARCHAR(64),
		    review_comment TEXT,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    reviewed_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_artifact_reviews_task ON artifact_reviews(task_id);
	`

	_, err := pool.Exec(ctx, schema)
	if err != nil {
		zap.L().Fatal("Failed to run migrations", zap.Error(err))
	}
	// Artifact and Video Project tables (video creation upgrade)
	artifactSchema := `
		CREATE TABLE IF NOT EXISTS artifacts (
		    id VARCHAR(64) PRIMARY KEY,
		    project_id VARCHAR(64) NOT NULL,
		    workflow_run_id VARCHAR(64),
		    task_id TEXT,
		    stage_name VARCHAR(128) NOT NULL,
		    role_agent_id TEXT,
		    unit_id VARCHAR(64) DEFAULT '',
		    kind VARCHAR(32) NOT NULL DEFAULT 'JSON',
		    name VARCHAR(255) NOT NULL,
		    version INT NOT NULL DEFAULT 1,
		    parent_id VARCHAR(64),
		    storage_type VARCHAR(16) NOT NULL DEFAULT 'inline',
		    storage_ref TEXT,
		    inline_json TEXT,
		    mime_type VARCHAR(128),
		    size_bytes BIGINT DEFAULT 0,
		    content_hash VARCHAR(128),
		    prompt_hash VARCHAR(128),
		    provider VARCHAR(128),
		    model VARCHAR(128),
		    is_current BOOLEAN NOT NULL DEFAULT true,
		    status TEXT DEFAULT 'valid',
		    human_approved BOOLEAN DEFAULT false,
		    depends_on JSONB DEFAULT '[]',
		    produced_by_node TEXT,
		    produced_by_tool TEXT,
		    produced_by_role TEXT,
		    metadata JSONB DEFAULT '{}',
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_artifacts_project_stage ON artifacts(project_id, stage_name, unit_id);
		CREATE INDEX IF NOT EXISTS idx_artifacts_current ON artifacts(project_id, stage_name, unit_id) WHERE is_current = true;
		CREATE INDEX IF NOT EXISTS idx_artifacts_project_status ON artifacts(project_id, status);
		CREATE INDEX IF NOT EXISTS idx_artifacts_project_kind_current ON artifacts(project_id, kind, is_current);
		CREATE INDEX IF NOT EXISTS idx_artifacts_project_human_approved ON artifacts(project_id, human_approved);
			CREATE INDEX IF NOT EXISTS idx_artifacts_depends_on_gin ON artifacts USING gin(depends_on);

		CREATE TABLE IF NOT EXISTS video_projects (
		    id VARCHAR(64) PRIMARY KEY,
		    user_id VARCHAR(64) DEFAULT 'default',
		    name VARCHAR(255) NOT NULL,
		    description TEXT,
		    mode VARCHAR(32) NOT NULL,
		    status VARCHAR(32) DEFAULT 'DRAFT',
		    skill_name VARCHAR(128) NOT NULL,
		    skill_version VARCHAR(32) NOT NULL,
		    workflow_name VARCHAR(128) NOT NULL,
		    workflow_version VARCHAR(32) NOT NULL,
		    generation_mode VARCHAR(32) DEFAULT 'provider_api',
		    aspect_ratio VARCHAR(16),
		    target_duration_sec INT,
		    language VARCHAR(16) DEFAULT 'zh-CN',
		    config JSONB DEFAULT '{}',
		    current_run_id VARCHAR(64),
		    local_path_hint TEXT,
		    deleted_at TIMESTAMPTZ,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_video_project_status ON video_projects(status) WHERE deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_video_project_updated ON video_projects(user_id, updated_at DESC) WHERE deleted_at IS NULL;
	`
	if _, err := pool.Exec(ctx, artifactSchema); err != nil {
		zap.L().Warn("Failed to run video creation migrations (non-fatal)", zap.Error(err))
	}
	if err := ensureVideoProjectConfigRevision(ctx, pool); err != nil {
		zap.L().Fatal("Failed to install required video project config revision schema", zap.Error(err))
	}
	if _, err := pool.Exec(ctx, agentTerminalOutboxMigration); err != nil {
		zap.L().Fatal("Failed to install required agent terminal outbox schema", zap.Error(err))
	}

	// Workflow Run tables (video creation upgrade P3)
	// Run ALTER TABLE separately — it fails if the table doesn't exist yet.
	alterTemplates := `ALTER TABLE workflow_templates ADD COLUMN IF NOT EXISTS version VARCHAR(32) DEFAULT '1.0.0';`
	if _, err := pool.Exec(ctx, alterTemplates); err != nil {
		zap.L().Warn("Failed to alter workflow_templates (non-fatal)", zap.Error(err))
	}

	workflowRunSchema := `
		CREATE TABLE IF NOT EXISTS workflow_runs (
		    id VARCHAR(64) PRIMARY KEY,
		    project_id VARCHAR(64) NOT NULL,
		    user_id VARCHAR(64) DEFAULT 'default',
		    template_id VARCHAR(64) NOT NULL,
		    template_version VARCHAR(32) NOT NULL,
		    task_id VARCHAR(64),
		    status VARCHAR(20) DEFAULT 'PENDING',
		    attempt INT DEFAULT 1,
		    input JSONB DEFAULT '{}',
		    output JSONB DEFAULT '{}',
		    stage_statuses JSONB DEFAULT '{}',
		    trace_id VARCHAR(128),
		    started_at TIMESTAMPTZ,
		    finished_at TIMESTAMPTZ,
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_workflow_runs_project ON workflow_runs(project_id);

		CREATE TABLE IF NOT EXISTS workflow_attempts (
		    id VARCHAR(64) PRIMARY KEY,
		    stage_run_id VARCHAR(64) NOT NULL,
		    number INT NOT NULL,
		    trigger_type VARCHAR(20) DEFAULT 'INITIAL',
		    node_ids JSONB DEFAULT '[]',
		    status VARCHAR(20) DEFAULT 'RUNNING',
		    error_message TEXT,
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS model_calls (
		    id VARCHAR(64) PRIMARY KEY,
		    project_id VARCHAR(64),
		    user_id VARCHAR(64) DEFAULT 'default',
		    provider VARCHAR(64) NOT NULL,
		    model VARCHAR(64) NOT NULL,
		    capability VARCHAR(32) NOT NULL,
		    fingerprint VARCHAR(128),
		    prompt_tokens INT DEFAULT 0,
		    output_tokens INT DEFAULT 0,
		    cost_usd NUMERIC(10,6) DEFAULT 0,
		    duration_ms INT DEFAULT 0,
		    status VARCHAR(20) NOT NULL DEFAULT 'SUCCESS',
		    error_message TEXT,
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_model_calls_project ON model_calls(project_id);
		CREATE INDEX IF NOT EXISTS idx_model_calls_fingerprint ON model_calls(fingerprint);
		ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS user_id VARCHAR(64) DEFAULT 'default';
		ALTER TABLE model_calls ADD COLUMN IF NOT EXISTS user_id VARCHAR(64) DEFAULT 'default';
	`
	if _, err := pool.Exec(ctx, workflowRunSchema); err != nil {
		zap.L().Warn("Failed to run workflow run migrations (non-fatal)", zap.Error(err))
	}
	if err := ensureRunManifestSchema(ctx, pool); err != nil {
		zap.L().Fatal("Failed to install required run manifest schema", zap.Error(err))
	}
	if err := ensureObservabilityRelaySchema(ctx, pool); err != nil {
		zap.L().Fatal("Failed to install required observability relay schema", zap.Error(err))
	}

	// Local Runner tables (video creation upgrade P7)
	localRunnerSchema := `
		CREATE TABLE IF NOT EXISTS local_runners (
		    id VARCHAR(64) PRIMARY KEY,
		    device_id VARCHAR(128),
		    user_id VARCHAR(64),
		    name VARCHAR(128) NOT NULL DEFAULT '',
		    runner_version VARCHAR(64) DEFAULT '',
		    platform JSONB DEFAULT '{}',
		    workspace_root TEXT DEFAULT '',
		    capabilities JSONB DEFAULT '[]',
		    session_id VARCHAR(128),
		    status VARCHAR(20) DEFAULT 'ONLINE',
		    last_heartbeat TIMESTAMPTZ DEFAULT NOW(),
		    running_jobs INT DEFAULT 0,
		    disk_free_mb BIGINT DEFAULT 0,
		    cpu_load DOUBLE PRECISION DEFAULT 0,
		    memory_usage_mb BIGINT DEFAULT 0,
		    last_error TEXT,
		    updated_at TIMESTAMPTZ DEFAULT NOW(),
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS device_id VARCHAR(128);
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS user_id VARCHAR(64);
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS runner_version VARCHAR(64) DEFAULT '';
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS platform JSONB DEFAULT '{}';
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS workspace_root TEXT DEFAULT '';
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS capabilities JSONB DEFAULT '[]';
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS session_id VARCHAR(128);
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS running_jobs INT DEFAULT 0;
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS disk_free_mb BIGINT DEFAULT 0;
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS cpu_load DOUBLE PRECISION DEFAULT 0;
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS memory_usage_mb BIGINT DEFAULT 0;
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS last_error TEXT;
		ALTER TABLE local_runners ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW();

		CREATE TABLE IF NOT EXISTS local_jobs (
		    id VARCHAR(64) PRIMARY KEY,
		    runner_id VARCHAR(64),
		    user_id VARCHAR(64),
		    target_runner_id VARCHAR(64),
		    catalog_revision VARCHAR(64),
		    mcp_provider_id VARCHAR(128),
		    mcp_logical_tool_name VARCHAR(128),
		    mcp_remote_tool_name VARCHAR(128),
		    project_id VARCHAR(64) NOT NULL,
		    task_id VARCHAR(64),
		    node_id VARCHAR(64),
		    tool_name VARCHAR(255),
		    command VARCHAR(64) NOT NULL,
		    payload TEXT,
		    status VARCHAR(20) DEFAULT 'PENDING',
		    progress REAL DEFAULT 0,
		    current_step VARCHAR(256) DEFAULT '',
		    message TEXT DEFAULT '',
		    output TEXT,
		    error_message TEXT,
		    error_json JSONB DEFAULT '{}',
		    diagnostics JSONB DEFAULT '{}',
		    retryable BOOLEAN DEFAULT true,
		    timeout_sec INT DEFAULT 1800,
		    artifact_policy JSONB DEFAULT '{}',
		    idempotency_key VARCHAR(128),
		    attempt INT DEFAULT 1,
		    result_callback_state VARCHAR(20) NOT NULL DEFAULT 'PENDING',
		    followup_callback_state VARCHAR(20) NOT NULL DEFAULT 'PENDING',
		    claimed_at TIMESTAMPTZ,
		    lease_expires_at TIMESTAMPTZ,
		    completed_at TIMESTAMPTZ,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS task_id VARCHAR(64);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS user_id VARCHAR(64);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS target_runner_id VARCHAR(64);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS catalog_revision VARCHAR(64);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS mcp_provider_id VARCHAR(128);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS mcp_logical_tool_name VARCHAR(128);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS mcp_remote_tool_name VARCHAR(128);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS node_id VARCHAR(64);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS tool_name VARCHAR(255);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS message TEXT DEFAULT '';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS error_json JSONB DEFAULT '{}';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS diagnostics JSONB DEFAULT '{}';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS retryable BOOLEAN DEFAULT true;
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS timeout_sec INT DEFAULT 1800;
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS artifact_policy JSONB DEFAULT '{}';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(128);
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS attempt INT DEFAULT 1;
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS result_callback_state VARCHAR(20) NOT NULL DEFAULT 'DELIVERED';
		ALTER TABLE local_jobs ALTER COLUMN result_callback_state SET DEFAULT 'PENDING';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS followup_callback_state VARCHAR(20) NOT NULL DEFAULT 'DELIVERED';
		ALTER TABLE local_jobs ALTER COLUMN followup_callback_state SET DEFAULT 'PENDING';
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
		ALTER TABLE local_jobs ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
		UPDATE local_jobs lj
		SET user_id=task.user_id
		FROM ai_task task
		WHERE lj.task_id=task.id
		  AND (lj.user_id IS NULL OR lj.user_id='')
		  AND task.user_id IS NOT NULL
		  AND task.user_id<>'';
		CREATE INDEX IF NOT EXISTS idx_local_jobs_status ON local_jobs(status);
		CREATE INDEX IF NOT EXISTS idx_local_jobs_user_status ON local_jobs(user_id, status);
		CREATE INDEX IF NOT EXISTS idx_local_jobs_target_status ON local_jobs(target_runner_id, status);
		CREATE INDEX IF NOT EXISTS idx_local_jobs_node ON local_jobs(node_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_local_jobs_idempotency ON local_jobs(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

		CREATE TABLE IF NOT EXISTS local_job_logs (
		    id BIGSERIAL PRIMARY KEY,
		    job_id VARCHAR(64) NOT NULL,
		    level VARCHAR(16) DEFAULT 'INFO',
		    message TEXT NOT NULL,
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_local_job_logs_job ON local_job_logs(job_id, created_at);
	`
	if _, err := pool.Exec(ctx, localRunnerSchema); err != nil {
		zap.L().Warn("Failed to run local runner migrations (non-fatal)", zap.Error(err))
	}
	if _, err := pool.Exec(ctx, localJobCallbackOutboxMigration); err != nil {
		zap.L().Fatal("Failed to install required local job callback outbox schema", zap.Error(err))
	}
	if _, err := pool.Exec(ctx, localMCPReplanMigrationSQL); err != nil {
		zap.L().Warn("Failed to quarantine legacy MCP jobs (non-fatal)", zap.Error(err))
	}
	if _, err := pool.Exec(ctx, localOrphanJobMigrationSQL); err != nil {
		zap.L().Warn("Failed to quarantine ownerless local jobs (non-fatal)", zap.Error(err))
	}

	// Migrate: drop legacy bid tables (业务线已移除)
	dropBidTables := `
DROP TABLE IF EXISTS bid_chapters;
DROP TABLE IF EXISTS bid_projects;
DROP TABLE IF EXISTS bid_templates;
`
	if _, err := pool.Exec(ctx, dropBidTables); err != nil {
		zap.L().Warn("Failed to drop legacy bid tables (non-fatal)", zap.Error(err))
	}

	// Add columns that may be missing from older (Java) schema
	alterStatements := []string{
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(128)`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS retry_count INT DEFAULT 0`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS max_retry INT DEFAULT 3`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS priority INT DEFAULT 5`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS worker_group VARCHAR(50) DEFAULT 'default'`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS version INT DEFAULT 0`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS error_message TEXT`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS condition TEXT`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS long_running BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS progress DOUBLE PRECISION DEFAULT 0.0`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS current_step VARCHAR(500) DEFAULT ''`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS heartbeat_timeout_sec INT DEFAULT 300`,
		`ALTER TABLE ai_node ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ`,
		`ALTER TABLE ai_task ADD COLUMN IF NOT EXISTS pause_reason TEXT`,
		`ALTER TABLE ai_context ADD COLUMN IF NOT EXISTS source_module VARCHAR(50)`,
		`ALTER TABLE ai_context ADD COLUMN IF NOT EXISTS source_topic VARCHAR(100)`,
		`ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS metadata_json JSONB DEFAULT '{}'`,
		`ALTER TABLE ai_node DROP CONSTRAINT IF EXISTS ai_node_status_check`,
		`ALTER TABLE ai_node ADD CONSTRAINT ai_node_status_check CHECK (status IN ('CREATED','READY','RUNNING','WAITING_LOCAL','LOCAL_CLAIMED','LOCAL_RUNNING','LOCAL_COMPLETED','LOCAL_FAILED','RETRYING','HEARTBEAT_TIMEOUT','SUCCESS','FAILED','SKIPPED','CANCELLED'))`,
		`ALTER TABLE ai_task DROP CONSTRAINT IF EXISTS ai_task_status_check`,
		`ALTER TABLE ai_task ADD CONSTRAINT ai_task_status_check CHECK (status IN ('CREATED','RUNNING','PAUSED','SUCCESS','FAILED'))`,
		`ALTER TABLE ai_context DROP CONSTRAINT IF EXISTS ai_context_context_type_check`,
		`ALTER TABLE ai_context ADD CONSTRAINT ai_context_context_type_check CHECK (context_type IN ('TASK_CREATED','TASK_SUCCESS','TASK_FAILED','NODE_SCHEDULED','NODE_READY','NODE_SUCCESS','NODE_FAILED','NODE_RETRY','NODE_SNAPSHOT','NODE_PROGRESS','NODE_CHECKPOINT','NODE_HEARTBEAT_TIMEOUT','NODE_REVIEW_REQUIRED','NODE_SKIPPED','DAG_SUBMITTED','DAG_VALIDATED','AI_REVISE','AI_CANCELLED'))`,
		`UPDATE ai_context SET created_at = NOW() WHERE created_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_node_heartbeat ON ai_node(heartbeat_at) WHERE long_running = TRUE`,
		// Artifact v1.0-beta-rc1: promote metadata fields to dedicated columns
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS task_id TEXT`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS role_agent_id TEXT`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'valid'`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS human_approved BOOLEAN DEFAULT false`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS depends_on JSONB DEFAULT '[]'`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS produced_by_node TEXT`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS produced_by_tool TEXT`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS produced_by_role TEXT`,
		`ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW()`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project_status ON artifacts(project_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project_kind_current ON artifacts(project_id, kind, is_current)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project_human_approved ON artifacts(project_id, human_approved)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_depends_on_gin ON artifacts USING gin(depends_on)`,
	}
	for _, stmt := range alterStatements {
		_, _ = pool.Exec(ctx, stmt)
	}

	zap.L().Info("Database migrations completed")
}

func DropAll(ctx context.Context, pool *pgxpool.Pool) {
	drop := fmt.Sprintln(`
	DROP TABLE IF EXISTS local_job_logs;
	DROP TABLE IF EXISTS local_jobs;
	DROP TABLE IF EXISTS local_runners;
	DROP TABLE IF EXISTS observability_run_summaries;
	DROP TABLE IF EXISTS observability_event_outbox;
	DROP TABLE IF EXISTS outbox_dlq;
	DROP TABLE IF EXISTS outbox;
	DROP TABLE IF EXISTS ai_context;
	DROP TABLE IF EXISTS ai_node_dependency;
	DROP TABLE IF EXISTS ai_node;
	DROP TABLE IF EXISTS ai_task;
	`)
	_, err := pool.Exec(ctx, drop)
	if err != nil {
		zap.L().Fatal("Failed to drop tables", zap.Error(err))
	}
	zap.L().Info("All tables dropped")
}
