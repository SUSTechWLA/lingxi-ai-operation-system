# 动态 LLM Planner 升级完整方案

目标：让系统真正支持这种输入：

```text id="833v1l"
请帮我根据端午节的来历创作一个口播知识分享视频。
```

然后由 **LLM Planner 自主选择工具、编排任务、生成中间产物、等待用户审核、继续执行、最终导出视频创作包**，而不是退回固定 Skill Workflow。

---

# 1. 当前系统判断

基于我看到的公开 `develop_go` 分支，系统现在已经按运行边界拆成 `frontend / local-backend / cloud-backend`，其中 `cloud-backend` 是云端 AIOS Core，负责 LLM/API、配置、编排、云端日志和外部集成。这个边界适合继续扩展动态 Agent。([GitHub][1])

当前架构文档仍然把核心流程描述为：

```text id="1nn5sa"
用户输入一句话
  ↓
POST /api/skills/route
  ↓
POST /api/video-projects
  ↓
POST /api/video-projects/:id/workflow-runs
  ↓
从 template 取出 Skill 编译出的 DAG
  ↓
Orchestrator 执行
```

也就是说，当前视频创作主路径仍然强依赖 `Skill → Workflow Template → Workflow Run`。([GitHub][2])

同时，当前 Chat `PlanService` 仍然是让 LLM 直接生成 `DAGRequest`，代码注释也明确写着“calls the LLM directly to produce a DAG”。这不是你现在要的目标。你需要的是：**LLM 生成 AgentPlan，系统再校验和编译成临时 DAG**。([GitHub][3])

另外，当前 `ToolManifestService.FormatForPrompt` 仍然会 `ListAll` 全量工具，然后拼成 Prompt 给 LLM。这在工具少时可用，但工具多了以后会增加 token 成本、误选工具概率和规划不稳定性。([GitHub][4])

---

# 2. 升级目标

## 2.1 取消固定 Workflow 作为主路径

不要再把新 Agent 主路径设计成：

```text id="f4f87y"
Skill → Workflow Template → Workflow Run
```

改成：

```text id="hjg8pp"
自然语言需求
  ↓
LLM Planner
  ↓
AgentPlan
  ↓
PlanGuard
  ↓
Transient DAG
  ↓
Orchestrator
  ↓
Worker
  ↓
Artifact + Review
```

注意：**不是删除 Orchestrator，也不是删除 DAG。**

你要取消的是：

```text id="7xvjon"
长期固化 workflow_templates 作为主路径
```

保留的是：

```text id="euw03h"
运行时临时 DAG 执行能力
```

---

## 2.2 新旧模式并存

建议支持三种执行模式：

| 模式              | 说明                              |
| --------------- | ------------------------------- |
| `workflow`      | 旧模式，固定 Skill Workflow，保留兼容      |
| `dynamic_agent` | 新主路径，LLM Planner 动态编排           |
| `hybrid`        | LLM Planner 失败时回退 workflow，用于灰度 |

你当前要验证端午节口播视频，应走：

```text id="g4fdxv"
executionMode = dynamic_agent
```

不要回退 workflow，除非 LLM Planner 连 AgentPlan 都生成不了，或者 Guard 检查失败后无法修复。

---

# 3. 目标架构

新增两个核心模块：

```text id="bcja8f"
cloud-backend/internal/core/agentruntime/
cloud-backend/internal/core/skillcapability/
```

整体结构：

```text id="ch88pw"
用户请求
  ↓
/api/agent/runs
  ↓
AgentRuntime.Runner
  ├── IntentRouter
  ├── SkillCapabilityRetriever
  ├── ToolRetriever
  ├── LLMPlanner
  ├── PlanGuard
  ├── PlanCompiler
  └── Orchestrator.SubmitDAG
        ↓
      Worker.ExecuteNode
        ↓
      ToolRegistry / PromptTool / ExternalTool
        ↓
      ArtifactStore + CONTROL Review
```

---

# 4. 后端新增模块设计

## 4.1 新增 `agentruntime`

```text id="297l5t"
cloud-backend/internal/core/agentruntime/
├── plan.go
├── request.go
├── runner.go
├── planner.go
├── llm_planner.go
├── heuristic_planner.go
├── tool_retriever.go
├── skill_retriever.go
├── guard.go
├── compiler.go
├── budget.go
├── json_repair.go
├── observation.go
├── artifact_gate.go
├── handler.go
└── prompts/
    └── planner_prompt.go
```

