# Skill: Worker 执行契约统一与多语言工具 & 沙箱支持

## 目标
重构 Worker 内部执行模型，定义一套**语言无关、执行环境无关**的请求/响应数据结构，使工具开发者只需生成标准执行请求，架构层可通过配置在“本地直接执行”与“Rust 沙箱执行”之间动态切换。支持 Python、C++、Java 等多语言工具代码的统一调度，且初期不依赖沙箱即可完整运行。

## 前置条件
- 项目已具备基础的事件驱动调度、Node/Task 数据模型、Orchestrator 与 Worker 的基本通信。
- 已有 `Tool` 接口定义，部分工具（bash、llm_api 等）已实现。
- 已预留配置开关与执行器选择逻辑的位置。

## 实施原则
- **统一数据契约**：所有工具与执行器间仅通过 `ExecutionRequest`/`ExecutionResult` 交互，彻底解耦。
- **渐进增强**：初期无需沙箱即可工作；后期开启沙箱只需配置切换，不修改工具代码。
- **接口明确**：工具开发者只需关注 `BuildableTool` 接口，无需关心底层是本地进程还是沙箱调用。
- **向后兼容**：原有的 `Tool` 和直接执行路径保留，但标记为待下线。

## 步骤概览
1. 定义标准化执行数据契约（`ExecutionRequest`/`ExecutionResult`）和增强的 `Tool` 接口。
2. 实现 `DirectExecutor`（本地执行器），供初期开发使用。
3. 改造 Worker 的主流程（`NodeExecutor`），使其通过统一的 `Executor` 接口调度。
4. 提供 `SandboxExecutor` 的适配点（gRPC 客户端骨架），供后期集成 Rust 沙箱。
5. 指导工具开发者将多语言工具实现为 `BuildableTool`。
6. 添加配置项与选择逻辑，实现本地/沙箱双轨运行。

---

## 第一步：定义核心数据契约与接口

### 1.1 新建 `internal/worker/executor/types.go`

```go
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
    Command    string            `json:"command"`          // 可执行文件或解释器
    Args       []string          `json:"args,omitempty"`   // 参数（推荐防注入）
    Env        map[string]string `json:"env,omitempty"`
    WorkDir    string            `json:"workDir"`          // 工作目录（执行器可替换为临时路径）
    TimeoutSec uint32            `json:"timeoutSec"`
    Limits     ResourceLimits    `json:"limits"`
    InputFiles map[string][]byte `json:"inputFiles"`       // 文件名→内容，写入工作目录
    Stdin      []byte            `json:"stdin,omitempty"`
}

// ExecutionResult 执行器返回的统一结果
type ExecutionResult struct {
    ExitCode      int32          `json:"exitCode"`
    Stdout        []byte         `json:"stdout"`
    Stderr        []byte         `json:"stderr"`
    TimedOut      bool           `json:"timedOut"`
    ResourceUsage *ResourceUsage `json:"resourceUsage,omitempty"`
    Error         string         `json:"error,omitempty"`       // 基础设施错误
    OutputRef     string         `json:"outputRef,omitempty"`   // 大输出外存引用
}
```

### 1.2 增强 `internal/worker/tools/tool.go` 中的接口

在原有基础上增加：

```go
// BuildableTool 是推荐多语言工具实现的接口
// 它生成一个 ExecutionRequest，由执行器负责真正执行
type BuildableTool interface {
    Tool
    // BuildExecutionRequest 根据参数构造执行请求（不执行任何系统调用）
    BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error)
}

// ExecutableTool 保留原有直接执行模式（仅用于无法标准化的工具，如纯HTTP调用）
type ExecutableTool interface {
    Tool
    Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult
}
```

> 注意：导入 `executor` 包时需避免循环依赖；可将 `ExecutionRequest` 等定义在独立包或 tools 包内部，此处假定放在 `internal/worker/executor`。

### 1.3 明确工具开发者的遵循规则

- 对于需要调用外部命令或解释器的工具（bash, python, cpp, java 等），实现 `BuildableTool`，**不要**自行调用 `exec.Command`。
- 对于纯内部逻辑或 HTTP 工具（如 `llm_api`），可暂时实现 `ExecutableTool`，由本地执行器直接调用。
- 所有工具必须实现 `ValidateParameters`。

---

## 第二步：实现 DirectExecutor（本地执行器）

### 2.1 新建 `internal/worker/executor/direct.go`

