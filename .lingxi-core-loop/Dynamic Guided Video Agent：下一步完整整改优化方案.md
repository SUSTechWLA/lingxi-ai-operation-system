# Dynamic Guided Video Agent：下一步完整整改优化方案

## 1. 整改目标

当前系统下一步应从：

```text
固定 guided workflow + 部分动态 Agent
```

升级为：

```text
大模型自主编排 AgentPlan
  +
ToolManifest 声明工具能力、执行面、审核规则
  +
PlanGuard 校验本地能力和审核约束
  +
PlanCompiler 自动插入审核节点
  +
Orchestrator 状态机强制阻断
  +
local-backend 安全执行本地工具
```

最终原则：

```text
任务怎么编排：交给大模型
工具能不能用：ToolRetriever + PlanGuard 决定
工具是否需要审核：ToolManifest 决定
审核节点怎么插：PlanCompiler 决定
审核是否阻断：Orchestrator 决定
本地工具怎么执行：local-backend 决定
```

一句话：

**让大模型负责规划，不让大模型绕过审核。**

---

## 2. 当前要修正的核心问题

当前主要断点：

```text
1. LLMPlanner 仍偏 HeuristicPlanner，不是真正的相关工具召回 + 大模型自主编排。
2. compactToolManifests 给大模型的工具信息不完整。
3. ToolManifest 执行面字段、humanReview 字段需要统一。
4. PlanGuard 需要真正接入 LocalCapabilityValidator。
5. Dynamic Agent 的 local 工具需要明确走 LocalJob，不应在 cloud 内直接执行 builtin。
6. video_composition_builder、hyperframes_snapshot、final_review_generator 仍需补齐。
7. review API 需要补 submit-edited / regenerate。
8. 前端 Guided Video Studio 需要和后端 review API 完全对齐。
9. 固定 wf-guided-image-text-video 应降级为 fallback recipe，不应成为唯一主流程。
```

---

## 3. 新目标架构

## 3.1 主流程

```text
用户输入一句话
  ↓
/api/video/preflight
  ↓
HybridToolRetriever 召回视频创作相关工具
  ↓
LLMPlanner 基于工具能力自主生成 AgentPlan
  ↓
PlanGuard 校验工具、参数、本地能力、审核规则
  ↓
PlanCompiler 编译 DAG，并自动插入 CONTROL 审核节点
  ↓
Orchestrator 执行 DAG
  ↓
CONTROL 节点进入 WAITING_REVIEW
  ↓
用户审核通过
  ↓
继续执行下游节点
  ↓
local-backend 领取 LocalJob 执行本地工具
  ↓
final.mp4 + package + final_review
```

---

## 3.2 固定 workflow 的新定位

保留：

```text
wf-guided-image-text-video
```

但它不再是主流程，而是：

```text
1. fallback workflow
2. baseline recipe
3. LLMPlanner 编排参考样例
4. PlanGuard 校验参考模板
5. 测试用例模板
```

主入口应是：

```http
POST /api/agent/runs
```

固定 workflow 只在以下情况使用：

```text
1. LLMPlanner 失败
2. 用户明确选择“标准图文视频模板”
3. 系统处于降级模式
4. E2E 测试需要稳定基线
```

---

# 4. P0：ToolManifest 标准化整改

## 4.1 必须统一 ToolManifest 字段

所有工具必须具备以下字段：

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
    - PREVIEW
    - VIDEO_COMPOSITION_SPEC

humanReview:
  required: true
  gate: before_execute
  title: 确认最终渲染
  reviewFocus:
    - 预览画面是否已确认
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

## 4.2 approvalPolicy 与 humanReview 的分工

```text
approvalPolicy：
  给系统使用，决定是否插入 CONTROL 节点。

humanReview：
  给大模型和前端使用，描述审核标题、审核重点、用户操作。
```

映射关系：

