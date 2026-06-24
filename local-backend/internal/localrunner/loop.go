package localrunner

import (
	"context"
	"fmt"
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
	for {
		if err := l.RunOnce(ctx); err != nil {
			return err
		}
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
	_ = l.client.Heartbeat(ctx, l.runnerID, HeartbeatRequest{
		SessionID: l.sessionID,
		Status:    "online",
	})
	// Flush any pending reports from previous failed deliveries
	_ = l.flushPendingReports(ctx)
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
	case "HYPERFRAMES_PROJECT_GENERATE":
		return "hyperframes_project_generator"
	case "HYPERFRAMES_RENDER":
		return "hyperframes_renderer"
	case "HYPERFRAMES_LINT":
		return "hyperframes_linter"
	case "HYPERFRAMES_SNAPSHOT":
		return "hyperframes_snapshot"
	case "FFMPEG_PROBE":
		return "ffmpeg_probe"
	case "FFMPEG_CLIP_EXTRACT":
		return "ffmpeg_clip_extractor"
	case "FFMPEG_ASSEMBLE":
		return "ffmpeg_assembler"
	case "AUDIO_EXTRACT":
		return "audio_extractor"
	case "AUDIO_NORMALIZE":
		return "audio_normalizer"
	case "ASR_TRANSCRIBE":
		return "asr_transcriber_local"
	case "ARTIFACT_PACKAGE":
		return "artifact_packager"
	case "LOCAL_FILE_IMPORT":
		return "local_file_importer"
	case "LOCAL_MEDIA_INDEX":
		return "local_media_indexer"
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
	_ = l.client.ReportProgress(ctx, job.ID, ProgressRequest{
		Status:   "running",
		Progress: 0.01,
		Step:     "started",
		Message:  "Local job started",
	})
	result, err := l.registry.Execute(ctx, job)
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
			if err := l.client.CompleteJob(ctx, report.JobID, *report.Complete); err == nil {
				_ = l.pendingReports.Remove(report.JobID, "complete")
			}
		case "fail":
			if report.Fail == nil {
				_ = l.pendingReports.Remove(report.JobID, "fail")
				continue
			}
			if err := l.client.FailJob(ctx, report.JobID, *report.Fail); err == nil {
				_ = l.pendingReports.Remove(report.JobID, "fail")
			}
		default:
			_ = l.pendingReports.Remove(report.JobID, report.Type)
		}
	}
	return nil
}
