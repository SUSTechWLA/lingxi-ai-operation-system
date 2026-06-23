# 完整升级方案：从固定 Workflow 转为 Skill 驱动的动态视频 Agent

## 0. 总结

你的系统应该从：

```text
用户需求 → Skill 路由 → Skill 编译成 Workflow Template → 实例化 Workflow → 执行
```

升级为：

```text
用户需求
  ↓
动态 Agent 理解目标
  ↓
检索 Skill 能力包 + 工具类
  ↓
生成临时 AgentPlan
  ↓
系统校验工具、参数、成本、审核策略
  ↓
编译成一次性临时 DAG
  ↓
Orchestrator 执行
  ↓
中间产物需要用户审核时自动暂停
  ↓
用户通过后继续执行
```

核心变化：

> **取消 Workflow 固化作为主路径。保留 Orchestrator 的 DAG 执行能力，但不再把 Skill 固化成长期 Workflow Template。Skill 应该变成 Agent 的能力包，工具类成为执行单元，人工审核由 ToolManifest 声明自动触发。**

---

# 1. 当前系统基础判断

你当前仓库已经具备升级基础，不需要推翻重写。

README 显示系统分为 `frontend`、`local-backend`、`cloud-backend` 三部分，其中 `cloud-backend` 是云端 AIOS Core，负责 LLM/API、配置、编排、云端日志和外部集成。([GitHub][1])

架构文档显示当前核心层已经有 `orchestrator`、`workflow`、`skillruntime`、`artifact`、`worker/tool`、`modelgateway`、`context` 等模块。([GitHub][2])

当前 Chat Agent 的 `PlanService` 已经会把对话历史、媒体上下文和工具清单交给 LLM，让 LLM 直接生成 `DAGRequest`。这说明你现在已经具备“LLM 规划 DAG”的早期雏形，但它的问题是过早让模型直接产出执行图，缺少中间计划层、工具审核策略和成本控制。([GitHub][3])

当前 `ToolManifestService` 已经支持工具清单 DB 持久化、Redis 缓存、启动同步内置工具、注册外部工具。这个模块应该继续作为动态 Agent 的工具知识库基础。([GitHub][4])

当前 `ToolManifest` 已经有 `name / description / type / endpoint / timeout / parameters / output / sandbox / examples` 等基础字段，但缺少 `capabilities`、`costLevel`、`approvalPolicy`、`artifactPolicy`、`nextRecommendedTools` 等动态 Agent 需要的字段。([GitHub][5])

当前 Orchestrator 已经支持 `CONTROL` 节点进入 READY 后暂停任务，并记录“requires review”的上下文。这正好可以复用为中间产物人工审核机制。([GitHub][6])

当前数据库已有 `ai_task`、`ai_node`、`ai_node_dependency`、`ai_context`、`tool_manifests`、`workflow_templates`、`workflow_runs`、`artifacts` 等表。`workflow_templates` 和 `workflow_runs` 可以作为旧版固定流程保留，但动态 Agent 主路径应直接使用 `ai_task / ai_node / ai_node_dependency / ai_context / artifacts`。([GitHub][2])

---

# 2. 新架构定位

## 2.1 不再把 Workflow 作为主抽象

旧主路径：

```text
Skill Package
  ↓
compile
  ↓
workflow_templates
  ↓
workflow_runs
  ↓
DAG
  ↓
执行
```

新主路径：

```text
Skill Package
  ↓
Skill Capability Registry
  ↓
Agent Runtime 动态检索能力
  ↓
AgentPlan
  ↓
PlanGuard 校验
  ↓
Transient DAG
  ↓
Orchestrator 执行
```

关键区别：

| 维度       | 旧 Workflow 固化   | 新动态 Agent                  |
| -------- | --------------- | -------------------------- |
| 流程来源     | 开发者提前写死         | Agent 每次根据用户需求生成           |
| Skill 作用 | 编译成固定 Workflow  | 注册为能力包、规则、工具、参考资料          |
| 工具使用     | 固定节点调用          | Agent 动态选择和组合              |
| 人工审核     | Skill stage 上配置 | ToolManifest 中声明，系统自动插入    |
| 成本控制     | 流程固定，较可控        | 通过 TopK 工具、预算、PlanGuard 控制 |
| 灵活性      | 较低              | 高                          |
| 适合场景     | 高稳定批处理          | 视频 Agent、内容创作、模糊需求         |