```text
humanReview.required         → approvalPolicy.required
humanReview.gate             → approvalPolicy.mode
humanReview.userActions      → 前端按钮
humanReview.reviewFocus      → 审核面板提示
approvalPolicy.blocksDownstream → 状态机阻断
```

---

## 4.3 第一版工具必须补齐 humanReview

必须补齐以下工具：

```text
proposal_generator
video_script_generator
video_composition_builder
hyperframes_project_generator
hyperframes_snapshot
hyperframes_renderer
final_review_generator
artifact_packager
```

审核策略建议：

| 工具                            | executionPlane | humanReview  | gate           |
| ----------------------------- | -------------- | ------------ | -------------- |
| capability_preflight          | cloud          | false        | none           |
| proposal_generator            | cloud          | true         | after_artifact |
| video_script_generator        | cloud          | true         | after_artifact |
| video_composition_builder     | cloud          | true         | after_artifact |
| hyperframes_project_generator | local          | false 或 true | after_artifact |
| hyperframes_snapshot          | local          | true         | after_artifact |
| hyperframes_renderer          | local          | true         | before_execute |
| ffmpeg_probe                  | local          | false        | none           |
| final_review_generator        | cloud/local    | false        | none           |
| artifact_packager             | local          | false 或 true | after_artifact |

---

# 5. P0：LLMPlanner 改造成真正动态编排

## 5.1 接入 HybridToolRetriever

把当前：

```go
retriever := &HeuristicPlanner{tools: p.tools, maxTools: p.maxTools}
selected := retriever.selectTools(domain, req.Message)
```

改成：

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
```

第一版不要把所有工具都给模型，而是给“相关工具全集”。

推荐：

```text
CoarseTopK = 30
PlannerTopK = 12-16
```

---

## 5.2 第一版工具 allowlist

第一版只允许大模型看到：

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

第一版暂时不暴露：

```text
seedance_clip_generator
video_prompt_generator
keyframe_prompt_generator
tts_generator
asr_transcriber
publish_platform_api
complex_video_editor
local_media_indexer
ffmpeg_clip_extractor
audio_extract
```

目的：

```text
减少 LLM 编排空间，保证第一版稳定。
```

---

## 5.3 compactToolManifests 补齐字段

传给 LLM 的工具摘要必须包含：

```json
{
  "name": "video_script_generator",
  "description": "Generate a short Chinese voiceover script for image-text video.",
  "type": "cloud_prompt_tool",
  "executionPlane": "cloud",
  "requiresUserDevice": false,
  "artifactLocation": "cloud",
  "capabilities": [
    "script_generation",
    "video_planning"
  ],
  "parameters": {},
  "output": {},
  "approvalPolicy": {
    "required": true,
    "mode": "after_artifact",
    "blocksDownstream": true,
    "reason": "脚本会影响后续视频结构，必须确认。"
  },
  "humanReview": {
    "required": true,
    "title": "审核口播脚本",
    "reviewFocus": [
      "开头是否有吸引力",
      "表达是否自然",
      "内容是否准确",
      "时长是否合理"
    ],
    "userActions": [
      "approve",
      "edit",
      "regenerate",
      "reject"
    ]
  },
  "artifactPolicy": {
    "produceArtifact": true,
    "artifactKinds": [
      "VIDEO_SCRIPT"
    ]
  },
  "qualityPolicy": {
    "required": true,
    "checks": [
      "script_length",
      "section_structure",
      "language"
    ]
  },
  "nextRecommendedTools": [
    "video_composition_builder"
  ]
}
```

local 工具必须额外包含：

```json
{
  "executionPlane": "local",
  "requiresUserDevice": true,
  "artifactLocation": "local",
  "localCommand": "HYPERFRAMES_RENDER",
  "localRequirements": {
    "commands": ["node", "ffmpeg"],
    "minDiskMb": 2048
  }
}
```

---

## 5.4 LLM system prompt 硬约束

```text
你是 Dynamic Guided Video Planner。

