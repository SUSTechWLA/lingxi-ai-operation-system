# Guided Video Studio v1：下一步完整优化解决方案

## 1. 本轮优化目标

当前系统已经具备：

```text
1. Guided Video UI 雏形
2. Review API 基础闭环
3. PlanGuard 本地能力校验入口
4. NodeExecutor 本地工具 LocalJob 分发逻辑
5. local-backend 核心 executor
6. localrunner / pending_report / heartbeat
7. 默认 guided workflow
```

但第一版还不能上线，核心缺口是：

```text
1. 工具 Manifest 未完全标准化
2. LLMPlanner 仍未真正动态工具召回
3. 给大模型的工具上下文不完整
4. humanReview 没有结构化落到 Go Manifest
5. 第一版关键工具缺失
6. local 工具 manifest 没完全标注 local execution
7. 还缺完整 E2E 验证
```

本轮目标：

```text
把系统推进到：
一句话启动视频创作
  ↓
大模型根据相关工具自主编排
  ↓
系统自动插入审核节点
  ↓
用户分阶段确认
  ↓
本地生成 HyperFrames 项目
  ↓
本地预览 / 渲染 final.mp4
  ↓
前端预览 / 导出
```

核心原则：

```text
大模型负责编排，不负责绕过审核。
ToolManifest 声明能力、执行面、审核、质量策略。
PlanGuard 阻止不可执行计划。
PlanCompiler 自动插入审核节点。
Orchestrator 强制等待用户确认。
local-backend 执行本地工具。
```

---

# 2. 优化总路线

本轮分 6 个阶段：

```text
Phase 1：ToolManifest 标准化
Phase 2：Planner 动态编排升级
Phase 3：PlanGuard / PlanCompiler / LocalJob 主链路加固
Phase 4：补齐第一版图文视频关键工具
Phase 5：Review API + 前端 Guided Video Studio 闭环
Phase 6：E2E 验收与上线门禁
```

优先级：

```text
P0：不修就无法跑通第一版
P1：不修会影响产品体验或稳定性
P2：后续增强项
```

---

# 3. P0：ToolManifest 标准化

## 3.1 统一 ToolManifest 字段

所有工具必须统一支持以下字段：

```yaml
name: hyperframes_renderer
type: local_tool

executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_RENDER

localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - node
    - ffmpeg
  minDiskMb: 2048
  requiresNetwork: false

capabilities:
  - video_render
  - hyperframes
  - mp4_render

approvalPolicy:
  required: true
  mode: before_execute
  blocksDownstream: true
  reason: 最终渲染耗时较长，并会产生本地大文件，必须用户确认后执行。
  reviewArtifactKinds:
    - PREVIEW_SNAPSHOTS
    - VIDEO_COMPOSITION_SPEC

humanReview:
  required: true
  gate: before_execute
  title: 确认最终渲染
  reviewFocus:
    - 预览画面是否已经确认
    - 视频结构是否正确
    - 是否允许开始本地渲染
  userActions:
    - approve
    - reject
    - back_to_preview

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - VIDEO
  storage: local
  syncMetadataToCloud: true
  syncFileToCloud: false

qualityPolicy:
  required: true
  checks:
    - file_exists
    - file_size_valid
    - ffmpeg_probe
    - duration_valid

costLevel: medium
riskLevel: medium
sideEffect: true
```

---

## 3.2 Go 结构体必须补齐

在 `cloud-backend/internal/core/worker/tool/manifest.go` 中补齐：

```go
type ExecutionPlane string

const (
    ExecutionPlaneCloud      ExecutionPlane = "cloud"
    ExecutionPlaneLocal      ExecutionPlane = "local"
    ExecutionPlaneRemoteHTTP ExecutionPlane = "remote_http"
    ExecutionPlaneHybrid     ExecutionPlane = "hybrid"
)

type ArtifactLocation string

const (
    ArtifactLocationCloud ArtifactLocation = "cloud"
    ArtifactLocationLocal ArtifactLocation = "local"
    ArtifactLocationBoth  ArtifactLocation = "both"
)

type LocalRequirements struct {
    OS              []string `json:"os,omitempty" yaml:"os,omitempty"`
    Commands        []string `json:"commands,omitempty" yaml:"commands,omitempty"`
    MinDiskMb       int      `json:"minDiskMb,omitempty" yaml:"minDiskMb,omitempty"`
    RequiresNetwork bool     `json:"requiresNetwork,omitempty" yaml:"requiresNetwork,omitempty"`
}

type HumanReviewPolicy struct {
    Required    bool     `json:"required" yaml:"required"`
    Gate        string   `json:"gate,omitempty" yaml:"gate,omitempty"`
    Title       string   `json:"title,omitempty" yaml:"title,omitempty"`
    ReviewFocus []string `json:"reviewFocus,omitempty" yaml:"reviewFocus,omitempty"`
    UserActions []string `json:"userActions,omitempty" yaml:"userActions,omitempty"`
}
```

