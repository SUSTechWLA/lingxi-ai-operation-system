# MCP Provider 接入

躺营本地 Agent 只依赖统一的 MCP provider 配置，不关心 provider 是用 Python、Node、Go 还是其他语言实现。一个 provider 只需要能通过标准 MCP JSON-RPC 暴露 `tools/list` 和 `tools/call`。

## Provider 配置

配置通过本地 Agent API 保存：

```bash
curl -X PUT http://127.0.0.1:18080/api/local/mcp-providers \
  -H 'Content-Type: application/json' \
  -d '{
    "providers": [
      {
        "id": "jimeng",
        "label": "JiMeng MCP",
        "transport": "stdio",
        "command": "python3",
        "args": ["/absolute/path/to/mcp/jimeng/server.py"],
        "toolPrefix": "jimeng.",
        "enabled": true
      }
    ]
  }'
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | provider 逻辑 ID。运行计划通过它选择调用哪个 MCP。 |
| `label` | UI 展示名称。 |
| `transport` | `stdio` 或 `http`。不填时，有 `command` 默认 `stdio`，否则默认 `http`。 |
| `endpoint` | HTTP JSON-RPC provider 地址，仅 `http` transport 使用。 |
| `command` / `args` | stdio MCP server 启动命令。stdout 必须保留给 MCP 协议。 |
| `env` | 传给 stdio 子进程的环境变量。 |
| `workingDir` | stdio 子进程工作目录。 |
| `toolPrefix` | 逻辑工具名前缀。例：系统调用 `jimeng.generate_video`，远端实际收到 `generate_video`。 |
| `toolNameMap` | 显式工具名映射。例：`{"jimeng.generate_video":"video_create"}`。优先级高于 `toolPrefix`。 |
| `enabled` | 是否启用。 |
| `enabledTools` | 允许暴露的逻辑工具名或远端工具名白名单。不填表示 provider 内工具默认可用。 |
| `disabledTools` | 禁用的逻辑工具名或远端工具名黑名单，优先级高于 `enabledTools`。 |
| `timeout` | provider `tools/list` / `tools/call` 默认超时时间，单位秒。 |
| `approvalMode` | provider 级审批模式，用于 Guard / UI 判定，常见值：`none`、`before_execute`、`always`。 |

## tools/list 到 ToolManifest

本地 Agent 对 enabled provider 执行 MCP `initialize` 后调用 `tools/list`。每个 MCP tool 会被归一化为系统唯一逻辑工具模型 `ToolManifest`：

| ToolManifest 字段 | 来源 |
|---|---|
| `name` | `toolNameMap` 反向映射优先，否则 `toolPrefix + remoteToolName`。 |
| `boundary` | 固定为 `mcp_provider`。 |
| `type` | `mcp`。 |
| `executionPlane` | `local`。 |
| `requiresUserDevice` | `true`。 |
| `localCommand` | 固定为 `LOCAL_MCP_TOOL_CALL`。 |
| `provider` / `providerBinding.providerId` | provider `id`。 |
| `providerBinding.remoteToolName` | MCP `tools/list[].name`。 |
| `providerBinding.targetRunnerId` | 本次 Agent run 选中的在线 runner。 |
| `providerBinding.catalogRevision` | 该 run 固化的 SHA-256 catalog revision。 |
| `parameters` / `output` | 从 MCP `inputSchema` / `outputSchema` 的 JSON Schema properties 转成 `ParamDef`；原始 schema 保留在 `providerCapabilities`。 |
| `capabilities` | 根据 provider id、工具名和描述做保守推断。不要在 provider 侧伪装业务决策能力。 |

Planner 只能看到逻辑工具和能力；PlanCompiler 对 `mcp_provider` 工具统一编译为本地 `LOCAL_MCP_TOOL_CALL` payload。不要新增 `RUN_X_PROVIDER_CLI`、`LOCAL_JIMENG_*`、`LOCAL_VIDEOQA_*` 这类 provider-specific local runner command。

这些 manifest 不是全局注册项。Runner 心跳只上报脱敏且有界的 catalog；云端根据当前已认证用户、设备和可选 runner 为单次 Agent run 建立不可变快照，Planner、Guard 与 Compiler 全程使用同一份快照。用户 A 的 catalog 不会出现在用户 B 的候选工具中；同一用户多个设备暴露同名逻辑工具时，未明确选择设备/runner 会返回 `MCP_TOOL_AMBIGUOUS`。DAG 只保存执行所需的 runner、revision 和 provider/tool 绑定，不保存 provider command、环境变量或 HTTP Header。

派发 `LOCAL_MCP_TOOL_CALL` 前，云端会再次检查 task owner、runner 在线状态、catalog revision 和 provider/logical/remote 精确绑定。revision 已变化时返回 `MCP_CATALOG_STALE`，调用方必须重新规划；不会自动换用最近心跳或同名 sibling runner。IP Avatar 等内置 provider 也必须先由真实本地 runner 上报，不存在云端硬编码的用户工具 manifest。

校验通过后，cloud-owned local job 会保存该绑定的非 secret `inputSchema` / `outputSchema` 与 revision 快照。完成回调使用这份不可变快照校验 MCP `structuredContent`，不从全局 registry 查找动态工具，也不会因后续 heartbeat 更新而更换结果契约。`isError=true` 或 output schema 不合格均按失败处理。

## 即梦 Python MCP

仓库内置了标准 stdio MCP server：

```bash
python3 -m pip install -r mcp/jimeng/requirements.txt
python3 mcp/jimeng/server.py
```

它封装用户本机的 Dreamina CLI，工具包括：

| 逻辑工具名 | Python MCP 工具名 |
|---|---|
| `jimeng.check_status` | `check_status` |
| `jimeng.login_headless` | `login_headless` |
| `jimeng.check_login` | `check_login` |
| `jimeng.generate_image` | `generate_image` |
| `jimeng.generate_video` | `generate_video` |
| `jimeng.query_result` | `query_result` |
| `jimeng.list_task` | `list_task` |

快速注册即梦 stdio provider：

```bash
curl -X POST http://127.0.0.1:18080/api/local/jimeng/setup/register-mcp \
  -H 'Content-Type: application/json' \
  -d '{"transport":"stdio"}'
