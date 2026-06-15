package executor

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/tangying-ai/aios-core/internal/worker/executor/sandboxpb"
)

// SandboxExecutor executes commands via the Rust sandbox gRPC service.
// Provides resource isolation (memory, CPU, disk, PID limits), timeout enforcement,
// and automatic cleanup of temporary execution directories.
type SandboxExecutor struct {
	address string
	conn    *grpc.ClientConn
	client  sandboxpb.SandboxServiceClient
}

func NewSandboxExecutor(address string) (*SandboxExecutor, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox gRPC client: %w", err)
	}

	return &SandboxExecutor{
		address: address,
		conn:    conn,
		client:  sandboxpb.NewSandboxServiceClient(conn),
	}, nil
}

func (s *SandboxExecutor) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *SandboxExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
	if s.client == nil {
		return ExecutionResult{Error: "sandbox client not initialized"}, nil
	}

	// Map ResourceLimits to protobuf
	var pbLimits *sandboxpb.ResourceLimits
	if req.Limits.MemoryBytes > 0 || req.Limits.CPUShares > 0 || req.Limits.DiskBytes > 0 || req.Limits.MaxPIDs > 0 {
		pbLimits = &sandboxpb.ResourceLimits{
			MemoryBytes: req.Limits.MemoryBytes,
			CpuShares:   req.Limits.CPUShares,
			DiskBytes:   req.Limits.DiskBytes,
			MaxPids:     req.Limits.MaxPIDs,
		}
	}

	// Convert env map
	envMap := make(map[string]string)
	for k, v := range req.Env {
		envMap[k] = v
	}

	// Build request
	pbReq := &sandboxpb.ExecuteRequest{
		TaskId:     req.TaskID,
		NodeId:     req.NodeID,
		Command:    req.Command,
		Args:       req.Args,
		Env:        envMap,
		WorkDir:    req.WorkDir,
		TimeoutSec: req.TimeoutSec,
		Limits:     pbLimits,
		InputFiles: req.InputFiles,
		Stdin:      req.Stdin,
	}

	// Execute with context timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutSec+5)*time.Second)
	defer cancel()

	pbResult, err := s.client.Execute(execCtx, pbReq)
	if err != nil {
		return ExecutionResult{Error: fmt.Sprintf("sandbox gRPC call failed: %v", err)}, nil
	}

	result := ExecutionResult{
		ExitCode: pbResult.ExitCode,
		Stdout:   pbResult.Stdout,
		Stderr:   pbResult.Stderr,
		TimedOut: pbResult.TimedOut,
		Error:    pbResult.Error,
	}

	if pbResult.ResourceUsage != nil {
		result.ResourceUsage = &ResourceUsage{
			MemoryPeakBytes: pbResult.ResourceUsage.MemoryPeakBytes,
			CPUTimeUsec:     pbResult.ResourceUsage.CpuTimeUsec,
			UserTimeUsec:    pbResult.ResourceUsage.UserTimeUsec,
			SystemTimeUsec:  pbResult.ResourceUsage.SystemTimeUsec,
		}
	}

	return result, nil
}
