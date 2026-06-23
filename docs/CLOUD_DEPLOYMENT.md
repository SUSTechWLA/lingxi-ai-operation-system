# 云端部署说明

本文面向部署和运维同学。新的云端形态是：

```text
cloud-backend/
  -> Go AIOS Core, port 8080
  -> PostgreSQL / Redis / Redpanda / MinIO
  -> cloud-side integrations, account/config services
  -> cloud logs, trace, diagnostics analysis
```

云端负责远程配置、账号、远程编排、云端日志和诊断分析。本地用户不需要安装数据库或 Docker。桌面用户生成的脚本、JSON、图片、音频、视频等个人资产默认保存在本地；用户自配的基础模型 Provider token 也只保存在本机。云端只保存本地引用、hash、size、版本、服务日志和必要的诊断索引。

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
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

或使用根目录脚本：

```bash
bash scripts/start-cloud-backend.sh
```

根目录脚本会默认导出 `SKILL_ROOT=<repo>/cloud-backend/skills`。如果手动启动，`SKILL_ROOT=skills` 会优先按当前工作目录解析；当从仓库根目录启动时，后端会自动兜底到 `cloud-backend/skills`。

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

## Dynamic Agent API（v3.2）

不走 workflow_template 的动态 Agent 入口：

```text
POST /api/agent/runs                                    # 启动 dynamic agent run
GET  /api/agent/runs/:runId                             # 查询 run 状态和 plan
GET  /api/agent/runs/:runId/trace                       # 获取 DAG 执行追踪
GET  /api/agent/runs/:runId/reviews                     # 列出待审核节点
POST /api/agent/runs/:runId/reviews/:reviewId/approve   # 审核通过
POST /api/agent/runs/:runId/reviews/:reviewId/reject    # 审核驳回
```

`POST /api/agent/runs` 接收 `{"message": "..."}` 即可启动完整流程：LLMPlanner → PlanGuard → PlanCompiler → Transient DAG → Orchestrator 执行。

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

推荐诊断流程：

1. 用户在本地生成诊断包。
2. 用户确认并上传云端。
3. 云端按 trace ID、时间窗口、版本号关联本地日志和云端日志。
4. 判断问题归属：本地执行、云端 API、外部服务、配置、网络或用户文件。

## 验证

```bash
cd cloud-backend
go test ./...

cd ../frontend
npm run build
```
