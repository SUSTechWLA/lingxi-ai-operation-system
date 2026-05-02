# Skill：核心对话 Skill 模块集成（Translator 增强 + 会话编排）

## 一、目标与定位

本 Skill 用于在现有架构中新增 `internal/skill` 模块，向上提供多轮对话接口，向下复用现有 **Translator（NL→DAG）**、**Orchestrator（任务调度）**、**Worker（工具执行）**、**Context（审计）** 等能力，实现：

- 用户通过自然语言发起模糊创作需求
- 系统多轮补全必填信息
- 自动规划生成 DAG 并交付执行
- 整合结果返回完整自媒体内容包
- 支持自然语言修改与版本迭代

**Skill 层 = 有状态的会话壳**，它本身不重新实现工具调用或任务调度，而是编排现有模块完成闭环。

## 二、前置条件（执行前必须逐项确认）

1. `internal/translator/` 已实现基本的 `NL -> DAG` 转换能力，并可通过内部方法或接口调用（例如 `TranslateToDAG(userInput string) (*model.DAGRequest, error)`）。
2. `internal/orchestrator/` 已暴露 `SubmitTask(userID string, dag *model.DAGRequest) (taskID string, error)` 或同样的内部服务方法。
3. `ToolRegistry` 已注册所有可用工具（`llm_api`, `polisher`, `media_analyzer` 等），并能通过方法 `GetAllTools()` 获取元数据列表。
4. `PublishService`（或统一大模型调用封装）提供 `CallOpenAI(messages []Message, responseFormat JSONSchema) (string, error)` 能力。
5. Redis 可用，用于会话缓存 （`redis.Client` 可从 `internal/database/redis.go` 获取）。
6. `ai_context` 表已存在，且支持自定义 `context_type`（如 `SKILL_INTENT`）和 `source_module` 字段。
7. 前端 `PublishPage` 可新增或已有对话入口组件，或可接受简单的 Modal 弹窗。
8. 项目代码分层：handler/service/repository/model，所有新增代码严格遵循此规范。

## 三、实施原则（强制遵守）

1. **不修改现有模块核心逻辑**：仅通过新增包 `internal/skill` 来扩展，通过依赖注入调用现有服务。
2. **预设 DAG 模板优先**：高频场景（抖音全链路、小红书笔记等）直接加载 JSON 模板并注入参数，不走大模型规划，避免幻觉。
3. **完全复用现有接口**：工具调用交给 Orchestrator + Worker，大模型调用复用 `PublishService` 的封装，不做任何重复实现。
4. **全程可审计**：每个关键步骤（意图识别、计划生成、任务提交、任务完成）写入 `ai_context`，关联 `trace_id`。
5. **最小化前端改动**：通过 RESTful 会话接口与前端交互，前端只需按契约实现对话 UI。

## 四、模块目录结构

```
internal/skill/
├── handler/
│   ├── session_handler.go    # 会话创建、对话接口、进度查询、终止
│   └── routes.go              # 路由注册
├── service/
│   ├── intent_service.go     # 意图识别与需求结构化
│   ├── clarify_service.go    # 需求澄清与参数补全
│   ├── plan_service.go       # 执行计划生成（模板匹配 + 兜底NL→DAG）
│   ├── plan_validator.go     # DAG合法性校验
│   ├── result_assembler.go   # 结果整合
│   └── session_manager.go    # 会话上下文管理（Redis读写）
├── model/
│   └── types.go              # ConversationContext, StructuredDemand 等
├── templates/
│   └── default_templates.go  # 内嵌的JSON模板常量（或从配置文件加载）
└── prompts/
    └── prompts.go            # 意图识别、澄清、规划的 Prompt 模板常量
```

## 五、核心数据结构

