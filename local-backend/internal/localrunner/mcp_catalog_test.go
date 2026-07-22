package localrunner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

type fakeMCPToolLister struct {
	tools []localmcp.Tool
	err   error
}

func (f *fakeMCPToolLister) ListTools(context.Context) ([]localmcp.Tool, error) {
	return f.tools, f.err
}

func (f *fakeMCPToolLister) Close() error { return nil }

func TestMCPToolCatalogDiscoveryAdvertisesOnlySafeAllowlistedFields(t *testing.T) {
	const secret = "secret-canary-do-not-sync"
	providers := []localmcp.ProviderConfig{
		{
			ID:           "studio",
			Label:        secret,
			Endpoint:     "https://" + secret + ".invalid/mcp",
			Headers:      map[string]string{"Authorization": "Bearer " + secret},
			Env:          map[string]string{"API_KEY": secret},
			Enabled:      true,
			ToolPrefix:   "studio.",
			ToolNameMap:  map[string]string{"studio.render": "remote_render"},
			EnabledTools: []string{"studio.render"},
			ApprovalMode: localmcp.ApprovalModeBeforeExecute,
			TimeoutSec:   45,
		},
	}
	discoverer := newMCPToolCatalogDiscoverer(
		func() ([]localmcp.ProviderConfig, error) { return providers, nil },
		func(localmcp.ProviderConfig) mcpToolLister {
			return &fakeMCPToolLister{tools: []localmcp.Tool{
				{
					Name:         "studio.render",
					Description:  "Render one studio shot",
					InputSchema:  map[string]any{"type": "object", "properties": map[string]any{"shot": map[string]any{"type": "string"}}},
					OutputSchema: map[string]any{"type": "object"},
					Annotations: map[string]any{
						"title":           "Studio render",
						"readOnlyHint":    false,
						"destructiveHint": false,
						"futureHint":      secret,
					},
					Meta: map[string]any{"token": secret},
					Raw:  map[string]any{"_meta": map[string]any{"token": secret}, "endpoint": secret},
				},
				{Name: "studio.not_enabled", InputSchema: map[string]any{"type": "object"}},
			}}
		},
	)

	catalog, diagnostics := discoverer.Discover(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if catalog.Revision == "" || len(catalog.Tools) != 1 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	tool := catalog.Tools[0]
	if tool.ProviderID != "studio" || tool.LogicalToolName != "studio.render" || tool.RemoteToolName != "remote_render" {
		t.Fatalf("tool names were not mapped safely: %#v", tool)
	}
	if tool.ApprovalMode != localmcp.ApprovalModeBeforeExecute || tool.TimeoutSec != 45 {
		t.Fatalf("provider policy missing: %#v", tool)
	}
	if tool.Annotations.Title != "Studio render" || tool.Annotations.ReadOnlyHint == nil || *tool.Annotations.ReadOnlyHint {
		t.Fatalf("standard annotations missing: %#v", tool.Annotations)
	}
	wire, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) || strings.Contains(string(wire), "endpoint") || strings.Contains(string(wire), "headers") || strings.Contains(string(wire), "_meta") {
		t.Fatalf("catalog leaked provider transport or secret data: %s", wire)
	}
}

func TestMCPToolCatalogDiscoveryIsStableAndOmitsFailedProviders(t *testing.T) {
	providers := []localmcp.ProviderConfig{{ID: "z", Enabled: true}, {ID: "a", Enabled: true}, {ID: "off", Enabled: false}}
	discoverer := newMCPToolCatalogDiscoverer(
		func() ([]localmcp.ProviderConfig, error) { return providers, nil },
		func(provider localmcp.ProviderConfig) mcpToolLister {
			if provider.ID == "z" {
				return &fakeMCPToolLister{err: errors.New("offline")}
			}
			return &fakeMCPToolLister{tools: []localmcp.Tool{
				{Name: "beta", InputSchema: map[string]any{"type": "object"}},
				{Name: "alpha", InputSchema: map[string]any{"type": "object"}},
			}}
		},
	)

	first, diagnostics := discoverer.Discover(context.Background())
	second, _ := discoverer.Discover(context.Background())
	if first.Revision == "" || first.Revision != second.Revision {
		t.Fatalf("revision is not stable: first=%q second=%q", first.Revision, second.Revision)
	}
	if len(first.Tools) != 2 || first.Tools[0].LogicalToolName != "alpha" || first.Tools[1].LogicalToolName != "beta" {
		t.Fatalf("catalog must be canonically sorted and omit failed provider: %#v", first.Tools)
	}
	if len(diagnostics) != 1 || diagnostics[0].ProviderID != "z" {
		t.Fatalf("failed provider diagnostic missing: %#v", diagnostics)
	}
}