职责：

| 文件                  | 职责                                     |
| ------------------- | -------------------------------------- |
| `plan.go`           | 定义 AgentPlan / AgentStep               |
| `runner.go`         | 串起 Planner、Guard、Compiler、Orchestrator |
| `llm_planner.go`    | 调用模型生成 AgentPlan JSON                  |
| `tool_retriever.go` | 从工具库检索 TopK 工具                         |
| `guard.go`          | 校验工具存在性、参数、依赖、成本、风险                    |
| `compiler.go`       | AgentPlan → 临时 DAG                     |
| `artifact_gate.go`  | 检查中间产物是否审核通过                           |
| `handler.go`        | 暴露 `/api/agent/runs`                   |

---

## 4.2 新增 `skillcapability`

```text id="25wh9d"
cloud-backend/internal/core/skillcapability/
├── manifest.go
├── loader.go
├── registry.go
├── scanner.go
├── prompt_tool_builder.go
├── resource_index.go
├── recipe_loader.go
└── handler.go
```

职责：

```text id="r77617"
Skill 文件夹不再编译成 Workflow
而是注册为 Agent 能力包
```

---

# 5. 核心数据结构

## 5.1 AgentPlan

```go id="n42erh"
type AgentPlan struct {
    Goal       string      `json:"goal"`
    Domain     string      `json:"domain,omitempty"`
    Mode       string      `json:"mode"` // dynamic_agent
    Steps      []AgentStep `json:"steps"`
    Budget     AgentBudget `json:"budget,omitempty"`
    StopPolicy StopPolicy  `json:"stopPolicy,omitempty"`
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
    MaxSteps     int    `json:"maxSteps"`
    MaxToolCalls int    `json:"maxToolCalls"`
    MaxLLMCalls  int    `json:"maxLLMCalls"`
    MaxReplans   int    `json:"maxReplans"`
    MaxCostLevel string `json:"maxCostLevel"`
}

type StopPolicy struct {
    StopWhenEnough bool `json:"stopWhenEnough"`
}
```

LLM Planner 的输出必须是这个结构，不能直接输出 DAG。

---

## 5.2 Planner 接口

```go id="hy55ao"
type Planner interface {
    Plan(ctx context.Context, req PlanRequest) (*AgentPlan, error)
}

type PlanRequest struct {
    UserID  string                 `json:"userId"`
    Message string                 `json:"message"`
    Domain  string                 `json:"domain,omitempty"`
    Context map[string]interface{} `json:"context,omitempty"`
    Budget  AgentBudget            `json:"budget"`
}
```

这样你可以保留当前启发式 Planner，再增加 LLM Planner：

```text id="kj35u1"
HeuristicPlanner implements Planner
LLMPlanner implements Planner
```

后面的 Guard、Compiler、Orchestrator、Worker 不变。

---

# 6. ToolManifest 升级

当前公开代码里的 `ToolManifest` 仍然是基础字段：`Name / Description / Type / Endpoint / Timeout / Parameters / Output / Sandbox / Examples`。([GitHub][5])

为了让 LLM Planner 能正确选择工具，必须升级为：

```go id="xka4mz"
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

关键：**审核策略写在工具上，不写在 LLM Planner 上。**

LLM Planner 只负责选工具；是否插入审核节点，由 Compiler 根据 `ApprovalPolicy` 自动完成。

---

# 7. 数据库升级

## 7.1 扩展 `tool_manifests`

当前 `ToolManifestService` 会把 ToolManifest 转成 DB record 并缓存，升级时需要把新增字段持久化。([GitHub][4])

```sql id="i25b0o"
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

---

## 7.2 新增 `agent_runs`

