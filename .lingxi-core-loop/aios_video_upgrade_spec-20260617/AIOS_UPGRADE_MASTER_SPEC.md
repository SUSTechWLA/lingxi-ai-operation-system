# AIOS 自媒体视频创作升级主规格

> 本文件为文档包合并版。Coding Agent 可先读本文件，再按章节引用独立文件。

## 目录

- 需求与范围
- 详细设计
- 实施计划
- 测试与验收
- Coding Agent 执行指令

# AIOS 自媒体视频创作能力升级——需求规格说明

## 1. 文档目的

本说明用于指导 Coding Agent 在现有 `develop_go` 分支上完成增量升级。目标不是重写通用 Agent 平台，而是把两套已经通过 Codex Skill 实际验证的视频生产方法，转化为可持续运行、可审核、可恢复、可局部重跑的正式产品能力。

两条目标生产线：

1. **AIGC 镜头式视频**
   - 剧本生成
   - 主要人物、场景、道具设定和参考图
   - 拆分 3–15 秒 SHOT
   - 每个 SHOT 的分镜、关键帧 Prompt、视频 Prompt
   - 组合参考图与 Prompt 调用 Seedance 等视频 API
   - 逐 SHOT 审核、返修、导入和装配

2. **文字口播可视化视频**
   - 观点或自然语言输入
   - 意图识别与内容结构化
   - 口播稿生成和审核
   - 视频视觉参考文档
   - Visual Beat 与前端组件规划
   - 需要图片的位置调用 GPT Image 类 API
   - 生成 HyperGenKeyframe 工程
   - Electron 本地渲染、局部修改和导出

## 2. 当前系统基线

### 2.1 已有能力

当前代码仓已具备：

- Go 模块化单体后端，入口为 `aios-core/cmd/tangying-ai-os`。
- `internal/agents` 与 `internal/core` 的领域/核心边界。
- Orchestrator DAG、依赖检查、重试、暂停、恢复、取消、状态汇聚。
- Redpanda 事件总线和 Outbox 可靠投递。
- Worker Tool Registry、内置工具、外部 HTTP 工具注册。
- DirectExecutor 与 Rust gRPC SandboxExecutor。
- Workflow Template 持久化和实例化基础，已出现 `CONTROL` 人工审核节点。
- PostgreSQL、Redis、MinIO、Redpanda、Qdrant 基础设施。
- React + TypeScript + Zustand 前端。
- Electron 主进程、Preload、安全 IPC、`.dmg/.exe` 打包。
- 素材上传、媒体分析、对话式 DAG 规划、发布相关接口。
- `go test ./...` 与 API 脚本测试入口。

### 2.2 当前不足

- Workflow 目前偏“DAG 模板”，缺少面向视频项目的 Run、Stage、Artifact、Review、Version。
- Skill 主要作为对话规划概念存在，尚未形成可注册、可版本锁定、可按阶段加载的 Skill Package。
- 前端围绕标题、简介和发布，缺少视频项目、Shot/VisualBeat、阶段审核和版本界面。
- Electron 默认面向本地后端，缺少云端 API 配置和本地 Runner 任务协议。
- 模型调用没有完整抽象为文本、图像理解、图像生成、视频生成等统一能力网关。
- 外部 API 的成本、超时、429、异步轮询、幂等和结果归档缺少统一治理。
- 缺少两条视频流程的确定性模板、自动化模拟测试和局部重跑测试。
- 当前对话历史和 DAG 输出适合内容生成，不足以稳定承载长周期视频项目状态。

## 3. 产品目标

### 3.1 核心目标

用户应当能够在 Electron 或 Web 中：

1. 新建视频项目并选择两种生产模式之一。
2. 输入想法或导入已有剧本、口播稿、音频、参考图。
3. 启动固定工作流并看到每个阶段状态。
4. 在剧本、视觉资产、Shot 生产包和成片阶段进行人工审核。
5. 对单个 Shot、VisualBeat、关键帧或 Prompt 进行局部修改和重跑。
6. 关闭应用或服务重启后继续执行。
7. 使用云端完成 Agent、模型 API 和项目状态管理。
8. 使用 Electron 完成本地文件访问、HyperGenKeyframe、Node、FFmpeg 和导出。
9. 通过任务中心追踪所有模型调用、本地任务、失败原因、重试和成本。
10. 使用 Fake Provider 完成全流程自测试，不消耗真实 API 额度。

### 3.2 成功标准

- 两种模式均可从“新建项目”运行到“得到可导出的成片或生产包”。
- 任一阶段失败后可恢复，不要求重新执行已通过阶段。
- 任一 Shot/VisualBeat 可独立重跑，旧版本保留。
- 默认测试环境不需要真实模型 API。
- 现有内容发布、素材库和对话接口回归测试通过。
- 后端 `go test -race ./...` 通过。
- 前端 build、unit test 和关键 E2E 通过。
- Electron 能连接云端、注册本地 Runner、领取并完成模拟渲染任务。

## 4. 范围

### 4.1 本次必须实现

- 视频项目管理。
- Skill Package 注册、版本和加载。
- Workflow Run/Stage/Approval/Artifact。
- 两套内置工作流。
- 通用 Model Gateway 与 Fake Provider。
- AIGC Shot/VisualBeat 领域模型。
- 阶段审核、局部重跑和版本。
- 云端任务事件流。
- Electron Local Runner。
- 项目工作台、Shot 工作台、口播时间线工作台、任务中心。
- 自动化测试体系和 CI 门禁。
- Docker 云端启动配置。

### 4.2 本次不要求

- 自建或微调大模型。
- GPU 调度、CUDA、TensorRT。
- 完整 NLE 专业剪辑器。
- 自动发布到所有平台。
- 多租户企业权限体系。
- Agent 间自由聊天或无限 Subagent。
- 自动生成并执行任意 Shell。
- 一次性彻底移除旧 PublishPage。
- 真实 Seedance/GPT Image API 作为自动测试依赖。

## 5. 架构原则

1. **固定流程优先**：两类高频视频使用固定 Workflow，不让 LLM 任意重写主流程。
2. **Agent 负责判断**：模式选择、内容理解、失败分析和返修策略由 Agent 完成。
3. **Skill 保存方法**：提示词、参考规范、Schema、示例和质量门禁放入 Skill Package。
4. **Tool 负责动作**：模型 API、FFmpeg、本地渲染和文件处理均作为受控工具。
5. **Core 与业务隔离**：通用 Runtime 放 `internal/core`；视频领域放 `internal/agents/video`。
6. **双格式产物**：JSON 作为机器执行真相；Markdown、图片和视频供人审核。
7. **版本不可覆盖**：修改产生新 Artifact Version，不直接覆盖旧结果。
8. **人工审核优先**：高成本视频生成前必须存在可配置审核节点。
9. **云端控制、本地执行**：云端维护状态和模型调用；Electron 访问本地素材和渲染。
10. **测试不依赖付费服务**：所有 Provider 均须有 Fake/Mock 实现。

## 6. 用户角色

### U1：创作者/管理员

当前个人自用阶段可视为单一管理员，拥有项目、配置、审核、重跑和导出权限。

### U2：云端 Agent

负责选择工作流、生成阶段内容、调用工具、记录结果和触发审核。

### U3：Electron Local Runner

负责安全执行本地白名单任务，不能接收任意 Shell 字符串。

## 7. 核心用例

### UC-01 新建 AIGC 镜头式视频

- 输入：主题、目标时长、画幅、风格、参考资料。
- 输出：项目 ID、锁定的 Skill/Workflow 版本、第一阶段 Run。
- 验收：项目状态可查询，重启后仍存在。

### UC-02 新建口播可视化视频

- 输入：观点、目标时长、平台、口播语气。
- 输出：口播工作流 Run。
- 验收：能够生成并编辑 Beat 化口播稿。

### UC-03 审核阶段

