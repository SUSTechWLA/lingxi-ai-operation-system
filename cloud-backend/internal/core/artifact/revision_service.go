package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// RevisionGenerator generates either full replacement content or a selected
// replacement fragment, as specified by the user prompt.
type RevisionGenerator func(context.Context, string, string, ReviseLLMOptions) (string, error)

// revisionArtifactStore is the subset of artifact operations shared by legacy
// HTTP handlers and creator-step callers.
type revisionArtifactStore interface {
	GetByID(context.Context, string) (*Artifact, error)
	GetCurrent(context.Context, string, string, string) (*Artifact, error)
	CreateArtifact(context.Context, *CreateArtifactRequest) (*Artifact, error)
	MarkDownstreamStale(context.Context, string, string, string) ([]string, error)
}

// RevisionResult is returned by revise and restore operations.
type RevisionResult struct {
	Artifact        *Artifact `json:"artifact"`
	StaleStageNames []string  `json:"staleStageNames"`
}

type ReviseRequest struct {
	NewArtifactID  string
	ArtifactID     string
	Message        string
	DirectContent  []byte
	ModelProvider  map[string]interface{}
	ModelProviders map[string]interface{}
	// Provenance is immutable structured context supplied by higher-level
	// revision surfaces, such as a normalized rectangle or time selection.
	Provenance map[string]interface{}
}

// ReplacementMaterialIdentity is the canonical metadata-only identity of one
// project-registered local image. The creator service resolves and authorizes
// it before invoking RevisionService.Replace.
type ReplacementMaterialIdentity struct {
	ContentHash string `json:"contentHash"`
	StorageRef  string `json:"storageRef"`
	MimeType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
}

type ReplaceRequest struct {
	NewArtifactID string
	ArtifactID    string
	BaseVersion   int
	Material      ReplacementMaterialIdentity
	Provenance    map[string]interface{}
}

type RestoreRequest struct {
	ArtifactID    string
	NewArtifactID string
	ReviewerID    string
	Reason        string
	Provenance    map[string]interface{}
}

var (
	ErrRevisionArtifactNotFound   = errors.New("revision artifact not found")
	ErrRevisionContentUnavailable = errors.New("revision source content unavailable")
	ErrRevisionGeneration         = errors.New("revision generation failed")
	ErrRevisionInvalidReplacement = errors.New("revision replacement is invalid")
)

// RevisionService owns revision and restore orchestration. It never changes
// content on an existing artifact; every successful operation creates a new
// current version through Service.CreateArtifact.
type RevisionService struct {
	artifacts       revisionArtifactStore
	skillRoot       string
	generator       RevisionGenerator
	contentResolver func(context.Context, *Artifact) ([]byte, bool)
}

func NewRevisionService(artifacts revisionArtifactStore) *RevisionService {
	return &RevisionService{artifacts: artifacts}
}

func (s *RevisionService) SetConfig(skillRoot string, generator RevisionGenerator) {
	s.skillRoot = skillRoot
	s.generator = generator
}

// SetContentResolver supplies optional local-agent hydration for artifacts
// whose bytes intentionally remain outside the cloud database.
func (s *RevisionService) SetContentResolver(resolver func(context.Context, *Artifact) ([]byte, bool)) {
	s.contentResolver = resolver
}

func (s *RevisionService) Revise(ctx context.Context, req ReviseRequest) (*RevisionResult, error) {
	base, err := s.artifacts.GetByID(ctx, req.ArtifactID)
	if err != nil || base == nil {
		return nil, fmt.Errorf("%w: %v", ErrRevisionArtifactNotFound, err)
	}

	data := req.DirectContent
	if data == nil {
		originalContent := s.resolveOriginalContent(ctx, base)
		if originalContent == "" {
			return nil, ErrRevisionContentUnavailable
		}
		selection, hasSelection, selectionErr := textSelectionFromProvenance(req.Provenance)
		if selectionErr != nil {
			return nil, selectionErr
		}
		if s.generator != nil {
			systemPrompt := buildRevisionSystemPrompt(base.StageName, s.readStageInstruction(base))
			userPrompt := fmt.Sprintf("原始内容：\n\n%s\n\n---\n\n修改意见：\n%s\n\n请根据修改意见重新生成完整内容，保持原有的格式结构。", originalContent, req.Message)
			if hasSelection {
				userPrompt, err = buildSelectedRevisionUserPrompt(originalContent, selection, req.Message)
				if err != nil {
					return nil, err
				}
			}
			revisedText, err := s.generator(ctx, systemPrompt, userPrompt, ReviseLLMOptions{ModelProvider: textModelProviderFromRevisionRequest(req.ModelProvider, req.ModelProviders)})
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrRevisionGeneration, err)
			}
			if hasSelection {
				spliced, spliceErr := spliceUTF16Selection(originalContent, selection, revisedText)
				if spliceErr != nil {
					return nil, spliceErr
				}
				data = []byte(spliced)
			} else {
				data = []byte(revisedText)
			}
		} else if hasSelection {
			return nil, ErrRevisionGeneration
		} else {
			data = buildLocalRevisionData(base, req.Message)
		}
	}

	revisionRequest := BuildRevisionRequest(base, req.Message, data)
	revisionRequest.ID = req.NewArtifactID
	if revisionRequest.Metadata == nil {
		revisionRequest.Metadata = map[string]interface{}{}
	}
	for key, value := range req.Provenance {
		revisionRequest.Metadata[key] = deepCloneMetadataValue(value)
	}
	// An explicit revision is an auditable user action, even when it happens
	// to produce bytes that hash-identically to an existing version.
	revisionRequest.ForceNewVersion = true
	revision, err := s.artifacts.CreateArtifact(ctx, revisionRequest)
	if err != nil {
		return nil, err
	}
	staleStages := s.markDownstreamStale(ctx, base)
	return &RevisionResult{Artifact: revision, StaleStageNames: staleStages}, nil
}

