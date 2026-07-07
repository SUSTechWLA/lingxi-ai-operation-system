package localtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type RenderQualityChecker struct {
	ffprobePath string
}

func NewRenderQualityChecker() *RenderQualityChecker {
	return &RenderQualityChecker{}
}

func (q *RenderQualityChecker) Check(ctx context.Context, input LocalIpTalkingAvatarRenderInput, output LocalIpTalkingAvatarRenderOutput, expectedDuration float64, lipPath, motionPath string, asset *CharacterAsset) (RenderQAResult, error) {
	result := RenderQAResult{
		AudioExists:             fileExists(input.AudioPath),
		VideoExists:             fileExists(output.VideoPath),
		SubtitleExists:          input.SubtitlePath != "" && fileExists(input.SubtitlePath),
		MouthTimelineGenerated:  fileExists(lipPath),
		MotionTimelineGenerated: fileExists(motionPath),
		SceneGenerated:          fileExists(output.ScenePath),
		FinalVideoGenerated:     fileExists(output.VideoPath),
	}
	if asset != nil {
		result.ControlRigLoaded = asset.Renderer == "sprite2d" || asset.ControlRig != nil || fileExists(asset.AssetPaths["rig"])
		result.ReferenceSVGLoaded = fileExists(asset.AssetPaths["referenceSvg"])
	}
	if result.FinalVideoGenerated && expectedDuration > 0 {
		duration, err := q.probeVideoDuration(ctx, output.VideoPath)
		if err == nil {
			result.DurationMatched = absFloat(duration-expectedDuration) <= 0.75
		}
	}
	reportPath := filepath.Join(input.OutputDir, "render_report.json")
	report := map[string]interface{}{
		"schemaVersion":    "local-ip-talking-avatar-render-report/v1",
		"generatedAt":      nowRFC3339(),
		"characterId":      input.CharacterID,
		"videoPath":        output.VideoPath,
		"avatarVideoPath":  output.AvatarVideoPath,
		"timelinePath":     output.TimelinePath,
		"scenePath":        output.ScenePath,
		"voiceProfilePath": output.VoiceProfilePath,
		"audioPath":        input.AudioPath,
		"subtitlePath":     input.SubtitlePath,
		"expectedDuration": expectedDuration,
		"renderMode":       input.RenderMode,
		"interactionLevel": input.InteractionLevel,
		"qa":               result,
	}
	if err := writeJSONFile(reportPath, report); err != nil {
		return result, fmt.Errorf("write render_report.json: %w", err)
	}
	return result, nil
}

func (q *RenderQualityChecker) probeVideoDuration(ctx context.Context, path string) (float64, error) {
	ffprobe := q.ffprobePath
	if ffprobe == "" {
		var err error
		ffprobe, err = exec.LookPath("ffprobe")
		if err != nil {
			return 0, fmt.Errorf("ffprobe is required for render QA duration check")
		}
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, ffmpegCommandError("ffprobe final video duration", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}
	return duration, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
