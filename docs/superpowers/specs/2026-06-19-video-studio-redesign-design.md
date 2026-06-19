# 自媒体视频创作台前端重设计

日期：2026-06-19
范围：替换当前 `frontend/` 的旧发布表单体验，构建以视频 Skill 为核心的自媒体创作软件界面。

## 背景

当前前端仍围绕“上传素材、填写标题简介、选择多个平台、发布”展开。这个方向不符合当前产品核心：

- 多平台同步不是当前核心竞争力，且很多平台没有稳定开放的发布接口。
- 用户真正购买的是自媒体创作效率：把想法和素材转成可发布的视频内容包。
- 后端已经开始演进为 Skill 驱动的视频自动化平台，未来会持续新增不同类型的视频创作 Skill。

因此，新前端不再保留多平台发布作为核心流程。前端要变成一个“Skill 转软件功能”的创作台：后端新增一个视频 Skill，前端就能把它展示成一个可使用的视频创作功能。

## 产品目标

1. 主产品定位为自媒体视频创作台，而不是多平台发布工具。
2. 用户最终拿到的是一个可发布内容包：
   - 当前阶段优先返回可导入网页版视频生成工具的素材包。
   - 当用户导入或回填已生成视频后，内容包升级为成片视频。
   - 标题。
   - 简介。
   - 关键词。
   - 视频提示词、参考帧、关键帧、故事板、封面、脚本、分镜、素材包和审核说明。
3. 前端首页直接呈现创作软件本体，不做营销页，不保留旧的多平台发布工作流。
4. 支持当前三类视频创作 Skill，并能自然扩展到未来更多 Skill。
5. 打通“Skill Package -> 软件功能”的前后端契约，让前端不靠硬编码支持未来能力。
6. 用户必须能追踪中间态过程，并能通过对话对任意中间态做局部返修。

## 当前可用后端能力

已存在：

- `GET /api/skills`：Skill 列表、stage、健康状态。
- `GET /api/skills/:name/:version`：Skill 详情。
- `POST /api/skills/:name/:version/compile`：编译为 DAG。
- `GET /api/workflows`：Workflow 模板列表。
- `POST /api/workflows/:id/instantiate`：实例化 workflow。
- `POST /api/video-projects`：创建视频项目。
- `POST /api/video-projects/:pid/workflow-runs`：启动项目下的 workflow run。
- `GET /api/video-projects/:pid/workflow-runs/:rid`：查询 run。
- `GET /api/trace/:taskId`：任务、节点和上下文审计。

不足：

- `GET /api/skills` 只说明“有哪些 skill 和 stage”，不能直接告诉前端如何把 skill 渲染成用户可操作的软件功能。
- Skill stage 的 `input_schema` / `output_schema` 当前主要是文件路径，不是前端可直接消费的 UI schema。
- 当前没有统一的“最终发布包”API。
- Artifact 有后端模型，但缺少面向前端的查询与下载接口。
- 第三类视频 Skill 没有独立 `video_project.mode` 枚举。
- 图片生成当前只能通过 Codex 的 `$imagegen` 技能完成，后端不得假装已有通用图片生成 API。
- 视频生成 API 尚未接入，后端不得假装可以直接生成最终视频。

## 生成能力边界

### 图片生成

当前唯一可用图片生成通道是 Codex 内置 `$imagegen` 技能。

产品含义：

- 后端 Skill 运行阶段可以生成 `imagegen_request` 产物，包含可直接交给 Codex `$imagegen` 使用的 prompt、参考图角色、画幅、约束和产物槽位。
- 前端不展示“系统自动调用图片 API 已完成”的假状态。
- 图片阶段 UI 展示为：
  - `待生成请求`：还没有图片 prompt。
  - `待 Codex 生成`：已有 imagegen prompt，可复制或交给 Codex 执行。
  - `待导入结果`：用户或 Codex 已生成图片，需要上传/绑定到对应槽位。
  - `已绑定参考帧`：图片文件已作为 artifact 版本保存。
- 如果 Codex 在当前开发环境中实际执行 `$imagegen`，生成图片必须落到项目或对象存储中，并绑定到对应 artifact。不能只停留在 `$CODEX_HOME/generated_images`。

`imagegen_request` 最小结构：

```json
{
  "id": "imgreq-shot-01-primary",
  "stageName": "keyframe",
  "unitId": "SHOT_01",
  "role": "primary_keyframe",
  "prompt": "Use case: stylized-concept\nAsset type: video first-frame reference\n...",
  "aspectRatio": "16:9",
  "size": "1920x1080",
  "status": "pending_generation",
  "constraints": ["非真人风格化", "禁止真人写实", "无水印"],
  "boundArtifactId": null
}
```

