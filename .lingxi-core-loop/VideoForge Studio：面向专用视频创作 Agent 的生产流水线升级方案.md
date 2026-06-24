# VideoForge Studio：面向专用视频创作 Agent 的生产流水线升级方案

## 1. 升级主题

本次升级命名为：

**VideoForge Studio**

主题表达：

```text
从“动态 Agent 调工具生成视频素材”
升级为
“Pipeline Manifest + Stage Director + Canonical Artifact + 双引擎渲染 + 最终质检”的视频生产系统
```

其中：

* **VideoForge**：视频合成与渲染底座。
* **Studio**：面向创作者的一站式生产流水线。
* **Pipeline**：每类视频都有稳定流程。
* **Stage Director**：每个阶段有专门 Agent 负责。
* **Canonical Artifact**：每个阶段输出标准产物。
* **Dual-Engine**：HyperFrames 确定性合成 + Seedance 生成式视频。
* **Final Review**：最终视频交付前自动质检。

---

## 2. 设计目标

当前系统已经具备：

```text
Agent Runtime
ToolManifest
LLMPlanner
PlanGuard
PlanCompiler
HyperFrames Render Service
Artifact
Review
```

但如果要成为真正可用的视频创作产品，还需要补齐：

```text
1. 不同视频类型的固定生产 Pipeline
2. 每个阶段的标准产物协议
3. 生成前的能力预检和成本预估
4. HyperFrames / Seedance / 素材库 的策略选择
5. 参考视频分析与复刻能力
6. 用户关键节点审核
7. 最终视频质量检查
8. 可升级的 HyperFrames Adapter Layer
```

最终目标：

```text
用户一句话 / 上传 MP4 / 上传参考视频
  ↓
系统选择视频 Pipeline
  ↓
生成方案、成本和风险
  ↓
用户确认
  ↓
脚本、分镜、渲染策略、素材、合成、渲染
  ↓
自动质检
  ↓
输出 final.mp4 + 创作包
```

---

## 3. 总体架构

```text
用户输入
  ├── 一句话需求
  ├── MD 时间线
  ├── 上传 MP4
  └── 参考视频 URL / 文件
        ↓
VideoForge Studio Orchestrator
        ↓
Pipeline Selector
        ↓
Capability Preflight
        ↓
Proposal Stage
        ↓
Stage Directors
  ├── Brief Director
  ├── Script Director
  ├── Visual Plan Director
  ├── Render Strategy Director
  ├── Asset Director
  ├── Edit Director
  ├── Compose Director
  └── Publish Director
        ↓
Canonical Artifacts
  ├── video_brief.json
  ├── proposal_packet.json
  ├── script.json
  ├── shot_list.json
  ├── visual_plan.json
  ├── render_strategy.json
  ├── consistency_pack.json
  ├── asset_manifest.json
  ├── video_composition_spec.json
  ├── hyperframes_project_manifest.json
  ├── render_report.json
  ├── final_review.json
  └── publish_package.json
        ↓
Tool Execution
  ├── ModelGateway
  ├── Seedance / 视频模型
  ├── HyperFrames Render Service
  ├── FFmpeg
  ├── ASR
  ├── TTS
  ├── 图片生成
  └── 素材检索
        ↓
ArtifactStore + Review
        ↓
Final MP4 + Project Package
```

---

## 4. 核心升级原则

### 4.1 不让 LLM 自由乱跑

不能让 LLM 每次都从零决定所有流程。

错误方式：

```text
用户输入
  ↓
LLM 自由决定工具
  ↓
直接生成视频
```

正确方式：

```text
用户输入
  ↓
选择 Pipeline
  ↓
读取 Pipeline Manifest
  ↓
每个 Stage 调用对应 Director
  ↓
产出 Canonical Artifact
  ↓
PlanGuard / Review / QualityGate 控制
```

---

### 4.2 让 Pipeline 成为视频生产主骨架

新增目录：

```text
cloud-backend/video-pipelines/
├── knowledge-video.yaml
├── source-footage-remix.yaml
├── reference-video-remix.yaml
├── ai-generated-short.yaml
├── product-demo.yaml
└── cinematic-broll.yaml
```

每个 Pipeline 描述：

