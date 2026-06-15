package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/model"
)

type ToolManifestRepository struct {
	pool *pgxpool.Pool
}

func NewToolManifestRepository(pool *pgxpool.Pool) *ToolManifestRepository {
	return &ToolManifestRepository{pool: pool}
}

func (r *ToolManifestRepository) Upsert(ctx context.Context, m *model.ToolManifestRecord) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	params, _ := json.Marshal(m.Parameters)
	output, _ := json.Marshal(m.Output)
	examples, _ := json.Marshal(m.Examples)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO tool_manifests (name, description, type, version, endpoint, timeout_ms,
		 parameters, output, examples, sandbox, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (name) DO UPDATE SET
		   description=$2, type=$3, version=$4, endpoint=$5, timeout_ms=$6,
		   parameters=$7, output=$8, examples=$9, sandbox=$10, updated_at=$12`,
		m.Name, m.Description, m.Type, m.Version, m.Endpoint, m.TimeoutMs,
		params, output, examples, m.Sandbox,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *ToolManifestRepository) FindByName(ctx context.Context, name string) (*model.ToolManifestRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT name, description, type, version, endpoint, timeout_ms,
		        parameters, output, examples, sandbox, created_at, updated_at
		 FROM tool_manifests WHERE name=$1`, name,
	)

	return scanManifest(row)
}

func (r *ToolManifestRepository) FindAll(ctx context.Context) ([]*model.ToolManifestRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, description, type, version, endpoint, timeout_ms,
		        parameters, output, examples, sandbox, created_at, updated_at
		 FROM tool_manifests ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var manifests []*model.ToolManifestRecord
	for rows.Next() {
		m, err := scanManifest(rows)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

func (r *ToolManifestRepository) Delete(ctx context.Context, name string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tool_manifests WHERE name=$1`, name)
	return err
}

// scanManifest scans a single tool_manifests row into a ToolManifestRecord.
func scanManifest(row pgx.Row) (*model.ToolManifestRecord, error) {
	var m model.ToolManifestRecord
	var params, output, examples []byte
	var endpoint *string
	var version *string

	if err := row.Scan(
		&m.Name, &m.Description, &m.Type, &version, &endpoint, &m.TimeoutMs,
		&params, &output, &examples, &m.Sandbox, &m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if version != nil {
		m.Version = *version
	}
	if endpoint != nil {
		m.Endpoint = *endpoint
	}
	if len(params) > 0 {
		m.Parameters = params
	}
	if len(output) > 0 {
		m.Output = output
	}
	if len(examples) > 0 {
		m.Examples = examples
	}

	return &m, nil
}
