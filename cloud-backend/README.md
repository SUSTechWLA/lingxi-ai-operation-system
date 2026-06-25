# Tangying Cloud Backend

`cloud-backend/` 是云端控制面：保留 Go AIOS Core、数据库、缓存、消息队列、对象存储、LLM/API 配置、外部平台集成和云端日志。

本地桌面能力不放在这里；本机文件、缓存、执行日志和诊断包由根目录的 `local-backend/` 提供。

## 开发启动

```bash
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

也可以从仓库根目录运行：

```bash
bash scripts/start-cloud-backend.sh
```

## 云端部署

```bash
cd deploy
cp .env.cloud.example .env.cloud
# 填 OPENAI_API_KEY、数据库/MinIO 密码等
docker compose --env-file .env.cloud -f docker-compose.cloud.yml up -d --build
```

部署包含：

- `backend`: Go 单体服务，端口 `8080`
- `postgres`: 主数据库
- `redis`: 会话与缓存
- `redpanda`: Kafka 兼容事件流
- `minio`: 媒体对象存储
- `nginx`: 前端静态文件和 `/api` 反向代理

## 关键环境变量

| 变量 | 说明 |
|------|------|
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` | 云端模型访问配置 |
| `VIDEO_CREATION_ENABLED` | 启用视频创作业务 |
| `MODEL_PROVIDER_MODE` | `fake` 用于测试，`real` 用于生产 |
| `SKILL_ROOT` | Skill 包目录，容器内默认 `/app/skills` |
| `LOCAL_RUNNER_ENABLED` | 云端保持 `false`，本地执行交给 `local-backend` |
| `SKILL_CAPABILITY_ROOT` | Skill Capability 根目录（Dynamic Agent 工具注册），默认 `skill-capabilities` |
| `LEGACY_SKILL_WORKFLOW_AUTOREGISTER` | 是否自动将 legacy Skill 注册为 Workflow 模板，默认 `false` |
| `AGENT_PLANNER_MODE` | Planner 模式：`llm` / `heuristic` / `hybrid`（默认） |
| `AGENT_PLANNER_MAX_TOOLS` | HybridToolRetriever 检索 TopK 候选工具数，默认 `8` |

## 验证

```bash
go test ./...
curl http://localhost:8080/api/health
curl http://localhost:8080/api/health/ready
./scripts/test-apis.sh
```

完整架构与 API 见根目录 `docs/ARCHITECTURE.md`、`docs/CLOUD_DEPLOYMENT.md`。