`ToolManifest` 增加：

```go
ExecutionPlane     ExecutionPlane     `json:"executionPlane,omitempty" yaml:"executionPlane,omitempty"`
RequiresUserDevice bool               `json:"requiresUserDevice,omitempty" yaml:"requiresUserDevice,omitempty"`
ArtifactLocation   ArtifactLocation   `json:"artifactLocation,omitempty" yaml:"artifactLocation,omitempty"`
LocalCommand       string             `json:"localCommand,omitempty" yaml:"localCommand,omitempty"`
LocalRequirements  *LocalRequirements `json:"localRequirements,omitempty" yaml:"localRequirements,omitempty"`
HumanReview        HumanReviewPolicy  `json:"humanReview,omitempty" yaml:"humanReview,omitempty"`
```

---

## 3.3 approvalPolicy 与 humanReview 的职责

```text
approvalPolicy：
系统执行策略，给 PlanCompiler 和 Orchestrator 使用。

humanReview：
展示策略，给 LLMPlanner 和前端使用。
```

关系：

```text
approvalPolicy.required         决定是否插 CONTROL 节点
approvalPolicy.mode             决定审核节点插入位置
approvalPolicy.blocksDownstream 决定是否阻断下游

humanReview.title               前端审核标题
humanReview.reviewFocus         前端审核重点
humanReview.userActions         前端可用按钮
```

---

## 3.4 修复 local 工具 manifest

### hyperframes_project_generator.tool.yaml

```yaml
name: hyperframes_project_generator
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_PROJECT_GENERATE

localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - node
  minDiskMb: 512
  requiresNetwork: false

approvalPolicy:
  required: false
  mode: none
  blocksDownstream: false

humanReview:
  required: false

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - HYPERFRAMES_PROJECT
  storage: local
  syncMetadataToCloud: true
  syncFileToCloud: false
```

### hyperframes_renderer.tool.yaml

```yaml
name: hyperframes_renderer
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_RENDER

localRequirements:
  os:
    - darwin
    - windows
    - linux
  commands:
    - node
    - ffmpeg
  minDiskMb: 2048
  requiresNetwork: false

approvalPolicy:
  required: true
  mode: before_execute
  blocksDownstream: true
  reason: 最终渲染会消耗本地资源并生成大文件，必须确认预览后执行。
  reviewArtifactKinds:
    - PREVIEW_SNAPSHOTS
    - VIDEO_COMPOSITION_SPEC

humanReview:
  required: true
  gate: before_execute
  title: 确认最终渲染
  reviewFocus:
    - 预览截图是否正常
    - 文字是否溢出
    - 是否开始生成 final.mp4
  userActions:
    - approve
    - reject
    - back_to_preview

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - VIDEO
  storage: local
  syncMetadataToCloud: true
  syncFileToCloud: false
```

---

# 4. P0：LLMPlanner 动态编排升级

## 4.1 接入 HybridToolRetriever

当前问题：

```text
LLMPlanner 仍然使用 HeuristicPlanner 先筛工具。
这不符合“大模型自主编排”的目标。
```

目标：

```text
HybridToolRetriever 负责召回相关工具全集。
LLMPlanner 负责基于工具全集自主编排 AgentPlan。
HeuristicPlanner 只作为 fallback。
```

修改：

```go
retriever := NewHybridToolRetriever(p.tools.ListManifests())

selected, err := retriever.Retrieve(ctx, RetrieveRequest{
    Query: req.Message,
    Domain: domain,
    MaxCostLevel: req.MaxCostLevel,
    MaxRiskLevel: req.MaxRiskLevel,
    CoarseTopK: 30,
    PlannerTopK: 12,
    IncludeCapabilities: []string{
        "video_planning",
        "script_generation",
        "video_composition",
        "hyperframes",
        "preview",
        "video_render",
        "artifact_package",
        "quality_check",
    },
    ExcludeCapabilities: []string{
        "seedance",
        "tts",
        "asr",
        "platform_publish",
    },
})
if err != nil || len(selected) == 0 {
    selected = fallbackHeuristicPlanner.Select(...)
}
```

