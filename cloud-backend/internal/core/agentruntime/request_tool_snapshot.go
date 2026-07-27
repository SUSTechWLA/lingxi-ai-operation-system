package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

const (
	MCPToolAmbiguousCode      = "MCP_TOOL_AMBIGUOUS"
	MCPToolNameConflictCode   = "MCP_TOOL_NAME_CONFLICT"
	MCPUserScopeViolationCode = "MCP_USER_SCOPE_VIOLATION"
	MCPRunnerUnavailableCode  = "MCP_RUNNER_UNAVAILABLE"
	MCPCatalogInvalidCode     = "MCP_CATALOG_INVALID"
	LocalMCPGatewayToolName   = "__local_mcp_gateway__"
)

// RunnerMCPToolCatalogSource exposes only validated, non-secret MCP tool
// advertisements from online runners. Implementations must scope the query by
// authenticated user and optional authenticated device/selected runner.
type RunnerMCPToolCatalogSource interface {
	ListOnlineRunnerMCPToolCatalogs(ctx context.Context, userID, deviceID, runnerID string) ([]localrunner.RunnerMCPToolCatalog, error)
}

type RequestToolSnapshotResolver interface {
	Resolve(ctx context.Context, userID, deviceID, runnerID string) (*RequestToolSnapshot, error)
}

type RequestMCPRunnerRevision struct {
	RunnerID string `json:"runnerId"`
	DeviceID string `json:"deviceId,omitempty"`
	Revision string `json:"catalogRevision"`
}

// RequestToolSnapshot is created once per agent run. It combines the static
// cloud catalog with runner-bound virtual MCP manifests without registering
// those manifests globally or persisting them in a shared cache.
type RequestToolSnapshot struct {
	manifests map[string]*tool.ToolManifest
	ordered   []*tool.ToolManifest
	runners   []RequestMCPRunnerRevision
}

func newRequestToolSnapshot(manifests []*tool.ToolManifest, runners []RequestMCPRunnerRevision) *RequestToolSnapshot {
	snapshot := &RequestToolSnapshot{
		manifests: make(map[string]*tool.ToolManifest, len(manifests)),
		runners:   append([]RequestMCPRunnerRevision(nil), runners...),
	}
	for _, manifest := range manifests {
		cloned := cloneToolManifest(manifest)
		if cloned == nil || strings.TrimSpace(cloned.Name) == "" {
			continue
		}
		cloned.Name = strings.TrimSpace(cloned.Name)
		if _, exists := snapshot.manifests[cloned.Name]; !exists {
			snapshot.manifests[cloned.Name] = cloned
		}
		snapshot.ordered = append(snapshot.ordered, cloned)
	}
	sort.SliceStable(snapshot.ordered, func(i, j int) bool { return snapshot.ordered[i].Name < snapshot.ordered[j].Name })
	sort.Slice(snapshot.runners, func(i, j int) bool { return snapshot.runners[i].RunnerID < snapshot.runners[j].RunnerID })
	return snapshot
}

func (s *RequestToolSnapshot) GetManifest(name string) *tool.ToolManifest {
	if s == nil {
		return nil
	}
	return cloneToolManifest(s.manifests[name])
}

func (s *RequestToolSnapshot) ListManifests() []*tool.ToolManifest {
	if s == nil {
		return nil
	}
	manifests := make([]*tool.ToolManifest, 0, len(s.ordered))
	for _, manifest := range s.ordered {
		manifests = append(manifests, cloneToolManifest(manifest))
	}
	return manifests
}

func (s *RequestToolSnapshot) RunnerRevisions() []RequestMCPRunnerRevision {
	if s == nil {
		return nil
	}
	return append([]RequestMCPRunnerRevision(nil), s.runners...)
}

type LocalMCPRequestToolResolver struct {
	base   ToolListProvider
	source RunnerMCPToolCatalogSource
}

func NewLocalMCPRequestToolResolver(base ToolListProvider, source RunnerMCPToolCatalogSource) *LocalMCPRequestToolResolver {
	return &LocalMCPRequestToolResolver{base: base, source: source}
}