---

# 3. 目标模块结构

建议新增两个核心包：

```text
cloud-backend/internal/core/agentruntime/
cloud-backend/internal/core/skillcapability/
```

## 3.1 `agentruntime`

负责动态 Agent 执行。

```text
cloud-backend/internal/core/agentruntime/
├── plan.go                 # AgentPlan / AgentStep / Budget 定义
├── runner.go               # Agent 主入口
├── planner.go              # LLM 生成 AgentPlan
├── tool_retriever.go       # TopK 工具检索
├── skill_retriever.go      # 检索相关 Skill 能力包
├── plan_guard.go           # 工具、参数、权限、成本、审核校验
├── plan_compiler.go        # AgentPlan → 临时 DAG
├── approval_policy.go      # 审核策略解析
├── artifact_gate.go        # 中间产物审核门
├── observation.go          # 工具输出摘要
├── budget.go               # 成本预算
├── handler.go              # /api/agent/runs
└── prompts/
    ├── planner_prompt.go
    └── tool_selection_prompt.go
```

## 3.2 `skillcapability`

负责把 Skill 文件夹变成 Agent 能力资产。

```text
cloud-backend/internal/core/skillcapability/
├── manifest.go             # Skill capability manifest
├── package_scanner.go      # 扫描 skill 文件夹
├── loader.go               # 加载 skillcap.yaml
├── registry.go             # Skill 能力注册表
├── resource_index.go       # rules/references/examples 索引
├── prompt_loader.go        # 加载 prompt 模板
├── tool_builder.go         # prompt 工具注册
├── recipe_loader.go        # recipe 仅作为建议路径
└── handler.go              # /api/skill-capabilities/*
```

---

# 4. Skill 不再编译成 Workflow，而是注册成 Agent 能力包

## 4.1 新 Skill 目录结构

你通过 Codex 沉淀出来的 Skill 应该放成这种结构：

```text
cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/
├── skillcap.yaml
├── README.md
│
├── rules/
│   ├── global.md
│   ├── script_rules.md
│   ├── shot_rules.md
│   ├── video_prompt_rules.md
│   └── review_rules.md
│
├── prompts/
│   ├── topic_analyzer.prompt.md
│   ├── script_generator.prompt.md
│   ├── shot_splitter.prompt.md
│   ├── keyframe_prompt_generator.prompt.md
│   ├── video_prompt_generator.prompt.md
│   ├── publish_copy_generator.prompt.md
│   └── package_reviewer.prompt.md
│
├── references/
│   ├── style_reference.md
│   ├── platform_reference.md
│   ├── camera_language_reference.md
│   └── successful_cases.md
│
├── schemas/
│   ├── topic_analysis.schema.json
│   ├── script.schema.json
│   ├── shot_list.schema.json
│   ├── video_prompt.schema.json
│   └── package_review.schema.json
│
├── tools/
│   ├── video_script_generator.tool.yaml
│   ├── shot_splitter.tool.yaml
│   ├── video_prompt_generator.tool.yaml
│   └── package_reviewer.tool.yaml
│
├── scripts/
│   ├── validate_shot_list.py
│   ├── validate_video_prompt.py
│   └── export_video_package.py
│
├── examples/
│   ├── example_input.json
│   ├── example_script.md
│   ├── example_shot_list.json
│   ├── example_video_prompt.md
│   └── example_final_package.md
│
└── recipe.yaml
```

注意：这里没有 `workflow.yaml`。`recipe.yaml` 只是 Agent 的建议路径，不是固化流程。

---

## 4.2 `skillcap.yaml`

