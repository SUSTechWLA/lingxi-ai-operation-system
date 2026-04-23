# 灵犀AIOS 生产就绪状态

## 系统架构

```
┌──────────────┐     REST API      ┌────────────────┐
│ NL-Translator│ ────────────────▶ │ AI-Orchestrator│
│  (8081)      │   DAG JSON        │   (8080)       │
└──────────────┘                    └───────┬────────┘
       ▲                                    │
       │              ┌─────────────────────┼──────────────────┐
       │              │                     │                  │
       │              ▼                     ▼                  ▼
       │       ┌──────────┐        ┌──────────────┐   ┌──────────┐
       │       │AI-Context │        │   Redpanda   │   │ AI-Worker│
       │       │  (8082)  │        │  Event Bus   │   │  (8083)  │
       │       └──────────┘        └──────────────┘   └──────────┘
       │          (记录历史)      ┌───────┼────────┐      (执行工具)
       │                         │       │        │
       │                         ▼       ▼        ▼
       │               ai.node.executed  ai.node   ai.node.result
       │               ai.task.*        .ready     (idempotencyKey)
       │                                (idempotencyKey)
       └──────── 查询任务状态 ──────────────────────────────┘
```

---

## 已完成的核心模块

### 1. 数据层
- **JPA 实体类**：Task、Node（含 idempotencyKey）、NodeDependency
- **枚举**：TaskStatus、NodeStatus（含 RETRYING）、NodeType
- **Repository**：TaskRepository、NodeRepository（含 findCreatedNodes、乐观锁）、NodeDependencyRepository
- **数据库**：PostgreSQL 16，schema.sql 自动建表

### 2. 核心服务层（去中心化架构）
- **StateService** — 统一状态收敛层
  - 所有节点/任务状态变更唯一入口
  - checkDependenciesMet() 依赖检查
  - checkTaskCompleted/checkTaskFailed() 任务终态判断
  - initializeNodeReady() 初始化无依赖节点

- **DependencyChecker** — 依赖驱动调度
  - 消费 ai.node.executed 事件
  - 检查下游节点依赖是否满足
  - 满足则 transitionNode → READY，发布 ai.node.ready

- **RetryPolicy** — 指数退避重试
  - 延迟: min(2^retryCount * 1000, 60000) ms
  - shouldRetry(retryCount, maxRetry) 判断是否可重试

- **DAGValidator** — DAG 校验器
  - 环检测、孤立节点检测

- **Scheduler** — 调度器 + 恢复器
  - 定时扫描 READY 节点（每秒）
  - recoverCreatedNodes() 恢复中断后的 CREATED 节点
  - 乐观锁抢占

- **StateMachine** — 状态机（重构后）
  - onSuccess: StateService 转状态 + publish ai.node.executed
  - onFailure: RetryPolicy 判断重试或永久失败
  - 不再直接触发子节点（由 DependencyChecker 事件驱动）

- **OrchestratorService** — 核心编排服务
  - createTask / submitDAG（含 idempotencyKey 设置）/ getTaskWithDetails

### 3. AI-Worker 执行层
- **内置工具**：BashTool、LlmApiTool
- **外部工具**：注册/注销/健康检查/执行
- **事件消费**：ai.node.ready（含 idempotencyKey）
- **事件生产**：ai.node.result（idempotencyKey 作为 Kafka key）
- **超时控制**：WorkerConfig.toolTimeoutSeconds

### 4. AI-Context 上下文层
- **操作历史记录**：Task/Node 全生命周期事件
- **节点快照**：保存/恢复/查询
- **Kafka 事件自动记录**：监听 12 个 topic（含 ai.node.executed）

