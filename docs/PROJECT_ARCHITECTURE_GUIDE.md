# 躺营 AIOS 项目架构图文导览

> 适用版本：v4.0 beta  
> 阅读对象：产品、研发、测试、部署同学  
> 核心结论：这是一个面向个人创作者的视频创作 Agent 系统，采用「前端桌面工作台 + 本地轻量执行器 + 云端 AIOS Core + HyperFrames 渲染服务」的分层架构。

## 1. 一句话理解

躺营 AIOS 当前不是泛办公平台，而是一条视频创作生产线：用户在导演工作台输入想法，云端 Dynamic Agent Runtime 把自然语言目标编译成可执行 DAG，本地 Runner 负责重媒体和本地文件任务，Artifact 体系记录脚本、分镜、Prompt、视频、发布包等阶段性产物，人工审核和质量门禁负责把控关键节点。

```text
选题/想法
→ 创意方案
→ 脚本/口播稿
→ 分镜/镜头计划
→ 资产与 Prompt
→ 预览/渲染
→ Artifact 审核
→ 发布包
→ 发布记录与运营复盘
```

## 2. 总体架构图

```mermaid
flowchart LR
    User["创作者 / 桌面用户"]

    subgraph Desktop["用户桌面侧"]
        FE["frontend\nReact + Electron\n导演工作台"]
        LB["local-backend\nLocal Agent / Runner\n本地文件、缓存、日志、诊断"]
        LocalStore["OS App Data\nprojects / artifacts / logs / diagnostics"]
    end

    subgraph Cloud["云端侧"]
        CB["cloud-backend\nGo AIOS Core"]
        DB["PostgreSQL\n项目、DAG、Artifact 索引、审核记录"]
        MQ["Redis / Redpanda / Outbox\n异步与可靠投递"]
        MG["Model Gateway\nLLM / 图像 / 视频 Provider"]
    end

    subgraph Render["渲染侧"]
        HF["hyperframes-render-service\nHTML/CSS/JS -> video"]
    end

    User --> FE
    FE -->|Cloud API| CB
    FE -->|Local API| LB
    LB --> LocalStore
    CB --> DB
    CB --> MQ
    CB --> MG
    CB -->|local job 协议| LB
    LB -->|lint / render / snapshot| HF
    LB -->|manifest / hash / status| CB
```

这张图体现了项目最重要的边界：云端负责“计划、编排、状态和索引”，本地负责“用户文件、重媒体处理和用户自配 Provider”。本地运行时不能依赖 PostgreSQL、Redis、Kafka、MinIO、Docker，也不能打包云端 LLM API Key。

## 3. 代码目录边界

```text
frontend/                    React + Electron 前端，默认入口是导演工作台
local-backend/               本地轻量执行器与 local-agent
cloud-backend/               Go AIOS Core，负责动态 Agent、编排、API、模型网关
hyperframes-render-service/  HyperFrames 渲染服务
skill-capabilities/          能力/技能相关配置
scripts/                     本地开发、构建、启动脚本
docs/                        架构、部署、使用、测试文档
```

关键边界规则：

- `frontend` 只做用户体验、状态展示和 API 调用，不直接承担编排决策。
- `local-backend` 只处理本地文件、缓存、日志、诊断、白名单本地命令和 Runner 任务。
- `cloud-backend/internal/core` 是通用引擎层，不写死视频业务规则。
- `cloud-backend/internal/agents/video` 是当前视频创作业务入口。
- `cloud-backend/internal/agents/publish` 是 beta 发布兼容层，后续会迁移到 `distribution`。

## 4. 前端层：导演工作台

前端由 React + Electron 组成，默认入口在 `frontend/src/App.tsx`，登录后进入 `frontend/src/pages/DirectorStudioPage.tsx`。

```mermaid
flowchart TD
    App["App.tsx"]
    Auth["AuthScreen\n登录/恢复会话"]
    Studio["DirectorStudioPage\n导演工作台"]
    API["services/api.ts\n云端 API"]
    LocalAPI["services/localAgent.ts\n本地 Local Agent API"]

    App -->|未登录| Auth
    App -->|已登录| Studio
    Studio --> API
    Studio --> LocalAPI
```

导演工作台承担的用户侧工作包括：

- 创建视频项目、选择模式和目标时长。
- 启动 Dynamic Agent Run 或项目工作流。
- 展示阶段树、运行状态、Artifact、审核入口和发布素材。
- 在 Electron 环境下检查本地服务健康状态。
- 导出发布文案 Markdown/JSON 等交付物。

