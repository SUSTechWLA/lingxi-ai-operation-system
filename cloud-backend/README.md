# Tangying Cloud Backend

`cloud-backend/` 是云端控制面：保留 Go AIOS Core、数据库、缓存、消息队列、对象存储、外部平台集成和云端日志。封闭内测不在云端提供模型 API 服务；桌面客户端会把本机 OpenAI-compatible Provider 配置随请求传入，用户 token 不会同步到云端。

本地桌面能力不放在这里；本机文件、缓存、执行日志和诊断包由根目录的 `local-backend/` 提供。

## 开发启动

```bash
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

也可以从仓库根目录运行：

```bash
bash scripts/start-cloud-backend.sh
```

## Docker Compose

```bash
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

Compose project 名称以当前目录为准，应为 `cloud-backend`。

本地云端依赖包含：

- `postgres`: 主数据库
- `redis`: 会话与缓存
- `redpanda`: Kafka 兼容事件流
- `minio`: 媒体对象存储
- `qdrant`: 向量检索服务

## 关键环境变量

| 变量 | 说明 |
|------|------|
| `VIDEO_CREATION_ENABLED` | 启用视频创作业务 |
| `MODEL_PROVIDER_MODE` | 封闭内测保持 `fake`，真实模型调用由客户端 provider 随请求传入 |
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

完整架构与 API 见 `docs/ARCHITECTURE.md` 和 `docs/API_REFERENCE.md`。