func (r *LocalMCPRequestToolResolver) Resolve(ctx context.Context, userID, deviceID, runnerID string) (*RequestToolSnapshot, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	runnerID = strings.TrimSpace(runnerID)
	if userID == "" {
		return nil, fmt.Errorf("%s: authenticated user is required", MCPUserScopeViolationCode)
	}

	manifests := make([]*tool.ToolManifest, 0)
	globalNames := map[string]struct{}{}
	if r != nil && r.base != nil {
		for _, manifest := range r.base.ListManifests() {
			cloned := cloneToolManifest(manifest)
			if cloned == nil || strings.TrimSpace(cloned.Name) == "" {
				continue
			}
			globalNames[cloned.Name] = struct{}{}
			manifests = append(manifests, cloned)
		}
	}
	if r == nil || r.source == nil {
		return newRequestToolSnapshot(manifests, nil), nil
	}

	catalogs, err := r.source.ListOnlineRunnerMCPToolCatalogs(ctx, userID, deviceID, runnerID)
	if err != nil {
		return nil, fmt.Errorf("resolve request MCP catalogs: %w", err)
	}
	if runnerID != "" && len(catalogs) == 0 {
		return nil, fmt.Errorf("%s: selected runner is offline, outside the authenticated device, or has no MCP catalog", MCPRunnerUnavailableCode)
	}
	sort.Slice(catalogs, func(i, j int) bool { return catalogs[i].RunnerID < catalogs[j].RunnerID })
	dynamicNames := map[string]string{}
	runners := make([]RequestMCPRunnerRevision, 0, len(catalogs))
	for _, catalog := range catalogs {
		if strings.TrimSpace(catalog.UserID) != userID {
			return nil, fmt.Errorf("%s: runner %s belongs to a different user", MCPUserScopeViolationCode, catalog.RunnerID)
		}
		if deviceID != "" && strings.TrimSpace(catalog.DeviceID) != deviceID {
			return nil, fmt.Errorf("%s: runner %s belongs to a different device", MCPUserScopeViolationCode, catalog.RunnerID)
		}
		if runnerID != "" && strings.TrimSpace(catalog.RunnerID) != runnerID {
			return nil, fmt.Errorf("%s: catalog source returned an unselected runner", MCPUserScopeViolationCode)
		}
		if strings.TrimSpace(catalog.Revision) == "" || catalog.Revision != localrunner.MCPToolCatalogRevision(catalog.Tools) {
			return nil, fmt.Errorf("%s: runner %s catalog revision does not match its tools", MCPCatalogInvalidCode, catalog.RunnerID)
		}
		runners = append(runners, RequestMCPRunnerRevision{RunnerID: catalog.RunnerID, DeviceID: catalog.DeviceID, Revision: catalog.Revision})
		for _, advertisement := range catalog.Tools {
			name := strings.TrimSpace(advertisement.LogicalToolName)
			if _, exists := globalNames[name]; exists {
				return nil, fmt.Errorf("%s: local MCP tool %q would shadow a global tool", MCPToolNameConflictCode, name)
			}
			if previousRunner, exists := dynamicNames[name]; exists {
				return nil, fmt.Errorf("%s: tool %q is advertised by runners %s and %s; select one authenticated device or runner", MCPToolAmbiguousCode, name, previousRunner, catalog.RunnerID)
			}
			manifest, buildErr := requestMCPManifest(catalog, advertisement)
			if buildErr != nil {
				return nil, buildErr
			}
			dynamicNames[name] = catalog.RunnerID
			manifests = append(manifests, manifest)
		}
	}
	return newRequestToolSnapshot(manifests, runners), nil
}

func requestMCPManifest(catalog localrunner.RunnerMCPToolCatalog, advertised localrunner.MCPToolAdvertisement) (*tool.ToolManifest, error) {
	annotations := map[string]interface{}{}
	if advertised.Annotations.Title != "" {
		annotations["title"] = advertised.Annotations.Title
	}
	for key, value := range map[string]*bool{
		"readOnlyHint": advertised.Annotations.ReadOnlyHint, "destructiveHint": advertised.Annotations.DestructiveHint,
		"idempotentHint": advertised.Annotations.IdempotentHint, "openWorldHint": advertised.Annotations.OpenWorldHint,
	} {
		if value != nil {
			annotations[key] = *value
		}
	}
	provider := tool.MCPProviderConfig{
		ID: advertised.ProviderID, Enabled: true, Timeout: advertised.TimeoutSec,
		ApprovalMode: advertised.ApprovalMode,
		ToolNameMap:  map[string]string{advertised.LogicalToolName: advertised.RemoteToolName},
	}
	converted := tool.ManifestsFromMCPTools(provider, []tool.MCPTool{{
		Name: advertised.RemoteToolName, Description: advertised.Description,
		InputSchema: cloneSnapshotJSONMap(advertised.InputSchema), OutputSchema: cloneSnapshotJSONMap(advertised.OutputSchema), Annotations: annotations,
	}})
	if len(converted) != 1 || converted[0].Name != advertised.LogicalToolName {
		return nil, fmt.Errorf("%s: cannot normalize runner %s tool %q", MCPCatalogInvalidCode, catalog.RunnerID, advertised.LogicalToolName)
	}
	manifest := converted[0]
	manifest.Version = catalog.Revision
	manifest.Transport = nil // cloud never receives local provider transports or credentials
	manifest.ProviderBinding.TargetRunnerID = catalog.RunnerID
	manifest.ProviderBinding.CatalogRevision = catalog.Revision
	manifest.ProviderBinding.DeviceID = catalog.DeviceID
	manifest.ProviderCapabilities["requestScoped"] = true
	manifest.ProviderCapabilities["catalogRevision"] = catalog.Revision
	return manifest, nil
}

func cloneToolManifest(manifest *tool.ToolManifest) *tool.ToolManifest {
	if manifest == nil {
		return nil
	}
	wire, err := json.Marshal(manifest)
	if err != nil {
		return nil
	}
	var cloned tool.ToolManifest
	if err := json.Unmarshal(wire, &cloned); err != nil {
		return nil
	}
	return &cloned
}

func cloneSnapshotJSONMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(wire, &cloned); err != nil {
		return nil
	}
	return cloned
}

func toolProviderForRequest(req StartRunRequest, fallback ToolListProvider) ToolListProvider {
	if req.requestToolSnapshot != nil {
		return req.requestToolSnapshot
	}
	return fallback
}
