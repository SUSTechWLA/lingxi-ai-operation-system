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
	transport, _ := json.Marshal(m.Transport)
	capabilities, _ := json.Marshal(m.Capabilities)
	tags, _ := json.Marshal(m.Tags)
	whenToUse, _ := json.Marshal(m.WhenToUse)
	whenNotToUse, _ := json.Marshal(m.WhenNotToUse)
	approvalPolicy, _ := json.Marshal(m.ApprovalPolicy)
	artifactPolicy, _ := json.Marshal(m.ArtifactPolicy)
	localRequirements, _ := json.Marshal(m.LocalRequirements)
	providerBinding, _ := json.Marshal(m.ProviderBinding)
	providerCapabilities, _ := json.Marshal(m.ProviderCapabilities)
	nextRecommendedTools, _ := json.Marshal(m.NextRecommendedTools)
	failureModes, _ := json.Marshal(m.FailureModes)
	resourceRefs, _ := json.Marshal(m.ResourceRefs)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO tool_manifests (name, description, type, version, endpoint, transport, timeout_ms,
		 parameters, output, examples, sandbox, capabilities, tags, cost_level, latency_level,
		 risk_level, side_effect, idempotent, approval_policy, artifact_policy,
		 execution_plane, requires_user_device, artifact_location, local_command, local_requirements,
		 provider, provider_capabilities, next_recommended_tools, failure_modes, skill_package_id, prompt_ref, resource_refs,
		 boundary, when_to_use, when_not_to_use, provider_binding, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		         $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
		         $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
		         $31, $32, $33, $34, $35, $36, $37, $38)
		 ON CONFLICT (name) DO UPDATE SET
		   description=$2, type=$3, version=$4, endpoint=$5, transport=$6, timeout_ms=$7,
		   parameters=$8, output=$9, examples=$10, sandbox=$11, capabilities=$12,
		   tags=$13, cost_level=$14, latency_level=$15, risk_level=$16,
		   side_effect=$17, idempotent=$18, approval_policy=$19, artifact_policy=$20,
		   execution_plane=$21, requires_user_device=$22, artifact_location=$23,
		   local_command=$24, local_requirements=$25, provider=$26, provider_capabilities=$27,
		   next_recommended_tools=$28, failure_modes=$29, skill_package_id=$30,
		   prompt_ref=$31, resource_refs=$32, boundary=$33, when_to_use=$34,
		   when_not_to_use=$35, provider_binding=$36, updated_at=$38`,
		m.Name, m.Description, m.Type, m.Version, m.Endpoint, transport, m.TimeoutMs,
		params, output, examples, m.Sandbox,
		capabilities, tags, m.CostLevel, m.LatencyLevel, m.RiskLevel,
		m.SideEffect, m.Idempotent, approvalPolicy, artifactPolicy,
		m.ExecutionPlane, m.RequiresUserDevice, m.ArtifactLocation, m.LocalCommand, localRequirements,
		m.Provider, providerCapabilities, nextRecommendedTools, failureModes, m.SkillPackageID, m.PromptRef, resourceRefs,
		m.Boundary, whenToUse, whenNotToUse, providerBinding,
		m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *ToolManifestRepository) FindByName(ctx context.Context, name string) (*model.ToolManifestRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT name, description, type, version, endpoint, transport, timeout_ms,
		        parameters, output, examples, sandbox, capabilities, tags,
		        cost_level, latency_level, risk_level, side_effect, idempotent,
		        approval_policy, artifact_policy, execution_plane, requires_user_device,
		        artifact_location, local_command, local_requirements, provider, provider_capabilities,
		        next_recommended_tools, failure_modes, skill_package_id, prompt_ref, resource_refs,
		        boundary, when_to_use, when_not_to_use, provider_binding,
		        created_at, updated_at
		 FROM tool_manifests WHERE name=$1`, name,
	)

	return scanManifest(row)
}

func (r *ToolManifestRepository) FindAll(ctx context.Context) ([]*model.ToolManifestRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, description, type, version, endpoint, transport, timeout_ms,
		        parameters, output, examples, sandbox, capabilities, tags,
		        cost_level, latency_level, risk_level, side_effect, idempotent,
		        approval_policy, artifact_policy, execution_plane, requires_user_device,
		        artifact_location, local_command, local_requirements, provider, provider_capabilities,
		        next_recommended_tools, failure_modes, skill_package_id, prompt_ref, resource_refs,
		        boundary, when_to_use, when_not_to_use, provider_binding,
		        created_at, updated_at
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
	var params, output, examples, transport []byte
	var capabilities, tags, whenToUse, whenNotToUse, approvalPolicy, artifactPolicy, localRequirements, providerBinding, providerCapabilities []byte
	var nextRecommendedTools, failureModes, resourceRefs []byte
	var endpoint *string
	var version *string
	var costLevel, latencyLevel, riskLevel *string
	var executionPlane, artifactLocation, localCommand, provider, boundary *string
	var skillPackageID, promptRef *string

	if err := row.Scan(
		&m.Name, &m.Description, &m.Type, &version, &endpoint, &transport, &m.TimeoutMs,
		&params, &output, &examples, &m.Sandbox, &capabilities, &tags,
		&costLevel, &latencyLevel, &riskLevel, &m.SideEffect, &m.Idempotent,
		&approvalPolicy, &artifactPolicy, &executionPlane, &m.RequiresUserDevice,
		&artifactLocation, &localCommand, &localRequirements, &provider, &providerCapabilities,
		&nextRecommendedTools, &failureModes, &skillPackageID, &promptRef, &resourceRefs,
		&boundary, &whenToUse, &whenNotToUse, &providerBinding,
		&m.CreatedAt, &m.UpdatedAt,
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
	if len(transport) > 0 {
		m.Transport = transport
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
	if executionPlane != nil {
		m.ExecutionPlane = *executionPlane
	}
	if artifactLocation != nil {
		m.ArtifactLocation = *artifactLocation
	}
	if localCommand != nil {
		m.LocalCommand = *localCommand
	}
	if provider != nil {
		m.Provider = *provider
	}
	if boundary != nil {
		m.Boundary = *boundary
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
	if len(whenToUse) > 0 {
		m.WhenToUse = whenToUse
	}
	if len(whenNotToUse) > 0 {
		m.WhenNotToUse = whenNotToUse
	}
	if len(approvalPolicy) > 0 {
		m.ApprovalPolicy = approvalPolicy
	}
	if len(artifactPolicy) > 0 {
		m.ArtifactPolicy = artifactPolicy
	}
	if len(localRequirements) > 0 {
		m.LocalRequirements = localRequirements
	}
	if len(providerBinding) > 0 {
		m.ProviderBinding = providerBinding
	}
	if len(providerCapabilities) > 0 {
		m.ProviderCapabilities = providerCapabilities
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
