# Tangying AIOS 新开发者上手指南

> 当前代码基线：v4.0 closed beta，最后更新 2026-06-30。
> 本文面向刚加入项目的开发者，按当前仓库代码介绍项目定位、运行边界、关键目录、主流程和验证方式。

## 1. 项目是什么

Tangying AIOS 是面向个人创作者的视频创作 Agent 系统。当前封闭内测版本的目标不是“自动发布平台”，而是稳定跑通：

```text
视频想法
→ Dynamic Agent Runtime 自动规划
→ 脚本 / 分镜 / Shot List / Prompt
→ 素材依赖点
→ 用户外部生成素材并回填
→ Artifact Review 审核与返工
→ Local Runner / HyperFrames 渲染或校验
→ 小红书 / Bilibili 发布包导出
```

系统不会要求用户在项目开始前批量上传素材。当某个 shot 需要参考图、关键帧或 AIGC 视频片段时，Director Studio 会停在“素材依赖点”，把 Prompt、Negative Prompt、参考图、目标规格和上传入口展示给用户。用户可以使用任意外部网站生成素材，然后上传回这个依赖点。

桌面端不提供任意命令执行入口，也不触发自动发布。桌面用户配置的模型 API Key 只保存在本机 local agent。

## 2. 运行边界

仓库按运行边界拆分：

```text
frontend/                    # React + Electron UI
local-backend/               # 本地轻量 agent，无 DB/Docker/Redis/Kafka/MinIO
cloud-backend/               # Go AIOS Core 云端 backend
hyperframes-render-service/  # HyperFrames HTTP 渲染服务
docs/                        # 顶层架构、部署、内测和本地使用文档
scripts/                     # 开发启动、构建、文档生成辅助脚本
```

职责表：

| 边界 | 负责 | 不负责 |
|------|------|--------|
| `frontend/` | 登录、Director Studio、Artifact 审核、Trace、素材依赖点回填、本地设置 | 任意 shell、自动发布 |
| `local-backend/` | 本地文件、产物、缓存、日志、诊断、本机模型 Provider token | 云端编排、数据库、对象存储 |
| `cloud-backend/` | 账号、Agent Runtime、DAG 编排、Artifact/Review、视频项目 API、模型网关、云端日志 | 保存桌面用户大文件、保存桌面用户 token |
| `hyperframes-render-service/` | HyperFrames lint、snapshot、render | 业务编排、账号、资产索引 |

## 3. 当前产品入口

当前默认入口是 Director Studio：

```text
frontend/src/pages/DirectorStudioPage.tsx
frontend/src/pages/directorStudioLogic.ts
```

它覆盖：

- 创建或启动视频创作流程。
- 查看阶段树、运行状态、Trace。
- 查看、审核、编辑、返工 Artifact。
- 查看素材依赖点，复制 Prompt 和参考信息。
- 上传用户在外部网站生成的图片/视频结果。
- 导出手动发布材料。

旧式单页发布表单和 publish 兼容接口保留是为了封闭内测不中断，不是新功能的首选入口。

## 4. Cloud Backend 代码地图

云端启动入口：

```text
cloud-backend/cmd/tangying-ai-os/main.go
```

启动时会组装：

- Auth：注册、登录、当前用户。
- Health：`/api/health`、`/api/health/ready`。
- Orchestrator / Worker / Outbox / Eventbus：DAG 执行、状态机、异步事件。
- Dynamic Agent Runtime：`LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`，API 为 `/api/agent/runs`。
- Workflow / SkillRuntime / Skill Capabilities：视频能力包、workflow run、checkpoint/recover。
- Video Project：项目 CRUD、session 聚合、阶段审批。
- Artifact：版本化索引、内容读取、历史、返工、Review、stale tracking。
- Video Assets：外部生成素材结果登记。
- Local Runner：runner 注册、心跳、领取任务、进度和完成回传。
- Model Gateway：模型能力路由、服务端 Provider 配置、fake provider 测试。
- Publish Compatibility：封闭内测发布包/文案准备兼容层。
- OpenAPI：`/docs`、`/openapi.json`。

重要目录：

```text
cloud-backend/internal/core/
├── agentruntime/       # Dynamic Agent Runtime
├── orchestrator/       # DAG 任务与节点状态机
├── worker/             # 节点执行器与工具注册
├── workflow/           # workflow template / run / checkpoint
├── skillruntime/       # skill catalog / route / compile
├── modelgateway/       # 模型调用统一出口
├── artifact/           # Artifact、Review、stale tracking
├── localrunner/        # 云端到本地执行器协议
├── eventbus/           # Kafka/Redpanda 事件总线
└── outbox/             # 可靠事件投递

cloud-backend/internal/agents/video/
├── handler/            # video projects、workflow runs、stage approve
├── assistant/          # 项目级 assistant
├── assets/             # 素材依赖点和外部生成结果 manifest
├── service/            # 视频创作服务、渲染策略、验证
├── planjudge/          # 封闭内测计划质量与禁用能力拦截
├── inputresolver/      # 输入解析
└── knowledgepolicy/    # 知识策略
```