```yaml
id: codex-video-skill
name: Codex 视频创作能力包
version: 1.0.0
domain: video_creation
description: Codex 沉淀的视频创作能力，用于生成口播稿、分镜、关键帧提示词、视频提示词和发布文案。

activation:
  intents:
    - video_creation
    - script_generation
    - storyboard_generation
    - video_prompt_generation
  keywords:
    - 视频
    - 口播
    - 分镜
    - shot
    - prompt
    - 小红书
    - 短视频
    - 关键帧

resources:
  - id: global_rules
    type: rule
    path: rules/global.md
    scope: "*"
    load_strategy: full
    priority: 100

  - id: script_rules
    type: rule
    path: rules/script_rules.md
    scope: video_script_generator
    load_strategy: full
    priority: 90

  - id: shot_rules
    type: rule
    path: rules/shot_rules.md
    scope: shot_splitter
    load_strategy: full
    priority: 90

  - id: video_prompt_rules
    type: rule
    path: rules/video_prompt_rules.md
    scope: video_prompt_generator
    load_strategy: full
    priority: 90

  - id: style_reference
    type: reference
    path: references/style_reference.md
    scope: "*"
    load_strategy: summary
    priority: 60

  - id: successful_cases
    type: example
    path: examples/
    scope: "*"
    load_strategy: retrieval_top_k
    top_k: 3
    priority: 50

tools:
  - id: video_script_generator
    manifest: tools/video_script_generator.tool.yaml
    prompt: prompts/script_generator.prompt.md

  - id: shot_splitter
    manifest: tools/shot_splitter.tool.yaml
    prompt: prompts/shot_splitter.prompt.md

  - id: video_prompt_generator
    manifest: tools/video_prompt_generator.tool.yaml
    prompt: prompts/video_prompt_generator.prompt.md

recipe:
  path: recipe.yaml
  mode: advisory
```

---

# 5. ToolManifest 升级

你后续主要设计工具类，不再写复杂 workflow。工具类必须提供足够信息，让 Agent 能判断“什么时候用、如何用、是否要审核、下一步推荐什么”。

## 5.1 当前 ToolManifest 缺口

当前 `ToolManifest` 只有基础描述、参数、输出、沙箱、示例等字段。([GitHub][5])

升级后建议增加：

```go
type ToolManifest struct {
    Name        string              `json:"name"`
    Description string             `json:"description"`
    Version     string             `json:"version,omitempty"`
    Author      string             `json:"author,omitempty"`
    Type        string             `json:"type"`
    Endpoint    string             `json:"endpoint,omitempty"`
    Timeout     int                `json:"timeout,omitempty"`

    Parameters map[string]ParamDef `json:"parameters"`
    Output     map[string]ParamDef `json:"output"`
    Sandbox    bool                `json:"sandbox"`
    Examples   []ToolExample       `json:"examples,omitempty"`

    Capabilities []string          `json:"capabilities,omitempty"`
    Tags         []string          `json:"tags,omitempty"`

    CostLevel    string            `json:"costLevel,omitempty"`    // low / medium / high
    LatencyLevel string            `json:"latencyLevel,omitempty"` // low / medium / high
    RiskLevel    string            `json:"riskLevel,omitempty"`    // low / medium / high

    SideEffect bool                `json:"sideEffect,omitempty"`
    Idempotent bool                `json:"idempotent,omitempty"`

    ApprovalPolicy ApprovalPolicy  `json:"approvalPolicy,omitempty"`
    ArtifactPolicy ArtifactPolicy  `json:"artifactPolicy,omitempty"`

    NextRecommendedTools []string  `json:"nextRecommendedTools,omitempty"`
    FailureModes         []string  `json:"failureModes,omitempty"`

    SkillPackageID string          `json:"skillPackageId,omitempty"`
    PromptRef      string          `json:"promptRef,omitempty"`
    ResourceRefs   []string        `json:"resourceRefs,omitempty"`

    RegisteredAt time.Time         `json:"registeredAt,omitempty"`
}

type ApprovalPolicy struct {
    Required            bool     `json:"required"`
    Mode                string   `json:"mode"` // none / before_execute / after_artifact / before_downstream / before_side_effect / always
    BlocksDownstream    bool     `json:"blocksDownstream"`
    Reason              string   `json:"reason,omitempty"`
    ReviewArtifactKinds []string `json:"reviewArtifactKinds,omitempty"`
}

type ArtifactPolicy struct {
    ProduceArtifact       bool     `json:"produceArtifact"`
    ArtifactKinds         []string `json:"artifactKinds"`
    DefaultReviewRequired bool     `json:"defaultReviewRequired"`
}
```

