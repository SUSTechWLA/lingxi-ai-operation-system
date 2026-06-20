# 云端部署说明

本文面向部署和运维同学。新的云端形态是：

```text
cloud-backend/
  -> Go AIOS Core, port 8080
  -> PostgreSQL / Redis / Redpanda / MinIO
  -> LLM and external API integrations
  -> cloud logs, trace, diagnostics analysis
```

云端负责配置、账号、LLM/API 对接、远程编排、云端日志和诊断分析。本地用户不需要安装数据库或 Docker。

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
- minio: 云端对象存储。

## 前端云端 API 配置

桌面包和 Web 前端通过以下变量确定云端 API：

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api
```

Web 部署在 nginx 下时，也可以使用同域 `/api`。

## 云端日志和诊断

云端保留：

- backend structured logs。
- task / node trace。
- model call and external tool errors。
- uploaded local diagnostic packages。

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