```

本地 Agent 会自动生成 `python3 <repo>/mcp/jimeng/server.py` 的 provider 配置，并写入 `toolPrefix: "jimeng."`。

Provider 的 `env` 与 `headers` value 只写不回显；公开设置、状态和诊断接口只返回是否已配置及排序后的 key 名称。更新时省略这两个字段会保留已有 secret，显式传入 `{}` 才会清除。`approvalMode` 省略或为空时会归一为 `before_execute`，只有显式 `none` 才表示无需执行前审批。

## 视频素材路由

未来所有 CLI 模式的 AIGC 接入都必须走 MCP provider，不再给每个 CLI 新增本地工具命令。云端运行计划只指定 `providerId`、`mcpTool` 和 `externalGenerationRequests`；本地 runner 统一执行 `LOCAL_MCP_TOOL_CALL`。

当前视频创作链路的标准路由：

| 阶段 | kind | providerId | MCP tool | 说明 |
|---|---|---|---|---|
| 角色/场景/道具参考图 | `image` | `jimeng` | `jimeng.generate_image` | 生成全局一致性多视角参考图 |
| shot 关键帧 | `image` | `jimeng` | `jimeng.generate_image` | 生成每个 shot 的视觉锚点 |
| AIGC shot 视频 | `video` | `jimeng` | `jimeng.generate_video` | 生成 3-15 秒独立视频片段 |
| 结果查询/下载 | - | `jimeng` | `jimeng.query_result` | 轮询并导入本地 artifact |

`LOCAL_MCP_TOOL_CALL` 会根据请求 `kind` 自动把默认视频工具切换为图片工具。例如默认 `mcpTool=jimeng.generate_video`，当请求 `kind=image` 时会调用 `jimeng.generate_image`。

### 视频 Prompt 契约

`kind=video` 的 `prompt` 必须是可直接投放给视频模型的画面故事，不是系统内部说明。它应该像短片片段描述：

```text
非真人风格化动画，16:9 横屏，画面干净明亮。
这个画面表达：灵感被清楚流程轻松送到成片。
0-2秒：创作桌上，一颗写着“想法”的小星星被脚本纸、分镜卡和抽帧 QA 放大镜围住。
2-4秒：桌面打开成迷你传送带，“脚本”“分镜”“即梦素材”“抽帧 QA”四个小工位依次亮起。
4-6秒：传送带尽头弹出视频胶囊和开源星标，小机器人挥手，画面明亮稳定。
```

不要把 `AIGC_VIDEO`、`b-roll`、`ffmpeg`、`SHOT_VIDEO_CLIP`、`素材意图`、`镜头运动`、artifact、storageRef 或拼接说明写进 provider prompt。导演字段、口播意图和 QA 目标应该先被编译成具体可见画面，再交给 MCP provider。

### 调用前 Preflight QA

视频额度很贵，`LOCAL_MCP_TOOL_CALL` 会在真正调用 provider 前对每个 `kind=video` 请求做本地 QA。QA 不通过时，请求状态为 `blocked`，不会调用 Dreamina/JiMeng，不会消耗额度。

硬门检查：

| 检查项 | 阻断条件 |
|---|---|
| 提示词长度 | 太短，用户看完无法脑补出画面 |
| 时间段故事 | 缺少至少两个类似 `0-2秒`、`2-4秒` 的画面变化段 |
| 视觉锚点 | 缺少具体主体、道具或场景 |
| 动作变化 | 缺少“出现、打开、亮起、弹出、排队、收束”等可见变化 |
| 情绪/表达 | 缺少情绪氛围或表达思想 |
| 内部术语 | 出现 `ffmpeg`、`AIGC_VIDEO`、`b-roll`、`SHOT_VIDEO_CLIP`、`artifact`、`storageRef` 等生产说明 |
| 参考素材 | 声明了 `referenceAssetIds` 或 `references`，但没有可用的 `local://`、本地路径或 URL 参考文件 |