---

## 5.2 口播稿工具示例

```yaml
name: video_script_generator
description: 根据用户选题、平台、风格生成短视频口播稿。适合视频创作早期，不用于生成分镜或视频 Prompt。
type: builtin_prompt_tool
version: 1.0.0

capabilities:
  - video_creation
  - script_generation
  - short_video
  - xiaohongshu

tags:
  - video
  - script
  - talking_head

parameters:
  topic:
    type: string
    required: true
    description: 视频主题
  platform:
    type: string
    required: false
    default: 小红书
    description: 发布平台
  style:
    type: string
    required: false
    description: 视频风格

output:
  script:
    type: string
    description: 口播稿正文
  summary:
    type: string
    description: 摘要
  estimatedDurationSec:
    type: number
    description: 估算时长

costLevel: low
latencyLevel: medium
riskLevel: low
sideEffect: false
idempotent: true

approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: 口播稿会影响后续分镜和视频 Prompt，必须用户确认后继续。
  reviewArtifactKinds:
    - MARKDOWN

artifactPolicy:
  produceArtifact: true
  artifactKinds:
    - MARKDOWN
  defaultReviewRequired: true

nextRecommendedTools:
  - shot_splitter
  - publish_copy_generator
```

---

# 6. 新执行链路

## 6.1 用户发起

新增接口：

```http
POST /api/agent/runs
```

请求：

```json
{
  "message": "帮我把“AI替代的不是岗位，而是整套工作流程”做成一个小红书视频创作包",
  "domain": "video_creation",
  "context": {
    "platform": "小红书",
    "aspectRatio": "16:9",
    "language": "中文",
    "targetDurationSec": 60
  },
  "mode": "auto"
}
```

返回：

```json
{
  "runId": "agent_run_001",
  "taskId": "task_001",
  "status": "RUNNING"
}
```

---

## 6.2 Agent Runtime 内部流程

```text
1. IntentRouter 判断领域：video_creation
2. SkillCapabilityRetriever 检索相关能力包：codex-video-skill
3. ToolRetriever 检索 TopK 工具：video_script_generator、shot_splitter、video_prompt_generator 等
4. Planner 生成 AgentPlan
5. PlanGuard 校验：
   - 工具是否存在
   - 参数是否满足 schema
   - 成本是否超预算
   - 是否有副作用
   - 是否需要人工审核
   - 是否引用未经审核产物
6. PlanCompiler 生成临时 DAG
7. Orchestrator 执行 DAG
8. CONTROL 节点 READY 后任务自动暂停
9. 用户审核通过后继续
10. 最终输出创作包
```

---

# 7. AgentPlan 设计

LLM 不再直接输出 `DAGRequest`，而是输出 `AgentPlan`。

## 7.1 数据结构

```go
type AgentPlan struct {
    Goal       string      `json:"goal"`
    Domain     string      `json:"domain"`
    Mode       string      `json:"mode"` // dynamic_agent
    Steps      []AgentStep `json:"steps"`
    Budget     AgentBudget `json:"budget"`
    StopPolicy StopPolicy  `json:"stopPolicy"`
}

type AgentStep struct {
    ID              string                 `json:"id"`
    Intent          string                 `json:"intent"`
    Tool            string                 `json:"tool"`
    Arguments       map[string]interface{} `json:"arguments"`
    DependsOn       []string               `json:"dependsOn,omitempty"`
    ExpectedOutput  []string               `json:"expectedOutput,omitempty"`
    ProduceArtifact bool                   `json:"produceArtifact,omitempty"`
}

type AgentBudget struct {
    MaxLLMCalls  int    `json:"maxLLMCalls"`
    MaxToolCalls int    `json:"maxToolCalls"`
    MaxSteps     int    `json:"maxSteps"`
    MaxReplans   int    `json:"maxReplans"`
    MaxCostLevel string `json:"maxCostLevel"`
}

type StopPolicy struct {
    StopWhenEnough bool `json:"stopWhenEnough"`
}
```

---

## 7.2 AgentPlan 示例