```yaml
id: knowledge-video
name: 知识口播视频
description: 适合从一句话生成知识分享类视频，包含脚本、分镜、字幕、图文组件和最终 MP4。

stages:
  - id: brief
    director: brief_director
    produces: video_brief
    reviewRequired: false

  - id: proposal
    director: proposal_director
    produces: proposal_packet
    reviewRequired: true

  - id: script
    director: script_director
    produces: script
    reviewRequired: true

  - id: visual_plan
    director: visual_plan_director
    produces: visual_plan
    reviewRequired: true

  - id: render_strategy
    director: render_strategy_director
    produces: render_strategy
    reviewRequired: true

  - id: assets
    director: asset_director
    produces: asset_manifest
    reviewRequired: false

  - id: edit
    director: edit_director
    produces: video_composition_spec
    reviewRequired: true

  - id: compose
    director: compose_director
    produces: render_report
    reviewRequired: false

  - id: final_review
    director: final_review_director
    produces: final_review
    reviewRequired: false

  - id: publish
    director: publish_director
    produces: publish_package
    reviewRequired: true
```

---

## 5. Pipeline 类型设计

### 5.1 `knowledge-video`

适用场景：

```text
知识分享
口播科普
小红书 / B站 / 抖音知识视频
图文卡片解释
```

推荐引擎：

```text
HyperFrames 为主
Seedance 作为 B-roll 素材生成
```

典型链路：

```text
brief
  ↓
proposal
  ↓
script
  ↓
shot_list
  ↓
render_strategy
  ↓
caption / overlay / asset plan
  ↓
video_composition_spec
  ↓
hyperframes_project
  ↓
render
  ↓
final_review
  ↓
publish
```

---

### 5.2 `source-footage-remix`

适用场景：

```text
用户上传 MP4
自动剪辑
加字幕
加重点卡片
加标题
加转场
```

推荐引擎：

```text
FFmpeg + HyperFrames
```

典型链路：

```text
media_importer
  ↓
media_probe
  ↓
audio_extractor
  ↓
asr_transcriber
  ↓
clip_planner
  ↓
ffmpeg_clip_extractor
  ↓
caption_generator
  ↓
overlay_designer
  ↓
hyperframes_project
  ↓
render
```

---

### 5.3 `reference-video-remix`

适用场景：

```text
参考爆款视频
拆解节奏
借鉴结构
生成原创版本
```

典型链路：

```text
reference_video_analyzer
  ↓
reference_analysis_report
  ↓
proposal_packet
  ↓
script
  ↓
visual_plan
  ↓
render_strategy
  ↓
assets
  ↓
composition
  ↓
render
```

---

### 5.4 `ai-generated-short`

适用场景：

```text
需要大量 AI 视频模型生成画面
短剧情
氛围短片
非写实动画镜头
```

推荐引擎：

```text
Seedance / 视频模型生成短 clip
HyperFrames 最终合成字幕、卡片、音频、转场
```

典型链路：

```text
script
  ↓
shot_list
  ↓
consistency_pack
  ↓
seedance_prompt_builder
  ↓
seedance_clip_generator
  ↓
clip_qc
  ↓
hyperframes_composition
  ↓
render
```

---

## 6. Stage Director 设计

### 6.1 什么是 Stage Director

Stage Director 是每个生产阶段的专用 Agent。

它不是普通 Prompt，而是一个阶段级执行器：

```text
读取上游 artifacts
读取当前 pipeline stage manifest
选择工具
生成当前阶段 canonical artifact
做本阶段质量检查
```

---

### 6.2 建议新增 Director

```text
brief_director
proposal_director
script_director
visual_plan_director
render_strategy_director
asset_director
edit_director
compose_director
final_review_director
publish_director
```

---

### 6.3 示例：Render Strategy Director

职责：

```text
判断每个 shot 应该使用：
- HyperFrames
- Seedance
- Hybrid
- Existing Media
- Stock Footage
- FFmpeg-only
```

输入：

```text
video_brief
script
shot_list
visual_plan
tool_capabilities
budget
provider_capabilities
```

输出：

