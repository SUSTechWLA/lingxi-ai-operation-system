package agentruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ToolSnapshot is the immutable, content-addressed identity of the normalized
// tool registry used by one run. CreatedAt is metadata and is deliberately not
// included in CanonicalJSON or ID derivation.
type ToolSnapshot struct {
	ID            string          `json:"id"`
	SHA256        string          `json:"sha256"`
	CanonicalJSON json.RawMessage `json:"canonicalJson"`
	CreatedAt     time.Time       `json:"createdAt"`
}

const runManifestSchemaVersion = "1"

// RunManifest is deliberately limited to bounded execution metadata. Prompts,
// request context, tool arguments, transports, headers, and credentials do not
// belong in this persistence contract.
type RunManifest struct {
	SchemaVersion          string                     `json:"schemaVersion"`
	Runtime                string                     `json:"runtime"`
	RunID                  string                     `json:"runId"`
	TraceID                string                     `json:"traceId,omitempty"`
	ToolRegistrySnapshotID string                     `json:"toolRegistrySnapshotId"`
	ToolRegistrySHA256     string                     `json:"toolRegistrySha256"`
	ParentRunID            *string                    `json:"parentRunId,omitempty"`
	ReplayFromStageID      *string                    `json:"replayFromStageId,omitempty"`
	MCPRunnerRevisions     []RequestMCPRunnerRevision `json:"mcpRunnerRevisions,omitempty"`
	CreatedAt              time.Time                  `json:"createdAt"`
}

// BuildToolSnapshot returns a deterministic snapshot without modifying the
// supplied manifests. Tool order and every string-only list are
// order-insensitive. JSON object keys are emitted in lexical order by
// encoding/json. Typed nil and empty manifest collections collapse where the
// manifest JSON contract already treats them equivalently; explicit nulls in
// arbitrary JSON schema values remain meaningful and are preserved.
func BuildToolSnapshot(manifests []*tool.ToolManifest) (ToolSnapshot, error) {
	normalized := make([]map[string]interface{}, 0, len(manifests))
	seen := make(map[string]struct{}, len(manifests))
	for index, manifest := range manifests {
		if manifest == nil {
			return ToolSnapshot{}, fmt.Errorf("tool manifest at index %d is nil", index)
		}
		name := strings.TrimSpace(manifest.Name)
		if name == "" {
			return ToolSnapshot{}, fmt.Errorf("tool manifest at index %d has an empty logical tool name", index)
		}
		if _, exists := seen[name]; exists {
			return ToolSnapshot{}, fmt.Errorf("duplicate logical tool name %q", name)
		}
		seen[name] = struct{}{}

		clone, err := cloneManifestForSnapshot(manifest)
		if err != nil {
			return ToolSnapshot{}, fmt.Errorf("normalize tool manifest %q: %w", name, err)
		}
		clone.Name = name
		clone.RegisteredAt = time.Time{}
		if clone.Parameters == nil {
			clone.Parameters = map[string]tool.ParamDef{}
		}
		if clone.Output == nil {
			clone.Output = map[string]tool.ParamDef{}
		}
		for i := range clone.Examples {
			if clone.Examples[i].Input == nil {
				clone.Examples[i].Input = map[string]interface{}{}
			}
			if clone.Examples[i].Output == nil {
				clone.Examples[i].Output = map[string]interface{}{}
			}
		}

		wire, err := json.Marshal(clone)
		if err != nil {
			return ToolSnapshot{}, fmt.Errorf("marshal tool manifest %q: %w", name, err)
		}
		var value map[string]interface{}
		if err := json.Unmarshal(wire, &value); err != nil {
			return ToolSnapshot{}, fmt.Errorf("decode tool manifest %q: %w", name, err)
		}
		normalized = append(normalized, canonicalizeSnapshotMap(value))
	}

	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i]["name"].(string) < normalized[j]["name"].(string)
	})
	canonicalJSON, err := json.Marshal(normalized)
	if err != nil {
		return ToolSnapshot{}, fmt.Errorf("marshal canonical tool registry: %w", err)
	}
	digest := sha256.Sum256(canonicalJSON)
	hash := hex.EncodeToString(digest[:])
	return ToolSnapshot{
		ID:            "tool_snapshot_" + hash,
		SHA256:        hash,
		CanonicalJSON: append(json.RawMessage(nil), canonicalJSON...),
		CreatedAt:     time.Now().UTC(),
	}, nil
}

func cloneManifestForSnapshot(manifest *tool.ToolManifest) (*tool.ToolManifest, error) {
	wire, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var clone tool.ToolManifest
	if err := json.Unmarshal(wire, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func canonicalizeSnapshotMap(value map[string]interface{}) map[string]interface{} {
	for key, child := range value {
		value[key] = canonicalizeSnapshotValue(child)
	}
	return value
}

func canonicalizeSnapshotValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return canonicalizeSnapshotMap(typed)
	case []interface{}:
		allStrings := true
		stringsOnly := make([]string, len(typed))
		for i, child := range typed {
			typed[i] = canonicalizeSnapshotValue(child)
			stringValue, ok := typed[i].(string)
			if !ok {
				allStrings = false
				continue
			}
			stringsOnly[i] = stringValue
		}
		if allStrings {
			sort.Strings(stringsOnly)
			for i := range stringsOnly {
				typed[i] = stringsOnly[i]
			}
		}
		return typed
	default:
		return value
	}
}