```json
{
  "goal": "生成小红书 AI 视频创作包",
  "domain": "video_creation",
  "mode": "dynamic_agent",
  "steps": [
    {
      "id": "script_generation",
      "intent": "生成口播稿",
      "tool": "video_script_generator",
      "arguments": {
        "topic": "AI替代的不是岗位，而是整套工作流程",
        "platform": "小红书",
        "style": "真诚、有价值、适合中文平台"
      },
      "expectedOutput": ["script", "summary"],
      "produceArtifact": true
    },
    {
      "id": "shot_split",
      "intent": "把用户确认后的口播稿拆成分镜",
      "tool": "shot_splitter",
      "dependsOn": ["script_generation"],
      "arguments": {
        "script": "{{script_generation.output.script}}",
        "shotDurationRule": "3-15秒"
      },
      "expectedOutput": ["shotList"],
      "produceArtifact": true
    },
    {
      "id": "video_prompt_generation",
      "intent": "根据分镜生成视频生成提示词",
      "tool": "video_prompt_generator",
      "dependsOn": ["shot_split"],
      "arguments": {
        "shotList": "{{shot_split.output.shotList}}",
        "style": "非写实动画，去AI感，16:9"
      },
      "expectedOutput": ["videoPrompts"],
      "produceArtifact": true
    }
  ],
  "budget": {
    "maxLLMCalls": 4,
    "maxToolCalls": 6,
    "maxSteps": 6,
    "maxReplans": 1,
    "maxCostLevel": "medium"
  },
  "stopPolicy": {
    "stopWhenEnough": true
  }
}
```

---

# 8. PlanCompiler：AgentPlan 转临时 DAG

虽然取消 Workflow 固化，但不取消 DAG 执行。你的 Orchestrator 仍然需要 DAG，只是不再把它存成长期 `workflow_templates`。

## 8.1 编译规则

| AgentStep                                | 编译结果                           |
| ---------------------------------------- | ------------------------------ |
| 普通工具                                     | `TOOL` 节点                      |
| `approvalPolicy.mode=before_execute`     | `CONTROL` → `TOOL`             |
| `approvalPolicy.mode=after_artifact`     | `TOOL` → `CONTROL`             |
| `approvalPolicy.mode=before_downstream`  | `TOOL` → `CONTROL` → 下游        |
| `approvalPolicy.mode=before_side_effect` | `CONTROL` → `TOOL`             |
| `approvalPolicy.mode=always`             | `CONTROL` → `TOOL` → `CONTROL` |

---

## 8.2 示例

AgentPlan：

```text
script_generation → shot_split → video_prompt_generation
```

根据工具的 `approvalPolicy` 编译后：

```text
script_generation_exec TOOL
  ↓
script_generation_review CONTROL
  ↓
shot_split_exec TOOL
  ↓
shot_split_review CONTROL
  ↓
video_prompt_generation_exec TOOL
  ↓
video_prompt_generation_review CONTROL
```

`CONTROL` 节点进入 READY 后，当前系统已经会把 task 置为 PAUSED。([GitHub][6])

---

# 9. 中间产物审核机制

## 9.1 产物必须走 Artifact

工具输出不要把大正文直接塞进 node output。

工具返回：

```json
{
  "success": true,
  "data": {
    "summary": "已生成一版约900字口播稿。",
    "artifacts": [
      {
        "unitId": "script",
        "kind": "MARKDOWN",
        "name": "口播稿.md",
        "storageRef": "local://projects/p1/artifacts/script/v1.md",
        "contentHash": "sha256:xxx",
        "reviewRequired": true
      }
    ],
    "nextRecommendedTools": ["shot_splitter"]
  }
}
```

你的系统已有 `artifacts` 表，架构文档中说明它用于版本化产物索引，包含 `project/stage/unit/kind/version/parent_id/storage_type/storage_ref/content_hash/is_current` 等信息。([GitHub][2])

---

## 9.2 新增 Artifact Review 表

建议新增：

