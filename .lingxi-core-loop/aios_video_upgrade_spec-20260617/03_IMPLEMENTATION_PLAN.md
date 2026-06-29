# AIOS 视频创作升级——实施任务计划

## 1. 执行规则

- 每次只完成一个 Phase 或一个小 Task。
- 修改前执行基线测试并记录。
- 每个 Task 必须同时提交实现、测试、文档更新。
- 不允许为让测试通过而删除旧测试或降低断言。
- 真实 API 只做手工 smoke，自动测试使用 Fake Provider。
- Agent 必须更新 `TASK_STATUS.md`。
- 未通过当前 Phase 门禁，不进入下一 Phase。

## 2. Phase 划分

### P0 基线与防回归

#### P0-T01 仓库基线清点

- 核对实际包路径、main wiring、Workflow/CONTROL 实现。
- 列出全部现有测试。
- 运行：
  - `cd aios-core && go test ./...`
  - `cd aios-core && go test -race ./...`
  - `cd frontend && npm ci && npm run build`
  - `./aios-core/scripts/test-apis.sh`（环境可用时）
- 将结果写入 `docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md`。

DoD：

- 基线失败被明确分类为“升级前已有”或“环境问题”。
- 不开始业务代码修改。

#### P0-T02 Feature Flag

新增：

- `VIDEO_CREATION_ENABLED`
- `LOCAL_RUNNER_ENABLED`
- `MODEL_PROVIDER_MODE=fake|real`

DoD：

- Flag 关闭时旧路由和旧页面行为不变。
- 配置单元测试通过。

### P1 基础数据与 Artifact

#### P1-T01 Artifact Core

路径：`internal/core/artifact`

实现：

- Model、Repository、Service。
- MinIO 引用与 inline JSON。
- 版本切换事务。
- content hash。
- 查询 current/history。

测试：

- 新版本成功后旧版本失效。
- 新版本写入失败时旧版本仍 current。
- 重复 content hash 幂等策略。

#### P1-T02 Video Project Domain

路径：`internal/agents/video`

实现：

- Project CRUD。
- 软删除。
- 模式/版本锁定。
- 路由注册。

测试：

- mode 校验。
- skill/workflow version 不可随意修改。
- 分页和软删除。

P1 门禁：

```bash
go test -race ./internal/core/artifact/... ./internal/agents/video/...
go test -race ./...
```

### P2 Skill Runtime

#### P2-T01 Manifest 与 Loader

实现：

- 读取目录。
- JSON Schema 校验。
- 文件引用和 SHA256。
- 健康状态。
- Registry 查询。

#### P2-T02 两个 Skill 骨架

把现有两个 Skill 复制/拆分为版本目录。此 Task 不要求一次重写全部 Prompt，只要求：

- manifest 可加载。
- 每个 stage 有入口文件。
- input/output schema 完整。
- 原 Skill 全文保留在 references 或 migration 目录。

#### P2-T03 Skill API

- `GET /api/skills`
- `GET /api/skills/:name/:version`

P2 门禁：

- 缺文件、坏 YAML、坏 Schema 的测试。
- 两个 Skill 健康。
- 项目能锁定 Skill Version。

### P3 Workflow Run 与 Approval

#### P3-T01 Template Version

在不破坏旧 API 的情况下增加版本。优先新增 version table。

#### P3-T02 WorkflowRun / StageRun

实现：

- 创建 Run。
- 关联现有 Task。
- Stage 映射。
- 状态汇聚。
- snapshot 查询。

#### P3-T03 Approval

复用 CONTROL 节点：

- approve。
- revise/reject。
- 审核记录。
- 幂等。

#### P3-T04 Rerun

- Stage rerun。
- Unit rerun。
- Attempt。
- 下游 invalidation。
- 新 node id/idempotency key。

P3 门禁：

- 纯 Fake Tool 的 DAG 集成测试。
- 重启恢复测试。
- 旧 Workflow API 回归。

### P4 Model Gateway

#### P4-T01 视频创作接口和 Router

实现 capability、provider、request/result、error mapping。

#### P4-T02 Fake Provider

Fixtures 覆盖：

- success
- 429
- timeout
- 500
- invalid schema
- async video job

#### P4-T03 OpenAI-compatible Adapter

先实现：

- text_to_text
- image_to_text
- text_to_image（若接口兼容）

视频 Provider 使用独立 adapter，不强行假设 OpenAI 格式。

#### P4-T04 Usage/Cost/Idempotency

- model_calls 表。
- fingerprint。
- 重试。
- 结果缓存。
- 日志脱敏。

P4 门禁：

- httptest 合同测试。
- 无真实 API Key 时全部自动测试通过。

### P5 AIGC Shot Workflow

#### P5-T01 领域模型

- Script、Character、Scene、Prop、Shot、BoundaryState、ShotPackage。
- typed JSON validation。

#### P5-T02 工作流模板

注册 `aigc-shot-video@1.0.0`。

#### P5-T03 Stage Services

逐个实现：

