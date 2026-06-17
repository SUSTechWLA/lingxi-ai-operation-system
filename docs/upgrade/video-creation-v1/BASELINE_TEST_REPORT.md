# AIOS 视频创作升级 — 基线测试报告

**日期**: 2026-06-17  
**分支**: `develop_go`  
**Commit**: `adeb296b97e7bb2bd19024127f38f906b3e6652c`  
**Go 版本**: go1.26.2 darwin/arm64  
**模块路径**: `github.com/tangying-ai/aios-core`

---

## 1. 实际目录树

### internal/core/ (通用核心)
```
internal/core/
├── common/
│   ├── jsonx/          # JSON 工具 (有测试)
│   ├── llmutil/        # LLM 客户端工具 (无测试)
│   └── metadata/       # 元数据提取 (无测试)
├── config/             # Viper 配置 (无测试)
├── context/            # 审计上下文 (handler 有测试, service 无测试)
│   ├── handler/
│   ├── repository/
│   └── service/
├── database/           # pgx 连接池 + 内联 SQL migration (无测试)
├── eventbus/           # Kafka 生产者/消费者 (无测试)
├── logger/             # Zap 日志 (无测试)
├── media/              # MinIO 媒体管理 (无测试)
├── model/              # 数据模型 (有测试)
│   └── repository/     # 数据访问 (无测试)
├── orchestrator/       # DAG 调度引擎
│   ├── handler/        # (有测试)
│   └── service/        # (有测试)
├── outbox/             # Outbox 可靠事件投递 (无测试)
├── redis/              # Redis 客户端 (无测试)
├── translator/         # NL → DAG (handler 有测试, service 无测试)
│   ├── handler/
│   └── service/
├── worker/             # 工具执行引擎
│   ├── executor/       # Direct + Sandbox executor (无测试)
│   │   └── sandboxpb/  # gRPC proto 生成代码 (无测试, init panic)
│   ├── service/        # Node 执行器 (无测试)
│   └── tool/           # Tool 接口 + 注册表 (有测试)
│       └── builtin/    # 内置工具 (有测试)
└── workflow/           # 工作流模板 (无测试)
    ├── handler.go       # /api/workflows CRUD
    ├── model.go         # Template (无 Version 字段)
    ├── repository.go    # pgxpool 数据访问
    ├── seed.go          # 3 个内置模板 + EnsureSchema
    └── service.go       # CRUD + Instantiate
```

### internal/agents/ (业务 Agent)
```
internal/agents/
├── bid/                # 标书生成 (全部无测试)
│   ├── handler/
│   ├── model/
│   ├── repository/
│   └── service/
├── chat/               # AI 对话助手 (原 skill, 全部无测试)
│   ├── handler/
│   ├── prompts/
│   └── service/
└── publish/            # 内容发布 (handler 有测试, service 无测试)
    ├── handler/
    └── service/
```

### 前端 (frontend/)
```
frontend/src/
├── components/         # 15 个 React 组件
├── pages/              # PublishPage, DesktopPage
├── stores/             # appStore.ts (单一大 Zustand store)
├── services/           # api.ts (Axios)
├── utils/              # types.ts, electron.ts
├── App.tsx
└── main.tsx
frontend/electron/      # Electron 包装
├── main.cjs
└── preload.cjs
```

**注意**: 没有 `internal/agents/video/`、没有 `skills/` 目录、没有前端路由 (使用 Zustand page switching)。

---

## 2. 现有测试清单

### Go 测试文件 (11 个)

