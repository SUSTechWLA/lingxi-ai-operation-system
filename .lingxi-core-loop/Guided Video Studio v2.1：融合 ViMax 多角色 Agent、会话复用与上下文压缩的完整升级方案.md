# Guided Video Studio v2.1：融合 ViMax 多角色 Agent、会话复用与上下文压缩的完整升级方案

## 1. 方案定位

Guided Video Studio v2.1 的目标不是“一个大模型从头到尾直接生成视频”，而是构建一个**多专业角色协作的视频创作 Agent 系统**。

核心定位：

```text
一句话启动视频项目
  ↓
CreativeDirectorAgent 制定创作方向
  ↓
ScriptWriterAgent 写脚本
  ↓
CompositionDirectorAgent 设计图文视频结构
  ↓
ContinuityKeeperAgent 管理风格与一致性
  ↓
PreviewDirectorAgent 生成并审查预览
  ↓
RenderProducerAgent 管理本地渲染
  ↓
QualityReviewerAgent 做质量检查
  ↓
PackageProducerAgent 打包交付
```

系统最终不是一个“全自动出片按钮”，而是一个：

```text
可分工
可审核
可恢复
可修改
可压缩上下文
可追踪产物
可本地执行
可最终交付的视频 Agent 工作台
```

---

## 2. 核心升级原则

### 2.1 多角色 Agent 是上线必选能力

上一版中，多角色 StageDirector 被放在 P1，本版调整为 **P0 必须实现**。

原因：

```text
1. 视频创作链路天然不是单一步骤。
2. 不同阶段需要不同判断标准。
3. 一个 Agent 同时负责创意、脚本、结构、预览、渲染、质检，容易跳步。
4. 多角色分工可以天然形成审核边界。
5. 角色化 Agent 更适合后续扩展动态视频、角色一致性、参考图选择、镜头规划。
```

第一版必须有全部角色，但实现形式可以轻量化。

---

## 3. 第一版角色实现方式

不要一开始把每个角色做成独立进程或独立服务。第一版推荐：

```text
Role Agent = 角色定义 + 工具白名单 + 输入产物 + 输出产物 + 审核规则 + Prompt 模板
```

也就是：

```text
不是 8 个独立微服务
而是 8 个角色化 StageDirector
```

后续再升级为：

```text
独立 Agent Runtime
独立上下文
独立工具权限
独立质量评分
独立多候选生成
```

---

# 4. 多角色 Agent 总表

| 角色                       | 中文名      | 主要职责                                      | 必须输出                               | 是否审核  |
| ------------------------ | -------- | ----------------------------------------- | ---------------------------------- | ----- |
| CreativeDirectorAgent    | 创意总监     | 理解需求、确定主题、风格、平台、时长、创作方向                   | VIDEO_PROPOSAL                     | 是     |
| ScriptWriterAgent        | 脚本编剧     | 生成口播脚本、段落结构、标题、钩子、总结                      | VIDEO_SCRIPT                       | 是     |
| StoryboardArtistAgent    | 分镜/卡片设计师 | 把脚本拆成画面页、卡片、字幕、节奏                         | STORYBOARD_PLAN / CARD_PLAN        | 是     |
| CompositionDirectorAgent | 图文视频结构导演 | 生成 VideoCompositionSpec、轨道、时间轴、布局         | VIDEO_COMPOSITION_SPEC             | 是     |
| ReferenceSelectorAgent   | 参考资产选择器  | 选择风格参考、素材、图标、背景策略                         | REFERENCE_ASSET_PLAN               | 可选审核  |
| ContinuityKeeperAgent    | 连续性管理    | 管理风格一致性、术语一致性、画面一致性、产物失效                  | CONTINUITY_REPORT / STYLE_PROFILE  | 否/可选  |
| PreviewDirectorAgent     | 预览导演     | 生成 HyperFrames 项目和预览图，检查可读性               | PREVIEW_SNAPSHOTS / PREVIEW_REPORT | 是     |
| RenderProducerAgent      | 渲染制片     | 校验依赖，创建本地渲染任务，管理 LocalJob                 | RENDER_REPORT / VIDEO              | 渲染前审核 |
| QualityReviewerAgent     | 质量审核员    | 检查视频、时长、文件、字幕、产物完整性                       | FINAL_REVIEW                       | 否     |
| PackageProducerAgent     | 交付制片     | 打包 final.mp4、spec、manifest、review、package | PROJECT_PACKAGE                    | 可选审核  |

