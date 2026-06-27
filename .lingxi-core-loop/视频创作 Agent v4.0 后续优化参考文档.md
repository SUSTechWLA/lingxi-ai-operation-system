# 视频创作 Agent v4.0 后续优化参考文档

## 1. 优化目标

将当前系统从“泛 AIOS + 视频能力”进一步升级为：

> 一个真正能帮创作者稳定生产视频内容、沉淀创作资产、管理发布流程、持续优化账号表现的视频创作 Agent。

系统目标不是简单聊天，也不是泛化办公 Agent，而是围绕视频创作形成稳定生产闭环：

```text
选题 / 想法
→ 创作 Brief
→ 脚本 / 口播稿
→ 分镜 / Shot List
→ 角色 / 场景 / 道具 / 风格资产
→ 关键帧 Prompt / 视频 Prompt
→ 视频生成 / 素材导入
→ 剪辑 / 配音 / 字幕 / 封面
→ 人工审核
→ 多平台发布包
→ 发布记录
→ 数据复盘
→ 下一轮选题优化
```

---

## 2. 当前系统状态判断

当前系统已经具备以下基础：

```text
frontend/                    # React + Electron UI
local-backend/               # 本地执行器，负责本地文件、缓存、日志、产物
cloud-backend/               # 云端 AIOS Core，负责编排、模型、配置、日志
hyperframes-render-service/  # HyperFrames 渲染服务
```

核心能力包括：

```text
- Dynamic Agent Runtime
- DAG Orchestrator
- Skill Runtime
- Model Gateway
- Artifact Review
- Local Runner
- HyperFrames Render Service
- 视频项目 / 视频工作流基础
- 发布模块基础
```

后续优化原则：

```text
1. 不再新增非视频业务线。
2. 不再恢复 bid / generic chat。
3. 保留 core 通用引擎，但业务入口只服务视频创作。
4. publish 后续重构为 distribution。
5. 新增 operation，用于发布后数据复盘。
6. 所有能力围绕“视频生产闭环”建设。
```

---

## 3. 第一阶段：确认清理彻底完成

### 3.1 目标

确保系统已经彻底删除或隔离无关业务线：

```text
删除：
- bid
- generic chat
- /api/bid/*
- /api/chat/*
- 标书相关文档
- 通用聊天前端入口

保留：
- internal/core/agentruntime
- internal/core/orchestrator
- internal/core/workflow
- internal/core/skillruntime
- internal/core/modelgateway
- internal/core/artifact
- internal/core/localrunner
- internal/agents/video
- internal/agents/publish
```

### 3.2 检查命令

```bash
grep -R "internal/agents/bid\|internal/agents/chat\|/api/bid\|/api/chat" . -n
grep -R "标书\|投标\|通用对话\|AI 对话助手" . -n
find cloud-backend/internal/agents -maxdepth 1 -type d
```

### 3.3 验收标准

```text
1. cloud-backend/internal/agents 下不再存在 bid 和 chat 目录。
2. 前端导航不再出现标书、通用对话。
3. OpenAPI 文档不再暴露 /api/bid/* 和 /api/chat/*。
4. docs/ARCHITECTURE.md 不再把 bid/chat 写成当前业务线。
5. README、AGENTS.md、CLAUDE.md、ARCHITECTURE.md 对产品定位一致。
6. go test ./... 和 npm run build 通过。
```

---

## 4. 第二阶段：重写权威架构文档

### 4.1 目标

将 `docs/ARCHITECTURE.md` 重写为视频创作 Agent v4.0 的权威文档。

不要再保留“同一套引擎服务多条业务线”的表达。新的表达应该是：

```text
系统底层 core 保持通用，但当前产品只面向视频创作、发布管理和运营复盘。
```

### 4.2 新文档结构

建议 `docs/ARCHITECTURE.md` 改为：

```text
# 躺营 Video Agent 架构设计文档

1. 产品定位
2. 系统运行边界
3. 总体架构
4. Core 通用引擎
5. Video Agent 业务层
6. Distribution Agent 发布层
7. Operation Agent 运营复盘层
8. Skill Package 体系
9. Artifact 创作资产体系
10. Local Runner 本地执行体系
11. Model Gateway 模型网关
12. HyperFrames 渲染链路
13. API 设计
14. 数据模型
15. 前端页面结构
16. 测试与验收标准
17. 后续路线图
```

