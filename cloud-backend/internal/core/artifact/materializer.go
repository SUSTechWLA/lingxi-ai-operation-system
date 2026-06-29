package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

const ArtifactManifestInvalidCode = "ARTIFACT_MANIFEST_INVALID"

type ArtifactManifestInvalidError struct {
	Message string
}

func (e *ArtifactManifestInvalidError) Error() string {
	return ArtifactManifestInvalidCode + ": " + e.Message
}

func IsArtifactManifestInvalid(err error) bool {
	var manifestErr *ArtifactManifestInvalidError
	return errors.As(err, &manifestErr)
}

// BuildArtifactRequestsFromNode converts a successful workflow node output into
// displayable artifact versions from a local artifact manifest. User payloads
// must stay in the local agent; the cloud only receives storageRef/hash/size.
func BuildArtifactRequestsFromNode(projectID, workflowRunID string, node *model.Node) []*CreateArtifactRequest {
	requests, err := BuildArtifactRequestsFromNodeChecked(projectID, workflowRunID, node)
	if err != nil {
		return []*CreateArtifactRequest{}
	}
	return requests
}

// BuildArtifactRequestsFromNodeChecked is the strict materializer used by
// production paths. When a node declares artifacts[], every entry must include
// the required manifest keys; invalid manifests must not be silently skipped.
func BuildArtifactRequestsFromNodeChecked(projectID, workflowRunID string, node *model.Node) ([]*CreateArtifactRequest, error) {
	if node == nil || node.Status != model.NodeSuccess {
		return nil, nil
	}

	payload := parseNodeOutputPayload(node.Output)
	if len(payload) == 0 {
		return nil, nil
	}

	stage := stageNameFromNode(node)
	return buildRequestsFromArtifactManifest(projectID, workflowRunID, stage, payload["artifacts"], payload, node)
}

func BuildRevisionRequest(base *Artifact, instruction string, data []byte) *CreateArtifactRequest {
	contentHash := HashContent([]byte(base.ID + "\n" + base.StorageRef + "\n" + instruction))
	metadata := map[string]interface{}{
		"revisionInstruction": instruction,
		"revisionOf":          base.ID,
		"previousStorageRef":  base.StorageRef,
		"status":              "valid",
		"humanApproved":       false,
	}
	for k, v := range base.Metadata {
		metadata[k] = v
	}
	metadata["status"] = "valid"
	metadata["humanApproved"] = false
	return &CreateArtifactRequest{
		ProjectID:     base.ProjectID,
		WorkflowRunID: base.WorkflowRunID,
		TaskID:        base.TaskID,
		StageName:     base.StageName,
		RoleAgentID:   base.RoleAgentID,
		UnitID:        base.UnitID,
		Kind:          base.Kind,
		Name:          base.Name,
		StorageType:   StorageLocal,
		StorageRef:    LocalArtifactRef(base.ProjectID, base.StageName, base.UnitID, contentHash, base.Name),
		MimeType:      base.MimeType,
		ContentHash:   contentHash,
		Provider:      "artifact-revision",
		Model:         "local",
		Metadata:      metadata,
	}
}

func parseNodeOutputPayload(output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return nil
	}
	if stdout, ok := output["stdout"].(string); ok && strings.TrimSpace(stdout) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
			return parsed
		}
	}
	return output
}

func stageNameFromNode(node *model.Node) string {
	if stage, ok := node.Input["stage"].(string); ok && stage != "" {
		return stage
	}
	if params, ok := node.Input["parameters"].(map[string]interface{}); ok {
		if stage, ok := params["stage"].(string); ok && stage != "" {
			return stage
		}
	}
	stage := strings.TrimSuffix(node.ID, "_exec")
	if stage == "" {
		return "artifact"
	}
	return stage
}

