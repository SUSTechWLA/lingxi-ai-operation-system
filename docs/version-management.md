# 版本管理 Wiki

本文是仓库内的 Wiki 源文档。GitHub Wiki 更新时应与本文保持一致。

## 分支角色

| 分支 | 定位 | 使用规则 |
|---|---|---|
| `develop_go` | 开发者分支，口头简称 `developgo` | Go/核心系统、云端编排、本地 runner、桌面端联调都先进入这里 |
| `feature/*` | 功能分支 | 从 `develop_go` 拉出，完成后合回 `develop_go` |
| `hotfix/*` | 发布修复分支 | 从 `release` 或当前 tag 拉出，只修阻断问题 |
| `release` | 发布分支 | 只接收已验证的核心代码和发布文档 |

`release` 不直接承载日常开发。临时脚本、渲染缓存、E2E 产物、测试账号 token 不进入 `release` 提交。

## 合入 release 的硬性要求

合入 `release` 前必须完成：

1. 核心链路验证通过，至少包含相关 Go 单测、前端逻辑测试和构建。
2. README 的「Release 更新」追加本次版本的用户可读更新内容。
3. Wiki 源文档同步更新。涉及架构、流程、版本规则、MCP 或 QA 的改动都要写入 `docs/`。
4. 只 stage 本次发布需要的源码和文档，不 stage `tmp/`、`promo/`、本地渲染产物或用户私有配置。
5. 合并到 `release` 后创建语义化 tag。

## Tag 规则

使用语义化版本：

| 类型 | 示例 | 场景 |
|---|---|---|
| patch | `v0.1.3` | bugfix、小功能、文档和兼容增强 |
| minor | `v0.2.0` | 新增稳定用户能力或较大工作流能力 |
| major | `v1.0.0` | 对外 API、数据结构或使用方式出现破坏性变化 |

tag 必须指向 `release` 上的发布提交。不要移动已发布 tag；如果发布内容有误，创建新的 patch 版本。

## 发布记录规范

README 的 release note 要写给用户看，不写内部流水账。每条说明应回答：

- 用户现在能做什么。
- 哪些阻断问题被修复。
- 是否影响使用方式、配置方式或兼容性。
- 是否需要重新启动 cloud backend、local agent 或 frontend。

## 当前发布检查清单

v0.1.10 对应能力：

- Closed beta runbook 已补齐：支持平台、依赖安装、cloud/local/frontend/HyperFrames 启动、FFmpeg、MCP provider、JiMeng/Dreamina、OpenAI-compatible model provider、环境变量、日志、诊断包、fallback 判断和已知限制。
- 新增 beta smoke 和 fallback fixture：无真实 AIGC provider 时可验证 2-shot fallback 预览、artifact provenance、shot QA report 和 machine-readable repairPlan。
- 新增 beta readiness gate：`BETA_READINESS_REQUIRE_AIGC=1 bash scripts/beta-readiness-check.sh` 返回 `GO` 才能邀请真实创作者试用“一句话生成高质量真实 AIGC 视频”；`CONDITIONAL` 只代表 fallback 预览和工程链路可验证。
- Artifact provenance 增加 `schemaVersion`、`sourceType`、`providerName`、`providerJobId`、`fallbackReason`、`isFallback`、`generatedAt`、`inputPromptHash` 和 `sourceArtifactIds`，前端明确标出 fallback storyboard/preview。
- Video QA MCP 输出结构化 shot report、决策枚举和 `repairPlan`，可反向指导 AIGC 重生成、带参考重生成、HTML 重渲染、FFmpeg recomposite、prompt 修订或人工审核。
- Local agent 新增 `beta-diagnostics.zip`，包含脱敏环境、日志、MCP provider 状态、artifact manifest、QA 报告和失败栈索引，默认不打包用户原始素材。
- Release/production 启动增加安全 fail-fast：拒绝弱密钥、默认数据库/MinIO/admin token、通配 CORS、禁用 sandbox 或启用 sandbox fallback。