- 用户可以通过、要求修改、手动编辑后通过、驳回。
- 审核结果必须记录操作者、时间、备注、输入版本和输出版本。

### UC-04 局部重跑

- 用户选择一个 Shot/VisualBeat 或一个 Artifact。
- 系统只使该单元及其下游失效。
- 其他已通过单元不重跑。

### UC-05 导入人工生成视频

- 当视频平台没有稳定 API 时，用户可导出 Shot Package。
- 用户在外部平台生成后，将 MP4 拖入对应 Shot。
- 系统建立来源、版本和审核记录。

### UC-06 本地渲染口播项目

- 云端创建 Local Job。
- Electron Runner 领取 Bundle。
- 本地运行固定 HyperGenKeyframe 渲染命令。
- 上报进度、日志和输出位置。

### UC-07 故障恢复

- 模型 429/500、网络断开、Electron 离线、服务重启后，任务可重试或恢复。
- 重复事件不得造成重复 Artifact 或重复计费记录。

## 8. 功能需求

### FR-PROJECT 项目

- FR-PROJECT-001：支持创建、查询、更新、归档视频项目。
- FR-PROJECT-002：项目必须锁定 `mode`、`skill_name/version`、`workflow_id/version`。
- FR-PROJECT-003：项目保存当前阶段、状态、目标画幅、目标时长和本地映射路径。
- FR-PROJECT-004：支持从已有项目克隆，默认不复制成片二进制。
- FR-PROJECT-005：项目删除默认软删除。

### FR-SKILL Skill Runtime

- FR-SKILL-001：从配置目录加载 `skill.yaml`、`SKILL.md`、schemas、templates、references。
- FR-SKILL-002：启动时校验 Manifest 和引用文件。
- FR-SKILL-003：Skill Version 一旦被项目引用，不得原地修改。
- FR-SKILL-004：按 Stage 只加载必要 Prompt/Reference，避免把整个 Skill 塞入一次模型上下文。
- FR-SKILL-005：支持启用、禁用和健康检查。
- FR-SKILL-006：加载失败不应导致 Core 整体启动失败，但对应 Skill 不可用且必须暴露错误。

### FR-WORKFLOW 工作流

- FR-WORKFLOW-001：Workflow Template 必须有版本、输入 Schema、Stage 定义和 DAG。
- FR-WORKFLOW-002：实例化后生成 WorkflowRun，并关联现有 Orchestrator Task。
- FR-WORKFLOW-003：支持 Stage 状态：PENDING、RUNNING、WAITING_APPROVAL、SUCCEEDED、FAILED、CANCELLED、INVALIDATED。
- FR-WORKFLOW-004：支持 CONTROL/APPROVAL 节点。
- FR-WORKFLOW-005：支持按 Shot/VisualBeat 展开并行节点。
- FR-WORKFLOW-006：并行数可配置，默认图片 3、视频 2、本地渲染 1。
- FR-WORKFLOW-007：支持从 Stage 或 Unit 重跑。
- FR-WORKFLOW-008：重跑必须生成新 Run Attempt，不覆盖旧执行记录。
- FR-WORKFLOW-009：现有 workflow_templates 和旧实例化 API 保持兼容。

### FR-ARTIFACT 产物

- FR-ARTIFACT-001：所有中间产物统一登记 Artifact。
- FR-ARTIFACT-002：Artifact 支持 JSON、Markdown、Image、Audio、Video、Bundle、Log。
- FR-ARTIFACT-003：二进制存 MinIO；结构化元数据存 PostgreSQL。
- FR-ARTIFACT-004：Artifact 包含来源 Stage、Unit、Provider、Prompt Hash、版本和父版本。
- FR-ARTIFACT-005：支持 current version 指针和历史版本列表。
- FR-ARTIFACT-006：大模型输出必须先通过 JSON Schema 校验再登记为成功。

### FR-MODEL-GATEWAY 模型网关

- FR-MODEL-001：统一支持 TEXT_TO_TEXT、IMAGE_TO_TEXT、TEXT_TO_IMAGE、TEXT_IMAGE_TO_VIDEO。
- FR-MODEL-002：Provider Adapter 处理鉴权、请求映射、异步轮询、下载和错误映射。
- FR-MODEL-003：每次调用记录 request fingerprint、耗时、状态、用量和估算成本。
- FR-MODEL-004：支持超时、指数退避、429 Retry-After 和可配置最大重试。
- FR-MODEL-005：相同 fingerprint 可配置复用成功结果。
- FR-MODEL-006：提供 Fake Provider，用于单元、集成和 E2E。
- FR-MODEL-007：日志不得打印 API Key、完整用户敏感数据或大体积 base64。

### FR-AIGC AIGC Shot 视频

- FR-AIGC-001：支持 Script、Character、Scene、Prop、Shot、Storyboard、Keyframe、VideoPrompt。
- FR-AIGC-002：每个 Shot 必须保存开始状态和结束状态。
- FR-AIGC-003：Shot Package 必须包含参考资产、分镜、关键帧、视频 Prompt 和审核状态。
- FR-AIGC-004：视频生成前可配置为必须人工审核。
- FR-AIGC-005：支持 API 自动生成和手动导入 MP4 两种模式。
- FR-AIGC-006：视频导入后自动抽取首/中/尾关键帧并触发图片理解审核。
- FR-AIGC-007：支持单 Shot 重跑和版本对比。
- FR-AIGC-008：最终输出时间线清单、字幕/音频引用和片段列表；专业精剪可在外部软件完成。

### FR-VOICE 口播可视化视频

- FR-VOICE-001：支持 Opinion、NarrationScript、NarrationBeat、VisualBeat。
- FR-VOICE-002：口播稿必须分 Beat，并可独立编辑和重生成。
- FR-VOICE-003：VisualBeat 只能使用注册的视觉组件类型或 generated_image。
- FR-VOICE-004：模型不直接自由生成完整任意前端工程；应输出组件 DSL。
- FR-VOICE-005：Renderer 将 DSL 转换成 HyperGenKeyframe 工程。
- FR-VOICE-006：支持上传真人录音或引用外部 TTS 音频。
- FR-VOICE-007：本地渲染任务通过 Electron Runner 执行。
- FR-VOICE-008：支持按 VisualBeat 局部修改并重新渲染预览。

### FR-DESKTOP Electron Runner

- FR-DESKTOP-001：API Base URL 可配置为云端地址，不再固定 localhost。
- FR-DESKTOP-002：Runner 注册并定时 heartbeat。
- FR-DESKTOP-003：Runner 通过 claim/poll 领取任务，适应 NAT 环境。
- FR-DESKTOP-004：只允许执行注册的 command type，不允许云端下发任意 Shell。
- FR-DESKTOP-005：任务工作目录必须限制在用户配置的 AIOS 项目根目录。
- FR-DESKTOP-006：支持取消子进程、超时和 stdout/stderr 流式上报。
- FR-DESKTOP-007：Runner 离线后任务回到等待或失败，不丢失状态。

### FR-UI 前端

- FR-UI-001：新增创作中心、项目列表、项目工作台、任务中心、素材库。
- FR-UI-002：保留旧 PublishPage，使用 feature flag 控制新入口。
- FR-UI-003：项目工作台显示阶段树、当前产物、审核按钮和 Agent 助手。
- FR-UI-004：Shot 工作台显示 Shot 列表、分镜、关键帧、Prompt、视频、问题和版本。
- FR-UI-005：口播工作台显示播放器、时间线、NarrationBeat、VisualBeat 和组件配置。
- FR-UI-006：所有长任务使用 SSE 或轮询显示进度，页面刷新后可恢复。
- FR-UI-007：不可只用全屏 BlockingOverlay 阻塞整个应用，应允许用户切换项目或查看其他任务。

### FR-OBS 可观测性

