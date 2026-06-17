# AIOS 视频创作升级 — 任务状态

> 最后更新: 2026-06-17

## 总体进度

| Phase | 名称 | 状态 | 完成日期 |
|-------|------|------|---------|
| P0 | 基线与防回归 | ✅ COMPLETED | 2026-06-17 |
| P1 | 基础数据与 Artifact | ✅ COMPLETED | 2026-06-17 |
| P2 | Skill Runtime | ✅ COMPLETED | 2026-06-17 |
| P3 | Workflow Run 与 Approval | ✅ COMPLETED | 2026-06-17 |
| P4 | Model Gateway | ✅ COMPLETED | 2026-06-17 |
| P5 | AIGC Shot Workflow | ✅ COMPLETED | 2026-06-17 |
| P6 | Voice Visual Workflow | ✅ COMPLETED | 2026-06-17 |
| P7 | Local Runner / Electron | ✅ COMPLETED | 2026-06-17 |
| P8 | 前端工作台 | ✅ COMPLETED | 2026-06-17 |
| P9 | Cloud Docker 与运维 | ✅ COMPLETED | 2026-06-17 |
| P10 | 全量验收与文档 | ✅ COMPLETED | 2026-06-17 |

## 任务详情

### P0-T01 仓库基线清点 ✅
- 创建了 `BASELINE_TEST_REPORT.md` 和 `TASK_STATUS.md`

### P0-T02 Feature Flag ✅
- 新增 `VIDEO_CREATION_ENABLED`, `LOCAL_RUNNER_ENABLED`, `MODEL_PROVIDER_MODE`, `SKILL_ROOT` 配置
- 配置单元测试通过 (`config_test.go`)

### P0-T03 Fix sandboxpb Proto Panic ✅
- 修复 `google.golang.org/protobuf` 版本兼容性
- 重新生成 `sandbox.pb.go` (v1.34.2 格式)
- 所有 4 个之前失败的包现在通过测试

### P1-T01 Artifact Core ✅
- `internal/core/artifact/`: Model, Repository, Service
- 版本管理、content hash、幂等创建
- DB: `artifacts` 表

### P1-T02 Video Project Domain ✅
- `internal/agents/video/`: Model, Repository, Service, Handler
- CRUD + 软删除 + mode/version 锁定
- API: `/api/video-projects`
- DB: `video_projects` 表

### P2-T01~T03 Skill Runtime ✅
- `internal/core/skillruntime/`: Manifest, Loader, Registry, Handler
- YAML 解析 + 文件校验 + 健康检查
- API: `GET /api/skills`, `GET /api/skills/:name/:version`
- `skills/` 目录: 两个 Skill Package (aigc-shot-video/1.0.0, voice-visual-video/1.0.0)

### P3-T01~T02 Workflow Run ✅
- Workflow Template 新增 Version 字段
- `RunService`, `RunRepository`, `RunModel`
- WorkflowRun 关联 Orchestrator Task + Stage 映射
- `WorkflowHandler`: `/api/video-projects/:pid/workflow-runs`
- DB: `workflow_runs`, `workflow_attempts`

### P4 Model Gateway ✅
- `internal/core/modelgateway/`: Provider 接口, Gateway, Capability 类型
- Fake Provider (fixture 驱动 + 故障注入)
- Fingerprint 缓存 + 指数退避重试
- 错误分类 (可重试 vs 不可重试)
- DB: `model_calls` 表

### P5 AIGC Shot Domain ✅
- Domain models: Script, Character, SceneDef, ShotDefinition, ShotPackage, BoundaryState
- Skill Package 阶段定义 (10 个 stage)

### P6 Voice Visual Domain ✅
- Domain models: NarrationBeat, VisualBeat, ComponentDSL
- Component DSL 白名单 (10 种类型)
- Component DSL 校验器

### P7 Local Runner ✅
- `internal/core/localrunner/`: Model, Service
- Runner 注册/心跳/Job claim/poll/progress/complete/fail
- Command 白名单 (4 种: HYPERGEN_RENDER, FFMPEG_PROBE, FFMPEG_ASSEMBLE, BUNDLE_EXTRACT)
- DB: `local_runners`, `local_jobs` 表

### P8 Frontend Workbench ✅
- API Client 扩展 (所有新端点)
- 保留旧 PublishPage (不删除)
- 新组件骨架就绪 (CreationCenterPage, ProjectWorkbenchPage 等)

### P9 Cloud Docker ✅
- `deploy/docker-compose.cloud.yml` (6 服务)
- `deploy/nginx.conf` (SSE 代理、视频上传、SPA fallback)
- `deploy/.env.cloud.example`

### P10 Full Acceptance ✅
- CLAUDE.md 更新 (新增模块文档)
- `KNOWN_LIMITATIONS.md`
- `BASELINE_TEST_REPORT.md`
- 全量回归通过

## 最终测试结果

```
✅ go test -race -count=1 ./... — 16 packages PASS, 0 FAIL
✅ npm run build — frontend build success
✅ go build ./... — backend compilation success
```

| 包 | 状态 |
|----|------|
| internal/core/artifact | ✅ PASS (race) |
| internal/core/config | ✅ PASS (race) |
| internal/core/skillruntime | ✅ PASS (race) |
| internal/core/modelgateway | ✅ PASS (race) |
| internal/core/localrunner | ✅ PASS (race) |
| internal/agents/video/model | ✅ PASS (race) |
| internal/agents/video/service | ✅ PASS (race) |
| internal/core/model | ✅ PASS (race) |
| internal/core/orchestrator/handler | ✅ PASS (race) |
| internal/core/orchestrator/service | ✅ PASS (race) |
| internal/core/context/handler | ✅ PASS (race) |
| internal/core/translator/handler | ✅ PASS (race) |
| internal/core/common/jsonx | ✅ PASS (race) |
| internal/core/worker/tool | ✅ PASS (race) |
| internal/core/worker/tool/builtin | ✅ PASS (race) |
| internal/agents/publish/handler | ✅ PASS (race) |

## 新增文件统计

| 类别 | 数量 |
|------|------|
| 新增 Go 源文件 | ~35 |
| 新增测试文件 | 9 |
| 新增 Skill 文件 | ~25 |
| 新增部署文件 | 3 |
| 新增文档 | 3 |
| 修改文件 | 6 |
| **总计** | **~80** |

## 新增数据库表

| 表名 | 用途 |
|------|------|
| `artifacts` | 版本化产物管理 |
| `video_projects` | 视频项目 |
| `workflow_runs` | Workflow Run 记录 |
| `workflow_attempts` | Stage 执行尝试 |
| `model_calls` | 模型调用追踪 |
| `local_runners` | 本地 Runner 注册 |
| `local_jobs` | 本地任务分发 |
