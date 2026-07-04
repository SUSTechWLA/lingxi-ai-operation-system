# 视频抽帧 QA Wiki

本文说明 v0.1.10 的成片视觉 QA 链路。它的目标不是替代人工审片，而是在交付前把明显的文字遮挡、底部字幕拥挤、画面复杂度、剧本匹配、素材 provenance 和 shot 规格风险提前暴露出来，并把每个 shot 的问题量化成可用于返修的机器可读结论。

## 流程位置

视频创作 DAG 中，QA 位于本地渲染之后、发布文案之前：

```mermaid
flowchart LR
  A["脚本 / 分镜 / 提示词"] --> B["预览审核"]
  B --> C["本地渲染 final.mp4"]
  C --> D["VIDEO_FRAME_QA 抽帧"]
  D --> E["视觉 QA 人工审核门"]
  E --> F["发布文案 / 交付包"]
```

`visual_qa` 是系统自动补入的节点。用户不需要在提示词里要求 QA，只要走动态视频创作链路，render 成功后就会进入抽帧检查。

云端和 runner 仍通过 `VIDEO_FRAME_QA` 这个稳定本地命令调度，但实际 QA 能力已经拆成标准 MCP 服务：

```text
local runner VIDEO_FRAME_QA
  -> stdio MCP provider: mcp/video_qa/server.py
  -> MCP tool: video_qa.analyze_video
```

Go executor 只负责路径校验、`local://` 解析和 MCP 调用。抽帧、contact sheet、shot-level QA、`ShotQAReport` 和 `SHOT_REPAIR_PLAN` 都由 Python MCP server 生成。后续 OCR、ASR、PyIQA、VLM judge 等重媒体能力应继续加在 `mcp/video_qa/`，不要回到 Go 本地工具里直接堆实现。

## 产物

本地工具 `VIDEO_FRAME_QA` 会写入：

| 产物 | 路径 | 用途 |
|---|---|---|
| JSON 报告 | `local://projects/<projectId>/reports/video_frame_qa/video_frame_qa.json` | 结构化分数、问题、抽帧指标 |
| Shot QA 报告 | `local://projects/<projectId>/reports/video_frame_qa/shot_qa_reports.json` | 每个 shot 的 `ShotQAReport`、fatal gate、分项分数和工具级修复计划 |
| Shot 返修计划 | `local://projects/<projectId>/reports/video_frame_qa/shot_repair_plan.json` | 聚合后的 shot 决策和 repair action 映射 |
| Contact sheet | `local://projects/<projectId>/reports/video_frame_qa/contact_sheet.jpg` | 人工快速浏览所有采样帧 |
| 单帧图片 | `local://projects/<projectId>/reports/video_frame_qa/frames/frame_*.png` | 定位具体问题帧 |

contact sheet 会根据抽帧数量动态选择 tile，例如 8 张抽帧使用 `4x2`，避免固定 `4x4` 产生大块空黑区域。

## Shot 级量化输出

每个抽帧都会根据 `shotList` 映射回对应 shot。报告中的 `shotSummaries` 会为每个 shot 输出：

| 字段 | 说明 |
|---|---|
| `frameCount` | 当前 shot 被采样到的帧数 |
| `sampledTimesSec` | 当前 shot 的采样时间点 |
| `metricSummary` | 左上文字区、底部三分之一区域、全帧复杂度的平均值和最大值 |
| `blockingIssueCount` / `warningIssueCount` | 阻断问题和警告数量 |
| `score` | 当前 shot 的 0-100 分质量分 |
| `passed` | 当前 shot 是否通过 |
| `needsRegeneration` | 是否建议返修后重生成该 shot |
| `decision` | 标准化 shot 级决策，例如 `PASS`、`PASS_WITH_FIX`、`RERENDER_HTML`、`RECOMPOSITE`、`REVISE_SHOT_SPEC` |
| `repairAction` | 推荐工具动作，供 agent 决定重渲染 HTML、重合成、修订 shot spec 或人工复看 |
| `fatalGateTriggered` | 是否触发一票否决门禁 |
| `conclusion` | 给审核者看的明确结论 |
| `recommendations` | 可执行修复建议，例如减少左上叠字、压缩底部字幕、降低背景复杂度 |