---

## 4.2 第一版工具 allowlist

第一版只让大模型看到这些工具：

```text
capability_preflight
proposal_generator
video_script_generator
script_quality_checker
video_composition_builder
composition_quality_checker
hyperframes_project_generator
hyperframes_snapshot
hyperframes_renderer
ffmpeg_probe
final_review_generator
artifact_packager
package_quality_checker
```

暂时不要暴露：

```text
seedance_clip_generator
video_prompt_generator
keyframe_prompt_generator
tts_generator
asr_transcriber
platform_publish_api
complex_video_editor
local_media_indexer
ffmpeg_clip_extractor
audio_extract
```

原因：

```text
第一版目标是图文视频，不是完整动态 AI 视频。
降低工具空间，提升编排稳定性。
```

---

## 4.3 compactToolManifests 补字段

给大模型的工具摘要必须补齐：

```json
{
  "name": "hyperframes_renderer",
  "description": "Render HyperFrames project to final MP4.",
  "type": "local_tool",
  "executionPlane": "local",
  "requiresUserDevice": true,
  "artifactLocation": "local",
  "localCommand": "HYPERFRAMES_RENDER",
  "localRequirements": {
    "commands": ["node", "ffmpeg"],
    "minDiskMb": 2048
  },
  "capabilities": [
    "video_render",
    "hyperframes",
    "mp4_render"
  ],
  "approvalPolicy": {
    "required": true,
    "mode": "before_execute",
    "blocksDownstream": true,
    "reason": "最终渲染需要用户确认。"
  },
  "humanReview": {
    "required": true,
    "title": "确认最终渲染",
    "reviewFocus": [
      "预览是否已确认",
      "是否允许生成 final.mp4"
    ],
    "userActions": [
      "approve",
      "reject",
      "back_to_preview"
    ]
  },
  "artifactPolicy": {
    "produceArtifact": true,
    "artifactKinds": ["VIDEO"],
    "storage": "local"
  },
  "qualityPolicy": {
    "required": true,
    "checks": [
      "ffmpeg_probe",
      "file_size_valid",
      "duration_valid"
    ]
  },
  "nextRecommendedTools": [
    "ffmpeg_probe",
    "final_review_generator",
    "artifact_packager"
  ]
}
```

---

## 4.4 LLM 系统提示重写

```text
你是 Dynamic Guided Video Planner。

你负责：
1. 根据用户需求和候选工具生成 AgentPlan。
2. 自主决定工具顺序、依赖关系和参数引用。
3. 只使用候选工具，不得发明工具。
4. 尊重每个工具的 executionPlane。
5. 尊重每个工具的 approvalPolicy 和 humanReview。
6. 不需要手写审核节点，系统会根据 ToolManifest 自动插入。
7. 不得绕过需要人工审核的工具。
8. requiresUserDevice=true 的工具必须在 preflight 之后使用。
9. 第一版只生成图文视频，不使用 Seedance、TTS、ASR、平台发布。
10. hyperframes_renderer 只能在预览确认后执行。
```

---

# 5. P0：PlanGuard 加固

## 5.1 入口改造

统一使用：

```go
ValidatePlan(ctx context.Context, userID string, plan *AgentPlan) error
```

禁止继续使用没有 userID 的：

```go
Validate(plan)
```

---

## 5.2 本地能力校验

PlanGuard 必须校验：

```text
1. local runner 是否在线
2. runner 是否支持 localCommand
3. runner 是否满足 localRequirements.commands
4. runner 是否满足 minDiskMb
5. requiresUserDevice=true 时是否有 device
6. artifactLocation=local 时是否可写本地 artifact
```

伪代码：

```go
for _, step := range plan.Steps {
    manifest := toolRegistry.GetManifest(step.Tool)
    if manifest == nil {
        return ErrToolNotFound
    }

    if manifest.ExecutionPlane == tool.ExecutionPlaneLocal ||
       manifest.ExecutionPlane == tool.ExecutionPlaneHybrid {
        if err := localCapabilityValidator.ValidateForLocalExecution(ctx, userID, step, manifest); err != nil {
            return err
        }
    }
}
```

---

## 5.3 审核策略校验

