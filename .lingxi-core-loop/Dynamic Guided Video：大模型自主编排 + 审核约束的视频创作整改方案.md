# Dynamic Guided Video：大模型自主编排 + 审核约束的视频创作整改方案

## 1. 整改目标

当前目标不是“一句话直接生成最终视频”，而是：

```text
一句话启动视频创作
  ↓
系统把相关工具完整提供给大模型
  ↓
大模型自主编排 AgentPlan
  ↓
系统根据 ToolManifest 自动插入审核节点
  ↓
用户确认关键产物
  ↓
本地执行 HyperFrames 渲染
  ↓
生成 final.mp4
```

最终原则：

```text
大模型负责编排
ToolManifest 负责声明工具能力和审核规则
PlanGuard 负责阻止非法计划
PlanCompiler 负责插入审核节点和质量门禁
Orchestrator 负责状态机硬阻断
local-backend 负责本地工具执行
```

---

## 2. 当前问题总结

当前系统已经具备：

```text
1. ToolManifest 已有 approvalPolicy。
2. PlanCompiler 已能根据 approvalPolicy 自动插入 CONTROL 审核节点。
3. CONTROL 节点能阻断任务继续执行。
4. local-backend 已有 runner loop。
5. HyperFrames 本地 project/render/package 链路已经基本成型。
6. guided workflow 已经开始专注图文视频。
```

但仍存在问题：

```text
1. LLMPlanner 仍偏 HeuristicPlanner，不是真正基于完整工具上下文自主编排。
2. 给大模型的工具摘要缺 executionPlane、localCommand、localRequirements、qualityPolicy 等关键字段。
3. PlanGuard 有 LocalCapabilityValidator，但需要确保主链路真正调用。
4. 固定 workflow 仍然在主导流程，应该降级为 fallback / recipe。
5. 每个工具是否需要人工审核虽然有 approvalPolicy，但缺少面向前端和大模型的 humanReview 描述。
6. review API 和前端审核闭环需要产品化。
7. video_composition_builder、hyperframes_snapshot、final_review 仍需补齐第一版闭环。
```

---

## 3. 新主线：Dynamic Guided Video

### 3.1 主流程

```text
用户输入一句话
  ↓
Preflight：检查本地执行器和工具能力
  ↓
ToolRetriever：召回相关视频创作工具
  ↓
LLMPlanner：大模型自主编排 AgentPlan
  ↓
PlanGuard：校验工具、参数、本地能力、审核策略
  ↓
PlanCompiler：自动插入 Review Gate / Quality Gate
  ↓
Orchestrator：执行 DAG
  ↓
CONTROL 节点阻断等待用户审核
  ↓
用户确认后继续
  ↓
local-backend 执行 HyperFrames 本地渲染
  ↓
final.mp4 + package
```

### 3.2 固定 workflow 的定位调整

保留：

```text
wf-guided-image-text-video
```

但定位调整为：

```text
1. fallback workflow
2. baseline recipe
3. 大模型编排参考样例
4. PlanGuard 的结构校验参考
```

主入口改为：

```text
POST /api/agent/runs
```

即：

```text
message → Preflight → ToolRetriever → LLMPlanner → PlanGuard → PlanCompiler → DAG
```

如果 LLM 规划失败，再 fallback 到 `wf-guided-image-text-video`。

---

## 4. ToolManifest 整改

### 4.1 所有工具必须声明执行面

每个 tool manifest 必须包含：

```yaml
executionPlane: cloud | local | remote_http | hybrid
requiresUserDevice: true | false
artifactLocation: cloud | local | both
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
```

### 4.2 所有工具必须声明审核策略

当前已有 `approvalPolicy`，继续保留：

```yaml
approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: "脚本会影响后续视频结构和渲染，必须人工确认。"
  reviewArtifactKinds:
    - JSON
```

### 4.3 新增 humanReview 字段

为了避免 Agent 误解，需要新增面向大模型和前端展示的字段：

