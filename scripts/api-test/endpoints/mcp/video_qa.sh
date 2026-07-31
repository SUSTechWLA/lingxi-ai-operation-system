#!/usr/bin/env bash
# ============================================================================
# Video QA MCP Provider — Test Definitions
# ============================================================================
# Tests the Video QA MCP provider (shot-level frame analysis, repair plans).
# ============================================================================

test_mcp_video_qa_list() {
  test_mcp_tool_list "video_qa" "/api/local/mcp-providers/video_qa/tools"
}

test_mcp_video_qa_unittest() {
  info "Running video_qa Python unit tests"
  if python3 -m unittest discover -s "$ROOT_DIR/mcp/video_qa" -p 'test*.py' -q 2>&1; then
    ok "MCP video_qa unit tests pass"
  else
    fail "MCP video_qa unit tests failed"
  fi
}