PlanGuard 必须校验：

```text
1. approvalPolicy.required=true 的工具必须能被 PlanCompiler 插入 CONTROL。
2. approvalPolicy.blocksDownstream=true 时，下游必须依赖审核节点。
3. humanReview.required=true 时，必须有 review title / reviewFocus。
4. hyperframes_renderer 前必须存在 preview 或 composition 相关前置产物。
5. 第一版不允许出现 Seedance/TTS/ASR/平台发布工具。
```

错误示例：

```json
{
  "code": "LOCAL_RUNNER_NOT_AVAILABLE",
  "tool": "hyperframes_renderer",
  "message": "本地执行器未在线，无法执行视频渲染。"
}
```

```json
{
  "code": "REVIEW_GATE_REQUIRED",
  "tool": "video_script_generator",
  "message": "该工具要求人工审核，但计划无法插入审核节点。"
}
```

---

# 6. P0：LocalJob 主链路确认

## 6.1 本地工具必须走 LocalJob

规则：

```text
executionPlane=local
  ↓
NodeExecutor 不得直接执行工具
  ↓
创建 LocalJob
  ↓
ai_node 状态 WAITING_LOCAL
  ↓
local-backend claim
  ↓
local-backend Execute
  ↓
complete/fail 回传
  ↓
cloud 推进 DAG
```

---

## 6.2 必须走 LocalJob 的工具

```text
hyperframes_project_generator
hyperframes_snapshot
hyperframes_renderer
ffmpeg_probe
artifact_packager
```

如果它们被 cloud 直接执行，即判定为失败。

---

## 6.3 LocalJob 输出标准

```json
{
  "jobId": "local_job_001",
  "taskId": "task_001",
  "nodeId": "render_exec",
  "toolName": "hyperframes_renderer",
  "command": "HYPERFRAMES_RENDER",
  "status": "COMPLETED",
  "output": {
    "success": true,
    "artifacts": [
      {
        "kind": "VIDEO",
        "name": "final.mp4",
        "storageRef": "local://projects/task_001/renders/final.mp4",
        "mimeType": "video/mp4",
        "localOnly": true
      }
    ]
  }
}
```

---

# 7. P0：补齐第一版缺失工具

## 7.1 video_composition_builder

### Manifest

```yaml
name: video_composition_builder
type: cloud_prompt_tool
executionPlane: cloud
requiresUserDevice: false
artifactLocation: cloud

capabilities:
  - video_composition
  - image_text_video
  - composition_spec

approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: 视频结构会决定后续画面布局和渲染，必须确认。

humanReview:
  required: true
  gate: after_artifact
  title: 审核视频结构
  reviewFocus:
    - 卡片顺序是否合理
    - 每页文字是否过长
    - 每段时长是否合理
    - 字幕是否完整覆盖内容
  userActions:
    - approve
    - edit
    - regenerate
    - reject

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - VIDEO_COMPOSITION_SPEC
  storage: cloud
```

### 输出

```json
{
  "artifactKind": "VIDEO_COMPOSITION_SPEC",
  "specVersion": "aios-video-composition-v1",
  "projectType": "image_text_video",
  "durationSec": 45,
  "fps": 30,
  "resolution": {
    "width": 1920,
    "height": 1080
  },
  "tracks": [
    {
      "id": "overlay_track_01",
      "type": "overlay",
      "items": [
        {
          "id": "card_001",
          "kind": "title_card",
          "startSec": 0,
          "endSec": 6,
          "title": "AI Agent 改变的是工作流",
          "body": "不是替代某个岗位，而是重构任务流。"
        }
      ]
    },
    {
      "id": "caption_track_01",
      "type": "caption",
      "items": [
        {
          "id": "caption_001",
          "startSec": 0,
          "endSec": 6,
          "text": "AI Agent 改变的是工作流。"
        }
      ]
    }
  ]
}
```

---

## 7.2 hyperframes_snapshot

### Manifest

```yaml
name: hyperframes_snapshot
type: local_tool
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_SNAPSHOT

localRequirements:
  commands:
    - node
  minDiskMb: 512

capabilities:
  - hyperframes
  - preview
  - snapshot

approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: 用户必须确认预览画面后才能进入最终渲染。

humanReview:
  required: true
  gate: after_artifact
  title: 审核画面预览
  reviewFocus:
    - 文字是否溢出
    - 字体是否可读
    - 卡片顺序是否正确
    - 是否允许开始最终渲染
  userActions:
    - approve
    - reject
    - regenerate
    - back_to_composition
```

