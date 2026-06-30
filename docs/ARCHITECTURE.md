# 躺营 Video Agent 架构设计文档

> 版本：v4.0 closed beta
> 最后更新：2026-06-30
> 目标：面向 1-3 位熟悉创作者的封闭内测版本，稳定跑通从想法到发布包的生产闭环。

## 1. 产品定位

躺营 Video Agent 是面向个人创作者的视频生产系统。底层 `internal/core` 保持视频创作引擎能力，但当前产品入口只服务视频创作、发布准备和后续运营复盘。

当前 beta 的目标不是做泛办公平台，而是稳定支撑这一条主链路：

```text
选题 / 想法
→ Creative Brief
→ 脚本 / 口播稿
→ 分镜 / Beat / Shot List
→ 角色 / 场景 / 道具 / 风格资产
→ 关键帧 Prompt / 视频 Prompt
→ 外部素材依赖点 / 本地渲染或素材导入
→ 人工审核
→ 多平台发布包
→ 发布记录
→ 数据复盘
→ 下一轮选题优化
```

## 2. 封闭内测全流程

封闭内测以小范围真实创作者试用为验收目标。必须能在开发环境和 cloud compose 环境跑通：

1. 在前端导演工作台输入视频想法和目标平台。
2. 通过 `POST /api/agent/runs` 启动 Dynamic Agent Runtime，或通过 `/api/video-projects` 创建视频项目后启动工作流。
3. 生成脚本、分镜、Prompt、发布文案等 Artifact。
4. 通过 Artifact Review 审核关键产物；驳回或返工后，下游产物能标记为 stale。
5. 当系统需要关键帧、参考图或 AIGC 视频片段但不应强依赖 API 时，生成 `external_generation_request` 素材依赖点：包含 Prompt、Negative Prompt、参考图、本 shot 规格和上传回填入口。
6. 用户可在任意外部图片/视频网站手动生成素材，再上传回对应依赖点；local-backend 保存文件，cloud-backend 只登记 `storage_ref`、hash、size、manifest、shot 关联和 trace。
7. 本地 Runner 执行重媒体任务，云端只保存 manifest、hash、本地引用和状态。
8. HyperFrames Render Service 或本地 Runner 渲染链路可生成或校验可视化视频项目。
9. 使用发布包导出能力生成小红书/Bilibili 发布文案和素材清单；封闭内测不触发自动发布。
10. 发布后的数据复盘先以手动导入和文档化路线为准，后续收敛到 Operation Agent。

## 3. 系统运行边界

```text
frontend/                    # React + Electron UI，默认入口是导演工作台
local-backend/               # 本地轻量执行器，无 DB / Docker / Redis / Kafka / MinIO
cloud-backend/               # 云端 AIOS Core，负责账号、编排、API、日志和远程配置
hyperframes-render-service/  # HyperFrames HTML/CSS/JS → 视频渲染服务
```

边界规则：

- `local-backend` 不引入数据库、Docker、Redis、Kafka、MinIO 或云端 LLM API Key。
- 用户本地素材和大文件默认留在本地，云端保存索引、hash、状态和 trace。
- 封闭内测不要求用户在项目开始前主动上传素材；流程在具体素材依赖点暂停，把 Prompt、参考图和规格交给用户，用户自由选择外部网站生成后回填。
- 用户在桌面端填写的模型 Provider token 只保存到 local-backend；云端不提供 LLM、文生图片或文生视频 API 服务。
- `cloud-backend/internal/core` 不写具体视频业务规则；视频业务位于 `internal/agents/video` 和视频 skill/capability。
- 当前 `publish` 模块保留为 beta 兼容发布包/发布准备层，后续迁移到 `distribution`。

## 4. 总体架构

