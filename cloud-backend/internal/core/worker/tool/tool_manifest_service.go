package tool

import (
	"context"
	"encoding/json"
	"fmt"
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
	rdb      *redis.Client
	registry *ToolRegistry
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
	for _, m := range manifests {
		record := manifestToRecord(m)
		if err := s.repo.Upsert(ctx, record); err != nil {
			zap.L().Error("Failed to sync builtin tool", zap.String("name", m.Name), zap.Error(err))
			continue
		}
	}
	zap.L().Info("Synced builtin tools to database", zap.Int("count", len(manifests)))

	// Invalidate cache after sync
	return s.invalidateCache(ctx)
}

// RegisterExternal persists an external tool to DB and registry, then invalidates cache.
func (s *ToolManifestService) RegisterExternal(ctx context.Context, manifest *ToolManifest) error {
	record := manifestToRecord(manifest)
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
	record := manifestToRecord(manifest)
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
			return manifests, nil
		}
		// Corrupt cache — proceed to DB
		zap.L().Warn("Tool manifest cache corrupt, falling back to DB")
	}

	// 2. Query DB
	manifests, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query tool manifests: %w", err)
	}

	// 3. Populate cache
	data, _ := json.Marshal(manifests)
	if err := s.rdb.Set(ctx, toolCacheKey, data, toolCacheTTL).Err(); err != nil {
		zap.L().Warn("Failed to cache tool manifests", zap.Error(err))
	}

	return manifests, nil
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
		desc = "llm_api: 通用大模型调用，可执行任意文本生成任务"
	}
	return desc, nil
}

func (s *ToolManifestService) invalidateCache(ctx context.Context) error {
	return s.rdb.Del(ctx, toolCacheKey).Err()
}

// manifestToRecord converts a ToolManifest to a model.ToolManifestRecord for DB storage.
func manifestToRecord(m *ToolManifest) *model.ToolManifestRecord {
	params, _ := json.Marshal(m.Parameters)
	output, _ := json.Marshal(m.Output)
	examples, _ := json.Marshal(m.Examples)
	capabilities, _ := json.Marshal(m.Capabilities)
	tags, _ := json.Marshal(m.Tags)
	approvalPolicy, _ := json.Marshal(m.ApprovalPolicy)
	artifactPolicy, _ := json.Marshal(m.ArtifactPolicy)
	localRequirements, _ := json.Marshal(m.LocalRequirements)
	providerCapabilities, _ := json.Marshal(m.ProviderCapabilities)
	nextRecommendedTools, _ := json.Marshal(m.NextRecommendedTools)
	failureModes, _ := json.Marshal(m.FailureModes)
	resourceRefs, _ := json.Marshal(m.ResourceRefs)

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
		artifactLocation = ArtifactLocationCloud
	}

	return &model.ToolManifestRecord{
		Name:                 m.Name,
		Description:          m.Description,
		Type:                 m.Type,
		Version:              m.Version,
		Endpoint:             m.Endpoint,
		TimeoutMs:            m.Timeout,
		Parameters:           params,
		Output:               output,
		Examples:             examples,
		Sandbox:              m.Sandbox,
		Capabilities:         capabilities,
		Tags:                 tags,
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
		ProviderCapabilities: providerCapabilities,
		NextRecommendedTools: nextRecommendedTools,
		FailureModes:         failureModes,
		SkillPackageID:       m.SkillPackageID,
		PromptRef:            m.PromptRef,
		ResourceRefs:         resourceRefs,
	}
}
