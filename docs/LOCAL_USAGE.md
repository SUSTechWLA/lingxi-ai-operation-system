# 本地使用说明

> **版本：v4.0 closed beta** — 本地端服务 Director Studio、素材依赖点回填、本地文件/日志/诊断和本机模型 Provider 配置。

本文面向桌面端用户和交付同学。新的本地形态是：

```text
frontend/Electron UI
  -> local-backend local-agent, 127.0.0.1:18080
  -> cloud-backend API, configured by VITE_CLOUD_API_BASE or TANGYING_CLOUD_API_BASE
     -> Dynamic Agent: POST /api/agent/runs（自然语言 → 自动规划 → 执行）
```

本地端不启动 PostgreSQL、Redis、Kafka、MinIO，也不要求 Docker。它只负责和用户电脑强相关的事情。

## 本地端职责

- 选择、读取、保存本地文件。
- 运行受控的本地命令或本地处理任务。
- 管理本地缓存、项目文件、产物文件。
- 写本地执行日志。
- 生成诊断包，由用户授权后上传云端分析。
- 接收“素材依赖点”回填文件，计算 hash/size/mime type，并返回本机 `storageRef`。

本地端不内置平台 LLM API Key，不维护云端业务数据。用户可以在「系统 → 基础模型 API」里为文生文、文生图片、文生视频分别配置 OpenAI-compatible Provider；这些 token 只保存在用户本机，不上传云端。

桌面端渲染进程不暴露任意命令执行 IPC，也不提供自动发布入口。需要本地执行的媒体任务由 local agent / Local Runner 的受控协议承接。

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
├── config/
│   └── model-providers.json
├── projects/
├── artifacts/
│   └── <projectId>/<artifactId>/
│       ├── content
│       └── metadata.json
├── logs/
│   └── local-agent.jsonl
└── diagnostics/
    └── diagnostics-*.zip
```

可用环境变量覆盖：

```bash
TANGYING_LOCAL_DATA_DIR=/path/to/data
TANGYING_LOCAL_AGENT_ADDR=127.0.0.1:18080
VITE_LOCAL_AGENT_URL=http://127.0.0.1:18080
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
GET  /api/local/model-providers
PUT  /api/local/model-providers
POST /api/local/artifacts
GET  /api/local/artifacts/:id?projectId=<projectId>
DELETE /api/local/artifacts/:id?projectId=<projectId>
DELETE /api/local/projects/:id
POST /api/local/logs
POST /api/local/diagnostics
GET  /api/local/openapi.json
GET  /api/local/docs
```

健康检查：

```bash
curl http://127.0.0.1:18080/api/local/health
```

API 文档（Swagger UI）：浏览器打开 `http://127.0.0.1:18080/api/local/docs`，或获取原始 OpenAPI 规范 `http://127.0.0.1:18080/api/local/openapi.json`。

读取基础模型 API 设置：

```bash
curl http://127.0.0.1:18080/api/local/model-providers
```

保存基础模型 API 设置：

```bash
curl -X PUT http://127.0.0.1:18080/api/local/model-providers \
  -H 'Content-Type: application/json' \
  -d '{
    "providers": {
      "text_to_text": {
        "baseUrl": "https://api.openai.com/v1",
        "model": "gpt-4.1",
        "apiKey": "sk-..."
      },
      "text_to_image": {
        "baseUrl": "https://api.openai.com/v1",
        "model": "gpt-image-1",
        "apiKey": "sk-..."
      },
      "text_to_video": {
        "baseUrl": "https://api.openai.com/v1",
        "model": "sora",
        "apiKey": "sk-..."
      }
    }
  }'
```

字段遵循 OpenAI-compatible 习惯：`baseUrl`、`apiKey`、`model`。`GET` 返回时不会回显完整 token，只返回 `hasApiKey` 和 `apiKeyPreview`；`PUT` 中某个能力的 `apiKey` 为空时会保留本机已有 token。

保存本地产物：

```bash
curl -X POST http://127.0.0.1:18080/api/local/artifacts \
  -H 'Content-Type: application/json' \
  -d '{
    "id":"art-1",
    "projectId":"vp-1",
    "storageRef":"local://projects/vp-1/artifacts/script/content/hash/script.md",
    "mimeType":"text/markdown; charset=utf-8",
    "content":"## 用户脚本",
    "metadata":{"cloudPayloadStored":false}
  }'
```

文本类产物使用 `content`；图片、音频、视频等二进制产物使用 `contentBase64`，读取时也会以 `contentBase64` 返回。

保存素材依赖点回填文件：

```bash
curl -X POST http://127.0.0.1:18080/api/local/artifacts \
  -F 'id=extgen-shot-01-result' \
  -F 'projectId=vp-1' \
  -F 'mimeType=video/mp4' \
  -F 'metadata={
    "artifactType":"external_manual_generation_result",
    "externalGenerationRequestId":"extgen-shot-01",
    "generationKind":"video",
    "relatedShotId":"shot-01",
    "source":"external_manual_upload",
    "cloudPayloadStored":false,
    "localOnly":true
  }' \
  -F 'file=@shot-01.mp4'
```

本地 agent 返回 `storageRef`、`contentHash`、`sizeBytes` 后，前端再调用云端：

```text
POST /api/video-projects/:id/external-generation-results
```

云端只登记本地引用和依赖关系，不接收用户生成的视频正文。

读取本地产物：

```bash
curl 'http://127.0.0.1:18080/api/local/artifacts/art-1?projectId=vp-1'
```

删除单个本地产物：

```bash
curl -X DELETE 'http://127.0.0.1:18080/api/local/artifacts/art-1?projectId=vp-1'
```

删除一个本地项目的项目文件、产物和缓存：

```bash
curl -X DELETE http://127.0.0.1:18080/api/local/projects/vp-1
```

云端只保存 `storageRef`、hash、size、版本和服务 metadata。用户生成的脚本、JSON、图片、音频、视频正文应保存在本地 `artifacts/` 目录。

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
