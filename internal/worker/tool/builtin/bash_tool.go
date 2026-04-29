package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/executor"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

// ==================== BashTool (Sandboxed) ====================

var allowedCommands = map[string]bool{
	"ls": true, "cat": true, "echo": true, "curl": true,
	"python": true, "python3": true, "node": true,
	"head": true, "tail": true, "wc": true, "grep": true,
	"find": true, "which": true, "whoami": true, "date": true,
	"pwd": true, "uname": true, "df": true, "ps": true,
}

var dangerousPatterns = []string{";", "|", "&&", "||", "`", "$(", ">", "<", ">>", "<<", "&"}

type BashTool struct {
	timeoutSeconds int
}

func NewBashTool(cfg config.BashToolConfig) *BashTool {
	timeout := cfg.TimeoutSeconds
	if timeout == 0 {
		timeout = 30
	}
	return &BashTool{
		timeoutSeconds: timeout,
	}
}

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Execute shell commands (sandboxed)" }
func (t *BashTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *BashTool) BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	if err := validateBashCommand(command); err != nil {
		return nil, fmt.Errorf("command rejected: %s", err.Error())
	}

	timeout := uint32(t.timeoutSeconds)
	if timeoutSec, ok := params["timeoutSec"].(float64); ok {
		timeout = uint32(timeoutSec)
	}

	return &executor.ExecutionRequest{
		Command:    "bash",
		Args:       []string{"-c", command},
		WorkDir:    "/tmp/lingxi-sandbox",
		TimeoutSec: timeout,
		Limits: executor.ResourceLimits{
			MemoryBytes: 256 * 1024 * 1024,
			CPUShares:   100,
		},
	}, nil
}

func (t *BashTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
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
		return tool.FailureResult(fmt.Sprintf("command failed with exit code %d: %s", result.ExitCode, string(result.Stderr)))
	}

	return tool.SuccessResult(map[string]interface{}{
		"exitCode": result.ExitCode,
		"output":   string(result.Stdout),
		"command":  execReq.Args[1],
	})
}

func (t *BashTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     true,
		Parameters: map[string]tool.ParamDef{
			"command": {
				Type:        "string",
				Description: "Shell command to execute. Allowed commands: ls, cat, echo, curl, python, python3, node, head, tail, wc, grep, find, which, whoami, date, pwd, uname, df, ps",
				Required:    true,
			},
			"timeoutSec": {
				Type:        "number",
				Description: "Execution timeout in seconds (default: 30)",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"exitCode": {Type: "integer", Description: "Process exit code"},
			"output":   {Type: "string", Description: "Captured stdout text"},
			"command":  {Type: "string", Description: "The command string that was executed"},
		},
		Examples: []tool.ToolExample{
			{
				Input:  map[string]interface{}{"command": "ls -la /tmp"},
				Output: map[string]interface{}{"exitCode": 0, "output": "total 0\ndrwxr-xr-x  2 user  wheel  64 Apr 30 00:00 .", "command": "ls -la /tmp"},
			},
		},
	}
}

func (t *BashTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["command"].(string)
	return ok
}

func validateBashCommand(command string) error {
	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("dangerous pattern '%s' is not allowed", pattern)
		}
	}

	trimmed := strings.TrimSpace(command)
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	baseCmd := parts[0]
	if !allowedCommands[baseCmd] {
		return fmt.Errorf("command '%s' is not in the allowed list", baseCmd)
	}

	return nil
}
