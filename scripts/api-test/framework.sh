#!/usr/bin/env bash
# ============================================================================
# Tangying AIOS — API Test Framework
# ============================================================================
# Source order: config.sh → helpers.sh → endpoints/*.sh
#
# Framework capabilities:
#   - Service health checks (cloud, local, hyperframes)
#   - Per-endpoint HTTP status + JSON validation
#   - Auto-discovery of test_*() functions
#   - MCP provider tool discovery
#   - Structured console + JSON output
#   - CI-ready exit codes
#
# Extending with new endpoints:
#   1. Create scripts/api-test/endpoints/<service>.sh
#   2. Define test_<name>() functions using:
#      - test_endpoint <service> <METHOD> <path> <expected_status>
#      - test_endpoint_json <service> <METHOD> <path> <expected> <jq_filter>
#      - test_service_health <name> <url>
#   3. Run: bash scripts/api-smoke-test.sh
#
# Extending with new MCP providers:
#   1. Copy scripts/api-test/endpoints/mcp/template.sh
#   2. Fill in provider name and tool names
#   3. The test runner auto-discovers and executes test_*() functions
# ============================================================================