```sql id="u5obk5"
CREATE TABLE IF NOT EXISTS agent_runs (
    id VARCHAR PRIMARY KEY,
    task_id VARCHAR,
    user_id VARCHAR,
    domain VARCHAR,
    message TEXT NOT NULL,
    execution_mode VARCHAR NOT NULL DEFAULT 'dynamic_agent',
    planner_type VARCHAR NOT NULL DEFAULT 'llm',
    plan_json JSONB,
    status VARCHAR NOT NULL,
    budget_json JSONB,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

---

## 7.3 新增 `agent_steps`

```sql id="hcf9vj"
CREATE TABLE IF NOT EXISTS agent_steps (
    id VARCHAR PRIMARY KEY,
    run_id VARCHAR NOT NULL,
    step_id VARCHAR NOT NULL,
    tool_name VARCHAR,
    intent TEXT,
    status VARCHAR NOT NULL,
    input_json JSONB,
    output_json JSONB,
    artifact_ids JSONB,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

---

## 7.4 新增 `artifact_reviews`

```sql id="wa0x7p"
CREATE TABLE IF NOT EXISTS artifact_reviews (
    id VARCHAR PRIMARY KEY,
    task_id VARCHAR NOT NULL,
    node_id VARCHAR NOT NULL,
    agent_run_id VARCHAR,
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

---

## 7.5 新增 `skill_capabilities`

```sql id="5kj9l6"
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
```

---

# 8. LLM Planner 设计

## 8.1 LLMPlanner 输入

```text id="9h3z0c"
用户需求
领域 context
TopK 工具列表
相关 Skill Capability 摘要
预算
AgentPlan JSON Schema
```

不要输入：

```text id="l467lr"
全量工具
完整 Skill 全文
所有历史产物正文
workflow template
```

---

## 8.2 ToolForPlanner DTO

给 LLM 的工具描述要精简：

```go id="g0u4v0"
type ToolForPlanner struct {
    Name                 string              `json:"name"`
    Description          string              `json:"description"`
    Capabilities         []string            `json:"capabilities,omitempty"`
    Tags                 []string            `json:"tags,omitempty"`
    Parameters           map[string]ParamDef `json:"parameters"`
    Output               map[string]ParamDef `json:"output"`
    CostLevel            string              `json:"costLevel,omitempty"`
    RiskLevel            string              `json:"riskLevel,omitempty"`
    SideEffect           bool                `json:"sideEffect,omitempty"`
    NextRecommendedTools []string            `json:"nextRecommendedTools,omitempty"`
}
```

---

## 8.3 Planner Prompt

```text id="jdetuc"
你是 AIOS 的 LLM Planner。

你的职责：
根据用户需求，从给定工具列表中选择合适工具，并生成 AgentPlan JSON。

你不能直接执行工具。
你不能编造工具。
你不能输出自然语言解释。
你不能输出 Markdown。
你只能输出符合 JSON Schema 的 JSON 对象。

规划规则：
1. 只能使用 tools 中存在的工具 name。
2. 每个 step 必须包含 id、intent、tool、arguments。
3. 如果一个 step 依赖上游结果，必须使用 dependsOn。
4. 如果参数来自上游输出，使用 {{step_id.output.field}} 引用。
5. 尽量使用低成本、低风险、无副作用工具。
6. 不要超过 budget.maxSteps。
7. 不要规划无关步骤。
8. 工具是否需要人工审核由系统根据 ToolManifest 自动处理，你不需要插入审核步骤。
9. 发布、上传、删除、外发、付费调用等高风险动作必须选择带 approvalPolicy 的工具。
10. 输出必须是 AgentPlan JSON。

用户需求：
{{message}}

上下文：
{{context}}

预算：
{{budget}}

可用工具：
{{tools}}

相关能力包：
{{skill_capabilities}}

请输出 AgentPlan JSON。
```

---

## 8.4 JSON Schema

```go id="hx0my8"
const AgentPlanJSONSchema = `
{
  "type": "object",
  "required": ["goal", "mode", "steps"],
  "properties": {
    "goal": { "type": "string" },
    "domain": { "type": "string" },
    "mode": {
      "type": "string",
      "enum": ["dynamic_agent"]
    },
    "steps": {
      "type": "array",
      "minItems": 1,
      "maxItems": 8,
      "items": {
        "type": "object",
        "required": ["id", "intent", "tool", "arguments"],
        "properties": {
          "id": {
            "type": "string",
            "pattern": "^[a-zA-Z][a-zA-Z0-9_]{1,63}$"
          },
          "intent": { "type": "string" },
          "tool": { "type": "string" },
          "arguments": { "type": "object" },
          "dependsOn": {
            "type": "array",
            "items": { "type": "string" }
          },
          "expectedOutput": {
            "type": "array",
            "items": { "type": "string" }
          },
          "produceArtifact": { "type": "boolean" }
        }
      }
    },
    "budget": { "type": "object" },
    "stopPolicy": { "type": "object" }
  }
}
`
```

---

# 9. ToolRetriever 设计

替代全量 `FormatForPrompt`。

## 9.1 输入

```go id="xu8kaw"
type RetrieveRequest struct {
    Query        string
    Domain       string
    TopK         int
    MaxCostLevel string
    MaxRiskLevel string
}
```

## 9.2 第一版检索规则

```text id="uk2o5i"
score = 0
如果 capabilities 命中 domain，+30
如果 tags 命中关键词，+20
如果 description 命中关键词，+10
如果工具在 nextRecommendedTools 中，+10
如果 costLevel 超预算，过滤
如果 riskLevel 超权限，过滤
```

## 9.3 示例

用户输入：

```text id="uh2mrq"
请帮我根据端午节的来历创作一个口播知识分享视频
```

TopK 应返回：

```text id="8u3h43"
knowledge_researcher
fact_checker
video_script_generator
shot_splitter
video_prompt_generator
publish_copy_generator
video_package_exporter
```

---

# 10. PlanGuard 设计

LLM 输出的 AgentPlan 不允许直接执行。Guard 必须检查：

```text id="001h22"
1. 工具是否存在
2. 工具是否属于本次 TopK 或允许工具集合
3. arguments 是否满足 ToolManifest.Parameters
4. required 参数是否存在
5. dependsOn 是否引用存在 step
6. 是否有循环依赖
7. {{step.output.field}} 是否引用合法上游
8. steps 数量是否超过预算
9. costLevel 是否超过 MaxCostLevel
10. riskLevel 是否超过 MaxRiskLevel
11. sideEffect=true 的工具是否有 approvalPolicy
12. artifactPolicy.defaultReviewRequired=true 的产物是否会生成审核门
13. step.id 是否安全合法
```

失败时返回：

```go id="c5d0zl"
type GuardError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    StepID  string `json:"stepId,omitempty"`
    Tool    string `json:"tool,omitempty"`
}
```

Guard 失败策略：

| 失败类型   | 处理方式                  |
| ------ | --------------------- |
| 工具不存在  | 不执行，要求 LLM 修复一次       |
| 缺必填参数  | 若可从用户需求提取则修复，否则返回需要补充 |
| 依赖非法   | LLM 修复一次              |
| 成本超限   | 删除或替换高成本工具            |
| 高风险无审核 | 拒绝执行                  |
| 修复失败   | 返回用户明确错误              |

---

# 11. PlanCompiler 设计

## 11.1 输入

```text id="hs1dj8"
AgentPlan
ToolManifest map
```

## 11.2 输出

```text id="imwks8"
model.DAGRequest
```

但这个 DAG 是 **Transient DAG**：

```text id="ketav4"
只写 ai_task / ai_node / ai_node_dependency
不写 workflow_templates
不写 workflow_runs
```

---

## 11.3 编译规则

| ToolManifest 审核策略    | 编译结果                                                             |
| -------------------- | ---------------------------------------------------------------- |
| 无审核                  | `step_exec TOOL`                                                 |
| `before_execute`     | `step_pre_review CONTROL → step_exec TOOL`                       |
| `after_artifact`     | `step_exec TOOL → step_review CONTROL`                           |
| `before_downstream`  | `step_exec TOOL → step_review CONTROL → 下游`                      |
| `before_side_effect` | `step_pre_review CONTROL → step_exec TOOL`                       |
| `always`             | `step_pre_review CONTROL → step_exec TOOL → step_review CONTROL` |

---

## 11.4 端午视频 DAG 示例

AgentPlan：

```text id="6q4gad"
knowledge_research
  ↓
