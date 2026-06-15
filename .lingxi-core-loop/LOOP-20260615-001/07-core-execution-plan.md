# 07-Core-Execution-Plan: 实现计划

## 任务分解

### TASK-CORE-001: Node 模型 + 状态扩展
**目标**: 扩展数据模型支持长任务字段

**文件**:
- `internal/core/model/model.go` — 增加 Node 字段、NodeHeartbeatTimeout 状态、扩展 NodeRequest/DAGRequest

**禁止修改**: Task/Action/ConversationContext 等不相关类型

**DoD**: 
- `go build` 通过
- 现有测试通过

---

### TASK-CORE-002: 数据库 Schema 迁移
**目标**: 添加新列和索引

**文件**:
- `internal/core/database/database.go` — 迁移 SQL

**DoD**: 
- 新部署的表有完整列
- 已有部署的迁移不影响数据

---

### TASK-CORE-003: Repository 接口 + 实现
**目标**: 新增 NodeRepo 方法和实现

**文件**:
- `internal/core/model/repository/interfaces.go` — 接口
- `internal/core/model/repository/repository.go` — 实现

**新增方法**: `FindStaleRunningNodes(ctx, timeout)`, 扩展 `Save` 处理新字段

**DoD**: 
- 新方法正确查询/写入
- `go test ./internal/core/model/repository/...` 通过

---

### TASK-CORE-004: ProgressReporter Tool 接口
**目标**: 定义可选的进度报告接口

**文件**:
- `internal/core/worker/tool/tool.go` — 新增 ProgressReporter、ProgressUpdate 类型

**DoD**: `go build` 通过（没有编译错误即接口正确）

---

### TASK-CORE-005: NodeExecutor 长任务支持
**目标**: 扩展执行器支持心跳和进度回调

**文件**:
- `internal/core/worker/service/executor.go` — heartbeat 协程、进度回调链、checkpoint 注入

**DoD**: 
- 短任务行为不变
- 长任务启动心跳
- Tool 回调能发送进度事件

---

### TASK-CORE-006: StateMachine 扩展
**目标**: 新增 HEARTBEAT_TIMEOUT 处理 + checkpoint 恢复逻辑

**文件**:
- `internal/core/orchestrator/service/statemachine.go` — OnHeartbeatTimeout 方法

**DoD**: 
- HEARTBEAT_TIMEOUT 节点能正确重试
- 重试时 ToolContext 包含 checkpoint

---

### TASK-CORE-007: Scheduler 心跳检测
**目标**: 扩展 Scheduler 检测心跳超时

**文件**:
- `internal/core/orchestrator/service/scheduler.go` — 增加 detectHeartbeatTimeout

**DoD**: 
- 定时扫描 long_running + RUNNING 节点
- 超时节点转为 HEARTBEAT_TIMEOUT

---

### TASK-CORE-008: Progress 事件消费者
**目标**: 消费 ai.node.progress 事件，更新 DB + Redis

**实现方式**: 在 main.go 中注册新的 Consumer

**DoD**: 
- 进度事件正确持久化
- Redis 缓存更新
- 短 TTL 缓存策略

---

### TASK-CORE-009: 进度查询 API
**目标**: 提供任务和节点进度查询

**文件**:
- `internal/core/orchestrator/handler/handler.go` — 新端点
- `internal/core/orchestrator/service/orchestrator.go` — 进度聚合逻辑

**新增端点**:
- `GET /api/task/:taskId/progress`

**DoD**: 
- API 返回正确的聚合进度
- 响应格式符合规范

---

### TASK-CORE-010: 集成测试 + 回归验证
**目标**: 编写测试并确保所有回归通过

**DoD**:
- `go fmt ./...` 通过
- `go vet ./...` 通过
- `go test ./... -count=1` 全部通过
- `go test -race ./... -count=1` 通过
- 新功能有至少 3 个测试场景

## 执行顺序

```
TASK-001 (Model) 
  → TASK-002 (DB) 
    → TASK-003 (Repository) 
      → TASK-004 (Tool Interface)
        → TASK-005 (NodeExecutor)  ─┐
        → TASK-006 (StateMachine) ──┤ (可并行)
        → TASK-007 (Scheduler)   ──┤
          → TASK-008 (Consumer)  ──┘
            → TASK-009 (API)
              → TASK-010 (Tests)
```

## 风险控制

- 每个任务独立可测试
- 每步完成后跑 `go build` + 相关测试
- 不跨步骤修改文件