前端主要调用集中在 `frontend/src/services/api.ts`，包括：

- `/api/auth/*`：登录和会话。
- `/api/video-projects/*`：视频项目、工作流、阶段审批、项目 Artifact。
- `/api/agent/runs/*`：动态 Agent 运行、追踪和 Review。
- `/api/publish`、`/api/ai/*`：beta 发布兼容接口。

## 5. 云端层：AIOS Core

云端核心位于 `cloud-backend/internal/core`，承担平台机制；视频业务位于 `cloud-backend/internal/agents/video`。

```mermaid
flowchart TD
    API["Gin API Routes"]
    Auth["auth\n认证"]
    AgentRuntime["agentruntime\nPlanner / Guard / Compiler"]
    Orchestrator["orchestrator\nDAG 调度与状态机"]
    Workflow["workflow\n模板工作流与运行"]
    SkillRuntime["skillruntime\n技能目录与编译"]
    ModelGateway["modelgateway\nProvider 路由、重试、缓存"]
    Artifact["artifact\n版本化产物索引与审核状态"]
    LocalRunner["localrunner\n本地任务协议"]
    EventBus["eventbus / outbox\n异步事件"]
    Video["agents/video\n视频项目与业务流程"]
    Publish["agents/publish\nbeta 发布兼容层"]

    API --> Auth
    API --> AgentRuntime
    API --> Workflow
    API --> Video
    API --> Publish
    AgentRuntime --> Orchestrator
    AgentRuntime --> SkillRuntime
    AgentRuntime --> ModelGateway
    Orchestrator --> Artifact
    Orchestrator --> LocalRunner
    Workflow --> Orchestrator
    Video --> Workflow
    Video --> Artifact
    Artifact --> EventBus
```

核心模块说明：

| 模块 | 责任 |
| --- | --- |
| `agentruntime` | 从自然语言生成 AgentPlan，并经过 Guard 校验、Compiler 编译为临时 DAG |
| `orchestrator` | 执行 DAG，管理节点状态、依赖、重试、暂停、恢复和 CONTROL/Review 节点 |
| `workflow` | 维护工作流模板、Workflow Run、阶段审批与 checkpoint |
| `skillruntime` | 加载 skill manifest，形成可路由、可编译的能力目录 |
| `modelgateway` | 统一模型调用入口，屏蔽 Provider 差异，提供缓存与重试 |
| `artifact` | 管理版本化产物、存储引用、审核状态、stale 级联 |
| `localrunner` | 云端与本地 Runner 的注册、心跳、领任务、回传进度协议 |
| `eventbus/outbox` | 可靠事件投递和异步任务衔接 |

## 6. Dynamic Agent Runtime

Dynamic Agent Runtime 是项目 v4 架构的主心脏。它把用户的一句话变成可执行 DAG。

```mermaid
sequenceDiagram
    participant U as 用户
    participant FE as 前端导演工作台
    participant AR as Agent Runtime
    participant P as LLMPlanner/HeuristicPlanner
    participant G as PlanGuard
    participant C as PlanCompiler
    participant O as Orchestrator
    participant R as Review/Artifact

    U->>FE: 输入视频创作目标
    FE->>AR: POST /api/agent/runs
    AR->>P: GeneratePlan(message, context)
    P-->>AR: AgentPlan
    AR->>C: PreparePlan 注入知识上下文
    AR->>G: ValidatePlan 工具、参数、风险、阶段约束
    G-->>AR: 通过或拒绝
    AR->>C: Compile(plan)
    C-->>AR: Transient DAG
    AR->>O: CreateTask + SubmitDAG
    O->>R: 执行产物生成、质量门禁、人工审核
    FE->>AR: GET trace / reviews
    U->>FE: 审核、驳回、编辑或重新生成
    FE->>AR: approve / reject / submit-edited / regenerate
```

关键代码入口：