script_generation
  ↓
shot_split
  ↓
video_prompt_generation
```

如果每个工具都 `after_artifact` 审核，编译成：

```text id="0sjh7f"
knowledge_research_exec TOOL
  ↓
knowledge_research_review CONTROL
  ↓
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

当前 Orchestrator 已经支持 `CONTROL` 节点变 READY 后暂停任务，这可以复用。([GitHub][2])

---

# 12. Prompt Tool 设计

你的很多视频 Agent 工具本质上是 Prompt Tool，例如：

```text id="2uyh26"
knowledge_researcher
video_script_generator
shot_splitter
video_prompt_generator
publish_copy_generator
```

它们不是固定 Go 函数，而是：

```text id="2ngqla"
ToolManifest + promptRef + resourceRefs + ModelGateway
```

## 12.1 Prompt Tool 执行流程

```text id="gnbmp1"
Worker 接到 TOOL 节点
  ↓
读取 tool = video_script_generator
  ↓
ToolRegistry 找到 PromptTool
  ↓
读取 promptRef
  ↓
加载 resourceRefs：rules / references / examples
  ↓
把参数填入 Prompt
  ↓
调用 ModelGateway
  ↓
解析结构化输出
  ↓
保存 Artifact
  ↓
返回 output
```

---

## 12.2 Prompt Tool Manifest 示例