注意：

```text
StoryboardArtistAgent 是从 ViMax 思想中补入的新角色。
即使第一版是图文视频，也需要一个“卡片/分镜设计师”角色。
它不一定生成传统视频分镜，而是生成图文视频的卡片页、字幕节奏、画面段落。
```

---

# 5. v2.1 总体链路

```text
用户输入一句话
  ↓
Preflight
  ↓
AgentRunSession 创建 / 恢复
  ↓
CreativeDirectorAgent
  ↓
proposal review
  ↓
ScriptWriterAgent
  ↓
script review
  ↓
StoryboardArtistAgent
  ↓
storyboard/card plan review
  ↓
CompositionDirectorAgent
  ↓
composition review
  ↓
ReferenceSelectorAgent
  ↓
ContinuityKeeperAgent
  ↓
PreviewDirectorAgent
  ↓
preview review
  ↓
RenderDependencyGuard
  ↓
RenderProducerAgent
  ↓
render approval
  ↓
local HYPERFRAMES_RENDER
  ↓
QualityReviewerAgent
  ↓
PackageProducerAgent
  ↓
final.mp4 / package
```

---

# 6. 角色 Agent 的标准结构

每个角色必须有标准定义文件：

```text
skill-capabilities/video/guided-video-studio/2.1.0/agents/
├── creative_director.agent.yaml
├── script_writer.agent.yaml
├── storyboard_artist.agent.yaml
├── composition_director.agent.yaml
├── reference_selector.agent.yaml
├── continuity_keeper.agent.yaml
├── preview_director.agent.yaml
├── render_producer.agent.yaml
├── quality_reviewer.agent.yaml
└── package_producer.agent.yaml
```

标准结构：

```yaml
id: script_writer
name: ScriptWriterAgent
displayName: 脚本编剧
stage: script

goal: 基于已确认的创作方案，生成适合中文平台图文视频的口播脚本。

requiredInputs:
  - VIDEO_PROPOSAL

requiredOutputs:
  - VIDEO_SCRIPT

allowedTools:
  - video_script_generator
  - script_quality_checker

forbiddenTools:
  - video_composition_builder
  - hyperframes_project_generator
  - hyperframes_snapshot
  - hyperframes_renderer
  - artifact_packager

humanReview:
  required: true
  title: 审核口播脚本
  reviewFocus:
    - 开头是否有吸引力
    - 观点是否清晰
    - 表达是否自然
    - 时长是否合理
  userActions:
    - approve
    - edit
    - regenerate
    - reject

qualityPolicy:
  required: true
  checks:
    - script_length
    - section_structure
    - language_consistency
```

---

# 7. 每个角色的详细设计

## 7.1 CreativeDirectorAgent：创意总监

### 职责

```text
1. 理解用户一句话需求。
2. 判断视频类型。
3. 明确目标平台、时长、画幅、风格。
4. 生成创作方案。
5. 判断是否适合第一版图文视频能力。
```

### 输入

```text
USER_REQUEST
PREVIOUS_PROJECT_MEMORY 可选
STYLE_PROFILE 可选
```

### 输出

```text
VIDEO_PROPOSAL
PROJECT_BRIEF
STYLE_PROFILE 初版
```

### 工具白名单

```text
proposal_generator
capability_preflight
```

### 工具黑名单

```text
video_script_generator
hyperframes_renderer
artifact_packager
```

### 审核要求

必须审核。

审核重点：

```text
1. 主题是否准确
2. 目标用户是否明确
3. 视频形式是否适合图文视频
4. 时长是否合理
5. 是否继续进入脚本阶段
```

---