func TestMCPToolCatalogDiscoveryRejectsOversizedCatalog(t *testing.T) {
	discoverer := newMCPToolCatalogDiscoverer(
		func() ([]localmcp.ProviderConfig, error) {
			return []localmcp.ProviderConfig{{ID: "p", Enabled: true}}, nil
		},
		func(localmcp.ProviderConfig) mcpToolLister {
			return &fakeMCPToolLister{tools: []localmcp.Tool{{
				Name:        "huge",
				Description: strings.Repeat("x", maxMCPToolDescriptionBytes+1),
				InputSchema: map[string]any{"type": "object"},
			}}}
		},
	)

	catalog, diagnostics := discoverer.Discover(context.Background())
	if len(catalog.Tools) != 0 || len(diagnostics) != 1 || diagnostics[0].Code != "INVALID_TOOL_ADVERTISEMENT" {
		t.Fatalf("oversized tool should be omitted with diagnostic: catalog=%#v diagnostics=%#v", catalog, diagnostics)
	}
}

type blockingMCPToolLister struct{}

func (*blockingMCPToolLister) ListTools(ctx context.Context) ([]localmcp.Tool, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*blockingMCPToolLister) Close() error { return nil }

func TestMCPToolCatalogDiscoveryBoundsEachProviderProbe(t *testing.T) {
	discoverer := newMCPToolCatalogDiscoverer(
		func() ([]localmcp.ProviderConfig, error) {
			return []localmcp.ProviderConfig{{ID: "slow", Enabled: true}}, nil
		},
		func(localmcp.ProviderConfig) mcpToolLister { return &blockingMCPToolLister{} },
	)
	discoverer.providerProbeTimeout = 10 * time.Millisecond
	started := time.Now()
	_, diagnostics := discoverer.Discover(context.Background())
	if time.Since(started) > time.Second || len(diagnostics) != 1 || diagnostics[0].Code != "TOOLS_LIST_FAILED" {
		t.Fatalf("provider probe was not bounded: elapsed=%v diagnostics=%#v", time.Since(started), diagnostics)
	}
}

func TestMCPToolCatalogDiscoveryOmitsStructurallyInvalidToolsWithoutDroppingProvider(t *testing.T) {
	discoverer := newMCPToolCatalogDiscoverer(
		func() ([]localmcp.ProviderConfig, error) {
			return []localmcp.ProviderConfig{{ID: "p", Enabled: true, ApprovalMode: localmcp.ApprovalModeNone, TimeoutSec: 30}}, nil
		},
		func(localmcp.ProviderConfig) mcpToolLister {
			return &fakeMCPToolLister{tools: []localmcp.Tool{
				{Name: "good", InputSchema: map[string]any{"type": "object"}},
				{Name: "array-root", InputSchema: map[string]any{"type": "array"}},
				{Name: "remote-ref", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"$ref": "https://evil.invalid/schema"}}}},
				{Name: "bad-annotation", InputSchema: map[string]any{"type": "object"}, Annotations: map[string]any{"readOnlyHint": "yes"}},
			}}
		},
	)
	catalog, diagnostics := discoverer.Discover(context.Background())
	if len(catalog.Tools) != 1 || catalog.Tools[0].LogicalToolName != "good" {
		t.Fatalf("invalid tools should be isolated: %#v", catalog.Tools)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("want one diagnostic per invalid tool, got %#v", diagnostics)
	}
}

func TestLocalMCPAdvertisementRejectsInvalidProviderPolicyAndDeepSchema(t *testing.T) {
	base := MCPToolAdvertisement{
		ProviderID: "p", LogicalToolName: "p.echo", RemoteToolName: "echo",
		InputSchema: map[string]any{"type": "object"},
	}
	invalidApproval := base
	invalidApproval.ApprovalMode = "sometimes"
	if err := validateLocalMCPToolAdvertisement(invalidApproval); err == nil {
		t.Fatal("invalid approval mode accepted")
	}
	invalidTimeout := base
	invalidTimeout.TimeoutSec = 86401
	if err := validateLocalMCPToolAdvertisement(invalidTimeout); err == nil {
		t.Fatal("invalid timeout accepted")
	}
	deep := map[string]any{"type": "object"}
	cursor := deep
	for range maxMCPToolSchemaDepth + 2 {
		next := map[string]any{"type": "object"}
		cursor["properties"] = map[string]any{"nested": next}
		cursor = next
	}
	deepSchema := base
	deepSchema.InputSchema = deep
	if err := validateLocalMCPToolAdvertisement(deepSchema); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("deep schema accepted: %v", err)
	}
}
