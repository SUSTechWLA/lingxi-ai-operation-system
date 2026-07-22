package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

type ToolManifestRepository struct {
	pool *pgxpool.Pool
}

var toolManifestColumns = []string{
	"name", "description", "type", "version", "endpoint", "transport", "timeout_ms",
	"input_schema", "output_schema", "parameters", "output", "examples", "sandbox", "capabilities", "tags",
	"cost_level", "latency_level", "risk_level", "side_effect", "idempotent", "approval_policy", "artifact_policy",
	"execution_plane", "requires_user_device", "artifact_location", "local_command", "local_requirements",
	"provider", "provider_capabilities", "next_recommended_tools", "failure_modes", "skill_package_id", "prompt_ref", "resource_refs",
	"boundary", "when_to_use", "when_not_to_use", "provider_binding", "created_at", "updated_at",
}

var toolManifestSelectColumns = strings.Join(toolManifestColumns, ", ")

var toolManifestUpsertSQL = `INSERT INTO tool_manifests (` + toolManifestSelectColumns + `)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	        $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
	        $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
	        $31, $32, $33, $34, $35, $36, $37, $38, $39, $40)
	ON CONFLICT (name) DO UPDATE SET
	  description=$2, type=$3, version=$4, endpoint=$5, transport=$6, timeout_ms=$7,
	  input_schema=$8, output_schema=$9, parameters=$10, output=$11, examples=$12,
	  sandbox=$13, capabilities=$14, tags=$15, cost_level=$16, latency_level=$17,
	  risk_level=$18, side_effect=$19, idempotent=$20, approval_policy=$21,
	  artifact_policy=$22, execution_plane=$23, requires_user_device=$24,
	  artifact_location=$25, local_command=$26, local_requirements=$27, provider=$28,
	  provider_capabilities=$29, next_recommended_tools=$30, failure_modes=$31,
	  skill_package_id=$32, prompt_ref=$33, resource_refs=$34, boundary=$35,
	  when_to_use=$36, when_not_to_use=$37, provider_binding=$38, updated_at=$40`

var toolManifestFindByNameSQL = `SELECT ` + toolManifestSelectColumns + ` FROM tool_manifests WHERE name=$1`
var toolManifestFindAllSQL = `SELECT ` + toolManifestSelectColumns + ` FROM tool_manifests ORDER BY name`

func NewToolManifestRepository(pool *pgxpool.Pool) *ToolManifestRepository {
	return &ToolManifestRepository{pool: pool}
}

func (r *ToolManifestRepository) Upsert(ctx context.Context, m *model.ToolManifestRecord) error {
	if m == nil {
		return fmt.Errorf("tool manifest record is nil")
	}
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	args, err := buildToolManifestUpsertArgs(m)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, toolManifestUpsertSQL, args...)
	return err
}