### 视频生成

当前没有视频生成 API。

产品含义：

- Workflow 不承诺直接输出成片视频。
- 视频生成阶段输出 `video_generation_package`，供用户导入网页版视频生成工具。
- 包内必须包含：
  - `prompt_cn`：最终中文视频提示词，建议控制在 2000 中文字符以内。
  - `negative_prompt`：负面约束。
  - `reference_frames`：可下载或可导入的关键帧/参考帧。
  - `storyboard`：clean frames 和 contact sheet。
  - `duration_sec`、`aspect_ratio`、`shot_id`、`time_window_id`。
  - 导入说明：用户应复制哪些文本、上传哪些图片、生成后把结果回填到哪个槽位。
- 用户在网页视频工具生成后，可以把结果视频导入当前 run，系统再把发布包升级为 `video_imported`。

`video_generation_package` 最小结构：

```json
{
  "id": "vidpkg-shot-01",
  "stageName": "video_prompt",
  "unitId": "SHOT_01",
  "status": "ready_for_external_generation",
  "prompt_cn": "一段可直接复制到视频生成网页的中文提示词...",
  "negative_prompt": "禁止真人写实，禁止水印，禁止字幕错字...",
  "durationSec": 5,
  "aspectRatio": "16:9",
  "referenceFrameArtifactIds": ["art-keyframe-01"],
  "storyboardArtifactIds": ["art-storyboard-01"],
  "importInstructions": [
    "打开网页版视频生成工具",
    "上传 reference_frames 中的首帧图",
    "复制 prompt_cn 到提示词输入框",
    "生成后将 mp4 导入 SHOT_01/video 槽位"
  ]
}
```

## 中间态与对话返修

创作流程不能是“用户点击一次，系统直接吐最终结果”。所有关键中间态必须是一等对象，可展示、可确认、可回溯、可局部返修。

### Artifact 版本模型

前端需要能看到每个 stage 的产物版本：

- brief / intent。
- IP 或选题文档。
- 口播稿 / 剧本。
- 角色、场景、道具设定。
- 分镜和 SHOT 设计。
- imagegen request。
- 参考帧和关键帧。
- storyboard clean frames / contact sheet。
- video generation package。
- 标题、简介、关键词。
- 用户导入的视频文件。

每个 artifact 需要包含：

```json
{
  "id": "art-123",
  "runId": "wfr-12345678",
  "stageName": "script",
  "unitId": "SHOT_01",
  "kind": "MARKDOWN",
  "name": "口播稿 v2",
  "version": 2,
  "parentId": "art-122",
  "status": "draft",
  "approvalStatus": "pending",
  "revisionReason": "用户要求开头更抓人",
  "storageRef": "minio://...",
  "inlinePreview": "前三百字或结构摘要",
  "createdAt": "2026-06-19T00:00:00Z"
}
```

### 对话返修入口

任意中间态都要支持用户自然语言修改。

新增建议接口：

- `GET /api/skill-functions/runs/:runId/artifacts`
- `GET /api/skill-functions/runs/:runId/artifacts/:artifactId`
- `POST /api/skill-functions/runs/:runId/revisions`
- `POST /api/skill-functions/runs/:runId/artifacts/:artifactId/approve`
- `POST /api/skill-functions/runs/:runId/artifacts/:artifactId/import`

`POST /api/skill-functions/runs/:runId/revisions` 请求：

```json
{
  "message": "第 3 个镜头太平了，开头要更抓人，但不要改后面的口播结构",
  "target": {
    "stageName": "shot_design",
    "unitId": "SHOT_03",
    "artifactId": "art-shot-03-v1"
  }
}
```

