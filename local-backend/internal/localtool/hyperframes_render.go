package localtool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HyperFramesRenderExecutor calls the HyperFrames Render Service to render
// a HyperFrames HTML project into an MP4 video file.
type HyperFramesRenderExecutor struct {
	guard      *PathGuard
	dataDir    string
	serviceURL string
	timeout    time.Duration
}

// NewHyperFramesRenderExecutor creates a new render executor.
// serviceURL is the base URL of the HyperFrames Render Service (e.g. http://127.0.0.1:8787).
// timeout is the maximum duration for a single render job.
func NewHyperFramesRenderExecutor(dataDir, serviceURL string, timeout time.Duration) *HyperFramesRenderExecutor {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	return &HyperFramesRenderExecutor{
		guard:      NewPathGuard(dataDir),
		dataDir:    dataDir,
		serviceURL: strings.TrimRight(serviceURL, "/"),
		timeout:    timeout,
	}
}

type hyperFramesRenderRequest struct {
	ProjectDir string `json:"projectDir"`
	Entry      string `json:"entry,omitempty"`
	OutputPath string `json:"outputPath"`
	FPS        int    `json:"fps,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Format     string `json:"format,omitempty"`
}

type hyperFramesRenderResponse struct {
	OK         bool   `json:"ok"`
	JobID      string `json:"jobId"`
	OutputPath string `json:"outputPath,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Execute resolves local:// paths, calls the HyperFrames Render Service,
// and returns artifact metadata for the rendered video.
func (e *HyperFramesRenderExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}

	// Resolve input paths
	projectDirURI := stringFromPayload(job.Payload, "projectDir")
	if projectDirURI == "" {
		projectDirURI = "local://projects/" + projectID + "/hyperframes"
	}
	projectDir, err := e.guard.ResolveLocalURI(projectDirURI)
	if err != nil {
		return nil, fmt.Errorf("invalid projectDir: %w", err)
	}
	if err := e.guard.EnsureReadable(projectDirURI); err != nil {
		return nil, fmt.Errorf("projectDir not accessible: %w", err)
	}

	entry := stringFromPayload(job.Payload, "entry")
	if entry == "" {
		entry = "index.html"
	}
	entryPath := filepath.Join(projectDir, entry)
	if _, err := os.Stat(entryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("entry file not found: %s", entryPath)
	}

	// Resolve output path
	outputURI := stringFromPayload(job.Payload, "outputPath")
	if outputURI == "" {
		outputURI = "local://projects/" + projectID + "/renders/final.mp4"
	}
	outputPath, err := e.guard.ResolveLocalURI(outputURI)
	if err != nil {
		return nil, fmt.Errorf("invalid outputPath: %w", err)
	}
	if err := e.guard.EnsureWritable(outputURI); err != nil {
		return nil, fmt.Errorf("outputPath not writable: %w", err)
	}

	// Parse render options
	fps := 30
	if f, ok := job.Payload["fps"].(float64); ok && f > 0 {
		fps = int(f)
	}
	quality := stringFromPayload(job.Payload, "quality")
	if quality == "" {
		quality = "standard"
	}
	format := stringFromPayload(job.Payload, "format")
	if format == "" {
		format = "mp4"
	}
	width := 1920
	if value, ok := job.Payload["width"].(float64); ok && value > 0 {
		width = int(value)
	}
	height := 1080
	if value, ok := job.Payload["height"].(float64); ok && value > 0 {
		height = int(value)
	}

	fastAttempted := false
	if fastStoryboardRenderEnabled() {
		fastAttempted = true
		if fallback, handled, fallbackErr := e.tryFastStoryboardRender(ctx, projectID, projectDir, outputPath, fps, width, height); handled {
			if fallbackErr != nil {
				return nil, fallbackErr
			}
			return fallback, nil
		}
	}

	// Call HyperFrames Render Service
	renderServiceTimeout := time.Duration(job.TimeoutSec) * time.Second
	if payloadTimeout := mcpSecondsFromPayload(job.Payload, "timeoutSec", "renderTimeoutSec", "hyperframesRenderTimeoutSec"); payloadTimeout > 0 {
		renderServiceTimeout = payloadTimeout
	}
	if renderServiceTimeout <= 0 {
		renderServiceTimeout = e.timeout
	}
	renderCtx, cancel := context.WithTimeout(ctx, renderServiceTimeout)
	defer cancel()

	renderReq := hyperFramesRenderRequest{
		ProjectDir: projectDir,
		Entry:      entry,
		OutputPath: outputPath,
		FPS:        fps,
		Quality:    quality,
		Format:     format,
	}

	result, err := e.callRenderService(renderCtx, renderReq)
	if err != nil {
		if !fastAttempted && storyboardRenderFallbackEnabled() && ctx.Err() == nil {
			if fallback, handled, fallbackErr := e.tryFastStoryboardRender(ctx, projectID, projectDir, outputPath, fps, width, height); handled {
				if fallbackErr == nil {
					return fallback, nil
				}
				return nil, fmt.Errorf("hyperframes render failed: %w; storyboard fallback failed: %v", err, fallbackErr)
			}
		}
		return nil, fmt.Errorf("hyperframes render failed: %w", err)
	}

	// Verify output file
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("render output not found at %s: %w", outputPath, err)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("render output is empty: %s", outputPath)
	}

	return e.renderResult(projectID, outputPath, info.Size(), fps, width, height, result)
}

