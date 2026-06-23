# 1. 最终调用链路

升级后链路如下：

```text id="6unfug"
用户需求
  ↓
ToolRetriever 检索 TopK 工具
  ↓
LLMPlanner 生成 AgentPlan JSON
  ↓
PlanGuard 校验 AgentPlan
  ↓
PlanCompiler 编译成临时 DAG
  ↓
Orchestrator 创建 ai_task / ai_node
  ↓
Worker 调用真实工具
  ↓
CONTROL 节点处理人工审核
```

关键点：

> **LLM 不直接调用工具。LLM 只输出“应该调用哪些工具、参数是什么、依赖关系是什么”的 JSON 计划。真正调用工具的是 Worker。**

---

# 2. Planner 接口保持不变

先定义统一接口：

```go id="hkkyw6"
type Planner interface {
    Plan(ctx context.Context, req PlanRequest) (*AgentPlan, error)
}
```

请求结构：

```go id="t68mfo"
type PlanRequest struct {
    UserID      string                 `json:"userId"`
    Message     string                 `json:"message"`
    Domain      string                 `json:"domain,omitempty"`
    Context     map[string]interface{} `json:"context,omitempty"`
    Budget      AgentBudget            `json:"budget"`
}
```

返回结构：

```go id="kzqdfc"
type AgentPlan struct {
    Goal       string      `json:"goal"`
    Domain     string      `json:"domain,omitempty"`
    Mode       string      `json:"mode"`
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

# 3. HeuristicPlanner 和 LLMPlanner 的区别

## 当前启发式 Planner

它大概是这样：

```go id="d00sr4"
func (p *HeuristicPlanner) Plan(ctx context.Context, req PlanRequest) (*AgentPlan, error) {
    plan := &AgentPlan{
        Goal: req.Message,
        Mode: "dynamic_agent",
        Steps: []AgentStep{},
    }

    if strings.Contains(req.Message, "视频") {
        plan.Steps = append(plan.Steps, AgentStep{
            ID:   "script_generation",
            Tool: "video_script_generator",
            Arguments: map[string]interface{}{
                "topic": req.Message,
                "platform": "小红书",
            },
            ProduceArtifact: true,
        })
    }

    if strings.Contains(req.Message, "分镜") || strings.Contains(req.Message, "视频") {
        plan.Steps = append(plan.Steps, AgentStep{
            ID:        "shot_split",
            Tool:      "shot_splitter",
            DependsOn: []string{"script_generation"},
            Arguments: map[string]interface{}{
                "script": "{{script_generation.output.script}}",
            },
            ProduceArtifact: true,
        })
    }

    return plan, nil
}
```

特点：

```text id="6v5f0w"
靠关键词和固定规则生成 AgentPlan
```

---

## LLMPlanner

LLMPlanner 是这样：

```text id="frxp3c"
用户需求
  ↓
检索相关工具 TopK
  ↓
把工具清单、预算、上下文给 LLM
  ↓
要求 LLM 输出 AgentPlan JSON
  ↓
解析 JSON
  ↓