1. brief/script
2. visual rules/assets
3. shot plan/continuity
4. storyboard/keyframe/video prompt
5. approval
6. provider/manual import
7. frame review
8. timeline manifest

#### P5-T04 手动导入

- multipart MP4。
- 关联 Shot。
- FFprobe/抽帧工具。
- 触发 review。

#### P5-T05 局部重跑

完成 Shot 粒度版本与下游失效。

P5 门禁：

- 3 Shot Fake E2E。
- SHOT_02 故障只重跑 SHOT_02。
- 旧版本可查看。
- 视频 API 未配置时 manual_import 可工作。

### P6 Voice Visual Workflow

#### P6-T01 领域模型和组件白名单

- NarrationBeat
- VisualBeat
- Component DSL
- JSON Schema

#### P6-T02 Renderer

实现 DSL → HyperGenKeyframe project bundle。

第一版只支持 8–12 个稳定组件，不追求任意 React 代码。

#### P6-T03 工作流模板

注册 `voice-visual-video@1.0.0`。

#### P6-T04 图片生成与 Bundle

生成图片为可选 Stage，Bundle 登记为 Artifact。

#### P6-T05 局部 Beat 重跑

只更新指定 VisualBeat 和受影响 Bundle Version。

P6 门禁：

- Fake 观点 → 3 Beat → 3 VisualBeat → Bundle。
- Schema 不允许未知组件。
- 单 Beat 修改不会重生成其他 Beat 产物。

### P7 Local Runner / Electron

#### P7-T01 云端 Local Job API

- register
- heartbeat
- claim
- progress
- complete
- fail
- cancel

#### P7-T02 Electron 配置

- Cloud API Base URL。
- Project Root。
- Token 安全存储。
- Runner 开关。

#### P7-T03 Command Handlers

- HYPERGEN_RENDER
- FFMPEG_PROBE
- FFMPEG_ASSEMBLE
- BUNDLE_EXTRACT

#### P7-T04 Runner 恢复

- Electron 重启后重新查询已领取任务。
- 云端租约过期处理。
- 子进程取消。

P7 门禁：

- Fake Local Runner 集成。
- Playwright Electron smoke。
- 路径逃逸和任意 Shell 测试。

### P8 前端工作台

#### P8-T01 导航与项目列表

保留旧页面，增加新入口。

#### P8-T02 项目工作台

- 阶段树。
- Run snapshot。
- 审核。
- 事件流。

#### P8-T03 Shot 工作台

- Unit 列表。
- Artifact 预览。
- Prompt 编辑。
- 版本。
- 重跑。

#### P8-T04 Voice 工作台

- NarrationBeat。
- VisualBeat。
- Component props。
- Render 任务。

#### P8-T05 任务中心

统一显示 model/local/workflow tasks。

P8 门禁：

- Vitest/RTL。
- Playwright Fake Backend E2E。
- 刷新恢复。
- SSE 断线降级。

### P9 Cloud Docker 与运维

- `docker-compose.cloud.yml`
- Nginx SSE。
- 健康检查。
- volume/backup。
- `.env.cloud.example`
- 一键 smoke。

P9 门禁：

```bash
docker compose -f deploy/docker-compose.cloud.yml up -d
./scripts/smoke-video-creation.sh
```

### P10 全量验收与文档

- 更新 README、AGENTS.md、ARCHITECTURE、API_REFERENCE。
- 生成测试报告。
- 确认旧功能回归。
- 记录已知限制。
- 创建 release checklist。

## 3. Coding Agent 修改路径建议

### 应新增到 Core

- 视频创作 Skill loader。
- 视频创作 Artifact。
- 视频创作 Model Gateway。
- 视频创作 Workflow Run/Approval/Rerun。
- 视频创作 Local Runner 协议。

### 应新增到 Agent

- Video Project。
- Shot/VisualBeat。
- 两条业务工作流。
- 视频领域审核规则。
- Publication package 适配。

### 不应放入 Core

- Seedance 专属业务 Prompt。
- “范进中举”等项目规则。
- HyperGen 具体业务场景模板。
- 自媒体平台标题策略。
- 某个用户的本地路径。
- 外部工具内部算法。

## 4. 每个 Task 的提交模板

```text
Task: P?-T??
Changed:
- ...

Tests added:
- ...

Commands run:
- ...

Result:
- PASS / FAIL

Compatibility:
- Existing API affected? no/yes
- Migration required? no/yes
- Feature flag default: off/on

Remaining risks:
- ...
```

## 5. 禁止事项

- 禁止一次修改全部模块后再补测试。
- 禁止删除现有测试。
- 禁止测试中调用真实收费 API。
- 禁止把 API Key 写入仓库。
- 禁止让 Electron 执行任意云端 Shell。
- 禁止覆盖旧 Artifact。
- 禁止在 HTTP Handler 中等待长视频生成。
- 禁止重复实现已有 Orchestrator 调度。
- 禁止将视频业务逻辑放入 worker/tool registry 核心层。
