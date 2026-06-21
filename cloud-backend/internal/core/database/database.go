package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/config"
)

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
		    parameters JSONB DEFAULT '{}',
		    output JSONB DEFAULT '{}',
		    examples JSONB DEFAULT '[]',
		    sandbox BOOLEAN DEFAULT false,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
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
		    stage_name VARCHAR(128) NOT NULL,
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
		    metadata JSONB DEFAULT '{}',
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_artifacts_project_stage ON artifacts(project_id, stage_name, unit_id);
		CREATE INDEX IF NOT EXISTS idx_artifacts_current ON artifacts(project_id, stage_name, unit_id) WHERE is_current = true;

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

	// Workflow Run tables (video creation upgrade P3)
	workflowRunSchema := `
		ALTER TABLE workflow_templates ADD COLUMN IF NOT EXISTS version VARCHAR(32) DEFAULT '1.0.0';

		CREATE TABLE IF NOT EXISTS workflow_runs (
		    id VARCHAR(64) PRIMARY KEY,
		    project_id VARCHAR(64) NOT NULL,
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
	`
	if _, err := pool.Exec(ctx, workflowRunSchema); err != nil {
		zap.L().Warn("Failed to run workflow run migrations (non-fatal)", zap.Error(err))
	}

	// Local Runner tables (video creation upgrade P7)
	localRunnerSchema := `
		CREATE TABLE IF NOT EXISTS local_runners (
		    id VARCHAR(64) PRIMARY KEY,
		    name VARCHAR(128) NOT NULL,
		    status VARCHAR(20) DEFAULT 'ONLINE',
		    last_heartbeat TIMESTAMPTZ DEFAULT NOW(),
		    created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS local_jobs (
		    id VARCHAR(64) PRIMARY KEY,
		    runner_id VARCHAR(64),
		    project_id VARCHAR(64) NOT NULL,
		    command VARCHAR(64) NOT NULL,
		    payload TEXT,
		    status VARCHAR(20) DEFAULT 'PENDING',
		    progress REAL DEFAULT 0,
		    current_step VARCHAR(256) DEFAULT '',
		    output TEXT,
		    error_message TEXT,
		    lease_expires_at TIMESTAMPTZ,
		    created_at TIMESTAMPTZ DEFAULT NOW(),
		    updated_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_local_jobs_status ON local_jobs(status);
	`
	if _, err := pool.Exec(ctx, localRunnerSchema); err != nil {
		zap.L().Warn("Failed to run local runner migrations (non-fatal)", zap.Error(err))
	}

	// Bid (tender) generation tables
	bidSchema := `
	CREATE TABLE IF NOT EXISTS bid_projects (
	    id VARCHAR(64) PRIMARY KEY,
	    user_id VARCHAR(64) DEFAULT 'default',
	    name VARCHAR(255) NOT NULL,
	    status VARCHAR(32) DEFAULT 'DRAFT',
	    task_id VARCHAR(64),
	    template_id VARCHAR(64),
	    industry VARCHAR(128),
	    tender_file_path VARCHAR(512),
	    tender_file_name VARCHAR(255),
	    tender_analysis JSONB,
	    structure JSONB,
	    config JSONB,
	    progress REAL DEFAULT 0,
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_bid_project_task ON bid_projects(task_id);
	CREATE INDEX IF NOT EXISTS idx_bid_project_status ON bid_projects(status);

	CREATE TABLE IF NOT EXISTS bid_chapters (
	    id VARCHAR(64) PRIMARY KEY,
	    project_id VARCHAR(64) NOT NULL,
	    node_id VARCHAR(64),
	    title VARCHAR(255) NOT NULL,
	    content TEXT,
	    status VARCHAR(32) DEFAULT 'PENDING',
	    review_comment TEXT,
	    score_items JSONB,
	    sort_order INT DEFAULT 0,
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_bid_chapter_project ON bid_chapters(project_id);

	CREATE TABLE IF NOT EXISTS bid_templates (
	    id VARCHAR(64) PRIMARY KEY,
	    name VARCHAR(255) NOT NULL,
	    category VARCHAR(128),
	    industry VARCHAR(128),
	    structure JSONB NOT NULL,
	    workflow_dag JSONB,
	    created_at TIMESTAMPTZ DEFAULT NOW()
	);`
	if _, err := pool.Exec(ctx, bidSchema); err != nil {
		zap.L().Error("Failed to run bid migrations (non-fatal)", zap.Error(err))
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
		`ALTER TABLE ai_node DROP CONSTRAINT IF EXISTS ai_node_status_check`,
		`ALTER TABLE ai_node ADD CONSTRAINT ai_node_status_check CHECK (status IN ('CREATED','READY','RUNNING','RETRYING','HEARTBEAT_TIMEOUT','SUCCESS','FAILED','SKIPPED'))`,
		`ALTER TABLE ai_task DROP CONSTRAINT IF EXISTS ai_task_status_check`,
		`ALTER TABLE ai_task ADD CONSTRAINT ai_task_status_check CHECK (status IN ('CREATED','RUNNING','PAUSED','SUCCESS','FAILED'))`,
		`ALTER TABLE ai_context DROP CONSTRAINT IF EXISTS ai_context_context_type_check`,
		`ALTER TABLE ai_context ADD CONSTRAINT ai_context_context_type_check CHECK (context_type IN ('TASK_CREATED','TASK_SUCCESS','TASK_FAILED','NODE_SCHEDULED','NODE_READY','NODE_SUCCESS','NODE_FAILED','NODE_RETRY','NODE_SNAPSHOT','NODE_PROGRESS','NODE_CHECKPOINT','NODE_HEARTBEAT_TIMEOUT','NODE_REVIEW_REQUIRED','NODE_SKIPPED','DAG_SUBMITTED','DAG_VALIDATED','AI_REVISE','AI_CANCELLED'))`,
		`UPDATE ai_context SET created_at = NOW() WHERE created_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_node_heartbeat ON ai_node(heartbeat_at) WHERE long_running = TRUE`,
	}
	for _, stmt := range alterStatements {
		_, _ = pool.Exec(ctx, stmt)
	}

	zap.L().Info("Database migrations completed")
}

func DropAll(ctx context.Context, pool *pgxpool.Pool) {
	drop := fmt.Sprintln(`
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
