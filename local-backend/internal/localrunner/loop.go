package localrunner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localtool"
)

type CloudClient interface {
	Register(context.Context, RegisterRunnerRequest) (*RegisterRunnerResponse, error)
	Heartbeat(context.Context, string, HeartbeatRequest) error
	ClaimJob(context.Context, string) (*localtool.Job, error)
	ReportProgress(context.Context, string, ProgressRequest) error
	CompleteJob(context.Context, string, CompleteJobRequest) error
	FailJob(context.Context, string, FailJobRequest) error
}

type LoopOptions struct {
	DeviceID      string
	RunnerVersion string
	WorkspaceRoot string
	DataDir       string
	PollInterval  time.Duration
}

type Loop struct {
	client         CloudClient
	registry       *localtool.Registry
	options        LoopOptions
	runnerID       string
	sessionID      string
	pendingReports *PendingReportStore
}

func NewLoop(client CloudClient, registry *localtool.Registry, options LoopOptions) *Loop {
	if options.PollInterval <= 0 {
		options.PollInterval = 3 * time.Second
	}
	return &Loop{
		client:         client,
		registry:       registry,
		options:        options,
		pendingReports: NewPendingReportStore(options.DataDir),
	}
}

func (l *Loop) Run(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if err := l.RunOnce(ctx); err != nil {
			// If the context is done, exit cleanly.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Transient error — back off and retry instead of crashing.
			log.Printf("local runner loop error (will retry in %v): %v", backoff, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Reset backoff on a successful cycle.
		backoff = time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.options.PollInterval):
		}
	}
}

func (l *Loop) RunOnce(ctx context.Context) error {
	if err := l.ensureRegistered(ctx); err != nil {
		return err
	}
	if err := l.client.Heartbeat(ctx, l.runnerID, HeartbeatRequest{
		SessionID: l.sessionID,
		Status:    "online",
	}); err != nil {
		log.Printf("heartbeat error: %v", err)
		// Heartbeat failures are non-fatal; don't break the loop.
	}
	// Flush any pending reports from previous failed deliveries
	if err := l.flushPendingReports(ctx); err != nil {
		log.Printf("flush pending reports error: %v", err)
	}
	job, err := l.client.ClaimJob(ctx, l.runnerID)
	if err != nil || job == nil {
		return err
	}
	return l.executeAndReport(ctx, *job)
}

func (l *Loop) ensureRegistered(ctx context.Context) error {
	if l.runnerID != "" {
		return nil
	}
	probe := Probe(ctx, l.options.WorkspaceRoot)
	resp, err := l.client.Register(ctx, RegisterRunnerRequest{
		DeviceID:      l.options.DeviceID,
		RunnerVersion: l.options.RunnerVersion,
		Platform:      probe.Platform,
		WorkspaceRoot: l.options.WorkspaceRoot,
		Capabilities:  l.executableCapabilities(probe),
	})
	if err != nil {
		return err
	}
	l.runnerID = resp.RunnerID
	l.sessionID = resp.SessionID
	if resp.PollIntervalSec > 0 {
		l.options.PollInterval = time.Duration(resp.PollIntervalSec) * time.Second
	}
	return nil
}

func (l *Loop) executableCapabilities(probe ProbeResult) []Capability {
	probed := map[string]Capability{}
	for _, capability := range probe.Capabilities {
		if capability.Available {
			probed[localtool.NormalizeCommand(capability.Command)] = capability
		}
	}

	var result []Capability
	for _, command := range l.registry.RegisteredCommands() {
		normalized := localtool.NormalizeCommand(command)
		if capability, ok := probed[normalized]; ok {
			result = append(result, capability)
			continue
		}
		result = append(result, Capability{
			ToolName:  toolNameForCommand(normalized),
			Command:   normalized,
			Available: true,
			Version:   "local-adapter",
		})
	}
	return result
}

func toolNameForCommand(command string) string {
	switch command {
	case localtool.CommandHyperFramesProjectGenerate:
		return "hyperframes_project_generator"
	case localtool.CommandHyperFramesRender:
		return "hyperframes_renderer"
	case localtool.CommandHyperFramesLint:
		return "hyperframes_linter"
	case localtool.CommandHyperFramesSnapshot:
		return "hyperframes_snapshot"
	case localtool.CommandHyperGenRender:
		return "hypergen_renderer"
	case localtool.CommandFFmpegProbe:
		return "ffmpeg_probe"
	case localtool.CommandFFmpegClipExtract:
		return "ffmpeg_clip_extractor"
	case localtool.CommandFFmpegAssemble:
		return "ffmpeg_assembler"
	case localtool.CommandAudioExtract:
		return "audio_extractor"
	case localtool.CommandAudioNormalize:
		return "audio_normalizer"
	case localtool.CommandASRTranscribe:
		return "asr_transcriber_local"
	case localtool.CommandArtifactPackage:
		return "artifact_packager"
	case localtool.CommandLocalFileImport:
		return "local_file_importer"
	case localtool.CommandLocalMediaIndex:
		return "local_media_indexer"
	case localtool.CommandBundleExtract:
		return "bundle_extractor"
	default:
		return command
	}
}

