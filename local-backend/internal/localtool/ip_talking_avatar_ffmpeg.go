package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FfmpegComposer struct {
	ffmpegPath string
}

func NewFfmpegComposer() *FfmpegComposer {
	return &FfmpegComposer{}
}

func (c *FfmpegComposer) Compose(ctx context.Context, framesDir string, input LocalIpTalkingAvatarRenderInput, durationSec float64) (string, error) {
	ffmpeg := c.ffmpegPath
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			return "", fmt.Errorf("ffmpeg is required to compose final IP talking avatar video")
		}
	}
	finalPath := filepath.Join(input.OutputDir, "final.mp4")
	includeSubtitle := input.SubtitlePath != "" && input.Style.SubtitleEnabled
	args := buildComposeArgs(framesDir, input, durationSec, finalPath, includeSubtitle)
	if err := os.WriteFile(filepath.Join(input.OutputDir, "ffmpeg_compose_command.json"), []byte(commandJSON(ffmpeg, args)), 0o644); err != nil {
		return "", fmt.Errorf("write ffmpeg command log: %w", err)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.WriteFile(filepath.Join(input.OutputDir, "ffmpeg_compose_error.log"), out, 0o644)
		if includeSubtitle && subtitleFilterUnsupported(out) {
			warning := "当前本机 FFmpeg 不支持 subtitles/libass 滤镜，已生成不烧录字幕的视频；字幕文件仍保留在输出目录，可由前端单独预览。"
			_ = os.WriteFile(filepath.Join(input.OutputDir, "subtitle_composition_warning.txt"), []byte(warning+"\n"), 0o644)
			args = buildComposeArgs(framesDir, input, durationSec, finalPath, false)
			_ = os.WriteFile(filepath.Join(input.OutputDir, "ffmpeg_compose_command_no_subtitles.json"), []byte(commandJSON(ffmpeg, args)), 0o644)
			cmd = exec.CommandContext(ctx, ffmpeg, args...)
			if retryOut, retryErr := cmd.CombinedOutput(); retryErr == nil {
				return finalPath, nil
			} else {
				_ = os.WriteFile(filepath.Join(input.OutputDir, "ffmpeg_compose_retry_error.log"), retryOut, 0o644)
				return "", fmt.Errorf("ffmpeg compose final video without burned subtitles failed: %w; output: %s", retryErr, trimCommandOutput(retryOut))
			}
		}
		return "", fmt.Errorf("ffmpeg compose final video failed: %w; output: %s", err, trimCommandOutput(out))
	}
	return finalPath, nil
}

func buildComposeArgs(framesDir string, input LocalIpTalkingAvatarRenderInput, durationSec float64, finalPath string, includeSubtitle bool) []string {
	args := []string{"-y"}

	backgroundInputIndex := 0
	switch {
	case input.BackgroundPath == "" || !input.Style.BackgroundEnabled:
		args = append(args,
			"-f", "lavfi",
			"-i", fmt.Sprintf("color=c=0xf8fbff:s=%dx%d:r=%d:d=%.3f", input.Resolution.Width, input.Resolution.Height, input.FPS, durationSec),
		)
	case isLikelyImagePath(input.BackgroundPath):
		args = append(args,
			"-loop", "1",
			"-t", fmt.Sprintf("%.3f", durationSec),
			"-i", input.BackgroundPath,
		)
	default:
		args = append(args,
			"-stream_loop", "-1",
			"-t", fmt.Sprintf("%.3f", durationSec),
			"-i", input.BackgroundPath,
		)
	}

	avatarInputIndex := 1
	args = append(args,
		"-framerate", fmt.Sprintf("%d", input.FPS),
		"-i", filepath.Join(framesDir, "frame_%06d.png"),
	)
	audioInputIndex := 2
	args = append(args, "-i", input.AudioPath)

	bgmInputIndex := -1
	if input.BgmPath != "" {
		bgmInputIndex = 3
		args = append(args, "-stream_loop", "-1", "-i", input.BgmPath)
	}

	videoFilter := fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[bg];[%d:v]format=rgba[av];[bg][av]overlay=0:0:format=auto",
		backgroundInputIndex,
		input.Resolution.Width,
		input.Resolution.Height,
		input.Resolution.Width,
		input.Resolution.Height,
		avatarInputIndex,
	)
	if includeSubtitle {
		videoFilter += fmt.Sprintf(",subtitles=filename='%s'", escapeFFmpegFilterPath(input.SubtitlePath))
	}
	videoFilter += "[v]"

	filterComplex := videoFilter
	mapArgs := []string{"-map", "[v]"}
	if bgmInputIndex >= 0 {
		filterComplex += fmt.Sprintf(";[%d:a]volume=1.0[a0];[%d:a]volume=0.18[a1];[a0][a1]amix=inputs=2:duration=first:dropout_transition=1.5[a]", audioInputIndex, bgmInputIndex)
		mapArgs = append(mapArgs, "-map", "[a]")
	} else {
		mapArgs = append(mapArgs, "-map", fmt.Sprintf("%d:a:0", audioInputIndex))
	}

	args = append(args,
		"-filter_complex", filterComplex,
	)
	args = append(args, mapArgs...)
	args = append(args,
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", input.FPS),
		"-c:a", "aac",
		"-b:a", "192k",
		"-movflags", "+faststart",
		"-shortest",
		finalPath,
	)
	return args
}

func isLikelyImagePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func escapeFFmpegFilterPath(path string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"'", "\\'",
		":", "\\:",
	)
	return replacer.Replace(path)
}

func commandJSON(binary string, args []string) string {
	payload := map[string]interface{}{
		"binary":      binary,
		"args":        args,
		"generatedAt": nowRFC3339(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(data) + "\n"
}

func subtitleFilterUnsupported(output []byte) bool {
	text := strings.ToLower(string(output))
	return strings.Contains(text, "no such filter: 'subtitles'") ||
		strings.Contains(text, "no such filter: subtitles") ||
		strings.Contains(text, "libass") && strings.Contains(text, "not found")
}
