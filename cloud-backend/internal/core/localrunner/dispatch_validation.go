package localrunner

import (
	"encoding/json"
	"fmt"
	"strings"
)

func validateMCPDispatchBinding(req *DispatchLocalJobRequest) error {
	if req == nil {
		return fmt.Errorf("MCP dispatch request is required")
	}
	required := map[string]string{
		"userId":             strings.TrimSpace(req.UserID),
		"targetRunnerId":     strings.TrimSpace(req.TargetRunnerID),
		"catalogRevision":    strings.TrimSpace(req.CatalogRevision),
		"mcpProviderId":      strings.TrimSpace(req.MCPProviderID),
		"mcpLogicalToolName": strings.TrimSpace(req.MCPLogicalToolName),
		"mcpRemoteToolName":  strings.TrimSpace(req.MCPRemoteToolName),
	}
	for label, value := range required {
		if value == "" {
			return fmt.Errorf("%s is required for MCP local jobs", label)
		}
	}
	if len(req.CatalogRevision) != 64 {
		return fmt.Errorf("catalogRevision must be a SHA-256 hex digest")
	}
	if req.Payload == nil {
		req.Payload = map[string]interface{}{}
	}
	wire, err := json.Marshal(req.Payload)
	if err != nil {
		return fmt.Errorf("MCP payload must be JSON: %w", err)
	}
	payload := make(map[string]interface{}, len(req.Payload)+2)
	if err := json.Unmarshal(wire, &payload); err != nil {
		return fmt.Errorf("decode MCP payload: %w", err)
	}
	rootBindings := map[string]string{
		"providerId": req.MCPProviderID, "provider_id": req.MCPProviderID,
		"toolName": req.MCPLogicalToolName, "tool_name": req.MCPLogicalToolName, "mcpTool": req.MCPLogicalToolName,
		"remoteToolName": req.MCPRemoteToolName, "remote_tool_name": req.MCPRemoteToolName,
	}
	for key, expected := range rootBindings {
		if value, exists := payload[key]; exists {
			actual, ok := value.(string)
			if !ok || strings.TrimSpace(actual) != expected {
				return fmt.Errorf("payload %s conflicts with immutable MCP binding", key)
			}
		}
		delete(payload, key)
	}
	for key, value := range payload {
		if nestedMCPRoutingKey(value) {
			return fmt.Errorf("nested MCP routing override is not allowed under payload.%s", key)
		}
	}
	payload["providerId"] = req.MCPProviderID
	payload["toolName"] = req.MCPLogicalToolName
	req.Payload = payload
	return nil
}

func nestedMCPRoutingKey(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if isMCPRoutingKey(key) || nestedMCPRoutingKey(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if nestedMCPRoutingKey(child) {
				return true
			}
		}
	}
	return false
}

func isMCPRoutingKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "_", "")) {
	case "providerid", "toolname", "mcptool", "remotetoolname":
		return true
	default:
		return false
	}
}

func catalogAdvertisesBinding(catalog MCPToolCatalog, providerID, logicalToolName, remoteToolName string) bool {
	for _, advertised := range catalog.Tools {
		if advertised.ProviderID == providerID &&
			advertised.LogicalToolName == logicalToolName &&
			advertised.RemoteToolName == remoteToolName {
			return true
		}
	}
	return false
}