func (l *Loop) executeAndReport(ctx context.Context, job localtool.Job) error {
	if !l.registry.CanExecute(job.Command) {
		failReq := FailJobRequest{
			Success:   false,
			Retryable: false,
			Error: map[string]interface{}{
				"code":    "LOCAL_COMMAND_NOT_ALLOWED",
				"message": fmt.Sprintf("local command is not registered or allowed: %s", job.Command),
			},
		}
		// Persist pending report before cloud delivery
		_ = l.pendingReports.Save(PendingReport{JobID: job.ID, Type: "fail", Fail: &failReq})
		err := l.client.FailJob(ctx, job.ID, failReq)
		if err == nil {
			_ = l.pendingReports.Remove(job.ID, "fail")
		}
		return err
	}
	jobCtx := ctx
	cancelJob := func() {}
	if job.TimeoutSec > 0 {
		jobCtx, cancelJob = context.WithTimeout(ctx, time.Duration(job.TimeoutSec)*time.Second)
	}
	defer cancelJob()

	_ = l.client.ReportProgress(ctx, job.ID, ProgressRequest{
		Status:   "running",
		Progress: 0.01,
		Step:     "started",
		Message:  "Local job started",
	})

	// Start a heartbeat goroutine so the runner stays "online" during
	// long-running job execution (e.g. 10-minute video renders).
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := l.client.Heartbeat(hbCtx, l.runnerID, HeartbeatRequest{
					SessionID:   l.sessionID,
					Status:      "online",
					RunningJobs: 1,
				}); err != nil {
					log.Printf("heartbeat during execution failed: %v", err)
				}
			}
		}
	}()

	result, err := l.registry.Execute(jobCtx, job)
	if err != nil {
		failReq := FailJobRequest{
			Success:   false,
			Retryable: true,
			Error: map[string]interface{}{
				"code":    "LOCAL_TOOL_EXEC_FAILED",
				"message": err.Error(),
			},
		}
		_ = l.pendingReports.Save(PendingReport{JobID: job.ID, Type: "fail", Fail: &failReq})
		reportErr := l.client.FailJob(ctx, job.ID, failReq)
		if reportErr == nil {
			_ = l.pendingReports.Remove(job.ID, "fail")
		}
		return reportErr
	}
	output := map[string]interface{}{}
	if result != nil && result.Output != nil {
		output = result.Output
	}
	completeReq := CompleteJobRequest{Success: true, Output: output}
	_ = l.pendingReports.Save(PendingReport{JobID: job.ID, Type: "complete", Complete: &completeReq})
	reportErr := l.client.CompleteJob(ctx, job.ID, completeReq)
	if reportErr == nil {
		_ = l.pendingReports.Remove(job.ID, "complete")
	}
	return reportErr
}

// Shutdown flushes pending reports before the runner process exits.
// Call this when the Run loop exits cleanly (context cancelled).
func (l *Loop) Shutdown(ctx context.Context) error {
	return l.flushPendingReports(ctx)
}

// flushPendingReports retries delivering all persisted pending reports to the cloud.
func (l *Loop) flushPendingReports(ctx context.Context) error {
	reports, err := l.pendingReports.List()
	if err != nil || len(reports) == 0 {
		return err
	}
	for _, report := range reports {
		switch report.Type {
		case "complete":
			if report.Complete == nil {
				_ = l.pendingReports.Remove(report.JobID, "complete")
				continue
			}
			if err := l.client.CompleteJob(ctx, report.JobID, *report.Complete); err == nil || isTerminalPendingReportError(err) {
				_ = l.pendingReports.Remove(report.JobID, "complete")
			}
		case "fail":
			if report.Fail == nil {
				_ = l.pendingReports.Remove(report.JobID, "fail")
				continue
			}
			if err := l.client.FailJob(ctx, report.JobID, *report.Fail); err == nil || isTerminalPendingReportError(err) {
				_ = l.pendingReports.Remove(report.JobID, "fail")
			}
		default:
			_ = l.pendingReports.Remove(report.JobID, report.Type)
		}
	}
	return nil
}

func isTerminalPendingReportError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.StatusCode {
	case 403, 404, 409:
		return true
	default:
		return false
	}
}
