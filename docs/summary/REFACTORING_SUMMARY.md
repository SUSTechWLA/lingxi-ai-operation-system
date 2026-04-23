# 灵犀AIOS 重构总结

## 重构历程

### 阶段1：生产级重构 — "Everything is Node"
- JPA 实体类（Task、Node、NodeDependency）替代 Redis 内存模型
- PostgreSQL 持久化替代 Redis 存储
- DAGValidator 环检测和孤立节点检测
- Scheduler 定时调度 + 乐观锁抢占
- StateMachine 状态流转 + 子节点触发
- ContextService 操作历史记录

### 阶段2：去中心化架构升级 — 事件驱动调度
- 消除中心调度瓶颈，改为事件驱动依赖检查
- 统一状态收敛层，解决状态写入分散问题
- 清理遗留 Redis 模型，消除双重消费者冲突
- 添加幂等机制和指数退避重试
- 实现系统中断恢复能力

---

## 阶段2 改动详情

### 新增类

| 类 | 职责 |
|---|---|
| `StateService` | 统一状态收敛层，所有节点/任务状态变更入口 |
| `DependencyChecker` | 依赖驱动调度，消费 ai.node.executed 事件检查下游依赖 |
| `RetryPolicy` | 指数退避重试策略（1s, 2s, 4s, ... 60s max） |

### 删除类（11个）

| 类 | 原因 |
|---|---|
| `WorkerService` (orchestrator内) | 假执行器，与 ai-worker 在同一 topic 竞争消费 |
| `StateMachineService` | 遗留 Redis 状态机实现 |
| `RedisTaskRepository` | 遗留 Redis 仓储实现 |
| `model/Task` | 遗留 Redis 模型，与 entity/Task 冲突 |
| `model/Node` | 遗留 Redis 模型 |
| `model/DAG` | 遗留 Redis 模型 |
| `model/NodeStatus` | 与 entity/NodeStatus 重复 |
| `model/NodeType` | 与 entity/NodeType 重复 |
| `model/TaskStatus` | 与 entity/TaskStatus 重复 |
| `model/NodeTaskEvent` | 被 worker 包取代 |
| `model/NodeResultEvent` | 被 worker 包取代 |

### 重构类

| 类 | 改动 |
|---|---|
| `StateMachine` | 移除 triggerChildNodes，状态写入委托 StateService，发布 NodeExecuted 事件 |
| `OrchestratorService` | 状态变更委托 StateService，submitDAG 设置 idempotencyKey |
| `Scheduler` | 新增 recoverCreatedNodes() 恢复 CREATED 节点，状态更新委托 StateService |
| `TaskExecutionControl` | 使用 StateService + RetryPolicy，retryNode 先设 RETRYING 再转 CREATED |
| `EventProducer` | 使用 Map 替代已删除 model 类，新增 publishNodeExecuted/publishNodeReady(含幂等键) |
| `EventConsumer` | 移除 handleNodeReady/WorkerService 依赖，新增 onNodeExecuted 消费 |
| `Node` entity | 新增 idempotencyKey 字段 |
| `NodeStatus` entity | 新增 RETRYING 状态 |
| `NodeRepository` | 新增 findCreatedNodes() 查询 |
| `TaskController` | 新增 POST /api/node 端点（一步创建+提交） |

### Worker 侧改动

| 文件 | 改动 |
|---|---|
| `NodeTaskEvent` | 新增 idempotencyKey 字段 |
| `NodeResultEvent` | 新增 idempotencyKey 字段 |
| `NodeExecutor` | 提取并传递 idempotencyKey，更新所有方法签名 |
| `EventProducer` | publishNodeResult 使用 idempotencyKey 作为 Kafka 消息 key |

### Context 侧改动

| 文件 | 改动 |
|---|---|
| `ContextEventConsumer` | 新增 ai.node.executed topic 监听 |

---

## 架构变更对比

### Before（中心化调度）
```
Worker → ai.node.result → StateMachine → 直接触发子节点(CREATED→READY)
                                         → Scheduler 轮询 READY 节点
```

### After（事件驱动调度）
```
Worker → ai.node.result → StateMachine → StateService(状态收敛)
                                        → publish(ai.node.executed)
         → DependencyChecker 消费 → StateService.checkDependenciesMet
                                  → publish(ai.node.ready)
         → Scheduler 恢复 CREATED 节点（系统中断恢复）
```

---

## 新增 Kafka Topic

| Topic | 生产者 | 消费者 | 说明 |
|-------|--------|--------|------|
| `ai.node.executed` | StateMachine | DependencyChecker, Context | 节点执行完成事件 |
| `ai.node.failed` | StateMachine | Context | 节点永久失败事件 |
| `ai.task.success` | StateService | Context | 任务成功事件 |
| `ai.task.failed` | StateService | Context | 任务失败事件 |

---

## 数据结构变更

### Node Entity 新增字段
- `idempotencyKey` (VARCHAR(128), UNIQUE) — 幂等键，格式 `taskId + "-" + nodeId`

### NodeStatus 新增状态
- `RETRYING` — 重试中，区分首次 CREATED 和重试等待

---

## 核心设计理念

```
1. 一切皆 Node（Tool / LLM / Log / Control）
2. DAG = Node表 + Dependency表（禁止JSON DAG）
3. Orchestrator 必须纯确定性（不调用LLM）
4. Node 是最小执行单元
5. 状态必须可持久化（支持恢复/重试）
6. Worker 与 Orchestrator 解耦（事件驱动）
7. 所有交互统一JSON协议
8. 状态变更统一收敛（StateService）
9. 调度依赖驱动（DependencyChecker，非代码直接调用）
10. 幂等执行保障（idempotencyKey）
```
