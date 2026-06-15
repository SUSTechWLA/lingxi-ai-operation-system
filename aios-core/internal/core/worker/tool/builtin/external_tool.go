package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ExternalTool executes registered external tools via their registered HTTP endpoints.
// It acts as a bridge between the DAG pipeline and external tool services implemented
// in any language.
type ExternalTool struct {
	registry *tool.ToolRegistry
	client   *http.Client
}

func NewExternalTool(registry *tool.ToolRegistry) *ExternalTool {
	return &ExternalTool{
		registry: registry,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *ExternalTool) Name() string        { return "external" }
func (t *ExternalTool) Description() string  { return "Execute registered external tools via their HTTP endpoints" }
func (t *ExternalTool) Type() tool.ToolType  { return tool.ToolTypeCustom }

// Execute handles DAG node execution. The "params" must include a "tool" field
// naming the registered external tool. Parameters are forwarded to the external
// endpoint as JSON.
func (t *ExternalTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	toolName, _ := params["tool"].(string)
	if toolName == "" {
		return tool.FailureResult("'tool' parameter is required (name of the registered external tool)")
	}

	// Look up the tool manifest to find the endpoint
	manifest := t.registry.GetExternalManifest(toolName)
	if manifest == nil {
		return tool.FailureResult(fmt.Sprintf("external tool '%s' not found in registry", toolName))
	}

	if manifest.Endpoint == "" {
		return tool.FailureResult(fmt.Sprintf("external tool '%s' has no endpoint configured", toolName))
	}

	// Extract actual parameters (exclude meta fields)
	execParams := make(map[string]interface{})
	for k, v := range params {
		if k != "tool" && k != "endpoint" {
			execParams[k] = v
		}
	}

	// Build execution payload following the external tool contract
	payload := map[string]interface{}{
		"tool":    toolName,
		"params":  execParams,
		"task_id": toolCtx.TaskID,
		"node_id": toolCtx.NodeID,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("failed to marshal request: %s", err.Error()))
	}

	// Use endpoint override if provided, otherwise use registered endpoint
	targetURL := manifest.Endpoint
	if ep, ok := params["endpoint"].(string); ok && ep != "" {
		targetURL = ep
	}

	zap.L().Info("Executing external tool",
		zap.String("tool", toolName),
		zap.String("endpoint", targetURL),
		zap.String("taskId", toolCtx.TaskID))

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("failed to create HTTP request: %s", err.Error()))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tangying-ai-os/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("external tool HTTP call failed: %s", err.Error()))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("failed to read external tool response: %s", err.Error()))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tool.FailureResult(fmt.Sprintf("external tool '%s' returned HTTP %d: %s", toolName, resp.StatusCode, string(respBody)))
	}

	// Try to parse as JSON
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return raw text output if not valid JSON
		return tool.SuccessResult(map[string]interface{}{
			"content":    string(respBody),
			"tool":       toolName,
			"statusCode": resp.StatusCode,
		})
	}

	result["tool"] = toolName
	return tool.SuccessResult(result)
}

func (t *ExternalTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"tool": {
				Type:        "string",
				Description: "Name of the registered external tool to execute",
				Required:    true,
			},
			"endpoint": {
				Type:        "string",
				Description: "Override the registered endpoint URL (optional)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"tool": {Type: "string", Description: "Tool name echoed back"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"tool": "image_processor", "image_url": "https://example.com/photo.jpg"},
				Output: map[string]interface{}{"width": "1920", "height": "1080", "format": "jpeg", "tool": "image_processor"},
			},
		},
	}
}

func (t *ExternalTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["tool"].(string)
	return ok
}
