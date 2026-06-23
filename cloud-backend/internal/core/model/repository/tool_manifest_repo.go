package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/model"
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
	capabilities, _ := json.Marshal(m.Capabilities)
	tags, _ := json.Marshal(m.Tags)
	approvalPolicy, _ := json.Marshal(m.ApprovalPolicy)
	artifactPolicy, _ := json.Marshal(m.ArtifactPolicy)
	nextRecommendedTools, _ := json.Marshal(m.NextRecommendedTools)
	failureModes, _ := json.Marshal(m.FailureModes)
	resourceRefs, _ := json.Marshal(m.ResourceRefs)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO tool_manifests (name, description, type, version, endpoint, timeout_ms,
		 parameters, output, examples, sandbox, capabilities, tags, cost_level, latency_level,
		 risk_level, side_effect, idempotent, approval_policy, artifact_policy,
		 next_recommended_tools, failure_modes, skill_package_id, prompt_ref, resource_refs,
		 created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		         $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
		         $21, $22, $23, $24, $25, $26)
		 ON CONFLICT (name) DO UPDATE SET
		   description=$2, type=$3, version=$4, endpoint=$5, timeout_ms=$6,
		   parameters=$7, output=$8, examples=$9, sandbox=$10, capabilities=$11,
		   tags=$12, cost_level=$13, latency_level=$14, risk_level=$15,
		   side_effect=$16, idempotent=$17, approval_policy=$18, artifact_policy=$19,
		   next_recommended_tools=$20, failure_modes=$21, skill_package_id=$22,
		   prompt_ref=$23, resource_refs=$24, updated_at=$26`,
		m.Name, m.Description, m.Type, m.Version, m.Endpoint, m.TimeoutMs,
		params, output, examples, m.Sandbox,
		capabilities, tags, m.CostLevel, m.LatencyLevel, m.RiskLevel,
		m.SideEffect, m.Idempotent, approvalPolicy, artifactPolicy,
		nextRecommendedTools, failureModes, m.SkillPackageID, m.PromptRef, resourceRefs,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *ToolManifestRepository) FindByName(ctx context.Context, name string) (*model.ToolManifestRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT name, description, type, version, endpoint, timeout_ms,
		        parameters, output, examples, sandbox, capabilities, tags,
		        cost_level, latency_level, risk_level, side_effect, idempotent,
		        approval_policy, artifact_policy, next_recommended_tools, failure_modes,
		        skill_package_id, prompt_ref, resource_refs, created_at, updated_at
		 FROM tool_manifests WHERE name=$1`, name,
	)

	return scanManifest(row)
}

func (r *ToolManifestRepository) FindAll(ctx context.Context) ([]*model.ToolManifestRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, description, type, version, endpoint, timeout_ms,
		        parameters, output, examples, sandbox, capabilities, tags,
		        cost_level, latency_level, risk_level, side_effect, idempotent,
		        approval_policy, artifact_policy, next_recommended_tools, failure_modes,
		        skill_package_id, prompt_ref, resource_refs, created_at, updated_at
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
	var capabilities, tags, approvalPolicy, artifactPolicy, nextRecommendedTools, failureModes, resourceRefs []byte
	var endpoint *string
	var version *string
	var costLevel, latencyLevel, riskLevel *string
	var skillPackageID, promptRef *string

	if err := row.Scan(
		&m.Name, &m.Description, &m.Type, &version, &endpoint, &m.TimeoutMs,
		&params, &output, &examples, &m.Sandbox, &capabilities, &tags,
		&costLevel, &latencyLevel, &riskLevel, &m.SideEffect, &m.Idempotent,
		&approvalPolicy, &artifactPolicy, &nextRecommendedTools, &failureModes,
		&skillPackageID, &promptRef, &resourceRefs, &m.CreatedAt, &m.UpdatedAt,
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
	if costLevel != nil {
		m.CostLevel = *costLevel
	}
	if latencyLevel != nil {
		m.LatencyLevel = *latencyLevel
	}
	if riskLevel != nil {
		m.RiskLevel = *riskLevel
	}
	if skillPackageID != nil {
		m.SkillPackageID = *skillPackageID
	}
	if promptRef != nil {
		m.PromptRef = *promptRef
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
	if len(capabilities) > 0 {
		m.Capabilities = capabilities
	}
	if len(tags) > 0 {
		m.Tags = tags
	}
	if len(approvalPolicy) > 0 {
		m.ApprovalPolicy = approvalPolicy
	}
	if len(artifactPolicy) > 0 {
		m.ArtifactPolicy = artifactPolicy
	}
	if len(nextRecommendedTools) > 0 {
		m.NextRecommendedTools = nextRecommendedTools
	}
	if len(failureModes) > 0 {
		m.FailureModes = failureModes
	}
	if len(resourceRefs) > 0 {
		m.ResourceRefs = resourceRefs
	}

	return &m, nil
}