`shotReports` 是更完整的机器可消费结构，包含：

| 字段 | 说明 |
|---|---|
| `schemaVersion` | 当前为 `1` |
| `mode` | `talking_head`、`cinematic` 或 `hybrid` |
| `overallScore` | 当前 shot 的综合分 |
| `decision` | `PASS`、`PASS_WITH_FIX`、`REGEN_AIGC`、`REGEN_AIGC_WITH_REFERENCE`、`RERENDER_HTML`、`RECOMPOSITE`、`REGENERATE_TTS`、`RERENDER_SUBTITLE`、`REVISE_SHOT_SPEC`、`HUMAN_REVIEW` 等 |
| `scores` | `mediaSpec`、`textLayout`、`imageQuality`、`temporalStability`、`promptAlignment`、`continuity` 等分项分数 |
| `hardMetrics` | 从抽帧和 spec lint 得到的硬指标，例如文字安全区密度、底部区域密度、时长、画面文字长度 |
| `issues` | 带证据和修复建议的问题列表 |
| `repairPlan` | 工具级修复计划，包含 `schemaVersion`、`action`、`targetShotId`、`candidateId`、`toolOverrides`、`renderStrategyPatch`、`visualPlanPatch`、`promptPatch` |
| `artifactRefs` | 当前 shot 相关素材引用，含 fallback / AIGC provenance 时用于反向指导重生成 |
| `timestamps` | QA 采样时间点和报告生成时间 |
| `shotSpecLint` | 生成前规格检查结果，用来发现过长 shot、精确文字、长画面文本和 continuity reference 需求 |
| `scriptAlignment` | 剧本、画面描述、拍摄理由和参考资产覆盖是否完整 |

生成前 `shotSpecLints` 先检查不需要视频文件的风险：

- `durationSec` 不在 3-15 秒范围内：建议 `REVISE_SHOT_SPEC`。
- `screenText` 过长：建议压缩文字并使用 HTML overlay。
- `mustBeExact=true`：强制 `RERENDER_HTML` / HyperFrames overlay，禁止 AIGC 内生关键文字。
- `mustMatchPrevious=true`：要求前一 shot 的 reference image 或 end state。
- `directorReason` / `whyThisShot` 缺失：建议补充拍摄理由，否则影视 shot 不能解释画面意义。
- `referenceAssetIds` 为空：建议回到参考资产阶段补充角色、场景或道具锚点。
- `actionBeats` 不足：建议补充 shot 内动作节奏，避免 AIGC 画面只有静态描述。

影视模式会额外输出硬指标：

| 指标 | 说明 |
|---|---|
| `scriptTextChars` | 当前 shot 对应剧本文字长度 |
| `visualTextChars` | 当前 shot 画面描述长度 |
| `scriptVisualCompletenessScore` | 剧本和画面信息完整度，0-100 |
| `directorReasoningPresent` | 是否存在拍摄理由 |
| `referenceCoverageCount` | 绑定的角色、场景、道具参考资产数量 |
| `actionBeatCount` | shot 内动作节拍数量 |

顶层 `repairPlan` 会把所有 shot 聚合成下一步动作：

- `approve`：所有 shot 通过，可以继续发布。
- `manual_review`：没有阻断问题，但部分 shot 有警告，需要人工复看。
- `regenerate_shots`：存在阻断问题，应先重生成指定 shot。

同时 `repairPlan` 会输出 `decisionByShot`、`repairActionByShot` 和 `repairActionCounts`，供后续 agent 做更细的自动返工，而不是只按整片总分判断。

Closed beta 当前确定性映射：

