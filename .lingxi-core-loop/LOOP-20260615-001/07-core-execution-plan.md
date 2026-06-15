# 07-Core-Execution-Plan: 执行计划

## 任务分解

### TASK-CORE-001: CONTROL 节点自动暂停
- **目标**: CONTROL 节点 READY 时 Task 自动暂停
- **修改范围**: `internal/orchestrator/service/state.go`
- **禁止修改**: Worker 层、Kafka 消费、现有节点类型逻辑
- **依赖**: 无
- **DoD**: CONTROL 节点 READY → Task PAUSED，其他类型不变

### TASK-CORE-002: 进度事件支持
- **目标**: 新增 TopicProgress 和 ProgressEvent
- **修改范围**: `internal/eventbus/eventbus.go`, `internal/model/model.go`
- **禁止修改**: 现有 Event 结构
- **依赖**: 无
- **DoD**: 新 topic 和 event 定义可通过编译

### TASK-BID-001: 数据库迁移
- **目标**: 创建 bid_projects, bid_chapters, bid_templates 表
- **修改范围**: `internal/database/database.go`
- **依赖**: 无
- **DoD**: 表创建成功，go build 通过

### TASK-BID-002: Repository 层
- **目标**: BidProject, BidChapter, BidTemplate 的 CRUD 实现
- **修改范围**: `internal/model/repository/interfaces.go`, 新建 `internal/bid/repository/`
- **依赖**: TASK-BID-001
- **DoD**: 所有 CRUD 方法可通过单元测试

### TASK-BID-003: Service 层 + Workflow
- **目标**: BidService 核心业务逻辑 + DAG 构建
- **修改范围**: 新建 `internal/bid/service/`
- **依赖**: TASK-BID-002
- **DoD**: StartGeneration 可创建 Task 并提交 DAG

### TASK-BID-004: Handler 层
- **目标**: 17 个 API 端点
- **修改范围**: 新建 `internal/bid/handler/`
- **依赖**: TASK-BID-003
- **DoD**: 所有端点响应正确

### TASK-BID-005: main.go 注册
- **目标**: 将 bid 模块注册到 HTTP 路由
- **修改范围**: `cmd/tangying-ai-os/main.go`
- **依赖**: TASK-BID-004
- **DoD**: `/api/bid/*` 路由可访问

## 执行顺序

```
TASK-CORE-001 ──┐
                 ├──> TASK-BID-001 → TASK-BID-002 → TASK-BID-003 → TASK-BID-004 → TASK-BID-005
TASK-CORE-002 ──┘
```

## 回滚方案

- CONTROL 节点: 删除 state.go 中新增的判断块即可
- 进度事件: 无消费者时无副作用
- bid 模块: 删除整个 `internal/bid/` 目录 + 删除 main.go 注册行 + 删除数据库迁移

## Definition of Done

```yaml
- [ ] go build 通过
- [ ] gofmt 无 diff
- [ ] go vet 无警告
- [ ] go test ./... 通过 (至少新增模块)
- [ ] CONTROL 节点单元测试: READY → Task PAUSED
- [ ] CONTROL 节点单元测试: TOOL 节点 READY → Task 不暂停
- [ ] bid handler 集成测试: 创建项目 → 上传文件 → 启动 → 查询进度
- [ ] /api/health 仍正常
- [ ] 现有 API 端点行为不变
```