## 7.2 ScriptWriterAgent：脚本编剧

### 职责

```text
1. 基于已确认 proposal 生成口播脚本。
2. 拆分开头、观点、总结。
3. 控制时长。
4. 保持中文平台表达习惯。
```

### 输出

```text
VIDEO_SCRIPT
SCRIPT_SECTIONS
```

### 工具白名单

```text
video_script_generator
script_quality_checker
```

### 审核要求

必须审核。

审核重点：

```text
1. 开头是否有钩子
2. 表达是否自然
3. 是否有废话
4. 观点是否准确
5. 是否适合后续拆成卡片
```

---

## 7.3 StoryboardArtistAgent：分镜/卡片设计师

### 为什么必须增加这个角色

原方案里从脚本直接到 `VideoCompositionSpec`，中间缺少“画面设计”层。

这样会导致：

```text
脚本是文字逻辑
composition 是工程时间轴
中间缺少创作型画面规划
```

StoryboardArtistAgent 负责把脚本转为：

```text
每一页卡片讲什么
画面上显示什么
字幕如何拆
节奏如何安排
哪些内容要强调
```

### 输出

```text
STORYBOARD_PLAN
CARD_PLAN
CAPTION_PLAN
```

示例：

```json
{
  "artifactKind": "CARD_PLAN",
  "cards": [
    {
      "cardId": "card_001",
      "type": "hook",
      "durationSec": 6,
      "title": "你可能误解了 AI Agent",
      "body": "它改变的不是某个岗位，而是整套工作流。",
      "visualIntent": "强对比开场，突出误解与真相",
      "emphasisWords": ["误解", "工作流"]
    }
  ]
}
```

### 工具白名单

```text
storyboard_planner
card_plan_generator
caption_splitter
```

如果这些工具暂时没有，可以第一版由 `video_composition_builder` 内部兼容生成，但概念上必须保留这个角色。

### 审核要求

必须审核或并入 composition 审核。

建议第一版：

```text
StoryboardArtistAgent 输出 CARD_PLAN
CARD_PLAN 与 VIDEO_COMPOSITION_SPEC 一起审核
```

---

## 7.4 CompositionDirectorAgent：图文视频结构导演

### 职责

```text
1. 把 CARD_PLAN 转成 VideoCompositionSpec。
2. 生成 overlay track、caption track、background track。
3. 控制时间轴。
4. 保证卡片和字幕不超长。
5. 保证 16:9 安全区域。
```

### 输入

```text
VIDEO_SCRIPT
CARD_PLAN
STYLE_PROFILE
```

### 输出

```text
VIDEO_COMPOSITION_SPEC
```

### 工具白名单

```text
video_composition_builder
composition_quality_checker
```

### 审核要求

必须审核。

审核重点：

```text
1. 卡片顺序是否合理
2. 每页文字是否过长
3. 时间轴是否完整
4. 字幕是否覆盖口播
5. 是否适合 HyperFrames 渲染
```

---

## 7.5 ReferenceSelectorAgent：参考资产选择器

### 职责

```text
1. 选择图文视频风格参考。
2. 选择背景策略、图标策略、字体策略。
3. 后续支持参考图、角色图、场景图。
```

第一版不一定要联网找图，可以先输出：

```text
REFERENCE_ASSET_PLAN
```

示例：

```json
{
  "artifactKind": "REFERENCE_ASSET_PLAN",
  "visualReferences": [
    {
      "type": "background",
      "strategy": "soft_gradient"
    },
    {
      "type": "icon",
      "strategy": "minimal_line_icon"
    },
    {
      "type": "font",
      "strategy": "system_sans"
    }
  ]
}
```

### 工具白名单

```text
reference_asset_planner
asset_policy_generator
```

如果第一版没有真实素材库，可以先做成确定性策略生成器。

### 审核要求

第一版可选审核。

---

## 7.6 ContinuityKeeperAgent：连续性/一致性管理

### 职责

```text
1. 维护 StyleProfile。
2. 维护 ContinuityProfile。
3. 检查标题、术语、画幅、字体、背景一致性。
4. 当用户修改上游产物时，触发 StaleTracker。
5. 给 preview review 提供 consistency warnings。
```