### 4.3 新架构图

```text
frontend
├── 创作中心
├── 导演工作台
├── 资产库
├── 发布中心
└── 运营复盘

cloud-backend
├── internal/core
│   ├── agentruntime
│   ├── orchestrator
│   ├── workflow
│   ├── skillruntime
│   ├── modelgateway
│   ├── artifact
│   ├── localrunner
│   ├── eventbus
│   └── outbox
│
└── internal/agents
    ├── video
    ├── distribution
    └── operation

local-backend
├── local file manager
├── local artifact store
├── local model provider config
├── ffmpeg executor
├── hyperframes executor
└── diagnostics

hyperframes-render-service
└── html/css/js → video render
```

---

## 5. 第三阶段：重构业务 Agent 层

### 5.1 目标目录

将业务 Agent 层调整为：

```text
cloud-backend/internal/agents/
├── video/
├── distribution/
└── operation/
```

### 5.2 video 模块拆分

当前 `video` 模块不应该只是 project/run 的 CRUD，而应该成为视频创作主业务层。

建议目录：

```text
cloud-backend/internal/agents/video/
├── project/
│   ├── model.go
│   ├── repository.go
│   ├── service.go
│   └── handler.go
│
├── brief/
│   ├── model.go
│   └── service.go
│
├── script/
│   ├── model.go
│   └── service.go
│
├── storyboard/
│   ├── shot.go
│   ├── shot_repository.go
│   └── shot_service.go
│
├── asset/
│   ├── asset.go
│   ├── asset_library.go
│   └── service.go
│
├── prompt/
│   ├── keyframe_prompt.go
│   ├── video_prompt.go
│   └── service.go
│
├── render/
│   ├── provider.go
│   ├── job.go
│   ├── service.go
│   └── handler.go
│
├── review/
│   ├── review_item.go
│   └── service.go
│
└── assistant/
    ├── intent.go
    ├── service.go
    └── handler.go
```

### 5.3 distribution 模块

将原 `publish` 重构为 `distribution`。

目录：

```text
cloud-backend/internal/agents/distribution/
├── package/
│   ├── model.go
│   ├── service.go
│   └── handler.go
│
├── platform/
│   ├── platform.go
│   ├── rule.go
│   └── adapter.go
│
├── plan/
│   └── service.go
│
├── record/
│   ├── model.go
│   └── repository.go
│
└── compliance/
    └── checker.go
```

职责：

```text
1. 根据成片生成平台发布包。
2. 生成标题、简介、标签、封面文案。
3. 检查平台规则。
4. 记录发布计划。
5. 记录发布结果。
6. 未来支持自动发布。
```

### 5.4 operation 模块

新增：

```text
cloud-backend/internal/agents/operation/
├── metric/
│   ├── model.go
│   ├── importer.go
│   └── repository.go
│
├── insight/
│   ├── service.go
│   └── prompt.go
│
├── topic/
│   └── recommender.go
│
└── report/
    └── weekly_report.go
```

职责：

```text
1. 支持手动导入平台数据。
2. 支持单条视频复盘。
3. 支持账号周报。
4. 支持下一批选题推荐。
5. 支持标题、封面、开头钩子的效果归因。
```

---

## 6. 第四阶段：稳定视频创作主工作流

### 6.1 核心工作流一：口播可视化视频

MVP 优先做这个，因为它最容易闭环。

```text
用户输入观点
→ 生成 Creative Brief
→ 生成口播稿
→ 拆分 Beat
→ 生成视觉组件规划
→ 生成 HyperFrames 项目
→ 渲染视频
→ 生成标题 / 简介 / 封面文案
→ 人工审核
→ 导出发布包
```

### 6.2 核心工作流二：镜头式 AIGC 视频

```text
用户输入故事 / 主题
→ Creative Brief
→ 故事大纲
→ 完整脚本
→ 角色设定
→ 场景设定
→ 道具设定
→ Shot List
→ 关键帧 Prompt
→ 视频 Prompt
→ 视频生成任务 / 手动导入素材
→ 剪辑合成
→ 封面 / 标题 / 简介
→ 发布包
```

### 6.3 工作流执行原则

```text
1. 每个关键阶段必须产出 Artifact。
2. 每个 Artifact 必须有 version、hash、status。
3. 关键阶段必须人工审核。
4. 上游产物修改后，下游产物标记 stale。
5. 失败节点必须可以重试。
6. 长任务必须有 progress 和 heartbeat。
7. 高成本视频生成前必须有人工确认。
```

