package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
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
	sandboxDir     string
}

func NewBashTool(cfg config.BashToolConfig) *BashTool {
	sandboxDir := "/tmp/ai-sandbox"
	os.MkdirAll(sandboxDir, 0755)
	timeout := cfg.TimeoutSeconds
	if timeout == 0 {
		timeout = 30
	}
	return &BashTool{
		timeoutSeconds: timeout,
		sandboxDir:     sandboxDir,
	}
}

func (t *BashTool) Name() string       { return "bash" }
func (t *BashTool) Description() string { return "Execute shell commands (sandboxed)" }
func (t *BashTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *BashTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	command, _ := params["command"].(string)
	if command == "" {
		return tool.FailureResult("Command is required")
	}

	if err := validateBashCommand(command); err != nil {
		return tool.FailureResult(fmt.Sprintf("Command rejected: %s", err.Error()))
	}

	zap.L().Info("Executing bash command", zap.String("taskId", toolCtx.TaskID), zap.String("command", command))

	timeout := time.Duration(t.timeoutSeconds) * time.Second
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := execCommandContext(execCtx, "bash", "-c", command)
	cmd.Dir = t.sandboxDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if err != nil {
		return tool.FailureResult(fmt.Sprintf("Command failed: %s, output: %s", err.Error(), output))
	}

	return tool.SuccessResult(map[string]interface{}{
		"exitCode": 0,
		"output":   output,
		"command":  command,
	})
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