```go
// internal/skill/model/types.go

type ConversationContext struct {
    SessionID         string                 `json:"session_id"`
    UserID            string                 `json:"user_id"`
    Status            string                 `json:"status"` // active/finished/terminated
    Messages          []ChatMessage          `json:"messages"`
    Demand            *StructuredDemand      `json:"demand"`
    ExecutePlan       *model.DAGRequest      `json:"execute_plan"` // 复用现有 DAG 定义
    TaskID            string                 `json:"task_id"`
    FinalResult       map[string]interface{} `json:"final_result"`
    Version           int                    `json:"version"`
    CreatedAt         time.Time              `json:"created_at"`
    UpdatedAt         time.Time              `json:"updated_at"`
}

type ChatMessage struct {
    Role       string `json:"role"`       // "user" / "assistant"
    Content    string `json:"content"`
    Actions    []Action `json:"actions,omitempty"` // 前端执行的动作
    Timestamp  time.Time `json:"timestamp"`
}

type Action struct {
    Type    string `json:"type"`    // "fill_title", "fill_description", "ask_media", ...
    Content string `json:"content"`
}

type StructuredDemand struct {
    CoreTarget       string                 `json:"core_target"`
    SceneID          string                 `json:"scene_id"`
    SceneName        string                 `json:"scene_name"`
    ExtractedParams  map[string]interface{} `json:"extracted_params"`
    MissingParams    []string               `json:"missing_params"`
    IsOutOfRange     bool                   `json:"is_out_of_range"`
}
```

## 六、详细实施步骤

### 阶段 1：模块骨架与路由注册

1. 创建上述目录结构。
2. 在 `internal/skill/handler/routes.go` 中定义路由并注册到全局路由：
   ```go
   func RegisterSkillRoutes(r *gin.RouterGroup, h *SessionHandler) {
       r.POST("/session/create", h.CreateSession)
       r.POST("/session/:session_id/chat", h.Chat)
       r.GET("/session/:session_id/progress", h.Progress)
       r.POST("/session/:session_id/terminate", h.Terminate)
   }
   ```
3. 在 `cmd/lingxi-ai-os/main.go` 中注入所有依赖并注册路由：
   ```go
   skillSvc := skill.NewService(
       orchestratorService,
       translatorService,
       toolRegistry,
       publishService, // 大模型调用
       redisClient,
   )
   skillHandler := skill.NewHandler(skillSvc)
   skill.RegisterSkillRoutes(apiRouter.Group("/api/skill/dialog"), skillHandler)
   ```

### 阶段 2：会话管理

1. **创建会话**：
   - 生成 UUID 作为 `session_id`。
   - 初始化 `ConversationContext`，将用户首次消息写入 `Messages`。
   - 保存到 Redis（key：`skill:session:{session_id}`），设置 7 天过期。
   - 返回 `session_id` 和初始回复（如欢迎语）。

2. **处理对话**：
   - 加载 Redis 中的上下文。
   - 将用户新消息追加到 `Messages`。
   - 交由逻辑引擎（见阶段 3-6）处理。
   - 更新上下文并写回 Redis。

3. **进度查询**：
   - 如果已关联 `TaskID`，则调用现有 `TraceHandler`（`/api/trace/{taskId}`）获取节点状态，返回给前端。

4. **终止会话**：
   - 将会话状态设置为 `terminated`，并持久化。

### 阶段 3：意图识别（Service）

1. 构建 Prompt（使用设计文档中的 NLU Prompt 模板），注入场景库和参数清单。
2. 调用 `PublishService.CallOpenAI(messages, responseFormat)`，要求返回 JSON。
3. 解析为 `StructuredDemand`。
4. 如果 `IsOutOfRange` 为 true，直接生成兜底话术，不执行后续步骤。
5. 将 `Demand` 更新到上下文。
6. 写入 `ai_context` 审计记录。

### 阶段 4：需求澄清（Service）

1. 检查 `Demand.MissingParams` 是否为空。
2. 若不为空，且当前会话澄清轮次 < 3：
   - 调用大模型生成一次性提问，Prompt 中包含缺失参数名称和其枚举选项。
   - 将问题返回给前端，等待用户回复。
   - 用户回复后，重新调用意图识别（仅提取参数），更新 `Demand.ExtractedParams`，重新检查完整性。
3. 若达到最大轮次或用户要求跳过，使用默认值填充所有缺失参数。