## 5. Dynamic Agent Runtime

动态 Agent 入口：

```text
POST /api/agent/runs
```

请求示例：

```json
{
  "message": "请帮我根据端午节的来历创作一个口播知识分享视频"
}
```

执行链路：

```text
LLMPlanner 生成 AgentPlan JSON
→ PlanGuard 校验工具、参数、引用、风险
→ PlanCompiler 插入质量门禁和人工审核 CONTROL 节点
→ Transient DAG 提交 Orchestrator
→ Worker 执行节点
→ Artifact Review 暂停等待人工审核
→ 审核通过后继续，下游失败或返工时记录 trace/stale
```

常用 API：

```text
POST /api/agent/runs
GET  /api/agent/runs/:runId
GET  /api/agent/runs/:runId/trace
GET  /api/agent/runs/:runId/reviews
POST /api/agent/runs/:runId/reviews/:reviewId/approve
POST /api/agent/runs/:runId/reviews/:reviewId/reject
POST /api/agent/runs/:runId/reviews/:reviewId/submit-edited
POST /api/agent/runs/:runId/reviews/:reviewId/regenerate
```

## 6. 素材依赖点

这是当前内测最重要的产品边界之一：不要让系统绝对依赖图片/视频生成 API，也不要要求用户在开始项目前主动上传所有素材。

数据流：

```text
cloud-backend 生成 external_generation_request Artifact
→ frontend 展示素材依赖点
→ 用户复制 Prompt / 参考信息到外部网站生成素材
→ frontend 调用 local-backend POST /api/local/artifacts 上传文件
→ local-backend 返回 storageRef / contentHash / sizeBytes / mimeType
→ frontend 调用 cloud-backend POST /api/video-projects/:id/external-generation-results
→ cloud-backend 登记 external_generation_result Artifact
```

相关代码：

```text
frontend/src/pages/DirectorStudioPage.tsx
frontend/src/pages/directorStudioLogic.ts
frontend/src/services/localAgent.ts
cloud-backend/internal/agents/video/assets/handler.go
cloud-backend/internal/agents/video/assets/manifest.go
local-backend/internal/localagent/server.go
```

云端登记 payload 关注字段：

```json
{
  "kind": "video",
  "storageType": "local",
  "storageRef": "local://projects/vp-1/artifacts/...",
  "mimeType": "video/mp4",
  "sizeBytes": 123456,
  "contentHash": "sha256...",
  "relatedShotId": "shot-01",
  "generationRequestId": "extgen-shot-01",
  "source": "external_manual_upload",
  "tags": ["external_manual_upload", "material_dependency_result"]
}
```

云端不默认接收图片/视频正文，只保存本地引用、hash、大小、依赖关系和 trace。

## 7. Frontend 代码地图

技术栈：React 18、TypeScript、Vite、Electron 33、TailwindCSS。

当前关键文件：

```text
frontend/src/
├── App.tsx                         # App 入口和登录态
├── components/AuthScreen.tsx       # 登录 / 注册
├── pages/DirectorStudioPage.tsx    # 当前主工作台
├── pages/directorStudioLogic.ts    # 阶段、Artifact、Trace 映射
├── pages/DesktopPage.tsx           # local agent 和本机 Provider 设置
├── services/api.ts                 # cloud API client
├── services/auth.ts                # auth API client
├── services/localAgent.ts          # local agent API client
├── utils/api-types.generated.ts    # 生成类型，不手写
└── utils/electron.ts               # Electron 环境能力封装
```

常用命令：

```bash
cd frontend
npm install
npm run dev
npm run lint
npm run test:director
npm run test:security
npm run build
```

Electron 开发：

```bash
cd frontend
npm run electron:dev
```

## 8. Local Backend 代码地图

本地 agent 入口：

```text
local-backend/cmd/tangying-local-agent/main.go
local-backend/internal/localagent/server.go
```

本地 API：

```text
GET    /api/local/health
GET    /api/local/paths
GET    /api/local/model-providers
PUT    /api/local/model-providers
POST   /api/local/artifacts
GET    /api/local/artifacts/:id?projectId=<projectId>
DELETE /api/local/artifacts/:id?projectId=<projectId>
DELETE /api/local/projects/:id
POST   /api/local/logs
POST   /api/local/diagnostics
GET    /api/local/openapi.json
GET    /api/local/docs
```

本地数据目录：

```text
macOS:   ~/Library/Application Support/TangyingAIOS/
Windows: %APPDATA%/TangyingAIOS/
Linux:   ~/.tangying-aios/
```