```yaml
humanReview:
  required: true
  gate: after_artifact
  title: "审核口播脚本"
  reviewFocus:
    - "开头是否有吸引力"
    - "表达是否自然"
    - "内容是否准确"
    - "时长是否合理"
  userActions:
    - approve
    - edit
    - regenerate
    - reject
```

映射关系：

```text
humanReview.required          → approvalPolicy.required
humanReview.gate              → approvalPolicy.mode
humanReview.blocksDownstream  → approvalPolicy.blocksDownstream
humanReview.reviewFocus       → 前端审核面板
humanReview.userActions       → 前端操作按钮
```

### 4.4 第一版必须补齐的工具

第一版图文视频至少需要：

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

第一版不暴露：

```text
Seedance
TTS
ASR
复杂分镜
keyframe_prompt_generator
video_prompt_generator
publish_platform_api
```

---

## 5. LLMPlanner 整改

### 5.1 接入 HybridToolRetriever

把当前启发式选择工具：

```go
retriever := &HeuristicPlanner{tools: p.tools, maxTools: p.maxTools}
selected := retriever.selectTools(domain, req.Message)
```

改为：

```go
retriever := NewHybridToolRetriever(p.tools.ListManifests())

selected, err := retriever.Retrieve(ctx, RetrieveRequest{
    Query: req.Message,
    Domain: domain,
    MaxCostLevel: req.MaxCostLevel,
    MaxRiskLevel: req.MaxRiskLevel,
    CoarseTopK: 30,
    PlannerTopK: 12,
})
```

推荐参数：

```text
CoarseTopK = 30
PlannerTopK = 12 - 16
```

不要把所有工具都给模型；给“相关工具全集”。这样既保留自主编排，又降低上下文噪音。

---

### 5.2 工具摘要必须补齐字段

`compactToolManifests` 必须增加：

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
  "approvalPolicy": {
    "required": true,
    "mode": "before_execute",
    "blocksDownstream": true,
    "reason": "最终渲染耗时较长且会产生大文件，必须用户确认后执行。"
  },
  "humanReview": {
    "required": true,
    "title": "确认最终渲染",
    "reviewFocus": [
      "预览是否已确认",
      "是否允许生成 final.mp4"
    ]
  },
  "artifactPolicy": {
    "produceArtifact": true,
    "artifactKinds": ["VIDEO"]
  },
  "qualityPolicy": {
    "required": true,
    "checks": ["ffmpeg_probe", "file_exists", "duration_valid"]
  },
  "nextRecommendedTools": [
    "ffmpeg_probe",
    "final_review_generator",
    "artifact_packager"
  ]
}
```

### 5.3 LLM 系统提示修改

必须加入硬约束：

```text
你负责生成 AgentPlan，不直接执行工具。

规则：
1. 你可以自主选择和排列候选工具。
2. 你必须尊重每个工具的 executionPlane。
3. 你必须尊重每个工具的 approvalPolicy 和 humanReview。
4. 你不需要手写审核节点，系统会根据 ToolManifest 自动插入。
5. 你不得规划绕过人工审核的步骤。
6. 如果工具 requiresUserDevice=true，你必须确保前置包含 capability_preflight 或本地能力检查。
7. hyperframes_renderer 只能在 preview 或 composition 已确认后执行。
8. 第一版只使用图文视频工具，不使用 Seedance/TTS/ASR/发布平台。
```

---

## 6. PlanGuard 整改

### 6.1 启用 LocalCapabilityValidator

当前已有本地能力校验组件，需要接入主链路。

`PlanGuard.Validate` 从：

```go
Validate(plan *AgentPlan) error
```

升级为：

```go
Validate(ctx context.Context, userID string, plan *AgentPlan) error
```

校验逻辑：

```go
for _, step := range plan.Steps {
    manifest := toolRegistry.GetManifest(step.Tool)

    if manifest.ExecutionPlane == tool.ExecutionPlaneLocal ||
       manifest.ExecutionPlane == tool.ExecutionPlaneHybrid {
        err := localCapabilityValidator.ValidateForLocalExecution(ctx, userID, step, manifest)
        if err != nil {
            return err
        }
    }
}
```

### 6.2 必须校验的内容

```text
1. local runner 是否在线
2. runner 是否支持 localCommand
3. runner 是否满足 localRequirements.commands
4. runner 是否满足 minDiskMb
5. requiresUserDevice=true 时必须有 deviceId
6. artifactLocation=local 时必须能保存 local artifact
```

### 6.3 审核规则校验

PlanGuard 还必须校验：

```text
1. 如果 tool.approvalPolicy.required=true，则 PlanCompiler 必须能插入审核节点。
2. 如果 tool.approvalPolicy.blocksDownstream=true，则下游不得绕过审核。
3. 如果 tool.humanReview.required=true，则前端必须能展示对应 artifact。
4. 如果 hyperframes_renderer 出现在计划中，则必须存在 preview/composition 相关前置产物。
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

