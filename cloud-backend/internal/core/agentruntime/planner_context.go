package agentruntime

import (
	"encoding/json"
	"fmt"
	"strings"
)

const plannerContextRedaction = "[REDACTED]"

// sanitizedPlannerContext keeps the complete JSON-compatible planning context
// while ensuring model-provider credentials and nested secrets never become
// part of a downstream LLM prompt. The JSON round trip both validates and
// returns a detached copy of caller-owned data.
func sanitizedPlannerContext(input map[string]interface{}) (map[string]interface{}, error) {
	if input == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("planner context is not JSON-compatible: %w", err)
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("clone planner context: %w", err)
	}
	for key := range cloned {
		if isSensitiveModelProviderContextKey(key) {
			delete(cloned, key)
		}
	}
	redactPlannerSecrets(cloned)
	return cloned, nil
}

func redactPlannerSecrets(value interface{}) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, nested := range typed {
			if isPlannerSecretKey(key) {
				typed[key] = plannerContextRedaction
				continue
			}
			redactPlannerSecrets(nested)
		}
	case []interface{}:
		for _, nested := range typed {
			redactPlannerSecrets(nested)
		}
	}
}

func isPlannerSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
	normalized = replacer.Replace(normalized)
	for _, marker := range []string{"apikey", "token", "authorization", "cookie", "password", "secret", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
