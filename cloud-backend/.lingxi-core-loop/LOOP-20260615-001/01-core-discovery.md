# 01-Core-Discovery: 当前 AIOS Core 架构与长任务缺口

## 1. 架构总览

经过代码库探索，当前 Core 的实际目录结构为：

```
internal/core/
  orchestrator/   — 任务编排、DAG 生命周期、状态机、调度器
  worker/         — 节点执行引擎、工具注册、执行器
  translator/     — NL→DAG 翻译
  context/        — 审计日志、任务历史
  eventbus/       — Kafka 事件发布/订阅
  outbox/         — 出箱模式保证事件投递
  database/       — PostgreSQL 连接池 + 迁移
  redis/          — Redis 客户端
  model/          — 数据模型
  workflow/       — 工作流模板

internal/agents/
  bid/            — 标书生成（业务层）
  chat/           — AI 对话助手（业务层）
  publish/        — 内容发布（业务层）
```

## 2. 当前节点状态机

```
CREATED → READY → RUNNING → SUCCESS
                   ↓
                 FAILED → RETRYING → CREATED (重试)
                   ↓ (超过 MaxRetry)
                 永久 FAILED
                 
SKIPPED (条件不满足，等同于满足依赖)
```

关键特点：
- 从 RUNNING 到 SUCCESS/FAILED 是一次性同步转换
- NodeExecutor.ExecuteNode() 完全是同步的：等工具完成 → 发布结果
- 没有"仍在运行中"的中间进度表达
- 失败后只能从 CREATED 重新开始整个节点

## 3. 缺口分析

### 3.1 进度追踪 (缺失)

- `TopicProgress` 常量已在 eventbus 中定义但从未使用
- `ProgressEvent` 模型已在 model.go 中定义但未在 Core 使用
- Node 模型没有 progress 字段
- 没有 API 查询节点/任务进度

### 3.2 心跳/存活检测 (缺失)

- Scheduler 只恢复 stale CREATED 节点（超过 1 分钟未转为 READY）
- 没有检测 stuck RUNNING 节点的机制
- 长时间运行的节点如果 Worker 崩溃，会永远停留在 RUNNING 状态

### 3.3 失败回溯与恢复 (缺失)

- 重试从 CREATED 重新开始 → 丢失中间状态
- 没有 checkpoint 机制
- 没有"从上次断点继续"的能力
- Node 模型没有 checkpoint 字段

### 3.4 长任务记录 (部分存在，不足)

- Context 审计日志记录状态转换（NODE_READY, NODE_RUNNING, NODE_SUCCESS 等）
- 但缺少：进度日志、中间输出、阶段性结果
- Context 模型有 `SnapshotData` 字段但几乎不使用

### 3.5 超时机制 (不完整)

- Worker 层有执行超时（来自配置的 `ToolTimeoutSeconds`）
- 但没有长任务的心跳超时概念（"如果 5 分钟没收到心跳就标记失败"）
- 超时后没有恢复路径

## 4. 已有基础设施（可利用）

| 组件 | 状态 | 可用于 |
|------|------|--------|
| PostgreSQL | 就绪 | 持久化进度/心跳/checkpoint |
| Redis | 就绪 | 实时进度缓存 |
| Kafka 事件 | 就绪 | Progress 事件分发 |
| Outbox 模式 | 就绪 | 可靠事件投递 |
| Context 审计 | 就绪 | 扩展记录类型 |
| `TopicProgress` | 已定义未使用 | 进度事件通道 |
| `ProgressEvent` | 已定义未集成 | 进度数据结构 |

## 5. 影响范围

需要修改的 Core 模块：
1. `internal/core/model/` — 扩展 Node 模型
2. `internal/core/orchestrator/service/` — 状态机、调度器、状态服务
3. `internal/core/worker/service/` — 执行器支持长任务模式
4. `internal/core/context/service/` — 扩展审计类型
5. `internal/core/database/` — 数据库迁移
6. `internal/core/eventbus/` — Progress 消费者
7. `internal/core/orchestrator/handler/` — 新 API 端点

不修改：
- agents/ 业务模块（bid/chat/publish）
- worker/tool/ 工具接口（tool 通过新的回调上报进度）
- 外部工具
