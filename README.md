# 躺营 AIOS

> **版本：v3.2** — 动态 Agent Runtime：从自然语言一步生成可执行 DAG，含质量门禁 + 人工审核 + 完整包导出。

AIOS 现在按运行边界拆分为三个主要部分：

```text
frontend/                    # React + Electron 桌面/Web 前端
local-backend/               # 本地执行器，无数据库、无 Docker，只处理本地文件/缓存/日志/诊断
cloud-backend/               # 云端 AIOS Core，负责 LLM/API、配置、编排、云端日志和外部集成
                             #   🆕 Dynamic Agent Runtime: Planner → Guard → Compiler → DAG
hyperframes-render-service/  # HyperFrames 渲染服务（Node.js/TypeScript），无 CLI 依赖
```

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

云端运行 `cloud-backend`，包含 Go AIOS Core、PostgreSQL、Redis、Redpanda、MinIO、Nginx 和 LLM/外部服务配置。

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
cd ../frontend && npm run build
```

## 架构文档

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/LOCAL_USAGE.md](docs/LOCAL_USAGE.md)
- [docs/CLOUD_DEPLOYMENT.md](docs/CLOUD_DEPLOYMENT.md)