返回 AgentPlan
```

示例输出：

```json id="q0dqzy"
{
  "goal": "生成小红书视频创作包",
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
        "style": "中文平台，真诚，有价值"
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
        "script": "{{script_generation.output.script}}"
      },
      "expectedOutput": ["shotList"],
      "produceArtifact": true
    },
    {
      "id": "video_prompt_generation",
      "intent": "生成视频 Prompt",
      "tool": "video_prompt_generator",
      "dependsOn": ["shot_split"],
      "arguments": {
        "shotList": "{{shot_split.output.shotList}}"
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

# 4. LLMPlanner 代码结构

建议新增：

```text id="f0irwd"
cloud-backend/internal/core/agentruntime/
├── planner.go
├── heuristic_planner.go
├── llm_planner.go
├── planner_prompt.go
├── planner_schema.go
├── tool_retriever.go
└── json_repair.go
```

---

## 4.1 `planner.go`

```go id="bl0urg"
package agentruntime

import "context"

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

---

## 4.2 `llm_planner.go`

```go id="xl1kya"
package agentruntime

import (
    "context"
    "encoding/json"
    "fmt"
)

type LLMPlanner struct {
    modelClient   ModelClient
    toolRetriever ToolRetriever
    repairer      JSONRepairer
}

func NewLLMPlanner(
    modelClient ModelClient,
    toolRetriever ToolRetriever,
    repairer JSONRepairer,
) *LLMPlanner {
    return &LLMPlanner{
        modelClient:   modelClient,
        toolRetriever: toolRetriever,
        repairer:      repairer,
    }
}

func (p *LLMPlanner) Plan(ctx context.Context, req PlanRequest) (*AgentPlan, error) {
    tools, err := p.toolRetriever.Retrieve(ctx, ToolRetrieveRequest{
        Query:        req.Message,
        Domain:       req.Domain,
        TopK:         8,
        MaxCostLevel: req.Budget.MaxCostLevel,
    })
    if err != nil {
        return nil, fmt.Errorf("retrieve tools: %w", err)
    }

    if len(tools) == 0 {
        return nil, fmt.Errorf("no tools available for request")
    }

    prompt := BuildPlannerPrompt(req, tools)

    raw, err := p.modelClient.GenerateJSON(ctx, ModelJSONRequest{
        SystemPrompt: PlannerSystemPrompt,
        UserPrompt:   prompt,
        JSONSchema:   AgentPlanJSONSchema,
        Temperature:  0.2,
    })
    if err != nil {
        return nil, fmt.Errorf("llm planner call failed: %w", err)
    }

    plan, err := parseAgentPlan(raw)
    if err != nil {
        repaired, repairErr := p.repairer.Repair(ctx, raw, AgentPlanJSONSchema)
        if repairErr != nil {
            return nil, fmt.Errorf("parse plan failed: %w; repair failed: %v", err, repairErr)
        }

        plan, err = parseAgentPlan(repaired)
        if err != nil {
            return nil, fmt.Errorf("parse repaired plan failed: %w", err)
        }
    }

    normalizePlan(plan, req)

    return plan, nil
}

func parseAgentPlan(raw string) (*AgentPlan, error) {
    var plan AgentPlan
    if err := json.Unmarshal([]byte(raw), &plan); err != nil {
        return nil, err
    }
    if len(plan.Steps) == 0 {
        return nil, fmt.Errorf("agent plan has empty steps")
    }
    return &plan, nil
}

func normalizePlan(plan *AgentPlan, req PlanRequest) {
    if plan.Goal == "" {
        plan.Goal = req.Message
    }
    if plan.Mode == "" {
        plan.Mode = "dynamic_agent"
    }
    if plan.Domain == "" {
        plan.Domain = req.Domain
    }
    if plan.Budget.MaxSteps == 0 {
        plan.Budget = req.Budget
    }
}
```

---

# 5. ModelClient 适配

你现在已有 `PlanService` 调用 LLM 并用 `ChatWithJSON` 解析结果的方式。当前代码会调用 `llmClient.ChatWithJSON(ctx, llmMessages, &dag)` 让模型直接生成 `DAGRequest`。([GitHub][1])

LLMPlanner 可以先复用类似能力，但目标对象从 `DAGRequest` 换成 `AgentPlan`。

定义接口：

```go id="iamyts"
type ModelClient interface {
    GenerateJSON(ctx context.Context, req ModelJSONRequest) (string, error)
}

type ModelJSONRequest struct {
    SystemPrompt string
    UserPrompt   string
    JSONSchema   string
    Temperature  float64
}
```

如果你当前还没有统一 `ModelGateway` 接口，可以先包一层现有 `LLMClient`：

```go id="y0o79h"
type LLMClientAdapter struct {
    client *LLMClient
}

func (a *LLMClientAdapter) GenerateJSON(ctx context.Context, req ModelJSONRequest) (string, error) {
    messages := []map[string]interface{}{
        {
            "role": "system",
            "content": req.SystemPrompt,
        },
        {
            "role": "user",
            "content": req.UserPrompt,
        },
    }

    var raw json.RawMessage
    if err := a.client.ChatWithJSON(ctx, messages, &raw); err != nil {
        return "", err
    }

    return string(raw), nil
}
```

更推荐的最终形态：

```text id="xd8pmz"
LLMPlanner → modelgateway → user configured provider / cloud default provider
```

但第一版可以先接现有 `LLMClient`，减少改动。

---

# 6. Planner Prompt 设计

## 6.1 System Prompt

```go id="cl4mlq"
const PlannerSystemPrompt = `
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
8. 如果用户需求不完整，只规划可以完成的步骤，不要编造用户未提供的关键输入。
9. 工具是否需要人工审核由系统根据 ToolManifest 自动处理，你不需要插入审核步骤。
10. 输出必须是 AgentPlan JSON。
`
```

---

## 6.2 User Prompt

```go id="wt5ak1"
func BuildPlannerPrompt(req PlanRequest, tools []ToolForPlanner) string {
    toolsJSON, _ := json.MarshalIndent(tools, "", "  ")
    ctxJSON, _ := json.MarshalIndent(req.Context, "", "  ")
    budgetJSON, _ := json.MarshalIndent(req.Budget, "", "  ")

    return fmt.Sprintf(`
用户需求：
%s

任务领域：
%s

上下文：
%s

预算：
%s

可用工具列表：
%s

请基于以上信息输出 AgentPlan JSON。
`, req.Message, req.Domain, string(ctxJSON), string(budgetJSON), string(toolsJSON))
}
```

---

# 7. 给 LLM 的工具描述不要太长

你当前 `ToolManifest` 包含基础字段：`Name`、`Description`、`Type`、`Endpoint`、`Timeout`、`Parameters`、`Output`、`Sandbox`、`Examples` 等。([GitHub][2])

给 Planner 的工具 DTO 要精简，不能把所有字段都塞进去。

```go id="i0dqe0"
type ToolForPlanner struct {
    Name                 string                 `json:"name"`
    Description          string                 `json:"description"`
    Capabilities         []string               `json:"capabilities,omitempty"`
    Parameters           map[string]ParamDef    `json:"parameters"`
    Output               map[string]ParamDef    `json:"output"`
    CostLevel            string                 `json:"costLevel,omitempty"`
    RiskLevel            string                 `json:"riskLevel,omitempty"`
    SideEffect           bool                   `json:"sideEffect,omitempty"`
    NextRecommendedTools []string               `json:"nextRecommendedTools,omitempty"`
}
```

不要给 LLM：

```text id="8elgw5"
endpoint
author
registeredAt
sandbox 细节
内部执行路径
完整 examples 大文本
```

这些留给系统执行，不给 Planner。

---

# 8. ToolRetriever

LLMPlanner 不应该拿全量工具。应该先检索 TopK。

```go id="qocv1u"
type ToolRetriever interface {
    Retrieve(ctx context.Context, req ToolRetrieveRequest) ([]ToolForPlanner, error)
}

type ToolRetrieveRequest struct {
    Query        string
    Domain       string
    TopK         int
    MaxCostLevel string
}
```

MVP 可以用规则检索：

```go id="arj7u4"
func (r *KeywordToolRetriever) Retrieve(ctx context.Context, req ToolRetrieveRequest) ([]ToolForPlanner, error) {
    manifests, err := r.toolManifestSvc.ListEnabled(ctx)
    if err != nil {
        return nil, err
    }

    scored := make([]ScoredTool, 0)

    for _, m := range manifests {
        if !costAllowed(m.CostLevel, req.MaxCostLevel) {
            continue
        }

        score := 0
        text := strings.ToLower(m.Name + " " + m.Description + " " + strings.Join(m.Capabilities, " "))

        for _, token := range tokenize(req.Query) {
            if strings.Contains(text, strings.ToLower(token)) {
                score += 10
            }
        }

        if req.Domain != "" && contains(m.Capabilities, req.Domain) {
            score += 20
        }

        if score > 0 {
            scored = append(scored, ScoredTool{
                Tool:  ToToolForPlanner(m),
                Score: score,
            })
        }
    }

    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score > scored[j].Score
    })

    return topK(scored, req.TopK), nil
}
```

后续再升级为：

```text id="cmbqha"
capability tags + embedding + Qdrant
```

---

# 9. AgentPlan JSON Schema

建议写成常量：

```go id="m4ci86"
const AgentPlanJSONSchema = `
{
  "type": "object",
  "required": ["goal", "mode", "steps"],
  "properties": {
    "goal": {
      "type": "string"
    },
    "domain": {
      "type": "string"
    },
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
          "intent": {
            "type": "string"
          },
          "tool": {
            "type": "string"
          },
          "arguments": {
            "type": "object"
          },
          "dependsOn": {
            "type": "array",
            "items": {
              "type": "string"
            }
          },
          "expectedOutput": {
            "type": "array",
            "items": {
              "type": "string"
            }
          },
          "produceArtifact": {
            "type": "boolean"
          }
        }
      }
    },
    "budget": {
      "type": "object"
    },
    "stopPolicy": {
      "type": "object"
    }
  }
}
`
```

如果模型 API 支持 JSON Schema / structured output，就把这个 schema 传进去。
如果模型 API 不支持，就把 schema 放在 prompt 里，并做 JSON 修复。

---

# 10. JSON 修复机制

LLM 有时会输出：

````text id="rlp5ao"
下面是 JSON：
```json
{...}
````

