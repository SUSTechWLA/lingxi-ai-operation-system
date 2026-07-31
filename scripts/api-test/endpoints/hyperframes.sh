#!/usr/bin/env bash
# ============================================================================
# HyperFrames Render Service API — Full Endpoint Test Suite
# ============================================================================
# Port 8787 (Fastify), no authentication
# ============================================================================

# --- Health -----------------------------------------------------------------
test_hf_health() {
  test_endpoint_json hf GET "/health" 200 ".ok" "health"
}

# --- Infra ------------------------------------------------------------------
test_hf_jobs_list() {
  test_endpoint_json hf GET "/jobs" 200 ".jobs" "list jobs"
}

# --- Lint (requires body, returns 400 without it — correct validation) ------
test_hf_lint_validation() {
  body_override='{"projectDir":"/nonexistent"}' test_endpoint hf POST "/lint" 200 "lint with body"
  body_override=""
}

# --- Error Handling ---------------------------------------------------------
test_hf_404() {
  test_endpoint hf GET "/nope" 404 "404 handling"
}
