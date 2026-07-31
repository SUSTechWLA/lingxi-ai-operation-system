#!/usr/bin/env bash
# ============================================================================
# JiMeng / Dreamina MCP Provider — Test Definitions
# ============================================================================
# Tests the JiMeng MCP provider integration via the local agent API.
# The JiMeng MCP server wraps the user's Dreamina CLI.
# ============================================================================

test_mcp_jimeng_tools() {
  test_mcp_tool_list "jimeng" "/api/local/mcp-providers/jimeng/tools"
}

test_mcp_jimeng_register() {
  body_override='{"transport":"stdio"}' \
    test_endpoint_json local POST "/api/local/jimeng/setup/register-mcp" 200 ".provider" "register jimeng provider"
  body_override=""
}

test_mcp_jimeng_setup_check() {
  test_endpoint local GET "/api/local/jimeng/setup/status" 200 "jimeng setup status"
}