响应：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "revisionId": "rev-123",
    "affectedArtifacts": ["art-shot-03-v2", "imgreq-shot-03-primary-v2"],
    "rerunScope": ["shot_design:SHOT_03", "keyframe:SHOT_03"],
    "summary": "已加强开头冲突和镜头运动，只影响 SHOT_03 及其关键帧请求。"
  }
}
```

规则：

- 用户反馈必须归因到具体 stage、unit、artifact 或发布包字段。
- 只重跑受影响的中间态，不整条流水线重跑。
- 返修后保留旧版本，默认不覆盖。
- 每次返修生成一条 revision record，前端可在版本历史里查看。
- 审核通过的 artifact 才进入下游默认输入；用户可以回滚到旧版本再继续。

## 必须新增的 Skill 功能化契约

为了支持未来持续新增视频 Skill，后端需要向前端暴露一个稳定 facade。建议命名为 `skill-functions`。

### `GET /api/skill-functions`

返回所有可被前端当作软件功能展示的 Skill。

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "functions": [
      {
        "id": "aigc-shot-video@1.0.0",
        "kind": "video_creation",
        "name": "镜头式 AIGC 短片",
        "subtitle": "从故事想法生成分镜、关键帧提示词和外部视频生成素材包",
        "description": "适合剧情、广告、概念短片和产品故事。",
        "skillName": "aigc-shot-video",
        "skillVersion": "1.0.0",
        "templateId": "wf-aigc-shot-video-1-0-0",
        "status": "available",
        "tags": ["剧情", "分镜", "AIGC"],
        "capabilities": {
          "imageGeneration": "codex_imagegen_only",
          "videoGeneration": "external_web_import",
          "intermediateArtifacts": true,
          "conversationalRevision": true
        },
        "defaults": {
          "aspectRatio": "9:16",
          "targetDurationSec": 30,
          "language": "zh-CN"
        },
        "inputSchema": {},
        "outputContract": {},
        "stages": []
      }
    ]
  }
}
```

### `GET /api/skill-functions/:id`

返回单个功能的完整配置：

- display metadata：名称、描述、标签、适用场景、封面色或图标。
- input schema：前端表单字段、类型、默认值、校验规则、placeholder。
- stage schema：用户可理解的阶段名、是否人工审核、是否长任务。
- output contract：最终输出包包含哪些内容。
- capability contract：图片生成、视频生成、中间态、对话返修的实际支持方式。
- runtime status：依赖工具是否可用、workflow template 是否存在、skill 是否健康。

### `POST /api/skill-functions/:id/runs`

以“软件功能”的方式启动一次创作，不要求前端理解 Project、Workflow、DAG 的内部关系。

请求：

```json
{
  "projectName": "为什么普通人更需要 AI 工作流",
  "input": {
    "brief": "做一条讲普通人如何用 AI 提升工作效率的自媒体视频",
    "style": "观点清晰、节奏快、适合口播",
    "aspectRatio": "9:16",
    "targetDurationSec": 60,
    "language": "zh-CN"
  }
}
```

响应：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "runId": "wfr-12345678",
    "projectId": "vp-12345678",
    "taskId": "task-12345678",
    "traceUrl": "/api/trace/task-12345678"
  }
}
```

### `GET /api/skill-functions/runs/:runId`

返回运行状态、阶段状态、当前可见产物、待确认项和可返修目标。

### `GET /api/skill-functions/runs/:runId/publish-package`

返回最终可发布内容包。

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "status": "ready_for_external_video_generation",
    "video": {
      "url": null,
      "filename": null,
      "durationSec": null,
      "status": "not_generated"
    },
    "videoGenerationPackage": {
      "downloadUrl": "/api/skill-functions/runs/wfr-12345678/publish-package/download",
      "packages": ["vidpkg-shot-01", "vidpkg-shot-02"]
    },
    "cover": {
      "url": "https://..."
    },
    "title": "普通人真正该用 AI 的方式",
    "description": "这条视频讲清楚 AI 工作流为什么比单点工具更重要...",
    "keywords": ["AI工作流", "效率提升", "自媒体", "普通人AI"],
    "script": "...",
    "notes": ["已完成基础合规检查"]
  }
}
```

## Core Gap 决策

结论：`CORE_CHANGE_CANDIDATE`。

原因：

- 需求不是单个客户页面变化，而是平台需要把任意视频 Skill 稳定暴露成前端可消费的软件功能。
- 当前 `/api/skills`、`/api/workflows`、`/api/video-projects` 能支撑临时拼装，但不能提供 UI schema、输出契约和最终发布包。
- 如果不新增 facade，未来每增加一种视频 Skill，前端都需要硬编码一次，这会破坏 Skill Package 的扩展价值。
- 新增 `skill-functions` facade 属于入口层和 workflow/skillruntime 的通用平台能力，不包含具体业务规则。

本轮实现应包含后端 `skill-functions` facade 和前端创作台。兼容层只用于后端接口尚未启动或本地开发环境缺失时的降级展示。

## 首版兼容策略

如果本地环境暂时没有 `skill-functions`，前端可以用现有接口拼装只读能力：

