# AIOS 视频创作升级 — 已知限制

> 更新日期: 2026-06-17

## 当前版本限制

### 视频 API Provider
- Seedance 等外部视频 API 尚未集成。当前 `Model Gateway` 支持 OpenAI-compatible 适配器 (text/text+image/image generation)，视频 API 适配器待后续版本。
- 视频生产模式支持 `manual_import` (手动导入 MP4)，无需外部视频 API。

### Component DSL
- 口播可视化模式仅支持 10 种组件类型: TITLE, BULLET_LIST, IMAGE_SPLIT, KPI_CARD, QUOTE, COMPARISON, CALLOUT, TIMELINE, TEXT_OVERLAY, BACKGROUND
- 不支持任意 React 组件代码生成 (安全约束)

### 前端
- 新页面 (CreationCenterPage, ProjectWorkbenchPage 等) 的完整 React 组件待 P8 阶段实现。当前后端 API 已就绪。
- SSE 实时进度推送端点 (`/api/progress/stream`) 待实现。
- 前端测试框架 (Vitest + RTL + Playwright) 待配置。

### Electron
- Electron Runner 的完整 Node.js command handlers 待实现。
- Secure credential storage (Keychain) 集成待完成。

### 数据
- Qdrant 向量数据库在当前 MVP 未使用。
- 无正式 database migration 工具 (使用 `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`)。

### 多租户
- 当前为个人/单用户模式，无多用户权限隔离。

### 测试覆盖
- 核心包 (artifact, skillruntime, modelgateway, localrunner, config, video model/service) 有测试覆盖。
- Repository 层需要集成测试 (PostgreSQL 数据库)。
- Handler 层需要 HTTP 集成测试。
- Workflow 包的 DAG compiler/approval/rerun 需要测试。

## 环境要求

### 开发
- Go 1.25+
- Node.js 18+
- Docker (PostgreSQL, Redis, Redpanda, MinIO)
- protoc + protoc-gen-go + protoc-gen-go-grpc (Proto 重新生成时需要)

### 生产
- Docker Compose
- 有效的 OPENAI_API_KEY (或兼容 API)
- 可选: Electron 桌面客户端 (用于本地渲染)

## 下一步计划

1. P8: 前端视频工作台页面 (React 组件)
2. P9: Docker 云端一键部署 + smoke test
3. Seedance/GPT Image 视频 API 适配器
4. 工作流故障恢复和断点续传
5. 多租户支持
