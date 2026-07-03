package localtool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

type mcpToolCallExecutor struct {
	loadProviders MCPProviderLoader
}

func NewMCPToolCallExecutor(loader MCPProviderLoader) Executor {
	return &mcpToolCallExecutor{loadProviders: loader}
}

func (e *mcpToolCallExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	if e.loadProviders == nil {
		return nil, errors.New("mcp provider loader is not configured")
	}
	providerID := stringPayload(job.Payload, "providerId")
	if providerID == "" {
		providerID = stringPayload(job.Payload, "provider_id")
	}
	if providerID == "" {
		return nil, errors.New("providerId is required")
	}
	toolName := stringPayload(job.Payload, "toolName")
	if toolName == "" {
		toolName = stringPayload(job.Payload, "tool_name")
	}
	if toolName == "" {
		toolName = stringPayload(job.Payload, "mcpTool")
	}
	if toolName == "" {
		return nil, errors.New("toolName is required")
	}
	providers, err := e.loadProviders()
	if err != nil {
		return nil, err
	}
	provider, ok := findProvider(providers, providerID)
	if !ok {
		return nil, fmt.Errorf("mcp provider %q is not configured or enabled", providerID)
	}
	timeout := time.Duration(job.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	client := localmcp.NewClient(provider, &http.Client{Timeout: timeout})
	defer client.Close()
	if requests := slicePayload(job.Payload, "externalGenerationRequests"); len(requests) > 0 {
		return executeExternalGenerationBatch(ctx, client, provider.ID, toolName, requests)
	}
	args := mapPayload(job.Payload, "arguments")
	callResult, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return nil, err
	}
	return &Result{Output: map[string]interface{}{
		"providerId":        provider.ID,
		"toolName":          toolName,
		"content":           callResult.Content,
		"structuredContent": callResult.StructuredContent,
		"isError":           callResult.IsError,
		"error":             mcpErrorText(callResult),
	}}, nil
}

func executeExternalGenerationBatch(ctx context.Context, client *localmcp.Client, providerID string, toolName string, requests []interface{}) (*Result, error) {
	packages := make([]interface{}, 0, len(requests))
	results := make([]interface{}, 0, len(requests))
	remaining := make([]interface{}, 0)
	for _, item := range requests {
		request := mcpMapFromInterface(item)
		args := mcpArgumentsFromExternalRequest(request)
		callResult, err := client.CallTool(ctx, toolName, args)
		result := map[string]interface{}{
			"requestId":  request["requestId"],
			"shotId":     request["shotId"],
			"providerId": providerID,
			"toolName":   toolName,
		}
		if err != nil {
			result["status"] = "failed"
			result["error"] = err.Error()
			failed := copyMap(request)
			failed["status"] = "failed"
			failed["error"] = err.Error()
			remaining = append(remaining, failed)
			results = append(results, result)
			continue
		}
		result["status"] = "submitted"
		result["content"] = callResult.Content
		result["structuredContent"] = callResult.StructuredContent
		result["isError"] = callResult.IsError
		if callResult.IsError {
			errorText := mcpErrorText(callResult)
			result["status"] = "failed"
			result["error"] = errorText
			failed := copyMap(request)
			failed["status"] = "failed"
			failed["error"] = errorText
			failed["mcpResult"] = map[string]interface{}{
				"content":           callResult.Content,
				"structuredContent": callResult.StructuredContent,
				"isError":           callResult.IsError,
				"error":             errorText,
			}
			remaining = append(remaining, failed)
		} else {
			packages = append(packages, shotAssetPackageFromMCPResult(providerID, request, callResult.StructuredContent))
		}
		results = append(results, result)
	}
	return &Result{Output: map[string]interface{}{
		"providerId":                 providerID,
		"toolName":                   toolName,
		"shotAssetPackages":          packages,
		"generationResults":          results,
		"externalGenerationRequests": remaining,
	}}, nil
}

func mcpArgumentsFromExternalRequest(request map[string]interface{}) map[string]interface{} {
	target := mcpMapFromInterface(request["target"])
	args := map[string]interface{}{
		"prompt": mcpStringFromMap(request, "prompt", "promptText", "videoPrompt"),
	}
	if duration := mcpIntFromInterface(mcpFirstPresent(target, "durationSec", "duration")); duration > 0 {
		args["duration"] = duration
	}
	if ratio := mcpStringFromMap(target, "aspectRatio", "ratio"); ratio != "" {
		args["ratio"] = ratio
	}
	if resolution := mcpStringFromMap(target, "videoResolution", "video_resolution", "resolution"); resolution != "" {
		args["video_resolution"] = normalizeMCPVideoResolution(resolution)
	}
	if model := mcpStringFromMap(target, "modelVersion", "model_version"); model != "" {
		args["model_version"] = model
	}
	return args
}

func normalizeMCPVideoResolution(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "*", "x")
	switch normalized {
	case "3840x2160", "2160p", "4k", "uhd":
		return "4k"
	case "1920x1080", "1080p", "fullhd", "fhd":
		return "1080p"
	case "1280x720", "720p", "hd":
		return "720p"
	default:
		return strings.TrimSpace(value)
	}
}

func mcpErrorText(result *localmcp.ToolCallResult) string {
	if result == nil || !result.IsError {
		return ""
	}
	if text := strings.TrimSpace(mcpContentText(result.Content)); text != "" {
		return text
	}
	if message := mcpStringFromMap(result.StructuredContent, "error", "message", "fail_reason", "failReason"); message != "" {
		return message
	}
	return "mcp tool returned isError=true"
}

func mcpContentText(content []localmcp.ToolContent) string {
	var parts []string
	for _, item := range content {
		if text := strings.TrimSpace(item.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func shotAssetPackageFromMCPResult(providerID string, request map[string]interface{}, structured map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"shotId":          request["shotId"],
		"requestId":       request["requestId"],
		"referenceImages": request["references"],
		"prompts": map[string]interface{}{
			"videoPrompt":    mcpStringFromMap(request, "prompt", "promptText", "videoPrompt"),
			"negativePrompt": mcpStringFromMap(request, "negativePrompt"),
		},
		"aigcVideo": map[string]interface{}{
			"requestId": request["requestId"],
			"submitId":  mcpFirstPresent(structured, "submit_id", "submitId"),
			"genStatus": mcpFirstPresent(structured, "gen_status", "genStatus"),
			"provider":  providerID,
			"raw":       structured,
		},
	}
}

func findProvider(providers []localmcp.ProviderConfig, providerID string) (localmcp.ProviderConfig, bool) {
	for _, provider := range providers {
		if strings.EqualFold(provider.ID, providerID) && provider.Enabled {
			return provider, true
		}
	}
	return localmcp.ProviderConfig{}, false
}

func stringPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func mapPayload(payload map[string]interface{}, key string) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	if value, ok := payload[key].(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func slicePayload(payload map[string]interface{}, key string) []interface{} {
	if payload == nil {
		return nil
	}
	if value, ok := payload[key].([]interface{}); ok {
		return value
	}
	return nil
}

func mcpMapFromInterface(value interface{}) map[string]interface{} {
	if value, ok := value.(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func copyMap(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input)+2)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mcpStringFromMap(input map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mcpFirstPresent(input map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			return value
		}
	}
	return nil
}

func mcpIntFromInterface(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}
