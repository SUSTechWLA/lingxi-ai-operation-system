package executor

import (
	"context"
)

type SandboxExecutor struct {
	address string
	// client *sandbox.Client // gRPC 客户端封装，后期集成 Rust 沙箱时启用
}

func NewSandboxExecutor(address string) (*SandboxExecutor, error) {
	return &SandboxExecutor{address: address}, nil
}

func (s *SandboxExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
	return ExecutionResult{
		Error: "sandbox executor not yet implemented",
	}, nil
}