---

## 7. 第五阶段：Artifact 创作资产体系升级

### 7.1 目标

Artifact 不只是任务中间结果，而是创作者长期资产库。

### 7.2 资产类型

```text
creative_brief
script
voiceover_script
beat_plan
story_outline
character_bible
scene_bible
prop_bible
style_profile
shot_list
keyframe_prompt
video_prompt
keyframe_image
video_clip
subtitle
audio
cover
final_video
publish_package
operation_report
```

### 7.3 数据模型要求

Artifact 至少包含：

```text
id
project_id
artifact_type
asset_scope
version
content_hash
prompt_hash
storage_type
storage_ref
depends_on
status
review_status
human_approved
stale
stale_reason
created_at
updated_at
```

### 7.4 资产库能力

```text
1. 项目内资产查看。
2. 全局资产库查看。
3. 角色、场景、道具、风格复用。
4. 将项目资产提升为全局资产。
5. 按标签搜索资产。
6. 从历史项目复用 Prompt。
```

---

## 8. 第六阶段：Local Runner 媒体执行闭环

### 8.1 目标

让本地执行器真正承担视频创作中的重媒体任务。

### 8.2 必须支持的本地命令

```text
FFMPEG_PROBE
FFMPEG_ASSEMBLE
FFMPEG_TRANSCODE
AUDIO_NORMALIZE
AUDIO_DENOISE
SUBTITLE_BURN
HYPERFRAMES_RENDER
HYPERFRAMES_LINT
ASSET_IMPORT
ASSET_EXPORT_PACKAGE
LOCAL_MODEL_TEXT
LOCAL_MODEL_IMAGE
LOCAL_MODEL_VIDEO
```

### 8.3 安全规则

```text
1. 只允许访问用户授权项目目录。
2. 禁止任意 shell。
3. 命令必须白名单化。
4. 用户 API Key 不上传云端。
5. 本地生成的大文件默认不上传云端。
6. 云端只保存 manifest、hash、路径引用和状态。
7. 所有本地任务必须支持 cancel。
```

---

## 9. 第七阶段：视频 Provider 接入

### 9.1 目标

从“手动导入素材”升级为“自动提交视频生成任务”。

### 9.2 统一接口

```go
type VideoProvider interface {
    Submit(ctx context.Context, req VideoGenerateRequest) (*VideoJob, error)
    Get(ctx context.Context, providerJobID string) (*VideoJobStatus, error)
    Cancel(ctx context.Context, providerJobID string) error
}
```

### 9.3 请求结构

```go
type VideoGenerateRequest struct {
    ProjectID       string
    ShotID          string
    Mode            string // text_to_video / image_to_video
    Prompt          string
    NegativePrompt  string
    ReferenceImages []string
    DurationSec     int
    AspectRatio     string
    Resolution      string
    Seed            *int64
    Metadata        map[string]any
}
```

### 9.4 Provider 优先级

第一阶段只接一个 Provider：

```text
1. manual_import
2. hyperframes
3. seedance 或 kling
```

不要同时接太多，否则调试成本过高。

### 9.5 失败分类

```text
retryable:
- timeout
- rate_limited
- provider_busy
- network_error

non_retryable:
- invalid_prompt
- content_policy_violation
- invalid_reference_image
- insufficient_balance
- unsupported_aspect_ratio
- auth_failed
```

---

## 10. 第八阶段：前端产品化

### 10.1 导航结构

```text
创作中心
视频项目
导演工作台
资产库
发布中心
运营复盘
系统设置
```

### 10.2 创作中心

功能：

```text
1. 输入一句话想法。
2. 选择视频类型。
3. 设置目标平台。
4. 设置画幅、时长、风格。
5. 创建项目。
```

### 10.3 导演工作台

页面结构：

```text
顶部：
- 项目标题
- 当前状态
- 当前阶段
- 运行 / 暂停 / 继续 / 取消

左侧：
- 阶段树
- 每阶段状态
- 是否需要审核
- 是否 stale

中间：
- 当前阶段产物
- 可编辑内容
- 版本切换
- 对比历史版本

右侧：
- 质量评分
- 问题列表
- 返工建议
- 审核按钮

底部：
- 执行日志
- 工具调用记录
- 模型调用记录
```

