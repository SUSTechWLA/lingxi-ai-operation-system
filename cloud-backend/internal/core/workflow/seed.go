package workflow

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// EnsureSchema creates the workflow_templates table if it doesn't exist.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS workflow_templates (
	    id VARCHAR(64) PRIMARY KEY,
	    version VARCHAR(32) DEFAULT '1.0.0',
	    name VARCHAR(255) NOT NULL,
	    description TEXT,
	    category VARCHAR(128),
	    dag JSONB NOT NULL,
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		zap.L().Error("Failed to create workflow_templates table (non-fatal)", zap.Error(err))
		return
	}
	_, _ = pool.Exec(ctx, `ALTER TABLE workflow_templates ADD COLUMN IF NOT EXISTS version VARCHAR(32) DEFAULT '1.0.0'`)
	SeedBuiltinTemplates(ctx, pool)
}

// SeedBuiltinTemplates inserts the default Guided Video Studio workflow template.
// The legacy one-click workflow and archived workflows (bid, publish, code-review)
// are preserved but not registered by default.
func SeedBuiltinTemplates(ctx context.Context, pool *pgxpool.Pool) {
	templates := []struct {
		ID, Name, Desc, Cat, DAG string
	}{
		GuidedImageTextVideoWorkflow(),
	}

	for _, t := range templates {
		// Validate JSON before inserting
		if !json.Valid([]byte(t.DAG)) {
			zap.L().Warn("Invalid DAG JSON in seed template", zap.String("id", t.ID))
			continue
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO workflow_templates (id, name, description, category, dag)
			 VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`,
			t.ID, t.Name, t.Desc, t.Cat, []byte(t.DAG))
		if err != nil {
			zap.L().Warn("Failed to seed template", zap.String("id", t.ID), zap.Error(err))
		}
	}
	zap.L().Info("Workflow templates seeded", zap.Int("count", len(templates)))
}
