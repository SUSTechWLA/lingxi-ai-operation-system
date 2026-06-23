# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **权威架构文档：** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — 完整的模块说明（含 v3.2 动态 Agent Runtime）、数据模型、API 清单、前端架构、基础设施。
> 本文档为快速上手指南，详细内容请查阅架构文档。

> **v3.2 核心新增：** Dynamic Agent Runtime — `LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`，从自然语言一步生成可执行 DAG，含质量门禁体系和 Artifact Review 闭环。

## 系统概览

**躺营 AIOS** — 「本地执行面 + 云端控制面」的自媒体内容运营系统。

```text
frontend/       # React + Electron UI（桌面端 & Web）
local-backend/  # 本地轻量 agent（:18080），无数据库、无 Docker，管理用户本地文件/缓存/日志/产物
cloud-backend/  # 云端 AIOS Core（:8080），Go 单体，PostgreSQL/Redis/Redpanda/MinIO
hyperframes-render-service/  # HyperFrames 渲染服务（:8787），Node.js/TypeScript，无 CLI 依赖
```

### 核心边界

| 层 | 职责 | 不负责 |
|----|------|--------|
| **local-backend** | 本地文件读写、缓存、项目文件、产物文件、执行日志、诊断包生成 | 数据库、LLM API Key、云端业务数据 |
| **cloud-backend** | DAG 编排、LLM/API 对接、配置、工作流模板、远程编排、云端日志分析 | 保存用户生成的脚本/图片/音频/视频正文 |

### 产物存储边界（v3.0 关键变更）

- **用户生成内容**（脚本、JSON、图片、音频、视频）→ 保存在用户本机 `local-backend` 的 `artifacts/` 目录
- **云端 artifact 表** → 仅保存 `storage_type=local`、`storage_ref=local://...`、hash、size、version、provider/model 等索引元数据
- **云端不保存**用户产物正文（`inline_json` 为空），不把用户图片/音频/视频默认上传 MinIO

## 快速启动

### 本地桌面开发（无需 Docker）

```bash
# 终端 1：本地 agent
bash scripts/start-local-backend.sh     # 监听 127.0.0.1:18080

# 终端 2：前端 dev server
bash scripts/start-frontend.sh          # http://localhost:3000
```

本地启动不依赖 PostgreSQL、Redis、Kafka、MinIO 或 Docker。需要云端能力时通过 `VITE_CLOUD_API_BASE` 指向云端 API。

### 云端后端开发（需要 Docker）

```bash
cd cloud-backend
cp .env.example .env                    # 填 OPENAI_API_KEY
docker compose up -d                    # 启动 PG/Redis/Redpanda/MinIO
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os                  # 监听 :8080
```

### 启用视频创作

```env
# cloud-backend/.env
VIDEO_CREATION_ENABLED=true
MODEL_PROVIDER_MODE=fake               # 测试用 fake，生产用 real
SKILL_ROOT=skills
```

## 目录结构

```text
cloud-backend/
  cmd/tangying-ai-os/main.go           # 入口，路由注册，优雅关闭
  internal/
    core/                               # 通用引擎层（不绑定业务）
      agentruntime/                     # 🆕 动态 Agent Runtime（Planner→Guard→Compiler→DAG）
      orchestrator/                     # DAG 调度引擎
      workflow/                         # 工作流模板 + Run + Skill→DAG 编译器
      worker/                           # 工具执行引擎（14+ 内置工具 + 沙箱）
      skillruntime/                     # Skill 包加载 + LLM Router
      modelgateway/                     # 统一模型网关（指纹缓存 + 重试 + Provider 路由）
      artifact/                         # 版本化产物管理（v3.0: metadata-only，内容在本地）
      localrunner/                      # Electron 本地任务协议
      translator/                       # NL→DAG 翻译
      context/                          # 审计追踪
      media/                            # MinIO 媒体管理（云端服务资产/legacy 兼容）
      apispec/                          # OpenAPI 3.0 自动生成 + Swagger UI + 前端类型代码生成
      health/                           # 就绪健康检查（DB/Redis/Kafka 依赖探测）
      redis/                            # Redis 客户端封装
      outbox/ eventbus/ config/ database/ logger/ model/
      hyperframes/                       # HyperFrames Render Service HTTP 客户端（替代 CLI）
    agents/                             # 业务 Agent 层
      video/                            # 视频创作（Project/Run/审核/产物）
      bid/                              # 标书生成
      chat/                             # AI 对话助手
      publish/                          # 内容发布 + AI 生成
  skills/                               # 6 个 Skill Package（create-opinion-videos 等）
  deploy/                               # 云端 Docker Compose + nginx 配置
  scripts/                              # 启动/测试脚本

local-backend/
  cmd/local-agent/                      # 本地 agent 入口
  internal/localagent/                  # HTTP server（:18080），本地文件管理 + 产物存储 API

frontend/
  src/
    components/                         # Sidebar, CreatorWorkbenchPage 内联组件 等
    pages/                              # CreatorWorkbenchPage（创作台）, DesktopPage
    services/api.ts                     # axios，云端 API 调用
    stores/                             # Zustand（appStore，发布相关；创作台自管理 state）
  electron/                             # Electron 壳（main.cjs, preload.cjs）
```

## 常用命令

