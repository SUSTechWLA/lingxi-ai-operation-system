# AIOS 自媒体视频创作能力升级——详细设计

## 1. 设计摘要

本升级采用“核心能力增量扩展 + 视频领域适配器”的方式：

```text
React / Electron
        │ HTTPS + SSE
        ▼
Go AIOS Core
├── internal/core
│   ├── workflow         复用并扩展模板、Run、Stage、Approval
│   ├── orchestrator     保持现有 DAG 调度和状态机
│   ├── worker           保持工具注册和执行路由
│   ├── skillruntime     新增：Skill Package 加载与版本
│   ├── modelgateway     新增：外部多模态 API 统一网关
│   ├── artifact         新增：产物、版本和 MinIO 引用
│   └── localrunner      新增：Electron 本地任务协议
│
└── internal/agents/video
    ├── project          视频项目
    ├── creation         Shot / VisualBeat
    ├── review           审核和返修
    ├── handler
    ├── service
    └── workflows        两套内置工作流注册
```

现有 Orchestrator 仍是节点执行真相，不另写一套调度器。新增 WorkflowRun 负责产品级阶段和项目映射，内部编译或实例化为现有 DAG。

## 2. 关键设计决策

### ADR-01 不重写 Orchestrator

原因：

- 已支持 DAG、依赖、条件、重试、暂停、恢复、取消。
- 已有 Redpanda + Outbox。
- 重写会扩大回归面。

做法：

- `WorkflowRun` 关联 `orchestrator_task_id`。
- Stage 与 DAG node/group 建立 mapping。
- CONTROL 节点用于人工审核。
- Foreach/Parallel 在工作流实例化时展开为现有 DAG 节点。

### ADR-02 Skill 不直接等于 Workflow

- Skill：方法、Prompt、Schema、参考和质量要求。
- Workflow：确定性阶段、依赖、审核和重跑边界。
- Tool：一次确定性动作。
- Agent：在受控边界内判断和生成。

### ADR-03 生产运行不直接依赖 Codex CLI

现有 Codex + Skill 可保留为 Skill 开发和调试工具。正式产品运行时将 Skill 拆为：

- stage instructions
- input/output schema
- templates/references
- workflow nodes
- provider/tool calls

### ADR-04 视频 API 必须支持“手动导入模式”

第一版不能依赖 Seedance 等平台一定有稳定开放 API。`generation_mode`：

- `provider_api`
- `manual_import`

二者共用 Shot Package、Artifact、Review 和版本系统。

### ADR-05 组件 DSL 代替自由生成前端工程

口播模式中，模型只输出允许的组件 DSL。Renderer 负责将 DSL 转成 HyperGenKeyframe 代码，降低每次生成代码的不确定性。

## 3. 包结构

### 3.1 新增 Core 包

```text
aios-core/internal/core/
├── artifact/
│   ├── model.go
│   ├── repository.go
│   ├── service.go
│   ├── storage.go
│   └── service_test.go
├── skillruntime/
│   ├── manifest.go
│   ├── loader.go
│   ├── registry.go
│   ├── validator.go
│   └── loader_test.go
├── modelgateway/
│   ├── types.go
│   ├── gateway.go
│   ├── router.go
│   ├── usage.go
│   ├── provider.go
│   ├── providers/fake/
│   └── providers/openai_compatible/
└── localrunner/
    ├── model.go
    ├── repository.go
    ├── service.go
    ├── handler.go
    └── service_test.go
```

### 3.2 扩展 Workflow

```text
internal/core/workflow/
├── model.go               保留 Template，增加 Version/InputSchema
├── run_model.go           WorkflowRun / StageRun / Attempt
├── run_repository.go
├── run_service.go
├── compiler.go            高级 Stage → 现有 DAG
├── approval_service.go
├── rerun_service.go
└── *_test.go
```

### 3.3 视频领域 Agent

```text
internal/agents/video/
├── handler/
│   ├── project_handler.go
│   ├── workflow_handler.go
│   ├── unit_handler.go
│   └── review_handler.go
├── model/
│   ├── project.go
│   ├── asset.go
│   ├── shot.go
│   ├── voice.go
│   └── review.go
├── repository/
├── service/
│   ├── project_service.go
│   ├── aigc_service.go
│   ├── voice_service.go
│   ├── review_service.go
│   └── timeline_service.go
└── workflows/
    ├── register.go
    ├── aigc_shot_video_v1.go
    └── voice_visual_video_v1.go
```

## 4. Skill Package

### 4.1 推荐目录