```text
frontend
├── 创作中心 / 导演工作台
├── 视频项目
├── Artifact 审核与返工
├── 发布素材面板
└── 系统设置

cloud-backend
├── internal/core
│   ├── agentruntime
│   ├── orchestrator
│   ├── workflow
│   ├── skillruntime
│   ├── modelgateway
│   ├── artifact
│   ├── localrunner
│   ├── eventbus
│   └── outbox
└── internal/agents
    ├── video
    └── publish      # beta 兼容层，后续迁移为 distribution

local-backend
├── local file manager
├── local artifact store
├── local model provider config
├── ffmpeg executor
├── hyperframes executor
└── diagnostics

hyperframes-render-service
└── html/css/js → video render
```

## 4.1 当前代码实现地图

当前代码入口与职责如下：

```text
frontend/src/
├── App.tsx                         # 登录态、云端 API base、默认进入 Director Studio
├── components/AuthScreen.tsx       # 注册 / 登录 / 会话恢复
├── pages/DirectorStudioPage.tsx    # 当前主工作台：启动、审核、Trace、Artifact、导出、素材依赖点
├── pages/directorStudioLogic.ts    # 阶段、Artifact、Trace、错误与 UI 映射逻辑
├── pages/DesktopPage.tsx           # local agent 状态和本机模型 Provider 设置
├── services/api.ts                 # cloud-backend API client
├── services/localAgent.ts          # local-backend API client
└── utils/api-types.generated.ts    # 从 cloud OpenAPI 生成，不手写
```

```text
cloud-backend/cmd/tangying-ai-os/main.go
├── auth / health / context / trace
├── orchestrator + worker + outbox + eventbus
├── Dynamic Agent Runtime: /api/agent/runs
├── workflow / skillruntime / skill capabilities
├── video projects / workflow runs / checkpoints
├── artifact index / artifact review / stale tracking
├── video external-generation result registration
├── local runner protocol
├── client-provided model provider injection
├── publish compatibility API
└── OpenAPI: /docs, /openapi.json
```

```text
local-backend/internal/localagent/
├── server.go       # /api/local/* routes, local files, logs, diagnostics, model-provider config
├── openapi.go      # local OpenAPI source
└── docs.go         # local Swagger UI
```

```text
hyperframes-render-service/src/server.ts
├── GET  /health
├── POST /lint
├── POST /snapshot
├── POST /render
├── POST /render/stream
└── GET  /jobs, /jobs/:jobId
```

## 5. Core 视频创作引擎

`cloud-backend/internal/core` 提供平台机制，不绑定具体业务。

关键模块：

- `agentruntime`：`LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`，入口为 `/api/agent/runs`。
- `orchestrator`：DAG 任务、节点状态机、暂停/恢复、重试、CONTROL 节点和质量门禁。
- `workflow`：Workflow Template、Workflow Run、Skill 编译为 DAG。
- `skillruntime`：加载 `skills/*/*/skill.yaml`，提供 Skill catalog、route、compile。
- `modelgateway`：保留 Provider 抽象与 fake provider 测试能力；封闭内测主链路使用客户端随请求传入的 OpenAI-compatible provider。
- `artifact`：版本化产物索引、审核状态、stale 级联追踪。
- `localrunner`：云端控制本地执行器的 job/heartbeat/progress 协议。
- `eventbus` / `outbox`：事件可靠投递与异步执行。

## 6. Video Agent 业务层

目录：`cloud-backend/internal/agents/video/`

当前 beta 已覆盖：

- 视频项目 CRUD：`/api/video-projects`
- 项目 session 聚合视图：`/api/video-projects/:id/session`
- 视频工作流运行：`/api/video-projects/:id/workflow-runs`
- 阶段审批封装：`/api/video-projects/:id/stages/:stage/approve`
- 视频项目 Artifact 查询：`/api/video-projects/:id/artifacts`

后续业务拆分方向：

```text
video/
├── project
├── brief
├── script
├── storyboard
├── asset
├── prompt
├── render
├── review
└── assistant
```