```bash
# 云端后端
cd cloud-backend
make build                    # 编译
make run                      # 编译 + 运行
make test                     # 跑测试
make docker-up                # 启动 PG/Redis/Redpanda/MinIO
make docker-down              # 停止基础设施
make gen-docs                 # 生成 OpenAPI 文档 + 前端 TypeScript 类型
make lint                     # golangci-lint 检查
make fmt                      # gofmt + goimports 格式化
go test -race ./...           # 全量 + 竞态检测
./scripts/test-apis.sh        # API 集成测试

# 本地后端
cd local-backend
go test ./...                 # 跑测试
go build -o build/tangying-local-agent cmd/local-agent/main.go

# 前端
cd frontend
npm run dev                   # Dev server（:3000，proxy /api → :8080）
npm run build                 # 生产构建
npm run electron:build        # Electron .dmg/.exe 打包

# 桌面完整构建
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh

# 沙箱（可选，Rust）
cd cloud-backend && make sandbox-build
```

## 本地 Agent API

```text
GET  /api/local/health                          # 健康检查
GET  /api/local/paths                           # 数据目录路径
POST /api/local/artifacts                       # 保存本地产物（content 或 contentBase64）
GET  /api/local/artifacts/:id?projectId=<pid>   # 读取本地产物
POST /api/local/logs                            # 写本地日志
POST /api/local/diagnostics                     # 生成诊断包
```

产物存储结构：`<DataDir>/artifacts/<projectId>/<artifactId>/content` + `metadata.json`

## 动态 Agent API（v3.2 新增）

```text
POST /api/agent/runs                                    # 🆕 启动 dynamic agent run（消息 → AgentPlan → DAG → 执行）
GET  /api/agent/runs/:runId                             # 🆕 查询 run 状态和 plan
GET  /api/agent/runs/:runId/trace                       # 🆕 获取 DAG 执行追踪
GET  /api/agent/runs/:runId/reviews                     # 🆕 列出待审核节点（CONTROL + quality_gate）
POST /api/agent/runs/:runId/reviews/:reviewId/approve   # 🆕 审核通过
POST /api/agent/runs/:runId/reviews/:reviewId/reject    # 🆕 审核驳回
```

## 关键架构概念

- **DAG 工作流引擎** — 任务有向无环图，节点=操作（LLM/工具/审核/质量门禁），边=依赖，支持条件分支、重试、暂停/恢复
- **动态 Agent Runtime（v3.2）** — `LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`：用户一句话 → LLM 自动规划步骤 → Guard 校验（参数类型/引用合法性/output schema 字段）→ Compiler 自动插入审核节点和质量门禁 → 一次性 DAG 提交执行。不依赖固定 workflow_template。
- **Skill Package** — 业务流程写成 `skill.yaml` + stages Markdown，启动时自动编译为 DAG → Workflow 模板
- **LLM Skill Router** — 用户一句话自动选择最合适的 Skill + 推断画幅/时长/交付目标
- **质量门禁体系** — 关键工具自动插入 quality checker → quality gate CONTROL 节点：score≥85 自动通过，70-84 支持自动修复，<70 暂停人工确认
- **HybridToolRetriever** — 多信号评分从工具库检索 TopK 候选工具供给 LLMPlanner，扣成本/风险惩罚
- **HyperFrames Render Service** — 独立 Node.js 渲染服务（:8787），通过 `@hyperframes/producer` 程序化渲染 MP4，不再依赖 `npx hyperframes` CLI。Go 端通过 HTTP Client 调用
- **Outbox 可靠投递** — 事件先写 DB 再异步 relay 到 Kafka，基础设施抖动时不丢事件
- **事件驱动** — Kafka topics: `ai.node.ready` → `ai.node.result`（含 executed/failed）→ 状态机驱动
- **版本化产物** — 每个 stage 产物有 version/contentHash/promptHash，支持返工闭环
- **沙箱隔离** — Rust gRPC 沙箱执行不受信任代码（setrlimit 资源限制）
- **就绪健康检查** — `GET /api/health/ready` 探测 PostgreSQL/Redis/Kafka 依赖状态
- **OpenAPI 自动文档** — `/docs` Swagger UI + `/api/openapi.json`，由代码路由注册自动生成

## Feature Flags

```env
VIDEO_CREATION_ENABLED=false   # 视频创作功能（路由 + API）
LOCAL_RUNNER_ENABLED=false     # Electron 本地 Runner
MODEL_PROVIDER_MODE=fake       # fake | real
SKILL_ROOT=skills              # Skill Package 根目录

# HyperFrames Render Service（替代 CLI）
HYPERFRAMES_MODE=service            # disabled | service
HYPERFRAMES_SERVICE_URL=http://127.0.0.1:8787
HYPERFRAMES_TIMEOUT_SEC=1800
HYPERFRAMES_DEFAULT_FPS=30
HYPERFRAMES_DEFAULT_QUALITY=standard
HYPERFRAMES_DEFAULT_FORMAT=mp4
```

## 测试

```bash
# 云端
cd cloud-backend && go test ./...

# 本地
cd local-backend && go test ./...

# 前端
cd frontend && npm run build
```

## 新增业务线步骤

1. 在 `skills/{name}/1.0.0/` 写 `skill.yaml` + `stages/*.md`
2. （可选）在 `internal/agents/{name}/` 写领域 Agent，复用 orchestrator/workflow
3. 在 `main.go` 注册 handler（feature-gated）
4. 启动时自动加载 Skill → 编译 → 注册为 Workflow 模板

## 常见问题

- **NL-Translator 503**：OPENAI_API_KEY 缺失或无效
- **端口冲突**：确保 8080（云端）、18080（本地）、3000（前端）未被占用
- **Docker 未运行**：仅云端开发需要，本地开发不需要
- **Go 版本**：cloud-backend 需要 Go 1.25+，local-backend 需要 Go 1.23+
