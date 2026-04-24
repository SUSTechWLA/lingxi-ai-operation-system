# 系统架构设计

灵犀AI OS 的架构设计、数据流、组件关系和设计决策。

---

## 1. 系统架构

### 1.1 整体架构

灵犀AI OS 采用**单体模块化架构**（Modular Monolith），所有功能模块编译为一个 Go 二进制文件，内部保持清晰的模块边界。

```
┌────────────────────────────────────────────────────────────────────┐
│                      Go 单进程 (端口 8080)                          │
│                                                                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
│  │ Orchestrator  │  │    Worker     │  │  Translator   │             │
│  │  调度引擎     │  │  工具执行     │  │  NL→DAG 转换  │             │
│  │              │  │              │  │              │             │
│  │ StateService │  │ NodeExecutor │  │ NlToDagSvc   │             │
│  │ StateMachine │  │ ToolRegistry │  │ OpenAI Client│             │
│  │ DepChecker   │  │ BashTool     │  │              │             │
│  │ Scheduler    │  │ LlmApiTool   │  │              │             │
│  │ RetryPolicy  │  │ WeatherTool  │  │              │             │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘             │
│         │                 │                  │                     │
│  ┌──────▼─────────────────▼──────────────────▼──────────┐         │
│  │                   Context 审计服务                     │         │
│  │              事件记录 · 快照 · 恢复                    │         │
│  └───────────────────────┬──────────────────────────────┘         │
│                          │                                         │
│  ┌───────────────────────▼──────────────────────────────┐         │
│  │                  EventBus (Kafka)                     │         │
│  │            Producer · Consumer · Topic Router         │         │
│  └───────────────────────┬──────────────────────────────┘         │
│                          │                                         │
│  ┌───────────────────────▼──────────────────────────────┐         │
│  │               Outbox 发件箱模式                       │         │
│  │     DB 先写 → Relay 协程 100ms 轮询 → Kafka 转发      │         │
│  └───────────────────────┬──────────────────────────────┘         │
│                          │                                         │
│  ┌───────────┐  ┌───────▼───────┐  ┌──────────┐                  │
│  │  Config    │  │   Database     │  │   Redis   │                  │
│  │  (Viper)   │  │   (pgx Pool)  │  │ (go-redis)│                  │
│  └───────────┘  └───────────────┘  └──────────┘                  │
└────────────────────────────────────────────────────────────────────┘
         │                │                │
    ┌────▼────┐    ┌──────▼──────┐   ┌─────▼─────┐
    │  .env   │    │ PostgreSQL   │   │   Redis    │
    │ 配置文件 │    │   16         │   │   7        │
    └─────────┘    └─────────────┘   └───────────┘
                        │
                  ┌─────▼──────┐
                  │  Redpanda   │
                  │  (Kafka)    │
                  └────────────┘
```

### 1.2 为什么选择单体模块化

| 考量 | 微服务 | 单体模块化 (当前) |
|------|--------|-------------------|
| 开发复杂度 | 高 (4 个项目、4 套配置) | 低 (1 个项目、1 套配置) |
| 部署 | 4 次部署 | 1 次部署 |
| 调试 | 跨进程追踪困难 | 单进程，直接打断点 |
| 性能 | HTTP 调用有网络开销 | 内部调用零开销 |
| 扩展性 | 可独立扩缩 | 整体扩缩 |
| 适合阶段 | 成熟期大规模 | 早期快速迭代 |

> 当前项目处于初级阶段，单体模块化是最佳选择。未来如需拆分，模块边界已清晰，可直接按 `internal/` 下的目录拆为独立服务。

---

## 2. 模块设计

### 2.1 Orchestrator — 调度引擎

**职责**：接收 DAG 任务、管理状态流转、驱动依赖调度

**核心组件**：

