package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for artifacts.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// FindCurrent returns the current version of an artifact for a given scope.
func (r *Repository) FindCurrent(ctx context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	var a Artifact
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, workflow_run_id, stage_name, unit_id, kind, name,
		        version, parent_id, storage_type, storage_ref, inline_json,
		        mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		        is_current, metadata, created_at
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3 AND is_current=true`,
		projectID, stageName, unitID,
	).Scan(
		&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.StageName, &a.UnitID,
		&a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
		&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
		&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
		&a.IsCurrent, &a.Metadata, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("artifact not found: %w", err)
	}
	return &a, nil
}

// FindByID returns a single artifact by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*Artifact, error) {
	var a Artifact
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, workflow_run_id, stage_name, unit_id, kind, name,
		        version, parent_id, storage_type, storage_ref, inline_json,
		        mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		        is_current, metadata, created_at
		 FROM artifacts
		 WHERE id=$1`,
		id,
	).Scan(
		&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.StageName, &a.UnitID,
		&a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
		&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
		&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
		&a.IsCurrent, &a.Metadata, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("artifact not found: %w", err)
	}
	return &a, nil
}

// FindHistory returns all versions of an artifact for a given scope, newest first.
func (r *Repository) FindHistory(ctx context.Context, projectID, stageName, unitID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, workflow_run_id, stage_name, unit_id, kind, name,
		        version, parent_id, storage_type, storage_ref, inline_json,
		        mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		        is_current, metadata, created_at
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3
		 ORDER BY version DESC`,
		projectID, stageName, unitID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.StageName, &a.UnitID,
			&a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
			&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
			&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
			&a.IsCurrent, &a.Metadata, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, &a)
	}
	return artifacts, nil
}

// FindByHash finds an artifact by project+stage+unit+content hash (for idempotency).
func (r *Repository) FindByHash(ctx context.Context, projectID, stageName, unitID, contentHash string) (*Artifact, error) {
	var a Artifact
	err := r.pool.QueryRow(ctx,
		`SELECT id, project_id, workflow_run_id, stage_name, unit_id, kind, name,
		        version, parent_id, storage_type, storage_ref, inline_json,
		        mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		        is_current, metadata, created_at
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3 AND content_hash=$4
		 ORDER BY version DESC LIMIT 1`,
		projectID, stageName, unitID, contentHash,
	).Scan(
		&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.StageName, &a.UnitID,
		&a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
		&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
		&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
		&a.IsCurrent, &a.Metadata, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("artifact not found by hash: %w", err)
	}
	return &a, nil
}

// Save creates a new artifact version. In a transaction:
// 1. Sets all previous current versions to is_current=false
// 2. Inserts the new version with is_current=true
func (r *Repository) Save(ctx context.Context, a *Artifact) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Set previous current versions to false
	_, err = tx.Exec(ctx,
		`UPDATE artifacts SET is_current=false
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3 AND is_current=true`,
		a.ProjectID, a.StageName, a.UnitID,
	)
	if err != nil {
		return fmt.Errorf("failed to unset current versions: %w", err)
	}

	// Generate ID if not set
	if a.ID == "" {
		a.ID = "art-" + uuid.NewString()[:8]
	}
	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO artifacts (id, project_id, workflow_run_id, stage_name, unit_id,
		 kind, name, version, parent_id, storage_type, storage_ref, inline_json,
		 mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		 is_current, metadata, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		a.ID, a.ProjectID, a.WorkflowRunID, a.StageName, a.UnitID,
		string(a.Kind), a.Name, a.Version, a.ParentID, a.StorageType,
		a.StorageRef, a.InlineJSON, a.MimeType, a.SizeBytes,
		a.ContentHash, a.PromptHash, a.Provider, a.Model,
		a.IsCurrent, a.Metadata, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert artifact: %w", err)
	}

	return tx.Commit(ctx)
}

// ListByProject returns all current artifacts for a project.
func (r *Repository) ListByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, workflow_run_id, stage_name, unit_id, kind, name,
		        version, parent_id, storage_type, storage_ref, inline_json,
		        mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		        is_current, metadata, created_at
		 FROM artifacts
		 WHERE project_id=$1 AND is_current=true
		 ORDER BY stage_name, unit_id, version DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.StageName, &a.UnitID,
			&a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
			&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
			&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
			&a.IsCurrent, &a.Metadata, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, &a)
	}
	return artifacts, nil
}

// HashContent computes a SHA256 hash of the given data.
func HashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