```go
type DirectExecutor struct {
    // 可注入临时目录管理器等
}

func NewDirectExecutor() *DirectExecutor {
    return &DirectExecutor{}
}

func (d *DirectExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
    // 1. 创建临时工作目录（例如 /tmp/lingxi-worker-{nodeID}）
    workDir, err := os.MkdirTemp("", "lingxi-worker-"+req.NodeID)
    if err != nil {
        return ExecutionResult{Error: "failed to create temp dir"}, err
    }
    defer os.RemoveAll(workDir)

    // 2. 写入 InputFiles
    for name, content := range req.InputFiles {
        path := filepath.Join(workDir, name)
        if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
            return ExecutionResult{Error: err.Error()}, err
        }
        if err := os.WriteFile(path, content, 0600); err != nil {
            return ExecutionResult{Error: err.Error()}, err
        }
    }

    // 3. 构建执行命令（使用 req.Command + req.Args，不透过 shell）
    cmd := exec.CommandContext(ctx, req.Command, req.Args...)
    cmd.Dir = workDir
    cmd.Env = append(os.Environ(), mapToEnvSlice(req.Env)...)
    if req.Stdin != nil {
        cmd.Stdin = bytes.NewReader(req.Stdin)
    }
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // 4. 启动并等待，处理超时由 context 负责
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

    // 5. 收集结果（本地无法精确获取 cgroups 指标，ResourceUsage 暂空）
    return ExecutionResult{
        ExitCode: int32(exitCode),
        Stdout:   stdout.Bytes(),
        Stderr:   stderr.Bytes(),
        // ResourceUsage 留空，后期沙箱可提供
    }, nil
}
```

### 2.2 辅助函数

```go
func mapToEnvSlice(env map[string]string) []string {
    var s []string
    for k, v := range env {
        s = append(s, k+"="+v)
    }
    return s
}
```

---

## 第三步：重构 NodeExecutor 主流程

### 3.1 修改 `internal/worker/node_executor.go`

```go
type NodeExecutor struct {
    directExecutor  *executor.DirectExecutor
    sandboxExecutor *executor.SandboxExecutor // 初期可为 nil
    registry        tools.ToolRegistry
    config          *config.WorkerConfig
    eventBus        EventBus
}

// 内部统一的执行接口（可暂时使用 executor.Executor 接口）
type executorInterface interface {
    Execute(ctx context.Context, req executor.ExecutionRequest) (executor.ExecutionResult, error)
}

func (ne *NodeExecutor) ProcessEvent(ctx context.Context, event NodeTaskEvent) {
    // ... 幂等检查 ...

    tool, err := ne.registry.Get(event.ToolName)
    if err != nil {
        // 处理错误
    }

    // 决定使用哪个执行器
    execImpl := ne.selectExecutor(tool)

    var result executor.ExecutionResult
    var execErr error

    // 1. 尝试通过 BuildableTool 生成标准请求
    if bt, ok := tool.(tools.BuildableTool); ok {
        execReq, err := bt.BuildExecutionRequest(event.Payload)
        if err != nil {
            result.Error = err.Error()
        } else {
            // 填充任务级信息
            execReq.TaskID = event.TaskID
            execReq.NodeID = event.NodeID
            result, execErr = execImpl.Execute(ctx, *execReq)
        }
    } else if et, ok := tool.(tools.ExecutableTool); ok {
        // 降级：直接执行工具（用于不支持标准化的工具）
        toolResult := et.Execute(ctx, event.Payload, tools.ToolContext{...})
        result = executor.ExecutionResult{
            ExitCode: 0,
            Stdout:   []byte(toolResult.Output),
            Error:    toolResult.Error,
        }
    } else {
        result.Error = "tool does not implement any executable interface"
    }

    // 2. 处理执行结果，转换为 NodeResultEvent 并发布
    // ... （在此处可设置 ResourceUsage、OutputRef 等）
    ne.publishResult(ctx, event, result, execErr)
}
```

### 3.2 执行器选择逻辑

```go
func (ne *NodeExecutor) selectExecutor(tool tools.Tool) executorInterface {
    // 后期条件：配置启用沙箱 且 沙箱客户端可用 且 工具实现了 BuildableTool
    if ne.config.Sandbox.Enabled && ne.sandboxExecutor != nil {
        if _, ok := tool.(tools.BuildableTool); ok {
            return ne.sandboxExecutor
        }
    }
    // 默认返回本地执行器
    return ne.directExecutor
}
```

---

## 第四步：预留 SandboxExecutor 适配点

### 4.1 创建 `internal/worker/executor/sandbox.go`

