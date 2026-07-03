package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

// VideoFrameQAExecutor is a stable local runner command wrapper. The media QA
// implementation lives in mcp/video_qa/server.py and is invoked through the
// standard MCP tools/call protocol.
type VideoFrameQAExecutor struct {
	guard   *PathGuard
	dataDir string
	mcp     videoQAMCPClient
}

func NewVideoFrameQAExecutor(dataDir string) *VideoFrameQAExecutor {
	return NewVideoFrameQAExecutorWithMCPClient(dataDir, nil)
}

func NewVideoFrameQAExecutorWithMCPClient(dataDir string, client videoQAMCPClient) *VideoFrameQAExecutor {
	return &VideoFrameQAExecutor{guard: NewPathGuard(dataDir), dataDir: dataDir, mcp: client}
}

type videoQAMCPClient interface {
	CallTool(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error)
	Close() error
}

type localVideoQAMCPClient struct {
	client *localmcp.Client
}

func newDefaultVideoQAMCPClient(dataDir string, timeoutSec int) videoQAMCPClient {
	if timeoutSec <= 0 {
		timeoutSec = 10 * 60
	}
	cfg := localmcp.ProviderConfig{
		ID:         "video_qa",
		Label:      "Video QA MCP",
		Transport:  "stdio",
		Command:    defaultVideoQAMCPCommand(),
		Args:       []string{defaultVideoQAMCPServerPath()},
		WorkingDir: defaultVideoQAMCPWorkingDir(),
		ToolPrefix: "video_qa.",
		Env: map[string]string{
			"TANGYING_VIDEO_QA_DATA_DIR": dataDir,
		},
		Enabled: true,
	}
	return &localVideoQAMCPClient{client: localmcp.NewClient(cfg, &http.Client{Timeout: time.Duration(timeoutSec) * time.Second})}
}

func (c *localVideoQAMCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
	result, err := c.client.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("video_qa mcp returned error: %s", mcpContentText(result.Content))
	}
	if len(result.StructuredContent) > 0 {
		return result.StructuredContent, nil
	}
	for _, item := range result.Content {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return parsed, nil
		}
	}
	return map[string]interface{}{}, nil
}

func (c *localVideoQAMCPClient) Close() error {
	return c.client.Close()
}

func (e *VideoFrameQAExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	projectID := strings.TrimSpace(job.ProjectID)
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "projectId")
	}
	if projectID == "" {
		return nil, fmt.Errorf("video_frame_qa: projectId is required")
	}
	if err := validateLocalSegment(projectID); err != nil {
		return nil, fmt.Errorf("video_frame_qa: invalid projectId: %w", err)
	}

	inputURI := firstNonEmptyLocalString(
		stringFromPayload(job.Payload, "input"),
		stringFromPayload(job.Payload, "videoRef"),
		stringFromPayload(job.Payload, "finalVideo"),
	)
	if inputURI == "" {
		return nil, fmt.Errorf("video_frame_qa: input video is required")
	}
	inputPath, err := e.guard.ResolveLocalURI(inputURI)
	if err != nil {
		return nil, fmt.Errorf("video_frame_qa: invalid input path: %w", err)
	}
	if err := e.guard.EnsureReadable(inputURI); err != nil {
		return nil, fmt.Errorf("video_frame_qa: input not readable: %w", err)
	}

	sampleInterval := numberFromPayload(job.Payload, "sampleIntervalSec", 4)
	if sampleInterval <= 0 {
		sampleInterval = 4
	}

	reportDir := filepath.Join(e.dataDir, "projects", projectID, "reports", "video_frame_qa")
	client := e.mcp
	if client == nil {
		client = newDefaultVideoQAMCPClient(e.dataDir, job.TimeoutSec)
		defer client.Close()
	}
	args := copyLocalMap(job.Payload)
	args["projectId"] = projectID
	args["videoPath"] = inputPath
	args["videoRef"] = inputURI
	args["dataDir"] = e.dataDir
	args["outputDir"] = reportDir
	args["outputRefPrefix"] = "local://projects/" + projectID + "/reports/video_frame_qa"
	args["sampleIntervalSec"] = sampleInterval
	args["profile"] = normalizeVideoQAProfile(job.Payload)
	output, err := client.CallTool(ctx, "video_qa.analyze_video", args)
	if err != nil {
		return nil, fmt.Errorf("video_frame_qa: mcp analyze_video: %w", err)
	}
	if output == nil {
		output = map[string]interface{}{}
	}
	return &Result{Output: output}, nil
}

func normalizeVideoQAProfile(payload map[string]interface{}) string {
	for _, key := range []string{"profile", "profileId", "projectMode", "videoType", "creationProfile"} {
		if value := profileStringFromInterface(payload[key]); value != "" {
			return value
		}
	}
	return ""
}

func profileStringFromInterface(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		for _, key := range []string{"profileId", "id", "sourceRoute", "dagTemplateId"} {
			if value := profileStringFromInterface(typed[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func numberFromPayload(payload map[string]interface{}, key string, fallback float64) float64 {
	if payload == nil {
		return fallback
	}
	return numberFromInterfaceLocal(payload[key], fallback)
}

func numberFromInterfaceLocal(value interface{}, fallback float64) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed
		}
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(v, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func copyLocalMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input)+8)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func defaultVideoQAMCPCommand() string {
	if command := strings.TrimSpace(os.Getenv("VIDEO_QA_MCP_COMMAND")); command != "" {
		return command
	}
	for _, candidate := range videoQAPythonCandidates() {
		if command := compatibleVideoQAPythonExecutable(candidate); command != "" {
			return command
		}
	}
	return "python3"
}

func videoQAPythonCandidates() []string {
	candidates := []string{}
	if path, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if matches, err := filepath.Glob(filepath.Join(home, ".pyenv", "versions", "*", "bin", "python3")); err == nil {
			candidates = append(candidates, matches...)
		}
		if matches, err := filepath.Glob(filepath.Join(home, ".local", "bin", "python3")); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	candidates = append(candidates, "/opt/homebrew/bin/python3", "/usr/local/bin/python3", "python3")
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func compatibleVideoQAPythonExecutable(candidate string) string {
	out, err := exec.Command(candidate, "-c", "import sys; import mcp; sys.exit(1) if sys.version_info < (3, 10) else print(sys.executable)").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultVideoQAMCPServerPath() string {
	if script := strings.TrimSpace(os.Getenv("VIDEO_QA_MCP_STDIO_SCRIPT")); script != "" {
		return script
	}
	cwd, err := os.Getwd()
	if err == nil {
		candidates := []string{
			filepath.Join(cwd, "mcp", "video_qa", "server.py"),
			filepath.Join(cwd, "..", "mcp", "video_qa", "server.py"),
			filepath.Join(cwd, "..", "..", "mcp", "video_qa", "server.py"),
			filepath.Join(cwd, "..", "..", "..", "mcp", "video_qa", "server.py"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				if abs, err := filepath.Abs(candidate); err == nil {
					return abs
				}
				return candidate
			}
		}
	}
	return filepath.Join("mcp", "video_qa", "server.py")
}

func defaultVideoQAMCPWorkingDir() string {
	scriptPath := defaultVideoQAMCPServerPath()
	if filepath.IsAbs(scriptPath) {
		return filepath.Dir(scriptPath)
	}
	return ""
}

func firstNonEmptyLocalString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
