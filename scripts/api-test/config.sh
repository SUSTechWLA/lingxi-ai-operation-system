#!/usr/bin/env bash
# ============================================================================
# Tangying AIOS — API Test Framework Configuration
# ============================================================================
# This file defines URLs, credentials, and global settings for the API test
# suite. Override values via environment variables before running tests.
#
# Usage:
#   source scripts/api-test/config.sh
# ============================================================================

# --- Service URLs -----------------------------------------------------------
CLOUD_API_BASE="${CLOUD_API_BASE:-http://127.0.0.1:8080}"
LOCAL_API_BASE="${LOCAL_API_BASE:-http://127.0.0.1:18080}"
HYPERFRAMES_API_BASE="${HYPERFRAMES_API_BASE:-http://127.0.0.1:8787}"
FRONTEND_URL="${FRONTEND_URL:-http://127.0.0.1:3000}"

# --- Auth / Tokens ----------------------------------------------------------
# These should be set in CI or via environment. Never commit real tokens.
TANGYING_USER_TOKEN="${TANGYING_USER_TOKEN:-}"
TANGYING_DEVICE_ID="${TANGYING_DEVICE_ID:-}"
TEST_PROJECT_ID="${TEST_PROJECT_ID:-api-test-$(date +%Y%m%d-%H%M%S)}"

# --- Test Behaviour ---------------------------------------------------------
# Exit on first failure? (0=continue, 1=stop)
API_TEST_STRICT="${API_TEST_STRICT:-0}"

# Timeout for each curl call in seconds
API_TEST_TIMEOUT="${API_TEST_TIMEOUT:-10}"

# Output directory for reports
API_TEST_REPORT_DIR="${API_TEST_REPORT_DIR:-$ROOT_DIR/scripts/api-test/reports}"

# Verbose mode
API_TEST_VERBOSE="${API_TEST_VERBOSE:-0}"

# When 1, skip tests that require real AIGC providers
API_TEST_SKIP_AIGC="${API_TEST_SKIP_AIGC:-1}"

# When 1, produce machine-readable JSON output
API_TEST_JSON_OUTPUT="${API_TEST_JSON_OUTPUT:-0}"