// Replace creates an immutable child image version that points to already
// uploaded local bytes. It never reads or copies those bytes and never invokes
// the revision generator.
func (s *RevisionService) Replace(ctx context.Context, req ReplaceRequest) (*RevisionResult, error) {
	base, err := s.artifacts.GetByID(ctx, req.ArtifactID)
	if err != nil || base == nil {
		return nil, fmt.Errorf("%w: %v", ErrRevisionArtifactNotFound, err)
	}
	if req.BaseVersion <= 0 || base.Version != req.BaseVersion {
		return nil, ErrArtifactVersionConflict
	}
	material := req.Material
	storageRef := strings.TrimSpace(material.StorageRef)
	mimeType := strings.ToLower(strings.TrimSpace(material.MimeType))
	contentHash := strings.TrimSpace(material.ContentHash)
	if base.Kind != KindImage ||
		storageRef != material.StorageRef || mimeType != material.MimeType || contentHash != material.ContentHash ||
		!strings.HasPrefix(storageRef, "local://") || len(strings.TrimPrefix(storageRef, "local://")) == 0 ||
		!strings.HasPrefix(mimeType, "image/") || len(strings.TrimPrefix(mimeType, "image/")) == 0 ||
		material.SizeBytes < 0 ||
		!strings.HasPrefix(contentHash, "sha256:") ||
		len(strings.TrimPrefix(contentHash, "sha256:")) == 0 {
		return nil, ErrRevisionInvalidReplacement
	}

	metadata := cloneMetadata(base.Metadata)
	// Content identity belongs to the immutable artifact row. Carrying any of
	// these legacy metadata mirrors forward can make readers resolve the base
	// image even though the replacement row points at the new material.
	for _, key := range []string{
		"localPath", "path",
		"storageRef", "storage_ref",
		"contentHash", "content_hash",
		"mimeType", "mime_type",
		"sizeBytes", "size_bytes",
		"mediaUrl", "mediaURL", "mediaUrls", "mediaURLs",
	} {
		delete(metadata, key)
	}
	for key, value := range map[string]string{
		"producedByNode": base.ProducedByNode,
		"producedByTool": base.ProducedByTool,
		"producedByRole": base.ProducedByRole,
	} {
		delete(metadata, key)
		if strings.TrimSpace(value) != "" {
			metadata[key] = value
		}
	}
	metadata["status"] = string(ArtifactStatusValid)
	metadata["humanApproved"] = false
	metadata["replacementOf"] = base.ID
	for key, value := range req.Provenance {
		metadata[key] = deepCloneMetadataValue(value)
	}
	replacement, err := s.artifacts.CreateArtifact(ctx, &CreateArtifactRequest{
		ID: req.NewArtifactID, ProjectID: base.ProjectID, WorkflowRunID: base.WorkflowRunID, TaskID: base.TaskID,
		StageName: base.StageName, RoleAgentID: base.RoleAgentID, UnitID: base.UnitID,
		Kind: base.Kind, Name: base.Name, StorageType: StorageLocal, StorageRef: material.StorageRef,
		MimeType: material.MimeType, SizeBytes: material.SizeBytes, ContentHash: material.ContentHash,
		PromptHash: base.PromptHash, Provider: base.Provider, Model: base.Model, Metadata: metadata,
		ForceNewVersion: true, ExpectedParentID: base.ID, ExpectedParentVersion: req.BaseVersion,
	})
	if err != nil {
		return nil, err
	}
	staleStages := s.markDownstreamStale(ctx, base)
	return &RevisionResult{Artifact: replacement, StaleStageNames: staleStages}, nil
}