阻断输出示例：

```json
{
  "status": "blocked",
  "reason": "prompt_qa_failed",
  "preflightQa": {
    "passed": false,
    "score": 40,
    "timedSegmentCount": 0,
    "visualAnchorCount": 1,
    "actionVerbCount": 0,
    "usableReferenceCount": 0
  }
}
```

如果参考素材不可用，`reason` 为 `reference_qa_failed`。这类请求会保留在 `externalGenerationRequests` 中，等待上游重写 prompt、补充参考图或人工确认后再重试。

Dreamina 图片参数当前按以下规则归一：

| 输入 | MCP 参数 |
|---|---|
| `target.aspectRatio` | `ratio` |
| `target.resolution=1920x1080` / `1080p` | `resolution_type=2k` |
| `target.resolution=3840x2160` / `4k` | `resolution_type=4k` |
| `target.generateNum` | `generate_num` |

## 生成结果与素材来源

`LOCAL_MCP_TOOL_CALL` 的输出必须让用户看清楚哪些素材真实来自 MCP provider，哪些只是 fallback。标准输出字段：

| 字段 | 说明 |
|---|---|
| `generationResults` / `externalGenerationResults` | 每个请求的原始状态，包含 `requestId`、`shotId`、`kind`、`providerId`、`toolName`、`status`、`storageRef`、`localPath`、`error`。 |
| `assetProvenance` | 给前端和审查节点展示的素材来源清单。 |
| `sourceSummary` | 量化统计：视频/图片请求数、ready 数、blocked/failed/deferred/pending 数、是否需要 fallback。 |
| `requirementsSatisfied` | 是否满足本次 MCP 生成的最低 ready 视频素材要求。 |

`assetProvenance[]` 在 closed beta 中统一包含：

```json
{
  "schemaVersion": 1,
  "sourceType": "aigc_video",
  "providerName": "jimeng",
  "providerJobId": "provider-job-id",
  "fallbackReason": "",
  "isFallback": false,
  "generatedAt": "2026-07-04T00:00:00Z",
  "inputPromptHash": "sha256:...",
  "sourceArtifactIds": ["script-1", "reference-1"]
}
```

`sourceType=fallback_storyboard`、`fallback_preview` 或 `isFallback=true` 时，前端必须明确提示这是 fallback，不得标成真实 AIGC 视频素材。

自动插入的视频 MCP 步骤默认携带：

```json
{
  "maxReadyGenerations": 1,
  "minReadyVideoGenerations": 1
}
```

这表示系统会控制自动批量消耗额度，同时要求至少有 1 个真正 ready 的 AIGC 视频素材。如果 `jimeng.generate_video` 全部失败，例如 Dreamina 返回 `CreditPreDeductNotEnough`，输出会明确给出：

```json
{
  "sourceSummary": {
    "readyVideoCount": 0,
    "videoRequestCount": 4,
    "externalVideoRequirementSatisfied": false,
    "fallbackRequired": true,
    "needsAttention": true
  }
}
```