## 7. PlanCompiler 整改

### 7.1 继续由系统自动插入审核节点

不要让 LLM 自己写 review gate。

正确方式：

```text
LLM 输出：
proposal → script → composition → project → render → review → package

PlanCompiler 编译为：
proposal_exec
proposal_review_control
script_exec
script_review_control
composition_exec
composition_review_control
project_exec
project_review_control / preview_control
render_review_before_control
render_exec
final_review_exec
package_exec
```

### 7.2 approvalPolicy 编译规则

```text
mode=before_execute:
  review_before → exec

mode=after_artifact:
  exec → review_after

mode=before_downstream:
  exec → review_after → downstream

mode=always:
  review_before → exec → review_after
```

### 7.3 CONTROL 节点结构

```json
{
  "id": "review_script",
  "type": "CONTROL",
  "controlType": "HUMAN_REVIEW",
  "targetNodeId": "script_exec",
  "blocking": true,
  "reviewTitle": "审核口播脚本",
  "reviewFocus": [
    "开头是否有吸引力",
    "表达是否自然",
    "内容是否准确"
  ],
  "allowedActions": [
    "approve",
    "edit",
    "regenerate",
    "reject"
  ]
}
```

---

## 8. Orchestrator 整改

### 8.1 CONTROL 节点必须硬阻断

状态流转：

```text
TOOL_SUCCESS
  ↓
CONTROL_READY
  ↓
WAITING_REVIEW
  ↓
用户 approve
  ↓
CONTROL_SUCCESS
  ↓
下游 READY
```

禁止：

```text
1. CONTROL 自动 success。
2. 下游节点绕过 CONTROL。
3. preview 未确认就 render。
4. render 未成功就 package。
```

### 8.2 增加审核状态

```text
WAITING_REVIEW
REVIEW_APPROVED
REVIEW_REJECTED
REVISION_REQUIRED
```

### 8.3 reject / edit / regenerate 流程

```text
reject:
  当前 review 节点 → REVISION_REQUIRED
  下游节点保持 BLOCKED
  上游目标节点根据 revisionInstruction 重新执行

edit:
  用户编辑 artifact
  保存 edited artifact
  review 节点 → REVIEW_APPROVED
  下游继续

regenerate:
  重新执行 target node
  生成新 artifact
  再次进入 WAITING_REVIEW
```

---

## 9. Review API 整改

必须补齐：

```http
GET /api/tasks/{taskId}/reviews/pending
POST /api/tasks/{taskId}/reviews/{reviewId}/approve
POST /api/tasks/{taskId}/reviews/{reviewId}/reject
POST /api/tasks/{taskId}/reviews/{reviewId}/submit-edited
POST /api/tasks/{taskId}/reviews/{reviewId}/regenerate
```

### 9.1 查询待审核