```yaml id="t268m6"
name: video_script_generator
description: 根据主题、事实材料和风格要求生成短视频口播稿。
type: prompt_tool
version: 1.0.0

capabilities:
  - video_creation
  - script_generation
  - knowledge_sharing

tags:
  - video
  - script
  - oral_script
  - chinese_platform

parameters:
  topic:
    type: string
    required: true
    description: 视频主题
  facts:
    type: array
    required: false
    description: 知识事实材料
  style:
    type: string
    required: false
  targetDurationSec:
    type: number
    required: false

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

promptRef: prompts/video_script_generator.prompt.md
resourceRefs:
  - rules/global.md
  - rules/script_rules.md
  - references/knowledge_video_style.md
  - examples/example_script.md

costLevel: low
riskLevel: low
sideEffect: false
idempotent: true

approvalPolicy:
  required: true
  mode: after_artifact
  blocksDownstream: true
  reason: 口播稿会作为分镜和视频 Prompt 的基础，必须用户确认后继续。
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

# 13. Skill Capability 设计

Skill 不再变成 Workflow，而是变成 Agent 能力包。

## 13.1 目录结构

```text id="yfm99x"
cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/
├── skillcap.yaml
├── rules/
├── prompts/
├── references/
├── schemas/
├── tools/
├── examples/
└── recipe.yaml
```

## 13.2 `skillcap.yaml`

```yaml id="bco45i"
id: codex-video-skill
name: Codex 视频创作能力包
version: 1.0.0
domain: video_creation
description: 用于根据自然语言需求生成口播稿、分镜、视频 Prompt 和发布文案。

activation:
  intents:
    - video_creation
    - knowledge_sharing
    - oral_script_video
  keywords:
    - 视频
    - 口播
    - 分镜
    - prompt
    - 知识分享
    - 小红书
    - 短视频

tools:
  - id: knowledge_researcher
    manifest: tools/knowledge_researcher.tool.yaml
  - id: video_script_generator
    manifest: tools/video_script_generator.tool.yaml
  - id: shot_splitter
    manifest: tools/shot_splitter.tool.yaml
  - id: video_prompt_generator
    manifest: tools/video_prompt_generator.tool.yaml
  - id: publish_copy_generator
    manifest: tools/publish_copy_generator.tool.yaml
  - id: video_package_exporter
    manifest: tools/video_package_exporter.tool.yaml

recipe:
  path: recipe.yaml
  mode: advisory
