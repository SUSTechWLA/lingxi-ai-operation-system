# 云端部署说明

本文面向部署和运维同学。新的云端形态是：

```text
cloud-backend/
  -> Go AIOS Core, port 8080
  -> PostgreSQL / Redis / Redpanda / MinIO
  -> HyperFrames Render Service, port 8787
  -> cloud-side integrations, account/config services
  -> cloud logs, trace, diagnostics analysis
```

云端负责远程配置、账号、远程编排、云端日志和诊断分析。本地用户不需要安装数据库或 Docker。桌面用户生成的脚本、JSON、图片、音频、视频等个人资产默认保存在本地；用户自配的基础模型 Provider token 也只保存在本机。云端只保存本地引用、hash、size、版本、服务日志和必要的诊断索引。

当前云端服务的是封闭内测视频创作链路：Director Studio 调用云端 Dynamic Agent Runtime，云端生成和调度脚本、分镜、Prompt、Artifact Review、素材依赖点和发布包导出。云端不替用户自动发布，也不要求用户在项目开始前上传素材。

## 目录

```text
cloud-backend/
├── cmd/
├── internal/
├── skills/
├── deploy/
│   ├── docker-compose.cloud.yml
│   ├── nginx.conf
│   └── .env.cloud.example
├── scripts/
├── Dockerfile
└── .env.example
```

## 开发启动