### 输入

```text
VIDEO_PROPOSAL
VIDEO_SCRIPT
CARD_PLAN
VIDEO_COMPOSITION_SPEC
STYLE_PROFILE
ARTIFACT_INDEX
```

### 输出

```text
CONTINUITY_REPORT
STYLE_PROFILE
STALE_ARTIFACT_REPORT
```

### 工具白名单

```text
continuity_checker
style_profile_builder
stale_tracker
```

### 审核要求

默认不需要人工审核，但它的 warning 应展示给用户。

---

## 7.7 PreviewDirectorAgent：预览导演

### 职责

```text
1. 调用 hyperframes_project_generator。
2. 调用 hyperframes_snapshot。
3. 生成 preview report。
4. 检查文字溢出、卡片顺序、画面可读性。
5. 把预览图交给用户确认。
```

### 输出

```text
HYPERFRAMES_PROJECT
PREVIEW_SNAPSHOTS
PREVIEW_REPORT
```

### 工具白名单

```text
hyperframes_project_generator
hyperframes_snapshot
preview_quality_checker
```

### 工具黑名单

```text
hyperframes_renderer
artifact_packager
```

### 审核要求

必须审核。

审核重点：

```text
1. 画面是否可读
2. 字幕是否溢出
3. 卡片顺序是否正确
4. 风格是否一致
5. 是否允许最终渲染
```

---

## 7.8 RenderProducerAgent：渲染制片

### 职责

```text
1. 检查 RenderDependencyGuard。
2. 确认 preview_review 已通过。
3. 创建 HYPERFRAMES_RENDER LocalJob。
4. 跟踪本地渲染进度。
5. 生成 render_report。
```

### 输出

```text
VIDEO
RENDER_REPORT
```

### 工具白名单

```text
hyperframes_renderer
local_job_status_tracker
```

### 渲染前必须审核

RenderProducerAgent 的关键不是让用户看脚本，而是确认：

```text
现在是否允许开始最终渲染
```

这是 before_execute 审核。

---

## 7.9 QualityReviewerAgent：质量审核员

### 职责

```text
1. 调用 ffmpeg_probe。
2. 检查 final.mp4 是否存在。
3. 检查文件大小、时长、视频流。
4. 检查产物完整性。
5. 输出 final_review。
```

### 输出

```text
FFMPEG_PROBE_REPORT
FINAL_REVIEW
```

### 工具白名单

```text
ffmpeg_probe
final_review_generator
```

### 审核要求

默认不需要用户审核，但如果失败，必须阻断 package。

---

## 7.10 PackageProducerAgent：交付制片

### 职责

```text
1. 打包 final.mp4。
2. 打包 VideoCompositionSpec。
3. 打包 HyperFrames 项目。
4. 打包 decision_log、final_review。
5. 输出 project_package.zip。
```

### 输出

```text
PROJECT_PACKAGE
```

### 工具白名单

```text
artifact_packager
package_quality_checker
```

### 审核要求

第一版可选审核。

---

# 8. Role Agent 调度机制

## 8.1 RoleAgentRegistry

新增：

```go
type RoleAgent struct {
    ID              string
    Name            string
    Stage           string
    Goal            string
    RequiredInputs  []string
    RequiredOutputs []string
    AllowedTools    []string
    ForbiddenTools  []string
    HumanReview     HumanReviewPolicy
    QualityPolicy   QualityPolicy
}
```

Registry：

```go
type RoleAgentRegistry interface {
    GetByStage(stage string) (*RoleAgent, error)
    List() []RoleAgent
}
```

---

## 8.2 RoleAgent 不是替代 ToolManifest

两者分工：

```text
RoleAgent：
  管阶段，规定这个阶段能做什么。

ToolManifest：
  管工具，规定这个工具怎么执行、是否审核、产物是什么。

PlanGuard：
  检查 RoleAgent 和 ToolManifest 是否冲突。
```

---

