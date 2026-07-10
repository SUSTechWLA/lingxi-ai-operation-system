package agentruntime

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestManifestToolCatalogListsByCapabilityProviderAndBoundary(t *testing.T) {
	catalog := NewManifestToolCatalog([]*tool.ToolManifest{
		{Name: "script.generate", Boundary: tool.BoundaryCloudBuiltin, Capabilities: []string{"script_generation"}},
		{Name: "ffmpeg.probe", Boundary: tool.BoundaryLocalNative, ExecutionPlane: tool.ExecutionPlaneLocal, Capabilities: []string{"media_probe"}},
		{Name: "jimeng.generate_video", Boundary: tool.BoundaryMCPProvider, Provider: "jimeng", Capabilities: []string{"aigc_generation", "video_generation"}},
	})

	if got, ok := catalog.GetTool(context.Background(), "script.generate"); !ok || got.Name != "script.generate" {
		t.Fatalf("GetTool returned %#v, %v", got, ok)
	}
	byCapability, err := catalog.ListByCapability(context.Background(), "video_generation")
	if err != nil {
		t.Fatalf("ListByCapability returned error: %v", err)
	}
	if len(byCapability) != 1 || byCapability[0].Name != "jimeng.generate_video" {
		t.Fatalf("byCapability = %#v", byCapability)
	}
	byProvider, err := catalog.ListByProvider(context.Background(), "jimeng")
	if err != nil {
		t.Fatalf("ListByProvider returned error: %v", err)
	}
	if len(byProvider) != 1 || byProvider[0].Name != "jimeng.generate_video" {
		t.Fatalf("byProvider = %#v", byProvider)
	}
	localTools, err := catalog.ListLocalTools(context.Background())
	if err != nil {
		t.Fatalf("ListLocalTools returned error: %v", err)
	}
	if len(localTools) != 1 || localTools[0].Name != "ffmpeg.probe" {
		t.Fatalf("localTools = %#v", localTools)
	}
	mcpTools, err := catalog.ListMCPProviderTools(context.Background())
	if err != nil {
		t.Fatalf("ListMCPProviderTools returned error: %v", err)
	}
	if len(mcpTools) != 1 || mcpTools[0].Name != "jimeng.generate_video" {
		t.Fatalf("mcpTools = %#v", mcpTools)
	}
}
