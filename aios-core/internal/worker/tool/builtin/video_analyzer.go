package builtin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/common/llmutil"
	"github.com/tangying-ai/aios-core/internal/config"
	"github.com/tangying-ai/aios-core/internal/media"
	"github.com/tangying-ai/aios-core/internal/worker/tool"
)

type VideoAnalyzerTool struct {
	cfg      config.OpenAIConfig
	mediaSvc *media.MediaService
}

func NewVideoAnalyzerTool(cfg config.OpenAIConfig, mediaSvc *media.MediaService) *VideoAnalyzerTool {
	return &VideoAnalyzerTool{cfg: cfg, mediaSvc: mediaSvc}
}

func (t *VideoAnalyzerTool) Name() string { return "video_analyzer" }
func (t *VideoAnalyzerTool) Description() string {
	return "从短视频中提取关键帧（ffmpeg场景检测）和音频转录（Whisper）。输入cached_video_path或media_id，输出关键帧base64数据URL和对话转录文本。"
}
func (t *VideoAnalyzerTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *VideoAnalyzerTool) ValidateParameters(params map[string]interface{}) bool {
	if _, ok := params["cached_video_path"].(string); ok {
		return true
	}
	if _, ok := params["media_id"].(string); ok {
		return true
	}
	return false
}

