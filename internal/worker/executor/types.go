package executor

// ResourceLimits 沙箱或本地执行的资源约束
type ResourceLimits struct {
	MemoryBytes uint64 `json:"memoryBytes"`
	CPUShares   uint64 `json:"cpuShares"`
	DiskBytes   uint64 `json:"diskBytes,omitempty"`
	MaxPIDs     uint32 `json:"maxPids,omitempty"`
}

// ResourceUsage 执行后的实际资源消耗
type ResourceUsage struct {
	MemoryPeakBytes uint64 `json:"memoryPeakBytes"`
	CPUTimeUsec     uint64 `json:"cpuTimeUsec"`
	UserTimeUsec    uint64 `json:"userTimeUsec"`
	SystemTimeUsec  uint64 `json:"systemTimeUsec"`
}

// ExecutionRequest 工具生成的、独立于执行环境的请求
type ExecutionRequest struct {
	TaskID     string            `json:"taskId"`
	NodeID     string            `json:"nodeId"`
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	WorkDir    string            `json:"workDir"`
	TimeoutSec uint32            `json:"timeoutSec"`
	Limits     ResourceLimits    `json:"limits"`
	InputFiles map[string][]byte `json:"inputFiles"`
	Stdin      []byte            `json:"stdin,omitempty"`
}

// ExecutionResult 执行器返回的统一结果
type ExecutionResult struct {
	ExitCode      int32          `json:"exitCode"`
	Stdout        []byte         `json:"stdout"`
	Stderr        []byte         `json:"stderr"`
	TimedOut      bool           `json:"timedOut"`
	ResourceUsage *ResourceUsage `json:"resourceUsage,omitempty"`
	Error         string         `json:"error,omitempty"`
	OutputRef     string         `json:"outputRef,omitempty"`
}