````

所以需要修复器。

```go id="l73vcg"
type JSONRepairer interface {
    Repair(ctx context.Context, raw string, schema string) (string, error)
}
````

MVP 可以先做本地提取：

```go id="1hbz3c"
func ExtractJSONObject(raw string) (string, error) {
    start := strings.Index(raw, "{")
    end := strings.LastIndex(raw, "}")
    if start < 0 || end < 0 || end <= start {
        return "", fmt.Errorf("no json object found")
    }
    return raw[start : end+1], nil
}
```

流程：

```go id="l0z9wa"
jsonText, err := ExtractJSONObject(raw)
if err == nil {
    plan, err := parseAgentPlan(jsonText)
    if err == nil {
        return plan, nil
    }
}
```

如果还失败，再调用 cheap model 修复一次，不要无限修复。

---

# 11. PlanGuard 必须保留

LLMPlanner 生成的 AgentPlan 不能直接执行。

Guard 必须校验：

```text id="g0cnxy"
1. 工具是否存在
2. 工具是否在本次 TopK 或允许工具集合中
3. 参数是否满足 ToolManifest.Parameters
4. dependsOn 是否引用存在 step
5. 是否有循环依赖
6. {{step.output.field}} 是否引用上游 step
7. 是否超过预算 maxSteps / maxToolCalls / maxCostLevel
8. 是否使用高风险工具
9. 有副作用工具是否具备 approvalPolicy
10. step.id 是否合法
```

即使 LLM 编造工具：

```json id="55lyd2"
{
  "tool": "super_video_generator"
}
```

Guard 也要拦截。

---

# 12. PlanCompiler 不需要大改

Planner 输出 AgentPlan 后，Compiler 仍然做：

```text id="oid1xp"
AgentPlan → DAGRequest
```

并根据 ToolManifest 自动插入审核节点。

你当前 Orchestrator 已经支持 `CONTROL` 节点进入 READY 时自动暂停任务，用于人工审核。([GitHub][3])

编译规则：

```text id="89phnb"
普通工具：
step_exec TOOL