// Restore recreates a historical artifact as a new version. The selected
// artifact supplies the immutable bytes and provenance, while CreateArtifact
// resolves the authoritative current version as the new parent.
func (s *RevisionService) Restore(ctx context.Context, req RestoreRequest) (*RevisionResult, error) {
	historical, err := s.artifacts.GetByID(ctx, req.ArtifactID)
	if err != nil || historical == nil {
		return nil, fmt.Errorf("%w: %v", ErrRevisionArtifactNotFound, err)
	}
	current, err := s.artifacts.GetCurrent(ctx, historical.ProjectID, historical.StageName, historical.UnitID)
	if err != nil || current == nil {
		return nil, fmt.Errorf("%w: current artifact for history restore: %v", ErrRevisionArtifactNotFound, err)
	}

	data := []byte(nil)
	if historical.StorageType == StorageInline {
		data = []byte(historical.InlineJSON)
	}
	metadata := cloneMetadata(historical.Metadata)
	for key, value := range req.Provenance {
		metadata[key] = deepCloneMetadataValue(value)
	}
	// A restore selects historical bytes, but it is executed in the current
	// workflow. Never resurrect a stale run/task/node identity.
	currentProducer := map[string]string{
		"producedByNode": current.ProducedByNode, "producedByTool": current.ProducedByTool, "producedByRole": current.ProducedByRole,
	}
	for key, fieldValue := range currentProducer {
		delete(metadata, key)
		if strings.TrimSpace(fieldValue) != "" {
			metadata[key] = fieldValue
			continue
		}
		if value, ok := current.Metadata[key]; ok {
			metadata[key] = deepCloneMetadataValue(value)
		}
	}
	metadata["status"] = string(ArtifactStatusValid)
	metadata["humanApproved"] = false
	if strings.TrimSpace(req.ReviewerID) != "" {
		metadata["restoredByReviewerId"] = req.ReviewerID
	}
	if strings.TrimSpace(req.Reason) != "" {
		metadata["restoreReason"] = req.Reason
	}
	restored, err := s.artifacts.CreateArtifact(ctx, &CreateArtifactRequest{
		ID: req.NewArtifactID, ProjectID: historical.ProjectID, WorkflowRunID: current.WorkflowRunID, TaskID: current.TaskID,
		StageName: historical.StageName, RoleAgentID: current.RoleAgentID, UnitID: historical.UnitID,
		Kind: historical.Kind, Name: historical.Name, StorageType: historical.StorageType, StorageRef: historical.StorageRef,
		Data: data, MimeType: historical.MimeType, SizeBytes: historical.SizeBytes, ContentHash: historical.ContentHash,
		PromptHash: historical.PromptHash, Provider: historical.Provider, Model: historical.Model, Metadata: metadata,
		ForceNewVersion: true, RestoredFromID: historical.ID,
	})
	if err != nil {
		return nil, err
	}
	staleStages := s.markDownstreamStale(ctx, current)
	return &RevisionResult{Artifact: restored, StaleStageNames: staleStages}, nil
}

func (s *RevisionService) markDownstreamStale(ctx context.Context, base *Artifact) []string {
	if base.ProjectID == "" || base.StageName == "" {
		return nil
	}
	stages, err := s.artifacts.MarkDownstreamStale(ctx, base.ProjectID, base.ID, "用户返工修改产物")
	if err != nil {
		return nil
	}
	return stages
}

func (s *RevisionService) resolveOriginalContent(ctx context.Context, artifact *Artifact) string {
	if strings.TrimSpace(artifact.InlineJSON) != "" {
		return artifact.InlineJSON
	}
	if s.contentResolver != nil {
		if data, ok := s.contentResolver(ctx, artifact); ok {
			return string(data)
		}
	}
	content, _, _ := artifactContent(artifact)
	return stringifyContent(content)
}

func (s *RevisionService) readStageInstruction(artifact *Artifact) string {
	if s.skillRoot == "" || artifact.StageName == "" {
		return ""
	}
	entries, err := os.ReadDir(s.skillRoot)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(s.skillRoot, entry.Name()))
		if err != nil {
			continue
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			stagePath := filepath.Join(s.skillRoot, entry.Name(), version.Name(), "stages", artifact.StageName+".md")
			if data, err := os.ReadFile(stagePath); err == nil {
				return string(data)
			}
		}
	}
	return ""
}

func buildRevisionSystemPrompt(stageName string, stageInstruction string) string {
	prompt := fmt.Sprintf(`你是一个专业的内容返工助手，正在帮助用户修改「%s」阶段的产物。

重要规则：
- 严格根据用户的修改意见，在给定内容的基础上进行修改
- 严格遵循当前用户消息指定的输出范围
- 保持原始内容的结构、格式风格和未指定部分
- 只修改用户明确要求修改的部分，不要擅自改动其他内容
- 不要引入原始内容中没有的新字段、新章节或额外内容`, stageName)
	if stageInstruction != "" {
		prompt += "\n\n阶段说明（参考上下文）：\n" + stageInstruction
	}
	return prompt
}