```text
skills/
├── aigc-shot-video/
│   └── 1.0.0/
│       ├── skill.yaml
│       ├── SKILL.md
│       ├── stages/
│       │   ├── script.md
│       │   ├── visual_design.md
│       │   ├── shot_plan.md
│       │   ├── storyboard.md
│       │   ├── keyframe.md
│       │   ├── video_prompt.md
│       │   └── review.md
│       ├── schemas/
│       ├── templates/
│       ├── references/
│       └── examples/
└── voice-visual-video/
    └── 1.0.0/
        └── ...
```

### 4.2 加载策略

启动时：

1. 扫描 `SKILL_ROOT`。
2. 解析 `skill.yaml`。
3. 用 JSON Schema 校验。
4. 校验引用文件存在和 SHA256。
5. 注册 `{name, version}`。
6. 错误 Skill 标记 `UNHEALTHY`，不阻止其他 Skill。
7. 提供 `/api/skills` 查询健康状态。

### 4.3 上下文构建

Stage 调用模型时只加载：

```text
系统通用约束
+ 当前 Skill stage instruction
+ 必要 references
+ 当前项目摘要
+ 当前 unit 上下文
+ 上下游边界状态
+ 输出 JSON Schema
```

不加载完整 Skill、全部历史和全部 Shot。

## 5. Workflow 高级定义与编译

### 5.1 模板模型

建议增加：

```go
type Template struct {
    ID          string
    Version     string
    Name        string
    Description string
    Category    string
    InputSchema json.RawMessage
    Definition  json.RawMessage
    Enabled     bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

数据库主键建议 `(id, version)`，旧 `id` 主键表不直接破坏。迁移可先新增 `workflow_template_versions`，旧表继续兼容。

### 5.2 Stage 定义

```go
type StageDefinition struct {
    ID               string
    Name             string
    Kind             string // AGENT, TOOL, APPROVAL, PARALLEL, FOREACH, LOCAL_TOOL
    SkillStage       string
    Tool             string
    DependsOn        []string
    Input            map[string]any
    OutputSchemaRef  string
    ApprovalRequired bool
    RerunScope       string // SELF, UNIT_DOWNSTREAM, STAGE_DOWNSTREAM
}
```

### 5.3 编译到现有 DAG

- `AGENT` → LLM node 或受控 Agent tool node。
- `TOOL` → TOOL node。
- `APPROVAL` → CONTROL node。
- `PARALLEL` → 多个拥有相同依赖的节点。
- `FOREACH` → 实例化时按 Unit 展开。
- `LOCAL_TOOL` → TOOL node，工具名为 `local_job_dispatcher`。

### 5.4 Stage 状态映射

```text
全部映射节点 CREATED/READY       -> PENDING
任一 RUNNING/RETRYING            -> RUNNING
CONTROL 等待操作                 -> WAITING_APPROVAL
全部 SUCCESS/SKIPPED             -> SUCCEEDED
任一最终 FAILED                  -> FAILED
被重跑逻辑标记                   -> INVALIDATED
```

## 6. 数据模型

### 6.1 video_projects

```sql
CREATE TABLE video_projects (
  id UUID PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  mode VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  current_stage VARCHAR(128),
  skill_name VARCHAR(128) NOT NULL,
  skill_version VARCHAR(32) NOT NULL,
  workflow_id VARCHAR(128) NOT NULL,
  workflow_version VARCHAR(32) NOT NULL,
  aspect_ratio VARCHAR(16),
  target_duration_sec INT,
  language VARCHAR(16) DEFAULT 'zh-CN',
  settings JSONB NOT NULL DEFAULT '{}',
  local_path_hint TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);
