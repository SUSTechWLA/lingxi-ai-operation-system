# 影视类视频创作流程 Wiki

本文说明 `cinematic_story` 模式。它面向剧情短片、影视化项目介绍、AIGC shot 视频，不再把“分镜”当成简单画面列表，而是把剧本、角色、场景、道具、参考图、shot 设计和 QA 串成一条可追踪流水线。

## 核心原则

1. 先有故事大纲，再有详细剧本。
2. 先有角色、场景、道具档案，再生成全局参考图。
3. 每个 shot 必须能解释“为什么这样拍”，不能只有画面描述。
4. AIGC 负责无文字背景、局部动态、情绪、动作和视觉隐喻；HyperFrames 负责精确文字、UI、字幕、关键帧和安全包装；FFmpeg 负责把两层融合成完整 shot。
5. 每个 shot 都要 QA，QA 结论必须能指导返修或重生成。

## 标准链路

```mermaid
flowchart LR
  A["一句话影视需求"] --> B["故事大纲"]
  B --> C["详细剧本"]
  C --> D["角色 / 场景 / 道具档案"]
  D --> E["多视角参考图 MCP"]
  E --> F["Cinematic shot 设计"]
  F --> G["3-15s 时间窗"]
  G --> H["关键帧 / storyboard"]
  H --> I["Dreamina AIGC shot MCP"]
  I --> J["HyperFrames 包装渲染"]
  J --> K["Shot-level QA"]
  K --> L["发布文案 / 交付"]
```

## 剧本阶段

`video_script_generator` 在影视模式下必须输出：

| 输出 | 说明 |
|---|---|
| `storyOutline` | logline、主题、主要 beat、结尾 |
| `detailedScript` | 可拍摄的详细剧本，不只是一段口播 |
| `characters` | 主要角色档案，包含不变量和参考视角 |
| `scenes` | 主场景档案，包含空间锁定信息 |
| `props` | 核心道具档案，包含外观和用途 |
| `scriptSpans` | 可切成 3-15 秒 shot 的剧本片段 |

如果 LLM 输出漂移，系统会补充确定性 fallback，保证后续参考资产和 shot 设计不断链。

## 参考资产阶段

`reference_asset_planner` 为每个主要角色、场景、道具生成全局一致性参考资产：

- 角色：正面、侧面、背面、表情或动作细节。
- 场景：主视角、反打、侧视、空间关系。
- 道具：正面、侧面、背面、局部细节。

参考图通过标准 MCP 调用：

```text
providerId: jimeng
toolName: jimeng.generate_image
kind: image
resolution_type: 2k 或 4k
```

当前 release 验证中，Dreamina 图片 MCP 已成功生成 1 个参考图，其余请求按 `maxReadyGenerations` 预算 defer。系统会把生成结果打包成 `REFERENCE_IMAGE`，后续 shot 可引用这些全局参考资产。

## Shot 设计阶段

每个 shot 必须包含：

| 字段 | 目的 |
|---|---|
| `durationSec` | 3-15 秒，避免 AIGC 长镜头失控 |
| `visual` / `mainAction` | 画面主体和动作 |
| `camera` | 运镜，例如推近、横移、跟随、稳定定格 |
| `shotSize` / `composition` / `framing` | 景别、构图、画面范围 |
| `lighting` | 光影和情绪 |
| `whyThisShot` / `directorReason` | 解释这个画面为什么有意义 |
| `actionBeats` | shot 内 0-2s、2-4s、收束段动作节奏 |
| `referenceAssetIds` | 绑定角色、场景、道具参考资产 |
| `plannedAssetRoute` | `aigc_video`、`screen_recording` 或 `hyperframes` |

## 生成策略

| 素材类型 | 推荐工具 | 使用边界 |
|---|---|---|
| 精确标题、UI、流程图、CTA | HyperFrames | 不让 AIGC 内生小字和 UI |
| 页面演示 | 录屏 / HyperFrames | 展示真实系统状态 |
| 角色、场景、道具设定图 | Dreamina `generate_image` MCP | 生成全局参考图 |
| 情绪化动作、无厘头视觉隐喻 | Dreamina `generate_video` MCP | 生成 3-15 秒独立 shot |

每个 Shot 都会产出统一的三层画面设计和合成计划；是否调用 AIGC 不影响设计是否存在：

