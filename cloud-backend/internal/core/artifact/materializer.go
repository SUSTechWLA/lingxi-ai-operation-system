package artifact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

// BuildArtifactRequestsFromNode converts a successful workflow node output into
// displayable artifact versions. It understands the common video-creation output
// contract used by local and external tools: content, imageRequests,
// videoImportPackage, and audioPackage.
func BuildArtifactRequestsFromNode(projectID, workflowRunID string, node *model.Node) []*CreateArtifactRequest {
	if node == nil || node.Status != model.NodeSuccess {
		return nil
	}

	payload := parseNodeOutputPayload(node.Output)
	if len(payload) == 0 {
		return nil
	}

	stage := stageNameFromNode(node)
	requests := make([]*CreateArtifactRequest, 0, 5)

	if content, ok := payload["content"].(string); ok && strings.TrimSpace(content) != "" {
		requests = append(requests, buildInlineRequest(projectID, workflowRunID, stage, "content", KindMarkdown, stage+".md", "text/markdown; charset=utf-8", []byte(content), nil))
	}
	if value, ok := payload["imageRequests"]; ok {
		if data := marshalValue(value); len(data) > 0 {
			requests = append(requests, buildInlineRequest(projectID, workflowRunID, stage, "image-requests", KindImage, stage+" images", "application/json", data, nil))
		}
	}
	if value, ok := payload["videoImportPackage"]; ok {
		if data := marshalValue(value); len(data) > 0 {
			requests = append(requests, buildInlineRequest(projectID, workflowRunID, stage, "video-package", KindVideo, stage+" video package", "application/json", data, nil))
		}
	}
	if value, ok := payload["audioPackage"]; ok {
		if data := marshalValue(value); len(data) > 0 {
			requests = append(requests, buildInlineRequest(projectID, workflowRunID, stage, "audio-package", KindAudio, stage+" audio package", "application/json", data, nil))
		}
	}
	if value, ok := payload["publishCopy"]; ok {
		if data := marshalValue(value); len(data) > 0 {
			requests = append(requests, buildInlineRequest(projectID, workflowRunID, stage, "publish-copy", KindJSON, stage+" publish copy", "application/json", data, nil))
		}
	}

	return requests
}

func BuildRevisionRequest(base *Artifact, instruction string, data []byte) *CreateArtifactRequest {
	metadata := map[string]interface{}{
		"revisionInstruction": instruction,
		"revisionOf":          base.ID,
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
		StorageType:   "inline",
		Data:          data,
		MimeType:      base.MimeType,
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

func buildInlineRequest(projectID, workflowRunID, stage, unitID string, kind ArtifactKind, name, mime string, data []byte, metadata map[string]interface{}) *CreateArtifactRequest {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["displayable"] = true
	return &CreateArtifactRequest{
		ProjectID:     projectID,
		WorkflowRunID: workflowRunID,
		StageName:     stage,
		UnitID:        unitID,
		Kind:          kind,
		Name:          name,
		StorageType:   "inline",
		Data:          data,
		MimeType:      mime,
		Provider:      "workflow-node",
		Model:         "artifact-materializer",
		Metadata:      metadata,
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
