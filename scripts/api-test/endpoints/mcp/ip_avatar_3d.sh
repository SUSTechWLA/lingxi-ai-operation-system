#!/usr/bin/env bash
# ============================================================================
# IP 3D Avatar MCP Provider — Test Definitions
# ============================================================================
# Tests the IP Avatar 3D MCP provider (GLB/GLTF cartoon IP talking-video).
# ============================================================================

test_mcp_ip_avatar_tools() {
  test_mcp_tool_list "ip_avatar_3d" "/api/local/mcp-providers/ip_avatar_3d/tools"
}