### local-backend executor

新增：

```text
local-backend/internal/localtool/hyperframes_snapshot.go
local-backend/internal/localtool/hyperframes_snapshot_test.go
```

输出：

```json
{
  "artifactKind": "PREVIEW_SNAPSHOTS",
  "snapshots": [
    "local://projects/task_001/previews/snapshot_001.png",
    "local://projects/task_001/previews/snapshot_002.png",
    "local://projects/task_001/previews/snapshot_003.png"
  ]
}
```

---

## 7.3 final_review_generator

### Manifest

```yaml
name: final_review_generator
type: builtin_tool
executionPlane: cloud
requiresUserDevice: false
artifactLocation: cloud

capabilities:
  - final_review
  - video_quality_check

approvalPolicy:
  required: false

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - FINAL_REVIEW
  storage: cloud

qualityPolicy:
  required: true
  checks:
    - final_video_exists
    - file_size_valid
    - duration_valid
    - artifact_complete
```

输出：

```json
{
  "artifactKind": "FINAL_REVIEW",
  "passed": true,
  "checks": {
    "fileExists": true,
    "fileSizeValid": true,
    "durationValid": true,
    "hasVideoTrack": true
  },
  "finalVideo": {
    "storageRef": "local://projects/task_001/renders/final.mp4",
    "durationSec": 44.8
  }
}
```

---

## 7.4 skillcap.yaml 注册

必须加入：

```yaml
tools:
  - tools/proposal_generator.tool.yaml
  - tools/video_script_generator.tool.yaml
  - tools/script_quality_checker.tool.yaml
  - tools/video_composition_builder.tool.yaml
  - tools/composition_quality_checker.tool.yaml
  - tools/hyperframes_project_generator.tool.yaml
  - tools/hyperframes_snapshot.tool.yaml
  - tools/hyperframes_renderer.tool.yaml
  - tools/ffmpeg_probe.tool.yaml
  - tools/final_review_generator.tool.yaml
  - tools/artifact_packager.tool.yaml
  - tools/package_quality_checker.tool.yaml
```

---

# 8. P1：Review API 与前端闭环

## 8.1 Review API 必须完整

```http
GET /api/agent/runs/:runId/reviews
POST /api/agent/runs/:runId/reviews/:reviewId/approve
POST /api/agent/runs/:runId/reviews/:reviewId/reject
POST /api/agent/runs/:runId/reviews/:reviewId/submit-edited
POST /api/agent/runs/:runId/reviews/:reviewId/regenerate
```

---

## 8.2 reject 行为

```text
1. review node → REVISION_REQUIRED
2. 保存 revisionInstruction
3. 下游节点保持 BLOCKED
4. 允许重新生成 target node
```

---

## 8.3 submit-edited 行为

```text
1. 保存 editedArtifact
2. 标记 artifact source=user_edited
3. review node → SUCCESS
4. 写 decision_log
5. 下游继续
```

---

## 8.4 regenerate 行为

```text
1. target node 重新进入 READY
2. 带 revisionInstruction 重新生成 artifact
3. review node 重新 WAITING_REVIEW
4. 下游继续 blocked
```

---

## 8.5 前端改名

当前文件和导航如果还叫 OneClick，建议全部改：

```text
OneClickVideoPage.tsx → GuidedVideoStudioPage.tsx
activeNav: oneclick → guidedVideo
菜单：一键生成视频 → 引导式视频创作
```

推荐文案：

```text
一句话启动视频创作
分阶段确认后生成 MP4
```

禁止文案：

```text
一键出片
全自动生成最终视频
无需确认
```

---

# 9. P1：Preflight 强化

## 9.1 API

```http
GET /api/video/preflight
```

或在：

```http
POST /api/agent/runs
```

前自动执行。

---

## 9.2 检查内容

```text
1. local runner 是否在线
2. HYPERFRAMES_PROJECT_GENERATE 是否可用
3. HYPERFRAMES_SNAPSHOT 是否可用
4. HYPERFRAMES_RENDER 是否可用
5. FFMPEG_PROBE 是否可用
6. ARTIFACT_PACKAGE 是否可用
7. HyperFrames Render Service /health 是否通过
8. workspace 是否可写
9. 磁盘空间是否足够
```

失败时不能开始视频项目。

