package localrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localtool"
)

const (
	maxMCPDiagnosticBytes = 64 << 10
	maxMCPDiagnosticDepth = 12
	maxMCPDiagnosticItems = 64
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
	DeviceID                  string
	RunnerVersion             string
	WorkspaceRoot             string
	DataDir                   string
	PollInterval              time.Duration
	MCPToolCatalogSource      MCPToolCatalogSource
	MCPCatalogRefreshInterval time.Duration
}

type MCPToolCatalogSource interface {
	Discover(context.Context) (MCPToolCatalog, []MCPProviderDiagnostic)
}

type mcpToolCatalogFingerprinter interface {
	Fingerprint(context.Context) (string, error)
}

type Loop struct {
	client               CloudClient
	registry             *localtool.Registry
	options              LoopOptions
	runnerID             string
	sessionID            string
	pendingReports       *PendingReportStore
	capabilities         []Capability
	catalogRevision      string
	catalogDiscoveredAt  time.Time
	catalogFingerprint   string
	catalogNextRefreshAt time.Time
	catalogFailures      int
	now                  func() time.Time
}

func NewLoop(client CloudClient, registry *localtool.Registry, options LoopOptions) *Loop {
	if options.PollInterval <= 0 {
		options.PollInterval = 3 * time.Second
	}
	if options.MCPCatalogRefreshInterval <= 0 {
		options.MCPCatalogRefreshInterval = 10 * time.Minute
	}
	return &Loop{
		client:         client,
		registry:       registry,
		options:        options,
		pendingReports: NewPendingReportStore(options.DataDir),
		now:            time.Now,
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
	heartbeat := HeartbeatRequest{
		SessionID: l.sessionID,
		Status:    "online",
	}
	if capabilities := l.refreshMCPToolCatalog(ctx); capabilities != nil {
		heartbeat.Capabilities = &capabilities
	}
	if err := l.client.Heartbeat(ctx, l.runnerID, heartbeat); err != nil {
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
	l.capabilities = l.executableCapabilities(probe)
	if l.options.MCPToolCatalogSource != nil {
		fingerprint, fingerprintErr := l.mcpCatalogFingerprint(ctx)
		catalog, diagnostics := l.options.MCPToolCatalogSource.Discover(ctx)
		l.logMCPDiagnostics(diagnostics)
		l.capabilities = withMCPToolCatalog(l.capabilities, catalog)
		l.catalogRevision = catalog.Revision
		now := l.currentTime()
		l.catalogDiscoveredAt = now
		l.catalogFingerprint = fingerprint
		if fingerprintErr != nil || catalogDiscoveryFailed(diagnostics) {
			l.scheduleCatalogFailure(now)
		} else {
			l.scheduleCatalogSuccess(now)
		}
	}
	resp, err := l.client.Register(ctx, RegisterRunnerRequest{
		DeviceID:      l.options.DeviceID,
		RunnerVersion: l.options.RunnerVersion,
		Platform:      probe.Platform,
		WorkspaceRoot: l.options.WorkspaceRoot,
		Capabilities:  l.capabilities,
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

func (l *Loop) refreshMCPToolCatalog(ctx context.Context) []Capability {
	if l.options.MCPToolCatalogSource == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	now := l.currentTime()
	fingerprint, fingerprintErr := l.mcpCatalogFingerprint(ctx)
	if fingerprintErr != nil {
		if !now.Before(l.catalogNextRefreshAt) {
			l.catalogFingerprint = fingerprint
			l.scheduleCatalogFailure(now)
		}
		return nil
	}
	configurationChanged := fingerprint != "" && fingerprint != l.catalogFingerprint
	if !configurationChanged && !l.catalogNextRefreshAt.IsZero() && now.Before(l.catalogNextRefreshAt) {
		return nil
	}
	catalog, diagnostics := l.options.MCPToolCatalogSource.Discover(ctx)
	if err := ctx.Err(); err != nil {
		return nil
	}
	l.catalogDiscoveredAt = now
	l.catalogFingerprint = fingerprint
	l.logMCPDiagnostics(diagnostics)
	if catalogDiscoveryFailed(diagnostics) {
		l.scheduleCatalogFailure(now)
	} else {
		l.scheduleCatalogSuccess(now)
	}
	if catalog.Revision == l.catalogRevision {
		return nil
	}
	l.catalogRevision = catalog.Revision
	l.capabilities = withMCPToolCatalog(l.capabilities, catalog)
	return append([]Capability(nil), l.capabilities...)
}

func (l *Loop) currentTime() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (l *Loop) mcpCatalogFingerprint(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fingerprinter, ok := l.options.MCPToolCatalogSource.(mcpToolCatalogFingerprinter)
	if !ok {
		return "", nil
	}
	return fingerprinter.Fingerprint(ctx)
}

func (l *Loop) scheduleCatalogSuccess(now time.Time) {
	l.catalogFailures = 0
	l.catalogNextRefreshAt = now.Add(l.options.MCPCatalogRefreshInterval)
}

func (l *Loop) scheduleCatalogFailure(now time.Time) {
	l.catalogFailures++
	backoff := time.Minute
	for i := 1; i < l.catalogFailures && backoff < l.options.MCPCatalogRefreshInterval; i++ {
		backoff *= 2
	}
	if backoff > l.options.MCPCatalogRefreshInterval {
		backoff = l.options.MCPCatalogRefreshInterval
	}
	l.catalogNextRefreshAt = now.Add(backoff)
}

func catalogDiscoveryFailed(diagnostics []MCPProviderDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case "PROVIDER_CONFIG_UNAVAILABLE", "TOOLS_LIST_FAILED", "CATALOG_PAYLOAD_LIMIT":
			return true
		}
	}
	return false
}

func withMCPToolCatalog(capabilities []Capability, catalog MCPToolCatalog) []Capability {
	result := append([]Capability(nil), capabilities...)
	for index := range result {
		if localtool.NormalizeCommand(result[index].Command) != localtool.CommandLocalMCPToolCall {
			continue
		}
		result[index].CatalogRevision = catalog.Revision
		result[index].MCPTools = append([]MCPToolAdvertisement(nil), catalog.Tools...)
		break
	}
	return result
}

func (l *Loop) logMCPDiagnostics(diagnostics []MCPProviderDiagnostic) {
	for _, diagnostic := range diagnostics {
		log.Printf("MCP catalog diagnostic provider=%q code=%s: %s", diagnostic.ProviderID, diagnostic.Code, diagnostic.Message)
	}
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
	case localtool.CommandLocalIpTalkingAvatarRender:
		return "local_ip_talking_avatar_render"
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
	if localtool.NormalizeCommand(job.Command) == localtool.CommandLocalMCPToolCall && mcpResultIsError(output) {
		failReq := FailJobRequest{
			Success: false, Retryable: true,
			Error: map[string]interface{}{
				"code":    "MCP_TOOL_ERROR",
				"message": mcpResultErrorMessage(output),
			},
			Diagnostics: boundedMCPResultDiagnostics(output),
		}
		_ = l.pendingReports.Save(PendingReport{JobID: job.ID, Type: "fail", Fail: &failReq})
		reportErr := l.client.FailJob(ctx, job.ID, failReq)
		if reportErr == nil {
			_ = l.pendingReports.Remove(job.ID, "fail")
		}
		return reportErr
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

func mcpResultIsError(output map[string]interface{}) bool {
	isError, _ := output["isError"].(bool)
	return isError
}

func mcpResultErrorMessage(output map[string]interface{}) string {
	if message, ok := output["error"].(string); ok && strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	if content, ok := output["content"].([]interface{}); ok {
		for _, item := range content {
			if block, ok := item.(map[string]interface{}); ok {
				if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	return "MCP tool returned isError=true"
}

func boundedMCPResultDiagnostics(output map[string]interface{}) map[string]interface{} {
	selected := make(map[string]interface{}, 4)
	for _, key := range []string{"content", "structuredContent", "meta", "raw"} {
		if value, ok := output[key]; ok {
			selected[key] = value
		}
	}
	// Reserve JSON/container overhead in addition to the recursively-accounted
	// values so the encoded diagnostic remains below the public limit.
	budget := maxMCPDiagnosticBytes - (16 << 10)
	sanitized, _ := boundedDiagnosticValue(selected, 0, &budget).(map[string]interface{})
	if sanitized == nil {
		sanitized = map[string]interface{}{}
	}
	return map[string]interface{}{"mcpResult": sanitized, "truncated": budget <= 0}
}

func boundedDiagnosticValue(value interface{}, depth int, budget *int) interface{} {
	if *budget <= 0 {
		return "<truncated>"
	}
	if depth >= maxMCPDiagnosticDepth {
		*budget -= len("<max-depth>")
		return "<max-depth>"
	}
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int64, int32, uint, uint64, json.Number:
		return typed
	case string:
		limit := len(typed)
		if limit > 4096 {
			limit = 4096
		}
		if limit > *budget {
			limit = *budget
		}
		*budget -= limit
		if limit < len(typed) {
			return typed[:limit] + "<truncated>"
		}
		return typed
	case []byte:
		return boundedDiagnosticValue(string(typed), depth+1, budget)
	case []interface{}:
		limit := len(typed)
		if limit > maxMCPDiagnosticItems {
			limit = maxMCPDiagnosticItems
		}
		result := make([]interface{}, 0, limit+1)
		for index := 0; index < limit && *budget > 0; index++ {
			result = append(result, boundedDiagnosticValue(typed[index], depth+1, budget))
		}
		if limit < len(typed) {
			result = append(result, "<truncated-items>")
		}
		return result
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > maxMCPDiagnosticItems {
			keys = keys[:maxMCPDiagnosticItems]
		}
		result := make(map[string]interface{}, len(keys))
		for index, key := range keys {
			if *budget <= 0 {
				break
			}
			safeKey := key
			if len(safeKey) > 128 {
				safeKey = safeKey[:128] + "<truncated-key>"
			}
			if _, exists := result[safeKey]; exists {
				safeKey = fmt.Sprintf("%s#%d", safeKey, index)
			}
			*budget -= len(safeKey)
			result[safeKey] = boundedDiagnosticValue(typed[key], depth+1, budget)
		}
		return result
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("<unsupported:%T>", typed)
		}
		var generic interface{}
		if err := json.Unmarshal(encoded, &generic); err != nil {
			return boundedDiagnosticValue(string(encoded), depth+1, budget)
		}
		return boundedDiagnosticValue(generic, depth+1, budget)
	}
}