```json
{
  "artifactKind": "render_strategy",
  "strategyVersion": "video-render-strategy-v1",
  "overallMode": "hybrid",
  "shots": [
    {
      "shotId": "SHOT_01",
      "engine": "hyperframes",
      "reason": "标题和字幕需要准确文字，适合 HTML 确定性合成。",
      "riskLevel": "low"
    },
    {
      "shotId": "SHOT_02",
      "engine": "seedance",
      "reason": "需要生成古代江河和龙舟动态氛围，HTML 难以自然表现。",
      "riskLevel": "medium",
      "requiresConsistencyPack": true,
      "maxDurationSec": 8
    },
    {
      "shotId": "SHOT_03",
      "engine": "hybrid",
      "reason": "背景由 Seedance 生成，字幕和知识卡片由 HyperFrames 叠加。"
    }
  ],
  "finalAssemblyEngine": "hyperframes",
  "estimatedCost": {
    "seedanceClips": 3,
    "hyperframesRenders": 1,
    "costLevel": "medium"
  },
  "requiresUserApproval": true
}
```

---

## 7. Canonical Artifact 体系

OpenMontage 最值得借鉴的一点是：每个阶段必须输出标准产物。你的系统也应这样做。

### 7.1 Artifact 列表

```text
video_brief.json
proposal_packet.json
script.json
shot_list.json
visual_plan.json
render_strategy.json
consistency_pack.json
asset_manifest.json
edit_decisions.json
video_composition_spec.json
hyperframes_project_manifest.json
render_report.json
final_review.json
publish_package.json
```

---

### 7.2 `proposal_packet.json`

用于正式生成前，让用户选择方案。

```json
{
  "artifactKind": "proposal_packet",
  "options": [
    {
      "id": "option_a",
      "name": "低成本图文口播版",
      "description": "主要使用 HyperFrames，少量 AI 图片，成本低、可控性高。",
      "renderMode": "hyperframes_first",
      "estimatedDurationSec": 60,
      "estimatedCost": "low",
      "risk": "low"
    },
    {
      "id": "option_b",
      "name": "混合动态版",
      "description": "Seedance 生成部分 B-roll，HyperFrames 负责字幕和知识卡片。",
      "renderMode": "hybrid",
      "estimatedDurationSec": 60,
      "estimatedCost": "medium",
      "risk": "medium"
    },
    {
      "id": "option_c",
      "name": "电影感动态版",
      "description": "较多使用 Seedance，画面丰富，但一致性和成本风险更高。",
      "renderMode": "seedance_heavy",
      "estimatedDurationSec": 60,
      "estimatedCost": "high",
      "risk": "high"
    }
  ],
  "recommendedOptionId": "option_b",
  "requiresApproval": true
}
```

---

### 7.3 `visual_plan.json`

用于描述每个画面元素。

```json
{
  "artifactKind": "visual_plan",
  "shots": [
    {
      "shotId": "SHOT_01",
      "durationSec": 5,
      "visualDescription": "标题页，背景为低饱和纹理，中央出现端午节的来历",
      "elements": [
        {
          "id": "title",
          "type": "text",
          "content": "端午节的来历",
          "requiresExactText": true
        },
        {
          "id": "background",
          "type": "abstract_background",
          "requiresComplexMotion": false
        }
      ]
    }
  ]
}
```

---

### 7.4 `render_strategy.json`

双引擎决策核心产物。

```json
{
  "artifactKind": "render_strategy",
  "overallMode": "hybrid",
  "shots": [
    {
      "shotId": "SHOT_01",
      "renderMode": "hyperframes",
      "elements": [
        {
          "elementId": "title",
          "engine": "hyperframes",
          "reason": "文字需要准确。"
        }
      ]
    },
    {
      "shotId": "SHOT_02",
      "renderMode": "hybrid",
      "elements": [
        {
          "elementId": "dragon_boat_background",
          "engine": "seedance",
          "reason": "需要自然动态水面和龙舟运动。"
        },
        {
          "elementId": "caption",
          "engine": "hyperframes",
          "reason": "字幕需要精确时间轴。"
        }
      ]
    }
  ],
  "decisionLogId": "decision_render_strategy_001"
}
```

---

### 7.5 `video_composition_spec.json`

最终合成协议。

