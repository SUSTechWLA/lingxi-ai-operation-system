#!/usr/bin/env bash
# ============================================================================
# MCP Provider Template — Copy this file to add a new MCP provider test.
# ============================================================================
# To add a new MCP provider:
#   1. Copy this file: cp template.sh my_provider.sh
#   2. Replace PROVIDER_NAME and TOOL_NAMES with actual values
#   3. Add provider-specific tests (tool calls, auth, etc.)
#   4. Add the file to scripts/api-smoke-test.sh's provider list
#
# Naming: test_mcp_<provider>_<testname>()
# ============================================================================

PROVIDER_NAME="__PROVIDER__"

test_mcp_${PROVIDER_NAME}_tools() {
  test_mcp_tool_list "$PROVIDER_NAME" "/api/local/mcp-providers/${PROVIDER_NAME}/tools"
}

test_mcp_${PROVIDER_NAME}_health() {
  # Customize: test that the provider responds correctly to a tool call
  test_endpoint local GET "/api/local/mcp-providers/${PROVIDER_NAME}/status" 200 "${PROVIDER_NAME} status"
}