func buildRequestsFromArtifactManifest(projectID, workflowRunID, stage string, manifest interface{}, payload map[string]interface{}, node *model.Node) ([]*CreateArtifactRequest, error) {
	items, ok := manifest.([]interface{})
	if !ok {
		// Handle single-map format (legacy compatibility with tools that
		// output a flat artifacts object instead of an array).
		if singleItem, ok2 := manifest.(map[string]interface{}); ok2 {
			items = []interface{}{singleItem}
		} else {
			return nil, nil
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	requests := make([]*CreateArtifactRequest, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return nil, &ArtifactManifestInvalidError{Message: "artifact entry must be an object"}
		}
		unitID := stringValue(entry, "unitId")
		if unitID == "" {
			unitID = stringValue(entry, "unitID")
		}
		if unitID == "" {
			return nil, &ArtifactManifestInvalidError{Message: "artifact unitId is required"}
		}
		kindValue := stringValue(entry, "kind")
		if kindValue == "" {
			return nil, &ArtifactManifestInvalidError{Message: "artifact kind is required"}
		}
		kind := parseArtifactKind(kindValue)
		if kind == "" {
			return nil, &ArtifactManifestInvalidError{Message: "artifact kind is unsupported: " + kindValue}
		}
		name := stringValue(entry, "name")
		mime := stringValue(entry, "mimeType")
		contentHash := stringValue(entry, "contentHash")
		storageRef := stringValue(entry, "storageRef")
		sizeBytes := int64Value(entry["sizeBytes"])

		// Extract inline content from the tool payload so it can be displayed
		// before the local backend syncs it. This is a materialization-time
		// convenience; the authoritative copy lives on the local agent.
		data := extractArtifactContent(payload, unitID, kind)
		if unitID == "publish-copy" && len(data) == 0 {
			continue
		}

		// Generate a deterministic content hash when none is provided, so the
		// idempotency check in CreateArtifact prevents duplicate versions on
		// every poll.
		if contentHash == "" {
			contentHash = hashArtifactEntry(entry, data)
		}
		if sizeBytes == 0 && len(data) > 0 {
			sizeBytes = int64(len(data))
		}
		if storageRef == "" {
			storageRef = LocalArtifactRef(projectID, stage, unitID, contentHash, name)
		}
		requests = append(requests, buildLocalManifestRequest(projectID, workflowRunID, stage, unitID, kind, name, mime, storageRef, contentHash, sizeBytes, betaArtifactMetadata(metadataValue(entry["metadata"]), node), data))
	}
	return requests, nil
}

func buildLocalManifestRequest(projectID, workflowRunID, stage, unitID string, kind ArtifactKind, name, mime, storageRef, contentHash string, sizeBytes int64, metadata map[string]interface{}, data []byte) *CreateArtifactRequest {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["displayable"] = true
	metadata["cloudPayloadStored"] = len(data) > 0
	return &CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		TaskID:        stringValue(metadata, "taskId"),
		StageName:     stage,
		RoleAgentID:   stringValue(metadata, "roleAgentId"),
		UnitID:        unitID,
		Kind:          kind,
		Name:          name,
		StorageType:   StorageLocal,
		StorageRef:    storageRef,
		MimeType:      mime,
		SizeBytes:     sizeBytes,
		ContentHash:   contentHash,
		Data:          data,
		Provider:      "workflow-node",
		Model:         "artifact-materializer",
		Metadata:      metadata,
	}
}

func betaArtifactMetadata(metadata map[string]interface{}, node *model.Node) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if _, ok := metadata["status"]; !ok {
		metadata["status"] = "valid"
	}
	if _, ok := metadata["humanApproved"]; !ok {
		metadata["humanApproved"] = false
	}
	if node == nil {
		return metadata
	}
	metadata["taskId"] = node.TaskID
	metadata["producedByNode"] = node.ID
	if toolName, ok := node.Input["tool"].(string); ok && toolName != "" {
		metadata["producedByTool"] = toolName
	} else if node.Name != "" {
		metadata["producedByTool"] = node.Name
	} else {
		metadata["producedByTool"] = "external"
	}
	if roleAgentID := nodeInputString(node, "roleAgentId"); roleAgentID != "" {
		metadata["roleAgentId"] = roleAgentID
	}
	if roleAgent := nodeInputMap(node, "roleAgent"); roleAgent != nil {
		if displayName, ok := roleAgent["displayName"].(string); ok {
			metadata["producedByRole"] = displayName
		}
	}
	if dependsOn := stringSliceFromInterface(nodeInputValue(node, "requiredInputs")); len(dependsOn) > 0 {
		metadata["dependsOn"] = dependsOn
	}
	return metadata
}

