package localrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

const (
	maxMCPToolsPerRunner       = 128
	maxMCPToolNameBytes        = 128
	maxMCPToolDescriptionBytes = 2048
	maxMCPToolSchemaBytes      = 32768
	maxMCPToolCatalogPayload   = 262144
)

// MCPToolAnnotations is the safe subset of standard MCP ToolAnnotations that
// may leave the user's device. Unknown annotations and _meta are never copied.
type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

type MCPToolAdvertisement struct {
	ProviderID      string                 `json:"providerId"`
	LogicalToolName string                 `json:"logicalToolName"`
	RemoteToolName  string                 `json:"remoteToolName"`
	Description     string                 `json:"description,omitempty"`
	InputSchema     map[string]interface{} `json:"inputSchema"`
	OutputSchema    map[string]interface{} `json:"outputSchema,omitempty"`
	Annotations     MCPToolAnnotations     `json:"annotations,omitempty"`
	ApprovalMode    string                 `json:"approvalMode,omitempty"`
	TimeoutSec      int                    `json:"timeoutSec,omitempty"`
}

type MCPToolCatalog struct {
	Revision string                 `json:"revision"`
	Tools    []MCPToolAdvertisement `json:"tools"`
}

type MCPProviderDiagnostic struct {
	ProviderID string `json:"providerId"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

type mcpToolLister interface {
	ListTools(context.Context) ([]localmcp.Tool, error)
	Close() error
}

type MCPProviderLoader func() ([]localmcp.ProviderConfig, error)

type MCPToolCatalogDiscoverer struct {
	loadProviders        MCPProviderLoader
	newClient            func(localmcp.ProviderConfig) mcpToolLister
	providerProbeTimeout time.Duration
}

func NewMCPToolCatalogDiscoverer(loader MCPProviderLoader) *MCPToolCatalogDiscoverer {
	return newMCPToolCatalogDiscoverer(loader, func(cfg localmcp.ProviderConfig) mcpToolLister {
		return localmcp.NewClient(cfg, nil)
	})
}

func newMCPToolCatalogDiscoverer(loader MCPProviderLoader, factory func(localmcp.ProviderConfig) mcpToolLister) *MCPToolCatalogDiscoverer {
	return &MCPToolCatalogDiscoverer{loadProviders: loader, newClient: factory, providerProbeTimeout: 3 * time.Second}
}

func (d *MCPToolCatalogDiscoverer) Discover(ctx context.Context) (MCPToolCatalog, []MCPProviderDiagnostic) {
	if d == nil || d.loadProviders == nil || d.newClient == nil {
		return canonicalMCPToolCatalog(nil), nil
	}
	providers, err := d.loadProviders()
	if err != nil {
		return canonicalMCPToolCatalog(nil), []MCPProviderDiagnostic{{Code: "PROVIDER_CONFIG_UNAVAILABLE", Message: err.Error()}}
	}
	sort.SliceStable(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	tools := make([]MCPToolAdvertisement, 0)
	diagnostics := make([]MCPProviderDiagnostic, 0)
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		client := d.newClient(provider)
		probeCtx := ctx
		cancel := func() {}
		if d.providerProbeTimeout > 0 {
			probeCtx, cancel = context.WithTimeout(ctx, d.providerProbeTimeout)
		}
		listed, listErr := client.ListTools(probeCtx)
		cancel()
		closeErr := client.Close()
		if listErr != nil || closeErr != nil {
			if listErr == nil {
				listErr = closeErr
			}
			diagnostics = append(diagnostics, MCPProviderDiagnostic{ProviderID: provider.ID, Code: "TOOLS_LIST_FAILED", Message: listErr.Error()})
			continue
		}
		for _, tool := range listed {
			if !localmcp.ToolAllowed(provider, tool.Name) {
				continue
			}
			ad, adErr := safeMCPToolAdvertisement(provider, tool)
			if adErr != nil {
				diagnostics = append(diagnostics, MCPProviderDiagnostic{ProviderID: provider.ID, Code: "INVALID_TOOL_ADVERTISEMENT", Message: adErr.Error()})
				continue
			}
			if len(tools) >= maxMCPToolsPerRunner {
				diagnostics = append(diagnostics, MCPProviderDiagnostic{ProviderID: provider.ID, Code: "CATALOG_TOOL_LIMIT", Message: "runner MCP tool catalog limit reached"})
				break
			}
			tools = append(tools, ad)
		}
	}
	catalog := canonicalMCPToolCatalog(tools)
	if wire, marshalErr := json.Marshal(catalog); marshalErr != nil || len(wire) > maxMCPToolCatalogPayload {
		message := "runner MCP tool catalog payload is too large"
		if marshalErr != nil {
			message = marshalErr.Error()
		}
		diagnostics = append(diagnostics, MCPProviderDiagnostic{Code: "CATALOG_PAYLOAD_LIMIT", Message: message})
		return canonicalMCPToolCatalog(nil), diagnostics
	}
	return catalog, diagnostics
}

func safeMCPToolAdvertisement(provider localmcp.ProviderConfig, tool localmcp.Tool) (MCPToolAdvertisement, error) {
	ad := MCPToolAdvertisement{
		ProviderID:      strings.TrimSpace(provider.ID),
		LogicalToolName: strings.TrimSpace(tool.Name),
		RemoteToolName:  localmcp.RemoteToolName(provider, tool.Name),
		Description:     strings.TrimSpace(tool.Description),
		ApprovalMode:    strings.TrimSpace(provider.ApprovalMode),
		TimeoutSec:      provider.TimeoutSec,
		Annotations:     safeMCPToolAnnotations(tool.Annotations),
	}
	var err error
	if ad.InputSchema, err = cloneBoundedSchema(tool.InputSchema); err != nil {
		return MCPToolAdvertisement{}, fmt.Errorf("input schema: %w", err)
	}
	if ad.OutputSchema, err = cloneBoundedSchema(tool.OutputSchema); err != nil {
		return MCPToolAdvertisement{}, fmt.Errorf("output schema: %w", err)
	}
	if err := validateLocalMCPToolAdvertisement(ad); err != nil {
		return MCPToolAdvertisement{}, err
	}
	return ad, nil
}

func validateLocalMCPToolAdvertisement(ad MCPToolAdvertisement) error {
	for label, value := range map[string]string{
		"provider id": ad.ProviderID, "logical tool name": ad.LogicalToolName, "remote tool name": ad.RemoteToolName,
	} {
		if value == "" || len(value) > maxMCPToolNameBytes {
			return fmt.Errorf("%s must be 1..%d bytes", label, maxMCPToolNameBytes)
		}
	}
	if len(ad.Description) > maxMCPToolDescriptionBytes {
		return fmt.Errorf("description exceeds %d bytes", maxMCPToolDescriptionBytes)
	}
	if ad.InputSchema == nil {
		return fmt.Errorf("input schema is required")
	}
	return nil
}

func cloneBoundedSchema(schema map[string]interface{}) (map[string]interface{}, error) {
	if schema == nil {
		return nil, nil
	}
	wire, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	if len(wire) > maxMCPToolSchemaBytes {
		return nil, fmt.Errorf("schema exceeds %d bytes", maxMCPToolSchemaBytes)
	}
	var clone map[string]interface{}
	if err := json.Unmarshal(wire, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}

func safeMCPToolAnnotations(input map[string]interface{}) MCPToolAnnotations {
	var result MCPToolAnnotations
	if value, ok := input["title"].(string); ok && len(value) <= maxMCPToolNameBytes {
		result.Title = value
	}
	copyBool := func(key string, target **bool) {
		if value, ok := input[key].(bool); ok {
			copy := value
			*target = &copy
		}
	}
	copyBool("readOnlyHint", &result.ReadOnlyHint)
	copyBool("destructiveHint", &result.DestructiveHint)
	copyBool("idempotentHint", &result.IdempotentHint)
	copyBool("openWorldHint", &result.OpenWorldHint)
	return result
}

func canonicalMCPToolCatalog(tools []MCPToolAdvertisement) MCPToolCatalog {
	if tools == nil {
		tools = []MCPToolAdvertisement{}
	}
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].ProviderID != tools[j].ProviderID {
			return tools[i].ProviderID < tools[j].ProviderID
		}
		if tools[i].LogicalToolName != tools[j].LogicalToolName {
			return tools[i].LogicalToolName < tools[j].LogicalToolName
		}
		return tools[i].RemoteToolName < tools[j].RemoteToolName
	})
	wire, _ := json.Marshal(tools)
	sum := sha256.Sum256(wire)
	return MCPToolCatalog{Revision: hex.EncodeToString(sum[:]), Tools: tools}
}