你负责：
1. 根据用户需求和候选工具生成 AgentPlan。
2. 自主决定工具顺序、依赖关系和参数引用。
3. 只使用候选工具，不得发明工具。
4. 尊重每个工具的 approvalPolicy 和 humanReview。
5. 尊重每个工具的 executionPlane。
6. 本地工具 requiresUserDevice=true 时，必须确保前置包含 capability_preflight。
7. 你不需要手写审核节点，系统会自动根据 ToolManifest 插入。
8. 你不得绕过需要人工审核的工具。
9. 第一版只生成图文视频，不使用 Seedance、TTS、ASR、平台发布工具。
10. hyperframes_renderer 只能在 preview 或 composition 已确认后执行。
```

---

# 6. P0：PlanGuard 接入本地能力校验

## 6.1 修改接口

从：

```go
Validate(plan *AgentPlan) error
```

升级为：

```go
Validate(ctx context.Context, userID string, plan *AgentPlan) error
```

Runner 调用处也要改：

```go
err := r.guard.Validate(ctx, req.UserID, plan)
```

---

## 6.2 接入 LocalCapabilityValidator

校验逻辑：

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

## 6.3 必须校验

```text
1. 用户是否有 online local runner。
2. local runner 是否支持 localCommand。
3. local runner 是否满足 localRequirements.commands。
4. local runner 磁盘是否满足 minDiskMb。
5. requiresUserDevice=true 时是否有 deviceId。
6. artifactLocation=local 时是否可写本地 artifact。
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
  "code": "LOCAL_CAPABILITY_MISSING",
  "tool": "hyperframes_renderer",
  "message": "本地缺少 HYPERFRAMES_RENDER 能力，请确认 HyperFrames Render Service 已启动。"
}
```

---

# 7. P0：打通 cloud → LocalJob → local-backend 主链路

## 7.1 当前必须纠正的问题

Dynamic Agent 执行 local 工具时，不能在 cloud 中直接执行 builtin。

必须满足：

```text
executionPlane=local
  ↓
NodeExecutor 不执行工具
  ↓
创建 LocalJob
  ↓
ai_node 状态 WAITING_LOCAL
  ↓
local-backend claim
  ↓
local-backend Execute
  ↓
complete/fail
  ↓
cloud 推进 DAG
```

---

## 7.2 NodeExecutor 分发规则

新增或恢复：

```go
func (ne *NodeExecutor) ExecuteNode(ctx context.Context, node *model.Node) error {
    toolName := resolveToolName(node)
    manifest := ne.toolRegistry.GetManifest(toolName)

    switch manifest.ExecutionPlane {
    case tool.ExecutionPlaneLocal:
        return ne.dispatchLocalNode(ctx, node, manifest)

    case tool.ExecutionPlaneCloud:
        return ne.dispatchCloudNode(ctx, node, manifest)

    case tool.ExecutionPlaneRemoteHTTP:
        return ne.dispatchRemoteHTTPNode(ctx, node, manifest)

    case tool.ExecutionPlaneHybrid:
        return ne.dispatchHybridNode(ctx, node, manifest)

    default:
        return ne.dispatchCloudNode(ctx, node, manifest)
    }
}
```

---

## 7.3 LocalJob payload

```json
{
  "jobId": "local_job_001",
  "taskId": "task_001",
  "nodeId": "render_exec",
  "toolName": "hyperframes_renderer",
  "command": "HYPERFRAMES_RENDER",
  "payload": {
    "projectDir": "local://projects/task_001/hyperframes",
    "entry": "index.html",
    "outputPath": "local://projects/task_001/renders/final.mp4",
    "fps": 30,
    "quality": "standard"
  },
  "artifactPolicy": {
    "location": "local",
    "syncMetadataToCloud": true,
    "syncFileToCloud": false
  }
}
```

---

## 7.4 cloud main 必须注册 localrunner API

确保 main.go 注册：

```go
localRunnerService := localrunner.NewService(pool)
localrunner.NewHandler(localRunnerService, stateMachine, authMiddleware.RequireAuth()).RegisterRoutes(r)
nodeExecutor.SetLocalJobDispatcher(localRunnerService)
```

如果当前缺失，必须补。

---

# 8. P0：补齐第一版视频工具

## 8.1 video_composition_builder

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
  reason: 视频结构决定后续画面和渲染，必须人工确认。

humanReview:
  required: true
  gate: after_artifact
  title: 审核视频结构
  reviewFocus:
    - 卡片顺序是否合理
    - 每页文字是否过长
    - 每段时长是否合理
    - 字幕是否覆盖完整内容
  userActions:
    - approve
    - edit
    - regenerate
    - reject
```

