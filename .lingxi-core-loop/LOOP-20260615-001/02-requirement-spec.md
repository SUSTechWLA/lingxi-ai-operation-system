# 02-Requirement-Spec: 长任务支持

## 概述

为 AIOS Core 编排层添加系统性的长任务支持能力，包括：进度追踪、心跳检测、失败恢复（checkpoint）、实时状态查询。保持对现有短任务的完全向后兼容。

---

## 功能需求

### REQ-001: 长任务节点声明
节点在创建时可以声明自己是"长任务"（long_running: true），以便编排层区分处理策略。
- 短任务节点行为不变
- 长任务节点启用心跳检测和进度追踪

### REQ-002: 节点进度上报
长任务执行中的 Tool 可以通过回调上报进度（百分比 0.0-1.0 + 阶段性描述）。
- 进度数据持久化到 PostgreSQL
- 进度事件通过 Kafka Topic `ai.node.progress` 发布
- 进度缓存到 Redis 以便快速查询

### REQ-003: 节点进度查询 API
提供 HTTP API 查询节点和任务的实时进度。
- `GET /api/task/:taskId/progress` — 任务整体进度（所有节点加权）
- `GET /api/node/:nodeId/progress` — 单节点详情（百分比、当前步骤、预估剩余时间）

### REQ-004: 心跳机制
长任务节点运行时，Worker 定期上报心跳。编排层检测心跳超时。
- Node 模型增加 `heartbeat_at` 字段
- Scheduler 扩展：扫描 RUNNING 状态的长任务节点
- 心跳超时（默认 5 分钟）→ 标记为 HEARTBEAT_TIMEOUT → 触发重试或失败

### REQ-005: Checkpoint 保存与恢复
长任务可以在执行过程中保存 checkpoint（中间状态快照）。
- Tool 通过 `ToolContext` 回调保存 checkpoint 数据
- checkpoint 数据持久化到 `ai_context` 表的 `snapshot_data` 字段
- 节点重试时，checkpoint 数据注入到 ToolContext，Tool 可以从中断处恢复

### REQ-006: 节点状态扩展
扩展 Node 状态机支持长任务特有状态。

新增状态：
- `HEARTBEAT_TIMEOUT` — 心跳超时
- `RECOVERING` — 正在从 checkpoint 恢复

现有 RUNNING 状态语义扩展：
- 对于长任务，RUNNING 包含心跳追踪

### REQ-007: 任务进度事件消费
新增 Kafka Consumer 消费 `ai.node.progress` 事件，将进度数据持久化并更新缓存。

### REQ-008: Tool 接口扩展
Tool 接口新增可选接口 `ProgressReporter`，允许 Tool 在执行期间上报进度和 checkpoint。

```go
type ProgressReporter interface {
    SetProgressCallback(cb ProgressCallback)
}

type ProgressCallback func(ctx context.Context, progress ProgressUpdate)
```

### REQ-009: 节点 API 返回进度信息
`GET /api/task/:taskId` 返回数据中 node 对象增加进度字段：
```json
{
  "progress": 0.75,
  "currentStep": "正在生成第3章...",
  "heartbeatAt": "2026-06-15T10:30:00Z"
}
```

---

## 验收标准

### AC-001: 短任务行为不变
给定一个不声明 long_running 的 DAG 节点，其行为与现在完全一致。

### AC-002: 长任务进度可见
创建一个 long_running=true 的节点，Tool 上报 3 次进度（0.3, 0.6, 1.0），通过 API 能查询到每次进度更新。

### AC-003: 心跳超时自动恢复
长任务节点在 RUNNING 状态超过 5 分钟无心跳，系统自动将其标记为 HEARTBEAT_TIMEOUT 并触发重试。

### AC-004: Checkpoint 恢复
长任务节点执行到 50% 时保存 checkpoint，然后失败触发重试。重试时 ToolContext 中包含 checkpoint 数据，Tool 可以从 50% 恢复。

### AC-005: 任务进度聚合
一个包含 3 个节点的 DAG 任务（2 个已完成，1 个 50% 进度），`GET /api/task/:taskId/progress` 返回正确聚合的进度百分比。

### AC-006: 进度事件投递
每次进度更新都有对应的 Kafka 事件发布到 `ai.node.progress` topic。

---

## 非功能需求

### NFR-001: 性能
- 进度查询延迟 < 50ms（Redis 缓存）
- 心跳扫描不增加超过 5% 的数据库负载

### NFR-002: 兼容性
- 所有新增 Node 字段都有默认值，不影响已有记录
- 新增状态转换不影响已有状态机的正确性
- 现有 API 响应结构不变（只在 node 对象内增加字段）

### NFR-003: 数据库
- 新增列使用 ALTER TABLE ADD COLUMN IF NOT EXISTS
- 不修改已有列
- 新增索引不影响写入性能

### NFR-004: 并发安全
- 心跳更新使用乐观锁（version 字段）
- 进度更新幂等（相同或更低的 progress 值忽略）
