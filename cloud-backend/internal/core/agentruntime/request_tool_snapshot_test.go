package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type stubRunnerCatalogSource struct {
	catalogs []localrunner.RunnerMCPToolCatalog
	userID   string
	deviceID string
	runnerID string
}

func (s *stubRunnerCatalogSource) ListOnlineRunnerMCPToolCatalogs(_ context.Context, userID, deviceID, runnerID string) ([]localrunner.RunnerMCPToolCatalog, error) {
	s.userID, s.deviceID, s.runnerID = userID, deviceID, runnerID
	return append([]localrunner.RunnerMCPToolCatalog(nil), s.catalogs...), nil
}

func TestLocalMCPRequestToolResolverBuildsImmutableUserDeviceSnapshot(t *testing.T) {
	base := staticToolList{{Name: "cloud_tool", Type: "builtin"}}
	catalog := localrunner.RunnerMCPToolCatalog{
		RunnerID: "runner-a", DeviceID: "device-a", UserID: "user-a",
		Tools: []localrunner.MCPToolAdvertisement{{
			ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
			Description: "Render a studio shot", InputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
			},
		}},
	}
	catalog.Revision = localrunner.MCPToolCatalogRevision(catalog.Tools)
	source := &stubRunnerCatalogSource{catalogs: []localrunner.RunnerMCPToolCatalog{catalog}}
	resolver := NewLocalMCPRequestToolResolver(base, source)

	snapshot, err := resolver.Resolve(context.Background(), "user-a", "device-a", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if source.userID != "user-a" || source.deviceID != "device-a" || source.runnerID != "" {
		t.Fatalf("catalog query was not request scoped: user=%q device=%q runner=%q", source.userID, source.deviceID, source.runnerID)
	}
	manifest := snapshot.GetManifest("studio.render")
	if manifest == nil || manifest.ProviderBinding == nil {
		t.Fatalf("dynamic manifest missing from snapshot: %#v", manifest)
	}
	if manifest.ProviderBinding.TargetRunnerID != "runner-a" || manifest.ProviderBinding.CatalogRevision != catalog.Revision {
		t.Fatalf("immutable runner binding missing: %#v", manifest.ProviderBinding)
	}
	if manifest.ExecutionPlane != tool.ExecutionPlaneLocal || manifest.LocalCommand != localrunner.CommandLocalMCPToolCall {
		t.Fatalf("dynamic manifest did not use local MCP gateway: %#v", manifest)
	}
	if snapshot.GetManifest("cloud_tool") == nil {
		t.Fatal("global cloud manifest was not included in request snapshot")
	}

	// The source DTO may be mutated after Resolve, but one run's snapshot must
	// remain stable.
	source.catalogs[0].Tools[0].InputSchema["type"] = "string"
	if got := snapshot.GetManifest("studio.render").InputSchema["type"]; got != "object" {
		t.Fatalf("snapshot mutated with source catalog: type=%#v", got)
	}
}

func TestLocalMCPRequestToolResolverRejectsCrossUserCatalogAndMultiDeviceAmbiguity(t *testing.T) {
	toolAd := func(provider string) localrunner.MCPToolAdvertisement {
		return localrunner.MCPToolAdvertisement{
			ProviderID: provider, LogicalToolName: "studio.render", RemoteToolName: "render",
			InputSchema: map[string]interface{}{"type": "object"},
		}
	}
	catalogA1 := localrunner.RunnerMCPToolCatalog{RunnerID: "runner-a1", DeviceID: "device-a1", UserID: "user-a", Tools: []localrunner.MCPToolAdvertisement{toolAd("studio-a1")}}
	catalogA1.Revision = localrunner.MCPToolCatalogRevision(catalogA1.Tools)
	catalogA2 := localrunner.RunnerMCPToolCatalog{RunnerID: "runner-a2", DeviceID: "device-a2", UserID: "user-a", Tools: []localrunner.MCPToolAdvertisement{toolAd("studio-a2")}}
	catalogA2.Revision = localrunner.MCPToolCatalogRevision(catalogA2.Tools)
	source := &stubRunnerCatalogSource{catalogs: []localrunner.RunnerMCPToolCatalog{catalogA1, catalogA2}}
	resolver := NewLocalMCPRequestToolResolver(staticToolList{}, source)

	if _, err := resolver.Resolve(context.Background(), "user-a", "", ""); err == nil || !strings.Contains(err.Error(), MCPToolAmbiguousCode) {
		t.Fatalf("same-name tools on two devices must be rejected as ambiguous, got %v", err)
	}

	source.catalogs = []localrunner.RunnerMCPToolCatalog{catalogA2}
	selected, err := resolver.Resolve(context.Background(), "user-a", "device-a2", "runner-a2")
	if err != nil {
		t.Fatalf("explicit device/runner selection failed: %v", err)
	}
	if got := selected.GetManifest("studio.render").ProviderBinding.ProviderID; got != "studio-a2" {
		t.Fatalf("selected provider=%q, want studio-a2", got)
	}

	foreign := localrunner.RunnerMCPToolCatalog{
		RunnerID: "runner-b", DeviceID: "device-b", UserID: "user-b",
		Tools: []localrunner.MCPToolAdvertisement{toolAd("studio-b")},
	}
	foreign.Revision = localrunner.MCPToolCatalogRevision(foreign.Tools)
	source.catalogs = []localrunner.RunnerMCPToolCatalog{foreign}
	if _, err := resolver.Resolve(context.Background(), "user-a", "", ""); err == nil || !strings.Contains(err.Error(), MCPUserScopeViolationCode) {
		t.Fatalf("foreign-user catalog must fail closed, got %v", err)
	}
}

func TestLocalMCPRequestToolResolverRejectsGlobalNameCollision(t *testing.T) {
	catalog := localrunner.RunnerMCPToolCatalog{
		RunnerID: "runner-a", UserID: "user-a",
		Tools: []localrunner.MCPToolAdvertisement{{
			ProviderID: "studio", LogicalToolName: "cloud_tool", RemoteToolName: "render",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
	}
	catalog.Revision = localrunner.MCPToolCatalogRevision(catalog.Tools)
	source := &stubRunnerCatalogSource{catalogs: []localrunner.RunnerMCPToolCatalog{catalog}}
	resolver := NewLocalMCPRequestToolResolver(staticToolList{{Name: "cloud_tool"}}, source)
	if _, err := resolver.Resolve(context.Background(), "user-a", "", ""); err == nil || !strings.Contains(err.Error(), MCPToolNameConflictCode) {
		t.Fatalf("dynamic tool must not shadow global manifest, got %v", err)
	}
}