---

# 10. P1：decision_log

所有关键操作都写 decision_log：

```text
1. proposal approve/reject/edit/regenerate
2. script approve/reject/edit/regenerate
3. composition approve/reject/edit/regenerate
4. preview approve/reject/regenerate
5. final render approval
6. local render complete/fail
7. final_review result
```

结构：

```json
{
  "decisionId": "decision_script_approval_001",
  "runId": "run_001",
  "nodeId": "review_script",
  "type": "human_approval",
  "decision": "approve",
  "artifactKind": "VIDEO_SCRIPT",
  "comment": "脚本确认，继续生成结构。",
  "createdAt": "2026-06-25T00:00:00Z"
}
```

---

# 11. E2E 验收标准

## 11.1 标准输入

```text
请帮我做一个 45 秒视频，讲 AI Agent 改变的是工作流。
```

## 11.2 必须发生

```text
1. Preflight 通过
2. HybridToolRetriever 召回视频工具
3. LLMPlanner 生成 AgentPlan
4. compactToolManifests 中含 executionPlane/localCommand/humanReview
5. PlanGuard 校验通过
6. PlanCompiler 自动插入 review gates
7. proposal 生成后暂停等待确认
8. 用户确认 proposal 后生成 script
9. 用户确认 script 后生成 composition
10. 用户确认 composition 后生成 HyperFrames project
11. hyperframes_snapshot 生成预览图
12. 用户确认 preview 后创建 HYPERFRAMES_RENDER LocalJob
13. local-backend claim LocalJob
14. local-backend 渲染 final.mp4
15. ffmpeg_probe / final_review 通过
16. artifact_packager 输出 package
17. 前端可预览 final.mp4
18. 前端可打开文件夹 / 导出 package
```

## 11.3 不能通过的情况

```text
1. 预览未确认就创建 HYPERFRAMES_RENDER LocalJob
2. local 工具在 cloud 内直接执行
3. video_composition_builder 找不到
4. humanReview 字段被 YAML loader 忽略
5. local runner 离线时仍创建 DAG
6. 前端点击 submit-edited / regenerate 返回 404
7. final.mp4 生成后 cloud 不知道 artifact metadata
```

---

# 12. 实施顺序

## Phase 1：先修 Manifest 和编译

```text
1. ToolManifest 增加 executionPlane/localCommand/localRequirements/humanReview。
2. 修复 YAML loader。
3. 修复 local 工具 manifest。
4. cloud-backend go test ./...
5. local-backend go test ./...
```

验收：

```text
manifest 能正确解析 humanReview 和 local execution 字段。
```

---

## Phase 2：修 Planner

```text
1. LLMPlanner 接 HybridToolRetriever。
2. compactToolManifests 补字段。
3. HeuristicPlanner 降级 fallback。
4. 限制第一版工具 allowlist。
```

验收：

```text
LLM 能看到完整工具上下文。
```

---

## Phase 3：修第一版缺失工具

```text
1. video_composition_builder
2. hyperframes_snapshot
3. final_review_generator
4. skillcap.yaml 注册
5. local-backend 注册 HYPERFRAMES_SNAPSHOT
```

验收：

```text
第一版图文视频工具链完整。
```

---

## Phase 4：修本地链路

```text
1. NodeExecutor local 工具必须走 LocalJob。
2. preflight 检查 HYPERFRAMES_SNAPSHOT。
3. local-backend 能执行 HYPERFRAMES_SNAPSHOT。
4. final.mp4 metadata 回传 cloud。
```

验收：

```text
所有 local 工具都由 local-backend 执行。
```

---

## Phase 5：修 Review 产品闭环

```text
1. 前端改名 GuidedVideoStudio。
2. 对齐 approve/reject/edit/regenerate。
3. humanReview 驱动审核面板。
4. decision_log。
```

验收：

```text
用户可在每个阶段审核、编辑、重生成。
```

---

## Phase 6：完整 E2E

```text
1. 写 dynamic_guided_video_e2e.md。
2. 跑通一句话到 final.mp4。
3. 修所有失败路径。
4. 内测发布。
```

---

# 13. Coding Agent 执行指令