这种情况下，最终 HyperFrames/storyboard 成片只能算 fallback，不应对用户声明为“即梦 AIGC 视频素材已生成”。

成功下载的 MCP 文件会落在两处：

```text
~/Library/Application Support/TangyingAIOS/cache/mcp/<projectId>/<requestId>/
~/Library/Application Support/TangyingAIOS/artifacts/<projectId>/<requestId>/content
```

v0.1.5 验证中 `jimeng.generate_image` 成功生成 1 个参考图；`jimeng.generate_video` 因 Dreamina 账号余额不足返回 `CreditPreDeductNotEnough`，没有 ready 视频素材。v0.1.6 起该情况会被 `sourceSummary` 和 `assetProvenance` 明确标记。

## Video QA Python MCP

仓库内置了成片 QA stdio MCP server：

```bash
python3 -m pip install -r mcp/video_qa/requirements.txt
python3 mcp/video_qa/server.py
```

工具：

| 逻辑工具名 | Python MCP 工具名 | 说明 |
|---|---|---|
| `video_qa.analyze_video` | `analyze_video` | 抽帧、生成 contact sheet、输出 `SHOT_QA_REPORT` 和 `SHOT_REPAIR_PLAN`。 |

`VIDEO_FRAME_QA` 本地命令会默认启动这个 stdio MCP server。Go executor 只做路径校验和 MCP 调用，具体 QA 算法放在 `mcp/video_qa/` 内，后续 OCR、ASR、PyIQA、VLM judge 都应该继续以 MCP 工具方式扩展。

## 3D IP 口播层 MCP

仓库内置了一个 GLB 数字人视频层 stdio MCP server：

```bash
python3 -m pip install -r mcp/ip_avatar_3d/requirements.txt
python3 mcp/ip_avatar_3d/server.py
```

工具：

| 逻辑工具名 | Python MCP 工具名 | 说明 |
|---|---|---|
| `ip_avatar_3d.check_status` | `check_status` | 检查 Blender、FFmpeg、FFprobe 是否可用。 |
| `ip_avatar_3d.check_gpt_sovits_voice` | `check_gpt_sovits_voice` | 校验固定 GPT-SoVITS 音色的参考音频、模型和哈希。 |
| `ip_avatar_3d.synthesize_reference_voice` | `synthesize_reference_voice` | 使用固定 IP 音色或当前项目内已授权参考录音生成带 provenance 的正式旁白母带。 |
| `ip_avatar_3d.prepare_character_master` | `prepare_character_master` | 从带骨骼 FBX/GLB 生成并校验 A-roll 主资产；精修资产只写入 staging。 |
| `ip_avatar_3d.record_character_master_visual_inspection` | `record_character_master_visual_inspection` | 将审查人结论与 staged SHA、QA 图和对比图绑定。 |
| `ip_avatar_3d.create_character_master_publication_report` | `create_character_master_publication_report` | 通过 Blender 生成可验证的 v2 发布证据包。 |
| `ip_avatar_3d.publish_character_master` | `publish_character_master` | 校验绑定 staged SHA 的机器 QA 与人工视觉证据后，原子发布精修主资产。 |
| `ip_avatar_3d.validate_character_asset` | `validate_character_asset` | 校验骨架语义、蒙皮、口型和主资产配置。 |
| `ip_avatar_3d.list_aroll_actions` | `list_aroll_actions` | 列出带起止姿态状态的站立、坐姿和转场动作。 |
| `ip_avatar_3d.plan_motion` | `plan_motion` | 把口播文本转成口型和动作时间线。 |
| `ip_avatar_3d.render_talking_video` | `render_talking_video` | 用已批准主资产、固定场景和固定声音渲染 `ip_layer.mp4`。 |

注册示例：

```json
{
  "id": "ip_avatar_3d",
  "label": "IP Avatar 3D MCP",
  "transport": "stdio",
  "command": "python3",
  "args": ["/absolute/path/to/mcp/ip_avatar_3d/server.py"],
  "toolPrefix": "ip_avatar_3d.",
  "enabled": true
}
```

首次启动且本地尚未创建 `mcp-providers.json` 时，Local Agent 会自动发现并启用仓库内置的 `ip_avatar_3d` stdio provider；已经存在的用户配置不会被覆盖。主角色配置按“请求显式路径 -> `TANGYING_IP_AVATAR_PROFILE` -> 仓库内 `ip-assets/main-ip/character-profile.json`”的顺序解析，后续替换 IP 时无需修改编排代码。