- FR-OBS-001：所有请求使用 trace_id，贯穿 API、WorkflowRun、Task、Node、Tool、ModelCall、LocalJob。
- FR-OBS-002：提供项目和 Run 级事件流。
- FR-OBS-003：日志结构化，包含 module、project_id、run_id、stage_id、unit_id。
- FR-OBS-004：记录成功率、重试率、阶段耗时、Provider 延迟和成本。
- FR-OBS-005：模型失败与业务审核失败分开统计。

## 9. 非功能需求

### NFR-可靠性

- 重复提交、重复事件和重复回调必须幂等。
- 服务重启后数据库中的 RUNNING 任务必须进入恢复扫描。
- 关键状态更新与事件写入尽量使用事务 + Outbox。

### NFR-性能

- 普通项目查询 P95 小于 500ms，不包含模型调用。
- 项目包含 100 个 Shot 时列表接口必须分页。
- SSE 连接不得持有数据库事务。
- 大文件不得完整加载进 Go 堆内存。

### NFR-安全

- Provider Secret 只从环境变量或 Secret Store 读取。
- Electron IPC 使用 contextIsolation，禁止 nodeIntegration。
- 本地文件路径必须规范化并检查根目录逃逸。
- 上传文件校验 MIME、扩展名和大小。
- 生成 Bundle 不包含 API Key。

### NFR-兼容性

- 旧接口保持可用。
- 新数据库表采用新增方式，避免直接重命名旧表。
- 旧 Workflow Template 无 version 时按 `legacy-v1` 读取。
- 旧 Electron 本地后端模式仍可通过配置启用。

## 10. 验收级业务场景

### AC-BIZ-01 AIGC Fake 全流程

使用 Fake Provider 创建一个 3 Shot 项目，完成剧本、资产、Shot、生产包、人工审核、视频模拟生成、抽帧审核、最终清单。所有状态、Artifact 和版本可查询。

### AC-BIZ-02 Shot 局部返修

使 SHOT_02 审核失败，仅重跑 SHOT_02；SHOT_01/03 的 Artifact ID 和版本保持不变。

### AC-BIZ-03 口播 Fake 全流程

输入观点，生成 3 个 NarrationBeat、3 个 VisualBeat、组件 DSL、模拟图片、HyperGen Bundle，并由 Fake Local Runner 完成渲染任务。

### AC-BIZ-04 服务重启恢复

工作流执行中停止后端并重启，Run 能恢复为可继续状态，不产生重复计费记录或重复 Artifact。

### AC-BIZ-05 回归

现有健康检查、发布、素材、工具注册、对话 Session、DAG 调度测试全部通过。


---

# AIOS 自媒体视频创作能力升级——详细设计

## 1. 设计摘要

本升级采用“核心能力增量扩展 + 视频领域适配器”的方式：

```text
React / Electron
        │ HTTPS + SSE
        ▼
Go AIOS Core
├── internal/core
│   ├── workflow         复用并扩展模板、Run、Stage、Approval
│   ├── orchestrator     保持现有 DAG 调度和状态机
│   ├── worker           保持工具注册和执行路由
│   ├── skillruntime     新增：Skill Package 加载与版本
│   ├── modelgateway     新增：外部多模态 API 统一网关
│   ├── artifact         新增：产物、版本和 MinIO 引用
│   └── localrunner      新增：Electron 本地任务协议
│
└── internal/agents/video
    ├── project          视频项目
    ├── creation         Shot / VisualBeat
    ├── review           审核和返修
    ├── handler
    ├── service
    └── workflows        两套内置工作流注册
```

现有 Orchestrator 仍是节点执行真相，不另写一套调度器。新增 WorkflowRun 负责产品级阶段和项目映射，内部编译或实例化为现有 DAG。

## 2. 关键设计决策

### ADR-01 不重写 Orchestrator

原因：

- 已支持 DAG、依赖、条件、重试、暂停、恢复、取消。
- 已有 Redpanda + Outbox。
- 重写会扩大回归面。

做法：

- `WorkflowRun` 关联 `orchestrator_task_id`。
- Stage 与 DAG node/group 建立 mapping。
- CONTROL 节点用于人工审核。
- Foreach/Parallel 在工作流实例化时展开为现有 DAG 节点。

### ADR-02 Skill 不直接等于 Workflow

- Skill：方法、Prompt、Schema、参考和质量要求。
- Workflow：确定性阶段、依赖、审核和重跑边界。
- Tool：一次确定性动作。
- Agent：在受控边界内判断和生成。

### ADR-03 生产运行不直接依赖 Codex CLI

现有 Codex + Skill 可保留为 Skill 开发和调试工具。正式产品运行时将 Skill 拆为：

- stage instructions
- input/output schema
- templates/references
- workflow nodes
- provider/tool calls

### ADR-04 视频 API 必须支持“手动导入模式”

第一版不能依赖 Seedance 等平台一定有稳定开放 API。`generation_mode`：

- `provider_api`
- `manual_import`

二者共用 Shot Package、Artifact、Review 和版本系统。

### ADR-05 组件 DSL 代替自由生成前端工程

口播模式中，模型只输出允许的组件 DSL。Renderer 负责将 DSL 转成 HyperGenKeyframe 代码，降低每次生成代码的不确定性。

## 3. 包结构

### 3.1 新增 Core 包

```text
aios-core/internal/core/
├── artifact/
│   ├── model.go
│   ├── repository.go
│   ├── service.go
│   ├── storage.go
│   └── service_test.go
├── skillruntime/
│   ├── manifest.go
│   ├── loader.go
│   ├── registry.go
│   ├── validator.go
│   └── loader_test.go
├── modelgateway/
│   ├── types.go
│   ├── gateway.go
│   ├── router.go
│   ├── usage.go
│   ├── provider.go
│   ├── providers/fake/
│   └── providers/openai_compatible/
└── localrunner/
    ├── model.go
    ├── repository.go
    ├── service.go
    ├── handler.go
    └── service_test.go
```

### 3.2 扩展 Workflow

```text
internal/core/workflow/
├── model.go               保留 Template，增加 Version/InputSchema
├── run_model.go           WorkflowRun / StageRun / Attempt
├── run_repository.go
├── run_service.go
├── compiler.go            高级 Stage → 现有 DAG
├── approval_service.go
├── rerun_service.go
└── *_test.go
```

### 3.3 视频领域 Agent

```text
internal/agents/video/
├── handler/
│   ├── project_handler.go
│   ├── workflow_handler.go
│   ├── unit_handler.go
│   └── review_handler.go
├── model/
│   ├── project.go
│   ├── asset.go
│   ├── shot.go
│   ├── voice.go
│   └── review.go
├── repository/
├── service/
│   ├── project_service.go
│   ├── aigc_service.go
│   ├── voice_service.go
│   ├── review_service.go
│   └── timeline_service.go
└── workflows/
    ├── register.go
    ├── aigc_shot_video_v1.go
    └── voice_visual_video_v1.go
```

## 4. Skill Package

### 4.1 推荐目录

```text
skills/
├── aigc-shot-video/
│   └── 1.0.0/
│       ├── skill.yaml
│       ├── SKILL.md
│       ├── stages/
│       │   ├── script.md
│       │   ├── visual_design.md
│       │   ├── shot_plan.md
│       │   ├── storyboard.md
│       │   ├── keyframe.md
│       │   ├── video_prompt.md
│       │   └── review.md
│       ├── schemas/
│       ├── templates/
│       ├── references/
│       └── examples/
└── voice-visual-video/
    └── 1.0.0/
        └── ...
```

### 4.2 加载策略

启动时：

1. 扫描 `SKILL_ROOT`。
2. 解析 `skill.yaml`。
3. 用 JSON Schema 校验。
4. 校验引用文件存在和 SHA256。
5. 注册 `{name, version}`。
6. 错误 Skill 标记 `UNHEALTHY`，不阻止其他 Skill。
7. 提供 `/api/skills` 查询健康状态。

