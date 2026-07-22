package localrunner

import (
	"context"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ResolveMCPJobManifest reconstructs the non-secret canonical contract for an
// already bound local MCP job from that runner's current catalog. The revision
// and exact provider/logical/remote binding must still match; no global tool
// registration is consulted or mutated.
func (s *Service) ResolveMCPJobManifest(ctx context.Context, job *LocalJob) (*tool.ToolManifest, error) {
	if job == nil || NormalizeCommand(job.Command) != CommandLocalMCPToolCall {
		return nil, nil
	}
	if snapshot, ok := job.Payload[mcpContractSnapshotKey].(map[string]interface{}); ok {
		return mcpJobManifestFromSnapshot(job, snapshot)
	}
	catalog, err := s.GetOnlineRunnerMCPToolCatalog(ctx, job.UserID, "", job.TargetRunnerID)
	if err != nil {
		return nil, err
	}
	return mcpJobManifestFromCatalog(job, catalog)
}

func mcpJobManifestFromSnapshot(job *LocalJob, snapshot map[string]interface{}) (*tool.ToolManifest, error) {
	if job == nil || snapshot == nil {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: immutable MCP contract snapshot is missing")
	}
	bindings := map[string]string{
		"catalogRevision": job.CatalogRevision,
		"providerId":      job.MCPProviderID,
		"logicalToolName": job.MCPLogicalToolName,
		"remoteToolName":  job.MCPRemoteToolName,
	}
	for key, expected := range bindings {
		actual, ok := snapshot[key].(string)
		if !ok || strings.TrimSpace(actual) != strings.TrimSpace(expected) {
			return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: immutable MCP contract %s does not match the job binding", key)
		}
	}
	inputSchema, _ := snapshot["inputSchema"].(map[string]interface{})
	outputSchema, _ := snapshot["outputSchema"].(map[string]interface{})
	advertised := MCPToolAdvertisement{
		ProviderID: job.MCPProviderID, LogicalToolName: job.MCPLogicalToolName, RemoteToolName: job.MCPRemoteToolName,
		InputSchema: inputSchema, OutputSchema: outputSchema,
		ApprovalMode: strings.TrimSpace(fmt.Sprint(snapshot["approvalMode"])),
		TimeoutSec:   intFromJSONNumber(snapshot["timeoutSec"]),
	}
	if err := validateMCPToolAdvertisement(advertised); err != nil {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: immutable MCP contract is invalid: %w", err)
	}
	return mcpManifestFromAdvertisement(job, advertised, "")
}

func intFromJSONNumber(value interface{}) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func mcpJobManifestFromCatalog(job *LocalJob, catalog *RunnerMCPToolCatalog) (*tool.ToolManifest, error) {
	if job == nil {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: local MCP job is missing")
	}
	if catalog == nil || strings.TrimSpace(catalog.RunnerID) != strings.TrimSpace(job.TargetRunnerID) || strings.TrimSpace(catalog.UserID) != strings.TrimSpace(job.UserID) {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: bound runner catalog is unavailable in the job owner scope")
	}
	if catalog.Revision != MCPToolCatalogRevision(catalog.Tools) {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: stored runner catalog revision is invalid")
	}
	if strings.TrimSpace(catalog.Revision) == "" || catalog.Revision != job.CatalogRevision {
		return nil, fmt.Errorf("MCP_CATALOG_STALE: bound runner catalog changed before result verification")
	}
	for _, advertised := range catalog.Tools {
		if advertised.ProviderID != job.MCPProviderID || advertised.LogicalToolName != job.MCPLogicalToolName || advertised.RemoteToolName != job.MCPRemoteToolName {
			continue
		}
		return mcpManifestFromAdvertisement(job, advertised, catalog.DeviceID)
	}
	return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: exact provider/tool binding is no longer advertised")
}

func mcpManifestFromAdvertisement(job *LocalJob, advertised MCPToolAdvertisement, deviceID string) (*tool.ToolManifest, error) {
	provider := tool.MCPProviderConfig{
		ID: advertised.ProviderID, Enabled: true, Timeout: advertised.TimeoutSec, ApprovalMode: advertised.ApprovalMode,
		ToolNameMap: map[string]string{advertised.LogicalToolName: advertised.RemoteToolName},
	}
	converted := tool.ManifestsFromMCPTools(provider, []tool.MCPTool{{
		Name: advertised.RemoteToolName, Description: advertised.Description,
		InputSchema: advertised.InputSchema, OutputSchema: advertised.OutputSchema,
	}})
	if len(converted) != 1 || converted[0].Name != job.MCPLogicalToolName {
		return nil, fmt.Errorf("MCP_CATALOG_REPLAN_REQUIRED: bound MCP tool contract cannot be reconstructed")
	}
	manifest := converted[0]
	manifest.ProviderBinding.TargetRunnerID = job.TargetRunnerID
	manifest.ProviderBinding.CatalogRevision = job.CatalogRevision
	manifest.ProviderBinding.DeviceID = deviceID
	return manifest, nil
}
