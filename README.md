# 躺营 AIOS — 视频创作 Agent

> **版本：v4.0 closed beta** — 面向封闭内测的视频创作 Agent 系统，核心是 Director Studio + Dynamic Agent Runtime。

躺营 AIOS 当前不是泛办公平台，也不是自动发布工具。它聚焦一条可验证的视频生产闭环：创作者输入视频想法，系统规划并执行创作流程，生成脚本、分镜、Prompt、素材清单、可审核产物和发布包。封闭内测目标是让 1-3 位熟悉创作者稳定跑通从想法到可手动发布材料的流程。

当前主链路：

```text
视频想法
→ Dynamic Agent Runtime 规划
→ 脚本 / 分镜 / Shot List / Prompt
→ 素材依赖点
→ 用户外部生成素材并回填
→ Artifact Review 人工审核
→ Local Runner / HyperFrames 渲染或校验
→ 小红书 / Bilibili 发布包导出
```

系统不会要求用户在项目开始前批量上传素材。当某个 shot 需要参考图、关键帧或 AIGC 视频片段时，导演台会停在“素材依赖点”，展示 Prompt、Negative Prompt、参考图和目标规格。用户可以用任意外部图片/视频网站生成素材，再上传回这个依赖点；本地 agent 保存文件，云端只登记 `storage_ref`、hash、size、shot 关联和 trace。

桌面端不暴露任意命令执行或自动发布入口。用户在桌面端配置的模型 API Key 仅保存在本机 `local-backend`，不会同步到云端。

## 运行边界

躺营 AIOS 按运行边界拆分为四个部分：

```text
frontend/                    # React + Electron 桌面/Web 前端
local-backend/               # 本地执行器，无数据库、无 Docker，只处理本地文件/缓存/日志/诊断
cloud-backend/               # 云端 AIOS Core，负责账号、编排、云端日志、外部集成和产物索引
                             #   Dynamic Agent Runtime: Planner → Guard → Compiler → DAG
hyperframes-render-service/  # HyperFrames 渲染服务（Node.js/TypeScript），无 CLI 依赖
```

| 边界 | 当前职责 | 不负责 |
|------|----------|--------|
| `frontend/` | React + Electron UI、登录、导演工作台、审核、素材依赖点回填、本地设置 | 任意命令执行、自动发布 |
| `local-backend/` | 本地文件、缓存、产物、日志、诊断包、本机模型 Provider 配置 | PostgreSQL、Redis、Kafka、MinIO、云端业务编排 |
| `cloud-backend/` | 账号、Dynamic Agent Runtime、DAG 编排、Artifact/Review、视频项目 API、云端日志 | 提供模型 API 服务、保存用户本地大文件、保存桌面用户 API Key |
| `hyperframes-render-service/` | HyperFrames lint、snapshot、render HTTP 服务 | 业务编排、用户账户、资产索引 |

## 当前关键能力

- Director Studio：当前默认产品入口，覆盖项目创建、阶段视图、Artifact 审核、Trace、素材依赖点和发布包导出。
- Dynamic Agent Runtime：`LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`，入口为 `POST /api/agent/runs`。
- Artifact Review：关键产物进入 PENDING / APPROVED / REJECTED 流程，返工会触发下游 stale 标记。
- 素材依赖点：`external_generation_request` 展示外部生成所需 Prompt 与参考材料，回填后登记为 `external_generation_result`。
- Local Runner：云端调度，本地执行文件和媒体任务，只回传 manifest、hash、状态和日志。
- HyperFrames Render Service：通过 HTTP 提供 `/health`、`/lint`、`/snapshot`、`/render`、`/render/stream`。
- 发布包导出：封闭内测提供手动发布材料，不触发自动发布。

## 本地用户

本地用户只需要桌面应用。桌面应用会自动启动 `local-backend` 的轻量 local-agent，用于本机文件、缓存、日志和诊断包。

开发模式：

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

桌面打包：

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

详细说明见 [docs/LOCAL_USAGE.md](docs/LOCAL_USAGE.md)。

## 云端部署

云端运行 `cloud-backend`，包含 Go AIOS Core、PostgreSQL、Redis、Redpanda、MinIO、Nginx 和 HyperFrames Render Service。LLM、文生图片、文生视频 Provider 均由桌面客户端本机配置后随请求传入；云端不提供模型 API 服务，也不保存用户 token。

开发模式：

```bash
bash scripts/start-cloud-backend.sh
```

Docker Compose 部署：

```bash
cd cloud-backend/deploy
cp .env.cloud.example .env.cloud
docker compose --env-file .env.cloud -f docker-compose.cloud.yml up -d --build
```

详细说明见 [docs/CLOUD_DEPLOYMENT.md](docs/CLOUD_DEPLOYMENT.md)。

## 常用验证

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../cloud-backend && go run ./evals/video_beta
cd ../cloud-backend && make api-docs-check
cd ../frontend && npm run lint
cd ../frontend && npm run test:director
cd ../frontend && npm run test:security
cd ../frontend && npm run build
cd ../hyperframes-render-service && npm run build
```

## 架构文档

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/LOCAL_USAGE.md](docs/LOCAL_USAGE.md)
- [docs/CLOUD_DEPLOYMENT.md](docs/CLOUD_DEPLOYMENT.md)
- [docs/BETA_USAGE.md](docs/BETA_USAGE.md)
- [cloud-backend/docs/ONBOARDING.md](cloud-backend/docs/ONBOARDING.md)