func buildToolManifestUpsertArgs(m *model.ToolManifestRecord) ([]interface{}, error) {
	if m == nil {
		return nil, fmt.Errorf("tool manifest record is nil")
	}
	inputSchema, err := marshalToolManifestJSON("input_schema", m.InputSchema)
	if err != nil {
		return nil, err
	}
	outputSchema, err := marshalToolManifestJSON("output_schema", m.OutputSchema)
	if err != nil {
		return nil, err
	}
	params, err := marshalToolManifestJSON("parameters", m.Parameters)
	if err != nil {
		return nil, err
	}
	output, err := marshalToolManifestJSON("output", m.Output)
	if err != nil {
		return nil, err
	}
	examples, err := marshalToolManifestJSON("examples", m.Examples)
	if err != nil {
		return nil, err
	}
	transport, err := marshalToolManifestJSON("transport", m.Transport)
	if err != nil {
		return nil, err
	}
	capabilities, err := marshalToolManifestJSON("capabilities", m.Capabilities)
	if err != nil {
		return nil, err
	}
	tags, err := marshalToolManifestJSON("tags", m.Tags)
	if err != nil {
		return nil, err
	}
	whenToUse, err := marshalToolManifestJSON("when_to_use", m.WhenToUse)
	if err != nil {
		return nil, err
	}
	whenNotToUse, err := marshalToolManifestJSON("when_not_to_use", m.WhenNotToUse)
	if err != nil {
		return nil, err
	}
	approvalPolicy, err := marshalToolManifestJSON("approval_policy", m.ApprovalPolicy)
	if err != nil {
		return nil, err
	}
	artifactPolicy, err := marshalToolManifestJSON("artifact_policy", m.ArtifactPolicy)
	if err != nil {
		return nil, err
	}
	localRequirements, err := marshalToolManifestJSON("local_requirements", m.LocalRequirements)
	if err != nil {
		return nil, err
	}
	providerBinding, err := marshalToolManifestJSON("provider_binding", m.ProviderBinding)
	if err != nil {
		return nil, err
	}
	providerCapabilities, err := marshalToolManifestJSON("provider_capabilities", m.ProviderCapabilities)
	if err != nil {
		return nil, err
	}
	nextRecommendedTools, err := marshalToolManifestJSON("next_recommended_tools", m.NextRecommendedTools)
	if err != nil {
		return nil, err
	}
	failureModes, err := marshalToolManifestJSON("failure_modes", m.FailureModes)
	if err != nil {
		return nil, err
	}
	resourceRefs, err := marshalToolManifestJSON("resource_refs", m.ResourceRefs)
	if err != nil {
		return nil, err
	}

	return []interface{}{
		m.Name, m.Description, m.Type, m.Version, m.Endpoint, transport, m.TimeoutMs,
		inputSchema, outputSchema, params, output, examples, m.Sandbox,
		capabilities, tags, m.CostLevel, m.LatencyLevel, m.RiskLevel,
		m.SideEffect, m.Idempotent, approvalPolicy, artifactPolicy,
		m.ExecutionPlane, m.RequiresUserDevice, m.ArtifactLocation, m.LocalCommand, localRequirements,
		m.Provider, providerCapabilities, nextRecommendedTools, failureModes, m.SkillPackageID, m.PromptRef, resourceRefs,
		m.Boundary, whenToUse, whenNotToUse, providerBinding,
		m.CreatedAt, m.UpdatedAt,
	}, nil
}

func marshalToolManifestJSON(field string, value interface{}) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode tool manifest %s: %w", field, err)
	}
	return encoded, nil
}

func (r *ToolManifestRepository) FindByName(ctx context.Context, name string) (*model.ToolManifestRecord, error) {
	row := r.pool.QueryRow(ctx, toolManifestFindByNameSQL, name)

	return scanManifest(row)
}

func (r *ToolManifestRepository) FindAll(ctx context.Context) ([]*model.ToolManifestRecord, error) {
	rows, err := r.pool.Query(ctx, toolManifestFindAllSQL)
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
	var inputSchema, outputSchema, params, output, examples, transport []byte
	var capabilities, tags, whenToUse, whenNotToUse, approvalPolicy, artifactPolicy, localRequirements, providerBinding, providerCapabilities []byte
	var nextRecommendedTools, failureModes, resourceRefs []byte
	var endpoint *string
	var version *string
	var costLevel, latencyLevel, riskLevel *string
	var executionPlane, artifactLocation, localCommand, provider, boundary *string
	var skillPackageID, promptRef *string

	if err := row.Scan(
		&m.Name, &m.Description, &m.Type, &version, &endpoint, &transport, &m.TimeoutMs,
		&inputSchema, &outputSchema, &params, &output, &examples, &m.Sandbox, &capabilities, &tags,
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
	if len(inputSchema) > 0 {
		m.InputSchema = inputSchema
	}
	if len(outputSchema) > 0 {
		m.OutputSchema = outputSchema
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