- `cloud-backend/internal/core/agentruntime/runner.go`：`Start` 串联 Planner、Guard、Compiler、Orchestrator。
- `cloud-backend/internal/core/agentruntime/planner.go`：启发式 Planner，会根据领域和工具清单生成步骤。
- `cloud-backend/internal/core/agentruntime/llm_planner.go`：LLM Planner 路径。
- `cloud-backend/internal/core/agentruntime/plan_guard.go`：校验工具存在性、参数、引用、成本、风险、本地能力和阶段约束。
- `cloud-backend/internal/core/agentruntime/plan_compiler.go`：注入知识上下文、质量门禁、人工 Review 节点，并编译为 DAG。
- `cloud-backend/internal/core/agentruntime/handler.go`：`/api/agent/runs` 及审核相关 API。

## 7. Plan 到 DAG 的编译模型

`AgentPlan` 是逻辑计划，`DAGRequest` 是执行计划。Compiler 会根据工具 Manifest 和审批策略自动扩展节点。

```mermaid
flowchart LR
    Plan["AgentPlan\nsteps + budget + policy"]
    Knowledge["注入 knowledgeContext\nfresh facts / sources"]
    Quality["注入质量检查\nquality checker + gate"]
    Approval["注入人工审核\nreview gate / control node"]
    DAG["Transient DAG\nTOOL nodes + REVIEW_GATE nodes"]

    Plan --> Knowledge --> Quality --> Approval --> DAG
```

常见扩展规则：

- 如果工具有质量策略，Compiler 会插入质量检查工具和质量门禁节点。
- 如果工具需要人工审批，Compiler 会插入 `REVIEW_GATE` 节点。
- 如果步骤引用上游输出，Guard 会校验 `{{step.output.field}}` 是否指向真实上游。
- 对视频领域，Stage Director 会限制某阶段能用什么工具、最多调用几次、是否必须人工审核。

## 8. 视频业务层

视频业务主要位于 `cloud-backend/internal/agents/video`。

```mermaid
flowchart TD
    Project["Video Project\n项目元数据"]
    WorkflowRun["Workflow Run\n项目工作流运行"]
    Stage["Stage Approval\n阶段审批"]
    Session["Project Session\n聚合恢复视图"]
    Artifact["Project Artifacts\n阶段产物"]
    Assistant["Project Assistant\n解释/修改/返工"]

    Project --> WorkflowRun
    WorkflowRun --> Stage
    WorkflowRun --> Artifact
    Project --> Session
    Session --> Artifact
    Session --> Stage
    Project --> Assistant
    Assistant --> Artifact
```

当前 beta 已覆盖：

- 视频项目 CRUD：`/api/video-projects`
- 项目 session 聚合视图：`/api/video-projects/:id/session`
- 工作流运行：`/api/video-projects/:id/workflow-runs`
- 阶段审批：`/api/video-projects/:id/stages/:stage/approve`
- 项目 Artifact 查询：`/api/video-projects/:id/artifacts`
- 项目助手：`/api/video-projects/:id/assistant/*`

视频项目模型包含模式、状态、技能版本、工作流版本、生成方式、画幅、目标时长、语言、本地路径提示等字段。当前支持的生产模式包括：

- `aigc_shot`：AIGC 镜头型视频。
- `voice_visual`：口播可视化视频。

## 9. Artifact 资产体系

Artifact 是系统的长期资产索引，不只是中间文件。云端保存索引、状态、版本、hash 和本地引用，本地保存正文和媒体文件。

```mermaid
stateDiagram-v2
    [*] --> pending: 生成待审核产物
    pending --> valid: 人工审核通过
    pending --> rejected: 驳回
    valid --> stale: 上游被编辑/驳回/重新生成
    stale --> pending: 重新生成
    rejected --> pending: 重新生成
    valid --> deleted: 归档/删除
```

Artifact 关键字段：

| 字段 | 含义 |
| --- | --- |
| `projectId` | 所属视频项目 |
| `stageName` | 所属阶段，如 script、storyboard、render |
| `kind` | 产物类型，如 MARKDOWN、VIDEO、FINAL_REVIEW、PROJECT_PACKAGE |
| `version` | 同阶段同单元的版本号 |
| `storageType` | 新产物归一为 `local` |
| `storageRef` | 本地存储引用 |
| `contentHash` | 内容 hash |
| `isCurrent` | 是否当前有效版本 |
| `status` | valid、stale、rejected、failed、deleted 等 |
| `humanApproved` | 是否人工确认 |
| `dependsOn` | 上游产物依赖 |

典型产物包括：

```text
creative_brief
script
voiceover_script
beat_plan
shot_list
keyframe_prompt
video_prompt
keyframe_image
video_clip
subtitle
audio
cover
final_video
publish_package
operation_report
```