## 8.3 StageGuard

新增 StageGuard：

```text
检查当前 stage 中：
1. 是否使用了 forbiddenTools
2. 是否缺少 requiredInputs
3. 是否缺少 requiredOutputs
4. 是否越权调用后续阶段工具
5. 是否跳过 humanReview
```

示例：

```text
ScriptWriterAgent 禁止调用 hyperframes_renderer
PreviewDirectorAgent 禁止调用 artifact_packager
RenderProducerAgent 必须等待 preview_review.approved
```

---

# 9. Role Agent 与 ArtifactIndex 的关系

每个角色输出必须进入 ArtifactIndex：

```text
CreativeDirectorAgent       → VIDEO_PROPOSAL
ScriptWriterAgent           → VIDEO_SCRIPT
StoryboardArtistAgent       → CARD_PLAN
CompositionDirectorAgent    → VIDEO_COMPOSITION_SPEC
ReferenceSelectorAgent      → REFERENCE_ASSET_PLAN
ContinuityKeeperAgent       → CONTINUITY_REPORT
PreviewDirectorAgent        → PREVIEW_SNAPSHOTS
RenderProducerAgent         → VIDEO / RENDER_REPORT
QualityReviewerAgent        → FINAL_REVIEW
PackageProducerAgent        → PROJECT_PACKAGE
```

ArtifactIndex 记录：

```text
谁生成的
哪个版本
是否已审核
依赖谁
当前是否 stale
```

---

# 10. Role Agent 与上下文压缩的关系

上下文压缩不再只压缩“项目状态”，还要压缩每个角色的阶段记忆。

## 10.1 Role Memory

新增：

```go
type RoleMemory struct {
    RoleID        string   `json:"roleId"`
    Stage         string   `json:"stage"`
    Summary       string   `json:"summary"`
    KeyDecisions  []string `json:"keyDecisions"`
    Constraints   []string `json:"constraints"`
    ArtifactRefs  []string `json:"artifactRefs"`
    OpenIssues    []string `json:"openIssues"`
}
```

示例：

```json
{
  "roleId": "script_writer",
  "stage": "script",
  "summary": "已生成第三版口播脚本，用户确认开头直接强调AI Agent改变的是工作流。",
  "keyDecisions": [
    "保留中文平台表达",
    "开头不要学术化",
    "脚本时长控制45秒"
  ],
  "artifactRefs": [
    "VIDEO_SCRIPT:v3"
  ]
}
```

---

## 10.2 CompactedContext 增加 roleMemories

```go
type CompactedContext struct {
    ProjectGoal        string       `json:"projectGoal"`
    CurrentStage       string       `json:"currentStage"`
    CurrentStatus      string       `json:"currentStatus"`
    HardConstraints    []string     `json:"hardConstraints"`
    KeyDecisions       []string     `json:"keyDecisions"`
    ConfirmedArtifacts []ArtifactPointer `json:"confirmedArtifacts"`
    StaleArtifacts     []ArtifactPointer `json:"staleArtifacts"`
    RoleMemories       []RoleMemory `json:"roleMemories"`
    NextActions        []string     `json:"nextActions"`
}
```

---

# 11. 角色化工作流

## 11.1 第一版 v2.1 推荐流程

```text
CreativeDirectorAgent
  ↓
proposal review
  ↓
ScriptWriterAgent
  ↓
script review
  ↓
StoryboardArtistAgent
  ↓
card/storyboard review
  ↓
CompositionDirectorAgent
  ↓
composition review
  ↓
ReferenceSelectorAgent
  ↓
ContinuityKeeperAgent
  ↓
PreviewDirectorAgent
  ↓
preview review
  ↓
RenderProducerAgent
  ↓
render approval
  ↓
QualityReviewerAgent
  ↓
PackageProducerAgent
```

---

## 11.2 如果不想增加太多审核节点

第一版可以合并审核：

```text
proposal review
script review
composition review = storyboard + composition 一起审核
preview review
render approval
```

但内部角色仍必须存在：