func (t *VideoAnalyzerTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	maxKeyframes := 16
	if v, ok := params["max_keyframes"].(float64); ok && v > 0 {
		maxKeyframes = int(v)
		if maxKeyframes > 30 {
			maxKeyframes = 30
		}
	}

	strategy := "auto"
	if s, ok := params["strategy"].(string); ok && s != "" {
		strategy = s
	}

	// Resolve video path: use cached path if available, otherwise download
	videoPath, _ := params["cached_video_path"].(string)
	if videoPath == "" {
		mediaID, _ := params["media_id"].(string)
		if mediaID == "" {
			return tool.FailureResult("cached_video_path or media_id is required")
		}
		if t.mediaSvc == nil {
			return tool.FailureResult("media service is not available")
		}
		var err error
		videoPath, err = t.downloadToCache(ctx, mediaID)
		if err != nil {
			return tool.FailureResult("failed to get video: " + err.Error())
		}
	}

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return tool.FailureResult("video file not found: " + videoPath)
	}

	zap.L().Info("VideoAnalyzerTool starting",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("videoPath", videoPath),
		zap.Int("maxKeyframes", maxKeyframes),
		zap.String("strategy", strategy),
	)

	// Get metadata for fps/duration
	meta, err := runFFprobe(ctx, videoPath)
	if err != nil {
		return tool.FailureResult("ffprobe failed: " + err.Error())
	}

	// Resolve strategy
	strategy = resolveStrategy(strategy, meta)

	// Create temp work dir for frames and audio
	workDir, err := os.MkdirTemp("", "video-analyze-*")
	if err != nil {
		return tool.FailureResult("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(workDir)

	// Stage 1: Extract keyframes
	framesDir := filepath.Join(workDir, "frames")
	if err := os.MkdirAll(framesDir, 0700); err != nil {
		return tool.FailureResult("failed to create frames dir: " + err.Error())
	}
	frameCount, extractionMethod, err := extractKeyFrames(ctx, videoPath, framesDir, maxKeyframes, meta)
	if err != nil {
		return tool.FailureResult("frame extraction failed: " + err.Error())
	}

	// Stage 2: Extract and transcribe audio if applicable
	var transcription string
	if meta.HasAudio && (strategy == "audio" || strategy == "balanced") {
		audioPath := filepath.Join(workDir, "audio.mp3")
		if err := extractAudio(ctx, videoPath, audioPath); err != nil {
			zap.L().Warn("Audio extraction failed, continuing without transcription", zap.Error(err))
		} else {
			audioData, err := os.ReadFile(audioPath)
			if err != nil {
				zap.L().Warn("Failed to read audio file", zap.Error(err))
			} else {
				trans, err := callTranscription(ctx, t.cfg, audioData)
				if err != nil {
					zap.L().Warn("Transcription failed, continuing without it", zap.Error(err))
				} else {
					transcription = trans
				}
			}
		}
	}

	// Stage 3: Compress and encode frames to base64 data URLs
	imageDataURLs, err := compressAndEncodeFrames(framesDir)
	if err != nil {
		return tool.FailureResult("failed to encode frames: " + err.Error())
	}

	zap.L().Info("VideoAnalyzerTool completed",
		zap.String("taskId", toolCtx.TaskID),
		zap.Int("frameCount", frameCount),
		zap.Bool("hasTranscription", transcription != ""),
	)

	urlsInterface := make([]interface{}, len(imageDataURLs))
	var totalFramesSize int64
	for i, u := range imageDataURLs {
		urlsInterface[i] = u
		// Estimate base64 data URL size (before encoding, this is the actual data)
		totalFramesSize += int64(len(u))
	}

	return tool.SuccessResult(map[string]interface{}{
		"keyframes_data_urls":  urlsInterface,
		"transcription":        transcription,
		"frame_count":          frameCount,
		"strategy_used":        strategy,
		"extraction_method":    extractionMethod,
		"frame_resolution":     "720p",
		"total_frames_size_mb": float64(totalFramesSize) / (1024 * 1024),
		"transcription_length": len([]rune(transcription)),
		"duration_sec":         meta.DurationSec,
		"has_audio":            meta.HasAudio,
	})
}

func (t *VideoAnalyzerTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"cached_video_path": {
				Type:        "string",
				Description: "video_metadata工具输出的本地缓存视频路径（优先使用）",
				Required:    false,
			},
			"media_id": {
				Type:        "string",
				Description: "视频媒体资产ID（cached_video_path为空时使用，会重新下载）",
				Required:    false,
			},
			"max_keyframes": {
				Type:        "number",
				Description: "最大提取关键帧数量（默认16，最大30）",
				Required:    false,
				Default:     16,
			},
			"strategy": {
				Type:        "string",
				Description: "处理策略：auto(自动), audio(优先音频), visual(优先视觉), balanced(综合)",
				Required:    false,
				Default:     "auto",
				Enum:        []string{"auto", "audio", "visual", "balanced"},
			},
		},
		Output: map[string]tool.ParamDef{
			"keyframes_data_urls":  {Type: "array", Description: "关键帧base64数据URL数组（JPEG压缩，720p缩放）"},
			"transcription":        {Type: "string", Description: "Whisper音频转录文本（人物对话，已过滤背景音乐）"},
			"frame_count":          {Type: "number", Description: "实际提取的关键帧数量"},
			"strategy_used":        {Type: "string", Description: "实际使用的处理策略"},
			"extraction_method":    {Type: "string", Description: "关键帧提取方法（scene_detect 或 uniform_sampling）"},
			"frame_resolution":     {Type: "string", Description: "关键帧缩放分辨率（如 720p）"},
			"total_frames_size_mb": {Type: "number", Description: "所有关键帧base64 data URL总大小（MB）"},
			"transcription_length": {Type: "number", Description: "转录文本字符数"},
			"duration_sec":         {Type: "number", Description: "视频时长（秒）"},
			"has_audio":            {Type: "boolean", Description: "是否有音频轨道"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"cached_video_path": "/tmp/tangying-video-cache/media-xxx", "strategy": "balanced"},
				Output: map[string]interface{}{"frame_count": 12, "transcription": "今天给大家分享一个实用技巧...", "has_audio": true},
			},
		},
	}
}

func (t *VideoAnalyzerTool) downloadToCache(ctx context.Context, mediaID string) (string, error) {
	cachedPath := getCachedVideoPath(mediaID)
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	if err := os.MkdirAll(videoCacheDir, 0700); err != nil {
		return "", err
	}

	presignedURL, err := t.mediaSvc.GetURL(ctx, mediaID)
	if err != nil {
		return "", err
	}

	if err := downloadFile(ctx, presignedURL, cachedPath); err != nil {
		return "", err
	}

	return cachedPath, nil
}