| 字段 | 说明 |
|---|---|
| `visualLayers` | `shot_visual_layers_v1` 统一契约，包含 `ipAroll`、`hyperframes`、`aigc`、`composition`，并记录每层的 `designed`、`enabled`、`required`、`executionPolicy`、安全区和时间窗。 |
| `ipArollPlan` | 正式 3D IP 拍摄为 2D A-roll 的角色层计划，描述口播、口型、眼神、表情、动作、构图安全区和正式资产来源。 |
| `aigcPlan` | 给 Dreamina/JiMeng 或其他外部平台的 AIGC 视频层提示词。它来自 shot 画面说明和动作节奏，但只要求生成无文字背景或局部动态素材，并明确预留文字安全区。 |
| `hyperframesPlan` | 本地 HyperFrames / HyperKeyframes 文字特效层计划。中文标题、字幕、关键帧、流程标签、UI 卡片和精确排版都在这里处理。 |
| `ffmpegFusionPlan` | 合成计划。系统按同一 Shot 时间窗融合 IP A-roll、AIGC 背景/B-roll/局部动态和 HyperFrames 文字特效，统一规格并输出完整 Shot。 |

如果画面包含重要文字，优先让 AIGC 参考图或视频在文字区域留白，文字由 HyperFrames 或 final subtitle 渲染，避免乱码和错误汉字。AIGC 可以生成完整背景，也可以只生成画面中的 B-roll 或局部视频窗口；最终完整 Shot 与 IP A-roll 和文字特效层共同合成。纯本地模式下 `aigcPlan` 仍保留，但 `enabled=false`、`executionPolicy=disabled`，系统不得创建外部生成请求。

## Shot 产物工作台

产物页不是底层 artifact 列表，而是创作者逐步校验单个 shot 的工作台。LLM 会在项目开始时判断视频类型，进入 shot 后自动切换展示重点：

| 视频类型 | 用户首先看到 | AIGC 角色 | HyperFrames 角色 | 一致性要求 |
|---|---|---|---|---|
| 口播 / 知识类 | 口播稿、正式 IP A-roll、HyperFrames 时间线、AIGC 插入位置 | 为口播提供 b-roll、背景、局部动态和情绪素材，不承担跨 shot 主连续性 | 承担标题、字幕、图表、UI 卡片、关键帧和可控文字层 | IP 角色和声音保持连续；素材服务表达，不追求 AIGC 角色连续性 |
| 影视 / AIGC shot | 剧本片段、角色 / 场景 / 道具参考、故事板、AIGC 主画面提示词 | 承担主画面、人物动作、场景氛围、运镜和文学化情绪表达 | 只承担字幕、小号说明、安全区压边和少量可控图形 | 必须保持跨 shot 的角色、场景、道具和物理状态连续 |

每个 shot 按 1-6 线性步骤展开。上传、回填、提示词复制、参考图查看、字幕校对和最终视频预览都放在对应步骤内，不另设“高级回填”入口。用户可以从上往下逐步理解创作过程，也可以折叠已经确认的步骤。

用户可见产物应直接展示内容：

- 图片参考图、故事板和首帧以缩略图展示，支持点击放大，并可基于选中内容生成局部返工提示词。
- 视频素材、HyperFrames overlay 和完整 shot 以可播放视频展示，支持大弹窗播放。
- 字幕文件解析成时间轴，便于直接校对时间和文字。
- 正常成功态不反复显示 `已登记`、`预览已就绪`、`local://...` 或后端枚举；页面只保留用户需要操作或判断的业务状态。

Dreamina/JiMeng 视频调用必须使用 AIGC 层提示词。系统会优先读取 `aigcPrompt`、`aigcVideoPrompt` 或 `aigcPlan.prompt`，只有旧产物缺失这些字段时才回退到 legacy `prompt` / `promptText` / `videoPrompt`。HyperFrames 提示词只用于本地文字、字幕、UI 和关键帧层，不能作为 AIGC 视频模型 prompt。

## Dreamina 投放 Prompt

投放给 Dreamina/JiMeng `generate_video` 的 prompt 是 AIGC 视频层叙述，不是完整成片说明。用户端会同时展示 HyperFrames 和 FFmpeg 分工，但复制到外部 AIGC 平台时应聚焦背景或局部动态素材。

必须写清：

- 这个片段要表达的思想。
- 画面里具体有什么主体、道具、场景和符号。
- 每个时间段发生什么变化，例如 `0-2秒`、`2-4秒`、`4-6秒`。
- 最后半秒画面如何稳定收束，让观众看清结果。
- 非真人风格化、积极、干净、明亮的整体气质。
- 文字安全区在哪里，哪些区域要保持干净、纯色或弱纹理。
- 不要生成文字、字幕、Logo、水印、UI 文案或可读汉字，避免乱码。