### 10.4 资产库

功能：

```text
1. 查看角色资产。
2. 查看场景资产。
3. 查看道具资产。
4. 查看风格资产。
5. 查看历史 Prompt。
6. 查看封面模板。
7. 支持复用到新项目。
```

### 10.5 发布中心

功能：

```text
1. 查看待发布视频。
2. 生成多平台发布包。
3. 执行平台规则检查。
4. 人工确认发布。
5. 记录发布结果。
```

### 10.6 运营复盘

功能：

```text
1. 手动导入视频数据。
2. 查看单条视频表现。
3. 查看账号周报。
4. 生成下一批选题建议。
5. 分析标题、封面、开头钩子的效果。
```

---

## 11. 第九阶段：API 设计

### 11.1 视频项目

```http
POST   /api/video/projects
GET    /api/video/projects
GET    /api/video/projects/:id
PATCH  /api/video/projects/:id
DELETE /api/video/projects/:id
```

### 11.2 视频工作流

```http
POST /api/video/projects/:id/runs
GET  /api/video/projects/:id/runs
GET  /api/video/projects/:id/runs/:runId
POST /api/video/projects/:id/runs/:runId/pause
POST /api/video/projects/:id/runs/:runId/resume
POST /api/video/projects/:id/runs/:runId/cancel
```

### 11.3 视频助手

不要恢复通用 chat。只允许视频项目内助手。

```http
POST /api/video/projects/:id/assistant/message
POST /api/video/projects/:id/assistant/revise
POST /api/video/projects/:id/assistant/generate-brief
POST /api/video/projects/:id/assistant/generate-shot-list
```

### 11.4 资产

```http
GET  /api/video/projects/:id/artifacts
GET  /api/artifacts/:id
POST /api/artifacts/:id/revise
POST /api/artifacts/:id/approve
POST /api/artifacts/:id/reject
POST /api/artifacts/:id/promote-to-library
GET  /api/asset-library
```

### 11.5 发布

```http
POST /api/distribution/projects/:projectId/packages/generate
GET  /api/distribution/projects/:projectId/packages
POST /api/distribution/packages/:packageId/check
POST /api/distribution/packages/:packageId/approve
POST /api/distribution/packages/:packageId/publish
GET  /api/distribution/records
```

### 11.6 运营

```http
POST /api/operation/metrics/import
GET  /api/operation/projects/:projectId/metrics
POST /api/operation/projects/:projectId/insights/generate
GET  /api/operation/insights
POST /api/operation/topics/recommend
```

### 11.7 实时进度

```http
GET /api/progress/stream?projectId=xxx&runId=xxx
```

事件格式：

```json
{
  "type": "node.progress",
  "projectId": "xxx",
  "runId": "xxx",
  "stage": "video_prompt",
  "status": "running",
  "progress": 0.65,
  "message": "正在生成第 5 个镜头的视频 Prompt"
}
```

---

## 12. 第十阶段：数据模型

### 12.1 video_projects

```sql
CREATE TABLE IF NOT EXISTS video_projects (
    id UUID PRIMARY KEY,
    user_id UUID,
    title TEXT NOT NULL,
    description TEXT,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    target_platforms JSONB DEFAULT '[]',
    aspect_ratio TEXT DEFAULT '16:9',
    target_duration_sec INT,
    style_profile_id UUID,
    current_run_id UUID,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
```

### 12.2 video_shots

```sql
CREATE TABLE IF NOT EXISTS video_shots (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    run_id UUID,
    shot_no INT NOT NULL,
    title TEXT,
    duration_sec INT,
    scene TEXT,
    camera TEXT,
    action TEXT,
    transition_in TEXT,
    transition_out TEXT,
    continuity_notes TEXT,
    keyframe_prompt_artifact_id UUID,
    video_prompt_artifact_id UUID,
    keyframe_artifact_id UUID,
    clip_artifact_id UUID,
    status TEXT DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
```

### 12.3 video_render_jobs

```sql
CREATE TABLE IF NOT EXISTS video_render_jobs (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    shot_id UUID,
    provider TEXT NOT NULL,
    provider_job_id TEXT,
    mode TEXT NOT NULL,
    input_artifact_id UUID,
    output_artifact_id UUID,
    status TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    cost_estimate NUMERIC,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT now()
);
```