发布 v0.1.10 tag 前必须确认：

1. README 已追加 v0.1.10 release note，release badge 已更新。
2. `docs/BETA_RUNBOOK.md`、`docs/mcp-providers.md`、`docs/video-frame-qa.md` 和本文已同步 closed beta 状态。
3. `bash scripts/beta-smoke-check.sh` 通过，允许本地开发环境出现 production env validation skipped warning。
4. `python3 -m unittest scripts/test_beta_readiness.py` 和 `python3 -m unittest discover -s mcp/video_qa -p 'test*.py'` 通过。
5. 邀请真实创作者前，必须在已启动 local agent、HyperFrames、MCP provider 和模型 provider 的环境中运行 `BETA_READINESS_REQUIRE_AIGC=1 bash scripts/beta-readiness-check.sh` 并得到 `GO`。
6. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
7. `git tag v0.1.10 <release_commit>` 只在 release 提交后创建。

v0.1.9 对应能力：

- 发布链路加固：CI 覆盖 Go test/vet、前端 lint/build、Electron runtime 测试、HyperFrames Render Service build 和生产依赖审计。
- 生产配置 fail-fast：云端启动校验 `AUTH_TOKEN_SECRET`、数据库和 OpenAI 配置；Docker 镜像不再复制 `.env`。
- 本地执行边界 fail-closed：需要 sandbox 的工具在 sandbox 不可用时阻断；Electron 文件读取只允许用户明确授权路径。
- 清理无用代码和依赖：移除未调用 helper、孤立前端工具文件和冗余 package 依赖，补齐直接使用的 `esbuild`。

发布 v0.1.9 tag 前必须确认：

1. README 已追加 v0.1.9 release note。
2. GitHub Wiki 首页、英文首页和版本管理页已同步 v0.1.9 状态。
3. Go、前端、渲染服务、root smoke 和 Electron build 验证通过。
4. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
5. `git tag v0.1.9 <release_commit>` 只在 release 提交后创建。

v0.1.8 对应能力：

- 本地 `LOCAL_MCP_TOOL_CALL` 对每个 `kind=video` 请求增加 preflight QA，调用 Dreamina/JiMeng 前先检查提示词清晰度和参考素材可用性。
- 不清晰提示词、泄漏内部生产术语、缺少时间段画面变化、缺少可用参考图的请求会进入 `blocked` 状态，不调用 provider，不消耗视频额度。
- `generationResults` 和 `assetProvenance` 必须带 `preflightQa`，用于解释阻断原因和指导返修。

发布 v0.1.8 tag 前必须确认：

1. README 已追加 v0.1.8 release note。
2. `docs/mcp-providers.md` 和 `docs/cinematic-video-workflow.md` 已说明 AIGC 调用前 QA。
3. 本地 MCP 执行器测试覆盖不清晰 prompt 被阻断、不可用参考素材被阻断、清晰 prompt 才允许调用 provider。
4. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
5. `git tag v0.1.8 <release_commit>` 只在 release 提交后创建。

v0.1.7 对应能力：

- Dreamina/JiMeng 视频投放 prompt 改为 Vibe Creator 画面叙述：按时间段写具体主体、道具、场景、变化和表达思想。
- 外部视频模型 prompt 禁止泄漏 `AIGC_VIDEO`、`b-roll`、`ffmpeg`、`SHOT_VIDEO_CLIP`、`素材意图` 等内部生产标签。
- 导演字段、口播意图、趣味节拍和 QA 目标必须先转写成具体画面，再进入 `externalGenerationRequests[].prompt`。

发布 v0.1.7 tag 前必须确认：

1. README 已追加 v0.1.7 release note。
2. `docs/cinematic-video-workflow.md` 和 `docs/mcp-providers.md` 已说明 Dreamina 投放 prompt 契约。
3. 回归测试覆盖“坏 prompt 不再包含内部工程说明，而是包含时间段画面故事”。
4. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
5. `git tag v0.1.7 <release_commit>` 只在 release 提交后创建。