// --- Key frame extraction ---

func extractKeyFrames(ctx context.Context, videoPath, framesDir string, maxCount int, meta *videoMeta) (int, string, error) {
	scaleFilter := "'min(720,iw):min(720,ih):force_original_aspect_ratio=decrease'"

	// Approach 1: scene detection
	selectFilter := "select='gt(scene,0.3)',scale=" + scaleFilter
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", selectFilter,
		"-vsync", "vfr",
		"-frames:v", strconv.Itoa(maxCount),
		"-q:v", "3",
		"-y",
		filepath.Join(framesDir, "frame_%03d.jpg"),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		zap.L().Warn("ffmpeg scene detection failed", zap.Error(err), zap.String("stderr", stderr.String()))
	}

	frames, _ := filepath.Glob(filepath.Join(framesDir, "frame_*.jpg"))
	if len(frames) >= 3 {
		return len(frames), "scene_detect", nil
	}

	// Approach 2: uniform sampling fallback
	os.RemoveAll(framesDir)
	os.MkdirAll(framesDir, 0700)

	if meta.FPS <= 0 {
		meta.FPS = 30
	}
	totalFrames := int(meta.DurationSec * meta.FPS)
	if totalFrames < maxCount {
		maxCount = totalFrames
	}
	if maxCount < 1 {
		maxCount = 1
	}

	interval := totalFrames / maxCount
	if interval < 1 {
		interval = 1
	}

	selectFilter = fmt.Sprintf("select='not(mod(n,%d))',scale="+scaleFilter, interval)
	cmd2 := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", selectFilter,
		"-vsync", "vfr",
		"-frames:v", strconv.Itoa(maxCount),
		"-q:v", "3",
		"-y",
		filepath.Join(framesDir, "frame_%03d.jpg"),
	)
	var stderr2 bytes.Buffer
	cmd2.Stderr = &stderr2
	if err := cmd2.Run(); err != nil {
		zap.L().Warn("ffmpeg uniform sampling failed", zap.Error(err), zap.String("stderr", stderr2.String()))
	}

	frames, _ = filepath.Glob(filepath.Join(framesDir, "frame_*.jpg"))
	return len(frames), "uniform_sampling", nil
}

// --- Audio extraction ---

func extractAudio(ctx context.Context, videoPath, audioPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "2",
		"-ar", "16000",
		"-ac", "1",
		"-map", "0:a:0",
		"-y",
		audioPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err.Error(), stderr.String())
	}
	return nil
}

// --- Frame encoding ---

func compressAndEncodeFrames(framesDir string) ([]string, error) {
	entries, err := os.ReadDir(framesDir)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(framesDir, entry.Name()))
		if err != nil {
			zap.L().Warn("Failed to read frame", zap.String("name", entry.Name()), zap.Error(err))
			continue
		}
		compressed, mimeType := llmutil.CompressImageBytes(data)
		dataURL := llmutil.Base64DataURL(mimeType, compressed)
		urls = append(urls, dataURL)
	}
	return urls, nil
}

// --- Whisper Transcription ---

func callTranscription(ctx context.Context, cfg config.OpenAIConfig, audioData []byte) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("model", "whisper-1")
	_ = writer.WriteField("response_format", "text")
	_ = writer.WriteField("language", "zh")
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audioData); err != nil {
		return "", err
	}
	writer.Close()

	baseURL := strings.TrimRight(cfg.BaseURL, "/") + "/"
	endpoint := baseURL + "audio/transcriptions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transcription API returned %d: %s", resp.StatusCode, string(body))
	}

	return strings.TrimSpace(string(body)), nil
}

// --- Strategy ---

func resolveStrategy(paramStrategy string, meta *videoMeta) string {
	if paramStrategy != "auto" {
		return paramStrategy
	}
	if meta.HasAudio && meta.AudioDur >= 5.0 {
		return "balanced"
	}
	return "visual"
}
