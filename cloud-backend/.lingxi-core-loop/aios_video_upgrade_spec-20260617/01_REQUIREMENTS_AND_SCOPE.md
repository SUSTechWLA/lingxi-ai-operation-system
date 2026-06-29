# AIOS 自媒体视频创作能力升级——需求规格说明

## 1. 文档目的

本说明用于指导 Coding Agent 在现有 `develop_go` 分支上完成增量升级。目标不是重写视频创作 Agent 平台，而是把两套已经通过 Codex Skill 实际验证的视频生产方法，转化为可持续运行、可审核、可恢复、可局部重跑的正式产品能力。

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
- 视频创作 Model Gateway 与 Fake Provider。
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
5. **Core 与业务隔离**：视频创作 Runtime 放 `internal/core`；视频领域放 `internal/agents/video`。
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
