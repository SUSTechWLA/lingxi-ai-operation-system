#!/usr/bin/env bash
# ============================================================================
# Tangying AIOS — API Smoke Test Runner
# ============================================================================
# One-click script to test all API endpoints across all services.
#
# Usage:
#   bash scripts/api-smoke-test.sh                        # Full test suite
#   bash scripts/api-smoke-test.sh --cloud                # Cloud backend only
#   bash scripts/api-smoke-test.sh --local                # Local agent only
#   bash scripts/api-smoke-test.sh --mcp                  # MCP providers only
#   bash scripts/api-smoke-test.sh --quick                # Health checks only
#   bash scripts/api-smoke-test.sh --json                 # Machine-readable output
#   bash scripts/api-smoke-test.sh --verbose              # Verbose debug output
#
# Environment variables (see config.sh):
#   CLOUD_API_BASE    — Cloud backend URL (default http://127.0.0.1:8080)
#   LOCAL_API_BASE    — Local agent URL  (default http://127.0.0.1:18080)
#   HYPERFRAMES_API_BASE — Render service URL (default http://127.0.0.1:8787)
#   TANGYING_USER_TOKEN  — Auth token for protected endpoints
#   API_TEST_SKIP_AIGC   — Skip AIGC provider tests (default 1)
#   API_TEST_STRICT      — Exit on first failure (default 0)
# ============================================================================

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
API_TEST_DIR="$ROOT_DIR/scripts/api-test"

# Source framework
source "$API_TEST_DIR/config.sh"
source "$API_TEST_DIR/helpers.sh"

# Parse arguments
MODE_FULL=1
MODE_CLOUD=0
MODE_LOCAL=0
MODE_MCP=0
MODE_HF=0
MODE_QUICK=0
JSON_OUTPUT=0

for arg in "$@"; do
  case "$arg" in
    --cloud) MODE_FULL=0; MODE_CLOUD=1 ;;
    --local) MODE_FULL=0; MODE_LOCAL=1 ;;
    --mcp)   MODE_FULL=0; MODE_MCP=1 ;;
    --hf)    MODE_FULL=0; MODE_HF=1 ;;
    --quick) MODE_FULL=0; MODE_QUICK=1 ;;
    --json)  JSON_OUTPUT=1 ;;
    --verbose) API_TEST_VERBOSE=1 ;;
    --help|-h)
      echo "Usage: bash scripts/api-smoke-test.sh [--cloud|--local|--mcp|--hf|--quick|--json|--verbose]"
      exit 0
      ;;
  esac
done

echo ""
echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║          Tangying AIOS — API Smoke Test Suite v1.0               ║"
echo "╠══════════════════════════════════════════════════════════════════╣"
echo "║  Cloud Backend:      $CLOUD_API_BASE"
echo "║  Local Agent:        $LOCAL_API_BASE"
echo "║  HyperFrames:        $HYPERFRAMES_API_BASE"
echo "║  Timestamp:          $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo ""

# --- Pre-flight: dependency check -------------------------------------------
header "Pre-flight Checks"
FAILURES_BEFORE=0

require_command() {
  local name="$1" hint="$2"
  if command -v "$name" >/dev/null 2>&1; then
    ok "dependency: $name ($(command -v "$name"))"
  else
    fail "dependency: $name not found. $hint"
    FAILURES_BEFORE=$((FAILURES_BEFORE + 1))
  fi
}

require_command curl "Install curl"
require_command python3 "Install Python 3"

if command -v jq >/dev/null 2>&1; then
  ok "dependency: jq (enhanced JSON validation)"
else
  warn "dependency: jq not installed — JSON validation will be basic"
fi

if [[ "$FAILURES_BEFORE" -gt 0 ]]; then
  echo ""
  echo "❌ Pre-flight check failed — $FAILURES_BEFORE missing dependencies."
  exit 2
fi

# --- Service Health Check ---------------------------------------------------
header "Service Health Check"
CLOUD_UP=0; LOCAL_UP=0; HF_UP=0

if [[ "$MODE_FULL" -eq 1 ]] || [[ "$MODE_CLOUD" -eq 1 ]] || [[ "$MODE_QUICK" -eq 1 ]]; then
  test_service_health "cloud" "$CLOUD_API_BASE/api/health" && CLOUD_UP=1
  test_service_health "cloud-swagger" "$CLOUD_API_BASE/docs" && :
fi

if [[ "$MODE_FULL" -eq 1 ]] || [[ "$MODE_LOCAL" -eq 1 ]] || [[ "$MODE_QUICK" -eq 1 ]]; then
  test_service_health "local-agent" "$LOCAL_API_BASE/api/local/health" && LOCAL_UP=1
fi

if [[ "$MODE_FULL" -eq 1 ]] || [[ "$MODE_HF" -eq 1 ]] || [[ "$MODE_QUICK" -eq 1 ]]; then
  test_service_health "hyperframes" "$HYPERFRAMES_API_BASE/health" && HF_UP=1
fi

if [[ "$MODE_QUICK" -eq 1 ]]; then
  generate_summary
  exit $?
fi

# --- Cloud Backend API Tests ------------------------------------------------
if [[ "$CLOUD_UP" -eq 1 ]]; then
  run_test_suite "Cloud Backend API" "$API_TEST_DIR/endpoints/cloud-backend.sh"
else
  warn "Cloud backend is not running — skipping cloud API tests"
  skip "Cloud backend endpoints (service unreachable)"
fi

# --- Local Agent API Tests --------------------------------------------------
if [[ "$LOCAL_UP" -eq 1 ]]; then
  run_test_suite "Local Agent API" "$API_TEST_DIR/endpoints/local-backend.sh"
else
  warn "Local agent is not running — skipping local API tests"
  skip "Local agent endpoints (service unreachable)"
fi

# --- HyperFrames API Tests --------------------------------------------------
if [[ "$HF_UP" -eq 1 ]]; then
  run_test_suite "HyperFrames Render Service" "$API_TEST_DIR/endpoints/hyperframes.sh"
else
  warn "HyperFrames render service is not running — skipping HF tests"
  skip "HyperFrames endpoints (service unreachable)"
fi

# --- MCP Provider Tests -----------------------------------------------------
if [[ "$MODE_FULL" -eq 1 ]] || [[ "$MODE_MCP" -eq 1 ]]; then
  header "MCP Provider Tests"
  # JiMeng
  run_test_suite "MCP: JiMeng / Dreamina" "$API_TEST_DIR/endpoints/mcp/jimeng.sh"
  # Video QA
  run_test_suite "MCP: Video QA" "$API_TEST_DIR/endpoints/mcp/video_qa.sh"
  # IP Avatar 3D
  run_test_suite "MCP: IP Avatar 3D" "$API_TEST_DIR/endpoints/mcp/ip_avatar_3d.sh"
fi

# --- Summary ----------------------------------------------------------------
if [[ "$JSON_OUTPUT" -eq 1 ]]; then
  generate_json_report
fi

generate_summary