type revisionTextSelection struct {
	Start int
	End   int
	Text  string
}

func textSelectionFromProvenance(provenance map[string]interface{}) (revisionTextSelection, bool, error) {
	value, exists := provenance["selection"]
	if !exists {
		return revisionTextSelection{}, false, nil
	}
	selection, ok := value.(map[string]interface{})
	if !ok || selection["kind"] != "text" {
		return revisionTextSelection{}, false, nil
	}
	start, startOK := selection["start"].(int)
	end, endOK := selection["end"].(int)
	text, textOK := selection["text"].(string)
	if !startOK || !endOK || !textOK {
		return revisionTextSelection{}, false, ErrRevisionInvalidReplacement
	}
	return revisionTextSelection{Start: start, End: end, Text: text}, true, nil
}

func buildSelectedRevisionUserPrompt(source string, selection revisionTextSelection, instruction string) (string, error) {
	sourceUnits, err := validatedSelectionUnits(source, selection)
	if err != nil {
		return "", err
	}
	contextStart := max(0, selection.Start-320)
	contextEnd := min(len(sourceUnits), selection.End+320)
	before := string(utf16.Decode(sourceUnits[contextStart:selection.Start]))
	selected := string(utf16.Decode(sourceUnits[selection.Start:selection.End]))
	after := string(utf16.Decode(sourceUnits[selection.End:contextEnd]))
	return fmt.Sprintf(`上文（最多 320 个 UTF-16 单元）：
%s

所选文字：
%s

下文（最多 320 个 UTF-16 单元）：
%s

修改意见：
%s

只返回替换文字，不要返回上下文、说明、标题或代码块。`, before, selected, after, instruction), nil
}

func spliceUTF16Selection(source string, selection revisionTextSelection, replacement string) (string, error) {
	sourceUnits, err := validatedSelectionUnits(source, selection)
	if err != nil {
		return "", err
	}
	replacementUnits := utf16.Encode([]rune(replacement))
	result := make([]uint16, 0, len(sourceUnits)-(selection.End-selection.Start)+len(replacementUnits))
	result = append(result, sourceUnits[:selection.Start]...)
	result = append(result, replacementUnits...)
	result = append(result, sourceUnits[selection.End:]...)
	return string(utf16.Decode(result)), nil
}

func validatedSelectionUnits(source string, selection revisionTextSelection) ([]uint16, error) {
	units := utf16.Encode([]rune(source))
	if selection.Start < 0 || selection.End <= selection.Start || selection.End > len(units) ||
		selection.End-selection.Start > 4000 || splitsRevisionSurrogatePair(units, selection.Start) ||
		splitsRevisionSurrogatePair(units, selection.End) {
		return nil, ErrRevisionInvalidReplacement
	}
	selectedUnits := utf16.Encode([]rune(selection.Text))
	if len(selectedUnits) != selection.End-selection.Start {
		return nil, ErrRevisionInvalidReplacement
	}
	for index, unit := range selectedUnits {
		if units[selection.Start+index] != unit {
			return nil, ErrRevisionInvalidReplacement
		}
	}
	return units, nil
}

func splitsRevisionSurrogatePair(units []uint16, offset int) bool {
	return offset > 0 && offset < len(units) &&
		utf16.IsSurrogate(rune(units[offset-1])) && utf16.IsSurrogate(rune(units[offset]))
}

func textModelProviderFromRevisionRequest(modelProvider, modelProviders map[string]interface{}) map[string]interface{} {
	if len(modelProvider) > 0 {
		return modelProvider
	}
	if len(modelProviders) == 0 {
		return nil
	}
	textProvider, _ := modelProviders["text_to_text"].(map[string]interface{})
	return textProvider
}

func buildLocalRevisionData(base *Artifact, instruction string) []byte {
	content, _, _ := artifactContent(base)
	switch base.Kind {
	case KindMarkdown:
		return []byte("## 返工版本\n\n返工要求：" + instruction + "\n\n" + stringifyContent(content))
	case KindJSON, KindImage, KindVideo, KindAudio, KindBundle:
		data, _ := json.MarshalIndent(map[string]interface{}{
			"revisionInstruction": instruction, "previous": content, "status": "regenerated",
		}, "", "  ")
		return data
	default:
		return []byte("返工要求：" + instruction + "\n\n" + stringifyContent(content))
	}
}

func stringifyContent(content interface{}) string {
	if text, ok := content.(string); ok {
		return text
	}
	data, _ := json.MarshalIndent(content, "", "  ")
	return string(data)
}
