package tool

import (
	"context"
	"sync"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/executor"
)

type ToolType string

const (
	ToolTypeLLM    ToolType = "LLM"
	ToolTypeCustom ToolType = "CUSTOM"
	ToolTypeCode   ToolType = "CODE"
)

type ToolResult struct {
	Success    bool                   `json:"success"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Error      string                 `json:"error,omitempty"`
	StartTime  interface{}            `json:"startTime"`
	EndTime    interface{}            `json:"endTime"`
}

func SuccessResult(data map[string]interface{}) ToolResult {
	return ToolResult{Success: true, Data: data}
}

func FailureResult(err string) ToolResult {
	return ToolResult{Success: false, Error: err}
}

// ProgressUpdate carries a progress report from a long-running tool.
type ProgressUpdate struct {
	Progress   float64                `json:"progress"`             // 0.0 ~ 1.0
	Step       string                 `json:"step"`                 // 当前步骤描述
	Checkpoint map[string]interface{} `json:"checkpoint,omitempty"` // 断点数据
}

// ProgressCallback is the function signature tools call to report progress.
type ProgressCallback func(ctx context.Context, update ProgressUpdate)

// ProgressReporter is an optional interface that long-running tools can implement
// to receive a callback for reporting execution progress to the orchestrator.
type ProgressReporter interface {
	SetProgressCallback(cb ProgressCallback)
}

type ToolContext struct {
	TaskID     string                 `json:"taskId"`
	NodeID     string                 `json:"nodeId"`
	RetryCount int                    `json:"retryCount"`
	Checkpoint map[string]interface{} `json:"checkpoint,omitempty"` // 上次执行的断点
}

type Tool interface {
	Name() string
	Description() string
	Type() ToolType
	Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult
	ValidateParameters(params map[string]interface{}) bool
}

type BuildableTool interface {
	Tool
	BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error)
}

type ExecutableTool interface {
	Tool
	Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult
}

// ManifestProvider is an optional interface for tools to provide their full metadata
// (parameters, output schema, sandbox requirements, examples) for the knowledge base.
type ManifestProvider interface {
	Tool
	Manifest() ToolManifest
}

// ExternalToolProvider is implemented by tools that can execute registered external tools.
type ExternalToolProvider interface {
	Tool
	// ExecuteExternal executes an external tool by name with the given parameters.
	ExecuteExternal(ctx context.Context, toolName string, params map[string]interface{}, toolCtx ToolContext) ToolResult
}

type ToolRegistry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	manifests map[string]*ToolManifest // external tool registrations
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:     make(map[string]Tool),
		manifests: make(map[string]*ToolManifest),
	}
}

func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) All() map[string]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Return a copy to avoid concurrent map access
	result := make(map[string]Tool, len(r.tools))
	for k, v := range r.tools {
		result[k] = v
	}
	return result
}

func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// --- External Tool Registration (Knowledge Base) ---

// RegisterExternal registers an external tool manifest. The tool can then be executed
// via the "external" built-in tool which routes to the registered endpoint.
func (r *ToolRegistry) RegisterExternal(manifest *ToolManifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	manifest.RegisteredAt = time.Now()
	r.manifests[manifest.Name] = manifest
}

// DeregisterExternal removes an external tool registration.
func (r *ToolRegistry) DeregisterExternal(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.manifests[name]
	if ok {
		delete(r.manifests, name)
	}
	return ok
}

// GetManifest returns the manifest for a tool (builtin or external).
func (r *ToolRegistry) GetManifest(name string) *ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check external manifests first
	if m, ok := r.manifests[name]; ok {
		return m
	}

	// Generate manifest from built-in tool
	if t, ok := r.tools[name]; ok {
		m := ManifestForTool(t)
		return &m
	}

	return nil
}

// ListManifests returns manifests for all registered tools (built-in + external).
func (r *ToolRegistry) ListManifests() []*ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ToolManifest, 0, len(r.tools)+len(r.manifests))

	// Add built-in tools
	for _, t := range r.tools {
		m := ManifestForTool(t)
		result = append(result, &m)
	}

	// Add external tools
	for _, m := range r.manifests {
		result = append(result, m)
	}

	return result
}

// GetExternalManifest returns the manifest for an external tool only.
func (r *ToolRegistry) GetExternalManifest(name string) *ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.manifests[name]
}

// ListExternalManifests returns all external tool manifests.
func (r *ToolRegistry) ListExternalManifests() []*ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ToolManifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		result = append(result, m)
	}
	return result
}

// --- Tool Routing ---

func DetermineToolName(nodeType string, payload map[string]interface{}) string {
	if tool, ok := payload["tool"]; ok {
		if s, ok := tool.(string); ok && s != "" {
			return s
		}
	}
	if nodeType == string(model.NodeTypeTool) {
		if name, ok := payload["name"]; ok {
			if s, ok := name.(string); ok && s != "" {
				return s
			}
		}
	}
	if nodeType == string(model.NodeTypeLLM) {
		return "llm_api"
	}
	return "llm_api"
}

func ExtractParameters(payload map[string]interface{}) map[string]interface{} {
	if params, ok := payload["parameters"]; ok {
		if m, ok := params.(map[string]interface{}); ok {
			return m
		}
	}
	if input, ok := payload["input"]; ok {
		if m, ok := input.(map[string]interface{}); ok {
			return m
		}
	}
	return payload
}

