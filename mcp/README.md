# MCP Services

This directory contains local MCP servers used by Tangying AIOS.

Rules:

- Each CLI or external local capability is wrapped as a standard MCP server.
- Servers complete the standard `initialize` -> `notifications/initialized` ->
  paginated `tools/list` -> `tools/call` lifecycle.
- The main system discovers enabled providers dynamically and routes every call
  through `LOCAL_MCP_TOOL_CALL`.
- Do not add provider-specific JSON-RPC clients, planner/compiler branches, or
  local runner commands for new providers.
- Stdio servers reserve stdout for MCP protocol messages and write diagnostics
  only to stderr. Streamable HTTP providers use the standard SDK transport.
- Tool names and descriptions are non-empty. Every `inputSchema` is valid JSON
  Schema with `type: object`, `properties`, and a consistent `required` list.
- Server and client production entrypoints require the official MCP SDK. Test
  helper imports may fail clearly when it is absent, but must never impersonate
  a running MCP server.
- Provider secrets are write-only and must not appear in prompts, logs, status
  responses, diagnostics, or public configuration reads.
- Standard content, structured content, pagination cursors, and unknown raw
  extension fields are preserved across the local bridge.

Current services:

- `jimeng/`: Python stdio MCP server wrapping the user-managed Dreamina CLI.
- `ip_avatar_3d/`: Python stdio MCP server for rendering GLB/GLTF/rigged-FBX cartoon IP talking-video layers with Blender and FFmpeg.
- `video_qa/`: Python stdio MCP server for rendered-video frame sampling, shot-level QA reports, and repair plans.

All Python services use the supported official SDK range `mcp>=1.27,<2`.
Run the real stdio registration smoke test without invoking external tools or
services:

```bash
python3 -m pip install 'mcp>=1.27,<2'
python3 scripts/test_mcp_contracts.py
```

The full five-layer Agent/Tool/MCP contract is documented in
[`docs/agent-tool-mcp-contract.md`](../docs/agent-tool-mcp-contract.md).