```

`recipe.yaml` 只作为 LLM Planner 的参考，不固化执行。

---

# 14. API 设计

## 14.1 创建动态 Agent Run

```http id="e8z1u9"
POST /api/agent/runs
```

请求：

```json id="53xznv"
{
  "message": "请帮我根据端午节的来历创作一个口播知识分享视频。",
  "domain": "video_creation",
  "context": {
    "platform": "小红书",
    "aspectRatio": "16:9",
    "language": "zh-CN",
    "targetDurationSec": 90
  },
  "executionMode": "dynamic_agent",
  "plannerType": "llm"
}
```

返回：

```json id="0ij11w"
{
  "runId": "agent_run_001",
  "taskId": "task_001",
  "status": "RUNNING",
  "plan": {}
}
```

---

## 14.2 查询 Agent Run

```http id="1ezhw1"
GET /api/agent/runs/:runId
```

返回：

```json id="ngojc3"
{
  "runId": "agent_run_001",
  "taskId": "task_001",
  "status": "PAUSED",
  "currentReview": {
    "reviewId": "review_001",
    "artifactName": "端午节事实整理.md",
    "reason": "知识类视频的事实基础需要用户确认后再进入脚本生成。"
  },
  "steps": []
}
```

---

## 14.3 审核列表

```http id="f3hcrk"
GET /api/agent/runs/:runId/reviews
```

---

## 14.4 审核通过

```http id="2cglfb"
POST /api/agent/runs/:runId/reviews/:reviewId/approve
```

内部动作：

```text id="zfhmv2"
artifact_reviews.status = APPROVED
对应 CONTROL node → SUCCESS
任务恢复
下游节点进入 READY
```

---

## 14.5 审核驳回

```http id="a4o6g3"
POST /api/agent/runs/:runId/reviews/:reviewId/reject
```

请求：

```json id="bmmsv2"
{
  "comment": "内容太泛，需要增加屈原与民俗演变之间的关系。",
  "action": "revise"
}
```

内部动作：

```text id="ppjymh"
记录 REJECTED
触发局部 replan 或重跑上游工具
```

---

# 15. ModelGateway 接入

架构文档里已经把统一 Model Gateway 作为核心优势描述，目标是所有模型调用走缓存、重试和 Provider 路由。([GitHub][2])

LLM Planner 必须走 ModelGateway，而不是直接散落调用 LLMClient。

建议模型分层：

| 任务              | 模型策略                    |
| --------------- | ----------------------- |
| IntentRouter    | 小模型                     |
| ToolRetriever   | 规则/embedding，尽量不用 LLM   |
| LLMPlanner      | 中模型，temperature 0.1～0.3 |
| PromptTool 内容生成 | 按工具配置选择模型               |
| JSON 修复         | 小模型                     |
| Final Summary   | 中模型                     |

---

# 16. 端午节示例完整链路

用户输入：

```text id="8htdck"
请帮我根据端午节的来历创作一个口播知识分享视频。
```

## 16.1 ToolRetriever 返回

```text id="12j4pm"
knowledge_researcher
fact_checker
video_script_generator
shot_splitter
video_prompt_generator
publish_copy_generator
video_package_exporter
```

## 16.2 LLMPlanner 输出

```json id="tzssgg"
{
  "goal": "根据端午节的来历创作一个口播知识分享视频",
  "domain": "video_creation",
  "mode": "dynamic_agent",
  "steps": [
    {
      "id": "knowledge_research",
      "intent": "整理端午节来历、关键事实、常见习俗和讲述角度",
      "tool": "knowledge_researcher",
      "arguments": {
        "topic": "端午节的来历",
        "outputStyle": "适合大众口播知识分享视频"
      },
      "expectedOutput": ["facts", "storyAngles", "risks"],
      "produceArtifact": true
    },
    {
      "id": "fact_check",
      "intent": "检查端午节来历相关事实是否稳妥",
      "tool": "fact_checker",
      "dependsOn": ["knowledge_research"],
      "arguments": {
        "facts": "{{knowledge_research.output.facts}}",
        "topic": "端午节的来历"
      },
      "expectedOutput": ["checkedFacts", "warnings"],
      "produceArtifact": true
    },
    {
      "id": "script_generation",
      "intent": "生成口播知识分享视频脚本",
      "tool": "video_script_generator",
      "dependsOn": ["fact_check"],
      "arguments": {
        "topic": "端午节的来历",
        "facts": "{{fact_check.output.checkedFacts}}",
        "style": "真诚、有知识感、适合中文短视频平台",
        "targetDurationSec": 90
      },
      "expectedOutput": ["script", "summary"],
      "produceArtifact": true
    },
    {
      "id": "shot_split",
      "intent": "把口播稿拆成分镜",
      "tool": "shot_splitter",
      "dependsOn": ["script_generation"],
      "arguments": {
        "script": "{{script_generation.output.script}}",
        "shotDurationRule": "3-15秒",
        "aspectRatio": "16:9"
      },
      "expectedOutput": ["shotList"],
      "produceArtifact": true
    },
    {
      "id": "video_prompt_generation",
      "intent": "生成每个分镜的视频生成 Prompt",
      "tool": "video_prompt_generator",
      "dependsOn": ["shot_split"],
      "arguments": {
        "shotList": "{{shot_split.output.shotList}}",
        "style": "非写实动画，去AI感，中文知识分享视频，16:9"
      },
      "expectedOutput": ["videoPrompts"],
      "produceArtifact": true
    },
    {
      "id": "package_export",
      "intent": "导出完整视频创作包",
      "tool": "video_package_exporter",
      "dependsOn": [
        "script_generation",
        "shot_split",
        "video_prompt_generation"
      ],
      "arguments": {
        "script": "{{script_generation.output.script}}",
        "shotList": "{{shot_split.output.shotList}}",
        "videoPrompts": "{{video_prompt_generation.output.videoPrompts}}"
      },
      "expectedOutput": ["package"],
      "produceArtifact": true
    }
  ],
  "budget": {
    "maxSteps": 8,
    "maxToolCalls": 8,
    "maxLLMCalls": 6,
    "maxReplans": 1,
    "maxCostLevel": "medium"
  },
  "stopPolicy": {
    "stopWhenEnough": true
  }
}
```

## 16.3 Compiler 生成临时 DAG

```text id="ziflmw"
knowledge_research_exec TOOL
  ↓