| 触发 | `decision` | `repairPlan.action` |
|---|---|---|
| 文字安全区、字幕或 UI 叠字 | `RERENDER_HTML` | `RERENDER_HTML` |
| fallback storyboard / preview 不能满足真实 AIGC 素材要求 | `REGEN_AIGC` | `REGEN_AIGC` |
| AIGC shot 缺少参考资产 | `REGEN_AIGC_WITH_REFERENCE` | `REGEN_AIGC_WITH_REFERENCE` |
| provider prompt 泄漏 `ffmpeg`、`AIGC_VIDEO`、`artifact` 等内部术语 | `REVISE_SHOT_SPEC` | `PROMPT_PATCH_REGEN` |
| 画面复杂度、叠层或合成风险 | `RECOMPOSITE` | `RECOMPOSITE`，并带 `toolAction=FFMPEG_RECOMPOSITE` |
| 严重渲染失败或指标不足以自动判断 | `HUMAN_REVIEW` | `HUMAN_REVIEW` |

## 检查维度

当前 QA 是确定性本地 MCP 检查，不依赖云端视觉模型：

| 维度 | 目的 | 典型问题 |
|---|---|---|
| 左上文字安全区 | 防止品牌、镜头编号、网页原始文字互相挤压 | logo 和标题叠在一起 |
| 底部三分之一区域 | 防止主标题、字幕、原片字幕重复覆盖 | 口播字幕压住按钮或页面文字 |
| 全帧复杂度 | 发现过度复杂的伪 UI、背景、密集文字 | 短视频用户一眼看不清主体 |

阻断问题会让 `passed=false`，并阻断发布文案节点。警告问题允许继续，但应该人工复看。

## 人工审查建议

QA 审核门中优先看三件事：

1. 首帧是否一眼能看懂项目卖点。
2. 每个 shot 的主标题是否和背景、字幕分离。
3. contact sheet 中是否存在转场期间的文字重影、过暗或过乱画面。

如果发现问题，应该回到脚本、分镜或模板层压缩文字，而不是只调低 QA 阈值。

## 本次验证记录

v0.1.5 已用 release 分支当前代码完整生成一条 18 秒、16:9 的正能量搞笑影视宣传短片：

- project: `vp-b1a3a300`
- run: `agent_run_5ff5fbfe-7a61-4250-a103-17a5077a3882`
- task: `20260704050335-c8c8c8c8`
- output: `final.mp4`
- video: `1920x1080`，`18.000000` 秒
- QA: `passed=true`，`score=100`，`shotCount=4`，`frameCount=5`，`blockingIssueCount=0`，`warningIssueCount=0`
- 每个 shot 的 `scriptVisualCompletenessScore=100`，`directorReasoningPresent=true`，`referenceCoverageCount=4`，`actionBeatCount=3`

这条链路覆盖故事大纲、详细剧本、角色/场景/道具档案、参考图 MCP、AIGC shot MCP、预览、渲染、抽帧 QA 和发布文案审核。

v0.1.3 已用系统完整生成一条 30 秒、16:9 的“视频 Agent”开源上线宣传视频：

- project: `vp-f894df0c`
- run: `agent_run_d3bcf6fb-46d7-46d6-bd0d-7e1821fc61d7`
- task: `20260703200752-f8f8f8f8`
- output: `final.mp4`
- duration: `30.000000` 秒
- QA: `passed=true`，`score=100`，`shotCount=6`，`frameCount=8`，`repairPlan.nextAction=approve`

这条链路覆盖了脚本审核、提示词审核、预览审核、渲染前审核、成片抽帧 QA 审核和发布文案审核。

## 验证命令

```bash
python3 mcp/video_qa/test_server.py
python3 -m py_compile mcp/video_qa/server.py

cd local-backend
RUN_PYTHON_MCP_INTEGRATION=1 go test ./internal/localmcp -run TestClientListsToolsFromVideoQAPythonMCPServer -count=1
RUN_VIDEO_QA_MCP_E2E=1 go test ./internal/localtool -run TestVideoFrameQAExecutorRunsDefaultVideoQAMCPServer -count=1
go test ./internal/localtool ./internal/localagent ./internal/localrunner -count=1
```