### 4.3 上下文构建

Stage 调用模型时只加载：

```text
系统通用约束
+ 当前 Skill stage instruction
+ 必要 references
+ 当前项目摘要
+ 当前 unit 上下文
+ 上下游边界状态
+ 输出 JSON Schema
```

不加载完整 Skill、全部历史和全部 Shot。

## 5. Workflow 高级定义与编译

### 5.1 模板模型

建议增加：

```go
type Template struct {
    ID          string
    Version     string
    Name        string
    Description string
    Category    string
    InputSchema json.RawMessage
    Definition  json.RawMessage
    Enabled     bool
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

数据库主键建议 `(id, version)`，旧 `id` 主键表不直接破坏。迁移可先新增 `workflow_template_versions`，旧表继续兼容。

### 5.2 Stage 定义

```go
type StageDefinition struct {
    ID               string
    Name             string
    Kind             string // AGENT, TOOL, APPROVAL, PARALLEL, FOREACH, LOCAL_TOOL
    SkillStage       string
    Tool             string
    DependsOn        []string
    Input            map[string]any
    OutputSchemaRef  string
    ApprovalRequired bool
    RerunScope       string // SELF, UNIT_DOWNSTREAM, STAGE_DOWNSTREAM
}
```

### 5.3 编译到现有 DAG

- `AGENT` → LLM node 或受控 Agent tool node。
- `TOOL` → TOOL node。
- `APPROVAL` → CONTROL node。
- `PARALLEL` → 多个拥有相同依赖的节点。
- `FOREACH` → 实例化时按 Unit 展开。
- `LOCAL_TOOL` → TOOL node，工具名为 `local_job_dispatcher`。

### 5.4 Stage 状态映射

```text
全部映射节点 CREATED/READY       -> PENDING
任一 RUNNING/RETRYING            -> RUNNING
CONTROL 等待操作                 -> WAITING_APPROVAL
全部 SUCCESS/SKIPPED             -> SUCCEEDED
任一最终 FAILED                  -> FAILED
被重跑逻辑标记                   -> INVALIDATED
```

## 6. 数据模型

### 6.1 video_projects

```sql
CREATE TABLE video_projects (
  id UUID PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  mode VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  current_stage VARCHAR(128),
  skill_name VARCHAR(128) NOT NULL,
  skill_version VARCHAR(32) NOT NULL,
  workflow_id VARCHAR(128) NOT NULL,
  workflow_version VARCHAR(32) NOT NULL,
  aspect_ratio VARCHAR(16),
  target_duration_sec INT,
  language VARCHAR(16) DEFAULT 'zh-CN',
  settings JSONB NOT NULL DEFAULT '{}',
  local_path_hint TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);