禁止混入：

- `AIGC_VIDEO`、`b-roll`、`SHOT_VIDEO_CLIP` 等内部标签。
- `ffmpeg`、`simple_cut`、拼接、artifact、storageRef 等工程说明。
- “画面需包含主体、场景、动作、镜头运动...” 这种模板化检查句。
- 纯镜头技术参数。需要保留的导演意图应转写成观看体验和画面变化。

## AIGC 调用前 QA

每个 `kind=video` 请求在调用 Dreamina/JiMeng 前必须通过本地 preflight QA。QA 不通过时状态为 `blocked`，不会调用 provider，也不会消耗视频额度。

最低要求：

- 用户只看 prompt 就能脑补出画面是什么样子。
- prompt 至少包含两个明确时间段，说明画面如何变化。
- 有具体主体、道具、场景、动作和情绪，不只写“轻松视觉隐喻”。
- 不能出现 `ffmpeg`、`AIGC_VIDEO`、`b-roll`、`SHOT_VIDEO_CLIP`、artifact 等内部说明。
- 如果声明了参考资产，必须有可用的参考图路径或 URL；`manual://` 占位不允许直接进入视频生成。

QA 结果写入 `generationResults[].preflightQa` 和 `assetProvenance[].preflightQa`，用于审核和返修。

Dreamina 视频 MCP 如果返回余额不足、并发限制或超时，系统保留 `externalGenerationRequests`，并可以继续使用 storyboard / HyperFrames fallback 生成可 QA 的成片。但 fallback 必须被明确标记，不能被当成 AIGC 视频素材交付。

v0.1.6 起，`mcp_generation_runner` 会输出：

- `sourceSummary.readyVideoCount`
- `sourceSummary.videoRequestCount`
- `sourceSummary.externalVideoRequirementSatisfied`
- `sourceSummary.fallbackRequired`
- `assetProvenance[]`

影视模式默认至少要求 1 个 ready 的 MCP 视频素材。如果 `readyVideoCount=0` 且 `videoRequestCount>0`，则这次成片只能算 fallback 预览，应该提示用户充值、重试、降低请求数，或改为纯 HyperFrames/录屏路线。

## QA 质量门

影视模式的 `video_qa.analyze_video` 不只看整片总分。它会按 shot 输出：

- 剧本与画面描述完整度：`scriptVisualCompletenessScore`
- 导演理由是否存在：`directorReasoningPresent`
- 参考资产覆盖数：`referenceCoverageCount`
- 动作节拍数：`actionBeatCount`
- 文字安全区、底部区域和全帧复杂度
- `PASS`、`PASS_WITH_FIX`、`REVISE_SHOT_SPEC`、`RERENDER_HTML`、`RECOMPOSITE` 等返修动作

发布文案必须等 `visual_qa` 审核门通过后才继续。

## v0.1.6 素材来源规则

影视视频最终交付时，必须能回答：

| 问题 | 检查字段 |
|---|---|
| 哪些素材真的来自即梦？ | `assetProvenance[].providerId=jimeng` 且 `status=ready` |
| 文件在哪里？ | `assetProvenance[].storageRef` / `localPath` |
| 有几个即梦视频片段 ready？ | `sourceSummary.readyVideoCount` |
| 是否只是 fallback 成片？ | `sourceSummary.fallbackRequired=true` 或 `externalVideoRequirementSatisfied=false` |
| 为什么没用上即梦视频？ | `assetProvenance[].error` / `reason` |

## v0.1.5 验证记录

- project: `vp-b1a3a300`
- run: `agent_run_5ff5fbfe-7a61-4250-a103-17a5077a3882`
- task: `20260704050335-c8c8c8c8`
- output: `final.mp4`
- video: 1920x1080, 18 秒, 16:9
- QA: `passed=true`, `score=100`, `shotCount=4`, `frameCount=5`, `blockingIssueCount=0`, `warningIssueCount=0`
- MCP: `jimeng.generate_image` 成功 ready 1 个参考图，`jimeng.generate_video` 因 Dreamina 账户余额不足返回 `CreditPreDeductNotEnough`，没有 ready 视频素材；该成片应视为 fallback 预览，不应视为即梦视频素材主导的成片。