v0.1.6 对应能力：

- MCP AIGC 生成结果必须输出 `sourceSummary` 和 `assetProvenance`，让用户能直接看到哪些素材真实来自 provider，哪些请求失败、延迟或需要 fallback。
- 自动插入的 `mcp_generation_runner` 默认携带 `minReadyVideoGenerations=1`，避免 Dreamina/JiMeng 视频全失败时仍把 HyperFrames/storyboard fallback 误认为即梦成片。
- 即梦素材本地路径规范为 `TangyingAIOS/cache/mcp/<projectId>/<requestId>/` 和 `TangyingAIOS/artifacts/<projectId>/<requestId>/content`。
- 版本文档、MCP provider 文档和影视流程文档都要说明 ready / failed / deferred / fallback 的区别。

发布 v0.1.6 tag 前必须确认：

1. README 已追加 v0.1.6 release note。
2. `docs/mcp-providers.md` 和 `docs/cinematic-video-workflow.md` 已说明素材来源、路径和 fallback 规则。
3. 本地 MCP 执行器测试覆盖“视频请求全失败时 sourceSummary 明确标记未满足 ready 视频要求”。
4. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
5. `git tag v0.1.6 <release_commit>` 只在 release 提交后创建。

v0.1.5 对应能力：

- `cinematic_story` 影视创作模式已进入 release：故事大纲、详细剧本、角色/场景/道具档案、参考图、shot 设计、MCP 生成、render、QA 和发布文案全链路可跑通。
- 口播和影视视频都坚持“脚本先行，素材跟随脚本”的生成顺序，避免直接用单调 HyperFrames 堆页面。
- CLI/AIGC provider 统一通过 `mcp/` 下的标准 MCP 服务接入；云端只使用通用 `mcp_generation_runner`，本地只使用 `LOCAL_MCP_TOOL_CALL`。
- JiMeng/Dreamina 图片请求走 `jimeng.generate_image`，视频请求走 `jimeng.generate_video`；图片 `resolution_type` 已按 Dreamina MCP 要求归一为 `2k/4k`。
- Shot-level QA 增加剧本匹配、导演理由、参考资产覆盖、动作节拍等指标，最终输出 `SHOT_QA_REPORT` 和 `SHOT_REPAIR_PLAN`。
- release 分支验证记录：`vp-b1a3a300`，run `agent_run_5ff5fbfe-7a61-4250-a103-17a5077a3882`，29 个节点全成功，最终视频 1920x1080 / 18 秒，QA `score=100`。

发布 v0.1.5 tag 前必须确认：

1. README 已追加 v0.1.5 release note。
2. `docs/cinematic-video-workflow.md`、`docs/mcp-providers.md`、`docs/video-frame-qa.md` 已同步更新。
3. `promo/`、`scripts/tmp/`、本地 token、渲染缓存没有进入 staged changes。
4. `git tag v0.1.5 <release_commit>` 只在 release 提交后创建。

v0.1.4 对应能力：

- 动态视频计划在 render 后自动插入 `visual_qa`。
- 本地 `VIDEO_FRAME_QA` 通过标准 stdio MCP 调用 `mcp/video_qa/server.py`，Go runner 只做路径解析和 MCP 桥接。
- Python MCP 工具 `video_qa.analyze_video` 输出 JSON 报告、抽帧图片、contact sheet、shot 级质量摘要和返修计划。
- 每个 shot 都输出 `metricSummary`、`score`、`passed`、`needsRegeneration`、`conclusion` 和 `recommendations`。
- 顶层 `repairPlan` 输出 `approve`、`manual_review` 或 `regenerate_shots`，并列出需要返修的 shot。
- QA 审核门阻断 publish，人工确认后才继续发布文案。
- 本地 artifact 的 `localPath` 可被下游本地工具解析为真实视频路径。
- `SHOT_QA_REPORT` 和 `SHOT_REPAIR_PLAN` 作为稳定 artifact 输出，前端审核面板优先展示 shot 级结论。
