package localtool

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type Job struct {
	ID             string                 `json:"jobId"`
	ProjectID      string                 `json:"projectId,omitempty"`
	NodeID         string                 `json:"nodeId,omitempty"`
	ToolName       string                 `json:"toolName,omitempty"`
	Command        string                 `json:"command"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	TimeoutSec     int                    `json:"timeoutSec,omitempty"`
	ArtifactPolicy map[string]interface{} `json:"artifactPolicy,omitempty"`
}

type Result struct {
	Output map[string]interface{} `json:"output"`
}

type Executor interface {
	Execute(ctx context.Context, job Job) (*Result, error)
}

type ExecutorFunc func(ctx context.Context, job Job) (*Result, error)

func (f ExecutorFunc) Execute(ctx context.Context, job Job) (*Result, error) {
	return f(ctx, job)
}

type Registry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{executors: map[string]Executor{}}
}

func (r *Registry) Register(executor Executor, commands ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, command := range commands {
		normalized := NormalizeCommand(command)
		if IsAllowedCommand(normalized) {
			r.executors[normalized] = executor
		}
	}
}

func (r *Registry) CanExecute(command string) bool {
	normalized := NormalizeCommand(command)
	if !IsAllowedCommand(normalized) {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.executors[normalized]
	return ok
}

func (r *Registry) RegisteredCommands() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	commands := make([]string, 0, len(r.executors))
	for command := range r.executors {
		commands = append(commands, command)
	}
	return commands
}

func (r *Registry) Execute(ctx context.Context, job Job) (*Result, error) {
	normalized := NormalizeCommand(job.Command)
	if !IsAllowedCommand(normalized) {
		return nil, fmt.Errorf("local command not allowed: %s", job.Command)
	}
	r.mu.RLock()
	executor := r.executors[normalized]
	r.mu.RUnlock()
	if executor == nil {
		return nil, fmt.Errorf("local command not registered: %s", normalized)
	}
	job.Command = normalized
	return executor.Execute(ctx, job)
}

func NormalizeCommand(command string) string {
	return strings.ToUpper(strings.TrimSpace(command))
}

func IsAllowedCommand(command string) bool {
	return allowedCommands[NormalizeCommand(command)]
}

// Local command constants — aligned with cloud-backend ValidCommands.
const (
	CommandHyperFramesProjectGenerate = "HYPERFRAMES_PROJECT_GENERATE"
	CommandHyperFramesRender          = "HYPERFRAMES_RENDER"
	CommandHyperFramesLint            = "HYPERFRAMES_LINT"
	CommandHyperFramesSnapshot        = "HYPERFRAMES_SNAPSHOT"
	CommandHyperGenRender             = "HYPERGEN_RENDER"
	CommandFFmpegProbe                = "FFMPEG_PROBE"
	CommandFFmpegClipExtract          = "FFMPEG_CLIP_EXTRACT"
	CommandFFmpegAssemble             = "FFMPEG_ASSEMBLE"
	CommandAudioExtract               = "AUDIO_EXTRACT"
	CommandAudioNormalize             = "AUDIO_NORMALIZE"
	CommandASRTranscribe              = "ASR_TRANSCRIBE"
	CommandArtifactPackage            = "ARTIFACT_PACKAGE"
	CommandLocalFileImport            = "LOCAL_FILE_IMPORT"
	CommandLocalMediaIndex            = "LOCAL_MEDIA_INDEX"
	CommandBundleExtract              = "BUNDLE_EXTRACT"
)

var allowedCommands = map[string]bool{
	CommandHyperFramesProjectGenerate: true,
	CommandHyperFramesRender:          true,
	CommandHyperFramesLint:            true,
	CommandHyperFramesSnapshot:        true,
	CommandHyperGenRender:             true,
	CommandFFmpegProbe:                true,
	CommandFFmpegClipExtract:          true,
	CommandFFmpegAssemble:             true,
	CommandAudioExtract:               true,
	CommandAudioNormalize:             true,
	CommandASRTranscribe:              true,
	CommandArtifactPackage:            true,
	CommandLocalFileImport:            true,
	CommandLocalMediaIndex:            true,
	CommandBundleExtract:              true,
}
