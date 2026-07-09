package agentruntime

import (
	"context"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type ToolScope struct {
	Boundaries         []string
	ExecutionPlanes    []string
	Providers          []string
	Capabilities       []string
	IncludeLegacy      bool
	RequiresUserDevice *bool
}

type ToolCatalogFacade interface {
	ToolCatalog
	ListManifests() []*tool.ToolManifest
	ListLogicalTools(ctx context.Context, scope ToolScope) ([]tool.ToolManifest, error)
	GetTool(ctx context.Context, name string) (tool.ToolManifest, bool)
	ListByCapability(ctx context.Context, capability string) ([]tool.ToolManifest, error)
	ListByProvider(ctx context.Context, providerID string) ([]tool.ToolManifest, error)
	ListLocalTools(ctx context.Context) ([]tool.ToolManifest, error)
	ListMCPProviderTools(ctx context.Context) ([]tool.ToolManifest, error)
}

type ManifestToolCatalog struct {
	byName map[string]*tool.ToolManifest
	order  []string
}

func NewManifestToolCatalog(manifests []*tool.ToolManifest) *ManifestToolCatalog {
	byName := make(map[string]*tool.ToolManifest, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil || strings.TrimSpace(manifest.Name) == "" {
			continue
		}
		copy := *manifest
		byName[manifest.Name] = &copy
	}
	order := make([]string, 0, len(byName))
	for name := range byName {
		order = append(order, name)
	}
	sort.Strings(order)
	return &ManifestToolCatalog{byName: byName, order: order}
}

func (c *ManifestToolCatalog) GetManifest(name string) *tool.ToolManifest {
	if c == nil {
		return nil
	}
	return c.byName[name]
}

func (c *ManifestToolCatalog) ListManifests() []*tool.ToolManifest {
	if c == nil {
		return nil
	}
	out := make([]*tool.ToolManifest, 0, len(c.order))
	for _, name := range c.order {
		out = append(out, c.byName[name])
	}
	return out
}

func (c *ManifestToolCatalog) ListLogicalTools(ctx context.Context, scope ToolScope) ([]tool.ToolManifest, error) {
	_ = ctx
	if c == nil {
		return nil, nil
	}
	out := make([]tool.ToolManifest, 0, len(c.order))
	for _, name := range c.order {
		manifest := c.byName[name]
		if !manifestInScope(manifest, scope) {
			continue
		}
		out = append(out, *manifest)
	}
	return out, nil
}

func (c *ManifestToolCatalog) GetTool(ctx context.Context, name string) (tool.ToolManifest, bool) {
	_ = ctx
	manifest := c.GetManifest(name)
	if manifest == nil {
		return tool.ToolManifest{}, false
	}
	return *manifest, true
}

func (c *ManifestToolCatalog) ListByCapability(ctx context.Context, capability string) ([]tool.ToolManifest, error) {
	return c.ListLogicalTools(ctx, ToolScope{Capabilities: []string{capability}})
}

func (c *ManifestToolCatalog) ListByProvider(ctx context.Context, providerID string) ([]tool.ToolManifest, error) {
	return c.ListLogicalTools(ctx, ToolScope{Providers: []string{providerID}})
}

func (c *ManifestToolCatalog) ListLocalTools(ctx context.Context) ([]tool.ToolManifest, error) {
	return c.ListLogicalTools(ctx, ToolScope{ExecutionPlanes: []string{tool.ExecutionPlaneLocal, tool.ExecutionPlaneHybrid}})
}

func (c *ManifestToolCatalog) ListMCPProviderTools(ctx context.Context) ([]tool.ToolManifest, error) {
	return c.ListLogicalTools(ctx, ToolScope{Boundaries: []string{tool.BoundaryMCPProvider}})
}

func manifestInScope(manifest *tool.ToolManifest, scope ToolScope) bool {
	if manifest == nil {
		return false
	}
	if !scope.IncludeLegacy && manifest.Boundary == tool.BoundaryLegacy {
		return false
	}
	if len(scope.Boundaries) > 0 && !stringInSet(manifest.Boundary, scope.Boundaries) {
		return false
	}
	if len(scope.ExecutionPlanes) > 0 && !stringInSet(manifest.ExecutionPlane, scope.ExecutionPlanes) {
		return false
	}
	if len(scope.Providers) > 0 && !providerInSet(manifest, scope.Providers) {
		return false
	}
	if len(scope.Capabilities) > 0 && !capabilityOverlaps(manifest, scope.Capabilities) {
		return false
	}
	if scope.RequiresUserDevice != nil && manifest.RequiresUserDevice != *scope.RequiresUserDevice {
		return false
	}
	return true
}

func stringInSet(value string, allowed []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, item := range allowed {
		if value == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func providerInSet(manifest *tool.ToolManifest, providers []string) bool {
	if manifest == nil {
		return false
	}
	candidates := []string{manifest.Provider}
	if manifest.ProviderBinding != nil {
		candidates = append(candidates, manifest.ProviderBinding.ProviderID)
	}
	for _, candidate := range candidates {
		if stringInSet(candidate, providers) {
			return true
		}
	}
	return false
}
