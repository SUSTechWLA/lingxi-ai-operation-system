package localtool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HyperFramesSnapshotExecutor calls the HyperFrames Render Service to take
// preview screenshots of a HyperFrames HTML project at specified time points.
type HyperFramesSnapshotExecutor struct {
	guard      *PathGuard
	serviceURL string
	timeout    time.Duration
}

// NewHyperFramesSnapshotExecutor creates a new snapshot executor.
// serviceURL is the base URL of the HyperFrames Render Service (e.g. http://127.0.0.1:8787).
// timeout is the maximum duration for a snapshot job.
func NewHyperFramesSnapshotExecutor(dataDir, serviceURL string, timeout time.Duration) *HyperFramesSnapshotExecutor {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &HyperFramesSnapshotExecutor{
		guard:      NewPathGuard(dataDir),
		serviceURL: strings.TrimRight(serviceURL, "/"),
		timeout:    timeout,
	}
}

type hyperFramesSnapshotRequest struct {
	ProjectDir string   `json:"projectDir"`
	Entry      string   `json:"entry,omitempty"`
	TimePoints []string `json:"timePoints,omitempty"`
}

type hyperFramesSnapshotResponse struct {
	OK        bool               `json:"ok"`
	Snapshots []snapshotImage    `json:"snapshots,omitempty"`
	Count     int                `json:"count"`
	Error     string             `json:"error,omitempty"`
}

type snapshotImage struct {
	TimePoint string `json:"timePoint"`
	Data      string `json:"data,omitempty"`      // base64-encoded PNG
	Path      string `json:"path,omitempty"`       // absolute filesystem path (service-side)
	Filename  string `json:"filename,omitempty"`   // suggested filename
}

// Execute resolves local:// paths, calls the HyperFrames Render Service
// snapshot endpoint, saves preview images to the local previews directory,
// and returns artifact metadata.
func (e *HyperFramesSnapshotExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("invalid project id: %w", err)
	}

	// Resolve project directory
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

	// Parse time points
	var timePoints []string
	if raw, ok := job.Payload["timePoints"]; ok {
		switch v := raw.(type) {
		case []interface{}:
			for _, tp := range v {
				timePoints = append(timePoints, fmt.Sprint(tp))
			}
		case []string:
			timePoints = v
		}
	}
	if len(timePoints) == 0 {
		timePoints = []string{"0", "25%", "50%", "75%", "last"}
	}

	// Ensure previews directory is writable
	previewsURI := "local://projects/" + projectID + "/previews"
	if err := e.guard.EnsureWritable(previewsURI); err != nil {
		// Create the directory if it doesn't exist
		previewsDir, resolveErr := e.guard.ResolveLocalURI(previewsURI)
		if resolveErr != nil {
			return nil, fmt.Errorf("cannot resolve previews dir: %w", resolveErr)
		}
		if mkdirErr := os.MkdirAll(previewsDir, 0755); mkdirErr != nil {
			return nil, fmt.Errorf("cannot create previews dir: %w", mkdirErr)
		}
	}
	previewsDir, _ := e.guard.ResolveLocalURI(previewsURI)

	// Call HyperFrames Render Service snapshot endpoint
	timeoutSec := job.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = int(e.timeout.Seconds())
	}
	snapCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	snapReq := hyperFramesSnapshotRequest{
		ProjectDir: projectDir,
		Entry:      entry,
		TimePoints: timePoints,
	}

	result, err := e.callSnapshotService(snapCtx, snapReq)
	if err != nil {
		return nil, fmt.Errorf("hyperframes snapshot failed: %w", err)
	}

	// Save snapshot images to previews directory
	var localSnapshots []map[string]interface{}
	for i, img := range result.Snapshots {
		filename := img.Filename
		if filename == "" {
			filename = fmt.Sprintf("snapshot_%03d.png", i+1)
		}

		var savedPath string
		if img.Data != "" {
			// Decode base64 image data and write to disk
			decoded, decErr := base64.StdEncoding.DecodeString(img.Data)
			if decErr != nil {
				return nil, fmt.Errorf("decode snapshot %d (%s): %w", i, img.TimePoint, decErr)
			}
			outPath := filepath.Join(previewsDir, filename)
			if writeErr := os.WriteFile(outPath, decoded, 0644); writeErr != nil {
				return nil, fmt.Errorf("write snapshot %d: %w", i, writeErr)
			}
			savedPath = "local://projects/" + projectID + "/previews/" + filename
		} else if img.Path != "" {
			savedPath = img.Path
		} else {
			continue
		}

		localSnapshots = append(localSnapshots, map[string]interface{}{
			"timePoint": img.TimePoint,
			"path":      savedPath,
			"filename":  filename,
		})
	}

	artifactKind := "PREVIEW_SNAPSHOTS"
	return &Result{Output: map[string]interface{}{
		"success":      true,
		"summary":      fmt.Sprintf("预览截图完成，共 %d 张", len(localSnapshots)),
		"artifactKind": artifactKind,
		"snapshots":    localSnapshots,
		"count":        len(localSnapshots),
		"previewsDir":  "local://projects/" + projectID + "/previews",
		"artifacts": []map[string]interface{}{
			{
				"kind":           "PREVIEW_SNAPSHOTS",
				"name":           "preview_snapshots",
				"storageType":    "local",
				"storageRef":     "local://projects/" + projectID + "/previews",
				"mimeType":       "image/png",
				"sizeBytes":      0,
				"status":         "pending",
				"humanApproved":  false,
				"dependsOn":      []string{"HYPERFRAMES_PROJECT"},
				"producedByTool": "hyperframes_snapshot",
				"producedByRole": "预览导演",
				"metadata": map[string]interface{}{
					"snapshotCount": len(localSnapshots),
					"snapshotRefs":  localSnapshots,
				},
			},
		},
	}}, nil
}

func (e *HyperFramesSnapshotExecutor) callSnapshotService(ctx context.Context, reqBody hyperFramesSnapshotRequest) (*hyperFramesSnapshotResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.serviceURL+"/snapshot", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create snapshot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // context controls timeout
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snapshot service unreachable at %s: %w", e.serviceURL, err)
	}
	defer resp.Body.Close()

	var result hyperFramesSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode snapshot response: %w", err)
	}

	if resp.StatusCode >= 300 || !result.OK {
		if result.Error != "" {
			return &result, fmt.Errorf("snapshot service error: %s", result.Error)
		}
		return &result, fmt.Errorf("snapshot service returned status %d", resp.StatusCode)
	}

	return &result, nil
}
