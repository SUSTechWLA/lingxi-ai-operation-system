package localrunner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ProbeResult struct {
	Platform     PlatformInfo `json:"platform"`
	Capabilities []Capability `json:"capabilities"`
	Warnings     []Warning    `json:"warnings,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Probe(ctx context.Context, workspaceRoot string) ProbeResult {
	hostname, _ := os.Hostname()
	result := ProbeResult{
		Platform: PlatformInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: hostname},
	}
	result.Capabilities = append(result.Capabilities,
		probeCommand("ffmpeg_probe", "FFMPEG_PROBE", "ffprobe"),
		probeCommand("ffmpeg_clip_extractor", "FFMPEG_CLIP_EXTRACT", "ffmpeg"),
		probeCommand("ffmpeg_assembler", "FFMPEG_ASSEMBLE", "ffmpeg"),
		probeCommand("audio_extractor", "AUDIO_EXTRACT", "ffmpeg"),
		probeCommand("audio_normalizer", "AUDIO_NORMALIZE", "ffmpeg"),
		probeCommand("hyperframes_project_generator", "HYPERFRAMES_PROJECT_GENERATE", "node"),
		probeCommand("hyperframes_linter", "HYPERFRAMES_LINT", "node"),
		probeCommand("artifact_packager", "ARTIFACT_PACKAGE", "node"),
		probeCommand("local_file_importer", "LOCAL_FILE_IMPORT", "node"),
	)
	result.Capabilities = append(result.Capabilities, probeLocalIpTalkingAvatarRender())
	renderAvailable, renderVersion := probeHTTP(ctx, "http://127.0.0.1:8787/health")
	result.Capabilities = append(result.Capabilities, Capability{
		ToolName:  "hyperframes_renderer",
		Command:   "HYPERFRAMES_RENDER",
		Available: renderAvailable,
		Version:   renderVersion,
	})
	if workspaceRoot != "" {
		if err := ensureWorkspaceWritable(workspaceRoot); err != nil {
			result.Warnings = append(result.Warnings, Warning{Code: "WORKSPACE_NOT_WRITABLE", Message: err.Error()})
		}
	}
	return result
}

func probeCommand(toolName, semanticCommand, binary string) Capability {
	path, err := exec.LookPath(binary)
	if err != nil {
		return Capability{ToolName: toolName, Command: semanticCommand, Available: false}
	}
	return Capability{ToolName: toolName, Command: semanticCommand, Available: true, Version: path}
}

func probeLocalIpTalkingAvatarRender() Capability {
	ffmpeg, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobe, ffprobeErr := exec.LookPath("ffprobe")
	if ffmpegErr != nil || ffprobeErr != nil {
		return Capability{
			ToolName:  "local_ip_talking_avatar_render",
			Command:   "LOCAL_IP_TALKING_AVATAR_RENDER",
			Available: false,
			Version:   "requires ffmpeg and ffprobe",
		}
	}
	return Capability{
		ToolName:  "local_ip_talking_avatar_render",
		Command:   "LOCAL_IP_TALKING_AVATAR_RENDER",
		Available: true,
		Version:   fmt.Sprintf("ffmpeg=%s; ffprobe=%s", ffmpeg, ffprobe),
	}
}

func probeHTTP(ctx context.Context, url string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, ""
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("http %d", resp.StatusCode)
	}
	return true, "hyperframes-render-service"
}

func ensureWorkspaceWritable(root string) error {
	if strings.HasPrefix(root, "local://") {
		return nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	probeFile := filepath.Join(root, ".runner-write-test")
	if err := os.WriteFile(probeFile, []byte("ok"), 0o600); err != nil {
		return err
	}
	_ = os.Remove(probeFile)
	return nil
}