```bash
cd cloud-backend
cp .env.example .env
# 填 OPENAI_API_KEY / OPENAI_BASE_URL
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

或使用根目录脚本：

```bash
bash scripts/start-cloud-backend.sh
```

根目录脚本 `scripts/start-cloud-backend.sh` 会自动设置 `SKILL_ROOT` 等环境变量，从仓库根目录即可直接启动。如果手动启动，请确保在 `cloud-backend/` 目录下运行，或通过环境变量显式指定 `SKILL_ROOT=cloud-backend/skills`（相对于工作目录解析）。

## 云端 Docker Compose

```bash
cd cloud-backend/deploy
cp .env.cloud.example .env.cloud
# 填 OPENAI_API_KEY / POSTGRES_PASSWORD / MINIO_SECRET_KEY
docker compose --env-file .env.cloud -f docker-compose.cloud.yml up -d --build
```

云端 compose 包含：

- nginx: 前端静态文件和 `/api/*` 反代。
- backend: Go AIOS Core。
- postgres: 业务数据库。
- redis: 会话/缓存。
- redpanda: Kafka-compatible event bus。
- minio: 云端对象存储，用于云端服务资产和 legacy 媒体兼容；不作为桌面用户生成资产的默认存储。
- hyperframes-render-service: HyperFrames HTML/CSS/JS 渲染服务，backend 通过 `HYPERFRAMES_SERVICE_URL=http://hyperframes-render-service:8787` 调用。

Compose 会把 `cloud-backend/skill-capabilities` 打入 backend 镜像，并设置 `SKILL_CAPABILITY_ROOT=/app/skill-capabilities`。动态 Agent 依赖这个目录加载视频能力包。

`AUTH_TOKEN_SECRET` 和 `OPENAI_API_KEY` 是必填变量；缺失时 compose 会在启动前失败。Redpanda 在容器网络内广播 `internal://redpanda:9092`，不要改回 `localhost`，否则 backend 容器无法连接 Kafka。

## 当前云端服务职责

Go backend 启动入口为 `cloud-backend/cmd/tangying-ai-os/main.go`。当前进程会组装：

- 账号与鉴权：注册、登录、当前用户。
- Dynamic Agent Runtime：`/api/agent/runs` 及 run trace/review API。
- Orchestrator/Worker：DAG 节点状态机、重试、质量门禁、CONTROL 节点。
- Video Project：项目、session、workflow run、checkpoint/recover、stage approve。
- Artifact：版本化索引、内容读取、历史、返工、Review、stale tracking。
- 素材依赖点回填登记：`POST /api/video-projects/:id/external-generation-results`。
- Local Runner 协议：runner register、heartbeat、claim、progress、complete/fail。
- Model Gateway 与云端运维 override：服务端模型配置，不接收桌面用户 token。
- Publish 兼容层：封闭内测的发布包/文案准备，不触发自动发布。
- OpenAPI 与 Swagger UI：`/openapi.json`、`/docs`。

## 前端云端 API 配置

桌面包和 Web 前端通过以下变量确定云端 API：

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api
```

Web 部署在 nginx 下时，也可以使用同域 `/api`。

## 健康检查

- `/api/health`: liveness，只表示进程和 HTTP 路由仍在响应。
- `/api/health/ready`: readiness，会检查 PostgreSQL、Redis、Kafka 依赖；Docker Compose 和部署脚本应使用这个端点判断 backend 是否可接流量。

## Dynamic Agent API

不走 workflow_template 的动态 Agent 入口：

```text
POST /api/agent/runs                                    # 启动 dynamic agent run
GET  /api/agent/runs/:runId                             # 查询 run 状态和 plan
GET  /api/agent/runs/:runId/trace                       # 获取 DAG 执行追踪
GET  /api/agent/runs/:runId/reviews                     # 列出待审核节点
POST /api/agent/runs/:runId/reviews/:reviewId/approve   # 审核通过
POST /api/agent/runs/:runId/reviews/:reviewId/reject    # 审核驳回
POST /api/agent/runs/:runId/reviews/:reviewId/submit-edited # 提交人工编辑结果
POST /api/agent/runs/:runId/reviews/:reviewId/regenerate    # 触发上游阶段重生成
GET  /api/video/preflight                                # 视频流程启动前能力预检
GET  /api/video-projects/:id/workflow-runs/:rid/checkpoints # 查询恢复点
POST /api/video-projects/:id/workflow-runs/:rid/recover     # 从等待人工阶段恢复
```

`POST /api/agent/runs` 接收 `{"message": "..."}` 即可启动完整流程：LLMPlanner → PlanGuard → PlanCompiler → Transient DAG → Orchestrator 执行。

## 素材依赖点云端策略

素材依赖点用于降低对图片/视频生成 API 的绝对依赖。云端生成 `external_generation_request` Artifact，里面包含 Prompt、Negative Prompt、参考素材、目标规格和关联 shot。用户用外部网站生成素材后，桌面端先把文件保存到 local agent，再调用云端登记结果。

云端登记请求只应包含：

- `kind`: `image` 或 `video`。
- `storageType`: 通常为 `local`。
- `storageRef`: local agent 返回的本机引用。
- `mimeType`、`sizeBytes`、`contentHash`。
- `relatedShotId`、`generationRequestId`。
- `source=external_manual_upload`。
- 可选 `referenceAssetIds`、`externalPlatform`、`tags`。

云端不应把用户生成的视频/图片正文作为默认上传对象；需要故障排查时也应走用户确认后的诊断包。

## 云端日志和诊断

云端保留：

- backend structured logs。
- task / node trace（含 `agent_runs` 表的 AgentPlan 和执行记录）。
- `artifact_reviews` 表：PENDING / APPROVED / REJECTED 状态流转，绑定 reviewer 和 timestamps。
- model call and external tool errors。
- uploaded local diagnostic packages。
- artifact metadata: `storage_type=local`、`storage_ref=local://...`、hash、size、版本和 provider/model。
- sanitized node/task output: 只保留本地 manifest、hash、size、trace、状态和脱敏摘要。

云端默认不保留：

- 用户生成脚本、Prompt、分镜正文。
- 用户生成图片、配音音频、最终视频。
- 本地项目文件和缓存文件。

桌面端调用基础模型服务商的长期形态应由本地 agent 直连 Provider；云端只下发远程配置和策略，不代理用户大正文或二进制资产。

桌面端文生文、文生图片、文生视频的 Provider 配置位于本地「系统 → 基础模型 API」，字段为 OpenAI-compatible 的 `baseUrl`、`apiKey`、`model`。云端不保存这些用户 token。

`/api/config/model-provider` 是云端运维 runtime override 接口，用于服务端模型配置热更新；它不是桌面端用户 token 同步接口。封闭内测桌面端不会调用它保存用户 API Key。

推荐诊断流程：

1. 用户在本地生成诊断包。
2. 用户确认并上传云端。
3. 云端按 trace ID、时间窗口、版本号关联本地日志和云端日志。
4. 判断问题归属：本地执行、云端 API、外部服务、配置、网络或用户文件。

## 验证

```bash
cd cloud-backend
go test ./...
go test -race ./...
go run ./evals/video_beta
make api-docs-check

cd ../frontend
npm run lint
npm run test:director
npm run test:security
npm run build

cd ../hyperframes-render-service
npm run build
```