```go
type SandboxExecutor struct {
    client *sandbox.Client // gRPC 客户端封装
}

func NewSandboxExecutor(address string) (*SandboxExecutor, error) {
    c, err := sandbox.NewClient(address)
    if err != nil {
        return nil, err
    }
    return &SandboxExecutor{client: c}, nil
}

func (s *SandboxExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
    // 将 ExecutionRequest 转换为 protobuf 请求
    pbReq := toProto(req)
    pbResp, err := s.client.Execute(ctx, pbReq)
    if err != nil {
        return ExecutionResult{Error: "sandbox call failed: " + err.Error()}, err
    }
    return fromProto(pbResp), nil
}

// toProto / fromProto 实现与 proto/sandbox.proto 对应
```

### 4.2 在 `internal/sandbox/pb/` 中放置 protobuf 生成代码（可在沙箱开发者提供后更新）

---

## 第五步：配置与启动集成

### 5.1 更新 `internal/config/config.go`

```go
type SandboxConfig struct {
    Enabled  bool   `mapstructure:"sandbox_enabled"`
    Address  string `mapstructure:"sandbox_address"`
    Fallback bool   `mapstructure:"sandbox_fallback"` // 沙箱失败时是否回退本地
}
```

### 5.2 在 Worker 启动时组装

```go
// main.go 或 worker 初始化
directExec := executor.NewDirectExecutor()
var sandboxExec *executor.SandboxExecutor
if cfg.Sandbox.Enabled {
    sandboxExec, err = executor.NewSandboxExecutor(cfg.Sandbox.Address)
    if err != nil {
        log.Warn("sandbox client init failed, will fallback", zap.Error(err))
    }
}
nodeExec := NewNodeExecutor(directExec, sandboxExec, registry, cfg, eventBus)
```

---

## 第六步：指导工具开发者实现多语言工具

### 6.1 示例：PythonTool

```go
type PythonTool struct{}

func (t *PythonTool) Name() string        { return "python" }
func (t *PythonTool) Description() string { return "Execute Python3 code" }
func (t *PythonTool) Type() ToolType      { return "code" }
func (t *PythonTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["source"].(string)
    return ok
}

func (t *PythonTool) BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error) {
    source := params["source"].(string)
    timeout := 30
    if t, ok := params["timeout"].(float64); ok {
        timeout = int(t)
    }
    return &executor.ExecutionRequest{
        Command:    "python3",
        Args:       []string{"-c", source},
        WorkDir:    "/sandbox",   // 沙箱内路径，DirectExecutor 会替换为临时目录
        TimeoutSec: uint32(timeout),
        Limits: executor.ResourceLimits{
            MemoryBytes: 128 * 1024 * 1024, // 128MB
            CPUShares:   100,
        },
    }, nil
}
```

### 6.2 C++/Java 等编译型工具

需在 `BuildExecutionRequest` 中返回**两次执行请求**（编译+运行）。可定义工具返回 `[]ExecutionRequest`，或由 Worker 顺序执行。简单方案：在工具生成的请求中使用一个包装脚本：

```go
func (t *CppTool) BuildExecutionRequest(params map[string]interface{}) (*executor.ExecutionRequest, error) {
    source := params["source"].(string)
    script := fmt.Sprintf("g++ -O2 main.cpp -o prog && ./prog")
    return &executor.ExecutionRequest{
        Command: "bash",
        Args:    []string{"-c", script},
        InputFiles: map[string][]byte{
            "main.cpp": []byte(source),
        },
        // ...
    }, nil
}
```

> 推荐在沙箱内提供 `bash` 执行包装脚本，以避免多次注入命令注入风险。

---

## 第七步：数据模型补充（按需）

如果尚未完成，在 `ai_node` 表增加之前设计的字段（`timeout_sec`, `resource_limits`, `resource_usage`, `sandbox_instance`, `output_ref`），并确保 `NodeResultEvent` 包含相应字段。

---

## 第八步：测试与验证

1. **本地模式（无沙箱）**：启动 Worker，提交一个 bash/python 节点，确认 `DirectExecutor` 被执行，输出正常。
2. **沙箱模式（未来）**：启动 Rust 沙箱，开启配置，提交任务，确认 gRPC 通信成功，资源统计返回。
3. **降级测试**：沙箱不可用时，若 `Fallback=true` 应自动回退到 `DirectExecutor`。

---

## 并行开发指南（项目协调用）

- **架构师（你）**：完成前四步，交付接口定义和可运行的 `DirectExecutor` + `NodeExecutor` 骨架。
- **工具开发者**：基于 `BuildableTool` 接口实现各种语言工具，编写单元测试（mock 一个简单的 `Executor`）。
- **沙箱开发者**：根据 `ExecutionRequest`/`ExecutionResult` 对应的 protobuf 约定实现 Rust 沙箱服务，暴露 gRPC `Execute` 接口，无需了解工具业务逻辑。

三方均以 `internal/worker/executor/types.go` 和 `proto/sandbox.proto` 为唯一契约。