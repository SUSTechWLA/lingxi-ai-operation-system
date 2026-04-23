package tool

import (
	"context"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
)

type ToolType string

const (
	ToolTypeLLM    ToolType = "LLM"
	ToolTypeCustom ToolType = "CUSTOM"
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

type ToolContext struct {
	TaskID     string `json:"taskId"`
	NodeID     string `json:"nodeId"`
	RetryCount int    `json:"retryCount"`
}

type Tool interface {
	Name() string
	Description() string
	Type() ToolType
	Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult
	ValidateParameters(params map[string]interface{}) bool
}

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) All() map[string]Tool {
	return r.tools
}

func (r *ToolRegistry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

func DetermineToolName(nodeType string, payload map[string]interface{}) string {
	if tool, ok := payload["tool"]; ok {
		if s, ok := tool.(string); ok {
			return s
		}
	}
	if nodeType == string(model.NodeTypeTool) {
		if name, ok := payload["name"]; ok {
			if s, ok := name.(string); ok {
				return s
			}
		}
	}
	return "llm"
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