func nodeInputValue(node *model.Node, key string) interface{} {
	if node == nil || node.Input == nil || key == "" {
		return nil
	}
	if value, ok := node.Input[key]; ok {
		return value
	}
	if params, ok := node.Input["parameters"].(map[string]interface{}); ok {
		return params[key]
	}
	return nil
}

func nodeInputString(node *model.Node, key string) string {
	value, _ := nodeInputValue(node, key).(string)
	return strings.TrimSpace(value)
}

func nodeInputMap(node *model.Node, key string) map[string]interface{} {
	value, _ := nodeInputValue(node, key).(map[string]interface{})
	return value
}

// extractArtifactContent pulls inline content from the tool output payload for
// the given artifact kind + unitId. This lets the artifact content endpoint
// return real content before the local backend has synced it.
func extractArtifactContent(payload map[string]interface{}, unitID string, kind ArtifactKind) []byte {
	if payload == nil {
		return nil
	}
	switch unitID {
	case "script-content":
		// Prefer the parsed script; the raw "content" field often contains
		// the full LLM JSON response, not human-readable markdown.
		if script, ok := payload["script"].(string); ok && strings.TrimSpace(script) != "" {
			return []byte(script)
		}
		if content, ok := payload["content"].(string); ok {
			return []byte(content)
		}
	case "composition":
		if spec, ok := payload["compositionSpec"]; ok && spec != nil {
			return marshalValue(spec)
		}
	case "publish-copy":
		if pc, ok := payload["publishCopy"].(map[string]interface{}); ok {
			return normalizePublishCopy(pc)
		}
		if pkg, ok := payload["package"].(map[string]interface{}); ok {
			return normalizePublishCopy(pkg)
		}
		// Build from individual fields
		pubData := map[string]interface{}{}
		if t, ok := payload["title"]; ok && t != nil {
			pubData["title"] = t
		}
		if d, ok := payload["description"]; ok && d != nil {
			pubData["description"] = d
		}
		if kw, ok := payload["keywords"]; ok && kw != nil {
			pubData["keywords"] = kw
		} else if tags, ok := payload["tags"]; ok && tags != nil {
			pubData["keywords"] = tags
		}
		if len(pubData) > 0 {
			return normalizePublishCopy(pubData)
		}
	default:
		// Stage-named markdown artifacts: use parsed script or content field.
		if script, ok := payload["script"].(string); ok && strings.TrimSpace(script) != "" && kind == KindMarkdown {
			return []byte(script)
		}
		if content, ok := payload["content"].(string); ok && kind == KindMarkdown {
			return []byte(content)
		}
		// Generic JSON artifacts: serialize the whole payload.
		if isStructuredJSONArtifactKind(kind) && len(payload) > 0 {
			return marshalValue(payload)
		}
	}
	return nil
}

func isStructuredJSONArtifactKind(kind ArtifactKind) bool {
	if kind == KindJSON {
		return true
	}
	switch strings.ToUpper(string(kind)) {
	case "VIDEO_PROPOSAL",
		"CARD_PLAN",
		"CAPTION_PLAN",
		"SHOT_LIST",
		"VIDEO_COMPOSITION_SPEC",
		"REFERENCE_ASSET_PLAN",
		"STYLE_PROFILE",
		"CONTINUITY_REPORT",
		"PREVIEW_REPORT",
		"RENDER_REPORT",
		"FINAL_REVIEW",
		"PROJECT_PACKAGE":
		return true
	default:
		return false
	}
}