### 输出

```json
{
  "artifactKind": "video_composition_spec",
  "specVersion": "aios-video-composition-v1",
  "durationSec": 45,
  "fps": 30,
  "resolution": {
    "width": 1920,
    "height": 1080
  },
  "tracks": [
    {
      "type": "overlay",
      "items": [
        {
          "kind": "title_card",
          "startSec": 0,
          "endSec": 6,
          "title": "AI Agent 改变的是工作流",
          "body": "不是替代某个岗位，而是重构任务流。"
        }
      ]
    },
    {
      "type": "caption",
      "items": [
        {
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

## 8.2 hyperframes_snapshot

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
    - 画面是否可读
    - 卡片顺序是否正确
    - 是否允许开始最终渲染
  userActions:
    - approve
    - reject
    - regenerate
    - back_to_composition
```

### 输出

```json
{
  "artifactKind": "preview_snapshots",
  "snapshots": [
    "local://projects/task_001/previews/snapshot_001.png",
    "local://projects/task_001/previews/snapshot_002.png",
    "local://projects/task_001/previews/snapshot_003.png"
  ]
}
```

---

## 8.3 final_review_generator

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
  "artifactKind": "final_review",
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

# 9. P0：Review API 闭环

## 9.1 必须支持的 API

```http
GET /api/agent/runs/:runId/reviews
POST /api/agent/runs/:runId/reviews/:reviewId/approve
POST /api/agent/runs/:runId/reviews/:reviewId/reject
POST /api/agent/runs/:runId/reviews/:reviewId/submit-edited
POST /api/agent/runs/:runId/reviews/:reviewId/regenerate
```

---

## 9.2 approve

```text
1. review node → SUCCESS
2. 写 decision_log
3. 调 StateMachine.OnSuccess
4. 下游节点 READY
```

---

## 9.3 reject

```text
1. review node → REVISION_REQUIRED
2. 保存 revisionInstruction
3. 下游节点保持 BLOCKED
4. 允许重新生成 target node
```

---

## 9.4 submit-edited

```text
1. 保存 editedArtifact
2. 标记 artifact source=user_edited
3. review node → SUCCESS
4. 写 decision_log
5. 下游继续
```

---

## 9.5 regenerate

```text
1. 标记 target node 需要重新执行
2. 带 revisionInstruction 重新生成 artifact
3. 重新进入 WAITING_REVIEW
4. 下游继续 blocked
```

---

# 10. P1：Preflight 能力检查

## 10.1 API

```http
GET /api/video/preflight
```

或者在：

```http
POST /api/agent/runs
```

前自动执行。

---

## 10.2 返回

```json
{
  "status": "passed",
  "canStart": true,
  "capabilityMenu": {
    "localRunner": {
      "available": true
    },
    "hyperframes": {
      "available": true
    },
    "ffmpeg": {
      "available": true
    },
    "localCommands": [
      {
        "command": "HYPERFRAMES_PROJECT_GENERATE",
        "available": true
      },
      {
        "command": "HYPERFRAMES_SNAPSHOT",
        "available": true
      },
      {
        "command": "HYPERFRAMES_RENDER",
        "available": true
      },
      {
        "command": "FFMPEG_PROBE",
        "available": true
      },
      {
        "command": "ARTIFACT_PACKAGE",
        "available": true
      }
    ]
  }
}
```

