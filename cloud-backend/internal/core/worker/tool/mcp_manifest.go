package tool

import (
	"fmt"
	"strings"
)

// MCPProviderConfig is the provider discovery config needed to normalize MCP
// tools/list results into ToolManifest entries. It is not a parallel tool
// descriptor; it only supplies provider routing metadata for ToolManifest.
type MCPProviderConfig struct {
	ID            string            `json:"id"`
	Label         string            `json:"label,omitempty"`
	Transport     string            `json:"transport,omitempty"`
	Endpoint      string            `json:"endpoint,omitempty"`
	ToolPrefix    string            `json:"toolPrefix,omitempty"`
	ToolNameMap   map[string]string `json:"toolNameMap,omitempty"`
	Enabled       bool              `json:"enabled"`
	EnabledTools  []string          `json:"enabledTools,omitempty"`
	DisabledTools []string          `json:"disabledTools,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
	ApprovalMode  string            `json:"approvalMode,omitempty"`
}

// MCPTool mirrors one item returned by MCP tools/list.
type MCPTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"inputSchema,omitempty"`
	OutputSchema map[string]interface{} `json:"outputSchema,omitempty"`
}

func ManifestsFromMCPTools(provider MCPProviderConfig, tools []MCPTool) []*ToolManifest {
	if !provider.Enabled {
		return nil
	}
	manifests := make([]*ToolManifest, 0, len(tools))
	for _, remoteTool := range tools {
		remoteName := strings.TrimSpace(remoteTool.Name)
		if remoteName == "" {
			continue
		}
		logicalName := logicalMCPToolName(provider, remoteName)
		if !mcpProviderToolAllowed(provider, logicalName, remoteName) {
			continue
		}
		transport := strings.TrimSpace(provider.Transport)
		if transport == "" {
			transport = "stdio"
		}
		capabilities := inferMCPCapabilities(provider.ID, logicalName, remoteTool.Description)
		manifests = append(manifests, &ToolManifest{
			Name:               logicalName,
			Description:        remoteTool.Description,
			Type:               "mcp",
			Boundary:           BoundaryMCPProvider,
			Transport:          &ToolTransport{Type: transport, Endpoint: provider.Endpoint},
			Timeout:            provider.Timeout,
			Parameters:         jsonSchemaToParamDefs(remoteTool.InputSchema),
			Output:             jsonSchemaToParamDefs(remoteTool.OutputSchema),
			Capabilities:       capabilities,
			Tags:               []string{"mcp", strings.TrimSpace(provider.ID)},
			CostLevel:          CostMedium,
			LatencyLevel:       LatencyMedium,
			RiskLevel:          RiskMedium,
			SideEffect:         false,
			Idempotent:         false,
			ExecutionPlane:     ExecutionPlaneLocal,
			RequiresUserDevice: true,
			ArtifactLocation:   ArtifactLocationLocal,
			LocalCommand:       "LOCAL_MCP_TOOL_CALL",
			Provider:           provider.ID,
			ProviderBinding: &ProviderBinding{
				ProviderID:      provider.ID,
				RemoteToolName:  remoteName,
				LogicalToolName: logicalName,
				ToolPrefix:      provider.ToolPrefix,
				ToolNameMap:     provider.ToolNameMap,
			},
			ProviderCapabilities: map[string]interface{}{
				"providerLabel": provider.Label,
				"enabled":       provider.Enabled,
				"enabledTools":  append([]string(nil), provider.EnabledTools...),
				"disabledTools": append([]string(nil), provider.DisabledTools...),
				"approvalMode":  provider.ApprovalMode,
				"inputSchema":   remoteTool.InputSchema,
				"outputSchema":  remoteTool.OutputSchema,
			},
		})
	}
	return manifests
}

func logicalMCPToolName(provider MCPProviderConfig, remoteName string) string {
	for logical, remote := range provider.ToolNameMap {
		if strings.TrimSpace(remote) == remoteName && strings.TrimSpace(logical) != "" {
			return strings.TrimSpace(logical)
		}
	}
	prefix := strings.TrimSpace(provider.ToolPrefix)
	if prefix != "" && !strings.HasPrefix(remoteName, prefix) {
		return prefix + remoteName
	}
	return remoteName
}

func mcpProviderToolAllowed(provider MCPProviderConfig, logicalName, remoteName string) bool {
	if len(provider.EnabledTools) > 0 && !mcpToolNameInList(provider, logicalName, remoteName, provider.EnabledTools) {
		return false
	}
	if mcpToolNameInList(provider, logicalName, remoteName, provider.DisabledTools) {
		return false
	}
	return true
}

func mcpToolNameInList(provider MCPProviderConfig, logicalName, remoteName string, names []string) bool {
	for _, configured := range names {
		configured = strings.TrimSpace(configured)
		if configured == "" {
			continue
		}
		configuredRemote := configured
		if mapped, ok := provider.ToolNameMap[configured]; ok {
			configuredRemote = mapped
		}
		prefix := strings.TrimSpace(provider.ToolPrefix)
		if prefix != "" && strings.HasPrefix(configuredRemote, prefix) {
			configuredRemote = strings.TrimPrefix(configuredRemote, prefix)
		}
		if strings.EqualFold(configured, logicalName) ||
			strings.EqualFold(configured, remoteName) ||
			strings.EqualFold(configuredRemote, remoteName) {
			return true
		}
	}
	return false
}

func jsonSchemaToParamDefs(schema map[string]interface{}) map[string]ParamDef {
	if len(schema) == 0 {
		return map[string]ParamDef{}
	}
	properties, _ := schema["properties"].(map[string]interface{})
	if len(properties) == 0 {
		return map[string]ParamDef{}
	}
	required := schemaRequiredSet(schema["required"])
	out := make(map[string]ParamDef, len(properties))
	for name, raw := range properties {
		prop, _ := raw.(map[string]interface{})
		paramType := strings.TrimSpace(fmt.Sprint(prop["type"]))
		if paramType == "" || paramType == "<nil>" {
			paramType = "object"
		}
		description := strings.TrimSpace(fmt.Sprint(prop["description"]))
		if description == "<nil>" {
			description = ""
		}
		out[name] = ParamDef{
			Type:        paramType,
			Description: description,
			Required:    required[name],
			Enum:        schemaStringList(prop["enum"]),
		}
	}
	return out
}

func schemaRequiredSet(raw interface{}) map[string]bool {
	out := map[string]bool{}
	switch typed := raw.(type) {
	case []string:
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out[item] = true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out[text] = true
			}
		}
	}
	return out
}

func schemaStringList(raw interface{}) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func inferMCPCapabilities(providerID, logicalName, description string) []string {
	text := strings.ToLower(strings.Join([]string{providerID, logicalName, description}, " "))
	caps := []string{"mcp_provider"}
	add := func(capability string) {
		for _, existing := range caps {
			if existing == capability {
				return
			}
		}
		caps = append(caps, capability)
	}
	switch {
	case strings.Contains(text, "generate_video") || strings.Contains(text, "video generation") || strings.Contains(text, "视频生成"):
		add("aigc_generation")
		add("video_generation")
	case strings.Contains(text, "generate_image") || strings.Contains(text, "image generation") || strings.Contains(text, "图片生成"):
		add("aigc_generation")
		add("image_generation")
	case strings.Contains(text, "video_qa") || strings.Contains(text, "analyze_video") || strings.Contains(text, "quality"):
		add("video_quality_assessment")
	case strings.Contains(text, "check_status") || strings.Contains(text, "status"):
		add("provider_status")
	}
	return caps
}
