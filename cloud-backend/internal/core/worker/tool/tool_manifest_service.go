package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/model/repository"
)

const (
	toolCacheKey     = "tools:manifests:all"
	toolCacheTTL     = 5 * time.Minute
	toolCacheVersion = "tools:version" // version key for cache invalidation
)

// ToolManifestService manages tool manifests with DB persistence and Redis caching.
// All tool queries go through this service to ensure fast access and strong consistency.
type ToolManifestService struct {
	repo     repository.ToolManifestRepo
	rdb      toolManifestCache
	registry *ToolRegistry
}

type toolManifestCache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

func NewToolManifestService(repo repository.ToolManifestRepo, rdb *redis.Client, registry *ToolRegistry) *ToolManifestService {
	return &ToolManifestService{
		repo:     repo,
		rdb:      rdb,
		registry: registry,
	}
}

// SyncBuiltinTools upserts all currently registered builtin tools into the DB.
// Called once on startup. Idempotent — uses ON CONFLICT DO UPDATE.
func (s *ToolManifestService) SyncBuiltinTools(ctx context.Context) error {
	manifests := s.registry.ListManifests()
	synced := 0
	var syncErrors []error
	for _, m := range manifests {
		record, err := manifestToRecord(m)
		if err != nil {
			wrapped := fmt.Errorf("encode tool %q: %w", m.Name, err)
			zap.L().Error("Failed to encode builtin tool", zap.String("name", m.Name), zap.Error(err))
			syncErrors = append(syncErrors, wrapped)
			continue
		}
		if err := s.repo.Upsert(ctx, record); err != nil {
			zap.L().Error("Failed to sync builtin tool", zap.String("name", m.Name), zap.Error(err))
			syncErrors = append(syncErrors, fmt.Errorf("persist tool %q: %w", m.Name, err))
			continue
		}
		synced++
	}
	zap.L().Info("Synced builtin tools to database", zap.Int("count", synced), zap.Int("failed", len(syncErrors)))

	// Invalidate cache after sync
	if err := s.invalidateCache(ctx); err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("invalidate tool manifest cache: %w", err))
	}
	if len(syncErrors) > 0 {
		return fmt.Errorf("sync builtin tools: %w", errors.Join(syncErrors...))
	}
	return nil
}

// RegisterExternal persists an external tool to DB and registry, then invalidates cache.
func (s *ToolManifestService) RegisterExternal(ctx context.Context, manifest *ToolManifest) error {
	record, err := manifestToRecord(manifest)
	if err != nil {
		return fmt.Errorf("failed to encode external tool manifest: %w", err)
	}
	record.Type = "external"
	if err := s.repo.Upsert(ctx, record); err != nil {
		return fmt.Errorf("failed to persist external tool: %w", err)
	}
	s.registry.RegisterExternal(manifest)

	zap.L().Info("Registered external tool", zap.String("name", manifest.Name))
	return s.invalidateCache(ctx)
}

// RegisterManifest persists a manifest and makes it discoverable through the
// external bridge without rewriting its declared type. Skill capability prompt
// tools use this path because their type is meaningful to the agent planner.
func (s *ToolManifestService) RegisterManifest(ctx context.Context, manifest *ToolManifest) error {
	record, err := manifestToRecord(manifest)
	if err != nil {
		return fmt.Errorf("failed to encode tool manifest: %w", err)
	}
	if err := s.repo.Upsert(ctx, record); err != nil {
		return fmt.Errorf("failed to persist tool manifest: %w", err)
	}
	s.registry.RegisterExternal(manifest)

	zap.L().Info("Registered tool manifest", zap.String("name", manifest.Name), zap.String("type", manifest.Type))
	return s.invalidateCache(ctx)
}

// DeregisterExternal removes an external tool from DB and registry, then invalidates cache.
func (s *ToolManifestService) DeregisterExternal(ctx context.Context, name string) error {
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("failed to delete tool from DB: %w", err)
	}
	s.registry.DeregisterExternal(name)

	zap.L().Info("Deregistered external tool", zap.String("name", name))
	return s.invalidateCache(ctx)
}