```text
目标：
将当前 Guided Video Studio 从产品雏形推进到第一版可内测。重点修 ToolManifest、Planner、缺失工具、本地 LocalJob、Review 前端闭环。禁止继续扩展 Seedance/TTS/ASR/发布平台。

P0：Manifest
1. 修改 cloud-backend/internal/core/worker/tool/manifest.go。
2. 增加 ExecutionPlane、ArtifactLocation、LocalRequirements、HumanReviewPolicy。
3. ToolManifest 增加 executionPlane、requiresUserDevice、artifactLocation、localCommand、localRequirements、humanReview。
4. 确认 YAML loader 能解析 humanReview。
5. 修改 hyperframes_project_generator.tool.yaml 为 local_tool。
6. 修改 hyperframes_renderer.tool.yaml 为 local_tool。
7. 所有第一版工具补 approvalPolicy / humanReview / artifactPolicy / qualityPolicy。

P1：Planner
1. LLMPlanner 接入 HybridToolRetriever。
2. HeuristicPlanner 只作为 fallback。
3. compactToolManifests 增加 executionPlane、requiresUserDevice、artifactLocation、localCommand、localRequirements、humanReview、qualityPolicy。
4. 第一版只允许图文视频工具 allowlist。
5. 更新 LLM system prompt：大模型负责编排，不负责绕过审核。

P2：缺失工具
1. 新增 video_composition_builder.tool.yaml 和实现。
2. 新增 hyperframes_snapshot.tool.yaml 和实现。
3. 新增 final_review_generator.tool.yaml 和实现。
4. skillcap.yaml 注册这三个工具。
5. local-backend 新增 HYPERFRAMES_SNAPSHOT executor。
6. localtool bootstrap 注册 HYPERFRAMES_SNAPSHOT。

P3：LocalJob
1. 确保 executionPlane=local 的工具必须创建 LocalJob。
2. 禁止 hyperframes_project_generator / hyperframes_renderer / hyperframes_snapshot / ffmpeg_probe / artifact_packager 在 cloud 内直接执行。
3. local-backend complete 后 cloud 更新 ai_node output 和 artifact metadata。

P4：PlanGuard
1. ValidatePlan(ctx,userID,plan) 必须作为唯一入口。
2. 启用 LocalCapabilityValidator。
3. 校验 local runner online、localCommand、localRequirements。
4. local runner 不在线时阻断计划。

P5：Review
1. Review API 支持 approve/reject/submit-edited/regenerate。
2. reject 后下游保持 blocked。
3. regenerate 后 target node 重新执行并再次进入 WAITING_REVIEW。
4. humanReview 驱动前端审核标题、重点、按钮。
5. 写 decision_log。

P6：Frontend
1. OneClickVideoPage 改名 GuidedVideoStudioPage。
2. activeNav oneclick 改 guidedVideo。
3. 文案改为“一句话启动视频创作”。
4. 展示 preflight、review artifact、humanReview.reviewFocus、LocalJob 状态。
5. preview 未确认前不允许渲染。
6. final.mp4 支持预览、打开文件夹、导出 package。

P7：E2E
1. 新增 dynamic_guided_video_e2e.md。
2. 验证一句话 → AgentPlan → review gates → LocalJob → final.mp4 → package。
3. cloud-backend go test ./...
4. local-backend go test ./...
5. frontend build 通过。
```

---

# 14. 第一版内测准入标准

完成后必须满足：

```text
[ ] 大模型能看到完整工具上下文
[ ] LLMPlanner 使用 HybridToolRetriever
[ ] 所有第一版工具 manifest 完整
[ ] humanReview 能被 Go 加载
[ ] PlanCompiler 自动插入审核节点
[ ] CONTROL 节点阻断下游
[ ] local 工具全部走 LocalJob
[ ] local-backend 能生成 preview snapshots
[ ] local-backend 能渲染 final.mp4
[ ] 用户能 approve/edit/regenerate/reject
[ ] final.mp4 能在前端预览
[ ] package 能导出
```

---

# 15. 最终结论

下一步最关键不是继续加功能，而是打通这条主链路：

```text
ToolManifest 标准化
  ↓
HybridToolRetriever
  ↓
LLMPlanner 动态编排
  ↓
PlanGuard 本地能力校验
  ↓
PlanCompiler 自动插审核
  ↓
Review API / 前端确认
  ↓
LocalJob 本地执行
  ↓
final.mp4
```

只要这条链路跑通，你的第一版 `Guided Video Studio` 就可以进入内测。

一句话总结：

**下一步不要再扩展视频能力，集中把 Manifest、Planner、缺失工具、LocalJob、Review、E2E 六件事打穿。**
