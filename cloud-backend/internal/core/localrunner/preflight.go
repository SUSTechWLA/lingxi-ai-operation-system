package localrunner

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
	id               string
	requiredCommands []string
	optionalCommands []string
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

		profile := preflightProfileForPipeline(c.Query("pipeline"))
		resp := PreflightResponse{
			Pipeline: profile.id,
			Status:   "passed",
			CanStart: true,
		}

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
		for _, cmd := range profile.requiredCommands {
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
		for _, cmd := range profile.optionalCommands {
			supported, err := svc.SupportsCommand(c.Request.Context(), uid, cmd)
			resp.CapabilityMenu.LocalTools = append(resp.CapabilityMenu.LocalTools, LocalToolStatus{
				Command:   cmd,
				Available: err == nil && supported,
			})
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

func preflightProfileForPipeline(raw string) preflightPipelineProfile {
	switch strings.TrimSpace(raw) {
	case "wf-aigc-shot-video", "aigc-shot-video", "cinematic-aigc-shot-video":
		return preflightPipelineProfile{
			id: "wf-aigc-shot-video",
			requiredCommands: []string{
				"LOCAL_FILE_IMPORT",
				"FFMPEG_PROBE",
				"ARTIFACT_PACKAGE",
			},
			optionalCommands: []string{"LOCAL_MCP_TOOL_CALL"},
		}
	default:
		return preflightPipelineProfile{
			id: "wf-guided-image-text-video",
			requiredCommands: []string{
				"HYPERFRAMES_PROJECT_GENERATE",
				"HYPERFRAMES_SNAPSHOT",
				"HYPERFRAMES_RENDER",
				"FFMPEG_PROBE",
				"ARTIFACT_PACKAGE",
			},
		}
	}
}
