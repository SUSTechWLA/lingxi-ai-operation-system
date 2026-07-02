package localtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalFileImportExecutor struct {
	guard *PathGuard
}

func NewLocalFileImportExecutor(dataDir string) *LocalFileImportExecutor {
	return &LocalFileImportExecutor{guard: NewPathGuard(dataDir)}
}

func (e *LocalFileImportExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = firstPayloadString(job.Payload, "projectId", "project_id")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}

	sourceRef := firstPayloadString(job.Payload, "source", "input", "sourceRef", "inputRef", "path")
	if sourceRef == "" {
		return nil, fmt.Errorf("source is required for local file import")
	}
	if err := e.guard.EnsureReadable(sourceRef); err != nil {
		return nil, fmt.Errorf("source not readable: %w", err)
	}
	sourcePath, err := e.guard.ResolveLocalURI(sourceRef)
	if err != nil {
		return nil, fmt.Errorf("invalid source path: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat source: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source must be a file")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read source: %w", err)
	}
	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:])
	contentHash := "sha256:" + hashHex

	artifactID := safeImportSegment(firstPayloadString(job.Payload, "id", "artifactId", "unitId"), strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)))
	fileName := safeImportFileName(firstPayloadString(job.Payload, "fileName", "name"), filepath.Base(sourcePath))
	storageRef := firstPayloadString(job.Payload, "storageRef", "output", "destination")
	if storageRef == "" {
		storageRef = fmt.Sprintf("local://projects/%s/artifacts/%s/sha256_%s/%s", projectID, artifactID, hashHex[:16], fileName)
	}
	if err := e.guard.EnsureWritable(storageRef); err != nil {
		return nil, fmt.Errorf("destination not writable: %w", err)
	}
	destinationPath, err := e.guard.ResolveLocalURI(storageRef)
	if err != nil {
		return nil, fmt.Errorf("invalid destination path: %w", err)
	}
	if err := os.WriteFile(destinationPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("cannot write imported file: %w", err)
	}

	mimeType := firstPayloadString(job.Payload, "mimeType", "contentType")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	artifactKind := firstPayloadString(job.Payload, "artifactKind", "kind")
	if artifactKind == "" {
		artifactKind = "LOCAL_FILE"
	}
	metadata := copyPayloadMap(job.Payload["metadata"])
	metadata["sourceRef"] = sourceRef
	metadata["contentHash"] = contentHash
	metadata["importedAt"] = time.Now().UTC().Format(time.RFC3339)
	metadata["localOnly"] = true

	artifact := map[string]interface{}{
		"unitId":         artifactID,
		"kind":           artifactKind,
		"name":           fileName,
		"storageType":    "local",
		"storageRef":     storageRef,
		"mimeType":       mimeType,
		"sizeBytes":      info.Size(),
		"contentHash":    contentHash,
		"status":         "valid",
		"humanApproved":  false,
		"producedByTool": "local_file_importer",
		"producedByRole": "本地素材导入",
		"metadata":       metadata,
	}

	return &Result{Output: map[string]interface{}{
		"success":     true,
		"summary":     "本地文件导入完成",
		"id":          artifactID,
		"storageRef":  storageRef,
		"mimeType":    mimeType,
		"sizeBytes":   info.Size(),
		"contentHash": contentHash,
		"metadata":    metadata,
		"artifacts":   []map[string]interface{}{artifact},
	}}, nil
}

func firstPayloadString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromPayload(payload, key)); value != "" {
			return value
		}
	}
	return ""
}

func copyPayloadMap(value interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	if typed, ok := value.(map[string]interface{}); ok {
		for k, v := range typed {
			result[k] = v
		}
	}
	return result
}

func safeImportSegment(value, fallback string) string {
	candidate := sanitizeImportPart(value)
	if candidate == "" {
		candidate = sanitizeImportPart(fallback)
	}
	if candidate == "" {
		candidate = "imported-file"
	}
	if err := validateLocalSegment(candidate); err == nil {
		return candidate
	}
	return "imported-file"
}

func safeImportFileName(value, fallback string) string {
	candidate := sanitizeImportPart(value)
	if candidate == "" {
		candidate = sanitizeImportPart(fallback)
	}
	if candidate == "" {
		candidate = "imported-file"
	}
	return candidate
}

func sanitizeImportPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if allowed {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	result := strings.Trim(b.String(), "._-")
	if strings.Contains(result, "..") {
		result = strings.ReplaceAll(result, "..", "_")
	}
	return result
}