beta 阶段不强行迁移目录，避免破坏现有可用链路。

## 7. Publish 兼容发布包层与 Distribution 路线

当前可用模块：`cloud-backend/internal/agents/publish/`

当前 beta 保留接口：

- `POST /api/publish`
- `POST /api/ai/generate`
- `POST /api/ai/generate-from-media`
- `POST /api/ai/polish`
- `POST /api/ai/polish/submit`
- `GET /api/ai/polish/result`
- `GET /api/trace/recent`
- `GET /api/trace/:taskId`
- `GET /api/tools`

这些接口用于封闭内测的发布文案、素材分析、发布准备和 trace 调试。当前桌面包不提供自动发布 IPC，也不打包自动发布脚本。后续 P4 将迁移为：

```text
cloud-backend/internal/agents/distribution/
├── package
├── platform
├── plan
├── record
└── compliance
```

目标接口族为 `/api/distribution/*`。迁移前不得删除 beta 当前依赖的 `/api/publish` 和 `/api/ai/*`。

## 8. Operation 运营复盘路线

当前 beta 不实现完整 Operation Agent，但产品路线必须保留发布后闭环：

```text
手动导入平台数据
→ 单条视频表现复盘
→ 账号周报
→ 标题 / 封面 / 开头钩子归因
→ 下一批选题推荐
```

后续目录：

```text
cloud-backend/internal/agents/operation/
├── metric
├── insight
├── topic
└── report
```

第一版可用性以“发布记录 + 手动数据回填 + 复盘文档/报告”为起点。

## 9. Artifact 创作资产体系

Artifact 是视频创作过程的长期资产索引，不只是中间结果。

关键能力：

- 版本：同一 stage/unit 自动递增 version。
- 审核：PENDING / APPROVED / REJECTED。
- 状态：valid / stale / rejected / failed / deleted。
- 依赖：上游被修改或驳回后，下游产物标记 stale。
- 存储边界：本地正文和媒体文件保存在本机，云端只保存 `storage_type=local`、`storage_ref`、hash、size、metadata。

常见资产类型：

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

## 9.1 素材依赖点与外部生成回填

封闭内测不把“用户先上传素材”作为流程前置条件。系统在具体 shot 需要素材时创建素材依赖点，用户再决定使用哪个外部网站或工具生成。

数据流：

```text
cloud-backend 生成 external_generation_request Artifact
→ frontend Director Studio 展示 Prompt / Negative Prompt / 参考图 / 规格
→ 用户在外部网站生成图片或视频
→ frontend 调用 local-backend POST /api/local/artifacts 上传文件到本机
→ frontend 调用 cloud-backend POST /api/video-projects/:id/external-generation-results
→ cloud-backend 登记 external_generation_result Artifact 与原 shot / request 的依赖关系
```

关键约束：

- `external_generation_request` 是“等待用户提供素材”的明确暂停点，不代表系统强制调用图片或视频 API。
- 上传回填先落本地，`local-backend` 计算 hash、size、mime type 并返回 `storageRef`。
- 云端登记 `storageType=local`、`storageRef`、`contentHash`、`sizeBytes`、`relatedShotId`、`generationRequestId` 和 tags。
- 前端上传来源使用 `source=external_manual_upload`，便于和自动工具产物区分。
- 如果用户未配置图片/视频 Provider，流程仍可通过外部网站 + 手动回填继续。

## 10. Local Runner 本地执行体系

Local Runner 是桌面用户的本地执行面。云端负责调度，本地负责文件、媒体处理和本机 Provider 配置。

云端协议：

- `POST /api/local-runners/register`
- `POST /api/local-runners/:runnerId/heartbeat`
- `GET /api/local-runners/:runnerId/jobs/claim`
- `POST /api/local-jobs/:jobId/progress`
- `POST /api/local-jobs/:jobId/complete`
- `POST /api/local-jobs/:jobId/fail`

