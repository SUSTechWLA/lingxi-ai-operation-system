package artifact

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

// BuildArtifactRequestsFromNode converts a successful workflow node output into
// displayable artifact versions from a local artifact manifest. User payloads
// must stay in the local agent; the cloud only receives storageRef/hash/size.
func BuildArtifactRequestsFromNode(projectID, workflowRunID string, node *model.Node) []*CreateArtifactRequest {
	if node == nil || node.Status != model.NodeSuccess {
		return nil
	}

	payload := parseNodeOutputPayload(node.Output)
	if len(payload) == 0 {
		return nil
	}

	stage := stageNameFromNode(node)
	return buildRequestsFromArtifactManifest(projectID, workflowRunID, stage, payload["artifacts"])
}

func BuildRevisionRequest(base *Artifact, instruction string, data []byte) *CreateArtifactRequest {
	contentHash := HashContent([]byte(base.ID + "\n" + base.StorageRef + "\n" + instruction))
	metadata := map[string]interface{}{
		"revisionInstruction": instruction,
		"revisionOf":          base.ID,
		"previousStorageRef":  base.StorageRef,
	}
	for k, v := range base.Metadata {
		metadata[k] = v
	}
	return &CreateArtifactRequest{
		ProjectID:     base.ProjectID,
		WorkflowRunID: base.WorkflowRunID,
		StageName:     base.StageName,
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

func buildRequestsFromArtifactManifest(projectID, workflowRunID, stage string, manifest interface{}) []*CreateArtifactRequest {
	items, ok := manifest.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	requests := make([]*CreateArtifactRequest, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		unitID := stringValue(entry, "unitId")
		if unitID == "" {
			unitID = stringValue(entry, "unitID")
		}
		kind := parseArtifactKind(stringValue(entry, "kind"))
		name := stringValue(entry, "name")
		mime := stringValue(entry, "mimeType")
		contentHash := stringValue(entry, "contentHash")
		storageRef := stringValue(entry, "storageRef")
		sizeBytes := int64Value(entry["sizeBytes"])
		if unitID == "" || kind == "" {
			continue
		}
		if storageRef == "" {
			storageRef = LocalArtifactRef(projectID, stage, unitID, contentHash, name)
		}
		requests = append(requests, buildLocalManifestRequest(projectID, workflowRunID, stage, unitID, kind, name, mime, storageRef, contentHash, sizeBytes, metadataValue(entry["metadata"])))
	}
	return requests
}

func buildLocalManifestRequest(projectID, workflowRunID, stage, unitID string, kind ArtifactKind, name, mime, storageRef, contentHash string, sizeBytes int64, metadata map[string]interface{}) *CreateArtifactRequest {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["displayable"] = true
	metadata["cloudPayloadStored"] = false
	return &CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		StageName:     stage,
		UnitID:        unitID,
		Kind:          kind,
		Name:          name,
		StorageType:   StorageLocal,
		StorageRef:    storageRef,
		MimeType:      mime,
		SizeBytes:     sizeBytes,
		ContentHash:   contentHash,
		Provider:      "workflow-node",
		Model:         "artifact-materializer",
		Metadata:      metadata,
	}
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
		return ""
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