knowledge_research_review CONTROL
  ↓
fact_check_exec TOOL
  ↓
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
  ↓
package_export_exec TOOL
  ↓
package_export_review CONTROL
```

## 16.4 用户审核节奏

| 产物        | 是否暂停 |
| --------- | ---- |
| 事实整理      | 是    |
| 口播稿       | 是    |
| 分镜        | 是    |
| 视频 Prompt | 是    |
| 最终包       | 是    |

---

# 17. 前端升级

新增：

```text id="5p2jrq"
AgentRunPanel
ReviewQueue
AgentPlanPreview
AgentTraceTimeline
ArtifactReviewModal
```

页面流程：

```text id="g4r9qa"
用户输入一句话
  ↓
POST /api/agent/runs
  ↓
展示 AgentPlan
  ↓
展示执行时间线
  ↓
遇到 CONTROL 节点显示待审核产物
  ↓
用户通过 / 驳回 / 修改意见
  ↓
继续执行
```

旧的 `workflow-runs` 页面可保留，但新视频创作入口应默认走 `/api/agent/runs`。

---

# 18. 给编码 Agent 的完整执行任务

下面这段可以直接交给你的编码 Agent。

```text id="wcoaid"
目标：
将 tangying-ai-operation-system 的视频创作主路径从固定 Skill Workflow 升级为 LLM Planner 驱动的 dynamic_agent。保留 Orchestrator、Worker、Artifact、CONTROL 节点能力，但不再把新视频创作流程写入 workflow_templates。

任务 1：扩展 ToolManifest
- 修改 cloud-backend/internal/core/worker/tool/manifest.go
- 增加 capabilities、tags、costLevel、latencyLevel、riskLevel、sideEffect、idempotent、approvalPolicy、artifactPolicy、nextRecommendedTools、failureModes、skillPackageId、promptRef、resourceRefs。
- 修改 ToolManifestRecord、repository、database migration、manifestToRecord。
- 保证旧工具默认兼容：costLevel=low、riskLevel=low、approvalPolicy.required=false。

任务 2：新增 agentruntime 包
- 新增 cloud-backend/internal/core/agentruntime/
- 定义 AgentPlan、AgentStep、PlanRequest、AgentBudget。
- 定义 Planner 接口。
- 保留 HeuristicPlanner，实现 Planner。
- 新增 LLMPlanner，实现 Planner。
- LLMPlanner 只输出 AgentPlan JSON，不允许输出 DAG。

任务 3：实现 ToolRetriever
- 新增 ToolRetriever。
- 从 ToolManifestService.ListAll 读取工具。
- 根据 capabilities、tags、description、costLevel、riskLevel 做 TopK 检索。
- LLMPlanner 只接收 TopK 工具，不再使用全量 FormatForPrompt。

任务 4：实现 LLMPlanner
- 输入：用户需求、context、TopK tools、budget、skill capability hints。
- 输出：AgentPlan JSON。
- 支持 JSON Schema。
- 模型输出如果包裹自然语言，提取 JSON。
- JSON 解析失败时最多修复一次。
- LLMPlanner 失败时可配置 fallback_to_heuristic。

任务 5：实现 PlanGuard
- 校验工具存在。
- 校验工具属于本次允许工具集合。
- 校验 arguments 满足 parameters。
- 校验 required 参数。
- 校验 dependsOn 存在。
- 校验无环。
- 校验 {{step.output.field}} 引用合法。
- 校验成本、风险、副作用。
- sideEffect=true 且无 approvalPolicy 的工具禁止执行。

任务 6：实现 PlanCompiler
- AgentPlan 编译成 model.DAGRequest。
- DAG 只作为 transient DAG 提交 Orchestrator，不写 workflow_templates。
- 根据 ToolManifest.approvalPolicy 自动插入 CONTROL 节点。
- after_artifact: TOOL -> CONTROL。
- before_execute: CONTROL -> TOOL。
- before_side_effect: CONTROL -> TOOL。
- always: CONTROL -> TOOL -> CONTROL。

