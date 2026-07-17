package executor

import (
	"context"
)

// Executor 定义执行器统一接口
type Executor interface {
	Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error)
}
