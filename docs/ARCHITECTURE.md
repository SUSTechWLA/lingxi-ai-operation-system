# 系统架构设计

躺营AI OS 的完整架构设计、数据流、组件关系和设计决策。

---

## 目录

1. [整体架构](#1-整体架构)
2. [模块设计](#2-模块设计)
3. [前端架构](#3-前端架构)
4. [数据模型](#4-数据模型)
5. [事件系统](#5-事件系统)
6. [可靠性设计](#6-可靠性设计)
7. [配置系统](#7-配置系统)
8. [日志系统](#8-日志系统)
9. [扩展点](#9-扩展点)
10. [基础设施依赖](#10-基础设施依赖)

---

## 1. 整体架构

### 1.1 架构全景图

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                        浏览器 (React SPA)                                               │
│  http://localhost:3000                                                               │
│  ┌──────────────────────────────────────────────────────────────────┐                │
│  │  App.tsx (主入口)                                                  │                 │
│  │  ┌─────────────────────────────────────────────────────────────┐  │               │
│  │  │  PublishPage (创作发布页面)                                  │  │                    │
│  │  │  ┌──────────┐ ┌───────────┐ ┌─────────┐ ┌──────────────┐ │  │                  │
│  │  │  │上传素材   │ │标题/简介  │ │AI助手    │ │平台选择/发布 │ │  │                                │
│  │  │  │UploadCard│ │TitleInput │ │AIHelper  │ │PlatformSelect│ │  │                 │
│  │  │  │          │ │DescInput  │ │ContentType│ │PublishButton │ │  │                 │
│  │  │  └──────────┘ └───────────┘ └─────────┘ └──────────────┘ │  │                  │
│  │  └─────────────────────────────────────────────────────────────┘  │               │
│  │  Zustand Store (状态管理)                                          │                  │
│  │  Axios Client → Vite Proxy → /api/*                                │              │
│  └──────────────────────────────────────────────────────────────────┘                │
└──────────────────────────┬───────────────────────────────────────────────────────────┘
                           │ proxy /api/* (Vite config)                                 
                           ▼                                                            
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                                   Go 单进程 (端口 8080)                                   │
│                                                                                      │
│  ┌───────────────────────────────────────────────────────────────────────────────┐   │
│  │                                 Gin HTTP 路由层                                  │   │
│  │    /api/health | /api/publish | /api/media/* | /api/ai/*                       │   │
│  │         /api/task/* | /api/translate/* | /api/node/* | /api/context/*         │   │
│  └────────────────────────────────────────────────────┬──────────────────────────┘   │
│                                                       │                              │
│  ┌──────────────┐  ┌────────────────────┐  ┌──────────▼────────┐  ┌───────────────┐  │
│  │  Publish 模块  │  │    Media 素材管理模块    │  │   Orchestrator    │  │    Worker     │  │
│  │   (用户交互层)    │  │    · MinIO 对象存储    │  │       调度引擎        │  │     工具执行      │  │
│  │              │  │     · 素材 CRUD      │  │                   │  │               │  │
│  │  PublishSvc  │  │                    │  │   StateService    │  │ NodeExecutor  │  │
│  │ TraceHandler │  │                    │  │   StateMachine    │  │ ToolRegistry  │  │
│  │ AI Generate  │  │                    │  │ DependencyChecker │  │ 11 个内置工具   │  │
│  │  AI Polish   │  │                    │  │     Scheduler     │  │ + 外部工具代理  │  │
│  └──────┬───────┘  └─────────┬──────────┘  └──────┬────────────┘  └──────┬────────┘  │
│         │                     │                    │                      │          │
│  ┌──────▼─────────────────────┴────────────────────┴──────────────────────┴────────┐ │
│  │                           Skill — AI 对话助手模块                                 │  │
│  │  PlanService (LLM → DAG) · SessionManager (Redis) · ResultAssembler (轮询)     │  │
│  │  ToolManifestService (DB+Redis缓存工具知识库)                                      │  │
│  └────────────────────────────────────────┬────────────────────────────────────────┘ │
│         │                     │                    │                      │          │
│  ┌──────▼─────────────────────▼────────────────────▼──────────────────────▼────────┐ │
│  │                                  Context 审计服务                                  │  │
│  │                                 事件记录 · 快照 · 恢复                                 │  │
│  └────────────────────────────────────────┬────────────────────────────────────────┘ │
│                                           │                                          │
│  ┌────────────────────────────────────────▼────────────────────────────────────────┐ │
│  │                                EventBus (Kafka)                                │  │
│  │                       Producer · Consumer · Topic Router                       │  │
│  └────────────────────────────────────────┬────────────────────────────────────────┘ │
│                                           │                                          │
│  ┌────────────────────────────────────────▼────────────────────────────────────────┐ │
│  │                                  Outbox 发件箱模式                                  │  │
│  │                     DB 先写 -> Relay 协程 100ms 轮询 -> Kafka 转发                     │  │
│  └────────────────────────────────────────┬────────────────────────────────────────┘ │
│                                           │                                          │
│                    ┌───────────┐  ┌──────────▼──────────┐  ┌──────────┐              │
│                    │  Config    │  │   Database          │  │   Redis   │            │
│                    │  (Viper)   │  │   (pgx Pool)       │  │ (go-redis)│             │
│                    └───────────┘  └─────────────────────┘  └──────────┘              │
└──────────────────────────────────────────────────────────────────────────────────────┘
         │                       │                       │
    ┌────▼────┐          ┌───────▼───────┐        ┌─────▼─────┐
    │  .env   │          │  PostgreSQL    │        │   Redis    │
    │ 配置文件  │          │  16            │        │   7        │
    └─────────┘          └───────────────┘        └───────────┘
                                │
                          ┌─────▼──────┐
                          │  Redpanda   │
                          │  (Kafka)    │
                          └────────────┘
```

### 1.2 单体模块化架构

项目使用**单体模块化架构**（Modular Monolith），所有功能编译为一个 Go 二进制文件：

| 优点 | 说明 |
|------|------|
| 简单 | 单项目、单部署、单端口 |
| 高效 | 模块间 Go 函数调用，零网络开销 |
| 清晰 | Go 包目录即为模块边界 |
| 可拆分 | 未来可沿包边界拆为微服务 |

---

## 2. 模块设计

### 2.1 Publish — 用户发布模块

**职责**：处理用户发布内容的全部流程，是用户最直接使用的模块。

**组件**：

```
PublishHandler (HTTP 路由)
├── POST /api/publish              → 提交发布任务
├── POST /api/ai/generate           → AI 生成标题+简介
├── POST /api/ai/generate-from-media → 基于图片/视频的 AI 生成（含视频 pipeline）
├── POST /api/ai/polish             → AI 润色文字（同步）
├── POST /api/ai/polish/submit      → 提交异步润色任务
├── GET  /api/ai/polish/result      → 查询异步润色结果
├── GET  /api/trace/recent          → 查询最近一次任务追踪
├── GET  /api/trace/:taskId         → 查询指定任务追踪

MediaHandler (HTTP 路由)
├── POST /api/media/upload          → 上传素材
├── GET  /api/media/list            → 素材列表（分页+标签筛选）
├── GET  /api/media/:id             → 素材详情
└── PUT  /api/media/:id/tags        → 更新素材标签

PublishService (业务逻辑)
├── PublishContent()    → 创建 DAG 任务，使用 LLM 润色后发布
├── AIGenerateContent() → 调用 LLM 从文字想法生成标题/简介
├── AIGenerateFromMedia() → 根据上传的图片/视频+描述生成内容（含视频 pipeline）
├── AIPolishText()      → 调用 LLM 润色指定文本（同步）
├── AIPolishSubmit()    → 提交异步润色任务（返回 taskId+nodeId）
├── AIPolishQuery()     → 查询异步润色结果
└── callOpenAI()        → 通用 OpenAI API 调用封装

TraceHandler (追踪查询)
├── GET /api/trace/recent    → 查询最近一次任务的完整链路数据（任务详情 + 上下文列表）
└── GET /api/trace/:taskId   → 查询指定任务的完整链路数据

PublishRequest → PublishResponse 流程：
  1. 用户提交表单（标题 + 简介 + 关键词 + 平台 + 媒体文件）
  2. 创建 DAG 任务（polish_content → log_result）
  3. 提交到 Orchestrator
  4. 返回 taskId 给用户
```

**PublishRequest 数据结构**：

```go
type PublishRequest struct {
    Title       string                   // 标题
    Description string                   // 简介
    Body        string                   // 正文
    Keywords    string                   // 关键词（逗号分隔）
    Platforms   []string                 // 目标平台（["douyin", "xiaohongshu"]）
    ContentType string                   // 内容类型（"image" / "video"）
    CoverFile   *multipart.FileHeader    // 封面图片文件
    VideoFiles  []*multipart.FileHeader  // 视频文件
    ImageFiles  []*multipart.FileHeader  // 图片文件
}
```

**标准 API 响应格式**：

```json
{
  "code": 200,
  "message": "success",
  "data": { ... }
}
```

### 2.2 Orchestrator — 调度引擎

**职责**：接收 DAG 任务、管理状态流转、驱动依赖调度。

**核心组件**：

```
OrchestratorService
├── DAGValidator         验证 DAG 合法性（环检测、重复节点、引用完整性）
├── StateService         统一状态转换入口
│     ├── 状态转换        Task/Node 状态变更 + outbox 事件发布
│     ├── 依赖检查        判断节点依赖是否全部满足（SUCCESS + SKIPPED 均满足）
│     ├── TryMakeReady   节点创建后立即检查依赖，满足则转 READY（事件驱动）
│     └── 任务完成判断     判断所有节点是否成功或跳过
├── StateMachine         节点成功/失败状态机
│     ├── onSuccess      转换→SUCCESS, outbox 发布事件, 检查任务完成
│     └── onFailure      判断重试 or 永久失败
├── DependencyChecker    依赖驱动调度
│     ├── onNodeExecuted 上游完成→检查下游→评估条件→标记 READY→outbox 发布
│     └── evaluateCondition 条件评估（"nodeId.status == success" 格式）
├── Scheduler            兜底恢复（30s 间隔，仅处理停滞 >1 分钟的 CREATED 节点）
├── RetryPolicy          指数退避（1s→2s→4s→...→60s）
└── TaskExecutionControl  暂停/恢复/重试控制（持久化 pause_reason）
```

**调度流程**：

```
用户请求 → Handler → Service → 创建任务 → 提交 DAG
                                              │
                                        保存 Node + Edge 到 DB
                                              │
                                        TryMakeReady → READY
                                              │
                                        outbox → Kafka → Worker 消费
                                              │
                                        执行结果 → StateMachine
                                              │
                                        DependencyChecker → 下游节点 → ...
                                              │
                                        全部完成 → 任务结束
```

**条件分支机制**：

节点的 `condition` 字段（如 `"nodeA.status == success"`）实现条件分支：
- 满足条件 → 节点正常执行
- 不满足条件 → 节点标记为 SKIPPED（不影响下游）

```
  [节点A: 调用API] ──→ [节点B: 解析结果, condition="A.status == success"]
       │                        │
       │                    A成功 → B执行
       │                    A失败 → B跳过 (SKIPPED)
       │
       └──→ [节点C: 错误通知, condition="A.status == failed"]
                    │
               A成功 → C跳过 (SKIPPED)
               A失败 → C执行
```

### 2.3 Worker — 工具执行引擎

**职责**：消费节点就绪事件，调用工具执行，发布结果。

**工具接口**：

```go
type Tool interface {
    Name() string                                            // 唯一标识
    Description() string                                     // 描述
    Type() ToolType                                          // LLM / CUSTOM
    Execute(ctx, params, toolCtx) ToolResult                 // 执行
    ValidateParameters(params map[string]interface{}) bool    // 参数校验
}
```

**已实现工具**：

| 工具名 | 类型 | 用途 | 说明 |
|--------|------|------|------|
| `llm_api` | CUSTOM | AI 调用 | 调用 OpenAI 兼容 API 进行文本生成 |
| `bash` | CUSTOM | Shell 执行 | 沙箱执行 Shell 命令（白名单+危险过滤） |
| `python` | CUSTOM | Python 执行 | python3 -c 执行，资源限制 |
| `polisher` | CUSTOM | 文本润色 | 调用 LLM 对标题或简介进行润色优化 |
| `media_analyzer` | CUSTOM | 素材分析 | 分析图片/视频，输出标签和建议 |
| `content_generator` | CUSTOM | 内容生成 | 基于素材分析生成完整内容包（标题、简介、脚本、标签） |
| `content_checker` | CUSTOM | 合规检查 | 检测敏感词、极限词、平台规范违规 |
| `platform_adapter` | CUSTOM | 平台适配 | 将内容适配到抖音、小红书、B站等平台的风格 |
| `chat_generate` | CUSTOM | 对话生成 | 多轮对话式内容生成，支持完整消息历史 |
| `chat_revise` | CUSTOM | 内容修改 | 根据自然语言指令修改标题/简介/关键词 |
| `external` | CUSTOM | 外部工具代理 | 代理执行通过 `/api/tools/register` 注册的外部 HTTP 工具 |
| `video_metadata` | CUSTOM | 视频元数据 | 从 MinIO 下载视频，提取时长/分辨率/帧率/编码/音频信息，缓存到本地 |
| `video_analyzer` | CUSTOM | 视频分析 | ffmpeg 场景检测提取关键帧（base64）+ Whisper 音频转录 |
| `video_copy_generator` | CUSTOM | 短视频文案 | 基于元数据+关键帧+转录，调用多模态 LLM 生成平台适配的标题/文案/关键词 |

**BashTool 安全**：命令白名单 + 危险模式过滤 + `/tmp/ai-sandbox` 沙箱目录。

**执行监控**：Worker 在每个节点执行时自动记录以下指标到事件输出中：

| 字段 | 说明 | 示例 |
|------|------|------|
| `startedAt` | 节点开始执行的时间戳 | `"2026-04-26T23:13:21.048766+08:00"` |
| `durationMs` | 执行耗时（毫秒） | `12543` |
| `exitCode` | 进程退出码 | `0` |
| `error` | 错误信息（失败时） | `"command not found"` |
| `resourceUsage` | 资源使用统计（沙箱执行时记录 CPU 时间、内存峰值） | `{"cpuTimeUsec": 12345, "memoryPeakBytes": 67108864}` |

这些指标通过 Kafka 事件传递到 Context 服务，存入 `ai_context.metadata` 字段，供审计和调试使用。

### 2.4 NL-Translator — 自然语言翻译器

**职责**：将自然语言转为 DAG 任务图。

通过 System Prompt 引导 LLM 将用户输入（如"执行命令并生成总结"）转为结构化的节点和边。

### 2.5 Context — 上下文审计

**职责**：记录所有状态变更事件，提供快照恢复能力。

**上下文记录结构**：

```go
type Context struct {
    ID           int64                  // 自增主键
    ContextType  ContextType            // 事件类型
    TaskID       string                 // 关联任务 ID
    NodeID       string                 // 关联节点 ID（可选）
    SourceModule string                 // 来源模块（Orchestrator / StateMachine / ContextService）
    SourceTopic  string                 // 来源 Kafka Topic（仅 ContextService 消费的事件有值）
    Metadata     map[string]interface{} // 元数据（含 inputPreview/outputPreview/执行指标等）
    Message      string                 // 可读描述（纯文本，不含模块前缀）
    SnapshotData map[string]interface{} // 快照数据
    CreatedAt    time.Time              // 记录时间
}
```

**上下文生产来源**：

| 来源模块 | 写入方式 | 记录的上下文类型 |
|---------|---------|----------------|
| OrchestratorService | 直接 DB 写入 | TASK_CREATED, DAG_VALIDATED, DAG_SUBMITTED |
| StateMachine | 直接 DB 写入 | NODE_READY, NODE_SUCCESS, NODE_FAILED, NODE_SKIPPED, TASK_SUCCESS, TASK_FAILED |
| ContextService | 从 Kafka 事件消费 | NODE_SCHEDULED, NODE_SUCCESS, NODE_FAILED（从 ai.node.result 和 ai.node.failed 主题） |

**执行监控数据提取**：ContextService 在消费 Kafka 事件时，从事件 Output 中提取执行指标（`startedAt`, `durationMs`, `exitCode`, `error`, `resourceUsage`）写入上下文记录的 Metadata 字段，实现审计链路中的执行性能可见性。

记录的事件类型：`TASK_CREATED`, `DAG_VALIDATED`, `DAG_SUBMITTED`, `NODE_READY`, `NODE_SCHEDULED`, `NODE_SUCCESS`, `NODE_FAILED`, `NODE_SKIPPED`, `TASK_SUCCESS`, `TASK_FAILED`, `SNAPSHOT`, `CUSTOM`。

### 2.6 Skill — AI 对话助手模块

**职责**：提供多轮对话式 AI 内容创作能力。每次对话 = 一个 Task + DAG，经 Orchestrator → Worker 执行，Context 全程追踪。

**实现状态**：已完整实现，代码位于 `internal/skill/`。

**核心组件**：

```
SessionHandler (HTTP 路由 /api/skill/dialog)
├── POST /session/create      → 创建对话会话（含页面上下文 + 媒体 presigned URL）
├── GET  /session/:id          → 获取会话状态（消息历史 + 媒体上下文 + task 列表）
├── POST /session/:id/chat     → 发送消息 → LLM 规划 DAG → 执行 → 提取字段 → 返回
├── GET  /session/:id/progress → 查询执行进度（IDLE/EXECUTING/TERMINATED）
└── POST /session/:id/terminate → 终止会话

SessionManager (Redis 会话管理)
├── CreateSession()   → 创建会话 + 欢迎消息 + 媒体上下文（含 presigned URLs）
├── GetSession()      → 读取会话（含完整消息历史）
├── SaveSession()     → 持久化会话到 Redis (TTL 30min)
├── AppendMessage()   → 追加消息（超过 50 条自动裁剪）
└── TerminateSession() → 标记会话终止

PlanService (DAG 生成)
├── GeneratePlan()    → 构建 prompt（历史 + 媒体上下文 + 工具清单）→ LLM → DAG
├── 使用 LLMClient（typed JSON schema mode）调用 OpenAI
└── 依赖 ToolManifestService 获取全量工具描述

ResultAssembler (结果轮询与提取)
├── CreateTask()      → 通过 Orchestrator 创建任务（source: "skill_assistant"）
├── SubmitDAG()       → 提交 DAG（节点 ID scope 为 {taskId}-{原始ID} 防冲突）
├── PollAndExtract()  → 轮询任务结果（2s 间隔，5min 超时）→ 提取 title/description/keywords/body
└── extractFieldsFromOutputs() → 解析 LLM tool node output JSON

LLMClient (typed OpenAI client)
├── ChatCompletion()  → 发送消息到 OpenAI，要求 JSON 响应格式
└── JSON schema 约束确保返回结构化的 DAG 定义

ToolManifestService (工具知识库)
├── SyncBuiltinTools()   → 启动时同步 builtin 工具到 DB（tool_manifests 表）
├── ListAll()            → Redis 缓存查询（TTL 5min，DB fallback）
├── FormatForPrompt()    → 格式化为 LLM DAG prompt 中的工具描述
├── RegisterExternal()   → 注册外部工具（DB + in-memory Registry + 缓存失效）
└── DeregisterExternal() → 注销外部工具
```

**数据流**：
```
用户消息 → SessionHandler.Chat
  → 加载会话历史 + 媒体上下文（presigned URLs）
  → PlanService.GeneratePlan (历史 + 媒体 + 工具清单 → LLM → DAG JSON)
  → ResultAssembler.CreateTask → Orchestrator.CreateTask
  → ResultAssembler.SubmitDAG → Orchestrator.SubmitDAG (节点 ID scope)
  → Worker 消费 ai.node.ready → 执行工具节点
  → Context 记录审计事件
  → ResultAssembler.PollAndExtract (轮询 task 状态) → 提取字段
  → 保存 assistant 回复到会话历史 → 返回 ChatResponse{reply, fields} 给前端
```

**关键设计决策**：
- 每次对话 = 1 个 Task，不把多轮对话合并到一个 task
- Node ID 在 SubmitDAG 时 scope 为 `{taskId}-{原始ID}`，防止跨 task 冲突
- LLM DAG 生成时的工具清单来自 DB（`tool_manifests` 表），不做硬编码过滤
- 媒体上下文包含 presigned URLs（24h TTL），使 LLM 可以进行多模态视觉分析
- Session TTL 30min，消息上限 50 条，超限自动裁剪早期消息

---

## 3. 前端架构

### 3.1 技术选型

| 技术 | 用途 | 版本 |
|------|------|------|
| React | UI 框架 | 18.x |
| TypeScript | 类型安全 | 5.x |
| Vite | 构建工具 + 开发服务器 | 5.x |
| TailwindCSS | 样式框架 | 3.x |
| Zustand | 状态管理 | 4.x |
| Axios | HTTP 客户端 | 1.x |
| react-dropzone | 文件拖拽上传 | 14.x |

### 3.2 组件树

```
App.tsx (主入口)
├── Sidebar.tsx                    侧边导航栏（创作发布 / 桌面工具）
└── PublishPage.tsx                创作发布主页面
    ├── UploadCard.tsx (×2)        上传素材卡（视频/图片）+ 内容类型选择
    ├── TitleInput.tsx             标题输入 + AI 润色按钮（异步 submit/poll）
    ├── DescriptionInput.tsx       简介输入 + AI 润色按钮
    ├── KeywordInput.tsx           关键词标签输入
    ├── ContentTypeSelector.tsx    内容类型选择器（视频/图文/文章）
    ├── 右侧面板
    │   ├── AIHelperPanel.tsx      AI 生成/润色控制面板
    │   ├── AIAssistantTab.tsx     AI 对话式创作面板（Skill 会话集成）
    │   ├── PlatformSelector.tsx   发布平台选择器（10 个平台）
    │   └── PublishButton.tsx      一键发布按钮
    ├── BlockingOverlay.tsx        AI 操作全屏遮罩（进度条+spinner + 取消按钮）
    ├── MediaLibraryPanel.tsx      素材库浏览侧边面板（弹出式，支持标签筛选）
    └── 调试追踪按钮（右下角浮动）        点击查询最近一次任务链路追踪
```

### 3.3 数据流

```
用户操作 → React 组件 → Zustand Store → Axios API → Vite Proxy
                                                           │
                                                    Go 后端处理
                                                           │
                                                    JSON 响应返回
                                                           │
Zustand Store 更新 ← React 组件展示结果 ← 处理响应数据
```

### 3.4 Vite 代理配置

```typescript
// vite.config.ts
server: {
  port: 3000,
  proxy: {
    '/api': {
      target: 'http://localhost:8080',  // 后端地址
      changeOrigin: true,
    },
  },
}
```

前端开发时请求 `/api/*` 自动转发到后端 `8080` 端口，无需处理跨域。

### 3.5 状态管理

使用 Zustand 管理全局状态：

```typescript
interface AppState {
  title: string          // 标题
  description: string    // 简介
	  body: string             // 正文
  keywords: string       // 关键词
  videos: MediaFile[]    // 已上传的视频
  images: MediaFile[]    // 已上传的图片
	  cover: MediaFile | null  // 封面图片
  platforms: Platform[]  // 可用发布平台
  isPublishing: boolean  // 发布中状态
	  chatSessionId: string | null  // AI 对话会话 ID
	  contentType: ContentType       // 内容类型（"image" / "video" / null）
	  aiLoadingMessage: string | null // AI 操作加载提示文本
  // ... 操作方法
}
```

### 3.6 Electron 集成

项目支持 Electron 桌面应用模式（`electron/` 目录）：
- 原生文件选择对话框
- 系统托盘
- 后端服务健康检查

通过 `isElectron()` 和 `getElectronAPI()` 检测和调用 Electron API。

---

## 4. 数据模型

### 4.1 ER 图

```
┌──────────────┐       ┌──────────────────┐
│   ai_task     │       │     ai_node       │
├──────────────┤       ├──────────────────┤
│ id (PK)      │──┐    │ id (PK)          │
│ user_id      │  │    │ task_id (FK)     │←───┘
│ status       │  │    │ type             │
│ input (JSONB)│  └───→│ name             │
│ output(JSONB)│       │ status           │
│ pause_reason │       │ input (JSONB)    │
│ created_at   │       │ output (JSONB)   │
└──────────────┘       │ error_message    │
                       │ condition        │
                       │ retry_count      │
                       │ max_retry        │
                       │ priority         │
                       │ worker_group     │
                       │ version          │
                       │ idempotency_key  │
                       │ created_at       │
                       └────────┬─────────┘
                                │
               ┌────────────────┤
               │                │
┌──────────────▼────┐  ┌──────────────────────────┐
│ ai_node_dependency│  │     ai_context            │
├───────────────────┤  ├──────────────────────────┤
│ parent_node_id(PK)│  │ id (BIGSERIAL, PK)       │
│ child_node_id(PK) │  │ context_type             │
└───────────────────┘  │ task_id                  │
                       │ node_id                  │
┌───────────────────┐  │ source_module            │ ← 来源模块
│     outbox        │  │ source_topic             │ ← 来源 Kafka Topic
├───────────────────┤  │ metadata (JSONB)         │ ← inputPreview/执行指标等
│ id (BIGSERIAL,PK) │  │ message                  │
│ aggregate_type    │  │ snapshot_data (JSONB)    │
│ aggregate_id      │  │ resource_limits          │
│ event_type        │  │ resource_usage (JSONB)   │
│ payload (JSONB)   │  │ created_at               │
│ created_at        │  └──────────────────────────┘
└───────────────────┘
	┌──────────────────────┐
	│    media_assets      │
	├──────────────────────┤
	│ id (PK)              │
	│ user_id              │
	│ original_name        │
	│ mime_type            │
	│ size                 │
	│ minio_path           │
	│ tags (JSONB)         │
	│ embedding_id         │
	│ created_at           │
	│ updated_at           │
	└──────────────────────┘
┌──────────────────────┐
│   tool_manifests     │
├──────────────────────┤
│ name (PK)            │
│ description          │
│ type                 │
│ version              │
│ endpoint             │
│ timeout_ms           │
│ parameters (JSONB)   │
│ output (JSONB)       │
│ examples (JSONB)     │
│ sandbox              │
│ created_at           │
│ updated_at           │
└──────────────────────┘
```

### 4.2 表说明

| 表 | 说明 | 关键索引 |
|----|------|---------|
| `ai_task` | 任务主表 | `id` (PK) |
| `ai_node` | 节点表 | `task_id`, `status`, `idempotency_key` (UNIQUE) |
| `ai_node_dependency` | DAG 边表 | `(parent_node_id, child_node_id)` (PK) |
| `ai_context` | 上下文审计表 | `task_id`, `node_id` |
| `outbox` | 发件箱事件表 | `id` (PK, ASC) |
| `media_assets` | 素材文件元数据表 | `user_id`, `tags` |
| `tool_manifests` | 工具注册清单表 | `name` (PK) |

---

## 5. 事件系统

### 5.1 Outbox 发件箱模式

所有 Kafka 事件先写入 `outbox` 数据库表，再由 Relay 协程异步转发到 Kafka。

**为什么需要 Outbox**：
- 保证事件不丢：outbox 写入与业务操作在同一 DB 事务中
- Kafka 不可用时事件留在 outbox，恢复后自动转发
- 转发成功后立即从 outbox 删除

### 5.2 事件主题

| Topic | 发布者 | 消费者 | 说明 |
|-------|--------|--------|------|
| `ai.node.ready` | Orchestrator (StateService) | Worker | 节点就绪，Worker 消费后执行 |
| `ai.node.result` | Worker (NodeExecutor) | Orchestrator, Context | **唯一的结果事件源**：Worker 执行完节点后发布，Orchestrator 消费处理状态流转，Context 消费记录执行监控数据 |
| `ai.node.failed` | StateMachine | Context | 节点永久失败事件，Context 记录归档 |
| `ai.node.executed` | StateMachine | DependencyChecker | 节点成功后的内部反馈事件，DependencyChecker 消费后检查下游依赖 |
| `ai.task.completed` | StateMachine | — | **已取消订阅**（任务完成通过 DependencyChecker 直接触发 TransitionTask） |
| `ai.task.failed` | StateMachine | — | **已取消订阅**（同任务完成逻辑） |

> **消费优化说明**：ContextService 当前仅订阅 `ai.node.result` 和 `ai.node.failed` 两个主题，避免因多主题重复消费导致上下文记录重复。`ai.node.result` 同时携带 NODE_SCHEDULED（RUNNING 状态）和 NODE_SUCCESS（SUCCESS 状态）两种事件，由 Worker 的 NodeExecutor 一次性发布。

### 5.3 幂等性

每个节点有 `idempotencyKey = taskId + "-" + nodeId`，作为 Kafka 消息 key，确保同一节点不会被重复执行。

---

## 6. 可靠性设计

### 6.1 事件驱动调度

节点创建后 TryMakeReady 立即检查依赖，依赖满足后毫秒级触发下游，无需轮询。

### 6.2 兜底恢复

Scheduler 30 秒扫描一次，仅处理停滞超过 1 分钟的节点。正常情况下不触发。

### 6.3 重试策略

```
指数退避: 1s → 2s → 4s → 8s → 16s → 32s → 60s (max)
最大重试次数: 默认 3 次（可通过 maxRetry 配置）
```

### 6.4 优雅关闭

收到系统信号后按序关闭：HTTP Server → Kafka Consumer → Producer → Outbox Relay → DB 连接池 → Redis 连接。

### 6.5 数据库迁移

启动时自动执行建表语句（`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`），无额外迁移工具依赖。关键历史迁移：`ai_context` 表增加了 `source_module`（VARCHAR(50)）和 `source_topic`（VARCHAR(100)）字段，以及 `resource_limits` 和 `resource_usage`（JSONB）字段。

---

## 7. 配置系统

配置通过 `.env` 文件管理，使用 Viper 加载。

**优先级**：环境变量 > .env 文件 > 默认值

所有配置项在 `internal/config/config.go` 的 `setDefaults()` 中定义默认值。

关键配置分组：

| 分组 | 变量前缀 | 说明 |
|------|----------|------|
| Server | `SERVER_*` | HTTP 服务配置 |
| PostgreSQL | `POSTGRES_*` | 数据库配置 |
| Redis | `REDIS_*` | 缓存配置 |
| Kafka | `KAFKA_*` | 消息队列配置 |
| OpenAI | `OPENAI_*` | AI 服务配置 |
| Worker | `WORKER_*` | 工具执行配置 |
| Bash | `BASH_*` | Shell 工具安全配置 |
| MinIO | `MINIO_*` | 对象存储配置 |

---

## 8. 日志系统

使用 Zap 结构化日志：

| 模式 | 触发条件 | 输出格式 |
|------|---------|---------|
| development | 默认 | 带颜色的控制台输出 |
| production | `GIN_MODE=release` | JSON 格式 |

---

## 9. 扩展点

### 9.1 添加新工具
1. 创建工具文件 `internal/worker/tool/builtin/my_tool.go`
2. 实现 `Tool` 接口（Name, Description, Type, Execute, ValidateParameters）
3. 在 `cmd/lingxi-ai-os/main.go` 中注册

### 9.2 添加新前端组件
1. 在 `frontend/src/components/` 创建组件文件
2. 在需要的位置引入
3. 更新 `components/index.ts` 导出

### 9.3 添加新 API 端点
1. 在对应 handler 中添加方法
2. 在 `RegisterRoutes` 中注册路由
3. 添加对应 service 方法

### 9.4 添加新 Kafka 事件
1. 在 `eventbus/eventbus.go` 中定义 Topic 常量
2. 使用 `outbox.SaveEvent()` 发布事件
3. 创建 Consumer 订阅

### 9.5 拆分为微服务
模块边界已清晰，每个 `internal/` 子目录可独立拆分。当前代码中已预留 URL 配置。

---

## 10. 基础设施依赖

| 服务 | 版本 | 用途 | 必须启动 |
|------|------|------|---------|
| PostgreSQL | 16 | 持久化 + Outbox 表 | **是** |
| Redis | 7 | 缓存 + Skill 会话状态 | 是（Skill 对话功能需要） |
| Redpanda | latest | 事件总线 (Kafka) | **是** |
| MinIO | latest | 对象存储（素材管理功能需要） | 是（素材管理） |
| Qdrant | latest | 向量数据库 | 否（预留） |

启动命令：`docker compose up -d`