系统调用时使用通用 `LOCAL_MCP_TOOL_CALL`，不要新增 `LOCAL_IP_*` 一类本地命令。该 provider 返回 `videoPath` / `localPath` / `motionPlanPath` / `renderReportPath`，主系统只把它作为 IP A-roll 视频层预览和合成。

`talking_head` 计划在该能力可用时自动插入 `ip_aroll_generation`。本地 runner 会把结果封装为独立的 `aRollAssetPackages`；HyperFrames 将其作为全程连续的角色画面和独立音轨，并按 `startSec` / `durationSec` 叠加 HyperKeyframes、即梦或其他 AIGC B-roll。显式传入 `ipArollEnabled=false` 可以关闭自动插入。

主 IP 的生产配置固定使用 `production_1080p`、1920x1080、30 fps。云端为每次 Agent run 固化 `voiceSelection` 与工具 catalog 快照；同一任务重试不会因 provider 重新发现而改变工具定义，新任务才会看到新增工具。音频始终先由本地 runner 预处理，再调用 `render_talking_video`：

- `default_ip`：通过 `synthesize_reference_voice` 使用固定 `gpt_sovits_local` 音色 `main_ip_warm_knowledge_host_v1`；
- `reference_clone`：只接受当前项目内可解析的本地参考录音、完全一致且已确认的逐字稿，以及明确的使用权确认；
- `recorded_narration`：不调用 TTS，直接把用户完整实录母带化。

预处理原子写入 48 kHz、单声道 PCM16 的 `narration_master.wav` 与 `narration_master.provenance.json`，目标为 `-16 LUFS`、真峰值不高于 `-1.5 dBTP`。渲染器只接受位于本次输出目录、哈希与 sidecar 一致且 `productionReady=true` 的正式母带。固定音色、授权、格式或 provenance 任一校验失败都会 fail closed；ChatTTS 和 macOS `say` 不属于生产 provider。

云端不会接收音频字节或本机绝对路径。跨边界的声音上下文只允许包含项目内 artifact ID、`local://` storage ref、SHA-256、MIME、逐字稿、授权状态和非敏感设置；本地 runner 在执行时才把这些引用解析成项目根目录内的真实文件，并拒绝路径穿越、远程 URL、跨项目引用和哈希不一致。

站姿和坐姿共用已批准的暖色演播室、角色主资产和声音配置，直接调用参数仅切换 `presentationMode`：

```json
{
  "script": "今天分享一个值得关注的观点。",
  "characterProfilePath": "/absolute/path/to/ip-assets/main-ip/character-profile.json",
  "presentationMode": "standing"
}
```

```json
{
  "script": "今天分享一个值得关注的观点。",
  "characterProfilePath": "/absolute/path/to/ip-assets/main-ip/character-profile.json",
  "presentationMode": "seated"
}
```

角色渲染优先加载 `model.masterBlendPath`，避免每条视频重复重拓扑。当前树懒主资产保留源手部表面，并为左右各三根手指添加三段独立骨骼，且验证每根手指两个关节过渡区都有实际权重顶点；五官使用原始面部几何与源网格口型。眼部当前只承诺独立眯眼，不宣称真眼皮拓扑或完整眨眼。正式场景使用暗色知识分享演播室，人物与背景在同一 Blender 场景内接受灯光并产生阴影。

## 新 MCP 的扩展方式

新增 provider 不需要改云端编排，也不需要改本地 runner。只要完成三件事：

1. 用标准 MCP SDK 暴露工具。
2. 在本地 Agent 注册 provider。
3. 让运行计划中的 `providerId` 和 `toolName` 对应到该 provider。

不要为每个 CLI 新增一个本地工具命令或 provider 专用 runner。云端只使用通用 `mcp_generation_runner`，本地 runner 只执行通用 `LOCAL_MCP_TOOL_CALL`。

例如一个 Python stdio server 暴露 `generate_music`，可以注册成：

```json
{
  "id": "music",
  "label": "Music MCP",
  "transport": "stdio",
  "command": "python3",
  "args": ["/absolute/path/music_mcp.py"],
  "toolPrefix": "music.",
  "enabled": true
}
```

系统侧调用 `providerId: "music"`、`toolName: "music.generate_music"`；实际 MCP server 收到的工具名是 `generate_music`。
