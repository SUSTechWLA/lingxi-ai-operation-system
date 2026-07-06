package skillcapability

import (
	"encoding/json"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ToolManifests converts the capability entries into ToolManifest slices
// suitable for registration with the tool registry and manifest service.
func ToolManifests(entries []ToolEntry) []*tool.ToolManifest {
	if len(entries) == 0 {
		return nil
	}
	result := make([]*tool.ToolManifest, 0, len(entries))
	for _, entry := range entries {
		m := toToolManifest(entry)
		if m == nil {
			continue
		}
		result = append(result, m)
	}
	return result
}

// toToolManifest decodes a map[string]any entry into a *ToolManifest via JSON round-trip.
func toToolManifest(entry ToolEntry) *tool.ToolManifest {
	raw, err := json.Marshal(entry.Manifest)
	if err != nil {
		return nil
	}
	var manifest tool.ToolManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	return &manifest
}