当用户驳回、编辑或重新生成上游产物时，`agentruntime` 会触发 `artifact` 的 stale 级联，提醒下游脚本、分镜、视频或发布包已经过期。

## 10. 本地执行体系

本地运行时由两部分组成：

- `localagent`：提供本地 HTTP API，管理路径、日志、配置、诊断和本地 Artifact。
- `localrunner` / `localtool`：向云端注册 Runner、心跳、领取本地任务，并通过白名单命令执行。

```mermaid
sequenceDiagram
    participant C as cloud-backend
    participant LR as local-backend Runner
    participant T as localtool Registry
    participant FS as 本地文件系统
    participant HF as HyperFrames Service

    LR->>C: POST /api/local-runners/register
    LR->>C: POST /api/local-runners/:id/heartbeat
    LR->>C: GET /api/local-runners/:id/jobs/claim
    C-->>LR: local job
    LR->>T: Execute(command, payload)
    alt HyperFrames 任务
        T->>HF: lint / snapshot / render
        HF-->>T: render result
    else 文件/打包/FFmpeg 任务
        T->>FS: read/write project artifacts
    end
    LR->>C: progress / complete / fail
```

本地 API 入口在 `local-backend/internal/localagent/server.go`，包括：

- `GET /api/local/health`
- `GET /api/local/paths`
- `POST /api/local/logs`
- `GET/PUT /api/local/model-providers`
- `POST /api/local/artifacts`
- `GET/DELETE /api/local/artifacts/:id`
- `DELETE /api/local/projects/:id`
- `POST /api/local/diagnostics`
- `GET /api/local/docs`
- `GET /api/local/openapi.json`

本地命令必须经过白名单，典型命令包括：

```text
HYPERFRAMES_PROJECT_GENERATE
HYPERFRAMES_RENDER
HYPERFRAMES_LINT
HYPERFRAMES_SNAPSHOT
FFMPEG_PROBE
FFMPEG_ASSEMBLE
AUDIO_NORMALIZE
ASR_TRANSCRIBE
ARTIFACT_PACKAGE
FINAL_REVIEW
```

## 11. HyperFrames 渲染链路

HyperFrames 用于口播可视化、信息图、动效视频和发布素材生成。

```mermaid
flowchart LR
    Script["脚本 / Beat / 风格"]
    Manifest["HyperFrames project manifest"]
    Lint["lint / preview / snapshot"]
    Render["render service"]
    LocalArtifact["local artifact manifest"]
    CloudIndex["cloud artifact index"]

    Script --> Manifest --> Lint --> Render --> LocalArtifact --> CloudIndex
```

这里仍然遵守本地优先原则：视频正文和大文件留在本地，云端保存索引、hash、状态和 trace。

## 12. 发布与运营路线

当前发布层是 beta 兼容层，代码位于 `cloud-backend/internal/agents/publish`，保留接口包括：

- `POST /api/publish`
- `POST /api/ai/generate`
- `POST /api/ai/generate-from-media`
- `POST /api/ai/polish`
- `POST /api/ai/polish/submit`
- `GET /api/ai/polish/result`
- `GET /api/trace/recent`
- `GET /api/trace/:taskId`
- `GET /api/tools`

后续计划迁移到 `distribution`：

```text
cloud-backend/internal/agents/distribution/
├── package
├── platform
├── plan
├── record
└── compliance
```

运营复盘暂时以“发布记录 + 手动数据回填 + 复盘文档/报告”为起点，后续收敛为 Operation Agent。

## 13. 数据和状态流

```mermaid
flowchart TD
    UserInput["用户输入\nmessage / project config"]
    AgentRun["agent_runs\n动态 Agent 运行"]
    Task["ai_task\nDAG 任务"]
    Node["ai_node\nDAG 节点"]
    Artifact["artifacts\n版本化产物索引"]
    Review["artifact_reviews\n审核记录"]
    LocalJob["local_jobs\n本地执行任务"]
    LocalFile["本地 Artifact 文件\ncontent + metadata"]

    UserInput --> AgentRun
    AgentRun --> Task
    Task --> Node
    Node --> Artifact
    Artifact --> Review
    Node --> LocalJob
    LocalJob --> LocalFile
    LocalFile --> Artifact
```

核心表族：

