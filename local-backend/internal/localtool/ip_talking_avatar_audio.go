package localtool

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type AudioAnalyzer struct {
	ffmpegPath  string
	ffprobePath string
}

func NewAudioAnalyzer() *AudioAnalyzer {
	return &AudioAnalyzer{}
}

func (a *AudioAnalyzer) Analyze(ctx context.Context, audioPath, outputDir string, fps int) (*AudioAnalysis, error) {
	if strings.TrimSpace(audioPath) == "" {
		return nil, fmt.Errorf("audioPath is required")
	}
	if fps <= 0 {
		fps = 30
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir for audio analysis: %w", err)
	}
	if info, err := os.Stat(audioPath); err != nil {
		return nil, fmt.Errorf("audioPath not readable: %w", err)
	} else if info.IsDir() || info.Size() == 0 {
		return nil, fmt.Errorf("audioPath is not a readable audio file: %s", audioPath)
	}

	durationSec, err := a.probeDuration(ctx, audioPath)
	if err != nil {
		return nil, err
	}
	rawPath := filepath.Join(outputDir, "audio_analysis_input.s16le")
	if err := a.convertToPCM(ctx, audioPath, rawPath); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		return nil, fmt.Errorf("read converted pcm: %w", err)
	}
	const sampleRate = 16000
	samples := len(raw) / 2
	frameCount := int(math.Ceil(durationSec * float64(fps)))
	if frameCount < 1 {
		frameCount = 1
	}
	samplesPerFrame := sampleRate / fps
	if samplesPerFrame < 1 {
		samplesPerFrame = 1
	}
	frames := make([]AudioFrame, 0, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		start := frame * samplesPerFrame
		end := start + samplesPerFrame
		if end > samples {
			end = samples
		}
		var sum float64
		count := 0
		for i := start; i < end; i++ {
			offset := i * 2
			if offset+1 >= len(raw) {
				break
			}
			v := int16(binary.LittleEndian.Uint16(raw[offset : offset+2]))
			n := float64(v) / 32768.0
			sum += n * n
			count++
		}
		rms := 0.0
		if count > 0 {
			rms = math.Sqrt(sum / float64(count))
		}
		frames = append(frames, AudioFrame{
			TimeSec: float64(frame) / float64(fps),
			RMS:     roundFloat(rms, 5),
			Silent:  rms < 0.025,
		})
	}
	analysis := &AudioAnalysis{
		AudioPath:   audioPath,
		DurationSec: roundFloat(durationSec, 4),
		FPS:         fps,
		SampleRate:  sampleRate,
		Frames:      frames,
		GeneratedAt: nowRFC3339(),
	}
	if err := writeJSONFile(filepath.Join(outputDir, "audio_analysis.json"), analysis); err != nil {
		return nil, fmt.Errorf("write audio_analysis.json: %w", err)
	}
	_ = os.Remove(rawPath)
	return analysis, nil
}

func (a *AudioAnalyzer) probeDuration(ctx context.Context, audioPath string) (float64, error) {
	ffprobe := a.ffprobePath
	if ffprobe == "" {
		var err error
		ffprobe, err = exec.LookPath("ffprobe")
		if err != nil {
			return 0, fmt.Errorf("ffprobe is required for local IP talking avatar audio analysis")
		}
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, ffmpegCommandError("ffprobe audio duration", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("ffprobe returned invalid audio duration %q", strings.TrimSpace(string(out)))
	}
	return duration, nil
}

func (a *AudioAnalyzer) convertToPCM(ctx context.Context, audioPath, rawPath string) error {
	ffmpeg := a.ffmpegPath
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			return fmt.Errorf("ffmpeg is required for local IP talking avatar audio analysis")
		}
	}
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-i", audioPath,
		"-ac", "1",
		"-ar", "16000",
		"-f", "s16le",
		rawPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg convert audio to pcm failed: %w; output: %s", err, trimCommandOutput(out))
	}
	return nil
}

func writeJSONFile(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func roundFloat(v float64, digits int) float64 {
	pow := math.Pow10(digits)
	return math.Round(v*pow) / pow
}

func ffmpegCommandError(action string, err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("%s failed: %w; stderr: %s", action, err, trimCommandOutput(exitErr.Stderr))
	}
	return fmt.Errorf("%s failed: %w", action, err)
}

func trimCommandOutput(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 1200 {
		return text[:1200] + "...(truncated)"
	}
	return text
}
