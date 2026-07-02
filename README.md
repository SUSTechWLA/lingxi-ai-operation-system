# 躺营 AI Operation System

**语言 / Language:** 中文 | [English](README.en.md)

躺营 AI 自媒体运营助手是一个面向视频创作工作流的本地桌面 + 云端编排系统。它从一句话视频需求出发，生成可审核、可追踪、可回填、可导出的多阶段产物，并在用户端完成素材管理、外部模型交付、本地执行和最终视频查看。

项目 Wiki：

- [中文 Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki)
- [English Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki/English)

## 核心能力

- **两类视频工作流**：口播 / 知识类视频，以及影视化 AIGC shot 视频。
- **云端编排**：cloud backend 负责任务规划、Agent runtime、审核门、持久化和诊断。
- **本地执行**：local backend 负责本地文件、产物、缓存、日志、渲染和桌面工具执行。
- **外部模型交付**：不强制用户在系统内配置第三方模型 API，可复制提示词和参考信息到外部模型网站，再把结果上传回项目。
- **人工审核门**：脚本、分镜、预览、渲染和交付等关键阶段可查看、通过、驳回、编辑或重生成。

## 运行边界

```text
frontend/                       React + Electron 桌面客户端
local-backend/                  本地桌面 agent 和 local runner
cloud-backend/                  Go AIOS Core 云端后端和编排
hyperframes-render-service/     可选本地渲染服务
```

本地运行时负责本地文件、缓存、产物、日志、诊断和桌面执行。它不应该依赖 PostgreSQL、Redis、Kafka、MinIO、Docker 或 LLM API Key。

云端后端负责 API 集成、动态 Agent 计划、编排、持久化、远程配置和云端诊断。

## 核心流程

```text
用户提示词
  -> POST /api/agent/runs
  -> LLMPlanner / 客户端模型配置
  -> PlanGuard
  -> PlanCompiler
  -> 临时 DAG
  -> 产物审核门
  -> local runner 素材生成 / 渲染交付
  -> 最终视频产物在客户端可见
```

中间产物可审核。审核门支持通过、驳回、编辑或在支持的阶段重生成。

## 本地开发

启动本地后端：

```bash
bash scripts/start-local-backend.sh
```

启动桌面前端：

```bash
bash scripts/start-frontend.sh
```

启动云端后端：

```bash
bash scripts/start-cloud-backend.sh
```

构建桌面应用：

```bash
bash scripts/build-local-desktop.sh
```

macOS 构建产物会写入：

```text
frontend/release/
```

## 验证

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../frontend && npm run build
```

云端后端并发检查：

```bash
cd cloud-backend && go test -race ./...
```

## API 入口

Cloud backend：

```text
http://localhost:8080/docs
http://localhost:8080/openapi.json
```

Local agent：

```text
http://localhost:18080/api/local/docs
http://localhost:18080/api/local/openapi.json
```

## Release 分支范围

`release` 分支保留核心运行时代码、构建文件、运行时 skill 配置和项目 README。辅助文档、smoke assets、eval fixtures 和仅开发用脚本不进入 release。