```json
{
  "specVersion": "aios-video-composition-v1",
  "projectId": "project_001",
  "durationSec": 60,
  "fps": 30,
  "resolution": {
    "width": 1920,
    "height": 1080
  },
  "tracks": [
    {
      "id": "video_track_01",
      "type": "video",
      "clips": [
        {
          "id": "clip_001",
          "src": "assets/video/seedance_shot_02.mp4",
          "timelineStartSec": 5,
          "timelineEndSec": 13,
          "fit": "cover"
        }
      ]
    },
    {
      "id": "caption_track_01",
      "type": "caption",
      "items": [
        {
          "id": "caption_001",
          "startSec": 5,
          "endSec": 8,
          "text": "端午节并不只有一个来源。"
        }
      ]
    },
    {
      "id": "overlay_track_01",
      "type": "overlay",
      "items": [
        {
          "id": "card_001",
          "kind": "info_card",
          "startSec": 8,
          "endSec": 13,
          "title": "知识点 01",
          "body": "端午节有多重历史来源。",
          "animation": "slide_in_right"
        }
      ]
    }
  ]
}
```

---

## 8. Capability Preflight 能力预检

### 8.1 目的

在生成视频前，系统要先告诉用户：

```text
当前可以生成什么
缺少什么 Provider
哪些功能可用
哪些功能需要配置
预计成本和风险
```

---

### 8.2 接口

```http
GET /api/video/capabilities
```

返回：

```json
{
  "textModel": {
    "available": true,
    "provider": "openai-compatible"
  },
  "hyperframes": {
    "available": true,
    "serviceUrl": "http://127.0.0.1:8787",
    "contractVersion": "aios-hyperframes-render-v1",
    "supports": ["render", "lint", "captionOverlay", "videoClipComposition"]
  },
  "ffmpeg": {
    "available": true
  },
  "seedance": {
    "available": true,
    "maxDurationSec": 15,
    "supportsImageReference": true,
    "supportsVideoReference": true
  },
  "tts": {
    "available": false,
    "setupRequired": true
  },
  "asr": {
    "available": true
  },
  "recommendations": [
    "当前适合生成图文口播视频",
    "可使用 Seedance 生成短 B-roll",
    "TTS 未配置，无法自动生成口播音频"
  ]
}
```

---

## 9. Proposal 阶段

所有高成本视频生成任务都应先进入 Proposal 阶段。

### 9.1 不直接生成

错误方式：

```text
用户：做一个端午节视频
  ↓
直接开始调用 Seedance
```

正确方式：

```text
用户：做一个端午节视频
  ↓
系统给 2-3 个方案
  ↓
用户选择
  ↓
再生成
```

---

### 9.2 Proposal 应包含

```text
1. 视频风格
2. 目标时长
3. 是否使用 Seedance
4. 是否使用 HyperFrames
5. 预计素材数量
6. 成本等级
7. 一致性风险
8. 推荐方案
```

---

## 10. Decision Log 决策日志

每次关键选择必须记录。

### 10.1 需要记录的决策

```text
pipeline_selection
proposal_selection
render_strategy_selection
provider_selection
runtime_selection
fallback_selection
```

---

### 10.2 示例

```json
{
  "decisionId": "decision_render_strategy_001",
  "decisionType": "render_strategy_selection",
  "optionsConsidered": [
    {
      "mode": "hyperframes_only",
      "cost": "low",
      "risk": "low",
      "weakness": "复杂动态画面较弱"
    },
    {
      "mode": "seedance_heavy",
      "cost": "high",
      "risk": "high",
      "weakness": "长视频一致性风险较高"
    },
    {
      "mode": "hybrid",
      "cost": "medium",
      "risk": "medium",
      "recommendation": true
    }
  ],
  "selected": "hybrid",
  "approvedByUser": true,
  "timestamp": "2026-06-24T00:00:00Z"
}
```

---

## 11. Dual-Engine 双引擎渲染策略

### 11.1 规则

```text
HyperFrames 用于：
- 字幕
- 标题
- 文字卡片
- 图表
- 水印
- UI
- 已有 MP4 剪辑
- 最终合成

Seedance 用于：
- 复杂动态背景
- 人物动作
- 不存在的 B-roll
- 电影感镜头
- 自然场景运动

Hybrid 用于：
- Seedance 生成背景
- HyperFrames 叠加字幕、标题、知识卡片、BGM、转场
```

---

### 11.2 决策工具