```sql
CREATE TABLE IF NOT EXISTS artifact_reviews (
    id VARCHAR PRIMARY KEY,
    task_id VARCHAR NOT NULL,
    node_id VARCHAR NOT NULL,
    artifact_id VARCHAR,
    storage_ref TEXT,
    status VARCHAR NOT NULL, -- PENDING / APPROVED / REJECTED
    review_reason TEXT,
    reviewer_id VARCHAR,
    review_comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    reviewed_at TIMESTAMP
);
```

用途：

```text
script_generator 输出口播稿.md
  ↓
artifact_reviews 插入 PENDING
  ↓
CONTROL 节点 READY
  ↓
用户审核
  ↓
APPROVED / REJECTED
```

---

## 9.3 未审核产物禁止进入下游

PlanGuard / Executor 必须检查：

```text
如果 artifact.reviewRequired = true
并且 artifact.reviewStatus != APPROVED
则下游工具不能读取它
```

这样就能保证：

```text
口播稿未确认 → 不能拆分分镜
分镜未确认 → 不能生成视频 Prompt
视频 Prompt 未确认 → 不能调用视频生成模型
```

---

# 10. API 设计

## 10.1 动态 Agent

```http
POST /api/agent/runs
GET  /api/agent/runs/:runId
GET  /api/agent/runs/:runId/trace
POST /api/agent/runs/:runId/cancel
```

## 10.2 审核

```http
GET  /api/agent/runs/:runId/reviews
POST /api/agent/runs/:runId/reviews/:reviewId/approve
POST /api/agent/runs/:runId/reviews/:reviewId/reject
POST /api/agent/runs/:runId/reviews/:reviewId/revise
```

`approve` 内部逻辑：

```text
artifact_reviews.status = APPROVED
CONTROL node → SUCCESS
task 从 PAUSED 恢复
下游节点进入 READY
```

`reject` 内部逻辑：

```text
artifact_reviews.status = REJECTED
CONTROL node → FAILED 或保持 PAUSED
用户选择终止 / 返工
```

`revise` 内部逻辑：

```text
用户给修改意见
  ↓
创建局部 replan
  ↓
重跑上游工具
```

---

# 11. Skill 能力包注册流程

## 11.1 启动加载

```text
启动 cloud-backend
  ↓
扫描 cloud-backend/skill-capabilities/**
  ↓
加载 skillcap.yaml
  ↓
注册 resources
  ↓
注册 prompt tools
  ↓
加载 recipe hints
  ↓
写入 SkillCapabilityRegistry
```

## 11.2 工具注册

从 `tools/*.tool.yaml` 生成 ToolManifest，并注册到现有 `ToolManifestService`。当前系统已经有工具清单持久化与 Redis 缓存机制，可以直接扩展复用。([GitHub][4])

## 11.3 Prompt Tool

Prompt Tool 是把 Prompt 模板包装成工具：

```text
video_script_generator
  ↓
加载 prompts/script_generator.prompt.md
  ↓
加载相关 rules / references / examples
  ↓
调用 ModelGateway
  ↓
输出结构化结果 + artifact
```

---

# 12. 成本控制方案

## 12.1 不给 LLM 全量工具

当前 Chat PlanService 会构建包含工具清单的 prompt，并让 LLM 直接生成 DAG。([GitHub][3])

升级后改为：

```text
用户需求
  ↓
ToolRetriever
  ↓
TopK 工具，默认 6-8 个
  ↓
Planner
```

优点：

```text
降低 token
降低工具误选率
降低推理耗时
降低成本
```

---

## 12.2 模型分层

| 步骤               | 模型                           |
| ---------------- | ---------------------------- |
| IntentRouter     | 小模型                          |
| ToolRetriever    | 规则 + 标签 + embedding，尽量不用 LLM |
| AgentPlan 生成     | 中模型                          |
| Prompt Tool 内容生成 | 按工具配置选择模型                    |
| 输出摘要             | 小模型                          |
| 最终整合             | 中模型                          |

---

## 12.3 Plan Once 优先

默认不要 ReAct 无限循环。

```text
默认：Plan Once
失败：局部 Replan 一次
最大：MaxReplans = 1 或 2
```

---

# 13. 需要修改的代码位置

## 13.1 新增

```text
cloud-backend/internal/core/agentruntime/
cloud-backend/internal/core/skillcapability/
```

