# 躺营 AIOS 产品架构设计说明文档

> 版本：v3.2（动态 Agent Runtime + 质量门禁体系）
> 最后更新：2026-06-24
> 适用对象：新加入的后端 / 前端 / 部署工程师
> 配套文档：[README.md](../README.md)、[AGENTS.md](../AGENTS.md)、[docs/upgrade/video-creation-v1/](upgrade/video-creation-v1/)

---

## 目录

1. [产品概述与核心优势](#1-产品概述与核心优势)
2. [整体软件架构](#2-整体软件架构)
3. [技术栈与依赖](#3-技术栈与依赖)
4. [后端模块详解（internal/core）](#4-后端模块详解internalcore)
5. [业务 Agent 详解（internal/agents）](#5-业务-agent-详解internalagents)
6. [Skill Package 系统](#6-skill-package-系统)
7. [数据模型与数据库表](#7-数据模型与数据库表)
8. [完整 API 接口清单](#8-完整-api-接口清单)
9. [前端架构（React + Electron）](#9-前端架构react--electron)
10. [基础设施与事件机制](#10-基础设施与事件机制)
11. [安装、运行与云端部署](#11-安装运行与云端部署)
12. [开发工作流与测试](#12-开发工作流与测试)
13. [改进与优化建议](#13-改进与优化建议)

---

## 1. 产品概述与核心优势

### 1.1 一句话定位

**躺营 AIOS（AI Operation System）** 是一套面向自媒体内容运营的「本地执行面 + 云端控制面」系统：本地桌面端负责用户电脑上的文件、缓存、日志、诊断包和用户自配的基础模型 Provider；云端负责远程配置、编排、外部服务元数据、账号体系和云端日志分析。

当前代码仓按运行边界拆为：

```text
frontend/                    # React + Electron UI
local-backend/               # 本地轻量执行器，无数据库、无 Docker
cloud-backend/               # 云端 AIOS Core，包含原 Go 编排平台和云端部署
hyperframes-render-service/  # HyperFrames 渲染服务（Node.js/TypeScript），无 CLI 依赖
```

### 1.2 它解决什么问题

自媒体内容生产链条长、人工节点多（写脚本、出图、剪视频、改文案、平台适配），传统做法是把不同 AI 工具手动串起来。AIOS 的目标是：

- **一句话描述成片目标**，系统自动理解需求、选择最合适的「技能（Skill）」、编排成可执行的工作流；
- **全程可追踪、可审核、可返工**，每个中间产物都有版本，能回溯能改；
- **工具可插拔**，内置 30+ 工具（含 17 个通用内置工具 + 33 个视频创作工具 + Skill Capability 动态注册工具），外部工具通过 HTTP 注册即可接入；
- **同一套引擎服务多条业务线**（视频、发布、标书、对话），不重复造轮子。

### 1.3 核心优势

| 优势 | 说明 |
|------|------|
| **DAG 工作流引擎** | 任务=有向无环图，节点=操作（LLM/工具/审核），边=依赖。支持条件分支、重试、暂停/恢复、阶段审核（CONTROL 节点）。 |
| **动态 Agent Runtime** | `LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`：用户一句话，LLM 自动选择工具、规划步骤、Guard 校验（参数类型/引用合法性/风险等级）、Compiler 自动插入审核节点和质量门禁，生成一次性 DAG 提交执行。不依赖固定 workflow_template。 |
| **Skill Package 热加载** | 业务流程写成 `skill.yaml` + stages Markdown，启动时自动加载、校验、编译为 DAG 并注册为 Workflow 模板。新增业务线不改引擎代码。 |
| **Skill→Workflow 自动转换** | `approval_required` 自动生成「执行节点 + 审核节点」双链；`optional` 自动生成 skip 旁路。开发者只写业务语义。 |
| **自然语言入口路由** | 用户一句话，LLM Router 在可见的 Skill 目录里选出最合适的一个，并推断画幅/时长/交付目标，无需手动选「视频类型」。 |
| **统一 Model Gateway** | 所有模型调用走指纹缓存（幂等）+ 指数退避重试 + Provider 路由，Fake Provider 让无 API Key 也能跑通端到端测试。LLMPlanner、PromptTool、QualityChecker 统一走 ModelGateway。 |
| **质量门禁体系** | 关键生产工具（口播稿/分镜/视频Prompt）自动插入质量检查器，输出 `passed/score/issues/repairSuggestions`。质量门 CONTROL 节点：score≥85 自动通过，70-84 支持自动修复，<70 暂停人工确认。 |
| **VideoForge Studio Pipeline** | 视频创作可先走 Pipeline Manifest：`pipeline_selector → capability_preflight → proposal_generator → script → shot_list → visual_feasibility → render_strategy`，proposal 和 render strategy 通过 Artifact Review 阻塞下游，避免直接进入高成本生成。 |
| **版本化产物管理** | 每个 stage 产物有 `version`、`contentHash`、`promptHash`，支持历史回看与「说修改意见 → 生成新版本」的返工闭环。`artifact_reviews` 表记录审核状态（PENDING/APPROVED/REJECTED），未审核产物禁止进入下游。 |
| **沙箱隔离执行** | Rust gRPC 沙箱（setrlimit 内存/CPU/磁盘/PID 限制）执行不受信任的 Bash/Python 代码，危险命令拦截。 |
| **Electron 桌面 + Web 双形态** | 同一套 React 代码，既能打包成 .dmg/.exe 桌面应用（带本地命令执行能力），也能纯 Web 访问。 |
| **事件驱动 + Outbox 可靠投递** | 事件先写 DB 再异步 relay 到 Kafka，保证基础设施抖动时不丢事件。 |
| **HybridToolRetriever** | 多信号评分（能力0.3 + 关键词0.25 + 标签0.2 + 推荐链0.1 + 领域0.15）从工具库中检索 TopK 候选工具供给 LLMPlanner。预留向量检索接口。 |

### 1.4 当前业务线

| 业务线 | 入口 Agent | 状态 |
|--------|-----------|------|
| 自媒体视频创作（口播/镜头式/导演级/拉片） | `internal/agents/video` | ✅ MVP 可用（feature-gated） |
| VideoForge Studio P0（Pipeline/Proposal/Render Strategy） | `video-pipelines/` + `skill-capabilities/video/codex-video-skill` | ✅ 可用（Dynamic Agent 工具链） |
| 内容发布 + AI 生成/润色 | `internal/agents/publish` | ✅ 可用 |
| AI 对话助手（多轮对话 → DAG） | `internal/agents/chat` | ✅ 可用 |
| 标书/投标文档生成 | `internal/agents/bid` | ✅ 可用 |

---

## 2. 整体软件架构

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────────┐
│  frontend/ Electron 桌面端 / Web 浏览器                   │
│   ├─ 本地文件、缓存、日志、诊断、基础模型直连 → local-backend │
│   └─ 云端配置、账号、远程编排、日志分析 → cloud-backend       │
└──────────────┬──────────────────────────┬───────────────┘
               │ 127.0.0.1:18080           │ HTTPS /api
┌──────────────▼───────────────┐ ┌────────▼────────────────┐
│ local-backend local-agent     │ │ cloud-backend AIOS Core │
│ 无 DB / Redis / Kafka / MinIO │ │ Go 单体（:8080，Gin）   │
└───────────────────────────────┘ │                        │
│                                                          │
│  ┌──────────── 业务 Agent 层（internal/agents）────────┐ │
│  │  video   │  bid   │  chat   │  publish              │ │
│  │ (项目/Run)│(标书) │(对话)  │ (发布/AI)              │ │
│  └───────────────────┬─────────────────────────────────┘ │
│                      │ 复用                              │
│  ┌───────────────────▼─────────────────────────────────┐ │
│  │           通用引擎层（internal/core）                │ │
│  │  agentruntime │ orchestrator │ workflow │ skillruntime│ │
│  │  worker/tool  │ modelgateway │ translator │ context  │ │
│  │  artifact │ media │ localrunner │ outbox │ eventbus   │ │
│  │  apispec │ health │ redis │ database │ logger │ model │ │
│  └───────────────────┬─────────────────────────────────┘ │
└──────────────────────┬──────────────────────────────────┘
                       │
  ┌──────────┬─────────┼──────────┬──────────┐
  ▼          ▼         ▼          ▼          ▼
PostgreSQL  Redis   Redpanda     MinIO    (Qdrant)
(持久化)   (会话)   (Kafka事件)  (对象存储) (向量库,预留)
                       │
                       ▼
              Rust Sandbox（:50051 gRPC，可选）
```

### 2.2 两个关键分层原则

1. **`internal/core`（通用引擎）不绑定业务**：编排、工具执行、工作流、模型网关都是通用能力，不知道「视频」或「标书」的存在。
2. **`internal/agents`（业务 Agent）编排领域逻辑**：每个 Agent 用 core 提供的能力拼出自己的领域流程（如视频的 Project→Run，标书的 Project→Chapter）。

> 这种分层让「加一条新业务线」≈「加一个 Agent 包 + 一个 Skill 包」，不动引擎。

### 2.3 一次「视频创作」的完整时序

#### 路径 A：Dynamic Agent（推荐）

```
用户输入一句话
  → POST /api/agent/runs
        └─ Runner.Start
             ├─ LLMPlanner.GeneratePlan（HybridToolRetriever 筛选候选工具）
             │     └─ LLM 输出 AgentPlan JSON（steps 含 knowledge_researcher →
             │        fact_checker → video_script_generator → script_quality_checker
             │        → shot_splitter → video_prompt_generator → video_package_exporter）
             ├─ PlanGuard.Validate（校验工具存在、参数类型、引用合法性、风险等级）
             ├─ PlanGuard.ValidateWithWarnings（检测缺失的质量检查器）
             ├─ PlanCompiler.Compile
             │     ├─ injectQualityGates（自动插入 quality checker + quality gate CONTROL 节点）
             │     ├─ compileStep（根据 ApprovalPolicy 插入审核 CONTROL 节点）
             │     └─ 生成 Transient DAG（TOOL → QUALITY_CHECKER → QUALITY_GATE → CONTROL → 下游）
             ├─ OrchestratorService.CreateTask + SubmitDAG
             └─ 写 agent_runs 记录
  → 事件循环（Kafka）：
       ai.node.ready  → Worker 执行（调 PromptTool / LLM）
       ai.node.result → StateMachine 更新状态 → DependencyChecker 放行下游
       (CONTROL 节点变 READY 时自动 PAUSE，质量门 CONTROL 可 autoApprove)
  → 用户审核：
       GET  /api/agent/runs/:runId/reviews（查看审核列表）
       POST /api/agent/runs/:runId/reviews/:id/approve（审核通过 → artifact_reviews=APPROVED）
       POST /api/agent/runs/:runId/reviews/:id/reject（驳回）
  → GET /api/agent/runs/:runId/trace（查看执行追踪）
```

#### 路径 B：Workflow Template（传统）

```
用户输入一句话
  → POST /api/skills/route（LLM 选 Skill + 推断交付目标）
  → POST /api/video-projects（创建项目）
  → POST /api/video-projects/:id/workflow-runs
        └─ RunService.CreateRun
             ├─ 从 template 取出 Skill 编译出的 DAG
             ├─ OrchestratorService.CreateTask + SubmitDAG
             │     └─ 写 ai_node/ai_node_dependency，无依赖节点 → READY
             └─ 写 workflow_runs 记录
  → 事件循环（Kafka）：
       ai.node.ready  → NodeExecutor 执行（调工具/LLM）
       ai.node.result → StateMachine 更新状态 → DependencyChecker 放行下游
       (CONTROL 节点变 READY 时自动 PAUSE 任务，等待人工审核)
  → 用户在前端审片：
       POST /api/node/:id/success（审核通过，解除暂停）
       POST /api/artifacts/:id/revise（返工，生成新版本）
  → GET /api/video-projects/:id/artifacts（轮询产物，每 3s）
```

---

## 3. 技术栈与依赖

### 3.1 云端后端（`cloud-backend/`）

| 类别 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.25 | 单二进制，模块化单体 |
| Web 框架 | Gin | 路由 + 中间件 |
| 数据库 | PostgreSQL 16 + pgx/v5 | 连接池（2~10 连接） |
| 缓存/会话 | Redis 7 + go-redis | 对话会话（30min TTL, 50 条上限）+ 工具清单缓存 |
| 消息队列 | Redpanda（Kafka 兼容）+ sarama | 事件驱动 |
| 对象存储 | MinIO + minio-go | 云端服务资产 / legacy 媒体兼容；桌面用户生成资产默认留在本地 |
| 配置 | Viper + .env | 环境变量优先 |
| 日志 | Zap | dev/prod 双模式 |
| LLM | OpenAI 兼容 API | `OPENAI_BASE_URL` 可指向任意兼容服务 |
| 沙箱 | Rust + tonic + tokio | gRPC :50051，setrlimit 隔离 |
| 序列化 | encoding/json + yaml.v3 + protoc | |

### 3.2 本地后端（`local-backend/`）

| 类别 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.23+ | 轻量 HTTP agent |
| 监听 | `127.0.0.1:18080` | 只服务本机桌面端 |
| 持久化 | 本地文件 | cache/config/projects/artifacts/logs/diagnostics |
| Provider 配置 | OpenAI-compatible | 用户本机保存 text_to_text/text_to_image/text_to_video 的 baseUrl/apiKey/model |
| 依赖 | 标准库 | 不引入 DB、Docker、Redis、Kafka、MinIO |

### 3.3 前端（`frontend/`）

| 类别 | 选型 |
|------|------|
| 框架 | React 18 + TypeScript 5.5 |
| 构建 | Vite 5（dev :3000，proxy `/api` → :8080） |
| 样式 | TailwindCSS 3.4 |
| 状态 | Zustand 4（`appStore.ts`，发布页用；创作台自管理 state） |
| HTTP | axios |
| 拖拽上传 | react-dropzone |
| 图标 | react-icons (Feather) |
| 桌面 | Electron 33 + electron-builder（dmg/nsis/AppImage） |

### 3.3 模块路径约定

云端 Go module 仍为 `github.com/tangying-ai/aios-core`，位于 `cloud-backend/`。本地 Go module 为 `github.com/tangying-ai/tangying-ai-operation-system/local-backend`，位于 `local-backend/`。

---

## 4. 后端模块详解（internal/core）

### 4.1 orchestrator — DAG 调度引擎

DAG 任务的核心，是整个系统的「心脏」。目录：`internal/core/orchestrator/service/`。

**核心组件：**

| 组件 | 文件 | 职责 |
|------|------|------|
| `OrchestratorService` | `orchestrator.go` | `CreateTask` 创建任务；`SubmitDAG` 校验+落库节点/依赖+初始化无依赖节点；`GetTaskWithDetails`/`GetTaskProgress` 查询 |
| `StateService` | `state.go` | 节点/任务状态转换（幂等）；`TryMakeReady` 立即检查依赖；**CONTROL 节点变 READY 时自动暂停任务**（人工审核信号） |
| `StateMachine` | `statemachine.go` | 处理节点成功/失败；触发任务完成/失败判定；发布事件 |
| `DependencyChecker` | `dependency_checker.go` | 节点执行后检查下游依赖是否满足（SUCCESS 或 SKIPPED 算满足） |
| `RetryPolicy` | `retry_policy.go` | 指数退避：1s→2s→4s…→60s 封顶 |
| `Scheduler` | `scheduler.go` | 30s 兜底心跳：捞回卡在 CREATED 超过 1 分钟的节点；处理长任务心跳超时 |
| `TaskExecutionControl` | `task_execution_control.go` | 任务的暂停/恢复/重试，持久化 pause_reason |
| `DAGValidator` | `dag_validator.go` | 环检测 + 重复节点检查 |

**节点状态机：**
```
CREATED → READY（依赖满足）→ RUNNING → SUCCESS / FAILED
                                         │
                              FAILED → RETRYING → CREATED（Scheduler 兜底回 READY）
SKIPPED（条件未满足，对下游等同满足）
```

**节点类型（`model.NodeType`）：**
- `TOOL` — 调用工具执行
- `LLM` — 路由到内置 `llm_api` 工具
- `CONTROL` — 人工审核节点 / 质量门禁节点，变 READY 时暂停任务
- `SYSTEM_GATE` — 质量门禁（quality_gate CONTROL），根据 checker 输出 autoApprove 或阻塞

**依赖满足规则：** 父节点为 `SUCCESS` **或** `SKIPPED` 即满足（见 `state.go:228`）。

#### 4.1.5 agentruntime — 动态 Agent Runtime（v3.2 新增）

目录：`internal/core/agentruntime/`。从自然语言到可执行 DAG 的「规划—校验—编译」全链路。

**核心流程：** `Planner → PlanGuard → PlanCompiler → Transient DAG`

| 组件 | 文件 | 职责 |
|------|------|------|
| `LLMPlanner` | `llm_planner.go` | LLM 驱动生成 AgentPlan JSON。走 ModelGateway。内置 `AgentPlanJSONSchema` 约束。 |
| `HeuristicPlanner` | `planner.go` | 启发式选 TopK 工具线性编排，无需 LLM。 |
| `HybridPlanner` | `llm_planner.go` | LLMPlanner 优先，失败回退 HeuristicPlanner。 |
| `GatewayPlannerClient` | `gateway_planner_client.go` | ModelGateway 适配器，将 Planner 的 LLM 调用走统一网关（指纹缓存+重试）。 |
| `HybridToolRetriever` | `tool_retriever.go` | 多信号评分检索（能力+关键词+标签+推荐链+领域）扣成本/风险惩罚。 |
| `PlanGuard` | `plan_guard.go` | 校验工具存在、参数类型、引用表达式含 output schema 字段存在性、风险等级、侧效应、maxToolCalls。 |
| `PlanCompiler` | `plan_compiler.go` | 编译 AgentPlan→Transient DAG：自动插入审核 CONTROL 节点、quality checker + quality gate CONTROL 节点。 |
| `Runner` | `runner.go` | 编排 Planner→Guard→Compiler→Orchestrator 全流程。 |
| `Handler` | `handler.go` | HTTP：`POST/GET /api/agent/runs`，审核 approve/reject，集成 `artifact_reviews` 表。 |
| `ArtifactReviewStore` | `artifact_review.go` | `artifact_reviews` 表持久化：PENDING→APPROVED/REJECTED。 |
| `PlanRepairer` | `llm_planner.go` | Guard 失败后 LLM 修复一次，失败则 fallback。 |
| `AgentRunRepository` | `repository.go` | `agent_runs` 表持久化（plan JSONB/stage/status）。 |
| `E2E Smoke Tests` | `e2e_test.go` | 🆕 端到端冒烟测试（~740 行），覆盖 Planner→Guard→Compiler 全链路、质量门禁、Artifact Review。 |

**核心 Agent API：**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/agent/runs` | 启动 dynamic agent run（message → AgentPlan → DAG → 执行） |
| GET | `/api/agent/runs/:id` | 查询 run 状态和 plan |
| GET | `/api/agent/runs/:id/trace` | 获取 DAG 执行追踪 |
| GET | `/api/agent/runs/:id/reviews` | 列出待审核节点 |
| POST | `/api/agent/runs/:id/reviews/:reviewId/approve` | 审核通过 |
| POST | `/api/agent/runs/:id/reviews/:reviewId/reject` | 审核驳回 |

### 4.2 worker — 工具执行引擎

目录：`internal/core/worker/`。

#### 4.2.1 工具接口（`tool/tool.go`）

```go
Tool                 // 基础：Name/Description/Type/Execute/ValidateParameters
BuildableTool        // 产出 ExecutionRequest，路由到 Direct/Sandbox Executor（bash/python）
ExecutableTool       // 自执行（API 类工具，LLM/视频分析等）
ManifestProvider     // 提供 ToolManifest 给知识库
ExternalToolProvider // 按名执行已注册外部工具
ProgressReporter     // 长任务进度回调（heartbeat + progress + checkpoint）
```

**工具名路由规则（`DetermineToolName`）：**
1. `payload["tool"]` 显式指定 → 最高优先
2. TOOL 类型 → `payload["name"]`
3. LLM 类型 → 固定 `llm_api`

#### 4.2.2 内置工具（`tool/builtin/`）

| 工具名 | 文件 | 类型 | 作用 |
|--------|------|------|------|
| `bash` | bash_tool.go | Buildable | 沙箱 Shell：命令白名单 + 危险模式过滤 |
| `python` | python_tool.go | Buildable | python3 -c，资源限制 |
| `llm_api` | builtin.go | Executable | OpenAI chat/completions |
| `polisher` | polisher_tool.go | Executable | 标题/描述润色 |
| `media_analyzer` | media_analyzer.go | Executable | 图片/视频多模态分析，提标签/建议 |
| `content_generator` | content_generator.go | Executable | 基于媒体分析生成内容包 |
| `content_checker` | content_checker.go | Executable | 合规检查（敏感词、广告法、平台规则） |
| `platform_adapter` | platform_adapter.go | Executable | 跨平台适配（7 平台语气/格式/长度） |
| `chat_generate` | chat_generate_tool.go | Executable | 多轮对话内容生成 |
| `chat_revise` | chat_revise_tool.go | Executable | 自然语言改写 title/description/keywords |
| `external` | external_tool.go | Executable + ExternalToolProvider | HTTP 桥接已注册外部工具 |
| `video_metadata` | video_metadata.go | Executable | 下载视频→提取时长/分辨率/帧率/编码 |
| `video_analyzer` | video_analyzer.go | Executable | ffmpeg 关键帧 + Whisper 转录 |
| `video_copy_generator` | video_copy_generator.go | Executable | 多模态生成短视频文案 |
| `pipeline_selector` | video_creation_external_tools.go | builtin_prompt_tool | 读取 `video-pipelines/*.yaml`，选择 VideoForge pipeline |
| `capability_preflight` | video_creation_external_tools.go | builtin_prompt_tool | 输出文本模型、HyperFrames、Seedance、TTS/ASR 能力状态 |
| `proposal_generator` | video_creation_external_tools.go | builtin_prompt_tool | 生成 `proposal_packet`，需 Artifact Review 后继续 |
| `visual_feasibility_analyzer` | video_creation_external_tools.go | builtin_prompt_tool | 按镜头和元素评估 HyperFrames/Seedance/Hybrid 可行性 |
| `render_strategy_planner` | video_creation_external_tools.go | builtin_prompt_tool | 生成 `render_strategy` 和 decision log，需 Artifact Review 后继续 |
| *(动态注册)* | video_creation_external_tools.go | — | 视频创作开启时注册的 seedance/imagegen 等外部工具 |

#### 4.2.3 执行器与 NodeExecutor（`service/executor.go`）

- `DirectExecutor` — 本地子进程执行（BuildableTool 默认）
- `SandboxExecutor` — gRPC 到 Rust 沙箱（`SANDBOX_ENABLED=true` 时，BuildableTool 走这里）
- `NodeExecutor.ExecuteNode` — 消费 `ai.node.ready` 事件的核心流程：
  1. 从 DB 补回被 Kafka 裁掉的 `image_urls`（太大不入消息）
  2. 解析 `{{node_id.output.field}}` 节点引用（跨节点数据传递）
  3. 长任务：起 heartbeat goroutine + 进度回调
  4. 选执行器执行 → 发布 `ai.node.result`（SUCCESS/FAILED）

**长任务机制：** 节点标 `longRunning:true` 后，按 `heartbeat_interval` 发心跳、`heartbeat_timeout` 判超时；工具实现 `ProgressReporter` 可上报进度百分比和 checkpoint 断点。

#### 4.2.4 ToolManifest（`tool/manifest.go`）

工具的完整规格说明书（Name/Description/Parameters/Output/Sandbox/Examples），既是 AI DAG 规划的知识库，也是 `/api/tools` 的返回。内置工具若实现 `ManifestProvider` 用自定义 manifest，否则生成最小 manifest。

### 4.3 workflow — 工作流模板 + Run + Skill 编译器

目录：`internal/core/workflow/`。

| 概念 | 文件 | 说明 |
|------|------|------|
| `Template` | model.go | 可复用 DAG 蓝图（id/version/name/category/dag）。表 `workflow_templates` |
| `WorkflowRun` | run_model.go | 一次执行实例，关联 video_project + task。状态：PENDING/RUNNING/PAUSED/COMPLETED/FAILED/CANCELLED |
| `StageStatus` | run_model.go | 单阶段状态，含 WAITING_APPROVAL / INVALIDATED |
| `RunService` | run_service.go | `CreateRun`：取模板→应用 input 覆盖→建 task→提交 DAG→建 run 记录 |
| `CompileSkillToDAG` | skill_compiler.go | **核心**：把 Skill stages 编译成 DAG（见下） |
| `Service`/`Repository` | service.go/repository.go | 模板 CRUD（Upsert/FindByID/List） |
| `seed.go` | | 启动时种入内置模板 |
| `EnsureSchema` | | 建表 |

**CompileSkillToDAG 转换规则（`skill_compiler.go:16`）：**
- 普通 stage（`approval_required:false`）→ 单个 TOOL 节点
- 需审核 stage（`approval_required:true`）→ `{name}_exec`(TOOL) + `{name}`(CONTROL) 双节点链
- 可选 stage（`optional:true`）→ 额外生成 `{name}_skip`(CONTROL) 旁路
- `long_running` / `heartbeat_timeout_sec` 透传到节点

### 4.4 skillruntime — Skill 包加载运行时

目录：`internal/core/skillruntime/`。

| 文件 | 职责 |
|------|------|
| `manifest.go` | `SkillManifest`（name/version/stages/health/visibility）+ `StageDefinition`（name/tool/instruction/optional/approval_required/long_running）+ `SkillSummary` |
| `registry.go` | 线程安全的 `Registry`；`LoadSkills(root)` 扫描 `{root}/{name}/{version}/skill.yaml`，校验 instruction/schema 文件存在，失败标记 UNHEALTHY |
| `handler.go` | HTTP 路由 `/api/skills`（List/Catalog/Route/Get/Compile）；注入 `CompileFunc` 避免与 workflow 循环依赖 |
| `router.go` | **LLM Skill Router**：把用户 brief + 可见 video Skill 目录喂给 LLM，返回选中的 skill + route + deliverable + 画幅 + 时长；无 API Key 时走关键词规则兜底 |

**Skill 可见性：** `visibility: hidden` 的 Skill 不进 Catalog（如 `voice-visual-video` 已被 `create-opinion-videos` 取代，通过 `canonical_skill` 标注）。

### 4.5 modelgateway — 统一模型网关

目录：`internal/core/modelgateway/`。

| 文件 | 职责 |
|------|------|
| `gateway.go` | `Gateway.Execute`：指纹缓存（幂等）→ 按 Capability 选 Provider → 指数退避重试（3 次）→ 缓存成功结果 |
| `provider.go` | `Provider` 接口 |
| `types.go` | Capability（text_to_text/image_to_text/text_to_image/text_to_video/image_to_video）、ModelRequest/Result、GatewayError（带 Retry 标记）、TestConfig（故障注入） |
| `providers/fake/` | Fake Provider，用于无 API Key 的端到端测试，可注入 rate_limit/timeout/server_error 故障 |

> ⚠️ 注意：当前 main.go **未把 Gateway 接入主流程**，工具仍直接用 `cfg.OpenAI`。Gateway 已实现并测试，待后续作为统一出口（见已知限制）。

### 4.6 artifact — 版本化产物管理

目录：`internal/core/artifact/`。

| 文件 | 职责 |
|------|------|
| `model.go` | `Artifact`（projectID/stage/unit/kind/version/parentID/storageType/contentHash/promptHash/provider/model/isCurrent）。Kind: JSON/MARKDOWN/IMAGE/AUDIO/VIDEO/BUNDLE/LOG |
| `repository.go` | 带版本化的写入（同 project+stage+unit 自增 version，旧版 isCurrent=false） |
| `service.go` | CreateArtifact / GetByID / ListByProject / GetHistory |
| `materializer.go` | **`BuildArtifactRequestsFromNode`**：只从成功 node output 的 `artifacts` 本地 manifest 提取产物索引并落库 |
| `handler.go` | HTTP 路由：列表/详情/内容/历史/返工。返工 `ReviseArtifact` 生成新版本元数据，本体由本地 agent 保存 |

**产物存储边界：**

| 内容类型 | 保存位置 | 云端保存什么 |
|----------|----------|--------------|
| 用户生成文本、脚本、JSON、分镜、Prompt、图片、音频、视频 | 用户本机 `local-backend` 数据目录 `artifacts/` | `storage_type=local`、`storage_ref=local://...`、hash、size、版本、provider/model、必要 metadata |
| 云端服务日志、任务状态、模型调用审计、外部服务错误 | 云端 PostgreSQL / structured logs | 服务运行所需的日志和索引 |
| 历史 legacy 产物 | 旧库中可能仍有 `inline`/`minio` | 仅保留兼容读取，新产物不再默认写入 |

云端 artifact 层不得把用户产物正文写入 `artifacts.inline_json`，也不得把用户图片、音频、视频作为默认流程上传 MinIO。MinIO 只作为云端服务资产或历史兼容能力存在，不是本地用户生成资产的默认存储。

**本地 artifact manifest 协议：** 本地执行器保存正文/图片/音频/视频后，只向云端回传索引数组：

```json
{
  "artifacts": [
    {
      "unitId": "content",
      "kind": "MARKDOWN",
      "name": "script.md",
      "mimeType": "text/markdown; charset=utf-8",
      "storageRef": "local://projects/vp-1/artifacts/script/content/hash/script.md",
      "contentHash": "hash",
      "sizeBytes": 128,
      "metadata": {"displayable": true}
    }
  ]
}
```

`content`、`imageRequests`、`videoImportPackage`、`audioPackage` 等 legacy 正文字段不再被 materializer 转成云端产物；`ai_node.output` / `ai_task.output` 和 node result event 写入前会对正文、Prompt、data URL、媒体 URL 做脱敏，只保留本地引用、hash、size、状态和 trace 信息。

### 4.7 localrunner — Electron 本地任务协议

目录：`internal/core/localrunner/`。EdgeRun 的云端控制面协议：`local_runners` 负责本地执行器注册、心跳和能力探测结果；`local_jobs` 负责云端编排到本地执行面的语义化任务队列（PENDING→CLAIMED→RUNNING→COMPLETED/FAILED，带租约 + SKIP LOCKED 抢占）；`local_job_logs` 保存本地任务日志。

已接入 HTTP 路由：

- `POST /api/local-runners/register`
- `POST /api/local-runners/:runnerId/heartbeat`
- `GET /api/local-runners/:runnerId/jobs/claim`
- `POST /api/local-jobs/:jobId/progress`
- `POST /api/local-jobs/:jobId/complete`
- `POST /api/local-jobs/:jobId/fail`

`NodeExecutor` 会读取 `ToolManifest.executionPlane`，当工具声明 `local` 时创建 `LocalJob` 并把节点置为 `WAITING_LOCAL`，由本地 runner 主动拉取执行；完成或失败回调会进入 StateMachine 推进 DAG。

### 4.8 apispec — OpenAPI 规范自动生成

目录：`internal/core/apispec/`。

基于 Gin 路由树自动生成 OpenAPI 3.0 规范，并通过 Swagger UI 和 TypeScript 代码生成工具输出可消费的 API 文档和前端类型。

| 文件 | 职责 |
|------|------|
| `cloud_spec.go` | `BuildCloudSpec()` — 遍历所有已注册路由，构建 OpenAPI 3.0 规范对象（含请求/响应 schema） |
| `register.go` | `Register(r, spec)` — 在 Gin 路由上挂载 Swagger UI（`/docs`）和 `/api/openapi.json` 端点 |
| `cloud_schemas.go` | 所有 API 的请求/响应 JSON Schema 定义（统一 `{code, message, data}` 信封） |
| `builder.go` | OpenAPI spec 对象构建辅助函数 |
| `swaggerui.go` | 内嵌 Swagger UI HTML（无需外部 CDN） |
| `tscodegen.go` | 从 OpenAPI spec 生成前端 TypeScript 类型定义（`api-types.generated.ts`） |
| `markdown.go` | 从 OpenAPI spec 生成 API 参考文档（`API_REFERENCE.md`） |
| `schema.go` / `types.go` | 内部类型和 schema 工具 |

**设计原则：** 规范完全由代码路由注册驱动，不手写 OpenAPI YAML/JSON，避免代码与文档不同步。CI 可通过 `make api-docs-check` 校验生成文件是否过期。

### 4.9 health — 就绪健康检查

目录：`internal/core/health/`。

提供带依赖探测的就绪检查端点（readiness probe），供 Docker compose、Kubernetes、Electron 桌面端使用。

| 文件 | 职责 |
|------|------|
| `handler.go` | `GET /api/health/ready` — 返回 `{"status":"UP/DOWN","service":"tangying-ai-os","dependencies":{...}}`；每个依赖并发探测，2s 超时 |
| `handler_test.go` | 就绪检查单元测试 |

**依赖探测项：**
- `postgres` — pgx pool ping
- `redis` — redis client ping
- `kafka` — sarama client 连接 broker + 获取 controller

任一依赖不健康时 HTTP 状态码为 503，`status` 字段为 `"DOWN"`，对应依赖的 `status` 也为 `"DOWN"` 并附带 `error` 信息。所有依赖健康时返回 200 + `"UP"`。

### 4.10 redis — Redis 客户端

目录：`internal/core/redis/`。

| 文件 | 职责 |
|------|------|
| `redis.go` | `NewClient(cfg)` — 创建 go-redis 客户端，用于会话管理、工具清单缓存等 |

封装了 go-redis 客户端初始化，统一连接配置（地址、密码、DB、超时），供 `chat.SessionManager` 和 `ToolManifestService` 使用。

### 4.11 其他核心包

| 包 | 职责要点 |
|----|---------|
| `translator` | NL→DAG：`NlToDagService` 调 OpenAI 生成 DAG JSON，注入工具清单。路由 `/api/translate*` |
| `context` | 审计：`ContextService.HandleEvent` 从 Kafka 事件自动记录任务/节点生命周期；路由 `/api/task/:id/context`、`/api/context/record` |
| `media` | MinIO 媒体管理：`MediaHandler`（upload/list/get/tags）、`StorageService`（自动建桶、presigned URL）、路由 `/api/media/*`。默认不用于保存桌面用户生成资产 |
| `model` + `model/repository` | 数据模型（Task/Node/Context/MediaAsset/ToolManifest）+ pgx 仓储 |
| `outbox` | `Relay`（100ms ticker）读 outbox 表发 Kafka 成功后删除；`SaveEvent` 写 outbox。v3.1 新增接口抽象层（`EventSaver`/`EventPublisher`/`OutboxStore`）支持内存 mock 测试；Relay 新增指数退避重试 + DLQ 死信队列 |
| `eventbus` | sarama Producer/Consumer；Topic 常量 |
| `config` | Viper 加载 .env，`VideoConfig` 控制视频 feature flags；`HyperFramesConfig` 控制渲染服务连接 |
| `hyperframes` | HyperFrames Render Service HTTP 客户端：`Client`（Health/Render/Lint）、`Config`（Mode/ServiceURL/Timeout/Quality 等）— 替代旧 CLI `exec.Command("npx", "hyperframes")` |
| `skillcapability` | 🆕 Skill Capability 加载系统：`Loader` 扫描 `{root}/{domain}/{name}/{version}/skillcap.yaml` + `tools/*.tool.yaml`，构建 `CapabilityRegistry`；`Handler` 提供 `/api/skill-capabilities` 路由 |
| `database` | pgx 池 + `RunMigrations`（`CREATE TABLE IF NOT EXISTS` + `ALTER`） |
| `logger` | Zap dev/prod |
| `common/llmutil` `common/jsonx` `common/metadata` | OpenAI 客户端、JSON 提取、元数据摘要工具 |

---

## 5. 业务 Agent 详解（internal/agents）

### 5.1 video — 视频创作领域

目录：`internal/agents/video/`。

| 文件 | 职责 |
|------|------|
| `model/project.go` | `VideoProject`（mode: aigc_shot/voice_visual；status: DRAFT/RUNNING/PAUSED/COMPLETED/ARCHIVED；generationMode: provider_api/manual_import；skillName/Version/workflowName/Version/aspectRatio/targetDuration） |
| `model/shot.go` `model/voice.go` | Shot / 口播可视化模型（含组件 DSL 校验，`component_validator.go` 支持 10 种组件） |
| `repository/project_repo.go` | 项目 CRUD（软删除 via deleted_at） |
| `service/project_service.go` | CreateProject/GetProject/ListProjects/UpdateProject/ArchiveProject |
| `handler/project_handler.go` | 路由 `/api/video-projects`（GET/POST/:id/PATCH/DELETE） |
| `handler/workflow_handler.go` | 路由 `/api/video-projects/:id/workflow-runs`（POST 创建/GET 查询/pause/cancel） |

**两种生产模式：**
- `aigc_shot`（镜头式）— 关联 skill: aigc-shot-video / video-creator
- `voice_visual`（口播可视化）— 关联 skill: create-opinion-videos

**两种生成模式：**
- `provider_api` — 调外部模型 API
- `manual_import` — 手动导入成片（当前 MVP 默认，无需视频 API）

### 5.2 bid — 标书/投标文档生成

目录：`internal/agents/bid/`。完整的项目→章节→审核→导出流程：

- **模型：** `BidProject`（status: DRAFT/PARSING/GENERATING/...）、`BidChapter`（node_id 关联 CONTROL 节点）、`BidTemplate`
- **Service 流程：** 上传标书 → `SetTenderFile` → `StartGeneration`（建 task + 提交 `BuildBidDAG`）→ 逐章 `ApproveChapter`（把 CONTROL 节点标 SUCCESS 解锁下游）/ `RejectChapter`（标 FAILED 触发重试）→ `GetExportStatus`（查 doc_exporter 节点）
- **路由：** `/api/bid/projects/*`、`/api/bid/templates`

### 5.3 chat — AI 对话助手

目录：`internal/agents/chat/`。多轮对话，每轮 = 一个 Task。

| 组件 | 职责 |
|------|------|
| `SessionManager` | Redis 会话（30min TTL, 50 条消息上限），存消息历史 + 媒体上下文 + task IDs |
| `PlanService` | 用对话历史 + 媒体上下文 + 工具清单构建 prompt → LLM 生成 DAG |
| `ResultAssembler` | 建 task + 提交 DAG + 轮询 + 从 node output 提取 title/description/keywords |
| `LLMClient` | 类型化 OpenAI 客户端（JSON schema 响应） |
| `ToolManifestService` | DB 持久化 + Redis 缓存的工具知识库，启动同步内置工具 |
| `handler/session_handler.go` | **路由 `/api/chat/sessions`**（create/get/:id/chat/progress/terminate）。⚠️ 注意：旧文档写的 `/api/skill/dialog/session/*` 已废弃，实际是 `/api/chat/sessions/*` |

**对话流程：** append 用户消息 → GeneratePlan（DAG）→ CreateTask+SubmitDAG → PollAndExtract → append 助手回复 → 返回 reply + fields。

### 5.4 publish — 内容发布 + AI 生成

目录：`internal/agents/publish/`。

| Handler | 路由 |
|---------|------|
| `PublishHandler` | `/api/publish`、`/api/ai/generate`、`/api/ai/generate-from-media`、`/api/ai/polish`、`/api/ai/polish/submit`、`/api/ai/polish/result` |
| `TraceHandler` | `/api/trace/recent`、`/api/trace/:taskId` |
| `ToolHandler` | `/api/tools`（GET/GET:name/POST:register/DELETE:name） |

`PublishService` 把发布/AI 生成编排成 DAG 提交给 orchestrator；`AIGenerateFromMedia` 走视频元数据→帧分析→转录→文案的完整管线。

---

## 6. Skill Package 系统

### 6.1 目录结构

```
cloud-backend/skills/{name}/{version}/
├── skill.yaml          # 清单：stages + 元信息
├── stages/*.md         # 每个 stage 的指令（Prompt）
└── schemas/*.json      # 输入/输出 JSON Schema（可选）
```

### 6.2 skill.yaml 字段

```yaml
name: aigc-shot-video          # 目录名（运行时由路径注入，yaml 内可省略）
version: "1.0.0"
display_name: ...              # 可选，UI 显示名
description: ...
category: video                # Router 按此过滤（目前只用 video）
visibility: public|hidden      # hidden 不进 Catalog
canonical_skill: xxx@1.0.0     # 标注被谁取代
stages:
  - name: storyboard
    kind: TOOL|CONTROL|APPROVAL
    tool: skill_stage_agent    # 该 stage 调用的工具名（缺省 skill_stage_agent）
    instruction: stages/storyboard.md
    input_schema: schemas/x.json
    output_schema: schemas/y.json
    optional: true|false
    approval_required: true|false
    long_running: true|false
    heartbeat_timeout_sec: 600
requires:                      # 声明依赖的模型能力
  text_to_text: true
  text_to_image: true
```

### 6.3 当前 6 个 Skill

| Skill | 阶段数 | 路由用途 | 可见性 |
|-------|--------|----------|--------|
| `create-opinion-videos` | 8 | 口播/知识视频（默认路由） | public |
| `aigc-shot-video` | 9 | 镜头式 AIGC 短片（关键帧/视频提示词） | public |
| `video-creator` | 12 | 导演级视频流水线（IP/角色/导演剪辑） | public |
| `film-shot-reconstruction` | 10 | 经典镜头拉片学习 | public |
| `voice-post-production` | 4 | 音频后期处理 | public |
| `voice-visual-video` | 8 | 旧版口播可视化（已被 create-opinion-videos 取代） | hidden |

> ⚠️ 旧 README 写「2 个 Skill（aigc-shot-video / voice-visual-video）」已过时，实际 6 个。

### 6.4 三种转 Workflow 的方式

```bash
# 1) CLI
go run cmd/skill2workflow/main.go --skill skills/aigc-shot-video/1.0.0
go run cmd/skill2workflow/main.go --skill-root skills/ --output out/

# 2) API
curl -X POST http://localhost:8080/api/skills/aigc-shot-video/1.0.0/compile

# 3) 启动自动注册（LEGACY_SKILL_WORKFLOW_AUTOREGISTER=true 时，每个 healthy skill 自动 Upsert 为模板）
#    默认 false（v3.2+ 推荐走 Dynamic Agent Runtime，不再自动注册 legacy Skill 模板）
```

---

## 7. 数据模型与数据库表

所有建表在 `internal/core/database/database.go:RunMigrations`，用 `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`（无独立迁移工具）。

### 7.1 核心引擎表

| 表 | 用途 | 关键字段 |
|----|------|---------|
| `ai_task` | 任务 | id, status(CREATED/RUNNING/PAUSED/SUCCESS/FAILED), input/output JSONB, pause_reason |
| `ai_node` | DAG 节点 | id, task_id, type, name, status, input/output JSONB, condition, retry_count, max_retry, priority, worker_group, idempotency_key, long_running, progress, heartbeat_at |
| `ai_node_dependency` | 依赖边 | parent_node_id, child_node_id |
| `ai_context` | 审计日志 | context_type(17 种枚举), task_id, node_id, metadata, snapshot_data, source_module |
| `outbox` | 事件暂存 | aggregate_type, aggregate_id, event_type, payload |
| `media_assets` | 云端媒体资产 / legacy 媒体索引 | user_id, original_name, mime_type, size, minio_path, tags JSONB, embedding_id |
| `tool_manifests` | 工具清单持久化 | name, type, endpoint, execution_plane, artifact_location, local_command, local_requirements, parameters/output JSONB |
| `workflow_templates` | 工作流模板 | id, version, name, category, dag JSONB |
| `workflow_runs` | 工作流执行 | project_id, template_id, task_id, status, stage_statuses JSONB |
| `workflow_attempts` | 阶段重试记录 | stage_run_id, number, trigger_type(INITIAL/RERUN/RETRY), node_ids |
| `model_calls` | 模型调用审计 | provider, model, capability, fingerprint, tokens, cost_usd |
| `local_runners` | 本地 Runner 注册 | device_id, user_id, runner_version, platform, workspace_root, capabilities, session_id, status, last_heartbeat |
| `local_jobs` | 本地任务 | runner_id, project_id, task_id, node_id, tool_name, command, payload, status, progress, artifact_policy, lease_expires_at |
| `local_job_logs` | 本地任务日志 | job_id, level, message, created_at |

### 7.2 业务表

| 表 | 用途 |
|----|------|
| `video_projects` | 视频项目（mode/skill/workflow/generation_mode/aspect_ratio/target_duration，软删除 deleted_at） |
| `artifacts` | 版本化产物索引（project/stage/unit/kind/version/parent_id/storage_type/storage_ref/content_hash/is_current；新产物正文在本地） |
| `bid_projects` | 标书项目（task_id/template_id/industry/tender_file/structure/config） |
| `bid_chapters` | 标书章节（node_id 关联 CONTROL，status, review_comment, score_items） |
| `bid_templates` | 标书模板（structure JSONB, workflow_dag） |

---

## 8. 完整 API 接口清单

**统一响应格式：** `{"code": 200, "message": "success", "data": {...}}`

### 8.1 视频创作（v2.0，需 `VIDEO_CREATION_ENABLED=true`）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/skills` | Skill 列表 + 健康状态 |
| GET | `/api/skills/catalog?includeHidden=` | 轻量 Catalog（路由用） |
| POST | `/api/skills/route` | **核心入口**：brief → LLM 选 Skill + route + deliverable + 画幅/时长 |
| GET | `/api/skills/:name/:version` | Skill 详情（含 stages） |
| POST | `/api/skills/:name/:version/compile` | 编译为 DAG |
| GET | `/api/workflows` | 工作流模板列表 |
| POST | `/api/workflows` | 创建/更新模板 |
| POST | `/api/workflows/:id/instantiate` | 实例化（带 overrides） |
| GET/POST | `/api/video-projects` | 项目列表 / 创建 |
| GET/PATCH/DELETE | `/api/video-projects/:id` | 详情 / 更新 / 归档（软删除） |
| POST | `/api/video-projects/:id/workflow-runs` | 启动工作流（body: templateId/templateVersion/input） |
| GET | `/api/video-projects/:id/workflow-runs/:rid` | 查询 Run |
| POST | `/api/video-projects/:id/workflow-runs/:rid/pause` | 暂停 |
| POST | `/api/video-projects/:id/workflow-runs/:rid/cancel` | 取消 |
| POST | `/api/video-projects/:id/stages/:stage/approve` | 视频域阶段审核封装（body: runId/output/comment；内部映射 CONTROL 节点） |
| GET | `/api/video-projects/:id/artifacts` | 项目产物列表（触发 materialize） |
| GET | `/api/artifacts/:id` | 产物详情 |
| GET | `/api/artifacts/:id/content` | 产物内容。legacy inline/minio 可返回内容或 URL；新 local 产物返回 local storageRef，由桌面端向本地 agent 读取正文 |
| GET | `/api/artifacts/:id/history` | 产物版本历史 |
| POST | `/api/artifacts/:id/revise` | 返工（body: message → 新版本） |

### 8.2 任务调度与审核

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/task/create` | 创建任务 |
| POST | `/api/task/:taskId/dag` | 提交 DAG |
| GET | `/api/task/:taskId` | 任务详情（含节点/边） |
| POST | `/api/node` | 从 NL 翻译并提交（建 task + 提 DAG） |
| POST | `/api/node/:nodeId/success` | **人工审核通过**（CONTROL 节点 → SUCCESS，解锁下游） |
| POST | `/api/node/:nodeId/failure` | 标记节点失败 |
| POST | `/api/task/:taskId/fail` | 终止任务 |
| GET | `/api/trace/recent` `/api/trace/:taskId` | 审计追踪（task + contexts） |

### 8.3 内容发布与 AI

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/publish` | 发布（JSON 或 multipart：title/description/keywords/platforms/videos/images/cover） |
| POST | `/api/ai/generate` | 文本 → 生成标题/描述 |
| POST | `/api/ai/generate-from-media` | 图片/视频 + prompt → 多模态生成（multipart，180s 超时） |
| POST | `/api/ai/polish` | 同步润色（走 orchestrator→PolisherTool） |
| POST | `/api/ai/polish/submit` | 异步润色提交（返回 taskId+nodeId，可取消） |
| GET | `/api/ai/polish/result?taskId=&nodeId=` | 查询异步润色结果 |
| GET | `/api/task/:taskId/context` | 任务上下文历史 |
| POST | `/api/context/record` | 记录上下文事件 |

### 8.4 AI 对话助手

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/chat/sessions/create` | 建会话（带媒体上下文 + mediaIds→presigned URLs） |
| GET | `/api/chat/sessions/:id` | 会话状态（消息历史 + 媒体 + task IDs） |
| POST | `/api/chat/sessions/:id/chat` | 发消息 → LLM 规划 DAG → 执行 → 返回 reply + fields |
| GET | `/api/chat/sessions/:id/progress` | 进度（IDLE/EXECUTING/TERMINATED） |
| POST | `/api/chat/sessions/:id/terminate` | 终止会话 |

### 8.5 工具注册与媒体

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/tools` `/api/tools/:name` | 工具清单（内置+外部，含 manifest） |
| POST | `/api/tools/register` | 注册外部工具（HTTP endpoint + manifest） |
| DELETE | `/api/tools/:name` | 注销外部工具 |
| POST | `/api/media/upload` | 上传媒体（multipart → MinIO） |
| GET | `/api/media/list?userId=&offset=&limit=&tag=` | 列表（JSONB `@>` 标签过滤） |
| GET | `/api/media/:id` | 单个资产 |
| PUT | `/api/media/:id/tags` | 更新标签 |

### 8.6 标书生成

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/api/bid/projects` | 列表 / 创建 |
| GET/PUT/DELETE | `/api/bid/projects/:id` | 详情(含章节) / 更新 / 删除 |
| POST | `/api/bid/projects/:id/upload-tender` | 上传标书文件 |
| POST | `/api/bid/projects/:id/start` `/pause` `/resume` | 生成生命周期 |
| POST | `/api/bid/projects/:id/chapters/:chId/approve` `/reject` `/regenerate` | 章节审核（触发 CONTROL 节点） |
| POST | `/api/bid/projects/:id/export` | 导出（docx） |
| GET | `/api/bid/projects/:id/export/status` `/progress` `/trace` | 导出/进度/追踪 |
| GET | `/api/bid/templates` | 标书模板 |

### 8.7 健康检查

`GET /api/health/ready` — 就绪检查（readiness probe），供 Docker compose / Kubernetes / Electron 判活。

### 8.8 配置与能力注册

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/config/model-provider` | 读取云端模型 Provider 配置（不回显完整 API Key） |
| PUT | `/api/config/model-provider` | 更新云端模型 Provider 配置 |
| DELETE | `/api/config/model-provider` | 删除云端模型 Provider 配置 |
| GET | `/api/skill-capabilities` | 🆕 列出已加载的 Skill Capability 包 |
| GET | `/api/skill-capabilities/:id` | 🆕 获取 Capability 包详情（含 tools） |

### 8.9 健康检查响应格式

```json
{
  "status": "UP",
  "service": "tangying-ai-os",
  "dependencies": {
    "postgres": {"status": "UP"},
    "redis":    {"status": "UP"},
    "kafka":    {"status": "UP"}
  }
}
```

任一依赖不健康时 HTTP 状态码 503，`status` 为 `"DOWN"`，对应依赖项附带 `"error"` 字段说明原因。所有依赖探测并发执行，2 秒超时。

---

## 9. 前端架构（React + Electron）

### 9.1 导航与页面结构

`App.tsx` 用 `activeNav` 状态切换页面（无路由库），侧边栏 4 项：

| nav id | 页面 | 状态 |
|--------|------|------|
| `creator` | `CreatorWorkbenchPage`（创作台，主页面） | ✅ 完整实现 |
| `projects` | Placeholder（作品集） | 待实现 |
| `skills` | Placeholder（技能管理） | 待实现 |
| `system` | `DesktopPage`（系统：健康检查、命令执行） | ✅ 实现 |

> ⚠️ 旧文档里的 `PublishPage` 仍在文件树中但**已不被 App.tsx 引用**（遗留文件），实际入口是 CreatorWorkbenchPage。

### 9.2 创作台 CreatorWorkbenchPage（核心新页面）

一个自包含的「自然语言视频创作台」，主流程：

1. **一句话入口** — textarea + 示例 chips，输入后 **500ms 防抖**调 `/api/skills/route`
2. **系统理解面板** — 展示路由结果（RouteCard：talking_head/cinematic_short/director_pipeline/shot_learning 四条线）+ 命中能力卡片（阶段数/审核门/长任务/置信度）
3. **能力管理** — 4 条基础路径卡片，点击填入示例 prompt
4. **启动创作线** — `handleStart`：route → 建 video project → 建 workflow run
5. **制作进度** — 按 deliverable（publish_pack/script_only/keyframes/video_prompt/shot_learning）过滤展示 stage 卡片
6. **产物审片台** — 每 3s 轮询 `/api/video-projects/:id/artifacts`；点产物打开大窗 `ArtifactReviewModal`
7. **产物大窗** — 按 kind 渲染：MARKDOWN（文档）/ IMAGE（画廊）/ VIDEO（播放器+轨道）/ AUDIO（播放器）/ JSON（ReadableValue 树）；右侧返工面板（说修改意见→生成新版本）+ 审核按钮（调 `/api/video-projects/:id/stages/:stage/approve`，由后端映射到底层 CONTROL 节点）
8. **制作记录** — `TraceModal` 展示 task 节点状态 + context 时间线

**关键约定：**
- `templateIdForSkill(name,version)` = `wf-{name}-{version-with-dashes}`
- `projectModeForSkill`：aigc-shot-video / video-creator → `aigc_shot`，其余 → `voice_visual`
- `generationMode` 固定 `manual_import`（当前 MVP 不接视频 API）
- stage 中文名映射表 `stageNameMap`（覆盖全部 6 个 Skill 的所有 stage）

### 9.3 组件清单（`components/`）

| 组件 | 用途 | 所属页面 |
|------|------|---------|
| `Sidebar` | 侧边导航（4 项 + Runtime 说明卡） | 全局 |
| `CreatorWorkbenchPage` 内联组件 | PanelHeader/RouteCard/StatusPill/Metric/Skeleton/EmptyState/ArtifactWorkbench/ArtifactReviewModal/ArtifactRenderer(MarkdownDocument/ImageArtifact/VideoArtifact/AudioArtifact/ReadableArtifact/PublishCopyView)/TraceModal | 创作台 |
| `AIAssistantTab` | 对话助手面板（多轮 chat） | 发布相关（旧） |
| `UploadCard` | 拖拽上传 | 发布相关（旧） |
| `MediaLibraryPanel` | 媒体库筛选 | 发布相关（旧） |
| `PlatformSelector` | 10 平台多选 | 发布相关（旧） |
| `BlockingOverlay` | AI 操作全屏 loading | 发布相关（旧） |
| `ContentTypeSelector` `TitleInput` `DescriptionInput` `KeywordInput` `AIHelperPanel` `PublishButton` | 发布表单组件 | 发布相关（旧） |
| `DesktopToolbar` `CommandPanel` | 桌面命令执行 | DesktopPage |

> 说明：`appStore.ts`（Zustand）仍保留发布相关状态（title/description/keywords/videos/images/platforms/contentType/chatSessionId），但创作台是自管理 state，不复用 store。

### 9.4 API 服务（`services/api.ts`）

axios 实例，`API_BASE` 按运行环境配置：桌面包优先读取 `VITE_CLOUD_API_BASE` / `TANGYING_CLOUD_API_BASE` 指向云端 `/api`，Web 开发默认走 `/api`（Vite proxy）。本机能力不走这个 axios 实例，而是通过 Electron IPC / `127.0.0.1:18080` 调用 `local-backend`。基础模型 API 设置也走本地 agent，保存用户自配的 OpenAI-compatible `baseUrl/apiKey/model`，不进入云端数据库。按模块分组：

- **发布/AI**：publishContent / aiGenerateContent / aiGenerateFromMedia / aiPolishText / aiPolishSubmit / queryPolishResult
- **对话**：createSkillSession / chatSkillSession / getSkillSession(Progress) / terminateSkillSession（调 `/api/chat/sessions/*`）
- **媒体**：fetchMediaList / fetchMediaDetail / updateMediaTags / uploadMedia / batchProcessMedia
- **Skill/视频**：fetchSkills / fetchSkillCatalog / fetchSkillDetail / routeSkill / fetchWorkflows / fetchVideoProjects / createVideoProject / createWorkflowRun / fetchProjectArtifacts / fetchArtifact(Content/History) / reviseArtifact
- **节点/任务**：succeedNode / failNode / failTask / recordContextEvent / fetchTrace / fetchRecentTrace

### 9.5 Electron（`frontend/electron/`）

| 文件 | 职责 |
|------|------|
| `main.cjs` | 主进程：建 1400×960 窗口；dev 加载 `:3000`，prod 加载 `frontend/dist/index.html`；打包后自动启动 `resources/bin/tangying-local-agent` |
| `preload.cjs` | contextBridge 暴露 `window.electronAPI`：本地命令/文件对话框/本地 agent 健康检查/运行时配置 |
| `frontend/package.json` | electron-builder 配置：appId `com.tangying.aios.desktop`，输出 `release/`，额外打包本地 agent 二进制到 `resources/bin` |

**边界：** Electron 只负责桌面壳层和用户授权的本地交互；数据库和云端服务凭证不进入本地包。用户自配的模型 Provider token 只保存在本机 `config/model-providers.json`，本地 agent 负责本机缓存、日志、文件处理、诊断包生成和桌面端基础模型直连；云端 API 负责远程配置、账号、外部服务元数据和云端日志。

---

## 10. 基础设施与事件机制

### 10.1 Kafka Topics

| Topic | 生产者 | 消费者 |
|-------|--------|--------|
| `ai.node.ready` | StateService（节点就绪） | Worker（NodeExecutor） |
| `ai.node.result` | Worker（执行结果） | Orchestrator（StateMachine + DependencyChecker）+ Context |
| `ai.progress` | Worker（heartbeat/progress/checkpoint） | ProgressConsumer（更新 DB + 审计） |
| `ai.task.completed` / `ai.task.failed` | StateService | （外部观察） |

> ⚠️ 旧文档里的 `ai.node.executed` / `ai.node.failed` 已合并进 `ai.node.result`（用 Status 字段区分）。

### 10.2 消费者组

- `ai-worker-group` — 消费 ready，执行节点（每事件 `go nodeExecutor.ExecuteNode`）
- `orchestrator-group` — 消费 result，驱动状态机
- `ai-progress-group` — 消费 progress，更新心跳/进度/断点
- `ai-context-group` — 消费 result/failed，记审计

### 10.3 Outbox 可靠投递

`SaveEvent` 先写 `outbox` 表 → `Relay`（100ms ticker，`DefaultRelayConfig()`）读 pending 事件（`SELECT ... FOR UPDATE` 行锁，防并发重复投递）→ 发 Kafka → 成功后删除。投递失败按指数退避重试（`IncrementRetry`），超过最大重试次数的事件移入死信队列（`MoveToDLQ`）。

**接口抽象（`interfaces.go`）：**
- `EventSaver` — 事件写入接口
- `EventPublisher` — Kafka 发布接口（`eventbus.Producer` 实现）
- `OutboxStore` — DB 操作接口（`FetchPending` / `Delete` / `IncrementRetry` / `MoveToDLQ`），支持内存 mock 实现，便于单元测试

**测试覆盖：** `relay_test.go`（~600 行），覆盖重试、DLQ、优雅关闭等场景。

### 10.4 幂等

`idempotencyKey = taskId + "-" + nodeId`，作为 Kafka message key 去重；状态转换前检查「已在目标状态则跳过」。

### 10.5 HyperFrames Render Service（渲染服务）

**独立的 Node.js/TypeScript 服务**，提供 HTTP API 来程序化调用 HyperFrames 渲染管线，**不再依赖 `npx hyperframes` CLI**。

**目录：** `hyperframes-render-service/`

**API：**

| 端点 | 方法 | 作用 |
|------|------|------|
| `/health` | GET | 就绪检查（Node/FFmpeg/Chromium/Producer 状态） |
| `/render` | POST | 同步渲染：接收 `projectDir`/`outputPath`/`fps`/`quality`/`format` → 调 `@hyperframes/producer` → 返回 MP4 |
| `/lint` | POST | 项目检查：扫描 `index.html` 禁止非确定性模式（Date.now/Math.random/fetch） |
| `/snapshot` | POST | 关键帧快照：在指定时间点截取 PNG（Phase 2） |
| `/render/stream` | POST | SSE 流式进度推送（Phase 2） |
| `/jobs/:jobId` | GET | 查询任务状态 |

**安全：** `projectDir`/`outputPath` 白名单校验，只允许访问配置的 `HYPERFRAMES_PROJECT_ROOT` / `HYPERFRAMES_OUTPUT_ROOT`。

**Go 客户端：** `cloud-backend/internal/core/hyperframes/` 封装 HTTP 调用，AIOS 工具通过 `Client.Render()` 触发渲染。

**配置：** `cloud-backend/.env` 中设置：
```env
HYPERFRAMES_MODE=service            # disabled | service
HYPERFRAMES_SERVICE_URL=http://127.0.0.1:8787
HYPERFRAMES_TIMEOUT_SEC=1800
HYPERFRAMES_DEFAULT_FPS=30
HYPERFRAMES_DEFAULT_QUALITY=standard
HYPERFRAMES_DEFAULT_FORMAT=mp4
HYPERFRAMES_MAX_WORKERS=4
HYPERFRAMES_USE_GPU=false
HYPERFRAMES_PROJECT_ROOT=/data/aios/projects
HYPERFRAMES_OUTPUT_ROOT=/data/aios/projects
```

**运行方式：**
```bash
# 开发模式
cd hyperframes-render-service && npm install && npm run dev

# Docker 模式
docker build -t hyperframes-render-service .
docker run -p 8787:8787 -v /data/aios/projects:/data/aios/projects hyperframes-render-service
```

---

## 11. 安装、运行与云端部署

### 11.1 环境要求

| 工具 | 版本 | 安装 |
|------|------|------|
| Go | 1.25+ | `brew install go` |
| Node.js | 18+ | `brew install node` |
| Docker | 20+ | `brew install --cask docker` |
| Rust（可选，沙箱） | stable | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| protoc（可选） | 3.x | `brew install protobuf` |

### 11.2 本地桌面开发启动

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

本地启动不需要 PostgreSQL、Redis、Kafka、MinIO 或 Docker。需要云端能力时，通过 `VITE_CLOUD_API_BASE` / `TANGYING_CLOUD_API_BASE` 指向云端 API。

### 11.3 启用云端视频创作

`cloud-backend/.env` 里：
```bash
VIDEO_CREATION_ENABLED=true
MODEL_PROVIDER_MODE=fake          # 测试用 fake（无需视频 API）；生产用 real
SKILL_ROOT=skills                 # 默认即 cloud-backend/skills（legacy Skills）
SKILL_CAPABILITY_ROOT=skill-capabilities  # 🆕 Skill Capability 根目录（Dynamic Agent 工具注册表）
LEGACY_SKILL_WORKFLOW_AUTOREGISTER=false  # 🆕 是否自动将 legacy Skill 注册为 Workflow 模板
LOCAL_RUNNER_ENABLED=false        # 云端不启动桌面本地 Runner
```

### 11.4 云端后端启动（开发）

```bash
cd cloud-backend
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

### 11.5 云端部署（`deploy/`）

```bash
cd cloud-backend/deploy
cp .env.cloud.example .env.cloud   # 填 OPENAI_API_KEY / 密码
docker compose -f docker-compose.cloud.yml up -d
```

`docker-compose.cloud.yml` 启动 6 个服务：

| 服务 | 镜像 | 端口 |
|------|------|------|
| nginx | nginx:alpine | 80/443（反代 + SPA fallback + SSE 长连） |
| backend | 本地 Dockerfile | 8080（healthcheck: `/api/health/ready`） |
| postgres | postgres:16-alpine | 5432 |
| redis | redis:7-alpine | 6379 |
| redpanda | redpandadata/redpanda | 9092/19092 |
| minio | minio/minio | 9000/9001 |

`nginx.conf` 要点：`/api/` 反代到 backend（500M body / 300s 超时，视频上传友好）；`/api/progress/stream` 关闭缓冲支持 SSE（86400s）；`/` SPA fallback。

> 云端默认开 `VIDEO_CREATION_ENABLED=true` / `MODEL_PROVIDER_MODE=real`，并保持 `LOCAL_RUNNER_ENABLED=false`。本地执行能力由 `local-backend` 提供。

### 11.6 前端 / 桌面构建

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

---

## 12. 开发工作流与测试

### 12.1 Makefile / 常用命令

```bash
make build     # 编译后端
make run       # 编译 + 运行
make test      # 跑测试
go test -race ./...   # 全量 + 竞态
make sandbox-build    # 构建 Rust 沙箱
./scripts/test-apis.sh   # API 集成测试（在 cloud-backend/ 下执行）
```

### 12.2 测试覆盖现状

- ✅ 有测试：artifact、skillruntime、modelgateway、localrunner、config、video model/service、workflow compiler/run、skill router、retry_policy、dag_validator、node executor、video creation external tools
- ⚠️ 待补：Repository 层（需 PostgreSQL）、Handler 层 HTTP 集成、workflow 完整 rerun 流程

### 12.3 新增一条业务线的步骤

1. 在 `skills/{name}/1.0.0/` 写 `skill.yaml` + `stages/*.md`（可选 schemas）
2. （可选）在 `internal/agents/{name}/` 写领域 Agent（model/repository/service/handler），复用 orchestrator/workflow
3. 在 `main.go` 注册 handler（feature-gated）
4. 启动时自动加载 Skill → 编译 → 注册为 workflow 模板
5. Skill Router 会自动把它纳入 LLM 路由候选（category=video 且 healthy）

---

## 13. 改进与优化建议

按「投入产出比 / 严重程度」排序。

### 🔴 P0 — 文档与正确性（低投入、高收益）

1. **修正过时的 AGENTS.md / CLAUDE.md**
   - 当前 AGENTS.md（25KB）描述的是重构前的扁平 `internal/*` 结构，实际已迁移到 `internal/core/*` + `internal/agents/*`。所有路径、包名、模块说明都需更新，否则会严重误导新人。
   - CLAUDE.md（519 行）同样需校对。
   - **建议**：本架构文档作为权威源，AGENTS.md/CLAUDE.md 精简为「快速上手 + 指向本文档」。

2. **修复 README 的失效链接**
   - `docs/AIOS_CORE_BACKEND_BOUNDARY.md` 不存在；「2 个 Skill」实为 6 个；「14 个内置工具」需核对。

3. **API 路径文档与代码对齐**
   - 对话接口实际是 `/api/chat/sessions/*`，旧文档写 `/api/skill/dialog/session/*`；Kafka topic `ai.node.executed/failed` 已合并为 `ai.node.result`。以代码为准统一。

### 🟠 P1 — 架构一致性

4. **让 Model Gateway 真正接入主流程**
   - Gateway 已实现（指纹缓存 + 重试 + Provider 路由 + fake），但 `main.go` 未使用，各工具仍直连 `cfg.OpenAI`。接入后可统一观测（`model_calls` 表已建好但没写入）、统一重试、统一 mock。这是「写了没用」的最大一块技术债。

5. **统一人工审核入口**
   - 视频域已提供 `/api/video-projects/:id/stages/:stage/approve` 领域封装，前端不再直接猜测或操控底层节点 ID；底层 `/api/node/:id/success` 仍保留为 Core 通用接口。后续可继续把标书、视频等领域审核响应体和错误语义收敛成统一规范。

6. **local-backend 完成本地执行器工具适配**
   - 云端 EdgeRun 控制面已经接线：runner 注册/心跳/claim/progress/complete/fail、`executionPlane=local` 派发和 DAG 回调已可用。下一步应在 `local-backend` 补齐 FFmpeg、HyperFrames、Hypergen、ASR、文件导入等语义命令执行器，以及路径授权和本地 artifact 同步策略。

7. **完成本地直连模型 Provider 的执行链路**
   - 当前云端 artifact/materializer、node result event、`ai_node.output` / `ai_task.output` 已只保留本地 manifest、hash、size、trace 和脱敏摘要；桌面端 text_to_text/text_to_image/text_to_video 的 Provider 配置已下沉到本地 agent，下一阶段应把实际 LLM/图片/视频执行器也完全切到本地直连基础模型服务商，云端仅提供远程配置、额度/策略、日志索引和错误诊断，不经手原始用户正文或二进制数据。

### 🟡 P2 — 可靠性与可观测

8. **引入正式 DB 迁移工具**
   - 现在用 `CREATE TABLE IF NOT EXISTS` + `ALTER ADD COLUMN IF NOT EXISTS` 全堆在 `RunMigrations` 里（已 ~400 行）。建议上 `golang-migrate` 或 `goose`，支持版本回退、CI 校验、生产灰度。

9. **SSE 实时进度推送**
   - nginx 已为 `/api/progress/stream` 关闭缓冲，但后端该端点未实现，前端只能 3s 轮询 artifacts。实现 SSE 后体验和负载都更好（KNOWN_LIMITATIONS 也提到）。

10. **Outbox Relay 单点 + 可观测**
   - Relay 是单 goroutine，挂了事件就停。建议加 metrics（积压数、延迟）+ 多实例时用 `SELECT FOR UPDATE SKIP LOCKED` 分片。

11. **长任务心跳超时的恢复路径**
    - Scheduler 能检测 `HEARTBEAT_TIMEOUT`，但「超时后如何续跑 / 断点恢复」尚不完整（artifact materialize 只是读，不重跑）。结合 `checkpoint` 做真正的断点续传。

### 🟢 P3 — 前端与工程化

12. **清理发布相关遗留代码**
    - `PublishPage.tsx` 已不被引用，`appStore.ts` 的发布状态创作台也不用。要么恢复发布入口，要么删除降低维护噪音。

13. **补全占位页 + 引入路由**
    - `projects` / `skills` 两个 nav 是占位。随着功能增长，建议引入 React Router（现在用 state 切换，刷新即丢），并把创作台的状态拆到 store。

14. **前端测试体系**
    - 当前 0 前端测试。优先给 `CreatorWorkbenchPage` 的路由/启动/审核三个核心流程加 Vitest + RTL；产物渲染逻辑（mediaUrl 提取）是纯函数，单测性价比高。

15. **产物 materialize 时机**
    - 现在「每次 GET artifacts 都遍历所有 run 的所有成功 node 尝试落库」，规模上去会放大。建议改成节点成功事件触发增量 materialize（worker 发事件 → consumer 写 artifact）。

### 🔵 P4 — 长期演进

16. **多租户与权限**：当前单用户模式（user_id 默认 `default`），上 SaaS 需补租户隔离 + RBAC。
17. **Qdrant 向量库启用**：基础设施已起但未用，可用于媒体语义检索 / Skill 推荐。
18. **视频 API Provider 适配**：Seedance / GPT-Image 等外部视频/图片 API 适配器待补（Gateway 的 Capability 已预留 `text_to_video` / `image_to_video`），补齐后可从 `manual_import` 切到 `provider_api` 全自动。
19. **沙箱能力增强**：当前 Rust 沙箱用 setrlimit（同机隔离弱），生产可演进到 Firecracker microVM 或容器化执行器。

---

> 本文档基于截至 2026-06-21 的代码现状（分支 `develop_go`）撰写，所有路径、接口、表结构均经源码核对。如代码与本文档冲突，**以代码为准**并及时回更本文档。