新增：

```text
visual_feasibility_analyzer
render_strategy_planner
```

输出：

```json
{
  "shotId": "SHOT_02",
  "renderMode": "hybrid",
  "elements": [
    {
      "id": "background",
      "engine": "seedance",
      "reason": "复杂自然动态场景"
    },
    {
      "id": "caption",
      "engine": "hyperframes",
      "reason": "字幕需要准确"
    },
    {
      "id": "info_card",
      "engine": "hyperframes",
      "reason": "文字卡片需要可控排版"
    }
  ]
}
```

---

## 12. Seedance 长视频一致性机制

### 12.1 Consistency Pack

所有需要 Seedance 生成的项目必须先生成一致性包：

```text
consistency_pack/
├── style_bible.md
├── character_sheet.json
├── scene_sheet.json
├── prop_sheet.json
├── color_palette.json
├── camera_rules.json
├── negative_prompt.md
└── reference_images/
```

---

### 12.2 每个 Seedance Shot 必须受控

```json
{
  "shotId": "SHOT_04",
  "durationSec": 8,
  "prompt": "...",
  "negativePrompt": "...",
  "referenceImages": [
    "reference_images/style_main.png",
    "reference_images/scene_main.png"
  ],
  "previousEndFrame": "seedance/frames/shot_03_last.png",
  "nextStartRequirement": "从江面水波转入知识卡片"
}
```

---

### 12.3 生成后必须做 QC

```json
{
  "artifactKind": "seedance_clip_qc",
  "shotId": "SHOT_04",
  "passed": true,
  "score": 86,
  "checks": {
    "styleConsistency": true,
    "sceneConsistency": true,
    "characterConsistency": true,
    "durationValid": true,
    "noTextArtifacts": true
  },
  "issues": []
}
```

---

## 13. 真实素材检索作为第三条路径

OpenMontage 的重要启发是：不要只在 HyperFrames 和 Seedance 之间选。

还应有第三条路径：

```text
真实素材检索 / 用户素材 / 素材库
```

决策顺序：

```text
如果用户已有素材可用
  → 优先使用已有素材 + HyperFrames

如果素材库有合适 B-roll
  → 使用素材库 + HyperFrames

如果没有素材且需要复杂动态
  → 使用 Seedance

如果只是文字和组件
  → 使用 HyperFrames
```

新增工具：

```text
local_media_indexer
stock_video_search
clip_retriever
clip_ranker
license_checker
```

---

## 14. Final Review 最终质检

### 14.1 不要只用一个泛化 quality_checker

拆成多个确定性检查：

```text
ffprobe_validator
duration_checker
audio_level_checker
subtitle_sync_checker
black_frame_checker
frame_sampler
artifact_completeness_checker
render_strategy_consistency_checker
```

---

### 14.2 `final_review.json`

```json
{
  "artifactKind": "final_review",
  "passed": true,
  "score": 91,
  "checks": {
    "fileExists": true,
    "durationValid": true,
    "hasAudio": true,
    "subtitleExists": true,
    "blackFrameDetected": false,
    "runtimeSwapDetected": false,
    "artifactComplete": true
  },
  "warnings": [],
  "finalVideo": {
    "path": "renders/final.mp4",
    "durationSec": 59.7,
    "resolution": "1920x1080"
  }
}
```

---

## 15. 工作区规范

每个项目都应固定工作区：

```text
projects/{projectId}/
├── artifacts/
│   ├── video_brief.json
│   ├── proposal_packet.json
│   ├── script.json
│   ├── shot_list.json
│   ├── visual_plan.json
│   ├── render_strategy.json
│   ├── consistency_pack.json
│   ├── asset_manifest.json
│   ├── video_composition_spec.json
│   ├── render_report.json
│   └── final_review.json
├── assets/
│   ├── images/
│   ├── video/
│   ├── audio/
│   ├── music/
│   └── subtitles/
├── seedance/
│   ├── prompts/
│   ├── references/
│   ├── clips/
│   ├── frames/
│   └── qc/
├── hyperframes/
│   ├── index.html
│   ├── assets/
│   ├── manifest.json
│   └── DESIGN.md
└── renders/
    └── final.mp4
```

---

## 16. 模块改造方案

### 16.1 后端新增模块