// normalizePublishCopy ensures publish-copy content has the keys the frontend
// expects (title, description, keywords). Input may use "tags" or "keywords".
// When standard fields are missing, additional source fields are preserved so
// the frontend's ReadableArtifact fallback can still render useful content.
func normalizePublishCopy(src map[string]interface{}) []byte {
	out := map[string]interface{}{}
	if t, ok := src["title"]; ok {
		if text := ensureString(t); strings.TrimSpace(text) != "" {
			out["title"] = text
		}
	}
	if d, ok := src["description"]; ok {
		if text := ensureString(d); strings.TrimSpace(text) != "" {
			out["description"] = text
		}
	}
	if kw, ok := src["keywords"]; ok {
		if normalized := normalizeKeywordValue(kw); len(normalized) > 0 {
			out["keywords"] = normalized
		}
	} else if tags, ok := src["tags"]; ok {
		if normalized := normalizeKeywordValue(tags); len(normalized) > 0 {
			out["keywords"] = normalized
		}
	}

	if len(out) == 0 {
		return nil
	}

	extraKeys := []string{
		"negativePrompt", "videoPrompt", "prompt", "referenceFrames",
		"script", "narration", "voiceover", "transcript",
	}
	for _, k := range extraKeys {
		if v, ok := src[k]; ok && hasReadableValue(v) {
			out[k] = v
		}
	}

	return marshalValue(out)
}

func normalizeKeywordValue(value interface{}) []string {
	keywords := make([]string, 0)
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				keywords = append(keywords, trimmed)
			}
		}
	case []interface{}:
		for _, item := range typed {
			switch v := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					keywords = append(keywords, trimmed)
				}
			case map[string]interface{}:
				if text := firstStringField(v, "keyword", "tag", "label", "name", "text", "value"); text != "" {
					keywords = append(keywords, text)
				}
			default:
				if text := strings.TrimSpace(ensureString(v)); text != "" {
					keywords = append(keywords, text)
				}
			}
		}
	case string:
		for _, item := range strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '\n' || r == '\t' || r == ' '
		}) {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				keywords = append(keywords, trimmed)
			}
		}
	case map[string]interface{}:
		if text := firstStringField(typed, "keyword", "tag", "label", "name", "text", "value"); text != "" {
			keywords = append(keywords, text)
		}
	}
	return keywords
}

func firstStringField(record map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if text, ok := record[key].(string); ok {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func hasReadableValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(normalizeKeywordValue(typed)) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
}

// ensureString converts any value to a string representation, preventing
// nested objects from leaking to the frontend where String(obj) would render
// as "[object Object]".
func ensureString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// hashArtifactEntry produces a deterministic content hash from an artifact
// manifest entry and its inline data. When tools don't provide an explicit hash,
// this ensures the idempotency check in CreateArtifact works correctly.
func hashArtifactEntry(entry map[string]interface{}, data []byte) string {
	payload := append(marshalValue(entry), data...)
	return HashContent(payload)
}

func parseArtifactKind(value string) ArtifactKind {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(KindJSON):
		return KindJSON
	case string(KindMarkdown):
		return KindMarkdown
	case string(KindImage):
		return KindImage
	case string(KindAudio):
		return KindAudio
	case string(KindVideo):
		return KindVideo
	case string(KindBundle):
		return KindBundle
	case string(KindLog):
		return KindLog
	default:
		return ArtifactKind(strings.ToUpper(strings.TrimSpace(value)))
	}
}

func stringValue(entry map[string]interface{}, key string) string {
	value, _ := entry[key].(string)
	return strings.TrimSpace(value)
}

func int64Value(value interface{}) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		n, _ := typed.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return n
	default:
		return 0
	}
}

func metadataValue(value interface{}) map[string]interface{} {
	metadata, _ := value.(map[string]interface{})
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(metadata))
	for key, item := range metadata {
		cloned[key] = item
	}
	return cloned
}

func stringSliceFromInterface(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	default:
		return nil
	}
}

func marshalValue(value interface{}) []byte {
	switch typed := value.(type) {
	case string:
		return []byte(typed)
	case nil:
		return nil
	default:
		data, err := json.MarshalIndent(typed, "", "  ")
		if err != nil {
			return []byte(fmt.Sprintf("%v", typed))
		}
		return data
	}
}