任务 7：实现 Artifact Review
- 新增 artifact_reviews 表。
- 工具生成 artifact 后，如果 artifactPolicy.defaultReviewRequired=true，则创建 PENDING review。
- CONTROL 节点 READY 后任务暂停。
- approve 后 artifact_reviews.status=APPROVED，并将 CONTROL node 标记 SUCCESS。
- reject 后记录 REJECTED，支持终止或局部 replan。

任务 8：新增 API
- POST /api/agent/runs
- GET /api/agent/runs/:runId
- GET /api/agent/runs/:runId/reviews
- POST /api/agent/runs/:runId/reviews/:reviewId/approve
- POST /api/agent/runs/:runId/reviews/:reviewId/reject
- POST /api/agent/runs/:runId/reviews/:reviewId/revise

任务 9：新增 skillcapability 包
- 支持 cloud-backend/skill-capabilities/**/skillcap.yaml。
- Skill 不再编译为 Workflow。
- skillcap.yaml 注册 tools/*.tool.yaml 为 ToolManifest。
- 支持 promptRef、resourceRefs。
- recipe.yaml 只作为 LLMPlanner 参考，不固化执行。

任务 10：注册视频创作核心工具
- knowledge_researcher
- fact_checker
- video_script_generator
- shot_splitter
- video_prompt_generator
- publish_copy_generator
- video_package_exporter
- 这些工具必须提供 ToolManifest、approvalPolicy、artifactPolicy 和 nextRecommendedTools。

任务 11：接入 ModelGateway
- LLMPlanner、PromptTool、JSONRepairer 都通过 ModelGateway 调用模型。
- 支持不同任务使用不同模型。
- 记录 planner prompt、model、token、耗时和错误。

任务 12：前端改造
- 新增 AgentRunPanel。
- 新增 ReviewQueue。
- 新增 AgentPlanPreview。
- 新增 ArtifactReviewModal。
- 视频创作入口默认调用 POST /api/agent/runs，而不是 workflow-runs。
- 老 workflow 页面保留兼容。

验收用例：
输入：请帮我根据端午节的来历创作一个口播知识分享视频。
期望：
1. 系统调用 /api/agent/runs。
2. LLMPlanner 输出 AgentPlan，而不是 DAG。
3. AgentPlan 包含 knowledge_researcher、video_script_generator、shot_splitter、video_prompt_generator、video_package_exporter。
4. Guard 校验通过。
5. Compiler 生成 transient DAG。
6. 不写 workflow_templates。
7. Orchestrator 执行 ai_task / ai_node。
8. 事实整理、口播稿、分镜、视频 Prompt 都生成审核节点。
9. 未审核通过的产物不能进入下游。
10. 最终导出完整视频创作包。
```

---

# 19. 最终验收标准

## 必须满足

```text id="ywu0m2"
1. 用户一句话可以创建 AgentRun。
2. LLMPlanner 生成 AgentPlan JSON。
3. 系统不再让 LLM 直接生成 DAG。
4. 系统不再把新视频任务写入 workflow_templates。
5. ToolRetriever 只给 LLM TopK 工具。
6. PlanGuard 能拦截非法工具和非法参数。
7. PlanCompiler 能自动插入 CONTROL 节点。
8. 中间产物必须审核通过才能进入下游。
9. 视频创作结果包含：
   - 知识事实整理
   - 口播稿
   - 分镜
   - 视频 Prompt
   - 发布标题/简介
   - 最终创作包
```

## 不合格信号

```text id="1pv1jx"
1. 仍然调用 /api/skills/route 作为新视频主入口。
2. 仍然调用 /api/video-projects/:id/workflow-runs。
3. LLM 直接输出 DAGRequest。
4. LLM Prompt 里塞全量工具。
5. 工具没有 approvalPolicy。
6. 口播稿未审核就进入分镜。
7. 分镜未审核就进入视频 Prompt。
8. 视频生成/发布类工具没有 before_side_effect 审核。
```

---

# 20. 最终结论

你下一步升级的核心不是继续加强 Workflow，而是新增一层：

```text id="ty9us7"
Agent Runtime
```

最终架构应该变成：

```text id="u4xfcw"
Skill Capability 提供能力资产
ToolManifest 描述可调用工具
LLMPlanner 选择和组合工具
PlanGuard 负责安全校验
PlanCompiler 生成临时 DAG
Orchestrator 负责任务调度
Worker 负责真实工具调用
Artifact 负责中间产物
CONTROL 负责人工审核
```

一句话：

> **让 LLM 负责“想怎么做”，让系统负责“能不能这么做、怎么安全执行、什么时候暂停给用户审核”。**