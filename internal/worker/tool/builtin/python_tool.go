package builtin

import (
	"context"
	"fmt"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type PythonTool struct{}

func NewPythonTool() *PythonTool {
	return &PythonTool{}
}

func (t *PythonTool) Name() string                  { return "python" }
func (t *PythonTool) Description() string            { return "Execute Python3 code" }
func (t *PythonTool) Type() tool.ToolType            { return tool.ToolTypeCode }

func (t *PythonTool) BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error) {
	source, ok := params["source"].(string)
	if !ok || source == "" {
		return nil, fmt.Errorf("source code is required")
	}

	timeout := uint32(30)
	if timeoutSec, ok := params["timeoutSec"].(float64); ok {
		timeout = uint32(timeoutSec)
	}

	return &executor.ExecutionRequest{
		Command: "python3",
		Args:    []string{"-c", source},
		WorkDir: "/tmp/lingxi-sandbox",
		TimeoutSec: timeout,
		Limits: executor.ResourceLimits{
			MemoryBytes: 128 * 1024 * 1024,
			CPUShares:   100,
		},
	}, nil
}

func (t *PythonTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	execReq, err := t.BuildExecutionRequest(params)
	if err != nil {
		return tool.FailureResult(err.Error())
	}

	directExec := executor.NewDirectExecutor()
	result, execErr := directExec.Execute(ctx, *execReq)
	if execErr != nil {
		return tool.FailureResult(execErr.Error())
	}

	if result.ExitCode != 0 {
		return tool.FailureResult(fmt.Sprintf("python execution failed with exit code %d: %s", result.ExitCode, string(result.Stderr)))
	}

	return tool.SuccessResult(map[string]interface{}{
		"exitCode": result.ExitCode,
		"output":   string(result.Stdout),
		"stderr":   string(result.Stderr),
	})
}

func (t *PythonTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     true,
		Parameters: map[string]tool.ParamDef{
			"source": {
				Type:        "string",
				Description: "Python3 source code to execute (passed as python3 -c '...')",
				Required:    true,
			},
			"timeoutSec": {
				Type:        "number",
				Description: "Execution timeout in seconds (default: 30)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"exitCode": {Type: "integer", Description: "Process exit code (0 for success)"},
			"output":   {Type: "string", Description: "Captured stdout text"},
			"stderr":   {Type: "string", Description: "Captured stderr text"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"source": "print('hello world')"},
				Output: map[string]interface{}{"exitCode": 0, "output": "hello world\n", "stderr": ""},
			},
		},
	}
}

func (t *PythonTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["source"].(string)
	return ok
}