after_artifact：
step_exec TOOL → step_review CONTROL

before_execute：
step_review CONTROL → step_exec TOOL

before_side_effect：
step_review CONTROL → step_exec TOOL

always：
step_pre_review CONTROL → step_exec TOOL → step_post_review CONTROL
```

---

# 13. Worker 如何调用工具

LLMPlanner 生成：

```json id="czy780"
{
  "tool": "video_script_generator",
  "arguments": {
    "topic": "AI替代的不是岗位，而是整套工作流程",
    "platform": "小红书"
  }
}
```

Compiler 变成 DAG 节点：

```json id="k1552a"
{
  "id": "script_generation_exec",
  "type": "TOOL",
  "name": "external",
  "input": {
    "tool": "video_script_generator",
    "parameters": {
      "topic": "AI替代的不是岗位，而是整套工作流程",
      "platform": "小红书"
    }
  }
}
```

Worker 执行：

```text id="h6yv7n"
读取 node.input.tool
  ↓
从 ToolRegistry 找 video_script_generator
  ↓
解析 node.input.parameters
  ↓
调用工具实现
  ↓
保存 output / artifact
  ↓
节点 SUCCESS
```

所以：

> **LLMPlanner 不调用工具。Compiler 把 LLM 的计划翻译成节点。Worker 根据节点真正调用工具。**

---

# 14. 配置开关

不要直接替换线上 Planner。加配置：

```yaml id="6tbxfm"
agent:
  planner:
    type: heuristic # heuristic / llm
    llm:
      model: qwen-plus
      temperature: 0.2
      top_k_tools: 8
      max_repair_attempts: 1
      fallback_to_heuristic: true
```

创建 Planner：

```go id="otm5zf"
func NewPlanner(cfg Config, deps PlannerDeps) Planner {
    switch cfg.Agent.Planner.Type {
    case "llm":
        llm := NewLLMPlanner(deps.ModelClient, deps.ToolRetriever, deps.JSONRepairer)
        if cfg.Agent.Planner.LLM.FallbackToHeuristic {
            return NewFallbackPlanner(llm, deps.HeuristicPlanner)
        }
        return llm
    default:
        return deps.HeuristicPlanner
    }
}
```

FallbackPlanner：

```go id="kxbzp0"
type FallbackPlanner struct {
    primary  Planner
    fallback Planner
}