失败时必须阻断：

```json
{
  "status": "blocked",
  "canStart": false,
  "blockers": [
    {
      "code": "LOCAL_RUNNER_NOT_AVAILABLE",
      "message": "本地执行器未在线，无法生成视频。"
    }
  ]
}
```

---

# 11. P1：前端 Guided Video Studio 对齐

## 11.1 命名整改

改名：

```text
OneClickVideoPage.tsx → GuidedVideoStudioPage.tsx
activeNav: oneclick → guidedVideo
```

文案改为：

```text
一句话启动视频创作
分阶段确认后生成 MP4
```

禁止文案：

```text
一键出片
全自动生成视频
无需确认
```

---

## 11.2 页面必须包含

```text
1. 输入一句话
2. Preflight 状态
3. AgentPlan 阶段进度
4. 当前待审核 artifact
5. approve / reject / edit / regenerate
6. LocalJob 状态
7. final.mp4 预览
8. 打开文件夹
9. 导出 package
```

---

# 12. P1：决策日志 decision_log

任何用户审核和关键执行选择必须记录：

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

必须记录：

```text
1. proposal approval
2. script approval
3. composition approval
4. preview approval
5. final render approval
6. reject / regenerate / edit
```

---

# 13. 实施阶段

## Phase 1：编译与核心链路确认

```text
1. cloud-backend go test ./...
2. local-backend go test ./...
3. 确认 ToolManifest 含 executionPlane/localCommand/localRequirements/humanReview。
4. 确认 localrunner API 注册。
5. 确认 NodeExecutor 对 local 工具创建 LocalJob。
```

验收：

```text
所有后端能编译。
local 工具不会在 cloud 内直接执行。
```

---

## Phase 2：Planner 动态编排

```text
1. LLMPlanner 接入 HybridToolRetriever。
2. 第一版工具 allowlist。
3. compactToolManifests 补字段。
4. 更新 LLM system prompt。
```

验收：

```text
LLM 能看到完整工具上下文，并生成合理 AgentPlan。
```

---

## Phase 3：PlanGuard 与审核策略

```text
1. PlanGuard 接入 LocalCapabilityValidator。
2. PlanGuard 校验 approvalPolicy 可插入。
3. PlanGuard 校验 local runner 在线。
4. PlanGuard 校验 localCommand 可用。
```

验收：

```text
local runner 不在线时，计划阶段阻断。
需要审核的工具一定被插入 CONTROL 节点。
```

---

## Phase 4：补齐第一版工具

```text
1. video_composition_builder。
2. hyperframes_snapshot。
3. final_review_generator。
4. tool manifest 加 humanReview。
5. skillcap.yaml 注册新工具。
```

验收：

```text
proposal → script → composition → snapshot → render → final_review → package 可跑。
```

---

## Phase 5：Review API 与前端闭环

```text
1. submit-edited API。
2. regenerate API。
3. decision_log。
4. GuidedVideoStudioPage。
5. 前端与后端 API 对齐。
```

验收：

```text
用户能在每个审核节点 approve/edit/regenerate/reject。
```

---

## Phase 6：E2E 上线验收

输入：

```text
请帮我做一个 45 秒视频，讲 AI Agent 改变的是工作流。
```

必须发生：

```text
1. Preflight 通过。
2. LLM 生成 AgentPlan。
3. PlanCompiler 自动插入审核节点。
4. proposal 生成后暂停。
5. 用户确认 proposal 后生成 script。
6. 用户确认 script 后生成 composition。
7. 用户确认 composition 后生成 project + snapshot。
8. 用户确认 snapshot 后创建 HYPERFRAMES_RENDER LocalJob。
9. local-backend 渲染 final.mp4。
10. final_review 通过。
11. artifact_packager 输出 package。
12. 前端可预览和导出。
```