// ListAll returns all tool manifests, using Redis cache for speed.
// Cache miss → query DB → populate cache → return.
func (s *ToolManifestService) ListAll(ctx context.Context) ([]*model.ToolManifestRecord, error) {
	// 1. Try Redis cache
	cached, err := s.rdb.Get(ctx, toolCacheKey).Bytes()
	if err == nil && len(cached) > 0 {
		var manifests []*model.ToolManifestRecord
		if err := json.Unmarshal(cached, &manifests); err == nil {
			if err := normalizeToolManifestRecords(manifests); err == nil {
				return manifests, nil
			} else {
				zap.L().Warn("Tool manifest cache contains invalid records, falling back to DB", zap.Error(err))
			}
		} else {
			zap.L().Warn("Tool manifest cache corrupt, falling back to DB", zap.Error(err))
		}
	}

	// 2. Query DB
	manifests, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query tool manifests: %w", err)
	}
	if err := normalizeToolManifestRecords(manifests); err != nil {
		return nil, fmt.Errorf("normalize persisted tool manifests: %w", err)
	}

	// 3. Populate cache
	data, err := json.Marshal(manifests)
	if err != nil {
		return nil, fmt.Errorf("encode tool manifests for cache: %w", err)
	}
	if err := s.rdb.Set(ctx, toolCacheKey, data, toolCacheTTL).Err(); err != nil {
		zap.L().Warn("Failed to cache tool manifests", zap.Error(err))
	}

	return manifests, nil
}

func normalizeToolManifestRecords(records []*model.ToolManifestRecord) error {
	for i, record := range records {
		if record == nil {
			return fmt.Errorf("tool manifest record %d is nil", i)
		}
		manifest, err := manifestFromRecord(record)
		if err != nil {
			return fmt.Errorf("restore tool %q: %w", record.Name, err)
		}
		inputSchema, err := marshalCanonicalSchema(manifest.InputSchema, "input_schema")
		if err != nil {
			return fmt.Errorf("normalize tool %q: %w", record.Name, err)
		}
		outputSchema, err := marshalCanonicalSchema(manifest.OutputSchema, "output_schema")
		if err != nil {
			return fmt.Errorf("normalize tool %q: %w", record.Name, err)
		}
		record.InputSchema = inputSchema
		record.OutputSchema = outputSchema
	}
	return nil
}

// FormatForPrompt returns a string description of all available tools suitable for
// inclusion in the LLM DAG generation prompt.
func (s *ToolManifestService) FormatForPrompt(ctx context.Context) (string, error) {
	manifests, err := s.ListAll(ctx)
	if err != nil {
		return "", err
	}

	var desc string
	for _, m := range manifests {
		desc += fmt.Sprintf("- %s: %s\n", m.Name, m.Description)

		// Parse and include parameter info
		if len(m.Parameters) > 0 {
			var params map[string]ParamDef
			if err := json.Unmarshal(m.Parameters, &params); err == nil && len(params) > 0 {
				desc += "  参数: "
				for name, param := range params {
					req := ""
					if param.Required {
						req = " (必填)"
					}
					desc += fmt.Sprintf("%s(%s%s) ", name, param.Type, req)
				}
				desc += "\n"
			}
		}

		// Include output schema
		if len(m.Output) > 0 {
			var output map[string]ParamDef
			if err := json.Unmarshal(m.Output, &output); err == nil && len(output) > 0 {
				desc += "  输出: "
				for name, param := range output {
					desc += fmt.Sprintf("%s(%s) ", name, param.Type)
				}
				desc += "\n"
			}
		}
	}

	if desc == "" {
		desc = "llm_api: 视频创作大模型调用，可执行任意文本生成任务"
	}
	return desc, nil
}

func (s *ToolManifestService) invalidateCache(ctx context.Context) error {
	return s.rdb.Del(ctx, toolCacheKey).Err()
}