func (p *FallbackPlanner) Plan(ctx context.Context, req PlanRequest) (*AgentPlan, error) {
    plan, err := p.primary.Plan(ctx, req)
    if err == nil {
        return plan, nil
    }
    return p.fallback.Plan(ctx, req)
}
```

---

# 15. 落地顺序

## 阶段 1：抽象 Planner 接口

目标：

```text id="xg6dcq"
确认当前 HeuristicPlanner 实现 Planner 接口
```

验收：

```text id="wm9yd3"
使用 heuristic 模式，现有功能不变
Guard、Compiler、Orchestrator、Worker 正常执行
```

---

## 阶段 2：实现 ToolRetriever

目标：

```text id="lts6g9"
LLMPlanner 只拿 TopK 工具，不拿全量工具
```

验收：

```text id="t5glj1"
输入“生成视频脚本分镜”
返回 video_script_generator / shot_splitter / video_prompt_generator
```

---

## 阶段 3：实现 LLMPlanner

目标：

```text id="nxgk87"
LLM 输出 AgentPlan JSON
```

验收：

```text id="5mvlh2"
LLMPlanner.Plan() 能返回合法 AgentPlan
不直接返回 DAG
```

---

## 阶段 4：接入 PlanGuard

目标：

```text id="fmes3h"
拦截 LLM 错误计划
```

验收：

```text id="0e0sad"
LLM 编造工具时失败
LLM 参数缺失时失败
LLM 循环依赖时失败
超预算工具失败
```

---

## 阶段 5：灰度开启

配置：

```yaml id="sshg2n"
agent:
  planner:
    type: llm
    llm:
      fallback_to_heuristic: true
```

验收：

```text id="jum9ao"
LLMPlanner 成功时走 LLM
LLMPlanner 失败时自动回退 heuristic
```

---

# 16. 最小测试用例

## 用例 1：视频创作

输入：

```text id="50bdzv"
帮我把“AI替代的不是岗位，而是整套工作流程”做成小红书视频创作包
```

期望 AgentPlan：

```text id="ubdw8s"
video_script_generator
  ↓
shot_splitter
  ↓
video_prompt_generator
```

---

## 用例 2：只要标题

输入：

```text id="m5uzwk"
帮我给这个视频生成10个小红书标题
```

期望：

```text id="iwin4p"
只调用 publish_copy_generator 或 title_generator
不要调用 shot_splitter
不要调用 video_prompt_generator
```

---

## 用例 3：工具不存在

LLM 输出：

```json id="f3mdml"
{
  "tool": "magic_video_tool"
}
```

期望：

```text id="16461v"
PlanGuard 拦截
不进入 Compiler
不创建 DAG
```

---

## 用例 4：审核门

如果 `video_script_generator.approvalPolicy.mode = after_artifact`：

期望 DAG：

```text id="b78oz6"
script_generation_exec TOOL
  ↓
script_generation_review CONTROL
```

---

# 17. 给编码 Agent 的任务描述

可以直接给 Codex / Claude Code：

```text id="nqw6j7"
目标：将当前确定性 HeuristicPlanner 升级为可配置的 LLMPlanner，但保持 Guard、Compiler、Orchestrator、Worker 调用链不变。

要求：
1. 抽象 Planner 接口：Plan(ctx, PlanRequest) (*AgentPlan, error)。
2. 保留现有 HeuristicPlanner 实现。
3. 新增 LLMPlanner，实现同一个 Planner 接口。
4. LLMPlanner 不直接生成 DAG，只生成 AgentPlan JSON。
5. LLMPlanner 先通过 ToolRetriever 检索 TopK 工具，再把工具清单传给模型。
6. Prompt 中要求模型只能输出 AgentPlan JSON，不能输出自然语言。
7. 支持 JSON Schema 约束；如果当前模型不支持 structured output，则实现 JSON 提取和一次修复。
8. LLMPlanner 输出后必须经过现有 PlanGuard。
9. PlanGuard 负责校验工具存在性、参数 schema、依赖关系、成本、风险和审核策略。
10. PlanCompiler 继续负责 AgentPlan → DAG，并根据 ToolManifest.approvalPolicy 自动插入 CONTROL 节点。
11. 新增配置 agent.planner.type = heuristic / llm。
12. LLMPlanner 失败时可配置 fallback_to_heuristic。
13. 添加单元测试：
    - LLM 正常输出 AgentPlan
    - LLM 输出自然语言包裹 JSON 时可修复
    - LLM 编造工具时被 Guard 拦截
    - 审核工具会自动插入 CONTROL 节点
```

---

# 18. 最终结论

升级成 LLMPlanner 的本质不是“让大模型直接操作系统”，而是：

```text id="rg2lxx"
让大模型输出结构化 AgentPlan
```

完整链路：

```text id="g2g8wg"
LLMPlanner：生成 AgentPlan JSON
PlanGuard：检查计划是否合法
PlanCompiler：转成 DAG
Orchestrator：调度节点
Worker：真正调用工具
CONTROL：处理人工审核
Artifact：保存中间产物
```
