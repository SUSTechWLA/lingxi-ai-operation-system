package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/tangying-ai-operation-system/internal/config"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/media"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/worker/tool"
)

// shared video cache to avoid re-downloading across tools
const videoCacheDir = "/tmp/tangying-video-cache"

type VideoMetadataTool struct {
	cfg      config.OpenAIConfig
	mediaSvc *media.MediaService
}

func NewVideoMetadataTool(cfg config.OpenAIConfig, mediaSvc *media.MediaService) *VideoMetadataTool {
	return &VideoMetadataTool{cfg: cfg, mediaSvc: mediaSvc}
}

func (t *VideoMetadataTool) Name() string { return "video_metadata" }
func (t *VideoMetadataTool) Description() string {
	return "下载视频并提取元数据（时长、分辨率、帧率、编码、音频轨道信息）。输出缓存的本地视频路径供下游工具使用。"
}
func (t *VideoMetadataTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *VideoMetadataTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["media_id"].(string)
	return ok
}

type videoMeta struct {
	DurationSec float64 `json:"duration_sec"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
	VideoCodec  string  `json:"video_codec"`
	HasAudio    bool    `json:"has_audio"`
	AudioCodec  string  `json:"audio_codec,omitempty"`
	AudioDur    float64 `json:"audio_duration_sec"`
	FileSizeMB  float64 `json:"file_size_mb"`
}

func (t *VideoMetadataTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	mediaID, _ := params["media_id"].(string)
	if mediaID == "" {
		return tool.FailureResult("media_id is required")
	}

	if t.mediaSvc == nil {
		return tool.FailureResult("media service is not available")
	}

	zap.L().Info("VideoMetadataTool starting",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("mediaId", mediaID),
	)

	// Check cache first
	cachedPath := getCachedVideoPath(mediaID)
	if _, err := os.Stat(cachedPath); os.IsNotExist(err) {
		presignedURL, err := t.mediaSvc.GetURL(ctx, mediaID)
		if err != nil {
			return tool.FailureResult("failed to resolve video URL: " + err.Error())
		}

		if err := os.MkdirAll(videoCacheDir, 0700); err != nil {
			return tool.FailureResult("failed to create cache dir: " + err.Error())
		}

		if err := downloadFile(ctx, presignedURL, cachedPath); err != nil {
			return tool.FailureResult("failed to download video: " + err.Error())
		}
	}

	meta, err := runFFprobe(ctx, cachedPath)
	if err != nil {
		return tool.FailureResult("ffprobe failed: " + err.Error())
	}

	if meta.DurationSec > 60 {
		return tool.FailureResult(fmt.Sprintf("仅支持60秒以内的短视频（当前 %.0f 秒）", meta.DurationSec))
	}

	zap.L().Info("VideoMetadataTool completed",
		zap.String("taskId", toolCtx.TaskID),
		zap.Float64("durationSec", meta.DurationSec),
		zap.Bool("hasAudio", meta.HasAudio),
	)

	metaJSON, _ := json.Marshal(meta)
	return tool.SuccessResult(map[string]interface{}{
		"metadata":           string(metaJSON),
		"cached_video_path":  cachedPath,
		"duration_sec":       meta.DurationSec,
		"width":              meta.Width,
		"height":             meta.Height,
		"fps":                meta.FPS,
		"video_codec":        meta.VideoCodec,
		"has_audio":          meta.HasAudio,
		"audio_codec":        meta.AudioCodec,
		"audio_duration_sec": meta.AudioDur,
		"file_size_mb":       meta.FileSizeMB,
	})
}

func (t *VideoMetadataTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"media_id": {
				Type:        "string",
				Description: "视频媒体资产ID（通过上传接口获得的media ID）",
				Required:    true,
			},
		},
		Output: map[string]tool.ParamDef{
			"metadata":           {Type: "string", Description: "视频元数据JSON（时长、分辨率、帧率、编码、音频信息）"},
			"cached_video_path":  {Type: "string", Description: "缓存的本地视频文件路径，供下游工具使用"},
			"duration_sec":       {Type: "number", Description: "视频时长（秒）"},
			"width":              {Type: "number", Description: "视频宽度（像素）"},
			"height":             {Type: "number", Description: "视频高度（像素）"},
			"fps":                {Type: "number", Description: "视频帧率"},
			"video_codec":        {Type: "string", Description: "视频编码格式（如 h264, hevc）"},
			"has_audio":          {Type: "boolean", Description: "是否有音频轨道"},
			"audio_codec":        {Type: "string", Description: "音频编码格式（如 aac, mp3）"},
			"audio_duration_sec": {Type: "number", Description: "音频时长（秒）"},
			"file_size_mb":       {Type: "number", Description: "文件大小（MB）"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"media_id": "media-1700000000000-myvideo"},
				Output: map[string]interface{}{"duration_sec": 30.5, "has_audio": true, "width": 1920, "height": 1080},
			},
		},
	}
}

// --- shared helpers ---

func getCachedVideoPath(mediaID string) string {
	return filepath.Join(videoCacheDir, mediaID)
}

func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
	} `json:"format"`
}

func runFFprobe(ctx context.Context, videoPath string) (*videoMeta, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		videoPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %s", err.Error(), stderr.String())
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("parse ffprobe JSON: %w", err)
	}

	meta := &videoMeta{}

	if d, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil {
		meta.DurationSec = d
	}
	if s, err := strconv.ParseInt(out.Format.Size, 10, 64); err == nil {
		meta.FileSizeMB = float64(s) / (1024 * 1024)
	}

	for _, stream := range out.Streams {
		switch stream.CodecType {
		case "video":
			meta.Width = stream.Width
			meta.Height = stream.Height
			meta.VideoCodec = stream.CodecName
			meta.FPS = parseFrameRate(stream.RFrameRate)
			if meta.DurationSec == 0 {
				if d, err := strconv.ParseFloat(stream.Duration, 64); err == nil {
					meta.DurationSec = d
				}
			}
		case "audio":
			meta.HasAudio = true
			meta.AudioCodec = stream.CodecName
			if d, err := strconv.ParseFloat(stream.Duration, 64); err == nil {
				meta.AudioDur = d
			}
		}
	}

	if meta.Width == 0 && meta.Height == 0 {
		return nil, fmt.Errorf("no video stream found in file")
	}

	return meta, nil
}

func parseFrameRate(rate string) float64 {
	parts := strings.Split(rate, "/")
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}
