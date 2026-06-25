package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// fullSelectColumns is the canonical column list for all artifact SELECT queries.
const fullSelectColumns = `id, project_id, workflow_run_id, task_id, stage_name, role_agent_id,
		unit_id, kind, name, version, parent_id, storage_type, storage_ref, inline_json,
		mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		is_current, status, human_approved, depends_on, produced_by_node,
		produced_by_tool, produced_by_role, metadata, created_at, updated_at`

// scanArtifact scans the full column set into an Artifact struct.
func scanArtifact(scanner interface {
	Scan(dest ...interface{}) error
}) (*Artifact, error) {
	var a Artifact
	var dependsOnJSON []byte
	err := scanner.Scan(
		&a.ID, &a.ProjectID, &a.WorkflowRunID, &a.TaskID, &a.StageName, &a.RoleAgentID,
		&a.UnitID, &a.Kind, &a.Name, &a.Version, &a.ParentID, &a.StorageType,
		&a.StorageRef, &a.InlineJSON, &a.MimeType, &a.SizeBytes,
		&a.ContentHash, &a.PromptHash, &a.Provider, &a.Model,
		&a.IsCurrent, &a.Status, &a.HumanApproved, &dependsOnJSON,
		&a.ProducedByNode, &a.ProducedByTool, &a.ProducedByRole,
		&a.Metadata, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	// Parse depends_on JSONB into []string
	if len(dependsOnJSON) > 0 {
		_ = json.Unmarshal(dependsOnJSON, &a.DependsOn)
	}
	if a.DependsOn == nil {
		a.DependsOn = []string{}
	}
	// Normalize: ensure metadata has the promoted values for backward compat
	return promoteArtifactIndexFields(&a), nil
}

// scanArtifacts scans multiple rows.
func scanArtifacts(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close()
}) ([]*Artifact, error) {
	defer rows.Close()
	var artifacts []*Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, nil
}

// FindCurrent returns the current version of an artifact for a given scope.
func (r *Repository) FindCurrent(ctx context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	return scanArtifact(r.pool.QueryRow(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3 AND is_current=true`,
		projectID, stageName, unitID,
	))
}

// FindByID returns a single artifact by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*Artifact, error) {
	return scanArtifact(r.pool.QueryRow(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE id=$1`,
		id,
	))
}

// FindHistory returns all versions of an artifact for a given scope, newest first.
func (r *Repository) FindHistory(ctx context.Context, projectID, stageName, unitID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3
		 ORDER BY version DESC`,
		projectID, stageName, unitID,
	)
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows)
}

// FindByHash finds an artifact by project+stage+unit+content hash (for idempotency).
func (r *Repository) FindByHash(ctx context.Context, projectID, stageName, unitID, contentHash string) (*Artifact, error) {
	return scanArtifact(r.pool.QueryRow(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND unit_id=$3 AND content_hash=$4
		 ORDER BY version DESC LIMIT 1`,
		projectID, stageName, unitID, contentHash,
	))
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
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = now
	}
	if a.Status == "" {
		a.Status = "valid"
	}
	if a.Metadata == nil {
		a.Metadata = map[string]interface{}{}
	}

	// Marshal depends_on for JSONB
	dependsOnJSON := []byte("[]")
	if len(a.DependsOn) > 0 {
		dependsOnJSON, _ = json.Marshal(a.DependsOn)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO artifacts (id, project_id, workflow_run_id, task_id, stage_name, role_agent_id,
		 unit_id, kind, name, version, parent_id, storage_type, storage_ref, inline_json,
		 mime_type, size_bytes, content_hash, prompt_hash, provider, model,
		 is_current, status, human_approved, depends_on, produced_by_node,
		 produced_by_tool, produced_by_role, metadata, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`,
		a.ID, a.ProjectID, a.WorkflowRunID, a.TaskID, a.StageName, a.RoleAgentID,
		a.UnitID, string(a.Kind), a.Name, a.Version, a.ParentID, a.StorageType,
		a.StorageRef, a.InlineJSON, a.MimeType, a.SizeBytes,
		a.ContentHash, a.PromptHash, a.Provider, a.Model,
		a.IsCurrent, a.Status, a.HumanApproved, dependsOnJSON,
		a.ProducedByNode, a.ProducedByTool, a.ProducedByRole,
		a.Metadata, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert artifact: %w", err)
	}

	return tx.Commit(ctx)
}

// ListByProject returns all current artifacts for a project.
func (r *Repository) ListByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND is_current=true
		 ORDER BY stage_name, unit_id, version DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows)
}

// ListUsableByProject returns current artifacts that are valid (not stale/rejected/failed/deleted).
// Callers that need human-approval checks must inspect Artifact.HumanApproved themselves —
// only stages that require human review (e.g. PREVIEW_SNAPSHOTS) need it.
func (r *Repository) ListUsableByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND is_current=true AND status='valid'
		 ORDER BY stage_name, unit_id, version DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows)
}

// ListStaleByProject returns all stale artifacts for a project.
func (r *Repository) ListStaleByProject(ctx context.Context, projectID string) ([]*Artifact, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND is_current=true AND status='stale'
		 ORDER BY stage_name, unit_id, version DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	return scanArtifacts(rows)
}

// FindCurrentByKind finds the current artifact of a specific kind for a project.
func (r *Repository) FindCurrentByKind(ctx context.Context, projectID, stageName string) (*Artifact, error) {
	return scanArtifact(r.pool.QueryRow(ctx,
		`SELECT `+fullSelectColumns+`
		 FROM artifacts
		 WHERE project_id=$1 AND stage_name=$2 AND is_current=true
		 LIMIT 1`,
		projectID, stageName,
	))
}

// UpdateStatus updates the status of an artifact.
func (r *Repository) UpdateStatus(ctx context.Context, artifactID string, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE artifacts SET status=$1, updated_at=NOW() WHERE id=$2`,
		status, artifactID,
	)
	return err
}

// UpdateHumanApproved sets the human_approved flag on an artifact.
func (r *Repository) UpdateHumanApproved(ctx context.Context, artifactID string, approved bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE artifacts SET human_approved=$1, updated_at=NOW() WHERE id=$2`,
		approved, artifactID,
	)
	return err
}

// MarkStaleByKind marks all current artifacts of the given kinds as stale for a project.
// Returns the list of affected artifact IDs.
func (r *Repository) MarkStaleByKind(ctx context.Context, projectID string, kinds []string, reason string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Mark artifacts stale and reset human_approved
	rows, err := tx.Query(ctx,
		`UPDATE artifacts SET
		 status = 'stale',
		 human_approved = false,
		 metadata = jsonb_set(
		     COALESCE(metadata, '{}'::jsonb),
		     '{staleReason}',
		     to_jsonb($3::text),
		     true
		 ),
		 updated_at = NOW()
		 WHERE project_id = $1
		   AND is_current = true
		   AND stage_name = ANY($2)
		   AND status <> 'stale'
		 RETURNING id`,
		projectID, kinds, reason,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mark artifacts stale: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, tx.Commit(ctx)
}

// HashContent computes a SHA256 hash of the given data.
func HashContent(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
