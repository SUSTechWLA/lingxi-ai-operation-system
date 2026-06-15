package tool

import "time"

// ToolManifest represents the full specification of a tool, used as the knowledge base
// for AI assistants and external developers to understand how to use or implement tools.
type ToolManifest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     string              `json:"version,omitempty"`
	Author      string              `json:"author,omitempty"`
	Type        string              `json:"type"`       // "builtin", "http", "grpc", "executable"
	Endpoint    string              `json:"endpoint,omitempty"` // URL for external tools
	Timeout     int                 `json:"timeout,omitempty"`
	Parameters  map[string]ParamDef `json:"parameters"`
	Output      map[string]ParamDef `json:"output"`
	Sandbox     bool                `json:"sandbox"`
	Examples    []ToolExample       `json:"examples,omitempty"`
	RegisteredAt time.Time          `json:"registeredAt,omitempty"`
}

type ParamDef struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
}

type ToolExample struct {
	Input  map[string]interface{} `json:"input"`
	Output map[string]interface{} `json:"output"`
}

// ManifestForTool generates a ToolManifest from a Tool by extracting its metadata.
// If the tool implements ManifestProvider, its custom Manifest() is used directly.
// Otherwise, a minimal manifest with just Name and Description is generated.
func ManifestForTool(t Tool) ToolManifest {
	if mp, ok := t.(ManifestProvider); ok {
		return mp.Manifest()
	}

	return ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
	}
}