```text
cloud-backend/internal/core/video/
├── pipeline/
│   ├── manifest.go
│   ├── selector.go
│   └── runner.go
├── artifact/
│   ├── schema.go
│   ├── validator.go
│   └── store.go
├── composition/
│   ├── spec.go
│   ├── validator.go
│   └── builder.go
├── strategy/
│   ├── render_strategy.go
│   ├── feasibility.go
│   └── decision_log.go
├── media/
│   ├── probe.go
│   ├── ffmpeg.go
│   └── transcode.go
└── review/
    ├── final_review.go
    └── validators.go
```

---

### 16.2 新增工具

```text
pipeline_selector
capability_preflight
proposal_generator
visual_feasibility_analyzer
render_strategy_planner
consistency_pack_builder
video_composition_builder
media_probe
audio_extractor
asr_transcriber
clip_planner
ffmpeg_clip_extractor
seedance_prompt_builder
seedance_clip_generator
seedance_clip_qc
ffprobe_validator
frame_sampler
final_review_generator
```

---

## 17. 实施阶段

### Phase 1：Studio Pipeline Core

目标：让视频生产从动态工具调用升级为 Pipeline 驱动。

任务：

```text
1. 新增 video-pipelines/*.yaml
2. 新增 Pipeline Selector
3. 新增 Stage Director 基础框架
4. 定义 canonical artifact schema
5. 增加 proposal 阶段和用户审核
```

验收：

```text
用户一句话后，系统先输出 proposal_packet，而不是直接生成视频。
```

---

### Phase 2：Dual-Engine Render Strategy

目标：实现 HyperFrames / Seedance / Hybrid 的镜头级策略选择。

任务：

```text
1. 新增 visual_feasibility_analyzer
2. 新增 render_strategy_planner
3. 新增 render_strategy.json
4. 新增 decision_log
5. 用户批准 render_strategy 后才调用 Seedance
```

验收：

```text
每个 shot 都有 engine 选择和 reason。
```

---

### Phase 3：VideoCompositionSpec

目标：让所有视频最终统一进入 composition spec。

任务：

```text
1. 定义 VideoCompositionSpec v1
2. 改造 hyperframes_project_generator
3. 支持 video/audio/caption/overlay tracks
4. 支持用户上传 MP4
5. 支持 Seedance clips 作为素材输入
```

验收：

```text
不管素材来自 Seedance、上传 MP4、图片还是字幕，都能进入统一 composition spec。
```

---

### Phase 4：Final Review

目标：最终视频交付前自动质检。

任务：

```text
1. ffprobe_validator
2. duration_checker
3. audio_level_checker
4. subtitle_sync_checker
5. frame_sampler
6. final_review.json
```

验收：

```text
final.mp4 生成后必须通过 final_review 才能进入 READY_FOR_PUBLISH。
```

---

### Phase 5：Reference Video Remix

目标：支持参考视频分析和原创复刻。

任务：

```text
1. reference_video_analyzer
2. keyframe_extractor
3. pacing_analyzer
4. caption_style_analyzer
5. reference_analysis_report
6. remix proposal generator
```

验收：

```text
用户上传参考视频后，系统能分析结构、节奏、字幕风格，并生成原创改编方案。
```

---

## 18. 前端产品体验

前端应从“执行日志”升级为“视频生产工作台”。

### 18.1 页面结构

```text
VideoForge Studio Workspace
├── Capability Preflight
├── Proposal Options
├── Pipeline Timeline
├── Stage Artifact Preview
├── Render Strategy Board
├── Asset Board
├── Composition Preview
├── Render Progress
├── Final Review
└── Publish Package
```

---

### 18.2 Render Strategy Board

用户能看到：

```text
SHOT_01：HyperFrames
原因：标题和字幕需要准确文字
成本：低
风险：低

SHOT_02：Seedance
原因：需要复杂动态江河和龙舟
成本：中
风险：中
需要参考图：是

SHOT_03：Hybrid
原因：Seedance 生成背景，HyperFrames 叠加知识卡片
成本：中
风险：中
```

用户必须能：

```text
1. 确认策略
2. 修改某个 shot 的引擎
3. 降低 Seedance 使用比例
4. 改成低成本 HyperFrames-only
5. 查看预计成本
```

