# 本地使用说明

本文面向桌面端用户和交付同学。新的本地形态是：

```text
frontend/Electron UI
  -> local-backend local-agent, 127.0.0.1:18080
  -> cloud-backend API, configured by VITE_CLOUD_API_BASE or TANGYING_CLOUD_API_BASE
```

本地端不启动 PostgreSQL、Redis、Kafka、MinIO，也不要求 Docker。它只负责和用户电脑强相关的事情。

## 本地端职责

- 选择、读取、保存本地文件。
- 运行受控的本地命令或本地处理任务。
- 管理本地缓存、项目文件、产物文件。
- 写本地执行日志。
- 生成诊断包，由用户授权后上传云端分析。

本地端不保存 LLM API Key，不直接调用外部模型服务，不维护云端业务数据。

## 本地数据目录

默认目录：

```text
macOS:   ~/Library/Application Support/TangyingAIOS/
Windows: %APPDATA%/TangyingAIOS/
Linux:   ~/.tangying-aios/
```

目录结构：

```text
TangyingAIOS/
├── cache/
├── projects/
├── artifacts/
├── logs/
│   └── local-agent.jsonl
└── diagnostics/
    └── diagnostics-*.zip
```

可用环境变量覆盖：

```bash
TANGYING_LOCAL_DATA_DIR=/path/to/data
TANGYING_LOCAL_AGENT_ADDR=127.0.0.1:18080
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api
```

## 开发启动

终端 1：

```bash
bash scripts/start-local-backend.sh
```

终端 2：

```bash
bash scripts/start-frontend.sh
```

访问 `http://localhost:3000`。

## 本地 Agent API

```text
GET  /api/local/health
GET  /api/local/paths
POST /api/local/logs
POST /api/local/diagnostics
```

健康检查：

```bash
curl http://127.0.0.1:18080/api/local/health
```

生成诊断包：

```bash
curl -X POST http://127.0.0.1:18080/api/local/diagnostics \
  -H 'Content-Type: application/json' \
  -d '{"reason":"support-request"}'
```

## 桌面打包

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

打包流程会先构建 `local-backend` 的 `tangying-local-agent`，放入 `frontend/resources/bin/`，再执行 Electron 打包。用户打开桌面软件时，Electron 主进程会自动启动本地 agent。

## 故障诊断原则

本地日志可能包含文件路径、文件名、任务上下文。默认由本地生成诊断包，用户确认后上传云端。云端不应主动连接用户电脑拉取日志。
