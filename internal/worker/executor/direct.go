package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

type DirectExecutor struct{}

func NewDirectExecutor() *DirectExecutor {
	return &DirectExecutor{}
}

func (d *DirectExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
	workDir, err := os.MkdirTemp("", "lingxi-worker-"+req.NodeID)
	if err != nil {
		return ExecutionResult{Error: "failed to create temp dir"}, err
	}
	defer os.RemoveAll(workDir)

	for name, content := range req.InputFiles {
		path := filepath.Join(workDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return ExecutionResult{Error: err.Error()}, err
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			return ExecutionResult{Error: err.Error()}, err
		}
	}

	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), mapToEnvSlice(req.Env)...)
	if req.Stdin != nil {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ExecutionResult{
				TimedOut: true,
				Error:    "execution timed out",
			}, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return ExecutionResult{Error: err.Error()}, err
		}
	}

	return ExecutionResult{
		ExitCode: int32(exitCode),
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}, nil
}

func mapToEnvSlice(env map[string]string) []string {
	var s []string
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}