```text
StoryboardArtistAgent 可以输出 CARD_PLAN
CompositionDirectorAgent 基于 CARD_PLAN 输出 VIDEO_COMPOSITION_SPEC
```

---

# 12. 多角色 Agent 的 Prompt 模板

## 12.1 通用模板

```text
你是 {{RoleName}}。

你的职责：
{{Goal}}

你当前所在阶段：
{{Stage}}

你必须读取的输入：
{{RequiredInputs}}

你必须生成的输出：
{{RequiredOutputs}}

你允许使用的工具：
{{AllowedTools}}

你禁止使用的工具：
{{ForbiddenTools}}

你必须遵守的硬规则：
1. 不得跳过人工审核。
2. 不得调用 forbiddenTools。
3. 不得生成本阶段以外的最终产物。
4. 不得把未确认的上游产物当作已确认。
5. 输出必须写入 ArtifactIndex。
```

---

## 12.2 ScriptWriterAgent Prompt 示例

```text
你是 ScriptWriterAgent，负责生成中文图文视频口播脚本。

输入：
- 已确认 VIDEO_PROPOSAL
- StyleProfile
- ContinuityProfile

你只能使用：
- video_script_generator
- script_quality_checker

你禁止使用：
- video_composition_builder
- hyperframes_project_generator
- hyperframes_renderer
- artifact_packager

输出：
- VIDEO_SCRIPT

规则：
1. 不要生成 VideoCompositionSpec。
2. 不要调用本地渲染工具。
3. 脚本必须适合 30-60 秒中文图文视频。
4. 输出后必须进入 script review。
```

---

# 13. Role Agent 对第一版上线的影响

## 13.1 必须做的

第一版上线必须具备：

```text
[ ] RoleAgentRegistry
[ ] 每个阶段绑定一个 RoleAgent
[ ] RoleAgent 声明 allowedTools / forbiddenTools
[ ] StageGuard 校验工具越权
[ ] 每个 RoleAgent 有 requiredInputs / requiredOutputs
[ ] 每个 RoleAgent 输出 artifact
[ ] RoleMemory 进入 CompactedContext
```

---

## 13.2 可以简化的

第一版可以简化：

```text
1. 不必每个 RoleAgent 独立运行一个模型实例。
2. 不必每个 RoleAgent 有独立长期记忆库。
3. 不必做复杂多 Agent 协商。
4. 不必做并行多角色执行。
```

第一版只需要：

```text
同一个 LLMPlanner / Worker
  +
不同 RoleAgent Prompt
  +
不同工具权限
  +
不同 artifact contract
```

---

# 14. 更新后的优先级

## P0：上线必做

```text
1. RoleAgentRegistry
2. StageGuard
3. 8 个核心 RoleAgent 定义
4. RoleAgent allowedTools / forbiddenTools
5. RoleAgent requiredInputs / requiredOutputs
6. humanReview 贯通 review node
7. ArtifactIndex
8. StaleTracker
9. RenderDependencyGuard
10. 一句话到 final.mp4 E2E
```

## P0.5：上线前最好做

```text
1. AgentRunSession
2. AgentEvent
3. CompactedContext
4. RoleMemory
5. Resume API
```

## P1：第一版内测后增强

```text
1. 多候选 composition
2. 自动质量评分
3. ContinuityProfile 深度校验
4. ReferenceAssetIndex
5. 多角色独立上下文
```

---

# 15. 数据结构补充

## 15.1 RoleAgent

```go
type RoleAgent struct {
    ID              string   `json:"id"`
    Name            string   `json:"name"`
    DisplayName     string   `json:"displayName"`
    Stage           string   `json:"stage"`
    Goal            string   `json:"goal"`

    RequiredInputs  []string `json:"requiredInputs"`
    RequiredOutputs []string `json:"requiredOutputs"`

    AllowedTools    []string `json:"allowedTools"`
    ForbiddenTools  []string `json:"forbiddenTools"`

    HumanReview     *HumanReviewPolicy `json:"humanReview,omitempty"`
    QualityPolicy   *QualityPolicy     `json:"qualityPolicy,omitempty"`
}
```

