package repository

import (
	"encoding/json"
	"fmt"
	"strings"
)

const redactedUserAssetMarker = "USER_ASSET_REDACTED"

var sensitiveOutputKeys = map[string]bool{
	"audiopackage":       true,
	"body":               true,
	"content":            true,
	"description":        true,
	"imagerequests":      true,
	"markdown":           true,
	"prompt":             true,
	"publishcopy":        true,
	"script":             true,
	"text":               true,
	"transcript":         true,
	"videoimportpackage": true,
	"videoprompt":        true,
	"visualprompt":       true,
}

// SanitizeOutputForPersistence removes user-owned payloads before node/task
// output is persisted in cloud PostgreSQL. The workflow should exchange local
// artifact manifests, not generated scripts, prompts, images, audio, or video.
func SanitizeOutputForPersistence(output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return nil
	}
	cleaned := make(map[string]interface{}, len(output))
	for key, value := range output {
		cleaned[key] = sanitizeOutputValue(key, value)
	}
	return cleaned
}

func sanitizeOutputValue(key string, value interface{}) interface{} {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if lowerKey == "stdout" {
		if stdout, ok := value.(string); ok {
			return sanitizeStdout(stdout)
		}
	}
	if isSensitiveOutputKey(lowerKey) {
		return redactedUserAsset(lowerKey, value)
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		cleaned := make(map[string]interface{}, len(typed))
		for childKey, childValue := range typed {
			cleaned[childKey] = sanitizeOutputValue(childKey, childValue)
		}
		return cleaned
	case []interface{}:
		cleaned := make([]interface{}, len(typed))
		for i, item := range typed {
			cleaned[i] = sanitizeOutputValue("", item)
		}
		return cleaned
	case string:
		if shouldRedactString(typed) {
			return redactedUserAsset(lowerKey, typed)
		}
		return typed
	default:
		return typed
	}
}

func sanitizeStdout(stdout string) string {
	if strings.TrimSpace(stdout) == "" {
		return stdout
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		if shouldRedactString(stdout) {
			return fmt.Sprintf("[%s stdout]", redactedUserAssetMarker)
		}
		return stdout
	}
	cleaned := SanitizeOutputForPersistence(parsed)
	data, err := json.Marshal(cleaned)
	if err != nil {
		return fmt.Sprintf("[%s stdout]", redactedUserAssetMarker)
	}
	return string(data)
}

func isSensitiveOutputKey(lowerKey string) bool {
	return sensitiveOutputKeys[lowerKey]
}

func shouldRedactString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.HasPrefix(trimmed, "local://") {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "blob:") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		len(trimmed) > 512
}

func redactedUserAsset(key string, value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"redacted": true,
		"reason":   redactedUserAssetMarker,
		"field":    key,
		"summary":  redactedSummary(value),
	}
}

func redactedSummary(value interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	switch typed := value.(type) {
	case string:
		summary["type"] = "string"
		summary["bytes"] = len([]byte(typed))
	case []interface{}:
		summary["type"] = "array"
		summary["count"] = len(typed)
	case map[string]interface{}:
		summary["type"] = "object"
		summary["keys"] = len(typed)
	default:
		summary["type"] = fmt.Sprintf("%T", value)
	}
	return summary
}
