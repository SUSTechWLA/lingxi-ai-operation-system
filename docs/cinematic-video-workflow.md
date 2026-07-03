# 影视类视频创作流程 Wiki

本文说明 `cinematic_story` 模式。它面向剧情短片、影视化项目介绍、AIGC shot 视频，不再把“分镜”当成简单画面列表，而是把剧本、角色、场景、道具、参考图、shot 设计和 QA 串成一条可追踪流水线。

## 核心原则

1. 先有故事大纲，再有详细剧本。
2. 先有角色、场景、道具档案，再生成全局参考图。
3. 每个 shot 必须能解释“为什么这样拍”，不能只有画面描述。
4. AIGC 负责情绪、动作和视觉隐喻；HyperFrames 负责精确文字、UI、字幕和安全包装。
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