本地端不能新增 PostgreSQL、Redis、Kafka、MinIO、Docker 或云端 API Key 依赖。涉及用户文件的功能优先通过 local agent 受控接口实现。

## 9. HyperFrames Render Service

目录：

```text
hyperframes-render-service/
```

它是 Node.js/TypeScript HTTP 服务，供 Go backend 或本地执行链路调用，不依赖 shell 调 `npx hyperframes`。

API：

```text
GET  /health
POST /lint
POST /snapshot
POST /render
POST /render/stream
GET  /jobs
GET  /jobs/:jobId
```

常用命令：

```bash
cd hyperframes-render-service
npm install
npm run build
npm run dev
```

## 10. API 文档规则

API 文档从代码规范生成，不手写生成物。

Cloud:

```text
source: cloud-backend/internal/core/apispec/cloud_spec.go
docs:   cloud-backend/docs/API_REFERENCE.md
types:  frontend/src/utils/api-types.generated.ts
ui:     http://localhost:8080/docs
json:   http://localhost:8080/openapi.json
```

Local:

```text
source: local-backend/internal/localagent/openapi.go
docs:   local-backend/docs/API_REFERENCE.md
ui:     http://localhost:18080/api/local/docs
json:   http://localhost:18080/api/local/openapi.json
```

变更 API 时：

```bash
cd cloud-backend
make gen-docs
make api-docs-check

cd ../local-backend
go run ./cmd/gen-local-apidocs
```

不要手改带有 `DO NOT EDIT` banner 的生成文件。

## 11. 启动开发环境

本地桌面开发：

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

云端开发：

```bash
bash scripts/start-cloud-backend.sh
```

手动启动云端：

```bash
cd cloud-backend
cp .env.example .env
# 填 OPENAI_API_KEY / AUTH_TOKEN_SECRET
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

Cloud Compose 部署：

```bash
cd cloud-backend/deploy
cp .env.cloud.example .env.cloud
docker compose --env-file .env.cloud -f docker-compose.cloud.yml up -d --build
```

桌面打包：

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

## 12. 验证命令

提交前按变更范围运行。封闭内测完整验证建议：

```bash
(cd local-backend && go test ./...)
(cd cloud-backend && go test ./...)
(cd cloud-backend && go test -race ./...)
(cd cloud-backend && go run ./evals/video_beta)
(cd cloud-backend && make api-docs-check)
(cd frontend && npm run lint)
(cd frontend && npm run test:director)
(cd frontend && npm run test:security)
(cd frontend && npm run build)
(cd hyperframes-render-service && npm run build)
```

浏览器/桌面冒烟测试需要覆盖：

- 登录和会话恢复。
- Director Studio 启动创作流程。
- Preflight 显示。
- Artifact 审核、编辑、返工。
- Trace / Artifacts / Export tabs。
- 素材依赖点 Prompt 展示。
- 素材依赖点上传回填：local agent 保存文件，cloud 登记结果。
- Desktop 设置页无任意命令执行 UI。

## 13. 开发原则

- 新视频业务优先放在 `cloud-backend/internal/agents/video`，不要把业务规则写进 `internal/core`。
- `internal/core` 只承载通用 runtime、编排、Artifact、模型网关和本地执行协议。
- 不给 `local-backend` 增加数据库、Docker、Redis、Kafka、MinIO 或云端 LLM Key 依赖。
- 不在 Electron renderer 暴露任意命令执行 IPC。
- 不新增自动发布入口；封闭内测只导出发布材料。
- 用户素材默认本地保存；云端只登记引用、hash、大小、状态和 trace。
- 新增或修改 API 必须同步 OpenAPI source，并重新生成文档和类型。
- 新功能优先补强 Director Studio，不恢复旧式单页发布表单作为主入口。

## 14. 常见问题

### 前端连不上云端

检查 `VITE_CLOUD_API_BASE` 或 `TANGYING_CLOUD_API_BASE`。Web 同域部署可以使用 `/api`。

### local agent 不在线

运行：

```bash
bash scripts/start-local-backend.sh
curl http://127.0.0.1:18080/api/local/health
```

### 云端 ready 不通过

检查 PostgreSQL、Redis、Redpanda 是否启动：

```bash
cd cloud-backend
docker compose ps
curl http://localhost:8080/api/health/ready
```

### Agent 规划失败

检查服务端 `OPENAI_API_KEY`、`OPENAI_BASE_URL`、`OPENAI_MODEL`，或云端 `/api/config/model-provider` 运维 override。不要把桌面用户 token 上传到云端。

### 素材依赖点卡住

这是预期的人工暂停点。复制页面里的 Prompt 和参考信息，到外部图片/视频网站生成素材，然后上传回同一个依赖点。

### API 文档漂移

运行：

```bash
cd cloud-backend
make gen-docs
make api-docs-check
```
