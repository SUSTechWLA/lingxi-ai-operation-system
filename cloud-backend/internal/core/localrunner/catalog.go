package localrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxMCPToolsPerRunner       = 128
	MaxMCPToolNameBytes        = 128
	MaxMCPToolDescriptionBytes = 2048
	MaxMCPToolSchemaBytes      = 32768
	MaxRunnerCapabilitiesBytes = 262144
)

func ValidateRunnerCapabilities(capabilities []RunnerCapability) error {
	if wire, err := json.Marshal(capabilities); err != nil {
		return fmt.Errorf("encode runner capabilities: %w", err)
	} else if len(wire) > MaxRunnerCapabilitiesBytes {
		return fmt.Errorf("runner capabilities exceed %d bytes", MaxRunnerCapabilitiesBytes)
	}
	toolCount := 0
	mcpCapabilityCount := 0
	bindings := make(map[string]struct{})
	for _, capability := range capabilities {
		if len(capability.ToolName) > MaxMCPToolNameBytes || len(capability.Command) > MaxMCPToolNameBytes || len(capability.Version) > MaxMCPToolDescriptionBytes {
			return fmt.Errorf("runner capability fields exceed limits")
		}
		if NormalizeCommand(capability.Command) != CommandLocalMCPToolCall {
			if capability.CatalogRevision != "" || len(capability.MCPTools) > 0 {
				return fmt.Errorf("MCP catalog is only valid for %s", CommandLocalMCPToolCall)
			}
			continue
		}
		mcpCapabilityCount++
		if mcpCapabilityCount > 1 {
			return fmt.Errorf("runner may advertise only one MCP catalog capability")
		}
		if len(capability.CatalogRevision) != 64 && (capability.CatalogRevision != "" || len(capability.MCPTools) > 0) {
			return fmt.Errorf("MCP catalog revision must be a SHA-256 hex digest")
		}
		for _, tool := range capability.MCPTools {
			toolCount++
			if toolCount > MaxMCPToolsPerRunner {
				return fmt.Errorf("MCP tool count exceeds %d", MaxMCPToolsPerRunner)
			}
			if err := validateMCPToolAdvertisement(tool); err != nil {
				return err
			}
			binding := tool.ProviderID + "\x00" + tool.LogicalToolName + "\x00" + tool.RemoteToolName
			if _, exists := bindings[binding]; exists {
				return fmt.Errorf("duplicate MCP tool binding")
			}
			bindings[binding] = struct{}{}
		}
		if capability.CatalogRevision != "" && capability.CatalogRevision != MCPToolCatalogRevision(capability.MCPTools) {
			return fmt.Errorf("MCP catalog revision does not match advertised tools")
		}
	}
	return nil
}

type MCPToolCatalog struct {
	Revision string                 `json:"revision"`
	Tools    []MCPToolAdvertisement `json:"tools"`
}

func MCPToolCatalogRevision(tools []MCPToolAdvertisement) string {
	canonical := append([]MCPToolAdvertisement(nil), tools...)
	sort.SliceStable(canonical, func(i, j int) bool {
		if canonical[i].ProviderID != canonical[j].ProviderID {
			return canonical[i].ProviderID < canonical[j].ProviderID
		}
		if canonical[i].LogicalToolName != canonical[j].LogicalToolName {
			return canonical[i].LogicalToolName < canonical[j].LogicalToolName
		}
		return canonical[i].RemoteToolName < canonical[j].RemoteToolName
	})
	if canonical == nil {
		canonical = []MCPToolAdvertisement{}
	}
	wire, _ := json.Marshal(canonical)
	sum := sha256.Sum256(wire)
	return hex.EncodeToString(sum[:])
}

func MCPToolCatalogFromCapabilities(capabilities []RunnerCapability) (MCPToolCatalog, bool) {
	for _, capability := range capabilities {
		if NormalizeCommand(capability.Command) != CommandLocalMCPToolCall || !capability.Available || capability.CatalogRevision == "" {
			continue
		}
		return MCPToolCatalog{
			Revision: capability.CatalogRevision,
			Tools:    append([]MCPToolAdvertisement(nil), capability.MCPTools...),
		}, true
	}
	return MCPToolCatalog{}, false
}

func validateMCPToolAdvertisement(tool MCPToolAdvertisement) error {
	for label, value := range map[string]string{
		"providerId": tool.ProviderID, "logicalToolName": tool.LogicalToolName, "remoteToolName": tool.RemoteToolName,
	} {
		if strings.TrimSpace(value) == "" || len(value) > MaxMCPToolNameBytes {
			return fmt.Errorf("%s must be 1..%d bytes", label, MaxMCPToolNameBytes)
		}
	}
	if len(tool.Description) > MaxMCPToolDescriptionBytes || len(tool.Annotations.Title) > MaxMCPToolNameBytes {
		return fmt.Errorf("MCP tool text exceeds limits")
	}
	if tool.InputSchema == nil {
		return fmt.Errorf("MCP tool inputSchema is required")
	}
	for label, schema := range map[string]map[string]interface{}{"inputSchema": tool.InputSchema, "outputSchema": tool.OutputSchema} {
		if schema == nil {
			continue
		}
		wire, err := json.Marshal(schema)
		if err != nil {
			return fmt.Errorf("%s is not JSON: %w", label, err)
		}
		if len(wire) > MaxMCPToolSchemaBytes {
			return fmt.Errorf("%s exceeds %d bytes", label, MaxMCPToolSchemaBytes)
		}
	}
	switch tool.ApprovalMode {
	case "", "none", "before_execute", "always":
	default:
		return fmt.Errorf("invalid MCP approvalMode %q", tool.ApprovalMode)
	}
	if tool.TimeoutSec < 0 || tool.TimeoutSec > 86400 {
		return fmt.Errorf("invalid MCP timeoutSec")
	}
	return nil
}
