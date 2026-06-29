package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
)

func newAgentRuntimeArtifactReviewStore(pool *pgxpool.Pool) agentruntime.ArtifactReviewStore {
	if pool == nil {
		return nil
	}
	return agentruntime.NewPGXArtifactReviewStore(pool)
}
