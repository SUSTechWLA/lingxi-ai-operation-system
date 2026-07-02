package localrunner

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	PipelineGuidedImageTextVideo = "wf-guided-image-text-video"
	PipelineAIGCShotVideo        = "wf-aigc-shot-video"
)

// PreflightResponse is the capability check result for starting a video pipeline.
type PreflightResponse struct {
	Pipeline       string             `json:"pipeline"`
	Status         string             `json:"status"` // "passed" | "blocked"
	CanStart       bool               `json:"canStart"`
	CapabilityMenu CapabilityMenu     `json:"capabilityMenu"`
	Blockers       []PreflightBlocker `json:"blockers,omitempty"`
}

type CapabilityMenu struct {
	LocalRunner        LocalRunnerStatus  `json:"localRunner"`
	CompositionRuntime CompositionRuntime `json:"compositionRuntime"`
	LocalTools         []LocalToolStatus  `json:"localTools"`
	Warnings           []string           `json:"warnings"`
}

type LocalRunnerStatus struct {
	Available bool   `json:"available"`
	RunnerID  string `json:"runnerId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type CompositionRuntime struct {
	HyperFrames RuntimeStatus `json:"hyperframes"`
}

type RuntimeStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type LocalToolStatus struct {
	Command   string `json:"command"`
	Available bool   `json:"available"`
}

type PreflightBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type preflightPipelineProfile struct {
	ID               string
	RequiredCommands []string
	Warnings         []string
}

// PreflightService provides capability checks for video pipeline startup.
type PreflightService interface {
	HasOnlineRunner(ctx context.Context, userID string) (bool, error)
	SupportsCommand(ctx context.Context, userID string, command string) (bool, error)
}

// HandleVideoPreflight returns a Gin handler for GET /api/video/preflight.
func HandleVideoPreflight(svc PreflightService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("userID") // set by auth middleware
		uid, _ := userID.(string)

		profile, ok := resolvePreflightPipeline(c.Query("pipeline"))
		if !ok {
			writePreflightError(c, http.StatusBadRequest, "UNKNOWN_VIDEO_PIPELINE", "未知视频流水线: "+c.Query("pipeline"))
			return
		}
		resp := PreflightResponse{
			Pipeline: profile.ID,
			Status:   "passed",
			CanStart: true,
		}
		resp.CapabilityMenu.Warnings = append(resp.CapabilityMenu.Warnings, profile.Warnings...)

		var blockers []PreflightBlocker

		// 1. Check local runner
		hasRunner, err := svc.HasOnlineRunner(c.Request.Context(), uid)
		if err != nil || !hasRunner {
			blockers = append(blockers, PreflightBlocker{
				Code:    "LOCAL_RUNNER_NOT_AVAILABLE",
				Message: "本地执行器未启动，无法执行视频渲染。请启动桌面端 local-backend。",
			})
			resp.CapabilityMenu.LocalRunner = LocalRunnerStatus{Available: false, Reason: "未检测到在线 runner"}
		} else {
			resp.CapabilityMenu.LocalRunner = LocalRunnerStatus{Available: true}
		}

		// 2. Check required local commands
		for _, cmd := range profile.RequiredCommands {
			supported, err := svc.SupportsCommand(c.Request.Context(), uid, cmd)
			available := err == nil && supported
			resp.CapabilityMenu.LocalTools = append(resp.CapabilityMenu.LocalTools, LocalToolStatus{
				Command:   cmd,
				Available: available,
			})
			if !available {
				blockers = append(blockers, PreflightBlocker{
					Code:    cmd + "_NOT_AVAILABLE",
					Message: "本地未检测到 " + cmd + " 执行能力",
				})
			}
		}

		// 3. Composition runtime — HyperFrames is the only runtime for v1
		resp.CapabilityMenu.CompositionRuntime = CompositionRuntime{
			HyperFrames: RuntimeStatus{
				Available: len(blockers) <= 1, // at most runner-not-available
				Reason:    "HyperFrames Render Service 通过本地 runner 注册的能力检测",
			},
		}

		if len(blockers) > 0 {
			resp.Status = "blocked"
			resp.CanStart = false
			resp.Blockers = blockers
		}

		c.JSON(http.StatusOK, resp)
	}
}

func resolvePreflightPipeline(raw string) (preflightPipelineProfile, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		normalized = PipelineGuidedImageTextVideo
	}
	switch normalized {
	case PipelineGuidedImageTextVideo, "guided-image-text-video", "voice_visual", "voice-visual", "talking_head", "talking-head":
		return preflightPipelineProfile{
			ID: PipelineGuidedImageTextVideo,
			RequiredCommands: []string{
				CommandHyperFramesProjectGenerate,
				CommandHyperFramesSnapshot,
				CommandHyperFramesRender,
				CommandFFmpegProbe,
				CommandArtifactPackage,
			},
		}, true
	case PipelineAIGCShotVideo, "aigc-shot-video", "aigc_shot", "cinematic_story", "cinematic-story", "film_story", "film-story":
		return preflightPipelineProfile{
			ID: PipelineAIGCShotVideo,
			RequiredCommands: []string{
				CommandHyperFramesProjectGenerate,
				CommandHyperFramesSnapshot,
				CommandHyperFramesRender,
				CommandFFmpegProbe,
				CommandArtifactPackage,
			},
			Warnings: []string{
				"影视类视频会生成可审核的参考资产、关键帧和 AIGC 视频请求；文生图/图生视频 Provider 由桌面端本地配置后随运行请求透传，未配置时需要用户外部生成并回填素材。",
			},
		}, true
	default:
		return preflightPipelineProfile{}, false
	}
}

func writePreflightError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