```
OrchestratorService
  ├── DAGValidator        验证 DAG 合法性 (环检测、重复节点、引用完整性)
  ├── StateService        统一状态转换入口
  │     ├── 状态转换        Task/Node 状态变更 + outbox 事件发布
  │     ├── 依赖检查        判断节点依赖是否全部满足 (SUCCESS + SKIPPED 均满足)
  │     ├── TryMakeReady   节点创建后立即检查依赖，满足则转 READY (事件驱动)
  │     └── 任务完成判断     判断所有节点是否成功或跳过
  ├── StateMachine        节点成功/失败状态机
  │     ├── onSuccess      转换→SUCCESS, outbox 发布事件, 检查任务完成
  │     └── onFailure      判断重试 or 永久失败
  ├── DependencyChecker   依赖驱动调度
  │     ├── onNodeExecuted 上游完成→检查下游→评估条件→标记 READY→outbox 发布
  │     └── evaluateCondition 条件评估 ("nodeId.status == success" 格式)
  ├── Scheduler           兜底恢复 (30s 间隔，仅处理停滞 >1 分钟的 CREATED 节点)
  ├── RetryPolicy         指数退避 (1s→2s→4s→...→60s)
  └── TaskExecutionControl 暂停/恢复/重试控制 (持久化 pause_reason)
```

**数据流**：

```
                    创建任务 + 提交 DAG
                          │
                    DAGValidator 验证
                          │
                    保存 Node + Edge 到 DB
                          │
                    TransitionTask → RUNNING
                          │
                    初始化无依赖节点 → TryMakeReady → READY
                          │
                    outbox 写入 ai.node.ready 事件
                          │
                    Relay 协程转发到 Kafka
                          │
          ┌───────────────┼───────────────┐
          │               │               │
    Worker 执行      Worker 执行     Worker 执行
          │               │               │
    ai.node.result   ai.node.result  ai.node.result
          │               │               │
    StateMachine.OnSuccess/OnFailure
          │
    DependencyChecker → 评估条件 → 检查下游 → 标记 READY → outbox 发布
          │
    全部完成或跳过 → TransitionTask → SUCCESS
```

**条件分支机制**：

DAG 节点支持 `condition` 字段，格式为 `"nodeId.status == success"` 或 `"nodeId.status == failed"`。

- 依赖满足后，DependencyChecker 先评估 condition
- 条件不满足 → 节点标记为 SKIPPED（对下游等同于 SUCCESS，不阻塞）
- 条件满足 → 正常转为 READY 执行

```
  [节点A: 调用API] ──→ [节点B: 解析结果, condition="A.status == success"]
       │                        │
       │                   A成功 → B执行
       │                   A失败 → B跳过 (SKIPPED)
       │
       └──→ [节点C: 错误通知, condition="A.status == failed"]
                    │
               A成功 → C跳过 (SKIPPED)
               A失败 → C执行
```

### 2.2 Worker — 工具执行引擎

**职责**：消费节点就绪事件，调用工具执行，发布结果

**核心组件**：

```
NodeExecutor
  ├── 确定工具名    DetermineToolName (payload.tool > payload.name > "llm_api")
  ├── 提取参数      ExtractParameters (payload.parameters > payload.input > payload)
  ├── 查找工具      ToolRegistry.Get(name)
  ├── 验证参数      Tool.ValidateParameters(params)
  ├── 超时控制      goroutine + time.After
  └── 发布结果      ai.node.result (SUCCESS/FAILED) via outbox
```

**工具接口设计**：

```go
type Tool interface {
    Name() string                                              // 唯一标识
    Description() string                                       // 描述
    Type() ToolType                                            // LLM / CUSTOM
    Execute(ctx context.Context, params map[string]interface{}, // 执行
            toolCtx ToolContext) ToolResult
    ValidateParameters(params map[string]interface{}) bool      // 参数校验
}
```

**已实现工具**：

