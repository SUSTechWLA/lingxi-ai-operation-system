# Local Native Tool vs MCP Provider

本地能力分两类：Local Native Tool 和 MCP Provider Tool。不要因为某个 provider 是本地 CLI，就给它新增 provider-specific local runner command。

## Local Native Tool

Local Native Tool 是系统自有、确定性、强业务耦合的本地执行能力。

适合：

- path guard
- 本地文件导入
- artifact package
- ffmpeg probe
- HyperFrames project / snapshot / render
- final review 聚合
- 本地预检

约束：

- 只能注册在 `localtool.Registry` 白名单。
- 不允许任意 shell command。
- 不允许把用户输入拼进未白名单命令。
- 文件读写必须受 pathguard 或 artifact policy 约束。
- 输入、输出、错误要有摘要和 trace。

## MCP Provider Tool

MCP Provider Tool 是可替换的外部能力。

适合：

- Jimeng / Dreamina CLI
- Video QA OCR / ASR / PyIQA / VLM Judge
- 图像生成、视频生成、TTS、ASR
- 浏览器、Figma、GitHub、外部数据源
- 任何未来可能替换供应商或跨 Agent 复用的能力

约束：

- 必须实现 MCP `initialize`、`tools/list`、`tools/call`。
- 必须通过 `toolPrefix` / `toolNameMap` 归一工具名。
- 必须通过 `LOCAL_MCP_TOOL_CALL` 执行。
- Provider-specific 细节只能在 MCP provider 自身或 provider config 里。
- fallback 结果必须显式标记，不能伪装成真实 AIGC。

## 新增 MCP Provider 步骤

1. 实现 MCP server，提供 `tools/list` 和 `tools/call`。
2. 在本地 Agent 配置 provider：

```json
{
  "id": "my_provider",
  "label": "My Provider",
  "transport": "stdio",
  "command": "python3",
  "args": ["/absolute/path/to/server.py"],
  "toolPrefix": "my_provider.",
  "enabled": true,
  "enabledTools": ["my_provider.generate_video"],
  "disabledTools": [],
  "timeout": 600,
  "approvalMode": "before_execute"
}
```

3. 本地 Agent 调 `tools/list`。
4. 系统把 MCP tools 转成 `ToolManifest`，设置 `boundary=mcp_provider`、`executionPlane=local`、`localCommand=LOCAL_MCP_TOOL_CALL` 和 `providerBinding`。
5. Planner 只选择逻辑工具名。
6. PlanCompiler 编译为本地 MCP 调用 payload。
7. local runner 通过 `localmcp.Client` 调 provider。
8. 结果写入 `externalGenerationResults`、`assetProvenance`、`sourceSummary`。

## Jimeng

Jimeng 保持 MCP Provider：

- `jimeng.generate_image`
- `jimeng.generate_video`
- `jimeng.query_result`
- `jimeng.check_status`
- `jimeng.check_login`
- `jimeng.list_task`

不要新增 Jimeng 专属 local runner command。Prompt 必须先经过视频 prompt QA / preflight，禁止把内部术语、JSON、seed、shotId 直接塞给 provider prompt。

## Video QA

Video QA 能力继续放在 MCP provider 或 MCP-style local tool 中。OCR、ASR、PyIQA、VLM Judge 不回灌成云端 Native Tool。

Go executor 只负责：

- path 校验
- MCP 调用
- 结果归一化

QA Gate 是否通过仍由 Agent Core / PlanGuard / Workflow 决定。
