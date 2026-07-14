# MCP Services

This directory contains local MCP servers used by Tangying AIOS.

Rules:

- Each CLI or external local capability is wrapped as a standard MCP server.
- The main system talks to providers through MCP `tools/list` and `tools/call`.
- Do not add provider-specific local runner commands for new CLIs.
- Services may use Python, Node, Go, or any framework as long as the exposed protocol is standard MCP.

Current services:

- `jimeng/`: Python stdio MCP server wrapping the user-managed Dreamina CLI.
- `ip_avatar_3d/`: Python stdio MCP server for rendering GLB/GLTF/rigged-FBX cartoon IP talking-video layers with Blender and FFmpeg.
- `video_qa/`: Python stdio MCP server for rendered-video frame sampling, shot-level QA reports, and repair plans.