安全要求：

- 只访问用户授权项目目录。
- 禁止任意 shell 入口；Electron renderer 不暴露任意命令 IPC。
- 本地命令白名单化。
- 用户 API Key 不上传云端。
- 大文件默认不上传云端。
- 本地任务必须可取消并上报 progress/heartbeat。

## 11. 模型 Provider 策略

封闭内测不在云端提供模型 API 服务。用户在桌面端「系统 → 基础模型 API」分别配置文生文、文生图片、文生视频 Provider，字段统一为 OpenAI-compatible 的 `baseUrl`、`model`、`token`。客户端在启动 run 或返工时按次把 provider 传入云端执行节点。

如果没有配置 provider：

- 文生文节点输出明确配置提示，不调用云端 Key。
- 图片素材节点停在素材依赖点，输出 Prompt、参考信息和目标规格。
- 视频素材节点要求用户配置文生视频 Provider，或到外部网站生成后回填。
- 云端 OpenAPI 不暴露 `/api/config/model-provider`，用户 token 不进入云端持久化。

## 12. HyperFrames 渲染链路

HyperFrames 用于口播可视化、信息图、动效视频和发布素材生成。

链路：

```text
脚本 / Beat / 风格
→ HyperFrames project manifest
→ lint / preview
→ render service
→ local artifact manifest
→ cloud artifact index
```

服务边界：

- `hyperframes-render-service` 通过 HTTP 提供 lint/render 能力。
- 本地执行器可承接 HyperFrames 命令。
- 云端不默认保存用户生成视频正文，只保存索引与状态。
- Cloud compose 默认以 service mode 连接 `http://hyperframes-render-service:8787`。

## 13. API 设计

权威 API 来源是 `cloud-backend/internal/core/apispec/cloud_spec.go`。生成物：

- `cloud-backend/docs/API_REFERENCE.md`
- `frontend/src/utils/api-types.generated.ts`

常用 beta API：

```text
Auth:
POST /api/auth/register
POST /api/auth/login
GET  /api/auth/me

Dynamic Agent:
POST /api/agent/runs
GET  /api/agent/runs/:id
GET  /api/agent/runs/:id/trace
GET  /api/agent/runs/:id/reviews
POST /api/agent/runs/:id/reviews/:rid/approve
POST /api/agent/runs/:id/reviews/:rid/reject
POST /api/agent/runs/:id/reviews/:rid/submit-edited
POST /api/agent/runs/:id/reviews/:rid/regenerate

Video Runtime:
GET /api/video/preflight
GET /api/video/role-agents
GET /api/video/role-agents/:roleId

Video Projects:
GET    /api/video-projects
POST   /api/video-projects
GET    /api/video-projects/:id
GET    /api/video-projects/:id/session
PATCH  /api/video-projects/:id
DELETE /api/video-projects/:id

Workflow Runs:
POST /api/video-projects/:id/workflow-runs
GET  /api/video-projects/:id/workflow-runs/:rid
POST /api/video-projects/:id/workflow-runs/:rid/pause
POST /api/video-projects/:id/workflow-runs/:rid/cancel
GET  /api/video-projects/:id/workflow-runs/:rid/checkpoints
POST /api/video-projects/:id/workflow-runs/:rid/recover

Artifacts:
GET  /api/video-projects/:id/artifacts
POST /api/video-projects/:id/external-generation-results
GET  /api/artifacts/:id
GET  /api/artifacts/:id/content
GET  /api/artifacts/:id/history
POST /api/artifacts/:id/revise

Local Runner:
POST /api/local-runners/register
POST /api/local-runners/:runnerId/heartbeat
GET  /api/local-runners/:runnerId/jobs/claim
POST /api/local-jobs/:jobId/progress
POST /api/local-jobs/:jobId/complete
POST /api/local-jobs/:jobId/fail

Publish compatibility:
POST /api/publish
POST /api/ai/generate
POST /api/ai/generate-from-media
POST /api/ai/polish
POST /api/ai/polish/submit
GET  /api/ai/polish/result
```