- `agent_runs`：Dynamic Agent Run 状态、计划和 task 关联。
- `ai_task`、`ai_node`、`ai_node_dependency`：DAG 任务与节点。
- `workflow_templates`、`workflow_runs`：模板工作流和运行实例。
- `video_projects`：视频项目。
- `artifacts`：版本化产物索引。
- `artifact_reviews`：人工审核记录。
- `local_runners`、`local_jobs`、`local_job_logs`：本地执行协议。
- `outbox`：可靠事件投递。

## 14. 典型端到端流程

```mermaid
flowchart TD
    A["1. 登录进入导演工作台"]
    B["2. 创建视频项目\nmode / duration / aspect ratio"]
    C["3. 启动 Dynamic Agent 或 Workflow Run"]
    D["4. Planner 生成 AgentPlan"]
    E["5. Guard 校验工具、参数、风险、阶段约束"]
    F["6. Compiler 编译 DAG\n插入质量门禁和 Review"]
    G["7. Orchestrator 执行 DAG"]
    H["8. 生成 Artifact\n脚本、分镜、Prompt、预览、视频"]
    I["9. 人工审核\n通过/驳回/编辑/重新生成"]
    J["10. 本地 Runner 执行渲染、打包或媒体处理"]
    K["11. 生成发布包和发布文案"]
    L["12. 发布记录与复盘"]

    A --> B --> C --> D --> E --> F --> G --> H --> I --> J --> K --> L
    I -->|驳回/编辑| H
```

## 15. API 文档机制

OpenAPI 不是手写文档，而是从代码里的权威 spec 生成：

- 云端 spec：`cloud-backend/internal/core/apispec/cloud_spec.go`
- 云端生成文档：`cloud-backend/docs/API_REFERENCE.md`
- 前端生成类型：`frontend/src/utils/api-types.generated.ts`
- 本地 spec：`local-backend/internal/localagent/openapi.go`
- 本地生成文档：`local-backend/docs/API_REFERENCE.md`

变更 API 时需要同步：

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

本地 API 文档生成：

```bash
cd local-backend
go run ./cmd/gen-local-apidocs
```

## 16. 开发与验证

本地开发：

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

云端开发：

```bash
cd cloud-backend
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

推荐验证：

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../cloud-backend && go test -race ./...
cd ../frontend && npm run build
```

API 文档检查：

```bash
cd cloud-backend && make api-docs-check
```

## 17. 架构演进路线

```mermaid
timeline
    title Beta 到稳定版演进
    P0 : 清理文档和 API 一致性
       : 保证构建、测试、生成文档通过
    P1 : 稳定视频工作台
       : 完善 Artifact 审核、返工、进度展示
    P2 : 资产库
       : 角色、场景、道具、风格、Prompt 资产化
    P3 : 本地媒体执行
       : FFmpeg、HyperFrames、本地 manifest 同步
    P4 : Distribution 发布中心
       : 多平台发布包、规则检查、发布记录
    P5 : Operation 运营复盘
       : 数据导入、单条复盘、账号周报、选题推荐
```

## 18. 读代码建议

如果要快速上手，可以按下面顺序读：

1. `docs/ARCHITECTURE.md`：原始架构设计和路线图。
2. `frontend/src/App.tsx`、`frontend/src/pages/DirectorStudioPage.tsx`：用户入口。
3. `frontend/src/services/api.ts`：前端如何调用云端 API。
4. `cloud-backend/cmd/tangying-ai-os/main.go`：云端服务组装和路由注册。
5. `cloud-backend/internal/core/agentruntime/runner.go`：动态 Agent 主链路。
6. `cloud-backend/internal/core/agentruntime/plan_guard.go`：计划安全和一致性校验。
7. `cloud-backend/internal/core/agentruntime/plan_compiler.go`：计划如何编译成 DAG。
8. `cloud-backend/internal/agents/video/handler`：视频项目和工作流 API。
9. `cloud-backend/internal/core/artifact`：Artifact 版本、审核和 stale 规则。
10. `local-backend/internal/localagent/server.go`、`local-backend/internal/localtool/registry.go`：本地执行边界。

## 19. 总结

这个项目的架构核心是“云端编排 + 本地执行 + Artifact 闭环”。云端不直接吞掉用户的大文件，而是维护计划、状态、索引和审核流；本地承接文件和媒体处理；前端把复杂流程收束成导演工作台；Dynamic Agent Runtime 负责把自然语言创作目标变成可验证、可审核、可追踪的执行 DAG。

