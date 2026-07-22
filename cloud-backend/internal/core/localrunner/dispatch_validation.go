package localrunner

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

const mcpContractSnapshotKey = "_mcpContract"

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

// bindMCPContractSnapshot persists the non-secret canonical schemas and exact
// catalog identity inside the cloud-owned job payload after user arguments
// have passed routing-override checks. This keeps result verification stable
// even if the runner advertises a newer catalog after executing the job.
func bindMCPContractSnapshot(req *DispatchLocalJobRequest, catalog *RunnerMCPToolCatalog) error {
	if req == nil || catalog == nil {
		return fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: bound MCP catalog is unavailable")
	}
	for _, advertised := range catalog.Tools {
		if advertised.ProviderID != req.MCPProviderID || advertised.LogicalToolName != req.MCPLogicalToolName || advertised.RemoteToolName != req.MCPRemoteToolName {
			continue
		}
		arguments, _ := req.Payload["arguments"].(map[string]interface{})
		if arguments == nil {
			arguments = map[string]interface{}{}
		}
		if err := tool.ValidateManifestInput(&tool.ToolManifest{InputSchema: advertised.InputSchema}, arguments); err != nil {
			return fmt.Errorf("INPUT_SCHEMA_INVALID: %w", err)
		}
		wire, err := json.Marshal(map[string]interface{}{
			"catalogRevision": catalog.Revision,
			"providerId":      advertised.ProviderID,
			"logicalToolName": advertised.LogicalToolName,
			"remoteToolName":  advertised.RemoteToolName,
			"inputSchema":     advertised.InputSchema,
			"outputSchema":    advertised.OutputSchema,
			"approvalMode":    advertised.ApprovalMode,
			"timeoutSec":      advertised.TimeoutSec,
		})
		if err != nil {
			return fmt.Errorf("encode MCP contract snapshot: %w", err)
		}
		var snapshot map[string]interface{}
		if err := json.Unmarshal(wire, &snapshot); err != nil {
			return fmt.Errorf("decode MCP contract snapshot: %w", err)
		}
		if req.Payload == nil {
			req.Payload = map[string]interface{}{}
		}
		req.Payload[mcpContractSnapshotKey] = snapshot
		return nil
	}
	return fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: exact MCP contract is not advertised")
}