```json
{
  "taskId": "task_001",
  "review": {
    "reviewId": "review_script_001",
    "stage": "script",
    "targetNodeId": "script_exec",
    "artifactKind": "video_script",
    "artifact": {},
    "reviewTitle": "审核口播脚本",
    "reviewFocus": [
      "开头是否有吸引力",
      "表达是否自然",
      "内容是否准确"
    ],
    "allowedActions": [
      "approve",
      "edit",
      "regenerate",
      "reject"
    ]
  }
}
```

### 9.2 通过

```json
{
  "action": "approve",
  "comment": "脚本确认，继续生成视频结构。"
}
```

### 9.3 驳回

```json
{
  "action": "reject",
  "reason": "开头不够有冲突感。",
  "revisionInstruction": "开头更短，第一句话直接指出很多人误解了 AI Agent。"
}
```

### 9.4 编辑后提交

```json
{
  "action": "submit_edited",
  "editedArtifact": {}
}
```

---

## 10. Preflight 整改

新增：

```http
GET /api/video/preflight
```

或在 `POST /api/agent/runs` 前自动执行 preflight。

返回：

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

失败时：

```json
{
  "status": "blocked",
  "canStart": false,
  "blockers": [
    {
      "code": "HYPERFRAMES_NOT_AVAILABLE",
      "message": "HyperFrames Render Service 未启动，无法生成视频预览和最终 MP4。"
    }
  ],
  "setupActions": [
    {
      "type": "start_local_service",
      "label": "启动本地渲染服务"
    }
  ]
}
```

---

## 11. 第一版视频工具集合

### 11.1 允许给大模型的工具

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

### 11.2 禁止第一版暴露给大模型的工具

```text
seedance_clip_generator
video_prompt_generator
keyframe_prompt_generator
tts_generator
asr_transcriber
platform_publish_api
complex_video_editor
```

后续可逐步开放，但第一版先限制范围。

---

## 12. 必须新增或补齐的工具

### 12.1 video_composition_builder

执行面：

```yaml
executionPlane: cloud
requiresUserDevice: false
artifactLocation: cloud
```

职责：

```text
把 video_script 转换为 VideoCompositionSpec。
```

输出：

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

审核：

```yaml
humanReview:
  required: true
  gate: after_artifact
  title: "审核视频结构"
  reviewFocus:
    - "卡片顺序是否合理"
    - "每页文字是否过长"
    - "每段时长是否合理"
```

---

### 12.2 hyperframes_snapshot

执行面：

```yaml
executionPlane: local
requiresUserDevice: true
artifactLocation: local
localCommand: HYPERFRAMES_SNAPSHOT
```

职责：

```text
根据 HyperFrames 项目生成 3-5 张预览图，不进行最终 MP4 渲染。
```

输出：

```json
{
  "artifactKind": "preview_snapshots",
  "snapshots": [
    "local://projects/task_001/previews/snapshot_001.png",
    "local://projects/task_001/previews/snapshot_002.png"
  ]
}
```

审核：

```yaml
humanReview:
  required: true
  gate: after_artifact
  title: "审核画面预览"
  reviewFocus:
    - "文字是否溢出"
    - "画面是否可读"
    - "卡片顺序是否正确"
```

---

### 12.3 final_review_generator

职责：

```text
渲染后检查 final.mp4 是否可用。
```

第一版可调用：

```text
ffmpeg_probe
file_size_checker
artifact_completeness_checker
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
  }
}
```

---

## 13. 前端整改

页面名称：

```text
Guided Video Studio
```

副标题：

```text
一句话启动，分阶段确认，最后生成 MP4
```

### 13.1 页面模块

```text
1. 输入区
2. 本地能力状态区
3. 当前阶段区
4. Artifact 预览区
5. 审核操作区
6. 任务进度区
7. 最终视频预览区
8. 导出区
```

### 13.2 审核面板

每个审核节点必须显示：