func (e *HyperFramesRenderExecutor) tryFastStoryboardRender(ctx context.Context, projectID, projectDir, outputPath string, fps, width, height int) (*Result, bool, error) {
	started := time.Now()
	fallbackResult, handled, fallbackErr := e.renderFastStoryboard(ctx, projectID, projectDir, outputPath, fps, width, height)
	if !handled {
		return nil, false, nil
	}
	if fallbackErr != nil {
		return nil, true, fallbackErr
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, true, fmt.Errorf("render output not found at %s: %w", outputPath, err)
	}
	if info.Size() == 0 {
		return nil, true, fmt.Errorf("render output is empty: %s", outputPath)
	}
	fallbackResult.DurationMs = time.Since(started).Milliseconds()
	result, err := e.renderResult(projectID, outputPath, info.Size(), fps, width, height, fallbackResult)
	if err != nil {
		return nil, true, err
	}
	return result, true, nil
}

func (e *HyperFramesRenderExecutor) renderResult(projectID, outputPath string, sizeBytes int64, fps, width, height int, result *hyperFramesRenderResponse) (*Result, error) {
	provenance := hyperframesRenderProvenance(result)
	localRef, err := e.mirrorFinalVideoArtifact(projectID, outputPath, sizeBytes, fps, width, height, result.DurationMs, provenance)
	if err != nil {
		return nil, err
	}
	return &Result{Output: map[string]interface{}{
		"success":   true,
		"summary":   "HyperFrames 渲染完成",
		"outputRef": localRef,
		"artifacts": []map[string]interface{}{
			{
				"unitId":         "final-video",
				"kind":           "VIDEO",
				"name":           "final.mp4",
				"storageType":    "local",
				"storageRef":     localRef,
				"mimeType":       "video/mp4",
				"sizeBytes":      sizeBytes,
				"status":         "valid",
				"humanApproved":  false,
				"dependsOn":      []string{"PREVIEW_SNAPSHOTS", "HYPERFRAMES_PROJECT"},
				"producedByTool": "hyperframes_renderer",
				"producedByRole": "渲染制片",
				"metadata": map[string]interface{}{
					"renderTimeMs": result.DurationMs,
					"fps":          fps,
					"width":        width,
					"height":       height,
					"localPath":    outputPath,
					"provenance":   provenance,
					"sourceType":   provenance["sourceType"],
					"providerName": provenance["providerName"],
					"isFallback":   provenance["isFallback"],
				},
			},
		},
		"metrics": map[string]interface{}{
			"renderTimeMs": result.DurationMs,
			"fps":          fps,
			"width":        width,
			"height":       height,
		},
		"renderJobId": result.JobID,
	}}, nil
}