| 工具 | 类型 | 说明 |
|------|------|------|
| `bash` | CUSTOM | 沙箱执行 Shell 命令（白名单 + 危险模式过滤 + /tmp/ai-sandbox 目录） |
| `llm_api` | LLM | 调用 OpenAI 兼容 API（支持 prompt/message/content 字段） |
| `weather` | CUSTOM | 天气查询示例工具（模拟数据，展示 TOOL 类型工具开发） |

**BashTool 安全设计**：

```
命令白名单:
  ls, cat, echo, curl, python, python3, node,
  head, tail, wc, grep, find, which, whoami, date, pwd, uname, df, ps

危险模式过滤:
  ;  |  &&  ||  `  $(  >  <  >>  <<  &

沙箱目录:
  /tmp/ai-sandbox (所有命令在此目录下执行)
```

**工具名路由**：

- TOOL 类型节点：使用 `payload["name"]`（即节点名称，如 "weather"）作为工具名
- LLM 类型节点：固定路由到 `"llm_api"`
- 显式指定：`payload["tool"]` 优先级最高

### 2.3 NL-Translator — 自然语言翻译器

**职责**：将自然语言转为结构化 DAG

**流程**：
1. 构造 System Prompt（指导 LLM 输出 DAG JSON）
2. 调用 OpenAI Chat Completions API
3. 解析 LLM 返回的 JSON 为 `DAGRequest`
4. 可选：直接提交到 Orchestrator

**System Prompt 要点**：
- 指定输出格式：`{"nodes": [...], "edges": [...]}`
- 定义节点字段：id, type, name, input, deps
- 要求纯 JSON，无其他文字

### 2.4 Context — 上下文审计

**职责**：记录所有状态变更事件，提供快照恢复能力

**记录的事件类型**：

| ContextType | 触发时机 |
|-------------|---------|
| TASK_CREATED | 任务创建 |
| DAG_VALIDATED | DAG 验证通过 |
| DAG_SUBMITTED | DAG 提交 |
| NODE_READY | 节点就绪 |
| NODE_SCHEDULED | 节点开始执行 |
| NODE_SUCCESS | 节点成功 |
| NODE_FAILED | 节点失败 |
| NODE_RETRY | 节点重试 |
| NODE_SKIPPED | 节点条件不满足被跳过 |
| TASK_SUCCESS | 任务成功 |
| TASK_FAILED | 任务失败 |

---

## 3. 数据模型

### 3.1 ER 图

```
┌──────────────┐       ┌──────────────────┐
│   ai_task     │       │     ai_node       │
├──────────────┤       ├──────────────────┤
│ id (PK)      │──┐    │ id (PK)          │
│ user_id      │  │    │ task_id (FK)     │◄─┘
│ status       │  │    │ type             │
│ input (JSONB)│  └───→│ name             │
│ output(JSONB)│       │ status           │
│ pause_reason │       │ input (JSONB)    │
│ created_at   │       │ output (JSONB)   │
└──────────────┘       │ error_message    │
                       │ condition        │
                       │ retry_count      │
                       │ max_retry        │
                       │ priority         │
                       │ worker_group     │
                       │ version          │
                       │ idempotency_key  │
                       │ created_at       │
                       └────────┬─────────┘
                                │
               ┌────────────────┤
               │                │
┌──────────────▼────┐  ┌───────▼──────────────┐
│ ai_node_dependency│  │     ai_context        │
├───────────────────┤  ├──────────────────────┤
│ parent_node_id(PK)│  │ id (BIGSERIAL, PK)   │
│ child_node_id(PK) │  │ context_type         │
└───────────────────┘  │ task_id              │
                       │ node_id              │
┌───────────────────┐  │ metadata (JSONB)     │
│     outbox        │  │ message              │
├───────────────────┤  │ snapshot_data (JSONB)│
│ id (BIGSERIAL,PK) │  │ created_at           │
│ aggregate_type    │  └──────────────────────┘
│ aggregate_id      │
│ event_type        │
│ payload (JSONB)   │
│ created_at        │
└───────────────────┘
```

### 3.2 表说明

| 表 | 说明 | 关键索引 |
|----|------|---------|
| `ai_task` | 任务主表 | `id` (PK) |
| `ai_node` | 节点表 | `task_id`, `status`, `idempotency_key` (UNIQUE) |
| `ai_node_dependency` | DAG 边表 | `(parent_node_id, child_node_id)` (PK) |
| `ai_context` | 上下文审计表 | `task_id`, `node_id` |
| `outbox` | 发件箱事件表 | `id` (PK, ASC) |

### 3.3 版本控制

`ai_node.version` 字段实现乐观锁，每次更新自动 +1，防止并发修改冲突。

---

## 4. 事件系统

### 4.1 Outbox 发件箱模式

所有 Kafka 事件先写入 `outbox` 数据库表，再由 Relay 协程异步转发到 Kafka。

```
┌──────────────┐    写入     ┌──────────┐   转发    ┌──────────┐
│ StateService │───────────▶│  outbox   │─────────▶│  Kafka   │
│ StateMachine │   (DB事务) │  (DB表)   │ (100ms   │(Redpanda)│
│ DepChecker   │            │          │  轮询)    │          │
└──────────────┘            └──────────┘          └──────────┘
```

**保证**：
- 事件不丢：outbox 写入与业务操作在同一个 DB 事务中
- Kafka 不可用时：事件留在 outbox 表，等待下次转发
- 转发成功后：立即从 outbox 删除
- Relay 实时性：100ms 轮询间隔，兼顾性能与延迟

### 4.2 事件主题

```
┌─────────────┐  ai.node.ready   ┌──────────┐
│ Orchestrator │─────────────────▶│  Worker   │
│ (via outbox) │                  │(Consumer) │
└──────┬───────┘                  └─────┬─────┘
       │                                │
       │  ai.node.result                │
       │  ai.node.executed              │
       │◀───────────────────────────────┘
       │
       │  ai.node.failed
       │  ai.task.completed
       │  ai.task.failed
       │
       ▼
┌─────────────┐
│   Context    │  (消费所有事件，记录审计)
│ (Consumer)   │
└─────────────┘
```

### 4.3 消费者组

| Group ID | 订阅主题 | 说明 |
|----------|---------|------|
| `ai-worker-group` | `ai.node.ready` | Worker 消费就绪事件，执行节点 |
| `orchestrator-group` | `ai.node.result`, `ai.node.executed` | Orchestrator 处理执行结果 |
| `ai-context-group` | `ai.node.result`, `ai.node.executed`, `ai.node.failed`, `ai.task.completed`, `ai.task.failed` | Context 记录审计 |

### 4.4 幂等性

- Kafka 消息 key 使用 `idempotencyKey`（格式：`taskId + "-" + nodeId`）
- 相同 key 的消息由 Kafka 保证分区内有序
- Worker 执行前检查幂等 key，避免重复执行

---

## 5. 可靠性设计

### 5.1 事件驱动调度

节点依赖满足后立即触发下游调度，无需轮询：

```
节点A 完成成功
    │
    ▼
StateMachine.OnSuccess → 发布 ai.node.executed
    │
    ▼
DependencyChecker.OnNodeExecuted → 查找子节点 → 检查依赖
    │
    ├── 依赖满足 → 评估 condition → READY → outbox 发布 ai.node.ready
    └── 依赖不满足 → 等待更多上游完成
```

**TryMakeReady**：节点创建时（CREATED 状态），StateService 立即检查依赖，满足则直接转为 READY，实现毫秒级调度。

### 5.2 兜底恢复

Scheduler 作为最后保障，30 秒扫描一次，仅处理停滞超过 1 分钟的 CREATED 节点。正常情况下不应触发，仅在事件丢失等异常场景下起作用。

### 5.3 重试策略

```
指数退避: 1s → 2s → 4s → 8s → 16s → 32s → 60s (max)
最大重试次数: 默认 3 次 (可通过 DAG 节点的 maxRetry 配置)
```

- 节点失败后，如果 `retryCount < maxRetry`，状态转为 `RETRYING` → `CREATED`
- TryMakeReady 立即检查依赖，满足则转为 `READY`
- 超过最大重试次数，节点永久失败，任务暂停

### 5.4 任务暂停机制

当任意节点永久失败时：
1. 任务自动转为 `PAUSED`，`pause_reason` 持久化到数据库
2. 检查是否还有可重试的节点
3. 如果无可重试节点 → 任务 `FAILED`
4. 用户可手动 `POST /api/node/:nodeId/retry` 重试
5. 可通过 `GET /api/task/:taskId/pause-reason` 查询暂停原因

### 5.5 优雅关闭

```go
// 监听系统信号
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

// 收到信号后：
// 1. HTTP Server 优雅关闭 (10s 超时)
// 2. Kafka Consumer 停止消费
// 3. Kafka Producer 关闭
// 4. Outbox Relay 停止
// 5. PostgreSQL 连接池关闭
// 6. Redis 连接关闭
```

### 5.6 数据库迁移

启动时自动执行建表语句（`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`），确保表结构一致。兼容从 Java 版本升级的存量数据（自动补齐新列）。无额外迁移工具依赖。

---

## 6. 配置系统

配置通过 `.env` 文件管理，使用 Viper 加载：

```
.env 文件 → Viper 读取 → 环境变量覆盖 → Config 结构体
```

**优先级**：环境变量 > .env 文件 > 默认值

所有配置项在 `internal/config/config.go` 的 `setDefaults()` 中定义默认值，无需 .env 文件也可启动（除 `OPENAI_API_KEY` 外）。

---

## 7. 日志系统

使用 Zap 结构化日志，两个模式：

| 模式 | 触发 | 输出格式 |
|------|------|---------|
| development | 默认 | 带颜色的控制台输出 |
| production | `GIN_MODE=release` | JSON 格式 |

**日志规范**：
- 关键操作（状态转换、事件发布）记录 Info
- 失败操作记录 Error
- 非 critical 的上下文记录失败记录 Warn
- Scheduler 恢复停滞节点记录 Warn（正常情况不应触发）

---

## 8. 扩展点

### 8.1 添加新工具

实现 `Tool` 接口，在 `main.go` 中注册即可。无需修改其他代码。

示例：参考 `internal/worker/tool/builtin/weather_tool.go`，一个最简 TOOL 类型工具只需实现 5 个方法。

### 8.2 添加新 Kafka 事件

1. 在 `eventbus/eventbus.go` 中定义 Topic 常量
2. 在发布处调用 `outbox.SaveEvent()`（不再直接调用 `producer.Publish()`）
3. 创建 Consumer 订阅该 Topic

### 8.3 拆分为微服务

模块边界已清晰，每个 `internal/` 下的子目录可独立拆分：
- `internal/orchestrator/` → Orchestrator 服务
- `internal/worker/` → Worker 服务
- `internal/translator/` → Translator 服务
- `internal/context/` → Context 服务

拆分后模块间通过 Kafka 和 HTTP 通信，当前代码中已预留了 `ServicesConfig.OrchestratorURL` 和 `ServicesConfig.ContextServiceURL`。

---

## 9. 基础设施依赖

| 服务 | 版本 | 用途 | 必须启动 |
|------|------|------|---------|
| PostgreSQL | 16 | 持久化 + Outbox 表 | 是 |
| Redis | 7 | 缓存 | 否 (启动但不使用不影响核心功能) |
| Redpanda | latest | 事件总线 | 是 |
| MinIO | latest | 对象存储 | 否 (未使用) |
| Qdrant | latest | 向量数据库 | 否 (预留) |

启动命令：`docker compose up -d`