- `GET /api/skills` 获取 Skill 和 stages。
- `GET /api/workflows` 匹配 templateId。
- `POST /api/video-projects` 创建项目。
- `POST /api/video-projects/:pid/workflow-runs` 启动运行。
- `GET /api/trace/:taskId` 展示阶段和输出摘要。

该兼容路径不能成为正式方案；正式启动、阶段状态和发布包读取应走 `skill-functions`。

## 信息架构

旧前端主流程直接替换为单一自媒体创作台。

```text
┌────────────────────────────────────────────────────────────────────┐
│ 躺营 AI 自媒体创作台                         Skills / Runner / API │
├───────────────┬──────────────────────────┬─────────────────────────┤
│ 视频类型库    │ 创作 Brief                │ 发布素材包              │
│ 动态 skill    │ 主题、受众、风格、时长     │ 提示词、参考帧、文案     │
│ 标签与状态    │ 一键启动 / 当前阶段        │ 导出 / 导入视频 / 返修   │
├───────────────┴──────────────────────────┴─────────────────────────┤
│ 制作进度：阶段轴、审核点、Trace、错误、历史版本                      │
└────────────────────────────────────────────────────────────────────┘
```

导航收敛为：

- `创作台`：主页面，默认打开。
- `作品`：历史项目与发布包。
- `技能`：已加载 Skill、健康状态、依赖能力。
- `系统`：Electron 和后端状态，弱化为工具入口。

不再出现：

- 多平台选择。
- 一键发布到多平台。
- 超级会员卡片。
- 图文/视频发布类型切换。
- 旧的标题、简介、关键词散列表单作为主流程。

## 当前首批视频功能

### 镜头式 AIGC 短片

- skill：`aigc-shot-video`
- 输出重点：分镜、关键帧提示词、Codex imagegen 请求、视频生成素材包、标题、简介、关键词。
- 适合：剧情短片、广告短片、产品故事、概念片。

### 文字口播可视化

- skill：`voice-visual-video`
- 输出重点：口播稿、Visual Beat、组件 DSL、参考帧/画面资产请求、视频生成素材包、标题、简介、关键词。
- 适合：知识博主、观点解释、课程切片、商业复盘。

### 观点口播/HyperFrames

- skill：`create-opinion-videos`
- 输出重点：观点口播稿、HyperFrames 参考、画面资产请求、可导入外部工具的视频素材包、标题、简介、关键词。
- 适合：观点账号、商业评论、个人 IP、热点解读。

未来新增 Skill 时，只要后端返回 `skill-functions` 契约，前端自动出现在视频类型库里。

## 核心界面

### `CreatorWorkbenchPage`

替代当前 `PublishPage`，作为默认首页。

职责：

- 获取可用 skill functions。
- 管理当前选中的视频功能。
- 渲染动态 Brief 表单。
- 启动 run。
- 轮询 run、trace、artifacts 和 publish package。
- 展示中间态、返修入口和最终内容包。

### `FunctionLibrary`

左侧视频类型库。

展示：

- 功能名称和适用场景。
- skill 健康状态和依赖状态。
- 标签和预计流程复杂度。
- 是否当前可用。

交互：

- 点击切换功能。
- 不可用功能仍可查看原因。

### `DynamicBriefForm`

中间输入区，由 `inputSchema` 生成。

首版基础字段：

- `brief`：创作目标，必填。
- `audience`：目标受众。
- `style`：表达风格。
- `aspectRatio`：9:16、16:9、1:1。
- `targetDurationSec`：30、60、90、180。
- `referenceMaterial`：可选素材或链接。
- `language`：默认中文。

后续字段由 skill schema 控制，不在前端硬编码。

### `ProductionSpine`

制作进度阶段轴。

展示：

- stage 名称。
- 运行状态。
- 人工审核点。
- 长任务标记。
- 失败信息。
- 当前可见产物。
- 当前阶段的可返修入口。
- 当前阶段 artifact 版本历史。

状态来源优先级：

1. `GET /api/skill-functions/runs/:runId`。
2. `GET /api/video-projects/:pid/workflow-runs/:rid`。
3. `GET /api/trace/:taskId`。
4. 静态 skill stages。

### `PublishPackagePanel`

右侧最终结果和外部生成素材包预览。

状态：

- 未开始：展示所选功能将产出的素材包。
- 生成中：展示已生成的中间产物和待确认项。
- 可外部生成：展示视频提示词、参考帧、storyboard 和导入说明。
- 已导入视频：展示视频、标题、简介、关键词。
- 失败：展示失败阶段和重试建议。