## 13.2 修改 ToolManifest

```text
cloud-backend/internal/core/worker/tool/manifest.go
cloud-backend/internal/agents/chat/service/tool_manifest_service.go
cloud-backend/internal/core/model/model.go
cloud-backend/internal/core/model/repository/*
```

## 13.3 修改数据库

```text
cloud-backend/internal/core/database/*
cloud-backend/deploy/migrations/*
```

新增或扩展：

```sql
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS capabilities JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS tags JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS cost_level VARCHAR DEFAULT 'low';
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS latency_level VARCHAR DEFAULT 'medium';
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS risk_level VARCHAR DEFAULT 'low';
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS side_effect BOOLEAN DEFAULT FALSE;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS idempotent BOOLEAN DEFAULT TRUE;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS approval_policy JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS artifact_policy JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS next_recommended_tools JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS failure_modes JSONB;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS skill_package_id VARCHAR;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS prompt_ref TEXT;
ALTER TABLE tool_manifests ADD COLUMN IF NOT EXISTS resource_refs JSONB;
```

新增：

```sql
CREATE TABLE IF NOT EXISTS agent_runs (
    id VARCHAR PRIMARY KEY,
    task_id VARCHAR,
    user_id VARCHAR,
    domain VARCHAR,
    message TEXT,
    plan_json JSONB,
    status VARCHAR NOT NULL,
    budget_json JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS agent_steps (
    id VARCHAR PRIMARY KEY,
    run_id VARCHAR NOT NULL,
    step_id VARCHAR NOT NULL,
    tool_name VARCHAR,
    status VARCHAR NOT NULL,
    input_json JSONB,
    output_json JSONB,
    artifact_ids JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS skill_capabilities (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    version VARCHAR NOT NULL,
    domain VARCHAR,
    root_path TEXT,
    manifest_json JSONB,
    status VARCHAR NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS artifact_reviews (
    id VARCHAR PRIMARY KEY,
    task_id VARCHAR NOT NULL,
    node_id VARCHAR NOT NULL,
    artifact_id VARCHAR,
    storage_ref TEXT,
    status VARCHAR NOT NULL,
    review_reason TEXT,
    reviewer_id VARCHAR,
    review_comment TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    reviewed_at TIMESTAMP
);
```

---

# 14. 前端升级

当前前端创作台已经有自然语言入口、系统理解面板、制作进度和产物审片台，并会轮询 artifacts 展示产物。([GitHub][2])

升级方向：

```text
CreatorWorkbenchPage
  ↓
从“选 Skill + 启动 workflow run”
改成
  ↓
“自然语言 → /api/agent/runs”
```

新增组件：

```text
AgentRunPanel
├── 当前目标
├── Agent 计划
├── 工具调用时间线
├── 待审核产物
├── 通过 / 驳回 / 修改意见
└── 最终创作包
```

当前 `/api/video-projects/:id/stages/:stage/approve` 已经是视频域阶段审核封装，后续可以保留兼容，但新主路径建议用 `/api/agent/runs/:runId/reviews/*`。([GitHub][2])

---

# 15. 从 Codex Skill 转成视频 Agent 功能的实际步骤

## 15.1 整理 Skill 文件夹

把 Codex Skill 拆成：

```text
rules/
prompts/
references/
schemas/
tools/
examples/
recipe.yaml
skillcap.yaml
```

## 15.2 先注册 3 个核心工具

最小闭环：

```text
video_script_generator
shot_splitter
video_prompt_generator
```

## 15.3 每个工具写清楚审核策略

必须审核：

```text
口播稿
分镜
视频 Prompt
```

可选审核：

```text
选题分析
标题简介
封面 Prompt
```

## 15.4 Agent 执行

用户说：

```text
帮我做一个视频
```

Agent 自动：

```text
匹配 codex-video-skill
检索相关工具
生成 AgentPlan
执行 script_generator
暂停审核
执行 shot_splitter
暂停审核
执行 video_prompt_generator
暂停审核
导出最终包
```

---

# 16. 给编码 Agent 的升级任务清单

你可以直接把下面任务交给 Codex / Claude Code 执行。

## 任务 1：扩展 ToolManifest

目标：

