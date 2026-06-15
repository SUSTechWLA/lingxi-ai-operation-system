# 05-Core-Impact-Analysis: Core 影响分析

## CORE-001: CONTROL 节点自动暂停

### 受影响文件

| 文件 | 影响 | 改动量 |
|------|------|--------|
| `internal/orchestrator/service/statemachine.go` | 新增 `handleControlNode()` 方法 | ~30 行 |
| `internal/orchestrator/service/state.go` | `TransitionNode` 到 READY 后检查 NodeType | ~10 行 |
| `internal/orchestrator/service/state.go` | CONTROL 节点 SKIPPED 也算依赖满足 | 已有逻辑，验证即可 |

### 不改动

- `internal/model/model.go` — NodeType CONTROL 已定义
- `internal/orchestrator/service/orchestrator.go` — DAG 提交不变
- `internal/orchestrator/service/dependency_checker.go` — 依赖检查不变
- `internal/worker/` — Worker 不处理 CONTROL 节点

### 状态转换

```
CONTROL node CREATED
  → READY (dependencies met, 同其他节点)
    → Task 自动暂停 (新增行为)
      → 等待外部 API 调用
        → POST /api/node/:nodeId/success → SUCCESS → Task 恢复
        → POST /api/node/:nodeId/failure → FAILED → 触发重试逻辑
```

### 并发与事务

- `TransitionNode` 和 `TransitionTask` 已使用 pgx 事务，CONTROL 暂停逻辑复用同一事务
- 竞态条件: 用户快速连续调用 approve+reject，由 Version 乐观锁防护
- 幂等: TransitionNode 对相同状态幂等

### API 变化

- 无新 API 端点。复用现有:
  - `POST /api/node/:nodeId/success` — 审核通过
  - `POST /api/node/:nodeId/failure` — 审核驳回（附带 errorMessage）
  - `POST /api/node/:nodeId/retry` — 驳回后重试

### 向后兼容

- 不影响 TOOL/LLM/LOG 节点
- 不影响已有 DAG 行为
- CONTROL 节点不存在于任何现有 DAG 中，零风险

## CORE-002: 进度事件

### 受影响文件

| 文件 | 影响 | 改动量 |
|------|------|--------|
| `internal/eventbus/eventbus.go` | 新增 `TopicProgress` 常量 | ~2 行 |
| `internal/model/model.go` | 新增 `ProgressEvent` 结构体 | ~10 行 |

### 不改动

- `internal/outbox/` — 复用现有 SaveEvent → Relay → Kafka 通路
- `internal/orchestrator/` — 进度事件由业务模块发布，非编排层

### 事件结构

```go
type ProgressEvent struct {
    ProjectID   string  `json:"project_id"`
    Stage       string  `json:"stage"`
    Progress    float64 `json:"progress"`
    CurrentStep string  `json:"current_step"`
    TotalSteps  int     `json:"total_steps"`
    Timestamp   string  `json:"timestamp"`
}
```

Topic: `ai.node.progress` (通用，不仅限于 bid)

### 向后兼容

- 新增 topic，无消费者时不产生副作用
- 不影响已有 event 结构

---

## 新增 `internal/bid/` 模块

### 遵循现有模式

参考 `internal/publish/` 和 `internal/skill/` 的模块结构:

```
internal/bid/
  handler/
    handler.go       # Gin HTTP handler (17 个端点)
  service/
    service.go       # 业务逻辑
    workflow.go      # DAG 构建和提交
  repository/
    repository.go    # bid_projects, bid_chapters 数据访问
```

### 依赖关系

```
bid/handler → bid/service → model/repository (新增接口)
                          → orchestrator/service (Task 创建、DAG 提交)
                          → outbox (进度事件发布)
                          → worker/tool (外部工具调用)
```

### 数据库影响

- 3 张新表 (`bid_projects`, `bid_chapters`, `bid_templates`)
- 在 `database.RunMigrations()` 追加
- 不影响现有表

### 注册到 main.go

```go
bidRepo := repository.NewBidRepository(dbPool)
bidSvc := bidService.NewBidService(bidRepo, orchestratorService, eventSaver, toolRegistry)
bidHandler := bidHandler.NewBidHandler(bidSvc)
bidHandler.RegisterRoutes(r)
```

### 与现有模块的交互

| 交互 | 方式 |
|------|------|
| 创建 Task | `orchestratorService.CreateTask()` |
| 提交 DAG | `orchestratorService.SubmitDAG()` |
| 查询状态 | `GET /api/task/:taskId` (通过 orchestrator handler) |
| 工具调用 | 通过 DAG 节点，Worker 自动路由 |
| 暂停/恢复 | `taskExecutionControl.PauseTask()` / `ResumeTask()` |
| 发布进度 | `eventSaver.SaveEvent()` → outbox → Kafka |
| 文件上传 | `media.StorageService` (MinIO) |

## 风险矩阵

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| CONTROL 节点与现有依赖检查冲突 | 低 | 中 | SKIPPED 状态已处理，CONTROL 同理 |
| 并发 APPROVE/REJECT 导致状态不一致 | 低 | 中 | Version 乐观锁 + 状态幂等 |
| 进度事件消费端不存在导致 Kafka 堆积 | 低 | 低 | 独立 topic，不影响核心事件流 |
| bid 模块引入循环依赖 | 低 | 高 | 单向依赖: bid → orchestrator，不存在反向 |

## 总改动量估算

| 类别 | 文件数 | 预计行数 |
|------|--------|----------|
| Core 修改 (CONTROL) | 1-2 | ~40 |
| Core 修改 (Progress) | 2 | ~15 |
| bid 模块新增 | 3-5 | ~600 |
| 数据库迁移 | 1 (追加) | ~40 |
| main.go 注册 | 1 | ~15 |
| **总计** | **8-10** | **~710** |