操作：

- 复制标题。
- 复制简介。
- 复制关键词。
- 复制视频提示词。
- 下载参考帧和视频生成素材包。
- 导入外部网页生成的视频。
- 有视频时下载视频。
- 对标题、简介、关键词或中间态发起对话返修。

不包含多平台同步。

### `ProjectHistoryPage`

作品页。

展示：

- 项目列表。
- 状态：草稿、生成中、需要审核、已完成、失败。
- 最终发布包摘要。
- 中间态版本数量和最近返修记录。
- 继续编辑和查看 Trace。

### `SkillConsolePage`

技能页。

展示：

- 已加载 Skill。
- 版本。
- 健康状态。
- 输入 schema。
- 输出 contract。
- 依赖工具。
- 编译 workflow 的状态。

这页面向高级用户和开发者，用于验证“新增 Skill 已变成软件功能”。

## 数据模型（前端）

新增核心类型：

```ts
export interface SkillFunction {
  id: string
  kind: 'video_creation'
  name: string
  subtitle?: string
  description: string
  skillName: string
  skillVersion: string
  templateId: string
  status: 'available' | 'missing_template' | 'unhealthy' | 'disabled'
  capabilities: {
    imageGeneration: 'codex_imagegen_only' | 'none'
    videoGeneration: 'external_web_import' | 'provider_api' | 'none'
    intermediateArtifacts: boolean
    conversationalRevision: boolean
  }
  tags: string[]
  defaults: Record<string, unknown>
  inputSchema: FunctionInputSchema
  outputContract: PublishPackageContract
  stages: FunctionStage[]
}

export interface PublishPackage {
  status:
    | 'empty'
    | 'generating'
    | 'ready_for_external_video_generation'
    | 'video_imported'
    | 'failed'
  video?: {
    status: 'not_generated' | 'imported'
    url?: string
    filename?: string
    durationSec?: number
  }
  videoGenerationPackage?: VideoGenerationPackage
  cover?: { url?: string }
  title?: string
  description?: string
  keywords?: string[]
  script?: string
  notes?: string[]
}

export interface ImagegenRequest {
  id: string
  stageName: string
  unitId?: string
  role: 'reference' | 'primary_keyframe' | 'storyboard_frame' | 'cover'
  prompt: string
  aspectRatio: string
  size?: string
  status: 'pending_generation' | 'generated' | 'imported' | 'rejected'
  constraints: string[]
  boundArtifactId?: string
}

export interface VideoGenerationPackage {
  id: string
  stageName: string
  unitId?: string
  status: 'draft' | 'ready_for_external_generation' | 'video_imported'
  promptCn: string
  negativePrompt?: string
  durationSec?: number
  aspectRatio: string
  referenceFrameArtifactIds: string[]
  storyboardArtifactIds: string[]
  importInstructions: string[]
}

export interface ArtifactVersion {
  id: string
  runId: string
  stageName: string
  unitId?: string
  kind: 'MARKDOWN' | 'JSON' | 'IMAGE' | 'VIDEO' | 'BUNDLE' | 'LOG'
  name: string
  version: number
  parentId?: string
  status: 'draft' | 'current' | 'superseded' | 'rejected'
  approvalStatus: 'pending' | 'approved' | 'changes_requested'
  revisionReason?: string
  storageRef?: string
  inlinePreview?: string
  createdAt: string
}

export interface RevisionRecord {
  id: string
  runId: string
  userMessage: string
  targetStage?: string
  targetUnitId?: string
  targetArtifactId?: string
  affectedArtifactIds: string[]
  summary: string
  createdAt: string
}
```

## 视觉方向

产品应更像自媒体创作软件，而不是后台管理系统。

设计关键词：

- 清爽。
- 高级。
- 强创作感。
- 低学习成本。
- 结果导向。

色彩：

- `ink`: `#17181A`，主文字。
- `paper`: `#F8F7F2`，页面底色。
- `panel`: `#FFFFFF`，主面板。
- `signal`: `#00A6A6`，主操作和运行中。
- `lime`: `#D6FF4D`，可用和完成状态。
- `coral`: `#FF6B57`，错误。
- `line`: `#DDDED8`，分隔线。

视觉标志：

- `ProductionSpine` 是核心记忆点：用户看到的是一条视频生产线，不是技术 DAG。
- `PublishPackagePanel` 是核心价值点：始终提醒用户会拿到“外部视频生成素材包 + 标题简介关键词”，导入成片后再升级为完整发布包。