```text
1. 阶段名称
2. 产物内容
3. 审核重点
4. 确认继续
5. 编辑后确认
6. 重新生成
7. 驳回并填写修改意见
```

### 13.3 禁止文案

不要出现：

```text
一键出片
全自动生成最终视频
无需确认
自动完成全部流程
```

推荐文案：

```text
一句话启动视频创作
分阶段确认后生成 MP4
确认预览后开始最终渲染
```

---

## 14. 上线第一版最小链路

第一版 E2E：

```text
用户输入一句话
  ↓
Preflight passed
  ↓
LLMPlanner 生成 AgentPlan
  ↓
PlanGuard 校验通过
  ↓
PlanCompiler 自动插审核节点
  ↓
proposal_generator
  ↓
review proposal
  ↓
video_script_generator
  ↓
review script
  ↓
video_composition_builder
  ↓
review composition
  ↓
hyperframes_project_generator
  ↓
hyperframes_snapshot
  ↓
review preview
  ↓
hyperframes_renderer
  ↓
ffmpeg_probe / final_review_generator
  ↓
artifact_packager
  ↓
final.mp4 可预览
```

---

## 15. 实施阶段

### Phase 1：Planner 工具上下文整改

```text
1. LLMPlanner 接入 HybridToolRetriever。
2. compactToolManifests 增加 executionPlane / localCommand / localRequirements / humanReview / qualityPolicy。
3. 限制第一版可用工具集合。
4. 更新 LLM system prompt。
```

验收：

```text
LLM 能基于候选工具生成合理 AgentPlan，并理解哪些工具需要审核。
```

---

### Phase 2：ToolManifest 审核字段整改

```text
1. 所有第一版工具补 humanReview。
2. 所有 local 工具补 localRequirements。
3. 所有最终产物工具补 artifactPolicy。
4. 所有质量检查工具补 qualityPolicy。
```

验收：

```text
PlanCompiler 能基于 approvalPolicy 自动插入审核节点。
前端能基于 humanReview 展示审核面板。
```

---

### Phase 3：PlanGuard 整改

```text
1. PlanGuard 接入 LocalCapabilityValidator。
2. 校验 local runner 在线。
3. 校验 localCommand 支持。
4. 校验 requiresUserDevice。
5. 校验审核节点可插入。
```

验收：

```text
local runner 不在线时，计划阶段阻断，不进入 WAITING_LOCAL 卡死。
```

---

### Phase 4：Review API / 状态机闭环

```text
1. 补 pending reviews API。
2. 补 approve / reject / submit-edited / regenerate。
3. CONTROL 节点进入 WAITING_REVIEW 后必须等待用户操作。
4. reject 后支持 revisionInstruction。
```

验收：

```text
用户确认前，下游节点不执行。
用户确认后，下游节点继续。
```

---

### Phase 5：补第一版视频工具

```text
1. 新增 video_composition_builder。
2. 新增 hyperframes_snapshot。
3. 新增 final_review_generator。
4. 确认 artifact_packager 输出 package。
```

验收：

```text
proposal → script → composition → preview → render → review → package 全链路可跑。
```

---

### Phase 6：前端 Guided Video Studio

```text
1. 增加 Guided Video Studio 页面。
2. 增加本地能力状态。
3. 增加阶段式 artifact 审核面板。
4. 增加最终视频预览。
5. 增加打开文件夹 / 导出 package。
```

验收：

```text
用户不用看日志，即可完成从一句话到 final.mp4 的全流程。
```

---

## 16. Coding Agent 执行指令