### 5. API 层
| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/task/create` | POST | 创建任务 |
| `/api/task/{taskId}/dag` | POST | 提交 DAG |
| `/api/node` | POST | 一步创建+提交（NL-Translator 专用） |
| `/api/task/{taskId}` | GET | 查询任务详情 |
| `/api/task/{taskId}/context` | GET | 查询任务上下文 |
| `/api/task/{taskId}/pause` | POST | 暂停任务 |
| `/api/task/{taskId}/resume` | POST | 恢复任务 |
| `/api/task/{taskId}/pause-reason` | GET | 获取暂停原因 |
| `/api/node/{nodeId}/success` | POST | 标记节点成功 |
| `/api/node/{nodeId}/failure` | POST | 标记节点失败 |
| `/api/node/{nodeId}/retry` | POST | 重试失败节点 |
| `/api/node/{nodeId}/snapshot/latest` | GET | 获取节点快照 |
| `/api/node/{nodeId}/restore` | POST | 从快照恢复 |
| `/api/health` | GET | 健康检查 |

---

## 事件驱动架构

### Kafka Topics

| Topic | 生产者 | 消费者 | 说明 |
|-------|--------|--------|------|
| `ai.node.ready` | DependencyChecker | Worker | 节点就绪（含 idempotencyKey） |
| `ai.node.result` | Worker | Orchestrator | 节点执行结果（含 idempotencyKey） |
| `ai.node.executed` | StateMachine | DependencyChecker, Context | 节点执行完成 |
| `ai.node.failed` | StateMachine | Context | 节点永久失败 |
| `ai.task.created` | OrchestratorService | Context | 任务创建 |
| `ai.task.success` | StateService | Context | 任务成功 |
| `ai.task.failed` | StateService | Context | 任务失败 |

### 节点状态流转

```
CREATED ──(依赖满足)──▶ READY ──(Worker消费)──▶ RUNNING ──(执行成功)──▶ SUCCESS
   ▲                                                    │
   │                                          (执行失败，可重试)
   │                                                    │
   │                                                    ▼
   │                                    RETRYING ──(退避后)──▶ CREATED
   │                                                    │
   │                                          (达到最大重试次数)
   │                                                    │
   └────────────────────────────────────────────────────┘
                                                    FAILED（永久失败）
```

---

## 生产级特性

- **幂等执行**: idempotencyKey（taskId + "-" + nodeId）贯穿 Kafka 消息和数据库
- **指数退避重试**: 1s → 2s → 4s → 8s → 16s → 32s → 60s（最大）
- **系统恢复**: Scheduler recoverCreatedNodes() 自动恢复中断后的 CREATED 节点
- **状态收敛**: StateService 统一所有状态变更入口，防止并发写冲突
- **事件驱动调度**: DependencyChecker 替代代码直接触发，解耦调度与执行
- **双重消费者冲突已解决**: 移除 Orchestrator 内的 WorkerService，Worker 独占 ai.node.ready 消费
- **遗留代码清理**: 11 个 Redis 遗留类已删除，消除双轨模型混淆

---

## 快速启动

```bash
# 1. 启动基础设施
docker compose up -d

# 2. 启动所有服务
./scripts/startup.sh

# 3. 启动外部天气工具（示例）
cd examples && pip install fastapi uvicorn && python weather_tool.py &

# 4. 注册外部工具
curl -X POST http://localhost:8083/worker/register \
  -H "Content-Type: application/json" \
  -d '{"endpoint": "http://localhost:8090"}'

# 5. 测试完整流程
# 创建任务 → 提交天气查询DAG → Worker执行 → 查询结果
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{"nodes":[{"id":"query-weather","type":"TOOL","name":"weather_query","input":{"tool":"weather_query","parameters":{"city":"北京"}},"maxRetry":3}],"edges":[]}'

sleep 5
curl "http://localhost:8080/api/task/${TASK_ID}"

# 6. 运行API测试套件
./scripts/test-apis.sh
```

---

## 文档索引

| 文档 | 内容 |
|------|------|
| [API_REFERENCE.md](../API_REFERENCE.md) | 完整 API 参考 |
| [ORCHESTRATOR_GUIDE.md](../guides/ORCHESTRATOR_GUIDE.md) | Orchestrator 详解 |
| [WORKER_GUIDE.md](../guides/WORKER_GUIDE.md) | Worker 详解 |
| [AI_CONTEXT_GUIDE.md](../guides/AI_CONTEXT_GUIDE.md) | Context 详解 |
| [NL_TRANSLATOR_GUIDE.md](../guides/NL_TRANSLATOR_GUIDE.md) | NL-Translator 详解 |
| [REFACTORING_SUMMARY.md](./REFACTORING_SUMMARY.md) | 重构历程总结 |