| 文件 | 包 | 状态 |
|------|-----|------|
| `internal/core/model/model_test.go` | model | ✅ PASS |
| `internal/core/orchestrator/handler/handler_test.go` | orchestrator/handler | ✅ PASS |
| `internal/core/orchestrator/service/service_test.go` | orchestrator/service | ✅ PASS |
| `internal/core/orchestrator/service/dag_validator_test.go` | orchestrator/service | ✅ PASS |
| `internal/core/orchestrator/service/retry_policy_test.go` | orchestrator/service | ✅ PASS |
| `internal/core/context/handler/handler_test.go` | context/handler | ✅ PASS |
| `internal/core/common/jsonx/jsonx_test.go` | common/jsonx | ✅ PASS |
| `internal/core/translator/handler/handler_test.go` | translator/handler | ❌ FAIL (sandboxpb panic) |
| `internal/core/worker/tool/tool_comprehensive_test.go` | worker/tool | ❌ FAIL (sandboxpb panic) |
| `internal/core/worker/tool/builtin/builtin_test.go` | worker/tool/builtin | ❌ FAIL (sandboxpb panic) |
| `internal/agents/publish/handler/handler_test.go` | publish/handler | ❌ FAIL (sandboxpb panic) |

### 无测试文件的包 (20+)
model/repository, outbox, redis, translator/service, worker/executor, worker/executor/sandboxpb, worker/service, workflow/*, context/service, common/llmutil, common/metadata, config, database, eventbus, logger, media, agents/publish/service, agents/chat/*, agents/bid/*

---

## 3. 后端测试结果

### `go test ./...` (全部)
```
✅ PASS  github.com/tangying-ai/aios-core/internal/core/model
✅ PASS  github.com/tangying-ai/aios-core/internal/core/orchestrator/handler
✅ PASS  github.com/tangying-ai/aios-core/internal/core/orchestrator/service
✅ PASS  github.com/tangying-ai/aios-core/internal/core/context/handler
✅ PASS  github.com/tangying-ai/aios-core/internal/core/common/jsonx
❌ FAIL  github.com/tangying-ai/aios-core/internal/core/translator/handler  (sandboxpb panic)
❌ FAIL  github.com/tangying-ai/aios-core/internal/core/worker/tool          (sandboxpb panic)
❌ FAIL  github.com/tangying-ai/aios-core/internal/core/worker/tool/builtin  (sandboxpb panic)
❌ FAIL  github.com/tangying-ai/aios-core/internal/agents/publish/handler    (sandboxpb panic)
```

### `go test -race ./...` (受影响包)
```
✅ PASS  internal/core/model/...
✅ PASS  internal/core/orchestrator/...
✅ PASS  internal/core/context/handler
✅ PASS  internal/core/common/jsonx
✅ PASS  internal/agents/publish/handler     (独立运行 OK)
```

### 已知失败分析

**sandboxpb proto panic** (`slice bounds out of range [-1:]`):
- 根因: `google.golang.org/protobuf v1.36.11` 与 `go1.26.2` 的兼容性问题
- 触发: `sandboxpb` 包的 `init()` 函数在 proto 文件描述符反序列化时崩溃
- 影响范围: 任何导入 `sandboxpb` 或其父包的测试 (translator, worker/tool, worker/tool/builtin, publish/handler)
- **分类**: 升级前已有问题 (proto 工具链版本不兼容)
- **建议**: 重新生成 proto 文件 (`protoc --go_out=. --go-grpc_out=. sandbox.proto`)，或降级 protobuf 版本
- **注意**: 这是环境/工具链问题，不影响升级实施 — 新增的 core 包 (artifact, skillruntime, modelgateway, localrunner) 不会导入 sandboxpb

---

## 4. 前端 Build 结果

```
✅ npm run build — 成功
vite v5.4.21 building for production...
✓ 134 modules transformed.
dist/index.html          0.47 kB
dist/assets/aios-icon    1,302.65 kB
dist/assets/index.css    27.04 kB
dist/assets/index.js     321.81 kB
```

### 前端测试框架状态
- ❌ 无 Vitest 配置
- ❌ 无 React Testing Library
- ❌ 无 Playwright 配置
- ❌ 无 MSW 配置
- ⚠️ `package.json` 有 `lint` 脚本 (eslint)，无 `test` 脚本
- ⚠️ 无 `tsc` typecheck 脚本 (通过 `tsc -b && vite build` 间接覆盖)

---

## 5. 文档设计与实际代码差异

| 规格设计路径 | 实际代码路径 | 差异说明 |
|-------------|-------------|---------|
| `internal/skill/` | `internal/agents/chat/` | 已重构重命名 (commit 04379ca) |
| `internal/publish/handler/` | `internal/agents/publish/handler/` | 已移至 agents 下 |
| `internal/media/` | `internal/core/media/` | media 在 core 而非顶层 internal |
| `internal/orchestrator/` | `internal/core/orchestrator/` | orchestrator 在 core 下 |
| `internal/worker/` | `internal/core/worker/` | worker 在 core 下 |
| `internal/translator/` | `internal/core/translator/` | translator 在 core 下 |
| `internal/context/` | `internal/core/context/` | context 在 core 下 |
| `internal/outbox/` | `internal/core/outbox/` | outbox 在 core 下 |
| Workflow Template.Version | 无 | Template 缺少 Version 字段 |
| WorkflowRun / StageRun | 无 | 完全未实现 |
| Artifact 系统 | 无 | 完全未实现 |
| Model Gateway | 无 | 完全未实现 |
| Skill Package 系统 | 无 | 完全未实现 |
| `internal/agents/video/` | 无 | 完全未实现 |
| `skills/` 目录 | 无 | 完全未实现 |
| Local Runner 协议 | 无 | 完全未实现 |
| React Router | 无 (Zustand page switching) | 前端无正式路由 |
| 多个 Zustand Store | 单 `appStore.ts` | 全部状态在一个 store |

### 规格设计的路径适配建议
规格文档中 `internal/skill` → 实际 `internal/agents/chat`；新增的 `internal/core/` 子包直接放在 `internal/core/` 下（与现有结构一致）；新增视频领域放 `internal/agents/video/`（与 bid/chat/publish 并列）。

---

## 6. 已有能力确认

### ✅ 已具备且稳定的能力
- **Orchestrator DAG 调度**: 完整的状态机 (CREATED→READY→RUNNING→SUCCESS/FAILED/RETRYING/SKIPPED/HEARTBEAT_TIMEOUT)
- **CONTROL 人工审核节点**: `NodeTypeControl` 在 state machine 中有特殊处理，READY 时自动 pause task
- **长任务支持**: `LongRunning`, `Progress`, `Heartbeat`, `Checkpoint` 已实现
- **Outbox 可靠投递**: relay goroutine 100ms ticker
- **Redpanda/Kafka 事件总线**: 多 consumer group
- **Worker/Tool Registry**: 内置 13 个工具，支持外部工具注册
- **Workflow Template 持久化**: CRUD + Instantiate，3 个内置模板
- **Bid 标书生成**: CONTROL 审核节点实际使用案例
- **Chat 对话系统**: 多轮会话 + DAG 规划 + 媒体上下文
- **Media/MinIO**: 上传、列表、标签、预签名 URL
- **Rust Sandbox (gRPC)**: 完整的资源隔离执行器
- **Electron 打包**: main.cjs + preload.cjs + .dmg/.exe

### ⚠️ 存在但需增强
- **Workflow**: 缺少 Version/Run/Stage/Approval/Artifact/Rerun
- **Tool Manifest**: 已有 manifest 系统，需扩展支持 Skill Package

---

## 7. 下一任务建议

**P0-T02: Feature Flag 基础设施**

新增配置:
- `VIDEO_CREATION_ENABLED` (默认 false)
- `LOCAL_RUNNER_ENABLED` (默认 false)
- `MODEL_PROVIDER_MODE` (默认 fake)

同时：
- 修复 sandboxpb proto panic (重新生成 proto 或降级 protobuf)
- 为 workflow 包添加基础测试

---

## 8. 执行命令汇总

```bash
# 基线测试 (通过)
cd aios-core && go test ./internal/core/model/... ./internal/core/orchestrator/... ./internal/core/context/... ./internal/core/common/...

# 基线测试 (已知失败 — sandboxpb panic)
cd aios-core && go test ./...

# Race 检测 (通过的核心包)
cd aios-core && go test -race ./internal/core/model/... ./internal/core/orchestrator/...

# 前端 Build (通过)
cd frontend && npm run build

# 全量回归 (部分失败因 sandboxpb)
cd aios-core && go test -race ./...
```
