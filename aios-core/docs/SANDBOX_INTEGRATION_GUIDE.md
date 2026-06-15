# 沙箱执行环境对接指南

> 本文档面向**沙箱运行时开发人员**，说明沙箱隔离执行环境如何与躺营 AI OS 的 Worker 模块集成。
>
> **当前状态**：Rust gRPC 沙箱服务已完整实现（`sandbox/` 目录），支持 Bash/Python 工具隔离执行。通过 `SANDBOX_ENABLED=true` 启用。

---

## 目录

1. [架构概览](#1-架构概览)
2. [当前集成现状](#2-当前集成现状)
3. [核心契约：ExecutionRequest → ExecutionResult](#3-核心契约executionrequest--executionresult)
4. [对接方式：gRPC 服务](#4-对接方式grpc-服务)
5. [Proto 协议定义](#5-proto-协议定义)
6. [文件传输策略](#6-文件传输策略)
7. [资源限制与监控](#7-资源限制与监控)
8. [环境变量注入](#8-环境变量注入)
9. [连接生命周期](#9-连接生命周期)
10. [安全边界](#10-安全边界)
11. [配置参考](#11-配置参考)
12. [实现步骤](#12-实现步骤)
13. [测试验证](#13-测试验证)
14. [常见问题](#14-常见问题)

---

## 1. 架构概览

### 1.1 沙箱在系统中的位置

```
Worker 模块内部
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  NodeExecutor (执行调度器)                                    │
│    ↓ 根据工具类型选择执行器                                    │
│  ┌──────────────────────────────────────┐                   │
│  │  selectExecutor(tool)                │                   │
│  │    ├─ SANDBOX_ENABLED && Buildable   │                   │
│  │    │   → SandboxExecutor (gRPC)      │                   │
│  │    └─ else                           │                   │
│  │        → DirectExecutor (子进程)      │                   │
│  └──────────────────────────────────────┘                   │
│         │                                                    │
│         ▼                                                    │
│  ┌──────────────┐    ┌──────────────────┐                    │
│  │ DirectExecutor│    │ SandboxExecutor  │ ← ★ 你需要实现的    │
│  │ (当前默认)     │    │   gRPC Client    │   沙箱客户端对接端   │
│  └──────┬───────┘    └────────┬─────────┘                    │
│         │                     │                              │
│    本地子进程              gRPC 调用                           │
│         │                     │                              │
└─────────┼─────────────────────┼──────────────────────────────┘
          │                     │
          ▼                     ▼
    ┌──────────┐     ┌──────────────────┐
    │ 本地OS    │     │ 沙箱服务 (你开发)   │
    │ 子进程    │     │ e.g. Rust/Go 守护进程│
    └──────────┘     │  Linux Namespace   │
                     │  cgroup/Seccomp    │
                     └──────────────────┘
```

### 1.2 路由选择逻辑

Worker 的 `NodeExecutor.selectExecutor()` 按以下优先级选择执行器：

```
工具实现了 BuildableTool 接口
    ↓
SANDBOX_ENABLED = true
    ↓
SandboxExecutor 初始化成功
    ↓
是 → 使用 SandboxExecutor（gRPC 远程执行）
否 → 回退到 DirectExecutor（本地子进程）
```

### 1.3 关键设计原则

| 原则 | 说明 |
|------|------|
| **通用** | 对接 Go/Rust/Python 任意语言实现的沙箱，通过 gRPC 解耦 |
| **可扩展** | 执行请求和结果结构支持未来增加字段（如 GPU、网络限制） |
| **兼容** | 不修改现有 BuildableTool 接口；各工具通过 `selectExecutor` 自动路由 |
| **兜底** | 沙箱不可用时自动回退到 DirectExecutor（受配置控制） |

---

## 2. 当前集成现状

### 2.1 已就绪的组件

系统已包含以下沙箱集成基础设施，你**无需修改**：

| 组件 | 文件 | 状态 | 说明 |
|------|------|------|------|
| `SandboxConfig` | `internal/config/config.go:82` | **已完成** | 配置结构体，支持 Enable/Address/Fallback |
| `.env` 配置项 | `.env.example:62` | **已完成** | `SANDBOX_ENABLED`、`SANDBOX_ADDRESS`、`SANDBOX_FALLBACK` |
| `SandboxExecutor` | `internal/worker/executor/sandbox.go` | **已实现** | gRPC 客户端，连接 Rust 沙箱服务 (`sandbox/`)，通过 `sandbox.proto` 协议通信 |
| `main.go` 初始化 | `cmd/tangying-ai-os/main.go` | **已完成** | 按配置初始化 SandboxExecutor，失败时 warn 但不阻塞启动 |
| `NodeExecutor.selectExecutor` | `internal/worker/service/executor.go` | **已完成** | 自动路由 BuildableTool 到沙箱（如果启用） |
| `ResourceLimits` | `internal/worker/executor/types.go` | **已完成** | 资源约束结构体 |
| `ResourceUsage` | `internal/worker/executor/types.go` | **已完成** | 资源使用统计结构体 |
| `ExecutionRequest` | `internal/worker/executor/types.go` | **已完成** | 统一的执行请求结构体 |
| `ExecutionResult` | `internal/worker/executor/types.go` | **已完成** | 统一的执行结果结构体 |
| **Rust 沙箱服务** | `sandbox/src/main.rs`, `sandbox/src/sandbox.rs` | **已实现** | tonic gRPC 服务，支持进程隔离执行 |
| **Proto 协议定义** | `sandbox/proto/sandbox.proto` | **已实现** | gRPC 服务 + 消息定义 |
| **沙箱构建** | `make sandbox-build` | **已实现** | `cargo build --release` → `build/tangying-sandbox` |

### 2.2 沙箱服务实现总览

Rust 沙箱服务（`sandbox/`）已完整实现以下能力：
1. **gRPC 服务** — 基于 tonic，接收 `ExecutionRequest`，返回 `ExecutionResult`
2. **进程隔离** — 通过 `setrlimit` 实现资源限制（内存、CPU、磁盘、PID 数量）
3. **文件系统** — 支持 `InputFiles` 写入和 `OutputRef` 读取
4. **超时控制** — 双层超时机制（Worker context + 沙箱内 timeout）

Go 侧 `SandboxExecutor` (`internal/worker/executor/sandbox.go`) 实现了完整的 gRPC 客户端，通过 `sandbox.proto` 协议与 Rust 服务通信。

构建和启动：
```bash
make sandbox-build                    # 编译 Rust 沙箱 → build/tangying-sandbox
./build/tangying-sandbox &            # 启动沙箱 gRPC 服务 (端口 50051)
SANDBOX_ENABLED=true make run         # 启动主程序（启用沙箱模式）
```

> **注意**：沙箱服务需要 macOS/Linux 环境。未启用沙箱时 (`SANDBOX_ENABLED=false`)，`BuildableTool`（Bash/Python）通过 `DirectExecutor` 本地执行。

---

## 3. 核心契约：ExecutionRequest → ExecutionResult

这是沙箱与 Worker 之间的**唯一接口**。以下两个结构体构成了完整的契约。

### 3.1 ExecutionRequest（请求）

```go
type ExecutionRequest struct {
    TaskID     string            // 当前任务 ID（用于审计追踪）
    NodeID     string            // 当前节点 ID
    Command    string            // 要执行的命令（如 "python3"、"bash"）
    Args       []string          // 命令参数（如 ["-c", "echo hello"]）
    Env        map[string]string // 环境变量键值对
    WorkDir    string            // 工作目录（沙箱内路径）
    TimeoutSec uint32            // 超时秒数（到期强制终止）
    Limits     ResourceLimits    // 资源约束（见下文）
    InputFiles map[string][]byte // 输入文件（文件名 → 内容字节）
    Stdin      []byte            // 标准输入数据
}
```

### 3.2 ExecutionResult（响应）

```go
type ExecutionResult struct {
    ExitCode      int32          // 进程退出码（0=成功，非0=失败）
    Stdout        []byte         // 标准输出内容
    Stderr        []byte         // 标准错误内容
    TimedOut      bool           // 是否超时
    ResourceUsage *ResourceUsage // 实际资源消耗（见下文）
    Error         string         // 错误描述（沙箱内部错误，非进程错误）
    OutputRef     string         // 大文件输出路径（超过 1MB 时使用）
}
```

### 3.3 核心流程

```
1. Worker 收到 ai.node.ready 事件
   ↓
2. NodeExecutor 根据工具类型确定 toolName
   ↓
3. 调用 t.BuildExecutionRequest(params) → ExecutionRequest
   ↓
4. NodeExecutor 记录 startedAt 时间戳
   ↓
5. Worker → (gRPC) → 沙箱服务
   ↓
6. 沙箱服务：
   a. 创建隔离环境（namespace/cgroup）
   b. 写入 InputFiles 到沙箱文件系统
   c. 设置环境变量
   d. 执行命令（Command + Args）
   e. 应用资源限制
   f. 捕获 Stdout / Stderr
   g. 记录资源消耗
   h. 清理隔离环境
   ↓
7. 沙箱服务 → (gRPC) → ExecutionResult
   ↓
8. Worker 计算 durationMs
   ↓
9. Worker 发布 ai.node.result 事件
```

---

## 4. 对接方式：gRPC 服务

### 4.1 推荐架构

```
┌─────────────────────┐       gRPC        ┌──────────────────────┐
│  Go 主进程           │ ◄──────────────►  │  沙箱服务（独立进程）   │
│                     │   Execute()       │                      │
│  SandboxExecutor    │                   │  - setrlimit 资源控制 │
│  (gRPC Client)       │                   │  - 超时/清理        │
│                     │   HealthCheck()   │  - Seccomp BPF       │
│                     │ ◄──────────────►  │  - 网络隔离           │
│                     │                   │                      │
│  main.go:           │                   │  语言：Go/Rust/C/任意 │
│  NewSandboxExecutor │                   │  部署：独立二进制文件   │
│  (address:50051)    │                   │  权限：root（需要 CAP) │
└─────────────────────┘                   └──────────────────────┘
```

### 4.2 gRPC 的优势

| 优势 | 说明 |
|------|------|
| **语言无关** | 沙箱服务可以用 Go、Rust、C++ 等任意语言实现 |
| **强类型契约** | proto 文件是权威的接口定义 |
| **双向流** | 支持流式 stdout/stderr（可选） |
| **健康检查** | 内置 gRPC Health Checking Protocol |
| **TLS** | 可选加密传输 |

---

## 5. Proto 协议定义

### 5.1 核心 proto 文件

```protobuf
syntax = "proto3";

package sandbox;

option go_package = "github.com/tangying-ai/tangying-ai-operation-system/internal/worker/executor/sandboxpb";

// =================== 服务定义 ===================

service SandboxService {
  // 执行一个工具命令（核心接口）
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // 健康检查（gRPC 标准协议）
  rpc Check(HealthCheckRequest) returns (HealthCheckResponse);

  // 流式执行（可选的扩展，用于实时 stdout/stderr）
  rpc ExecuteStream(ExecuteRequest) returns (stream ExecuteStreamResponse);
}

// =================== 请求消息 ===================

message ExecuteRequest {
  string task_id = 1;           // 任务 ID
  string node_id = 2;           // 节点 ID
  string command = 3;           // 可执行文件路径或命令名
  repeated string args = 4;     // 命令参数列表
  map<string, string> env = 5;  // 环境变量
  string work_dir = 6;          // 工作目录
  uint32 timeout_sec = 7;       // 超时秒数
  ResourceLimits limits = 8;    // 资源约束
  map<string, bytes> input_files = 9; // 输入文件（文件名→内容）
  bytes stdin = 10;             // 标准输入
}

message ResourceLimits {
  uint64 memory_bytes = 1;  // 最大内存（字节）
  uint64 cpu_shares = 2;    // CPU 权重（cgroup cpu.shares）
  uint64 disk_bytes = 3;    // 最大磁盘写入（字节，0=不限制）
  uint32 max_pids = 4;      // 最大进程数（0=不限制）
}

// =================== 响应消息 ===================

message ExecuteResponse {
  int32 exit_code = 1;              // 退出码
  bytes stdout = 2;                 // 标准输出
  bytes stderr = 3;                 // 标准错误
  bool timed_out = 4;               // 是否超时
  ResourceUsage resource_usage = 5; // 资源消耗
  string error = 6;                 // 沙箱内部错误信息（非进程错误）
  string output_ref = 7;            // 大文件输出引用路径
}

message ResourceUsage {
  uint64 memory_peak_bytes = 1; // 内存峰值（字节）
  uint64 cpu_time_usec = 2;     // CPU 时间（微秒）
  uint64 user_time_usec = 3;    // 用户态时间（微秒）
  uint64 system_time_usec = 4;  // 内核态时间（微秒）
}

// =================== 健康检查 ===================

message HealthCheckRequest {
  string service = 1;
}

message HealthCheckResponse {
  enum ServingStatus {
    UNKNOWN = 0;
    SERVING = 1;
    NOT_SERVING = 2;
    SERVICE_UNKNOWN = 3;
  }
  ServingStatus status = 1;
}

// =================== 流式执行（可选扩展） ===================

message ExecuteStreamResponse {
  oneof output_type {
    bytes stdout_chunk = 1;    // stdout 数据块
    bytes stderr_chunk = 2;    // stderr 数据块
  }
  // 最后一条消息包含完整结果
  ExecuteResponse final = 3;
}
```

### 5.2 字段映射关系

| Go ExecutionRequest | proto ExecuteRequest | 说明 |
|--------------------|--------------------|------|
| `TaskID` | `task_id` | 任务标识 |
| `NodeID` | `node_id` | 节点标识 |
| `Command` | `command` | 执行命令 |
| `Args` | `args` | 命令参数 |
| `Env` | `env` | 环境变量 |
| `WorkDir` | `work_dir` | 工作目录 |
| `TimeoutSec` | `timeout_sec` | 超时控制 |
| `Limits` | `limits` | 资源约束 |
| `InputFiles` | `input_files` | 输入文件 |
| `Stdin` | `stdin` | 标准输入 |

| Go ExecutionResult | proto ExecuteResponse | 说明 |
|--------------------|----------------------|------|
| `ExitCode` | `exit_code` | 退出码 |
| `Stdout` | `stdout` | 标准输出 |
| `Stderr` | `stderr` | 错误输出 |
| `TimedOut` | `timed_out` | 超时标志 |
| `ResourceUsage` | `resource_usage` | 资源消耗 |
| `Error` | `error` | 沙箱错误 |
| `OutputRef` | `output_ref` | 大文件引用 |

---

## 6. 文件传输策略

### 6.1 输入文件（InputFiles）

`InputFiles` 用于向沙箱传递脚本文件、配置文件等。Worker 侧通过 `BuildExecutionRequest` 写入。

**典型场景**：BashTool 传入一个 shell 脚本文件，或 PythonTool 传入 `.py` 文件。

```go
// BuildableTool 中构建 InputFiles 示例
req.InputFiles = map[string][]byte{
    "script.sh":  []byte("#!/bin/bash\necho hello"),
    "config.json": []byte(`{"key": "value"}`),
}
```

**沙箱服务端处理**：
1. 在沙箱的工作目录（`WorkDir`）下创建文件
2. 文件名保留原始路径（如果包含 `/`，递归创建目录）
3. 设置文件权限为 `0600`（可执行脚本设为 `0700`）
4. 执行命令前确保所有文件就位

### 6.2 大文件输出（OutputRef）

当工具输出超过 **1MB** 时，建议将输出写入文件而不是直接放在 `Stdout` 中。

**机制**：
1. 沙箱服务将输出写入 `WorkDir` 下的文件
2. 在 `OutputRef` 中返回文件路径
3. Worker 侧的 `DirectExecutor` 自动读取并清理

> 注：当前 DirectExecutor 未实现 OutputRef 读取，待后续增强。沙箱端可以先支持此字段以保持前瞻兼容。

### 6.3 文件隔离

沙箱必须确保：
- 输入文件对外部不可见
- 临时文件在执行完成后清理
- 不支持沙箱外部的绝对路径访问（如 `/etc/passwd`）

---

## 7. 资源限制与监控

### 7.1 资源限制（ResourceLimits）

| 字段 | Linux 实现方式 | 说明 |
|------|---------------|------|
| `MemoryBytes` | cgroup v2 `memory.max` | 进程组最大内存，超过则 OOM Kill |
| `CPUShares` | cgroup v2 `cpu.weight` | CPU 相对权重（默认 100，范围 1-10000） |
| `DiskBytes` | cgroup v2 `io.max` | 磁盘写入带宽限制（可选实现） |
| `MaxPIDs` | cgroup v2 `pids.max` | 最大进程/线程数，防 fork bomb |

### 7.2 资源监控（ResourceUsage）

| 字段 | Linux 获取方式 | 说明 |
|------|---------------|------|
| `MemoryPeakBytes` | cgroup v2 `memory.peak` | 执行期间的内存峰值 |
| `CPUTimeUsec` | cgroup v2 `cpu.stat` → `usage_usec` | 总 CPU 消耗时间 |
| `UserTimeUsec` | cgroup v2 `cpu.stat` → `user_usec` | 用户态 CPU 时间 |
| `SystemTimeUsec` | cgroup v2 `cpu.stat` → `system_usec` | 内核态 CPU 时间 |

### 7.3 超时控制

**双层超时机制**：

```
Worker 侧（Go）：context.WithTimeout → gRPC 调用超时
  |
  如果 Worker 侧超时先到，gRPC 调用被取消
  |
沙箱侧：接收到 context 取消信号 → 强制 Kill 进程组
  |
  如果沙箱侧超时先到（TimeoutSec），沙箱自行 Kill
```

沙箱服务应：
1. 尊重 `TimeoutSec` 字段，超时后立刻终止进程
2. 在 `TimedOut = true` 时仍返回已捕获的 stdout/stderr
3. 确保子进程组被完整清理（无僵尸进程）

---

## 8. 环境变量注入

沙箱执行时需注入以下系统环境变量：

### 8.1 系统级环境变量

由 `DirectExecutor` 自动注入（`os.Environ()` + `ExecutionRequest.Env`）：

```
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HOME=/tmp/tangying-sandbox
```

### 8.2 工具级环境变量

由 `ExecutionRequest.Env` 携带，沙箱需合并到进程环境：

```
TASK_ID=<taskId>     # 当前任务 ID
NODE_ID=<nodeId>     # 当前节点 ID
```

### 8.3 沙箱定制变量

沙箱可自动注入自身相关的变量（如需）：

```
SANDBOX_TYPE=linux-namespace    # 沙箱类型标识
SANDBOX_VERSION=1.0.0           # 沙箱版本
```

### 8.4 环境变量安全

- 禁止进程修改沙箱自身的环境变量
- 敏感变量（如 API Key）仅在内部沙箱中注入
- 不支持变量引用展开（不保证 `$PATH` 等会被沙箱解析）

---

## 9. 连接生命周期

### 9.1 启动顺序

```
1. 启动基础设施（Docker: PostgreSQL, Redpanda, Redis...）
       ↓
2. 启动沙箱服务（独立进程）
       ↓
3. 主程序初始化时连接沙箱服务
   ├─ gRPC dial → HealthCheck
   ├─ 成功 → 设置 sandboxExec
   └─ 失败 → warn（根据 Fallback 配置决定是否继续）
       ↓
4. 主程序开始消费事件
```

### 9.2 连接管理

| 场景 | 行为 |
|------|------|
| 启动时连接失败 | 仅 warn，不阻塞主程序启动 |
| 运行时连接断开 | 当前执行的任务失败（Error 描述连接断开） |
| 连接恢复 | 后续新任务自动使用恢复的连接（gRPC 自动重连） |
| 健康检查失败（连续 3 次） | 标记为不可用，后续请求回退到 DirectExecutor |
| 优雅关闭 | 收到 SIGTERM → 等待正在执行的任务完成 → 关闭 gRPC 连接 |

### 9.3 健康检查协议

使用 gRPC 标准 Health Checking Protocol：

```bash
# 验证沙箱服务健康状态
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# 预期响应
{
  "status": "SERVING"  // 或 NOT_SERVING
}
```

主程序每 30 秒执行一次健康检查。连续 3 次失败则标记为不可用。

---

## 10. 安全边界

### 10.1 最小权限原则

| 维度 | 要求 |
|------|------|
| **用户** | 沙箱内进程以非 root 用户运行（如 nobody） |
| **网络** | 默认禁止网络访问（白名单模式，需要时由 Worker 通过参数开启） |
| **文件系统** | 仅可访问沙箱工作目录，`/tmp/tangying-sandbox/xxx` |
| **挂载** | 使用 `mount --bind` 仅挂载必要目录（`/usr/bin`、`/lib` 只读） |
| **系统调用** | 使用 seccomp BPF 过滤危险 syscall（`clone`、`mount`、`reboot` 等） |

### 10.2 必须过滤的系统调用（seccomp）

```python
# 建议黑名单（blocklist 方式）
BLOCKED_SYSCALLS = [
    "clone",        # 防止创建新 namespace
    "mount",        # 防止挂载
    "umount2",      # 防止卸载
    "reboot",       # 防止重启
    "swapon",       # 防止交换分区操作
    "swapoff",      # 同上
    "init_module",  # 防止加载内核模块
    "finit_module", # 同上
    "delete_module",# 防止卸载内核模块
    "kexec_load",   # 防止加载新内核
    "bpf",          # 防止加载 BPF 程序（防止逃逸）
]

# 或使用 allowlist 方式（更安全）：只放行普通应用需要的 ~50 个 syscall
```

### 10.3 目录映射

```
沙箱内路径                 宿主机路径
──────────────────────────────────────────────────
/tmp/tangying-sandbox/<id>/   → 临时目录（读写，用完即删）
/usr/bin/                   → /usr/bin（只读）
/usr/lib/                   → /usr/lib（只读）
/lib/                       → /lib（只读）
/lib64/                     → /lib64（只读）
/etc/alternatives/          → /etc/alternatives（只读，可选）
/bin/sh                     → /bin/sh（只读）
```

### 10.4 防止逃逸

| 风险 | 缓解措施 |
|------|---------|
| 通过 `/proc` 逃逸 | 将 `/proc` 挂载为新的 proc 实例（`mount -t proc`） |
| 通过 `/sys` 逃逸 | 不挂载 `/sys` |
| 通过 `ptrace` 逃逸 | seccomp 拦截 `ptrace` |
| 通过 `cgroup` 逃逸 | 不在沙箱内暴露 cgroup 文件系统 |
| 通过 `socket` 逃逸 | 使用 network namespace 隔离 |
| 通过 `setns` 逃逸 | seccomp 拦截 `setns` |
| 通过 `capabilities` 逃逸 | 仅保留 `CAP_NET_BIND_SERVICE`（如果需要网络） |

---

## 11. 配置参考

### 11.1 当前配置项

```bash
# .env 中的沙箱配置
SANDBOX_ENABLED=false            # 是否启用沙箱
SANDBOX_ADDRESS=localhost:50051  # 沙箱服务 gRPC 地址
SANDBOX_FALLBACK=true            # 沙箱不可用时是否回退到 DirectExecutor
```

### 11.2 建议新增配置项（沙箱服务端）

```bash
# 沙箱服务自身的配置
SANDBOX_MAX_CONCURRENCY=16       # 最大并发执行数
SANDBOX_MEMORY_MB=512            # 单次执行默认内存限制（MB）
SANDBOX_CPU_SHARES=256           # 单次执行默认 CPU 权重
SANDBOX_TIMEOUT_SEC=120          # 单次执行默认超时（秒）
SANDBOX_NETWORK_ENABLED=false    # 是否允许网络访问
SANDBOX_TEMP_DIR=/tmp/sandbox    # 沙箱工作目录根路径
SANDBOX_READONLY_PATHS=/usr/bin:/usr/lib:/lib:/lib64  # 只读挂载路径列表
```

> 这些配置属于沙箱服务自身范畴，应由沙箱服务的独立配置文件管理，而非主程序的 `.env`。

---

## 12. 实现步骤（参考）

> **注意**：以下步骤已完成。此章节保留作为二次开发或定制沙箱服务的参考。

### Step 1：定义 proto 文件

Proto 文件位于 `sandbox/proto/sandbox.proto`，定义参见 [第 5 节](#5-proto-协议定义)。

Rust 侧通过 `build.rs`（tonic-build）自动生成，Go 侧通过 `protoc` 生成到 `internal/worker/executor/sandboxpb/`。

```bash
# Go
protoc --go_out=. --go-grpc_out=. proto/sandbox.proto

# Rust (tonic)
cargo build  # Cargo.toml 中引用 proto

# Python
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. proto/sandbox.proto
```

### Step 2：实现沙箱服务端

当前 Rust 实现位于 `sandbox/src/sandbox.rs`（~210 行），核心逻辑：

```go
func (s *SandboxServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
    // 1. 创建隔离环境
    id := generateID()
    cg := createCgroup(id, req.Limits)
    ns := createNamespaces()
    defer cleanup(id, cg, ns)

    // 2. 准备文件系统
    workDir := filepath.Join("/tmp/sandbox", id)
    writeInputFiles(workDir, req.InputFiles)
    setupReadonlyMounts(workDir)
    defer os.RemoveAll(workDir)

    // 3. 执行命令
    cmd := exec.CommandContext(ctx, req.Command, req.Args...)
    applySeccomp(cmd)
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWPID |
                    syscall.CLONE_NEWNET | syscall.CLONE_NEWUTS,
        Credential: &syscall.Credential{Uid: 65534, Gid: 65534},
    }
    cmd.Dir = workDir

    // 4. 捕获输出
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // 5. 执行并收集指标
    start := time.Now()
    err := cmd.Run()
    duration := time.Since(start)

    // 6. 收集资源使用
    usage := readResourceUsage(cg)

    // 7. 返回结果
    exitCode := 0
    if err != nil {
        exitCode = extractExitCode(err)
    }

    return &pb.ExecuteResponse{
        ExitCode:     int32(exitCode),
        Stdout:       stdout.Bytes(),
        Stderr:       stderr.Bytes(),
        TimedOut:     ctx.Err() == context.DeadlineExceeded,
        ResourceUsage: usage,
        Error:        extractError(err),
    }, nil
}
```

### Step 3：Go 侧 SandboxExecutor（已实现）

```go
// internal/worker/executor/sandbox.go
package executor

import (
    "context"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/tangying-ai/tangying-ai-operation-system/internal/worker/executor/sandboxpb"
)

type SandboxExecutor struct {
    address string
    conn    *grpc.ClientConn
    client  pb.SandboxServiceClient
}

func NewSandboxExecutor(address string) (*SandboxExecutor, error) {
    conn, err := grpc.Dial(address,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        return nil, err
    }
    return &SandboxExecutor{
        address: address,
        conn:    conn,
        client:  pb.NewSandboxServiceClient(conn),
    }, nil
}

func (s *SandboxExecutor) Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error) {
    pbReq := &pb.ExecuteRequest{
        TaskId:     req.TaskID,
        NodeId:     req.NodeID,
        Command:    req.Command,
        Args:       req.Args,
        Env:        req.Env,
        WorkDir:    req.WorkDir,
        TimeoutSec: req.TimeoutSec,
        Limits: &pb.ResourceLimits{
            MemoryBytes: req.Limits.MemoryBytes,
            CpuShares:   req.Limits.CPUShares,
            DiskBytes:   req.Limits.DiskBytes,
            MaxPids:     req.Limits.MaxPIDs,
        },
        InputFiles: req.InputFiles,
        Stdin:      req.Stdin,
    }

    pbResp, err := s.client.Execute(ctx, pbReq)
    if err != nil {
        return ExecutionResult{Error: err.Error()}, err
    }

    result := ExecutionResult{
        ExitCode: pbResp.ExitCode,
        Stdout:   pbResp.Stdout,
        Stderr:   pbResp.Stderr,
        TimedOut: pbResp.TimedOut,
        Error:    pbResp.Error,
        OutputRef: pbResp.OutputRef,
    }

    if pbResp.ResourceUsage != nil {
        result.ResourceUsage = &ResourceUsage{
            MemoryPeakBytes: pbResp.ResourceUsage.MemoryPeakBytes,
            CPUTimeUsec:    pbResp.ResourceUsage.CpuTimeUsec,
            UserTimeUsec:   pbResp.ResourceUsage.UserTimeUsec,
            SystemTimeUsec: pbResp.ResourceUsage.SystemTimeUsec,
        }
    }

    return result, nil
}
```

### Step 4：构建和部署（已实现）

```bash
# 主程序侧（Go）—— 重新编译以包含 gRPC 客户端
cd tangying-ai-operation-system
go mod tidy
make build

# 沙箱服务侧（独立项目）
cd sandbox-service
cargo build --release  # 或 go build / docker build

# 启动
# 先启动沙箱服务
./sandbox-service --addr 0.0.0.0:50051

# 再启动主程序
cd tangying-ai-operation-system
SANDBOX_ENABLED=true SANDBOX_ADDRESS=localhost:50051 ./build/tangying-ai-os
```

### Step 5：验证集成（可直接使用）

```bash
# 验证沙箱服务健康
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# 提交一个使用 BashTool 的 DAG 任务（会被自动路由到沙箱）
curl -X POST http://localhost:8080/api/node \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "sandbox-test",
        "type": "TOOL",
        "name": "bash",
        "input": {"command": "echo hello sandbox && whoami"}
      }
    ],
    "edges": []
  }'

# 查看执行结果
curl http://localhost:8080/api/trace/recent | python3 -m json.tool
```

---

## 13. 测试验证

### 13.1 单元测试清单

```go
// 沙箱服务端测试（示例）
func TestExecute_Success(t *testing.T) {
    // 输入：echo hello
    // 预期：exit_code=0, stdout="hello\n"
}

func TestExecute_Timeout(t *testing.T) {
    // 输入：sleep 100, timeout_sec=1
    // 预期：timed_out=true
}

func TestExecute_MemoryLimit(t *testing.T) {
    // 输入：申请 1GB 内存, memory_bytes=512MB
    // 预期：进程被 OOM Kill, exit_code != 0
}

func TestExecute_InputFiles(t *testing.T) {
    // 输入：input_files={"test.py": "print('ok')"}, command=python3, args=["test.py"]
    // 预期：exit_code=0, stdout="ok\n"
}

func TestExecute_Seccomp(t *testing.T) {
    // 输入：mount 命令
    // 预期：exit_code != 0, stderr 包含 "Operation not permitted"
}

func TestExecute_NetworkIsolation(t *testing.T) {
    // 输入：curl http://example.com
    // 预期：exit_code != 0（网络被隔离）
}

func TestResourceUsage(t *testing.T) {
    // 输入：消耗 CPU 1 秒
    // 预期：cpu_time_usec > 0, memory_peak_bytes > 0
}

func TestConcurrentExecutions(t *testing.T) {
    // 同时执行 10 个任务
    // 预期：全部完成，互不干扰
}
```

### 13.2 安全测试清单

```go
func TestNoPrivilegeEscalation(t *testing.T) {
    // sudo、su、setuid 等操作应被拦截
}

func TestNoFilesystemEscape(t *testing.T) {
    // 尝试访问 /etc/passwd、/root/ 等应被拒绝
}

func TestNoProcessEscape(t *testing.T) {
    // 尝试创建新 namespace、ptrace 其他进程应被拦截
}

func TestNoForkBomb(t *testing.T) {
    // fork 大量子进程应被 max_pids 限制
}
```

### 13.3 集成测试

```bash
# 测试脚本（保存为 test-sandbox.sh）
#!/bin/bash
set -e

BASE_URL="http://localhost:8080"

echo "=== 沙箱集成测试 ==="

# 1. 健康检查
echo "1. 服务健康检查"
curl -sf $BASE_URL/api/health | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['status']=='UP'"
echo "   ✓ OK"

# 2. 执行简单命令
echo "2. 执行简单命令"
TASK=$(curl -sf -X POST $BASE_URL/api/node \
  -H "Content-Type: application/json" \
  -d '{"nodes":[{"id":"t1","type":"TOOL","name":"bash","input":{"command":"echo ok"}}],"edges":[]}')
TASK_ID=$(echo $TASK | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

# 等待完成
sleep 3
RESULT=$(curl -sf $BASE_URL/api/trace/$TASK_ID)
echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin)['data']['task']; assert d['status']=='SUCCESS'"
echo "   ✓ 任务成功完成"

# 3. 验证资源使用数据
echo "3. 验证资源使用数据"
echo "$RESULT" | python3 -c "
import sys,json
d=json.load(sys.stdin)['data']
task = d['task']
node = task['nodes'][0]
assert 'durationMs' in node['output'], 'missing durationMs'
print(f'   ✓ durationMs={node[\"output\"][\"durationMs\"]}')
assert 'exitCode' in node['output'], 'missing exitCode'
print(f'   ✓ exitCode={node[\"output\"][\"exitCode\"]}')

# 验证上下文审计中包含执行监控数据
ctxs = d['contexts']
scheduled = [c for c in ctxs if c['contextType']=='NODE_SCHEDULED']
success_ctx = [c for c in ctxs if c['contextType']=='NODE_SUCCESS' and c.get('sourceModule')=='ContextService']
assert len(scheduled) > 0, 'missing NODE_SCHEDULED'
assert len(success_ctx) > 0, 'missing ContextService NODE_SUCCESS'
if scheduled[0].get('metadata'):
    print(f'   ✓ startedAt={scheduled[0][\"metadata\"].get(\"startedAt\")}')
if success_ctx[0].get('metadata'):
    print(f'   ✓ durationMs={success_ctx[0][\"metadata\"].get(\"durationMs\")}')
print('   全部验证通过！')
"

# 4. 验证资源限制
echo "4. 验证超时限制"
# ...

echo ""
echo "=== 测试完成 ==="
```

---

## 14. 常见问题

### Q: 为什么用 gRPC 而不是共享内存/Unix Socket？

gRPC 提供了跨语言、跨平台的标准化接口，便于未来将沙箱部署到远程宿主机。当前推荐使用 Unix Socket 以获得更低延迟：

```go
conn, err := grpc.Dial("unix:///tmp/sandbox.sock", ...)
```

### Q: 沙箱服务需要 root 权限吗？

是的。创建 Linux Namespace、管理 cgroup、配置 seccomp 都需要 `CAP_SYS_ADMIN` 或其他 capabilities。建议：
- 沙箱服务以 root 启动
- 沙箱内部的进程降权为 nobody 用户
- 主程序不需要 root 权限

### Q: 多个并发执行如何隔离？

每个 `Execute` 请求创建独立的隔离环境：
- 独立的 cgroup 子树（`/sys/fs/cgroup/sandbox/<id>/`）
- 独立的工作目录（`/tmp/sandbox/<id>/`）
- 独立的 PID namespace
- 并发上限由 `SANDBOX_MAX_CONCURRENCY` 控制

### Q: 如何调试沙箱问题？

1. **检查沙箱服务日志**：沙箱服务应该有独立的日志输出
2. **主程序日志**：Worker 日志会记录 `execute failed via sandbox` 等
3. **任务追踪**：`curl /api/trace/<taskId>` 查看完整的执行链路
4. **健康检查**：`grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check`

### Q: 沙箱服务需要和主程序同一台机器吗？

不需要。由于使用 gRPC 通信，沙箱服务和主程序可以分布在不同机器上。主程序通过 `SANDBOX_ADDRESS` 配置连接地址。但在网络延迟敏感的场景下，建议放在同一宿主机或低延迟网络中。

### Q: 如何添加新的资源限制类型？

1. 在 `proto/sandbox.proto` 的 `ResourceLimits` 中添加字段
2. 生成新的 gRPC 代码
3. 在 Go 侧更新 `types.go` 的 `ResourceLimits` 结构体
4. 在沙箱服务端实现对应 cgroup 配置
5. 在 `SandboxExecutor` 中填充新字段

这种方式保证了向后兼容——旧沙箱服务会忽略不认识的新字段。

### Q: 现有 BashTool 会自动通过沙箱执行吗？

会。启动条件满足时（`SANDBOX_ENABLED=true`、沙箱服务可用），`NodeExecutor.selectExecutor()` 会自动将 BashTool（它实现了 `BuildableTool`）路由到 `SandboxExecutor`。BashTool 本身无需任何修改。

---

## 附录 A：参考实现（Rust 伪代码）

```rust
// 沙箱服务的核心执行逻辑（Rust + tonic + nix crate）
use nix::sched::{clone, CloneFlags};
use nix::unistd::{Gid, Uid};

fn execute_in_sandbox(req: ExecuteRequest) -> ExecuteResponse {
    // 1. 创建 cgroup
    let cg = Cgroup::new(&format!("sandbox-{}", uuid()));
    cg.set_memory_max(req.limits.memory_bytes);
    cg.set_cpu_weight(req.limits.cpu_shares);
    cg.set_pids_max(req.limits.max_pids);

    // 2. 创建临时工作目录
    let work_dir = create_temp_dir();
    write_input_files(&work_dir, &req.input_files);

    // 3. 创建子进程并隔离
    let result = clone(Box::new(|| {
        // 设置 namespaces
        unshare(CloneFlags::CLONE_NEWNS | CloneFlags::CLONE_NEWPID |
                CloneFlags::CLONE_NEWNET | CloneFlags::CLONE_NEWUTS)?;

        // 降权
        set_gid(Gid::from_raw(65534))?;
        set_uid(Uid::from_raw(65534))?;

        // 执行命令
        Command::new(&req.command)
            .args(&req.args)
            .env_clear()
            .envs(&req.env)
            .current_dir(&work_dir)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
    }), ...);

    // 4. 收集结果和资源使用
    let usage = cg.read_usage();
    ExecuteResponse {
        exit_code: result.exit_code(),
        stdout: result.stdout(),
        stderr: result.stderr(),
        resource_usage: Some(usage),
        ..
    }
}
```

---

> **相关文档**：
> - [TOOL_DEVELOPMENT_GUIDE.md](./TOOL_DEVELOPMENT_GUIDE.md) — 工具开发对接指南
> - [ARCHITECTURE.md](./ARCHITECTURE.md) — 系统架构设计
> - [API_REFERENCE.md](./API_REFERENCE.md) — API 参考
> - `internal/worker/executor/sandbox.go` — 当前沙箱客户端存根
> - `internal/worker/executor/types.go` — 核心数据结构定义
> - `internal/worker/service/executor.go` — 执行调度逻辑（selectExecutor）