```text
让工具支持 capabilities、成本、风险、审核策略、产物策略、推荐下游工具。
```

修改：

```text
cloud-backend/internal/core/worker/tool/manifest.go
cloud-backend/internal/agents/chat/service/tool_manifest_service.go
cloud-backend/internal/core/model/model.go
cloud-backend/internal/core/model/repository/*
```

验收：

```text
GET /api/tools 返回新增字段
POST /api/tools/register 可注册带 approvalPolicy 的工具
旧工具不受影响，默认 required=false、costLevel=low
```

---

## 任务 2：新增 skillcapability 包

目标：

```text
支持 Skill 文件夹变成 Agent 能力包，而不是 Workflow。
```

新增：

```text
cloud-backend/internal/core/skillcapability/
```

实现：

```text
扫描 skill-capabilities 目录
加载 skillcap.yaml
加载 tools/*.tool.yaml
注册 Prompt Tool
注册 resources
加载 recipe.yaml 作为建议路径
```

验收：

```text
启动时能识别 codex-video-skill
GET /api/skill-capabilities 能看到能力包
GET /api/tools 能看到能力包注册的工具
```

---

## 任务 3：新增 agentruntime 包

目标：

```text
实现动态 Agent 运行时。
```

新增：

```text
cloud-backend/internal/core/agentruntime/
```

实现：

```text
ToolRetriever
SkillRetriever
Planner
PlanGuard
PlanCompiler
Runner
BudgetManager
ObservationSummarizer
```

验收：

```text
POST /api/agent/runs 能根据用户需求生成 AgentPlan
AgentPlan 能编译成临时 DAG
DAG 不写入 workflow_templates
任务进入 ai_task / ai_node 执行
```

---

## 任务 4：实现审核门自动插入

目标：

```text
根据 ToolManifest.approvalPolicy 自动插入 CONTROL 节点。
```

实现规则：

```text
after_artifact → TOOL → CONTROL
before_execute → CONTROL → TOOL
before_side_effect → CONTROL → TOOL
always → CONTROL → TOOL → CONTROL
```

验收：

```text
video_script_generator 生成后任务 PAUSED
用户 approve 后 shot_splitter 才能执行
用户 reject 后下游不执行
```

---

## 任务 5：实现 Artifact Review

目标：

```text
中间态产物必须用户确认后才能进入下游。
```

实现：

```text
artifact_reviews 表
/api/agent/runs/:runId/reviews
approve/reject/revise API
PlanGuard 检查未审核 artifact
```

验收：

```text
口播稿 artifact 未 APPROVED 时，shot_splitter 不执行
审核通过后继续
驳回后可带修改意见重跑上游
```

---

## 任务 6：改造前端创作台

目标：

```text
从 workflow run 入口改成 dynamic agent run 入口。
```

修改：

```text
frontend CreatorWorkbenchPage
新增 AgentRunPanel
新增 ReviewQueue
复用 ArtifactReviewModal
```

验收：

```text
用户一句话启动 Agent
前端显示执行计划
中间产物弹窗审核
审核通过后继续执行
```

---

# 17. 最终产品形态

最终你的系统会变成：

```text
用户不断沉淀 Skill
  ↓
Skill 被拆成规则、Prompt、参考资料、工具 Manifest、样例
  ↓
AIOS 注册为 Agent 能力包
  ↓
用户自然语言提出需求
  ↓
Agent 动态检索能力包和工具
  ↓
生成临时计划
  ↓
系统执行工具组合
  ↓
中间产物必须用户审核
  ↓
最终生成视频创作包
```

核心原则：

```text
Workflow 不再是主路径
Skill 不再被强制编译成 Workflow
Skill 变成 Agent 能力包
工具类成为 Agent 的基本能力
审核策略写在工具 Manifest 中
Orchestrator 只负责执行临时 DAG
CONTROL 节点负责人工审核
Artifact 负责中间产物版本化
```

一句话概括：

> **把 Skill 从“固定流程源码”升级为“Agent 能力资产”；把 Workflow 从“长期模板”降级为“运行时临时 DAG”；让 Agent 通过工具类动态解决用户需求，并由工具声明自动控制中间产物审核。**