### 阶段 5：任务规划（生成 DAG）

1. **模板匹配**：
   - 定义预设模板 map：`map[string]string`，key 为 `scene_id`，value 为 DAG JSON 字符串。
   - 如果 `Demand.SceneID` 命中模板，则加载模板，使用 Go 的 `text/template` 将 `ExtractedParams` 中的值注入占位符（如 `{{platform}}`、`{{style}}`）。
   - 对于动态依赖占位符（如 `{{step1.output.title}}`），保留原样，由 Orchestrator 在运行时注入。

2. **兜底 NL→DAG**：
   - 如果没有模板，调用 `TranslationService.TranslateToDAG(“根据用户需求生成内容：…”)`，但在此之前需将结构化需求转为一句完整的自然语言描述。
   - 或者，构建一个专门用于规划的 Prompt，将工具列表和需求传入大模型，直接生成 DAG JSON。
   - 反序列化为 `*model.DAGRequest`。

3. **合法性校验**（`plan_validator.go`）：
   - 遍历所有节点，校验 `Type` 是否在 `ToolRegistry` 中。
   - 校验边的节点引用存在，无环。
   - 校验节点 `Input` 参数是否符合工具的 `ValidateParameters`（如果 Tool 接口提供方法）。
   - 校验失败重试最多 3 次，仍失败则返回错误并终止。

4. 将生成的 DAG 存入上下文。

### 阶段 6：提交 Orchestrator 执行

1. 调用 `OrchestratorService.SubmitTask(ctx, UserID, dag)` 创建任务。
2. 获得 `taskID` 并写入上下文。
3. 任务执行由现有 Orchestrator/Worker 全权处理，Skill 层不再干预。
4. 可启动一个监听函数（或利用现有事件），当任务状态变更为 `SUCCESS` 或 `FAILED` 时，触发结果整合。

### 阶段 7：结果整合与反馈优化

1. **结果整合**：
   - 通过 `TaskID` 查询 `ai_node` 表的输出，或直接从 Kafka 消费任务完成事件获得节点输出。
   - 按照场景模板定义的映射，组装成最终的内容包（标题、简介、标签、素材等）。
   - 将最终结果存入上下文。

2. **自然语言修改**：
   - 当用户提出修改要求时，识别修改目标（标题、简介等）。
   - 生成一个单节点 DAG（如仅执行 `polisher` 工具），指定输入为当前内容和修改指令。
   - 提交 Orchestrator 执行，得到优化后的字段，更新上下文中的 `FinalResult`。
   - 记录版本号。

3. **反馈**：前端接收 `Actions` 直接更新表单。

### 阶段 8：前端对接

前端只需实现一个对话 Modal：

- 调用 `POST /api/skill/dialog/session/create` 获取 `session_id`。
- 发送消息：`POST /api/skill/dialog/session/{session_id}/chat`。
- 接收响应中的 `reply` 和 `actions`，渲染消息，执行 `actions`（如填充标题）。
- 进度查询：轮询 `/api/skill/dialog/session/{session_id}/progress`，展示执行步骤。
- 结果采纳：点击“填入编辑区”，将内容映射到 `PublishPage` 的 Zustand store 中。

## 七、验收标准

1. 发送“帮我做一个宠物猫的抖音爆款视频”，经过反问补全信息后，能生成标题、口播文案、标签，并自动填入发布页。
2. 发送“标题再改得更有趣”，仅标题被优化，其余不动。
3. 新对话结束后，刷新页面重新进入，会话可恢复并查看历史。
4. 所有工具调用均来自已注册列表，日志无“未注册工具”错误。
5. 预设模板场景，大模型未被调用生成 DAG。
6. 系统原有功能不受任何影响。

## 八、注意事项

- 大模型调用务必设置超时和重试，避免会话卡死。
- 会话 Redis 存储大小限制为 1MB，超过后可裁剪早期消息。
- 模板渲染时，必须防止注入（Go 的 `text/template` 是安全的，但需注意参数值的合法性）。
- 结果整合时，如果存在异步工具结果尚未返回，需等待或返回部分结果。