---

## 19. 给 Coding Agent 的任务指令

```text
目标：
实施 VideoForge Studio 升级。该升级吸收 OpenMontage 的 Pipeline Manifest、Stage Director、Canonical Artifact、Capability Preflight、Decision Log、Final Review 思想，但不复制其代码。系统应从动态工具调用升级为可审计的视频生产流水线，并支持 HyperFrames / Seedance / Hybrid 的双引擎渲染策略。

P0：
1. 新增 cloud-backend/video-pipelines/knowledge-video.yaml。
2. 新增 Pipeline Selector。
3. 新增 proposal_generator 工具。
4. 定义 proposal_packet.json schema。
5. 用户确认 proposal 后才进入 script。
6. 定义 render_strategy.json schema。
7. 新增 visual_feasibility_analyzer 和 render_strategy_planner。
8. render_strategy 必须进入 ArtifactReview。
9. 用户确认 render_strategy 后才允许调用 Seedance。
10. 新增 decision_log 表或 artifact。

P1：
1. 定义 VideoCompositionSpec v1。
2. 新增 video_composition_builder。
3. 改造 hyperframes_project_generator，优先从 compositionSpec 生成 HTML。
4. 支持 video/audio/caption/overlay tracks。
5. 支持 Seedance clip 和上传 MP4 作为 video track。
6. 支持字幕和信息卡片作为 overlay/caption track。

P2：
1. 新增 consistency_pack_builder。
2. 新增 seedance_prompt_builder。
3. 新增 seedance_clip_qc。
4. 新增 frame_extractor。
5. 新增 continuity_checker。
6. Seedance 每个 clip 生成后必须质检。
7. 失败时允许重试或 fallback 到 HyperFrames parallax。

P3：
1. 新增 final_review 阶段。
2. 实现 ffprobe_validator。
3. 实现 duration_checker。
4. 实现 audio_level_checker。
5. 实现 black_frame_checker。
6. 实现 artifact_completeness_checker。
7. 生成 final_review.json。
8. final_review 通过后，publish_package 状态才允许 READY_FOR_PUBLISH。

P4：
1. 新增 reference-video-remix pipeline。
2. 新增 reference_video_analyzer。
3. 支持参考视频抽帧、转录、节奏分析、字幕样式分析。
4. 生成 reference_analysis_report。
5. 基于参考视频生成原创 proposal。
```

---

## 20. 验收标准

### 20.1 一句话知识视频

输入：

```text
请帮我根据端午节的来历创作一个 60 秒知识分享视频。
```

必须输出：

```text
proposal_packet
script
shot_list
visual_plan
render_strategy
video_composition_spec
hyperframes_project
final.mp4
final_review
publish_package
```

---

### 20.2 双引擎策略

必须看到：

```text
哪些 shot 用 HyperFrames
哪些 shot 用 Seedance
哪些 shot 用 Hybrid
每个选择都有 reason
Seedance 调用前必须用户批准
```

---

### 20.3 最终视频

必须满足：

```text
final.mp4 存在
时长合理
有音频
有字幕
没有黑屏
最终包包含所有核心 artifact
```

---

## 21. 最终结论

VideoForge Studio 的核心升级是：

```text
从“Agent 动态调用工具”
升级为
“视频生产 Pipeline 系统”
```

它吸收 OpenMontage 的核心思想，但结合你的 AIOS 做成产品化版本：

```text
Pipeline Manifest
Stage Director
Canonical Artifact
Capability Preflight
Proposal Approval
Dual-Engine Render Strategy
Decision Log
Consistency Pack
VideoCompositionSpec
HyperFrames Render
Final Review
Publish Package
```

最终你的专用视频创作 Agent 应具备：

```text
1. 能先给方案，而不是直接生成
2. 能判断每个画面用 HyperFrames 还是 Seedance
3. 能管理 Seedance 长视频一致性
4. 能把所有素材统一合成到 HyperFrames
5. 能对最终视频做确定性质检
6. 能将整个过程沉淀为 artifact 和 decision log
```

一句话总结：

**VideoForge Studio 的目标不是让大模型“直接生成视频”，而是让大模型成为视频生产流水线的导演，让 AIOS 成为可控、可审计、可扩展的视频生产系统。**