---

# 14. 第一版上线差距

当前距离第一版上线：

```text
约 65% - 75%
```

完成以下 6 项后可以进入第一版内测：

```text
1. LLMPlanner 接 HybridToolRetriever。
2. compactToolManifests 补完整工具上下文。
3. PlanGuard 接 LocalCapabilityValidator。
4. NodeExecutor 确保 local 工具走 LocalJob。
5. 补 video_composition_builder / hyperframes_snapshot / final_review_generator。
6. Review API 和前端审核闭环补齐。
```

---

# 15. Coding Agent 执行指令

```text
目标：
将当前视频 Agent 整改为 Dynamic Guided Video。大模型负责编排工具，系统根据 ToolManifest 自动插入审核节点，并通过 PlanGuard/Orchestrator/local-backend 确保审核、本地能力和执行安全。

P0：编译和 LocalJob 主链路
1. 执行 cloud-backend go test ./...。
2. 修复 ToolManifest 中缺失的 executionPlane、requiresUserDevice、artifactLocation、localCommand、localRequirements、humanReview 字段。
3. 确认 localrunner service 和 handler 在 main.go 中注册。
4. 修改 NodeExecutor：executionPlane=local 时必须创建 LocalJob，不得 cloud 内直接执行。
5. hyperframes_project_generator、hyperframes_renderer、ffmpeg_probe、artifact_packager 必须走 LocalJob。

P1：Planner
1. LLMPlanner 接入 HybridToolRetriever。
2. HeuristicPlanner 只保留 fallback。
3. 第一版只召回图文视频工具 allowlist。
4. compactToolManifests 补 executionPlane、localCommand、localRequirements、approvalPolicy、humanReview、qualityPolicy。
5. 更新 LLM system prompt，声明大模型只负责编排，不负责绕过审核。

P2：PlanGuard
1. Validate 增加 ctx 和 userID。
2. 接入 LocalCapabilityValidator。
3. 校验 runner online、localCommand、localRequirements。
4. 校验 required approval 可插入 CONTROL。
5. local runner 不在线时阻断计划。

P3：工具补齐
1. 新增 video_composition_builder.tool.yaml 和实现。
2. 新增 hyperframes_snapshot.tool.yaml 和实现。
3. 新增 final_review_generator.tool.yaml 和实现。
4. skillcap.yaml 注册这三个工具。
5. 所有第一版工具补 humanReview。

P4：Review 闭环
1. 新增 submit-edited API。
2. 新增 regenerate API。
3. approve/reject/edit/regenerate 写 decision_log。
4. reject 后下游保持 blocked。
5. regenerate 后重新进入 WAITING_REVIEW。

P5：前端
1. OneClickVideoPage 改名 GuidedVideoStudioPage。
2. activeNav oneclick 改为 guidedVideo。
3. 对齐 approve/reject/submit-edited/regenerate API。
4. 显示 pending review artifact。
5. preview 未确认前不允许 render。
6. final.mp4 支持预览和导出。

P6：E2E
1. 增加 dynamic_guided_video_e2e.md。
2. 跑通一句话 → AgentPlan → review gates → LocalJob → final.mp4 → package。
3. 确保 cloud-backend/local-backend/frontend build/test 通过。
```

---

# 16. 最终结论

当前最重要的不是继续加视频功能，而是把主链路打通：

```text
大模型自主编排
  ↓
ToolManifest 审核规则
  ↓
PlanGuard 强校验
  ↓
PlanCompiler 自动插审核
  ↓
Orchestrator 阻断等待确认
  ↓
LocalJob 本地执行
  ↓
final.mp4
```

一句话总结：

**下一步要把系统从“Guided Workflow 产品雏形”升级为“Dynamic Guided Video Agent”：大模型负责编排，系统负责审核和执行约束。**