新增或变更 API 时必须更新 `cloud_spec.go` 并运行：

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```

## 14. 数据模型

核心表族：

- `agent_runs`：Dynamic Agent Run 状态、计划和 task 关联。
- `ai_task` / `ai_node` / `ai_node_dependency`：DAG 任务与节点。
- `workflow_templates` / `workflow_runs`：模板和视频工作流运行。
- `workflow_checkpoints`：阶段边界恢复点，用于 workflow run recover。
- `video_projects`：视频项目。
- `artifacts`：版本化产物索引。
- `artifact_reviews`：产物审核记录。
- `local_runners` / `local_jobs` / `local_job_logs`：本地执行器协议。
- `outbox`：事件可靠投递。

beta 不新增重型数据模型，优先复用现有表族跑通流程。

## 15. 前端页面结构

当前前端默认入口：`frontend/src/pages/DirectorStudioPage.tsx`

beta 主要视图：

- 登录/会话恢复：`AuthScreen`
- 导演工作台：项目创建、阶段树、Artifact 审核、运行状态、发布素材
- 本地服务状态：Electron 环境下检查 local agent health
- 系统设置：本地服务状态和本机模型 Provider 设置

旧发布页源码可暂留，但不作为默认入口。新功能优先补强导演工作台，而不是恢复旧式单页发布表单。桌面设置页不再提供命令执行面板。

## 16. 测试与验收标准

基础验证：

```bash
cd cloud-backend && go test ./...
cd cloud-backend && go test -race ./...
cd cloud-backend && go run ./evals/video_beta
cd cloud-backend && make api-docs-check
cd local-backend && go test ./...
cd local-backend && go test -race ./...
cd hyperframes-render-service && npm run build
cd frontend && npm run lint
cd frontend && npm run test:director
cd frontend && npm run test:security
cd frontend && npm run build
```

beta 自用验收：

- 能登录并进入导演工作台。
- 能创建视频项目或启动 Dynamic Agent Run。
- 能生成并查看 Artifact。
- 能执行审核、驳回或返工。
- PlanJudge 阻断封闭内测禁用工具、缺失发布文案、缺失预览依赖和重复非质量工具。
- 桌面端无任意命令执行 IPC、无自动发布 IPC。
- 用户模型 API Key 不从桌面端同步到云端，云端也不提供模型 API fallback。
- 本地 Runner 可注册、心跳、领取任务并回传状态。
- HyperFrames 服务可健康检查并用于渲染链路。
- 当前发布兼容接口可生成发布文案或发布准备任务。
- 生成 API 文档不暴露已移除的非视频产品入口。

## 17. 后续路线图

P0：清理一致性和 beta 可用性。

- 清理 API 文档和架构文档中的旧业务入口。
- 保证构建、测试、生成文档通过。
- 保留当前发布兼容层，确保自用全流程不断。

P1：视频工作台稳定。

- 完善项目创建、阶段树、Artifact 审核和返工。
- 增强 SSE/progress 展示。
- 跑通口播可视化视频主流程。

P2：资产库。

- 角色、场景、道具、风格、Prompt 资产化。
- 支持项目资产提升为全局资产。
- 支持历史资产复用到新项目。

P3：本地媒体执行。

- FFmpeg 合成、转码、音频标准化、字幕烧录。
- HyperFrames lint/render。
- 本地 manifest 自动同步。

P4：Distribution 发布中心。

- 将 `publish` 兼容层迁移到 `distribution`。
- 生成多平台发布包。
- 发布前平台规则检查。
- 发布记录与后续可授权发布能力。

P5：Operation 运营复盘。

- 手动导入平台数据。
- 单条视频复盘。
- 账号周报。
- 下一批选题推荐。
