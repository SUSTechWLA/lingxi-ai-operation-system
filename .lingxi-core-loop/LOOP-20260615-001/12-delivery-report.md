# 12-Delivery-Report: 长任务支持

## 1. 为什么需要修改 Core

AIOS Core 编排层原先只能处理秒级到分钟级的短任务。标书生成、视频分析等场景需要小时级的任务执行能力。进度追踪、心跳检测和 checkpoint 恢复是编排层的视频创作职责，无法通过外部 Tool、Workflow 配置或前端实现。

## 2. 修改了哪些行为、接口和状态

### 数据模型变更
- **Node 模型扩展**: 新增 5 个字段 (`LongRunning`, `Progress`, `CurrentStep`, `HeartbeatTimeoutSec`, `HeartbeatAt`)
- **NodeRequest 扩展**: DAG 提交时可声明 `longRunning` 和 `heartbeatTimeoutSec`
- **新增 NodeStatus**: `HEARTBEAT_TIMEOUT` — 心跳超时状态
- **新增 ContextType**: `NODE_PROGRESS`, `NODE_CHECKPOINT`, `NODE_HEARTBEAT_TIMEOUT`
- **新增响应类型**: `TaskProgressResponse`, `NodeProgressInfo`

### API 新增
- `GET /api/task/:taskId/progress` — 任务聚合进度查询
- 已有 `GET /api/task/:taskId` 返回的 node 对象新增 `longRunning`, `progress`, `currentStep`, `heartbeatAt` 字段

### Tool 接口新增
- `ProgressReporter` 接口 — 可选，Tool 可实现以接收进度回调
- `ProgressUpdate` 类型 — 进度数据 (progress + step + checkpoint)
- `ToolContext` 新增 `Checkpoint` 字段

### 编排层变更
- `StateMachine.OnHeartbeatTimeout()` — 心跳超时处理器
- `Scheduler.detectHeartbeatTimeout()` — 定时扫描心跳超时节点
- `NodeExecutor` — 长任务心跳 goroutine + 进度回调链
- `OrchestratorService.GetTaskProgress()` — 进度聚合

### 事件变更
- `ai.node.progress` topic 新增 HEARTBEAT、PROGRESS、CHECKPOINT 消息类型
- 新增 `ai-progress-group` Consumer Group

### 数据库变更
- `ai_node` 表新增 5 列 (long_running, progress, current_step, heartbeat_timeout_sec, heartbeat_at)
- 新增 partial index `idx_node_heartbeat`

## 3. 执行了哪些测试

```bash
go fmt ./...          # PASSED (9 files formatted)
go vet ./...           # PASSED
go build ./...         # PASSED
go test ./internal/core/model/...            # PASSED
go test ./internal/core/orchestrator/...     # PASSED (handler + service)
go test ./internal/core/context/...          # PASSED
```

## 4. 风险

| 风险 | 缓解 |
|------|------|
| Kafka 消息量增加 | 心跳间隔 30s，可配置 |
| 长任务节点消耗 goroutine | 每节点 1 个 goroutine，可控 |
| 心跳扫描增加 DB 负载 | partial index + 30s 间隔 |
| 新列兼容旧数据 | 全部有默认值 |

## 5. 部署

- `release_scope: DEVELOPMENT_ONLY`
- 数据库迁移自动执行（`ALTER TABLE ADD COLUMN IF NOT EXISTS`）
- 无需手动配置（新配置项有默认值）
- 回滚：停止使用新功能即可，短任务行为完全不受影响
