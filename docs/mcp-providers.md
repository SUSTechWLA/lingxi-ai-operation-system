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

Dreamina 图片参数当前按以下规则归一：

| 输入 | MCP 参数 |
|---|---|
| `target.aspectRatio` | `ratio` |
| `target.resolution=1920x1080` / `1080p` | `resolution_type=2k` |
| `target.resolution=3840x2160` / `4k` | `resolution_type=4k` |
| `target.generateNum` | `generate_num` |

v0.1.5 已验证 `jimeng.generate_image` 可成功生成参考图；`jimeng.generate_video` 如果因 Dreamina 账号余额不足返回 `CreditPreDeductNotEnough`，系统会保留失败请求并继续 fallback 渲染和 QA。

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