func hyperframesRenderProvenance(result *hyperFramesRenderResponse) map[string]interface{} {
	jobID := ""
	if result != nil {
		jobID = result.JobID
	}
	sourceType := "hyperframes"
	providerName := "hyperframes-render-service"
	isFallback := false
	fallbackReason := ""
	if jobID == "storyboard_fast_render" {
		sourceType = "fallback_storyboard"
		providerName = "local-storyboard-renderer"
		isFallback = true
		fallbackReason = "storyboard_fast_render"
	}
	return map[string]interface{}{
		"schemaVersion":     1,
		"sourceType":        sourceType,
		"providerName":      providerName,
		"providerJobId":     jobID,
		"fallbackReason":    fallbackReason,
		"isFallback":        isFallback,
		"generatedAt":       time.Now().UTC().Format(time.RFC3339),
		"inputPromptHash":   "",
		"sourceArtifactIds": []interface{}{"PREVIEW_SNAPSHOTS", "HYPERFRAMES_PROJECT"},
	}
}

func (e *HyperFramesRenderExecutor) callRenderService(ctx context.Context, reqBody hyperFramesRenderRequest) (*hyperFramesRenderResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal render request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.serviceURL+"/render", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create render request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // context controls timeout
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("render service unreachable at %s: %w", e.serviceURL, err)
	}
	defer resp.Body.Close()

	var result hyperFramesRenderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode render response: %w", err)
	}

	if resp.StatusCode >= 300 || !result.OK {
		if result.Error != "" {
			return &result, fmt.Errorf("render service error: %s", result.Error)
		}
		return &result, fmt.Errorf("render service returned status %d", resp.StatusCode)
	}

	return &result, nil
}

func (e *HyperFramesRenderExecutor) mirrorFinalVideoArtifact(projectID, outputPath string, sizeBytes int64, fps, width, height int, renderTimeMs int64, provenance map[string]interface{}) (string, error) {
	artifactID := "final-video"
	if err := validateLocalSegment(projectID); err != nil {
		return "", fmt.Errorf("invalid project id: %w", err)
	}
	if err := validateLocalSegment(artifactID); err != nil {
		return "", fmt.Errorf("invalid artifact id: %w", err)
	}

	contentPath := filepath.Join(e.dataDir, "artifacts", projectID, artifactID, "content")
	metadataPath := filepath.Join(e.dataDir, "artifacts", projectID, artifactID, "metadata.json")
	if err := ensureInside(e.dataDir, contentPath); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}

	source, err := os.Open(outputPath)
	if err != nil {
		return "", fmt.Errorf("open render output: %w", err)
	}
	defer source.Close()

	target, err := os.Create(contentPath)
	if err != nil {
		return "", fmt.Errorf("create artifact content: %w", err)
	}
	hasher := sha256.New()
	copied, copyErr := io.Copy(io.MultiWriter(target, hasher), source)
	closeErr := target.Close()
	if copyErr != nil {
		return "", fmt.Errorf("copy render output to artifact: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close artifact content: %w", closeErr)
	}
	if copied > 0 {
		sizeBytes = copied
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	storageRef := "local://projects/" + projectID + "/artifacts/" + artifactID + "/" + hash + "/final.mp4"
	metadata := map[string]interface{}{
		"id":                artifactID,
		"projectId":         projectID,
		"storageRef":        storageRef,
		"mimeType":          "video/mp4",
		"contentHash":       "sha256:" + hash,
		"sizeBytes":         sizeBytes,
		"sourcePath":        outputPath,
		"renderTimeMs":      renderTimeMs,
		"fps":               fps,
		"width":             width,
		"height":            height,
		"schemaVersion":     1,
		"sourceType":        provenance["sourceType"],
		"providerName":      provenance["providerName"],
		"providerJobId":     provenance["providerJobId"],
		"fallbackReason":    provenance["fallbackReason"],
		"isFallback":        provenance["isFallback"],
		"generatedAt":       provenance["generatedAt"],
		"inputPromptHash":   provenance["inputPromptHash"],
		"sourceArtifactIds": provenance["sourceArtifactIds"],
		"provenance":        provenance,
		"updatedAt":         time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeLocalToolJSON(metadataPath, metadata); err != nil {
		return "", fmt.Errorf("write artifact metadata: %w", err)
	}
	return storageRef, nil
}

func writeLocalToolJSON(path string, value interface{}) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