```text
目标：
将当前视频 Agent 整改为 Dynamic Guided Video：大模型自主编排工具，但审核、质量、本地执行由系统强约束。用户一句话只启动项目，不允许跳过人工审核直接生成 final.mp4。

P0：Planner 整改
1. LLMPlanner 接入 HybridToolRetriever，替换 HeuristicPlanner。
2. ToolRetriever 返回第一版相关工具集合。
3. compactToolManifests 增加：
   - executionPlane
   - requiresUserDevice
   - artifactLocation
   - localCommand
   - localRequirements
   - approvalPolicy
   - humanReview
   - artifactPolicy
   - qualityPolicy
   - providerCapabilities
4. 更新 LLM system prompt：大模型负责编排，不负责绕过审核。

P1：ToolManifest 整改
1. 所有第一版工具补 humanReview 字段。
2. proposal_generator、video_script_generator、video_composition_builder、hyperframes_snapshot 必须 humanReview.required=true。
3. hyperframes_renderer 必须 approvalPolicy.mode=before_execute。
4. hyperframes_renderer 必须 humanReview.required=true。
5. local 工具必须补 localRequirements。

P2：PlanGuard 整改
1. PlanGuard Validate 增加 ctx 和 userID。
2. 接入 LocalCapabilityValidator。
3. 校验 local runner 在线。
4. 校验 localCommand 支持。
5. 校验 localRequirements。
6. 校验 required approval 能插入审核节点。

P3：PlanCompiler / Orchestrator 整改
1. 继续由 PlanCompiler 根据 approvalPolicy 自动插入 CONTROL 节点。
2. CONTROL 节点必须 blocking=true。
3. CONTROL READY 后进入 WAITING_REVIEW。
4. 下游节点必须等待 CONTROL SUCCESS。
5. reject/edit/regenerate 要能重新生成上游 artifact。

P4：Review API
1. 新增 GET pending reviews。
2. 新增 POST approve。
3. 新增 POST reject。
4. 新增 POST submit-edited。
5. 新增 POST regenerate。
6. 审核操作写 decision_log。

P5：视频工具补齐
1. 新增 video_composition_builder.tool.yaml 和实现。
2. 新增 hyperframes_snapshot.tool.yaml 和实现。
3. 新增 final_review_generator.tool.yaml 和实现。
4. 确保 artifact_packager 可打包 final.mp4、spec、manifest、review。

P6：前端
1. 新增 Guided Video Studio。
2. 文案改为“一句话启动视频创作”。
3. 展示 pending review。
4. 支持 approve/edit/regenerate/reject。
5. preview 未确认前不显示开始最终渲染。
6. final.mp4 生成后支持预览和导出。

验收：
1. 大模型能看到相关工具完整信息。
2. 大模型能生成 AgentPlan。
3. 系统自动插入审核节点。
4. 用户确认前，下游不执行。
5. local runner 离线时计划阶段阻断。
6. preview 未确认时，不创建 HYPERFRAMES_RENDER LocalJob。
7. 最终能生成 final.mp4 并打包。
```

---

## 17. 第一版上线差距

当前距离第一版上线约：

```text
80% 左右
```

还缺最关键的 6 件事：

```text
1. LLMPlanner 接入 HybridToolRetriever。
2. 工具摘要补齐执行面、审核、本地能力信息。
3. PlanGuard 启用 LocalCapabilityValidator。
4. 补 video_composition_builder。
5. 补 review API + 前端审核面板。
6. 补 hyperframes_snapshot / preview 审核。
```

完成后，第一版可以达到：

```text
一句话启动
大模型编排
分阶段审核
本地渲染
final.mp4 预览导出
```

---

## 18. 最终结论

你要的目标不是固定流程，也不是全自动出片，而是：

```text
Dynamic Guided Video
```

即：

```text
大模型自主编排工具
系统自动加审核
用户确认关键产物
本地安全执行渲染
```

最终原则：

```text
工具是否需要审核，由 ToolManifest 决定。
审核节点是否存在，由 PlanCompiler 保证。
审核是否阻断，由 Orchestrator 状态机保证。
本地工具是否可执行，由 PlanGuard 保证。
任务怎么排列，由大模型负责。
```

一句话总结：

**让大模型负责编排，但不要让大模型决定是否绕过审核；审核、质量、本地执行能力必须是系统级硬约束。**