### 12.4 publish_packages

```sql
CREATE TABLE IF NOT EXISTS publish_packages (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    platform TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    tags JSONB DEFAULT '[]',
    cover_artifact_id UUID,
    video_artifact_id UUID,
    publish_check_result JSONB DEFAULT '{}',
    status TEXT DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);
```

### 12.5 content_metrics

```sql
CREATE TABLE IF NOT EXISTS content_metrics (
    id UUID PRIMARY KEY,
    publish_record_id UUID NOT NULL,
    collected_at TIMESTAMP NOT NULL,
    views INT,
    likes INT,
    comments INT,
    favorites INT,
    shares INT,
    completion_rate NUMERIC,
    followers_delta INT,
    raw_payload JSONB DEFAULT '{}'
);
```

---

## 13. 测试要求

### 13.1 必须通过

```bash
cd cloud-backend && go test ./...
cd local-backend && go test ./...
cd frontend && npm run build
```

### 13.2 建议补充

```bash
cd cloud-backend && go test -race ./...
cd cloud-backend && make api-docs-check
```

### 13.3 集成测试场景

```text
场景一：口播视频
输入观点 → 生成口播稿 → Beat → HyperFrames → 发布包

场景二：镜头式视频
输入故事 → 脚本 → 分镜 → Prompt → 手动导入素材 → 发布包

场景三：Artifact 返工
审核驳回脚本 → 新版本 → 下游 shot/video prompt 标记 stale

场景四：发布包
成片 → 小红书发布包 → B站发布包 → 发布前检查

场景五：运营复盘
导入数据 → 生成单条视频复盘 → 推荐下一批选题
```

---

## 14. 优先级路线图

### P0：清理一致性

```text
1. 确认 bid/chat 已从代码、路由、文档、OpenAPI、前端入口删除。
2. 统一 README / AGENTS / CLAUDE / ARCHITECTURE 的产品定位。
3. 确保构建和测试通过。
```

### P1：视频工作台稳定

```text
1. 完善视频项目创建。
2. 完善阶段树。
3. 完善 Artifact 审核、返工、版本。
4. 实现 SSE 进度推送。
5. 确保口播视频工作流端到端跑通。
```

### P2：资产库

```text
1. 将角色、场景、道具、风格、Prompt 资产化。
2. 支持项目资产提升为全局资产。
3. 支持资产复用到新项目。
```

### P3：本地媒体执行

```text
1. FFmpeg 合成。
2. HyperFrames 渲染。
3. 音频标准化。
4. 字幕烧录。
5. 本地 manifest 同步。
```

### P4：发布中心

```text
1. publish 重构为 distribution。
2. 生成多平台发布包。
3. 发布前检查。
4. 发布记录。
```

### P5：运营复盘

```text
1. 手动数据导入。
2. 单条视频复盘。
3. 账号周报。
4. 下一批选题推荐。
```

---

## 15. 交付验收标准

### 15.1 产品验收

```text
用户输入：
“我想做一期 90 秒视频，讲 AI Agent 替代的不是岗位，而是工作流程，发小红书和 B站。”

系统应该输出：
1. Creative Brief
2. 口播稿
3. Beat 拆分
4. 视觉组件方案
5. HyperFrames 渲染项目
6. 视频成片或导出包
7. 小红书发布包
8. B站发布包
9. 人工审核记录
10. 后续可导入数据做复盘
```

### 15.2 工程验收

```text
1. 所有核心 API 有 OpenAPI 文档。
2. 所有工作流节点有状态追踪。
3. 所有 Artifact 有版本。
4. 所有模型调用进入 Model Gateway。
5. 所有高成本节点有人工确认。
6. 所有长任务有 heartbeat 和 progress。
7. 所有失败节点可重试。
8. 所有删除的 bid/chat 引用不再出现。
```

---

## 16. 禁止事项

后续 Agent 优化时禁止：

```text
1. 新增 bid、合同、标书、论文等非视频业务线。
2. 恢复通用 chat 产品入口。
3. 让视频项目绕过 Artifact 直接生成最终结果。
4. 让高成本视频生成绕过人工审核。
5. 将用户 API Key 明文写入日志。
6. 将用户本地视频默认上传云端。
7. 手写 API 文档后不更新 OpenAPI。
8. 只写 Prompt，不写输入输出 Schema。
```