```

### 6.2 workflow_runs

```sql
CREATE TABLE workflow_runs (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  workflow_id VARCHAR(128) NOT NULL,
  workflow_version VARCHAR(32) NOT NULL,
  orchestrator_task_id VARCHAR(128),
  status VARCHAR(32) NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  input JSONB NOT NULL DEFAULT '{}',
  output JSONB NOT NULL DEFAULT '{}',
  trace_id VARCHAR(128) NOT NULL,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.3 workflow_stage_runs

```sql
CREATE TABLE workflow_stage_runs (
  id UUID PRIMARY KEY,
  workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id),
  stage_id VARCHAR(128) NOT NULL,
  unit_id UUID,
  status VARCHAR(32) NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  node_ids JSONB NOT NULL DEFAULT '[]',
  input JSONB NOT NULL DEFAULT '{}',
  output JSONB NOT NULL DEFAULT '{}',
  error_code VARCHAR(128),
  error_message TEXT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  UNIQUE(workflow_run_id, stage_id, unit_id, attempt)
);
```

### 6.4 creation_units

统一 Shot 与 VisualBeat：

```sql
CREATE TABLE creation_units (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  kind VARCHAR(32) NOT NULL,
  sequence_no INT NOT NULL,
  start_sec NUMERIC,
  end_sec NUMERIC,
  status VARCHAR(32) NOT NULL,
  version INT NOT NULL DEFAULT 1,
  data JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, kind, sequence_no, version)
);
```

`data` 内应由 Go typed struct 校验，不允许业务层长期依赖任意 map。

### 6.5 artifacts

```sql
CREATE TABLE artifacts (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  workflow_run_id UUID,
  stage_id VARCHAR(128),
  unit_id UUID,
  kind VARCHAR(32) NOT NULL,
  name VARCHAR(255) NOT NULL,
  version INT NOT NULL,
  parent_artifact_id UUID,
  storage_type VARCHAR(16) NOT NULL,
  storage_ref TEXT,
  inline_json JSONB,
  mime_type VARCHAR(128),
  size_bytes BIGINT,
  content_hash VARCHAR(128),
  prompt_hash VARCHAR(128),
  provider VARCHAR(128),
  model VARCHAR(128),
  is_current BOOLEAN NOT NULL DEFAULT TRUE,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

写入新版本时，在同一事务中将旧 current 设为 false。

### 6.6 reviews

```sql
CREATE TABLE reviews (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  stage_id VARCHAR(128),
  unit_id UUID,
  artifact_id UUID,
  review_type VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  score NUMERIC,
  issues JSONB NOT NULL DEFAULT '[]',
  comment TEXT,
  reviewer VARCHAR(128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.7 model_calls

记录 Provider 调用、fingerprint、成本和幂等。

### 6.8 local_runners / local_jobs

Runner heartbeat 和本地任务状态。Local Job Payload 只能是结构化命令：

```json
{
  "commandType": "HYPERGEN_RENDER",
  "projectId": "...",
  "bundleArtifactId": "...",
  "outputRelativePath": "exports/preview.mp4",
  "options": {"composition": "main"}
}
```

禁止：

```json
{"shell": "rm -rf ..."}
```

## 7. 领域模型

### 7.1 AIGC Shot

```go
type ShotData struct {
    ShotCode       string
    DurationSec    float64
    NarrativeGoal  string
    SceneID        string
    CharacterIDs   []string
    PropIDs        []string
    StartState     BoundaryState
    Timeline       []TimelineEvent
    EndState       BoundaryState
    NegativeRules  []string
}
```

`BoundaryState` 至少包含人物位置、身体姿态、视线、手部、道具、相机、光线。

### 7.2 ShotPackage

```go
type ShotPackage struct {
    ShotID             string
    StoryboardArtifact string
    KeyframeArtifacts  []string
    VideoPromptArtifact string
    ReferenceArtifacts []string
    ExpectedStart      BoundaryState
    ExpectedEnd        BoundaryState
    ReviewStatus       string
}
```

### 7.3 Voice Visual

```go
type NarrationBeat struct {
    BeatCode       string
    Text           string
    Emotion        string
    EstimatedSec   float64
    VisualPurpose  string
}

type VisualBeat struct {
    BeatCode        string
    StartSec        float64
    EndSec          float64
    ComponentType   string
    Props           json.RawMessage
    ImageNeeds      []ImageRequirement
    Transition      string
}
```

组件类型使用白名单。

## 8. Model Gateway

### 8.1 能力接口

```go
type Capability string

const (
    TextToText      Capability = "TEXT_TO_TEXT"
    ImageToText     Capability = "IMAGE_TO_TEXT"
    TextToImage     Capability = "TEXT_TO_IMAGE"
    TextImageToVideo Capability = "TEXT_IMAGE_TO_VIDEO"
)

type Provider interface {
    Name() string
    Supports(Capability) bool
    Invoke(ctx context.Context, req Request) (Result, error)
}
```

### 8.2 异步视频

Provider Adapter 内部允许：

1. Submit。
2. 持久化 external_job_id。
3. Poll 或处理 webhook。
4. 下载结果到 MinIO。
5. 登记 Artifact。
6. 发布统一完成事件。

禁止让 HTTP Handler 同步等待几分钟。

### 8.3 错误分类

- `PROVIDER_RATE_LIMITED`
- `PROVIDER_AUTH_FAILED`
- `PROVIDER_TIMEOUT`
- `PROVIDER_CONTENT_REJECTED`
- `PROVIDER_INVALID_REQUEST`
- `PROVIDER_UNAVAILABLE`
- `OUTPUT_SCHEMA_INVALID`
- `OUTPUT_DOWNLOAD_FAILED`

只有可重试错误进入自动重试。

### 8.4 Fake Provider

Fake Provider 使用 fixture：

- 固定 script JSON。
- 固定 3 Shot。
- 固定 SVG/PNG 占位图。
- 固定 MP4 小样或生成轻量测试视频。
- 支持通过请求参数注入 429、500、timeout、schema invalid。

## 9. 两套工作流

### 9.1 AIGC Shot Video v1

```text
brief
→ script_generate
→ script_approval
→ visual_rules
→ asset_plan
→ [characters | scenes | props] parallel
→ assets_approval
→ shot_plan
→ continuity_plan
→ foreach shot:
     storyboard
     keyframes
     video_prompt
     shot_package_approval
     video_generate_or_import
     video_review
     conditional_repair
→ timeline_manifest
→ final_approval
→ publication_package
```

### 9.2 Voice Visual Video v1

```text
opinion_intent
→ narration_script
→ narration_approval
→ audio_bind
→ narration_timing
→ visual_document
→ visual_beats
→ foreach visual beat:
     component_plan
     optional_image_generate
→ hypergen_bundle
→ local_render
→ render_review
→ conditional_beat_repair
→ final_export
→ publication_package
```

## 10. 局部重跑算法

输入：`project_id, stage_id, unit_id?, artifact_id?`

1. 加锁 `project:{id}:rerun`。
2. 校验项目不在删除或终止状态。
3. 根据 Workflow Definition 计算下游影响集合。
4. 若有 unit_id，仅失效该 unit 的相关 StageRun/Artifact。
5. 创建新的 StageRun attempt。
6. 旧 Artifact 保留，`is_current` 在新结果成功后切换。
7. 新 DAG node ID 必须带 attempt 后缀，避免幂等键冲突。
8. 若新执行失败，旧 current Artifact 仍可用。
9. 发布 `workflow.stage.rerun_requested` 和完成/失败事件。

## 11. API 设计

统一响应沿用现有：

```json
{"code": 200, "message": "success", "data": {}}
```

### 11.1 项目

```http
POST   /api/video-projects
GET    /api/video-projects
GET    /api/video-projects/:id
PATCH  /api/video-projects/:id
POST   /api/video-projects/:id/clone
DELETE /api/video-projects/:id
```

### 11.2 工作流

```http
POST /api/video-projects/:id/workflow-runs
GET  /api/workflow-runs/:runId
POST /api/workflow-runs/:runId/pause
POST /api/workflow-runs/:runId/resume
POST /api/workflow-runs/:runId/cancel
GET  /api/workflow-runs/:runId/events
```

### 11.3 审核和重跑

```http
POST /api/workflow-runs/:runId/stages/:stageId/approve
POST /api/workflow-runs/:runId/stages/:stageId/revise
POST /api/workflow-runs/:runId/stages/:stageId/rerun
POST /api/creation-units/:unitId/rerun
```

### 11.4 Unit 和 Artifact

```http
GET /api/video-projects/:id/units
GET /api/creation-units/:unitId
GET /api/creation-units/:unitId/artifacts
GET /api/artifacts/:artifactId
POST /api/creation-units/:unitId/import-video
```

### 11.5 Skill 和 Provider

```http
GET /api/skills
GET /api/skills/:name/:version
GET /api/model-providers
```

### 11.6 Local Runner

```http
POST /api/local-runners/register
POST /api/local-runners/:id/heartbeat
POST /api/local-runners/:id/jobs/claim
POST /api/local-jobs/:id/progress
POST /api/local-jobs/:id/complete
POST /api/local-jobs/:id/fail
POST /api/local-jobs/:id/cancel
```

## 12. 事件

新增事件统一走 Outbox：

```text
video.project.created
workflow.run.created
workflow.stage.started
workflow.stage.waiting_approval
workflow.stage.succeeded
workflow.stage.failed
workflow.stage.rerun_requested
artifact.created
artifact.version_activated
review.created
model.call.started
model.call.completed
model.call.failed
local.job.created
local.job.claimed
local.job.progress
local.job.completed
local.job.failed
```

事件必须包含：

```json
{
  "eventId": "...",
  "traceId": "...",
  "projectId": "...",
  "workflowRunId": "...",
  "stageId": "...",
  "unitId": "...",
  "occurredAt": "..."
}
```

## 13. 前端设计

### 13.1 路由

当前前端无正式 router。升级建议引入 React Router，但第一步可先保持 Zustand page switching，避免一次改动过大。最终路由：

```text
/creation
/projects
/projects/:id
/projects/:id/shots/:unitId
/projects/:id/voice
/tasks
/media
/settings
/publish-legacy
```

### 13.2 页面

- `CreationCenterPage`：两种模板入口。
- `ProjectListPage`：筛选状态和模式。
- `ProjectWorkbenchPage`：阶段树、产物、审核。
- `ShotWorkbenchPage`：Shot 生产包和版本。
- `VoiceWorkbenchPage`：Beat/VisualBeat 时间线。
- `TaskCenterPage`：云端模型、本地任务和日志。
- 保留 `PublishPage`。

### 13.3 Store

不要继续把所有状态放入单个 `appStore`。拆分：

```text
projectStore
workflowStore
artifactStore
taskStore
uiStore
legacyPublishStore
```

服务端状态优先考虑 TanStack Query；Zustand 仅保存 UI 和临时编辑状态。

### 13.4 SSE

- 页面刷新后先 GET Run snapshot，再订阅 SSE。
- 使用 event id 支持 Last-Event-ID。
- SSE 断开自动退化为 3–5 秒轮询。
- 一个项目页面只维护一个事件连接。

## 14. Electron 设计

### 14.1 配置

```json
{
  "apiBaseUrl": "https://aios.example.com/api",
  "projectRoot": "/Users/.../AIOS-Projects",
  "runnerEnabled": true
}
```

凭证放系统 Keychain，不写明文 JSON。

### 14.2 IPC 白名单

- `config:get/set`
- `project:selectRoot`
- `runner:start/stop/status`
- `file:openArtifact`
- `file:revealInFolder`
- `render:cancel`

Renderer 不能直接执行 shell。

### 14.3 Local Executor

Command Handler 映射：

```text
HYPERGEN_RENDER -> 固定 Node 脚本
FFMPEG_PROBE     -> 固定 ffprobe 参数构造器
FFMPEG_ASSEMBLE  -> 固定 ffmpeg 参数构造器
BUNDLE_EXTRACT   -> 安全解压并检查路径
```

每种 Handler 独立校验 payload。

## 15. Docker 云端部署

当前 compose 主要为基础设施。新增生产 compose：

```text
nginx
frontend
aios-core
postgres
redis
redpanda
minio
qdrant(optional)
sandbox(optional)
```

建议文件：

```text
deploy/docker-compose.cloud.yml
deploy/nginx.conf
deploy/.env.cloud.example
```

要求：

- 数据卷持久化。
- 健康检查。
- backend 依赖 infra health，而非只依赖启动顺序。
- 不暴露 PostgreSQL/Redis/MinIO 管理端口到公网。
- API Key 使用 env/secret。
- 上传大小和 Nginx timeout 适配视频。
- `/api/workflow-runs/*/events` 关闭代理缓冲。

## 16. 兼容与迁移

1. 新表全部 `CREATE TABLE IF NOT EXISTS`，后续引入正式 migration 工具。
2. 旧 Workflow Template API 不删除。
3. `workflow_templates` 保留，新增 version table 或兼容列。
4. 旧 Publish/Chat/Media 路由不改语义。
5. 新前端受 `VITE_VIDEO_CREATION_ENABLED` 控制。
6. 后端受 `VIDEO_CREATION_ENABLED` 控制。
7. Electron `apiBaseUrl` 默认仍可指向 localhost。
8. 若新功能关闭，旧系统完整可用。

## 17. 安全与成本

- 模型请求只持久化必要 Prompt；敏感内容可配置脱敏。
- 图片/视频 presigned URL 设置短 TTL。
- Provider 回调必须验签。
- 请求 fingerprint = capability + provider + model + normalized input + asset hashes + parameters。
- 视频生成默认最大自动重试 1 次，防止成本失控。
- 人工修改后 prompt hash 变化，不复用旧结果。
- 每项目可设置预算软限制和硬限制。

## 18. 可观测性

建议 Prometheus 指标：

```text
aios_workflow_stage_total{stage,status}
aios_workflow_stage_duration_seconds
aios_model_call_total{provider,capability,status}
aios_model_call_duration_seconds
aios_model_estimated_cost_total
aios_local_job_total{command_type,status}
aios_artifact_created_total{kind}
aios_rerun_total{scope}
```

日志必须使用现有 Zap，并统一字段。