---

## 15.2 StageExecutionContext

```go
type StageExecutionContext struct {
    ProjectID        string   `json:"projectId"`
    RunID            string   `json:"runId"`
    Stage            string   `json:"stage"`
    RoleAgentID      string   `json:"roleAgentId"`

    InputArtifacts   []ArtifactPointer `json:"inputArtifacts"`
    OutputArtifacts  []ArtifactPointer `json:"outputArtifacts"`

    AllowedTools     []string `json:"allowedTools"`
    ForbiddenTools   []string `json:"forbiddenTools"`

    CompactedContext *CompactedContext `json:"compactedContext,omitempty"`
}
```

---

## 15.3 RoleMemory

```go
type RoleMemory struct {
    RoleID       string   `json:"roleId"`
    Stage        string   `json:"stage"`
    Summary      string   `json:"summary"`
    KeyDecisions []string `json:"keyDecisions"`
    Constraints  []string `json:"constraints"`
    ArtifactRefs []string `json:"artifactRefs"`
    OpenIssues   []string `json:"openIssues"`
    UpdatedAt    string   `json:"updatedAt"`
}
```

---

# 16. API 补充

## 16.1 Role Agent API

```http
GET /api/video/role-agents
GET /api/video/role-agents/{roleId}
```

---

## 16.2 Stage Context API

```http
GET /api/video/projects/{projectId}/stages/{stage}/context
```

返回：

```json
{
  "stage": "script",
  "roleAgent": {
    "id": "script_writer",
    "displayName": "脚本编剧"
  },
  "inputArtifacts": [],
  "allowedTools": [],
  "forbiddenTools": [],
  "humanReview": {}
}
```

---

# 17. 前端升级

## 17.1 增加角色视图

前端阶段条不只显示“阶段”，还显示当前角色：

```text
创意总监：生成创作方案
脚本编剧：生成口播脚本
卡片设计师：拆分画面页
结构导演：生成视频结构
连续性管理：检查风格一致
预览导演：生成预览图
渲染制片：生成 final.mp4
质量审核员：检查视频质量
交付制片：打包导出
```

---

## 17.2 审核面板显示角色责任

审核面板显示：

```text
当前角色
该角色职责
本阶段输入
本阶段输出
审核重点
允许操作
```

---

# 18. E2E 验收标准

## 18.1 标准输入

```text
请帮我做一个 45 秒视频，讲 AI Agent 改变的是工作流。
```

## 18.2 必须发生

```text
1. CreativeDirectorAgent 生成 VIDEO_PROPOSAL
2. 用户确认 proposal
3. ScriptWriterAgent 生成 VIDEO_SCRIPT
4. 用户确认 script
5. StoryboardArtistAgent 生成 CARD_PLAN
6. CompositionDirectorAgent 生成 VIDEO_COMPOSITION_SPEC
7. 用户确认 composition
8. ContinuityKeeperAgent 输出 CONTINUITY_REPORT
9. PreviewDirectorAgent 生成 HYPERFRAMES_PROJECT 和 PREVIEW_SNAPSHOTS
10. 用户确认 preview
11. RenderProducerAgent 通过 RenderDependencyGuard
12. 用户确认最终渲染
13. local-backend 生成 final.mp4
14. QualityReviewerAgent 输出 FINAL_REVIEW
15. PackageProducerAgent 输出 PROJECT_PACKAGE
16. 前端可预览 final.mp4
17. 前端可导出 package
```

---

# 19. Coding Agent 执行指令