```

### 6.2 workflow_runs

```sql
CREATE TABLE workflow_runs (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  workflow_id VARCHAR(128) NOT NULL,
  workflow_version VARCHAR(32) NOT NULL,
  orchestrator_task_id VARCHAR(128),
  status VARCHAR(32) NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  input JSONB NOT NULL DEFAULT '{}',
  output JSONB NOT NULL DEFAULT '{}',
  trace_id VARCHAR(128) NOT NULL,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.3 workflow_stage_runs

```sql
CREATE TABLE workflow_stage_runs (
  id UUID PRIMARY KEY,
  workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id),
  stage_id VARCHAR(128) NOT NULL,
  unit_id UUID,
  status VARCHAR(32) NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  node_ids JSONB NOT NULL DEFAULT '[]',
  input JSONB NOT NULL DEFAULT '{}',
  output JSONB NOT NULL DEFAULT '{}',
  error_code VARCHAR(128),
  error_message TEXT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  UNIQUE(workflow_run_id, stage_id, unit_id, attempt)
);
```

### 6.4 creation_units

统一 Shot 与 VisualBeat：

```sql
CREATE TABLE creation_units (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  kind VARCHAR(32) NOT NULL,
  sequence_no INT NOT NULL,
  start_sec NUMERIC,
  end_sec NUMERIC,
  status VARCHAR(32) NOT NULL,
  version INT NOT NULL DEFAULT 1,
  data JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(project_id, kind, sequence_no, version)
);
```

`data` 内应由 Go typed struct 校验，不允许业务层长期依赖任意 map。

### 6.5 artifacts

```sql
CREATE TABLE artifacts (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  workflow_run_id UUID,
  stage_id VARCHAR(128),
  unit_id UUID,
  kind VARCHAR(32) NOT NULL,
  name VARCHAR(255) NOT NULL,
  version INT NOT NULL,
  parent_artifact_id UUID,
  storage_type VARCHAR(16) NOT NULL,
  storage_ref TEXT,
  inline_json JSONB,
  mime_type VARCHAR(128),
  size_bytes BIGINT,
  content_hash VARCHAR(128),
  prompt_hash VARCHAR(128),
  provider VARCHAR(128),
  model VARCHAR(128),
  is_current BOOLEAN NOT NULL DEFAULT TRUE,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

写入新版本时，在同一事务中将旧 current 设为 false。

### 6.6 reviews

```sql
CREATE TABLE reviews (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES video_projects(id),
  stage_id VARCHAR(128),
  unit_id UUID,
  artifact_id UUID,
  review_type VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  score NUMERIC,
  issues JSONB NOT NULL DEFAULT '[]',
  comment TEXT,
  reviewer VARCHAR(128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.7 model_calls

记录 Provider 调用、fingerprint、成本和幂等。

### 6.8 local_runners / local_jobs

Runner heartbeat 和本地任务状态。Local Job Payload 只能是结构化命令：

```json
{
  "commandType": "HYPERGEN_RENDER",
  "projectId": "...",
  "bundleArtifactId": "...",
  "outputRelativePath": "exports/preview.mp4",
  "options": {"composition": "main"}
}
```

禁止：

```json
{"shell": "rm -rf ..."}
```

## 7. 领域模型

### 7.1 AIGC Shot

```go
type ShotData struct {
    ShotCode       string
    DurationSec    float64
    NarrativeGoal  string
    SceneID        string
    CharacterIDs   []string
    PropIDs        []string
    StartState     BoundaryState
    Timeline       []TimelineEvent
    EndState       BoundaryState
    NegativeRules  []string
}
```

`BoundaryState` 至少包含人物位置、身体姿态、视线、手部、道具、相机、光线。

### 7.2 ShotPackage

```go
type ShotPackage struct {
    ShotID             string
    StoryboardArtifact string
    KeyframeArtifacts  []string
    VideoPromptArtifact string
    ReferenceArtifacts []string
    ExpectedStart      BoundaryState
    ExpectedEnd        BoundaryState
    ReviewStatus       string
}
```

### 7.3 Voice Visual

```go
type NarrationBeat struct {
    BeatCode       string
    Text           string
    Emotion        string
    EstimatedSec   float64
    VisualPurpose  string
}

type VisualBeat struct {
    BeatCode        string
    StartSec        float64
    EndSec          float64
    ComponentType   string
    Props           json.RawMessage
    ImageNeeds      []ImageRequirement
    Transition      string
}
```

组件类型使用白名单。

## 8. Model Gateway

### 8.1 能力接口

```go
type Capability string

const (
    TextToText      Capability = "TEXT_TO_TEXT"
    ImageToText     Capability = "IMAGE_TO_TEXT"
    TextToImage     Capability = "TEXT_TO_IMAGE"
    TextImageToVideo Capability = "TEXT_IMAGE_TO_VIDEO"
)

type Provider interface {
    Name() string
    Supports(Capability) bool
    Invoke(ctx context.Context, req Request) (Result, error)
}
```

### 8.2 异步视频

Provider Adapter 内部允许：

1. Submit。
2. 持久化 external_job_id。
3. Poll 或处理 webhook。
4. 下载结果到 MinIO。
5. 登记 Artifact。
6. 发布统一完成事件。

禁止让 HTTP Handler 同步等待几分钟。

### 8.3 错误分类

- `PROVIDER_RATE_LIMITED`
- `PROVIDER_AUTH_FAILED`
- `PROVIDER_TIMEOUT`
- `PROVIDER_CONTENT_REJECTED`
- `PROVIDER_INVALID_REQUEST`
- `PROVIDER_UNAVAILABLE`
- `OUTPUT_SCHEMA_INVALID`
- `OUTPUT_DOWNLOAD_FAILED`

只有可重试错误进入自动重试。

### 8.4 Fake Provider

Fake Provider 使用 fixture：

- 固定 script JSON。
- 固定 3 Shot。
- 固定 SVG/PNG 占位图。
- 固定 MP4 小样或生成轻量测试视频。
- 支持通过请求参数注入 429、500、timeout、schema invalid。

## 9. 两套工作流

### 9.1 AIGC Shot Video v1

```text
brief
→ script_generate
→ script_approval
→ visual_rules
→ asset_plan
→ [characters | scenes | props] parallel
→ assets_approval
→ shot_plan
→ continuity_plan
→ foreach shot:
     storyboard
     keyframes
     video_prompt
     shot_package_approval
     video_generate_or_import
     video_review
     conditional_repair
→ timeline_manifest
→ final_approval
→ publication_package
```

### 9.2 Voice Visual Video v1

```text
opinion_intent
→ narration_script
→ narration_approval
→ audio_bind
→ narration_timing
→ visual_document
→ visual_beats
→ foreach visual beat:
     component_plan
     optional_image_generate
→ hypergen_bundle
→ local_render
→ render_review
→ conditional_beat_repair
→ final_export
→ publication_package
```

## 10. 局部重跑算法

输入：`project_id, stage_id, unit_id?, artifact_id?`

1. 加锁 `project:{id}:rerun`。
2. 校验项目不在删除或终止状态。
3. 根据 Workflow Definition 计算下游影响集合。
4. 若有 unit_id，仅失效该 unit 的相关 StageRun/Artifact。
5. 创建新的 StageRun attempt。
6. 旧 Artifact 保留，`is_current` 在新结果成功后切换。
7. 新 DAG node ID 必须带 attempt 后缀，避免幂等键冲突。
8. 若新执行失败，旧 current Artifact 仍可用。
9. 发布 `workflow.stage.rerun_requested` 和完成/失败事件。

## 11. API 设计

统一响应沿用现有：

```json
{"code": 200, "message": "success", "data": {}}
```

### 11.1 项目

```http
POST   /api/video-projects
GET    /api/video-projects
GET    /api/video-projects/:id
PATCH  /api/video-projects/:id
POST   /api/video-projects/:id/clone
DELETE /api/video-projects/:id
```

### 11.2 工作流

```http
POST /api/video-projects/:id/workflow-runs
GET  /api/workflow-runs/:runId
POST /api/workflow-runs/:runId/pause
POST /api/workflow-runs/:runId/resume
POST /api/workflow-runs/:runId/cancel
GET  /api/workflow-runs/:runId/events
```

### 11.3 审核和重跑

```http
POST /api/workflow-runs/:runId/stages/:stageId/approve
POST /api/workflow-runs/:runId/stages/:stageId/revise
POST /api/workflow-runs/:runId/stages/:stageId/rerun
POST /api/creation-units/:unitId/rerun
```

### 11.4 Unit 和 Artifact

```http
GET /api/video-projects/:id/units
GET /api/creation-units/:unitId
GET /api/creation-units/:unitId/artifacts
GET /api/artifacts/:artifactId
POST /api/creation-units/:unitId/import-video
```

### 11.5 Skill 和 Provider

```http
GET /api/skills
GET /api/skills/:name/:version
GET /api/model-providers
```

### 11.6 Local Runner

```http
POST /api/local-runners/register
POST /api/local-runners/:id/heartbeat
POST /api/local-runners/:id/jobs/claim
POST /api/local-jobs/:id/progress
POST /api/local-jobs/:id/complete
POST /api/local-jobs/:id/fail
POST /api/local-jobs/:id/cancel
```

## 12. 事件

新增事件统一走 Outbox：

```text
video.project.created
workflow.run.created
workflow.stage.started
workflow.stage.waiting_approval
workflow.stage.succeeded
workflow.stage.failed
workflow.stage.rerun_requested
artifact.created
artifact.version_activated
review.created
model.call.started
model.call.completed
model.call.failed
local.job.created
local.job.claimed
local.job.progress
local.job.completed
local.job.failed
```

事件必须包含：

```json
{
  "eventId": "...",
  "traceId": "...",
  "projectId": "...",
  "workflowRunId": "...",
  "stageId": "...",
  "unitId": "...",
  "occurredAt": "..."
}
```

## 13. 前端设计

### 13.1 路由

当前前端无正式 router。升级建议引入 React Router，但第一步可先保持 Zustand page switching，避免一次改动过大。最终路由：

```text
/creation
/projects
/projects/:id
/projects/:id/shots/:unitId
/projects/:id/voice
/tasks
/media
/settings
/publish-legacy
```

### 13.2 页面

- `CreationCenterPage`：两种模板入口。
- `ProjectListPage`：筛选状态和模式。
- `ProjectWorkbenchPage`：阶段树、产物、审核。
- `ShotWorkbenchPage`：Shot 生产包和版本。
- `VoiceWorkbenchPage`：Beat/VisualBeat 时间线。
- `TaskCenterPage`：云端模型、本地任务和日志。
- 保留 `PublishPage`。

### 13.3 Store

不要继续把所有状态放入单个 `appStore`。拆分：

```text
projectStore
workflowStore
artifactStore
taskStore
uiStore
legacyPublishStore
```

服务端状态优先考虑 TanStack Query；Zustand 仅保存 UI 和临时编辑状态。

### 13.4 SSE

- 页面刷新后先 GET Run snapshot，再订阅 SSE。
- 使用 event id 支持 Last-Event-ID。
- SSE 断开自动退化为 3–5 秒轮询。
- 一个项目页面只维护一个事件连接。

## 14. Electron 设计

### 14.1 配置

```json
{
  "apiBaseUrl": "https://aios.example.com/api",
  "projectRoot": "/Users/.../AIOS-Projects",
  "runnerEnabled": true
}
```

凭证放系统 Keychain，不写明文 JSON。

### 14.2 IPC 白名单

- `config:get/set`
- `project:selectRoot`
- `runner:start/stop/status`
- `file:openArtifact`
- `file:revealInFolder`
- `render:cancel`

Renderer 不能直接执行 shell。

### 14.3 Local Executor

Command Handler 映射：

```text
HYPERGEN_RENDER -> 固定 Node 脚本
FFMPEG_PROBE     -> 固定 ffprobe 参数构造器
FFMPEG_ASSEMBLE  -> 固定 ffmpeg 参数构造器
BUNDLE_EXTRACT   -> 安全解压并检查路径
```

每种 Handler 独立校验 payload。

## 15. Docker 云端部署

当前 compose 主要为基础设施。新增生产 compose：

```text
nginx
frontend
aios-core
postgres
redis
redpanda
minio
qdrant(optional)
sandbox(optional)
```

建议文件：

```text
deploy/docker-compose.cloud.yml
deploy/nginx.conf
deploy/.env.cloud.example
```

要求：

- 数据卷持久化。
- 健康检查。
- backend 依赖 infra health，而非只依赖启动顺序。
- 不暴露 PostgreSQL/Redis/MinIO 管理端口到公网。
- API Key 使用 env/secret。
- 上传大小和 Nginx timeout 适配视频。
- `/api/workflow-runs/*/events` 关闭代理缓冲。

## 16. 兼容与迁移

1. 新表全部 `CREATE TABLE IF NOT EXISTS`，后续引入正式 migration 工具。
2. 旧 Workflow Template API 不删除。
3. `workflow_templates` 保留，新增 version table 或兼容列。
4. 旧 Publish/Chat/Media 路由不改语义。
5. 新前端受 `VITE_VIDEO_CREATION_ENABLED` 控制。
6. 后端受 `VIDEO_CREATION_ENABLED` 控制。
7. Electron `apiBaseUrl` 默认仍可指向 localhost。
8. 若新功能关闭，旧系统完整可用。

## 17. 安全与成本

- 模型请求只持久化必要 Prompt；敏感内容可配置脱敏。
- 图片/视频 presigned URL 设置短 TTL。
- Provider 回调必须验签。
- 请求 fingerprint = capability + provider + model + normalized input + asset hashes + parameters。
- 视频生成默认最大自动重试 1 次，防止成本失控。
- 人工修改后 prompt hash 变化，不复用旧结果。
- 每项目可设置预算软限制和硬限制。

## 18. 可观测性

建议 Prometheus 指标：

```text
aios_workflow_stage_total{stage,status}
aios_workflow_stage_duration_seconds
aios_model_call_total{provider,capability,status}
aios_model_call_duration_seconds
aios_model_estimated_cost_total
aios_local_job_total{command_type,status}
aios_artifact_created_total{kind}
aios_rerun_total{scope}
```

日志必须使用现有 Zap，并统一字段。


---

# AIOS 视频创作升级——实施任务计划

## 1. 执行规则

- 每次只完成一个 Phase 或一个小 Task。
- 修改前执行基线测试并记录。
- 每个 Task 必须同时提交实现、测试、文档更新。
- 不允许为让测试通过而删除旧测试或降低断言。
- 真实 API 只做手工 smoke，自动测试使用 Fake Provider。
- Agent 必须更新 `TASK_STATUS.md`。
- 未通过当前 Phase 门禁，不进入下一 Phase。

## 2. Phase 划分

### P0 基线与防回归

#### P0-T01 仓库基线清点

- 核对实际包路径、main wiring、Workflow/CONTROL 实现。
- 列出全部现有测试。
- 运行：
  - `cd aios-core && go test ./...`
  - `cd aios-core && go test -race ./...`
  - `cd frontend && npm ci && npm run build`
  - `./aios-core/scripts/test-apis.sh`（环境可用时）
- 将结果写入 `docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md`。

DoD：

- 基线失败被明确分类为“升级前已有”或“环境问题”。
- 不开始业务代码修改。

#### P0-T02 Feature Flag

新增：

- `VIDEO_CREATION_ENABLED`
- `LOCAL_RUNNER_ENABLED`
- `MODEL_PROVIDER_MODE=fake|real`

DoD：

- Flag 关闭时旧路由和旧页面行为不变。
- 配置单元测试通过。

### P1 基础数据与 Artifact

#### P1-T01 Artifact Core

路径：`internal/core/artifact`

实现：

- Model、Repository、Service。
- MinIO 引用与 inline JSON。
- 版本切换事务。
- content hash。
- 查询 current/history。

测试：

- 新版本成功后旧版本失效。
- 新版本写入失败时旧版本仍 current。
- 重复 content hash 幂等策略。

#### P1-T02 Video Project Domain

路径：`internal/agents/video`

实现：

- Project CRUD。
- 软删除。
- 模式/版本锁定。
- 路由注册。

测试：

- mode 校验。
- skill/workflow version 不可随意修改。
- 分页和软删除。

P1 门禁：

```bash
go test -race ./internal/core/artifact/... ./internal/agents/video/...
go test -race ./...
```

### P2 Skill Runtime

#### P2-T01 Manifest 与 Loader

实现：

- 读取目录。
- JSON Schema 校验。
- 文件引用和 SHA256。
- 健康状态。
- Registry 查询。

#### P2-T02 两个 Skill 骨架

把现有两个 Skill 复制/拆分为版本目录。此 Task 不要求一次重写全部 Prompt，只要求：

- manifest 可加载。
- 每个 stage 有入口文件。
- input/output schema 完整。
- 原 Skill 全文保留在 references 或 migration 目录。

#### P2-T03 Skill API

- `GET /api/skills`
- `GET /api/skills/:name/:version`

P2 门禁：

- 缺文件、坏 YAML、坏 Schema 的测试。
- 两个 Skill 健康。
- 项目能锁定 Skill Version。

### P3 Workflow Run 与 Approval

#### P3-T01 Template Version

在不破坏旧 API 的情况下增加版本。优先新增 version table。

#### P3-T02 WorkflowRun / StageRun

实现：

- 创建 Run。
- 关联现有 Task。
- Stage 映射。
- 状态汇聚。
- snapshot 查询。

#### P3-T03 Approval

复用 CONTROL 节点：

- approve。
- revise/reject。
- 审核记录。
- 幂等。

#### P3-T04 Rerun

- Stage rerun。
- Unit rerun。
- Attempt。
- 下游 invalidation。
- 新 node id/idempotency key。

P3 门禁：

- 纯 Fake Tool 的 DAG 集成测试。
- 重启恢复测试。
- 旧 Workflow API 回归。

### P4 Model Gateway

#### P4-T01 通用接口和 Router

实现 capability、provider、request/result、error mapping。

#### P4-T02 Fake Provider

Fixtures 覆盖：

- success
- 429
- timeout
- 500
- invalid schema
- async video job

#### P4-T03 OpenAI-compatible Adapter

先实现：

- text_to_text
- image_to_text
- text_to_image（若接口兼容）

视频 Provider 使用独立 adapter，不强行假设 OpenAI 格式。

#### P4-T04 Usage/Cost/Idempotency

- model_calls 表。
- fingerprint。
- 重试。
- 结果缓存。
- 日志脱敏。

P4 门禁：

- httptest 合同测试。
- 无真实 API Key 时全部自动测试通过。

### P5 AIGC Shot Workflow

#### P5-T01 领域模型

- Script、Character、Scene、Prop、Shot、BoundaryState、ShotPackage。
- typed JSON validation。

#### P5-T02 工作流模板

注册 `aigc-shot-video@1.0.0`。

#### P5-T03 Stage Services

逐个实现：

1. brief/script
2. visual rules/assets
3. shot plan/continuity
4. storyboard/keyframe/video prompt
5. approval
6. provider/manual import
7. frame review
8. timeline manifest

#### P5-T04 手动导入

- multipart MP4。
- 关联 Shot。
- FFprobe/抽帧工具。
- 触发 review。

#### P5-T05 局部重跑

完成 Shot 粒度版本与下游失效。

P5 门禁：

- 3 Shot Fake E2E。
- SHOT_02 故障只重跑 SHOT_02。
- 旧版本可查看。
- 视频 API 未配置时 manual_import 可工作。

### P6 Voice Visual Workflow

#### P6-T01 领域模型和组件白名单

- NarrationBeat
- VisualBeat
- Component DSL
- JSON Schema

#### P6-T02 Renderer

实现 DSL → HyperGenKeyframe project bundle。

第一版只支持 8–12 个稳定组件，不追求任意 React 代码。

#### P6-T03 工作流模板

注册 `voice-visual-video@1.0.0`。

#### P6-T04 图片生成与 Bundle

生成图片为可选 Stage，Bundle 登记为 Artifact。

#### P6-T05 局部 Beat 重跑

只更新指定 VisualBeat 和受影响 Bundle Version。

P6 门禁：

- Fake 观点 → 3 Beat → 3 VisualBeat → Bundle。
- Schema 不允许未知组件。
- 单 Beat 修改不会重生成其他 Beat 产物。

### P7 Local Runner / Electron

#### P7-T01 云端 Local Job API

- register
- heartbeat
- claim
- progress
- complete
- fail
- cancel

#### P7-T02 Electron 配置

- Cloud API Base URL。
- Project Root。
- Token 安全存储。
- Runner 开关。

#### P7-T03 Command Handlers

- HYPERGEN_RENDER
- FFMPEG_PROBE
- FFMPEG_ASSEMBLE
- BUNDLE_EXTRACT

#### P7-T04 Runner 恢复

- Electron 重启后重新查询已领取任务。
- 云端租约过期处理。
- 子进程取消。

P7 门禁：

- Fake Local Runner 集成。
- Playwright Electron smoke。
- 路径逃逸和任意 Shell 测试。

### P8 前端工作台

#### P8-T01 导航与项目列表

保留旧页面，增加新入口。

#### P8-T02 项目工作台

- 阶段树。
- Run snapshot。
- 审核。
- 事件流。

#### P8-T03 Shot 工作台

- Unit 列表。
- Artifact 预览。
- Prompt 编辑。
- 版本。
- 重跑。

#### P8-T04 Voice 工作台

- NarrationBeat。
- VisualBeat。
- Component props。
- Render 任务。

#### P8-T05 任务中心

统一显示 model/local/workflow tasks。

P8 门禁：

- Vitest/RTL。
- Playwright Fake Backend E2E。
- 刷新恢复。
- SSE 断线降级。

### P9 Cloud Docker 与运维

- `docker-compose.cloud.yml`
- Nginx SSE。
- 健康检查。
- volume/backup。
- `.env.cloud.example`
- 一键 smoke。

P9 门禁：

```bash
docker compose -f deploy/docker-compose.cloud.yml up -d
./scripts/smoke-video-creation.sh
```

### P10 全量验收与文档

- 更新 README、AGENTS.md、ARCHITECTURE、API_REFERENCE。
- 生成测试报告。
- 确认旧功能回归。
- 记录已知限制。
- 创建 release checklist。

## 3. Coding Agent 修改路径建议

### 应新增到 Core

- 通用 Skill loader。
- 通用 Artifact。
- 通用 Model Gateway。
- 通用 Workflow Run/Approval/Rerun。
- 通用 Local Runner 协议。

### 应新增到 Agent

- Video Project。
- Shot/VisualBeat。
- 两条业务工作流。
- 视频领域审核规则。
- Publication package 适配。

### 不应放入 Core

- Seedance 专属业务 Prompt。
- “范进中举”等项目规则。
- HyperGen 具体业务场景模板。
- 自媒体平台标题策略。
- 某个用户的本地路径。
- 外部工具内部算法。

## 4. 每个 Task 的提交模板

```text
Task: P?-T??
Changed:
- ...

Tests added:
- ...

Commands run:
- ...

Result:
- PASS / FAIL

Compatibility:
- Existing API affected? no/yes
- Migration required? no/yes
- Feature flag default: off/on

Remaining risks:
- ...
```

## 5. 禁止事项

- 禁止一次修改全部模块后再补测试。
- 禁止删除现有测试。
- 禁止测试中调用真实收费 API。
- 禁止把 API Key 写入仓库。
- 禁止让 Electron 执行任意云端 Shell。
- 禁止覆盖旧 Artifact。
- 禁止在 HTTP Handler 中等待长视频生成。
- 禁止重复实现已有 Orchestrator 调度。
- 禁止将视频业务逻辑放入 worker/tool registry 核心层。


---

# AIOS 视频创作升级——测试与验收规范

## 1. 测试目标

保证升级具备：

- 业务正确性
- DAG/事件幂等
- 故障恢复
- 局部重跑隔离
- Provider 合同稳定
- Electron 本地执行安全
- 旧功能兼容
- 不消耗真实 API 的可重复 CI

## 2. 测试金字塔

### L1 单元测试

覆盖：

- Skill manifest/parser/validator。
- Workflow compiler。
- Stage 状态映射。
- Rerun 影响范围。
- Artifact 版本事务。
- Model fingerprint/error mapping/retry。
- Shot BoundaryState 校验。
- Visual Component DSL 校验。
- Local command payload 校验。
- 前端 Store 和关键组件。

### L2 Repository 集成

使用测试 PostgreSQL/Redis/MinIO：

- 项目 CRUD。
- Artifact current/version。
- WorkflowRun/StageRun。
- 并发更新。
- Outbox。
- Runner lease。

### L3 服务集成

启动 Go Service + Fake Provider + Fake Local Runner：

- API。
- Workflow。
- Event。
- Approval。
- Rerun。
- Recovery。

### L4 E2E

浏览器/Electron：

- 新建项目。
- 完成审核。
- 查看 Shot/Beat。
- 局部重跑。
- 任务进度。
- 刷新恢复。
- 本地模拟渲染。

## 3. 测试环境

建议新增：

```text
deploy/docker-compose.test.yml
scripts/test-all.sh
scripts/test-backend.sh
scripts/test-frontend.sh
scripts/test-e2e.sh
test/fixtures/
```

环境变量：

```env
MODEL_PROVIDER_MODE=fake
VIDEO_CREATION_ENABLED=true
LOCAL_RUNNER_ENABLED=true
FAKE_PROVIDER_LATENCY_MS=10
```

## 4. 后端门禁

基础：

```bash
cd aios-core
go test ./...
go test -race ./...
go vet ./...
```

建议增加：

```bash
staticcheck ./...
golangci-lint run
```

如果仓库尚未引入对应工具，不得把工具缺失误报为业务失败；应在 CI 镜像中固定版本。

覆盖率建议：

- 新增 Core 包：行覆盖率 ≥ 80%。
- 新增视频领域 Service：≥ 75%。
- Handler 不强制高覆盖，但核心错误分支必须测试。

## 5. 前端门禁

建议添加：

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm run test -- --run
npm run build
```

测试工具建议：

- Vitest
- React Testing Library
- MSW
- Playwright

## 6. Electron 门禁

- preload 暴露 API 快照测试。
- command type 白名单。
- 路径逃逸。
- 超时取消。
- Runner heartbeat。
- Playwright Electron 启动 smoke。

## 7. 核心测试矩阵

| ID | 场景 | 期望 |
|---|---|---|
| T-SKILL-01 | 加载两个合法 Skill | HEALTHY |
| T-SKILL-02 | 缺少 stage 文件 | 对应 Skill UNHEALTHY，Core 正常 |
| T-SKILL-03 | 重复 name/version | 启动报告冲突，不静默覆盖 |
| T-WF-01 | 创建 WorkflowRun | 关联 Task 和 trace |
| T-WF-02 | Approval approve 两次 | 第二次幂等 |
| T-WF-03 | Approval reject | 下游不执行 |
| T-WF-04 | 服务重启 | Run 可恢复 |
| T-WF-05 | 重复 Kafka result | 节点/Artifact 不重复 |
| T-RERUN-01 | Shot_02 重跑 | Shot_01/03 不变 |
| T-RERUN-02 | 新重跑失败 | 旧 current Artifact 仍有效 |
| T-ART-01 | 两版本并发写 | 只有一个 current |
| T-MODEL-01 | Fake success | Artifact + usage |
| T-MODEL-02 | 429 + Retry-After | 限次重试后成功 |
| T-MODEL-03 | 401 | 不重试，映射 AUTH_FAILED |
| T-MODEL-04 | schema invalid | Stage 失败，不登记成功 Artifact |
| T-MODEL-05 | 相同 fingerprint | 按策略复用 |
| T-AIGC-01 | 3 Shot 全流程 | 完成 |
| T-AIGC-02 | 手动导入视频 | 关联正确 Shot |
| T-AIGC-03 | 帧审核失败 | Review issue 结构化 |
| T-VOICE-01 | 3 Beat 全流程 | Bundle 生成 |
| T-VOICE-02 | 未知组件 | Schema 拒绝 |
| T-VOICE-03 | 单 Beat 修改 | 只更新受影响 Artifact |
| T-LOCAL-01 | Runner claim | 租约正确 |
| T-LOCAL-02 | Runner 离线 | Job 可重新分配/等待 |
| T-LOCAL-03 | `../` 路径 | 拒绝 |
| T-LOCAL-04 | 任意 shell payload | 拒绝 |
| T-UI-01 | 页面刷新 | 状态恢复 |
| T-UI-02 | SSE 断开 | 轮询降级 |
| T-REG-01 | 旧 publish API | 通过 |
| T-REG-02 | 旧 skill dialog | 通过 |
| T-REG-03 | 旧 media/tool API | 通过 |

## 8. 工作流模拟测试

### 8.1 AIGC fixture

输入：

```json
{
  "idea": "一只猫用叫声传输二进制消息",
  "durationSec": 30,
  "aspectRatio": "16:9",
  "shotCount": 3
}
```

Fake Provider 输出：

- 1 个 Script。
- 2 个 Character/Prop。
- 3 个 Shot。
- 每 Shot 1 个 Storyboard 和 2 个 Keyframe。
- 3 个轻量 MP4 或占位 Artifact。
- SHOT_02 可通过参数注入审核失败。

断言：

- 产物数量。
- 版本。
- 顺序。
- 边界状态。
- 成本记录。
- 重跑隔离。

### 8.2 Voice fixture

输入：

```json
{
  "opinion": "AI替代的不是岗位，而是整套工作流程",
  "durationSec": 45
}
```

输出：

- 3 NarrationBeat。
- 3 VisualBeat。
- 其中 1 个 generated_image。
- 1 个 HyperGen Bundle。
- 1 个 Fake Render MP4。

## 9. 故障注入

Fake Provider 请求参数：

```json
{
  "_test": {
    "failMode": "rate_limit|timeout|server_error|invalid_schema",
    "failCount": 1
  }
}
```

必须测试：

- 429 一次后成功。
- 连续 500 超过重试后失败。
- timeout 被 context cancel。
- 回调重复。
- MinIO 上传失败。
- DB 成功但事件总线不可用，由 Outbox 后续补发。
- Electron 完成回调重复。
- 后端重启扫描卡住 RUNNING。

## 10. 数据库测试

- Schema 可重复执行。
- migration 向前。
- 新功能关闭时旧表不影响启动。
- FK 和索引。
- 分页。
- soft delete。
- 并发 current artifact。

建议索引：

```sql
CREATE INDEX ON video_projects(user_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX ON workflow_runs(project_id, created_at DESC);
CREATE INDEX ON workflow_stage_runs(workflow_run_id, stage_id, status);
CREATE INDEX ON creation_units(project_id, kind, sequence_no);
CREATE INDEX ON artifacts(project_id, unit_id, kind, is_current);
CREATE INDEX ON model_calls(project_id, created_at DESC);
CREATE INDEX ON local_jobs(status, created_at);
```

## 11. 安全测试

- API key 不出现在日志和错误响应。
- 上传伪造 MIME。
- 压缩包 Zip Slip。
- 本地路径 traversal。
- Electron XSS 不能获得 Node API。
- 超大 JSON 和超大上传限制。
- Provider webhook 伪造。
- Presigned URL 过期。
- 非法 Project/Artifact 关联。

## 12. 性能与稳定性

最低压力场景：

- 100 个项目分页。
- 单项目 100 个 Shot。
- 20 个并发 Fake 模型调用。
- 10 个 SSE 客户端。
- 50 个重复事件。
- 1 GB 文件采用流式上传，不加载进内存。

观察：

- goroutine 泄漏。
- 连接池耗尽。
- Redis 锁未释放。
- SSE 重连风暴。
- 临时文件未清理。

## 13. 回归门禁

任何 Phase 完成后都必须运行：

```bash
cd aios-core && go test -race ./...
cd frontend && npm run build
```

在 P5 以后还必须运行：

```bash
./scripts/test-video-workflows.sh
```

在 P7 以后：

```bash
./scripts/test-electron-runner.sh
```

发布前：

```bash
./scripts/test-all.sh
```

## 14. Definition of Done

一个功能只有满足以下条件才算完成：

- 需求 ID 有对应实现。
- 有自动化测试。
- 错误路径有测试。
- API/Schema 文档更新。
- 日志包含 trace。
- 不记录 secret。
- Feature flag 行为验证。
- 旧功能回归。
- Agent 更新 TASK_STATUS。
- 无未解释的 race、panic、测试跳过。


---

# Coding Agent 主执行指令

你正在升级仓库 `SUSTechWLA/tangying-ai-operation-system` 的 `develop_go` 分支。

## 目标

按照本目录文档，将系统升级为能稳定运行以下两类视频生产工作流的自媒体 AIOS：

1. AIGC 镜头式视频。
2. 文字口播 + HyperGenKeyframe 可视化视频。

## 必读顺序

1. `README.md`
2. `01_REQUIREMENTS_AND_SCOPE.md`
3. `02_TECHNICAL_DESIGN.md`
4. `03_IMPLEMENTATION_PLAN.md`
5. `04_TEST_AND_ACCEPTANCE.md`
6. `TASK_STATUS.md` 或 `TASK_STATUS_TEMPLATE.md`

同时阅读仓库：

- 根目录 `AGENTS.md`
- 根目录 `CLAUDE.md`
- `aios-core/docs/AIOS_CORE_BACKEND_BOUNDARY.md`
- `aios-core/docs/ARCHITECTURE.md`
- `aios-core/docs/TOOL_DEVELOPMENT_GUIDE.md`

## 强制执行方式

### 1. 不得一次实现全部

从 P0 开始，一次只完成一个 Task。完成后：

- 运行该 Task 测试。
- 运行全量回归。
- 更新 `TASK_STATUS.md`。
- 输出变更摘要和风险。

### 2. 先检查实际代码

文档是设计目标，不替代源码事实。任何修改前：

- 定位当前包和接口。
- 搜索现有同类实现。
- 优先复用。
- 如果文档路径与源码不一致，保持架构边界并在状态文件记录差异。

### 3. 不重写已有核心

禁止重写：

- Orchestrator DAG。
- Worker/Tool Registry。
- Outbox。
- Rust Sandbox。
- 旧 Publish/Chat/Media API。

只做必要的接口扩展和依赖注入。

### 4. 测试优先

每个 Task：

1. 写或更新测试。
2. 实现最小代码。
3. 运行目标测试。
4. 运行 `go test -race ./...`。
5. 不得通过删除测试、跳过断言或调用真实收费 API 解决失败。

### 5. 数据兼容

- 新表优先。
- 旧表和 API 保留。
- migration 可重复。
- 新功能由 feature flag 控制。
- 默认关闭或不影响旧功能，直到对应 Phase 验收完成。

### 6. 外部模型

- 使用 Model Gateway。
- 自动测试只用 Fake Provider。
- 不把 Key、Token、base64 大结果写入日志。
- 长视频生成不得阻塞 HTTP Handler。

### 7. Electron 安全

- 不接受任意 Shell。
- 只执行 command type 白名单。
- 校验 project root 和路径。
- 使用 context isolation。
- 云端 Token 使用安全存储。

## 当前执行命令

第一次执行时只完成：

```text
P0-T01 仓库基线清点
```

不要开始 P1。生成：

```text
docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md
docs/upgrade/video-creation-v1/TASK_STATUS.md
```

报告至少包含：

- 当前 commit。
- 实际目录树。
- 现有测试清单。
- 后端测试结果。
- 前端 build 结果。
- 当前已知失败。
- 文档设计与实际代码差异。
- 下一任务建议。

## 每轮输出格式

```text
完成任务：
修改文件：
新增测试：
执行命令：
测试结果：
兼容性影响：
剩余风险：
下一任务：
```

遇到无法访问的外部服务时，使用 Fake/Mock 继续，不得把任务停在“等待用户提供 API Key”。
