package localtool

import (
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

// ExecutorConfig holds configuration for all local tool executors.
type ExecutorConfig struct {
	// DataDir is the root directory for all local data (projects, artifacts, cache, logs).
	DataDir string

	// HyperFramesServiceURL is the base URL of the HyperFrames Render Service (e.g. http://127.0.0.1:8787).
	HyperFramesServiceURL string

	// RenderTimeoutSec is the maximum duration for a single HyperFrames render job in seconds.
	RenderTimeoutSec int

	// FFmpegPath is the path to the ffmpeg/ffprobe binaries. Leave empty to use PATH lookup.
	FFmpegPath string

	// MCPProviderLoader loads user-configured local MCP providers at execution time.
	MCPProviderLoader MCPProviderLoader
}

// RegisterDefaultExecutors registers all standard local tool executors on the
// given Registry using the provided configuration. Call this once at startup
// instead of registering each executor individually.
func RegisterDefaultExecutors(reg *Registry, cfg ExecutorConfig) error {
	guard := NewPathGuard(cfg.DataDir)

	renderTimeout := time.Duration(cfg.RenderTimeoutSec) * time.Second
	if renderTimeout <= 0 {
		renderTimeout = 30 * time.Minute
	}

	reg.Register(
		NewHyperFramesProjectExecutor(cfg.DataDir),
		CommandHyperFramesProjectGenerate,
	)
	snapshotTimeout := 5 * time.Minute
	reg.Register(
		NewHyperFramesSnapshotExecutor(cfg.DataDir, cfg.HyperFramesServiceURL, snapshotTimeout),
		CommandHyperFramesSnapshot,
	)
	reg.Register(
		NewHyperFramesRenderExecutor(cfg.DataDir, cfg.HyperFramesServiceURL, renderTimeout),
		CommandHyperFramesRender,
	)
	reg.Register(
		NewFFmpegProbeExecutor(cfg.DataDir),
		CommandFFmpegProbe,
	)
	reg.Register(
		NewArtifactPackageExecutor(cfg.DataDir),
		CommandArtifactPackage,
	)
	reg.Register(
		NewLocalFileImportExecutor(cfg.DataDir),
		CommandLocalFileImport,
	)
	reg.Register(
		NewFinalReviewExecutor(guard, cfg.DataDir),
		CommandFinalReview,
	)
	reg.Register(
		NewVideoFrameQAExecutor(cfg.DataDir),
		CommandVideoFrameQA,
	)
	if cfg.MCPProviderLoader != nil {
		reg.Register(
			NewMCPToolCallExecutor(cfg.MCPProviderLoader),
			CommandLocalMCPToolCall,
		)
	}

	// Prevent unused variable warning — guard is passed to each New*Executor
	// constructor above for executors that accept a *PathGuard directly.
	// Future executors added here should also use guard for path safety.
	_ = guard

	return nil
}

type MCPProviderLoader func() ([]localmcp.ProviderConfig, error)