// manifestToRecord converts a ToolManifest to a model.ToolManifestRecord for DB storage.
func manifestToRecord(m *ToolManifest) (*model.ToolManifestRecord, error) {
	if m == nil {
		return nil, fmt.Errorf("tool manifest is nil")
	}
	costLevel := m.CostLevel
	if costLevel == "" {
		costLevel = CostLow
	}
	latencyLevel := m.LatencyLevel
	if latencyLevel == "" {
		latencyLevel = LatencyMedium
	}
	riskLevel := m.RiskLevel
	if riskLevel == "" {
		riskLevel = RiskLow
	}
	executionPlane := m.ExecutionPlane
	if executionPlane == "" {
		executionPlane = ExecutionPlaneCloud
	}
	artifactLocation := m.ArtifactLocation
	if artifactLocation == "" {
		artifactLocation = ArtifactLocationLocal
	}
	artifactPolicyValue := m.ArtifactPolicy
	if artifactPolicyValue.Storage == "" {
		artifactPolicyValue.Storage = artifactLocation
	}
	if artifactPolicyValue.Storage == ArtifactLocationLocal {
		artifactPolicyValue.SyncFileToCloud = false
	}

	inputSchema, err := marshalCanonicalSchema(m.InputSchema, "input_schema")
	if err != nil {
		return nil, err
	}
	outputSchema, err := marshalCanonicalSchema(m.OutputSchema, "output_schema")
	if err != nil {
		return nil, err
	}
	params, err := marshalManifestJSON("parameters", m.Parameters)
	if err != nil {
		return nil, err
	}
	output, err := marshalManifestJSON("output", m.Output)
	if err != nil {
		return nil, err
	}
	examples, err := marshalManifestJSON("examples", m.Examples)
	if err != nil {
		return nil, err
	}
	transport, err := marshalManifestJSON("transport", m.Transport)
	if err != nil {
		return nil, err
	}
	capabilities, err := marshalManifestJSON("capabilities", m.Capabilities)
	if err != nil {
		return nil, err
	}
	tags, err := marshalManifestJSON("tags", m.Tags)
	if err != nil {
		return nil, err
	}
	whenToUse, err := marshalManifestJSON("when_to_use", m.WhenToUse)
	if err != nil {
		return nil, err
	}
	whenNotToUse, err := marshalManifestJSON("when_not_to_use", m.WhenNotToUse)
	if err != nil {
		return nil, err
	}
	approvalPolicy, err := marshalManifestJSON("approval_policy", m.ApprovalPolicy)
	if err != nil {
		return nil, err
	}
	artifactPolicy, err := marshalManifestJSON("artifact_policy", artifactPolicyValue)
	if err != nil {
		return nil, err
	}
	localRequirements, err := marshalManifestJSON("local_requirements", m.LocalRequirements)
	if err != nil {
		return nil, err
	}
	providerBinding, err := marshalManifestJSON("provider_binding", m.ProviderBinding)
	if err != nil {
		return nil, err
	}
	providerCapabilities, err := marshalManifestJSON("provider_capabilities", m.ProviderCapabilities)
	if err != nil {
		return nil, err
	}
	nextRecommendedTools, err := marshalManifestJSON("next_recommended_tools", m.NextRecommendedTools)
	if err != nil {
		return nil, err
	}
	failureModes, err := marshalManifestJSON("failure_modes", m.FailureModes)
	if err != nil {
		return nil, err
	}
	resourceRefs, err := marshalManifestJSON("resource_refs", m.ResourceRefs)
	if err != nil {
		return nil, err
	}

	return &model.ToolManifestRecord{
		Name:                 m.Name,
		Description:          m.Description,
		Type:                 m.Type,
		Boundary:             m.Boundary,
		Version:              m.Version,
		Endpoint:             m.Endpoint,
		Transport:            transport,
		TimeoutMs:            m.Timeout,
		InputSchema:          inputSchema,
		OutputSchema:         outputSchema,
		Parameters:           params,
		Output:               output,
		Examples:             examples,
		Sandbox:              m.Sandbox,
		Capabilities:         capabilities,
		Tags:                 tags,
		WhenToUse:            whenToUse,
		WhenNotToUse:         whenNotToUse,
		CostLevel:            costLevel,
		LatencyLevel:         latencyLevel,
		RiskLevel:            riskLevel,
		SideEffect:           m.SideEffect,
		Idempotent:           m.Idempotent || !m.SideEffect,
		ApprovalPolicy:       approvalPolicy,
		ArtifactPolicy:       artifactPolicy,
		ExecutionPlane:       executionPlane,
		RequiresUserDevice:   m.RequiresUserDevice,
		ArtifactLocation:     artifactLocation,
		LocalCommand:         m.LocalCommand,
		LocalRequirements:    localRequirements,
		Provider:             m.Provider,
		ProviderBinding:      providerBinding,
		ProviderCapabilities: providerCapabilities,
		NextRecommendedTools: nextRecommendedTools,
		FailureModes:         failureModes,
		SkillPackageID:       m.SkillPackageID,
		PromptRef:            m.PromptRef,
		ResourceRefs:         resourceRefs,
	}, nil
}