组件形态：

- 卡片半径不超过 8px。
- 不使用大面积紫色渐变。
- 不使用装饰光斑。
- 不用多层嵌套卡片。
- 操作区以清晰按钮、图标、分隔线和状态标签组织。

## 实施范围

替换或弱化：

- `PublishPage.tsx`
- `PlatformSelector.tsx`
- `PublishButton.tsx`
- 旧 `ContentTypeSelector.tsx`
- 旧多平台相关 store 字段

新增：

- `frontend/src/pages/CreatorWorkbenchPage.tsx`
- `frontend/src/pages/ProjectHistoryPage.tsx`
- `frontend/src/pages/SkillConsolePage.tsx`
- `frontend/src/components/creator/FunctionLibrary.tsx`
- `frontend/src/components/creator/DynamicBriefForm.tsx`
- `frontend/src/components/creator/ProductionSpine.tsx`
- `frontend/src/components/creator/PublishPackagePanel.tsx`
- `frontend/src/components/creator/ArtifactTimeline.tsx`
- `frontend/src/components/creator/RevisionChatPanel.tsx`
- `frontend/src/components/creator/RunTraceDrawer.tsx`
- `frontend/src/stores/creatorStore.ts`

修改：

- `frontend/src/App.tsx`
- `frontend/src/components/Sidebar.tsx`
- `frontend/src/services/api.ts`
- `frontend/src/utils/types.ts`
- `frontend/src/index.css`

本轮新增后端接口：

- `GET /api/skill-functions`
- `GET /api/skill-functions/:id`
- `POST /api/skill-functions/:id/runs`
- `GET /api/skill-functions/runs/:runId`
- `GET /api/skill-functions/runs/:runId/publish-package`
- `GET /api/skill-functions/runs/:runId/artifacts`
- `GET /api/skill-functions/runs/:runId/artifacts/:artifactId`
- `POST /api/skill-functions/runs/:runId/revisions`
- `POST /api/skill-functions/runs/:runId/artifacts/:artifactId/approve`
- `POST /api/skill-functions/runs/:runId/artifacts/:artifactId/import`

前端保留兼容层，但只用于显示“后端功能化接口未启用”的可恢复状态，不作为核心交互路径。

## 验收标准

1. 默认进入自媒体创作台，不再默认进入旧发布页。
2. 页面没有多平台同步发布入口。
3. 当前三类视频 Skill 展示为三个可理解的视频创作功能。
4. 未来新增 Skill 后，只要后端返回 `skill-functions` 契约，前端无需新增专属页面即可展示。
5. 用户能输入 Brief 并启动一次创作 run。
6. 生成过程中能看到制作阶段、审核点、中间产物版本、失败原因和 Trace。
7. 图片生成阶段展示 Codex `$imagegen` 请求和导入槽位，不展示虚假的后端图片 API 成功状态。
8. 视频生成阶段展示可导入网页版视频工具的素材包：视频提示词、负面提示、参考帧、故事板和导入说明。
9. 用户能对任意中间态通过对话发起局部返修，并看到新旧版本历史。
10. 用户导入外部生成的视频后，发布包升级为视频、标题、简介、关键词。
11. 标题、简介、关键词可以复制；外部生成素材包可以下载；已导入视频可以下载或打开。
12. 后端视频功能未启用时，前端显示明确配置提示。
13. `cd frontend && npm run build` 通过。

## 风险与决策

1. 当前 Artifact 没有前端 HTTP API，所以“最终发布包”需要后端补 facade；前端兼容层只能从 Trace 输出中尽力解析。
2. Skill schema 当前不等于 UI schema，必须新增功能化契约，否则未来每个新 Skill 都会逼前端写硬编码。
3. 原始前端可替换，但旧 API 客户端函数不要一次性全删；先停止入口引用，构建稳定后再清理。
4. 第三类 `create-opinion-videos` 暂无独立 project mode。功能化 facade 应屏蔽这个后端细节。
5. 多平台发布不进入核心流程；如果未来需要，只作为导出后的附属能力，不回到主体验。
6. 图片生成只允许走 Codex `$imagegen` 或用户导入结果；后端不要新增虚假的图片模型 provider。
7. 视频生成 API 未接入前，正式交付是外部视频生成素材包；用户导入成片后才进入 `video_imported` 状态。
8. 对话返修必须按 artifact 版本和受影响范围执行，禁止一次局部意见触发整条流水线重跑。
