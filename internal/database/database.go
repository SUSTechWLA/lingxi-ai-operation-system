package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
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
	    created_at TIMESTAMPTZ DEFAULT NOW()
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
	`

	_, err := pool.Exec(ctx, schema)
	if err != nil {
		zap.L().Fatal("Failed to run migrations", zap.Error(err))
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
		`ALTER TABLE ai_task ADD COLUMN IF NOT EXISTS pause_reason TEXT`,
		`ALTER TABLE ai_context ADD COLUMN IF NOT EXISTS source_module VARCHAR(50)`,
		`ALTER TABLE ai_context ADD COLUMN IF NOT EXISTS source_topic VARCHAR(100)`,
		`ALTER TABLE ai_node DROP CONSTRAINT IF EXISTS ai_node_status_check`,
		`ALTER TABLE ai_node ADD CONSTRAINT ai_node_status_check CHECK (status IN ('CREATED','READY','RUNNING','RETRYING','SUCCESS','FAILED','SKIPPED'))`,
		`ALTER TABLE ai_task DROP CONSTRAINT IF EXISTS ai_task_status_check`,
		`ALTER TABLE ai_task ADD CONSTRAINT ai_task_status_check CHECK (status IN ('CREATED','RUNNING','PAUSED','SUCCESS','FAILED'))`,
		`UPDATE ai_context SET created_at = NOW() WHERE created_at IS NULL`,
	}
	for _, stmt := range alterStatements {
		_, _ = pool.Exec(ctx, stmt)
	}

	zap.L().Info("Database migrations completed")
}

func DropAll(ctx context.Context, pool *pgxpool.Pool) {
	drop := fmt.Sprintln(`
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