// manifestFromRecord restores a persisted manifest. Canonical JSON Schemas are
// authoritative when present. Rows created before canonical schema persistence
// are upgraded in memory from their legacy Parameters and Output projections.
func manifestFromRecord(record *model.ToolManifestRecord) (*ToolManifest, error) {
	if record == nil {
		return nil, fmt.Errorf("tool manifest record is nil")
	}

	var parameters map[string]ParamDef
	if err := unmarshalRecordJSON(record.Parameters, &parameters, "parameters"); err != nil {
		return nil, err
	}
	var output map[string]ParamDef
	if err := unmarshalRecordJSON(record.Output, &output, "output"); err != nil {
		return nil, err
	}

	inputSchema, err := unmarshalCanonicalSchema(record.InputSchema, "input_schema")
	if err != nil {
		return nil, err
	}
	if len(inputSchema) == 0 && len(parameters) > 0 {
		inputSchema = legacyProjectionToSchema(parameters)
	}
	outputSchema, err := unmarshalCanonicalSchema(record.OutputSchema, "output_schema")
	if err != nil {
		return nil, err
	}
	if len(outputSchema) == 0 && len(output) > 0 {
		outputSchema = legacyProjectionToSchema(output)
	}

	manifest := &ToolManifest{
		Name:               record.Name,
		Description:        record.Description,
		Type:               record.Type,
		Boundary:           record.Boundary,
		Version:            record.Version,
		Endpoint:           record.Endpoint,
		Timeout:            record.TimeoutMs,
		InputSchema:        inputSchema,
		OutputSchema:       outputSchema,
		Parameters:         parameters,
		Output:             output,
		Sandbox:            record.Sandbox,
		CostLevel:          record.CostLevel,
		LatencyLevel:       record.LatencyLevel,
		RiskLevel:          record.RiskLevel,
		SideEffect:         record.SideEffect,
		Idempotent:         record.Idempotent,
		ExecutionPlane:     record.ExecutionPlane,
		RequiresUserDevice: record.RequiresUserDevice,
		ArtifactLocation:   record.ArtifactLocation,
		LocalCommand:       record.LocalCommand,
		Provider:           record.Provider,
		SkillPackageID:     record.SkillPackageID,
		PromptRef:          record.PromptRef,
		RegisteredAt:       record.CreatedAt,
	}

	if err := unmarshalRecordJSON(record.Transport, &manifest.Transport, "transport"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.Examples, &manifest.Examples, "examples"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.Capabilities, &manifest.Capabilities, "capabilities"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.Tags, &manifest.Tags, "tags"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.WhenToUse, &manifest.WhenToUse, "when_to_use"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.WhenNotToUse, &manifest.WhenNotToUse, "when_not_to_use"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.ApprovalPolicy, &manifest.ApprovalPolicy, "approval_policy"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.ArtifactPolicy, &manifest.ArtifactPolicy, "artifact_policy"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.LocalRequirements, &manifest.LocalRequirements, "local_requirements"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.ProviderBinding, &manifest.ProviderBinding, "provider_binding"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.ProviderCapabilities, &manifest.ProviderCapabilities, "provider_capabilities"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.NextRecommendedTools, &manifest.NextRecommendedTools, "next_recommended_tools"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.FailureModes, &manifest.FailureModes, "failure_modes"); err != nil {
		return nil, err
	}
	if err := unmarshalRecordJSON(record.ResourceRefs, &manifest.ResourceRefs, "resource_refs"); err != nil {
		return nil, err
	}

	return manifest, nil
}

func marshalCanonicalSchema(schema map[string]interface{}, field string) (json.RawMessage, error) {
	if len(schema) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return marshalManifestJSON(field, schema)
}

func marshalManifestJSON(field string, value interface{}) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode tool manifest %s: %w", field, err)
	}
	return encoded, nil
}

func unmarshalCanonicalSchema(raw json.RawMessage, field string) (map[string]interface{}, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return nil, nil
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return nil, fmt.Errorf("decode tool manifest %s: %w", field, err)
	}
	return schema, nil
}

func unmarshalRecordJSON(raw json.RawMessage, target interface{}, field string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(trimmed, target); err != nil {
		return fmt.Errorf("decode tool manifest %s: %w", field, err)
	}
	return nil
}

func legacyProjectionToSchema(projection map[string]ParamDef) map[string]interface{} {
	properties := make(map[string]interface{}, len(projection))
	requiredNames := make([]string, 0, len(projection))
	for name, parameter := range projection {
		property := map[string]interface{}{}
		if parameter.Type != "" {
			property["type"] = parameter.Type
		}
		if parameter.Description != "" {
			property["description"] = parameter.Description
		}
		if parameter.Default != nil {
			property["default"] = parameter.Default
		}
		if len(parameter.Enum) > 0 {
			values := make([]interface{}, len(parameter.Enum))
			for i, value := range parameter.Enum {
				values[i] = value
			}
			property["enum"] = values
		}
		properties[name] = property
		if parameter.Required {
			requiredNames = append(requiredNames, name)
		}
	}

	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             []interface{}{},
		"additionalProperties": false,
	}
	if len(requiredNames) > 0 {
		sort.Strings(requiredNames)
		required := make([]interface{}, len(requiredNames))
		for i, name := range requiredNames {
			required[i] = name
		}
		schema["required"] = required
	}
	return schema
}
