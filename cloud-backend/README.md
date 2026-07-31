<p align="center">
  <h1 align="center">躺营 AI 视频创作助手</h1>
  <p align="center">一句话生成高质量 AI 视频 — 云端编排 + 本地执行 + 人工审核</p>
</p>

<p align="center">
  <img alt="Release" src="https://img.shields.io/badge/Release-v0.2.2-111827?style=for-the-badge" />
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25-2F80ED?style=for-the-badge" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Required-2496ED?style=for-the-badge" />
</p>

---

## 概述

躺营是一个 AI 视频制片台，将视频创作拆成**选题 → 脚本 → 分镜 → 素材生成 → 审核 → 渲染 → 交付**的可追踪流水线。云端负责编排，用户端负责任务执行和本地文件管理。

**本仓库** 是云端编排后端（Go），桌面客户端和本地 Agent 单独分发。

---

## 核心模块

| 模块 | 路径 | 说明 |
|------|------|------|
| Agent 运行时 | `internal/core/agentruntime/` | LLM 规划 → DAG 编译 → 编排执行 |
| 视频创作服务 | `internal/agents/video/` | 口播/影视双流水线、Shot 管理、QA 门控 |
| 工作流引擎 | `internal/core/workflow/` | 模板管理、阶段审核、Checkpoint |
| 本地 Runner 桥 | `internal/core/localrunner/` | 云端 ↔ 本地 Agent 的任务分发 |
| 可观测性 | `internal/core/logger/` | 结构化日志、请求追踪、诊断 |
| 视频管线 | `video-pipelines/` | 知识口播管线定义 |

## 默认 A-roll —— 一键生成 IP 口播视频

系统内置树懒 IP 角色，通过 `ip_avatar_3d` MCP provider 自动生成 3D 角色口播 A-roll：

1. 用户在导演台输入主题
2. 系统自动生成脚本 → 音频 → Shot 清单
3. `ip_avatar_3d.render_talking_video` 渲染 A-roll 视频层
4. HyperFrames 合成字幕、B-roll 和最终成片

A-roll 是口播视频的默认画面层，确保用户一句话即可产出带 IP 角色的完整视频。

## 快速开始

### 环境要求

- Go 1.25+、Docker Desktop
- 端口 8080（API）、5432（Postgres）、6379（Redis）、9092（Kafka）

### 启动

```bash
# 1. 配置环境
cp .env.example .env
# 编辑 .env 中的密码和密钥

# 2. 启动 Docker 依赖 + 后端
bash scripts/start-cloud-backend.sh

# 3. 验证
curl http://localhost:8080/api/health
```

API 文档: http://localhost:8080/docs

### 开发验证

```bash
go test ./...
```

---

## 版本历史

当前版本 **v0.2.2** — 新增可观测性中间件、Shot QA 门控处理器、结构化请求日志。

详见 [CHANGELOG.md](CHANGELOG.md)。

## 许可证

Apache 2.0 — 详见 [LICENSE](LICENSE)。
