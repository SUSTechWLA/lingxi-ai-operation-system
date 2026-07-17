package localtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// FFmpegProbeExecutor runs ffprobe on a local media file and returns
// structured media metadata (duration, resolution, codecs, etc.).
type FFmpegProbeExecutor struct {
	guard *PathGuard
}

// NewFFmpegProbeExecutor creates a new ffprobe executor.
func NewFFmpegProbeExecutor(dataDir string) *FFmpegProbeExecutor {
	return &FFmpegProbeExecutor{guard: NewPathGuard(dataDir)}
}

// ffprobeOutput mirrors the JSON structure emitted by ffprobe.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
}

// Execute runs ffprobe on the input file and returns structured metadata.
func (e *FFmpegProbeExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	inputURI := stringFromPayload(job.Payload, "input")
	if inputURI == "" {
		return nil, fmt.Errorf("input is required for ffprobe")
	}

	projectID := stringFromPayload(job.Payload, "projectId")
	if projectID == "" {
		projectID = stringFromPayload(job.Payload, "project_id")
	}

	inputPath, err := e.guard.ResolveLocalURI(inputURI)
	if err != nil {
		return nil, fmt.Errorf("invalid input path: %w", err)
	}
	if err := e.guard.EnsureReadable(inputURI); err != nil {
		return nil, fmt.Errorf("input not readable: %w", err)
	}

	// Run ffprobe: -v quiet -print_format json -show_format -show_streams
	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ffprobe failed: %s (stderr: %s)", exitErr.Error(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("ffprobe execution failed: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	// Build structured media metadata
	media := map[string]interface{}{}

	// Parse duration from format
	if probe.Format.Duration != "" {
		var durationSec float64
		if n, _ := fmt.Sscanf(probe.Format.Duration, "%f", &durationSec); n == 1 {
			media["durationSec"] = durationSec
		}
	}

	// Parse bitrate
	if probe.Format.BitRate != "" {
		var bitrate int64
		if n, _ := fmt.Sscanf(probe.Format.BitRate, "%d", &bitrate); n == 1 {
			media["bitrate"] = bitrate
		}
	}

	// Extract stream info
	var videoStream, audioStream *ffprobeStream
	for i := range probe.Streams {
		s := &probe.Streams[i]
		switch s.CodecType {
		case "video":
			if videoStream == nil {
				videoStream = s
			}
		case "audio":
			if audioStream == nil {
				audioStream = s
			}
		}
	}

	if videoStream != nil {
		media["width"] = videoStream.Width
		media["height"] = videoStream.Height
		media["videoCodec"] = videoStream.CodecName
		// Parse framerate (often "30/1" or "30000/1001")
		if videoStream.RFrameRate != "" {
			var num, den float64
			if n, _ := fmt.Sscanf(videoStream.RFrameRate, "%f/%f", &num, &den); n == 2 && den > 0 {
				media["fps"] = num / den
			} else if n, _ := fmt.Sscanf(videoStream.RFrameRate, "%f", &num); n == 1 {
				media["fps"] = num
			}
		}
	}

	if audioStream != nil {
		media["audioCodec"] = audioStream.CodecName
		media["hasAudio"] = true
	} else {
		media["hasAudio"] = false
	}

	return &Result{Output: map[string]interface{}{
		"success": true,
		"media":   media,
		"artifacts": []map[string]interface{}{
			{
				"unitId":         "ffmpeg-probe-report",
				"kind":           "FFMPEG_PROBE_REPORT",
				"name":           "ffprobe_report.json",
				"storageType":    "local",
				"storageRef":     "local://projects/" + projectID + "/reports/ffprobe.json",
				"mimeType":       "application/json",
				"sizeBytes":      0,
				"status":         "valid",
				"humanApproved":  false,
				"dependsOn":      []string{"VIDEO"},
				"producedByTool": "ffmpeg_probe",
				"producedByRole": "质量审核",
				"metadata":       media,
			},
		},
	}}, nil
}

// FFmpegAvailable checks if ffmpeg/ffprobe are available on the system PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}