```text
目标：
将 Guided Video Studio v2 升级为 v2.1，多角色 Agent 成为上线必选主架构。参考 ViMax 的多角色视频创作思想，把视频流程拆成 CreativeDirectorAgent、ScriptWriterAgent、StoryboardArtistAgent、CompositionDirectorAgent、ReferenceSelectorAgent、ContinuityKeeperAgent、PreviewDirectorAgent、RenderProducerAgent、QualityReviewerAgent、PackageProducerAgent。第一版不要求每个角色独立服务，但必须有 RoleAgent 定义、工具权限、输入输出产物、审核规则、StageGuard 和 RoleMemory。

P0：RoleAgentRegistry
1. 新增 RoleAgent 数据结构。
2. 新增 agents/*.agent.yaml。
3. 实现 RoleAgentRegistry。
4. 每个阶段绑定一个 RoleAgent。
5. 每个 RoleAgent 声明 allowedTools、forbiddenTools、requiredInputs、requiredOutputs。

P0：StageGuard
1. 校验当前 stage 不得使用 forbiddenTools。
2. 校验 requiredInputs 是否存在且 valid。
3. 校验 requiredOutputs 是否生成。
4. 校验 ScriptWriterAgent 不得调用 render/package 工具。
5. 校验 PreviewDirectorAgent 不得调用 render。
6. 校验 RenderProducerAgent 必须等待 preview_review.approved。

P0：角色产物链路
1. CreativeDirectorAgent 输出 VIDEO_PROPOSAL。
2. ScriptWriterAgent 输出 VIDEO_SCRIPT。
3. StoryboardArtistAgent 输出 CARD_PLAN。
4. CompositionDirectorAgent 输出 VIDEO_COMPOSITION_SPEC。
5. ReferenceSelectorAgent 输出 REFERENCE_ASSET_PLAN。
6. ContinuityKeeperAgent 输出 CONTINUITY_REPORT。
7. PreviewDirectorAgent 输出 PREVIEW_SNAPSHOTS。
8. RenderProducerAgent 输出 VIDEO/RENDER_REPORT。
9. QualityReviewerAgent 输出 FINAL_REVIEW。
10. PackageProducerAgent 输出 PROJECT_PACKAGE。

P0：humanReview 贯通
1. PlanCompiler review node input 增加 humanReview。
2. 前端审核面板显示当前 RoleAgent、职责、审核重点、允许操作。
3. 审核通过后 ArtifactIndex.humanApproved=true。

P0：ArtifactIndex / StaleTracker / RenderDependencyGuard
1. 每个 RoleAgent 输出 artifact 后写 ArtifactIndex。
2. 用户修改上游 artifact 后，StaleTracker 作废下游产物。
3. RenderDependencyGuard 禁止 preview 未确认就 render。

P0.5：RoleMemory 与上下文压缩
1. CompactedContext 增加 roleMemories。
2. 每个 stage 完成后更新 RoleMemory。
3. Resume 时加载当前 RoleAgent、RoleMemory、ArtifactIndex、DecisionLog。
4. 压缩时保留每个角色的关键决策和 artifact refs。

P1：前端角色视图
1. 阶段条显示角色名。
2. 审核面板显示当前角色职责。
3. 项目状态栏显示当前 RoleAgent、当前阶段、待审核产物、下游 stale 情况。

验收：
1. 一句话到 final.mp4 E2E 通过。
2. 每个阶段都有明确 RoleAgent。
3. 每个 RoleAgent 不能越权使用工具。
4. 每个关键产物写入 ArtifactIndex。
5. 用户修改脚本后，下游产物 stale。
6. preview 未确认不能 render。
7. Resume 后能恢复当前 RoleAgent 和 pending review。
```

---

# 20. 最终结论

本次优化的关键变化：

```text
1. 多角色 Agent 从 P1 增强项提升为 P0 上线必选能力。
2. 新增 StoryboardArtistAgent，补齐脚本到视频结构之间的创作层。
3. 明确 RoleAgent 不等于微服务，第一版可用角色配置 + 工具权限 + artifact contract 落地。
4. 引入 StageGuard，防止角色越权调用工具。
5. 引入 RoleMemory，让上下文压缩保留每个角色的阶段记忆。
6. 前端要展示“当前角色”，而不是只展示“当前阶段”。
```

一句话总结：

**Guided Video Studio v2.1 的核心不是一个 Agent 生成视频，而是一组专业角色围绕同一个 Artifact 工作区协作完成视频。第一版可以轻量实现，但角色、工具权限、产物契约和审核边界必须全部具备。**