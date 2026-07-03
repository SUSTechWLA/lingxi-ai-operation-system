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
| `sourceSummary` | 量化统计：视频/图片请求数、ready 数、failed/deferred/pending 数、是否需要 fallback。 |
| `requirementsSatisfied` | 是否满足本次 MCP 生成的最低 ready 视频素材要求。 |

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
