#!/usr/bin/env bash
# ============================================================================
# Local Agent API — Full Endpoint Test Suite
# ============================================================================
# Port 18080, no authentication required (localhost CORS guard on mutations).
# ============================================================================

# --- Health & Root ----------------------------------------------------------
test_local_health() {
  test_endpoint_json local GET "/api/local/health" 200 ".status" "health"
}

test_local_paths() {
  test_endpoint_json local GET "/api/local/paths" 200 ".dataDir" "paths"
}

test_local_openapi() {
  test_endpoint_json local GET "/api/local/openapi.json" 200 ".openapi" "openapi spec"
}

test_local_docs() {
  test_endpoint local GET "/api/local/docs" 200 "docs page"
}

# --- Logging ----------------------------------------------------------------
test_local_logs() {
  body_override='{"message":"api-smoke-test-log","level":"info","source":"test"}' \
    test_endpoint_json local POST "/api/local/logs" 200 ".status" "write log"
  body_override=""
}

# --- Model Providers --------------------------------------------------------
test_local_model_providers_get() {
  test_endpoint_json local GET "/api/local/model-providers" 200 ".providers" "get model providers"
}

test_local_model_providers_get_with_keys() {
  test_endpoint_json local GET "/api/local/model-providers?include_key=true" 200 ".providers" "get with keys"
}

# --- MCP Providers ----------------------------------------------------------
test_local_mcp_providers_list() {
  test_endpoint_json local GET "/api/local/mcp-providers" 200 ".providers" "list MCP providers"
}

test_local_mcp_providers_status() {
  test_endpoint_json local GET "/api/local/mcp-providers/status" 200 ".providers" "MCP provider status"
}

# --- JiMeng Setup -----------------------------------------------------------
test_local_jimeng_setup_status() {
  test_endpoint_json local GET "/api/local/jimeng/setup/status" 200 ".installCommand" "jimeng setup status"
}

test_local_jimeng_register_mcp() {
  body_override='{"transport":"stdio"}' \
    test_endpoint_json local POST "/api/local/jimeng/setup/register-mcp" 200 ".provider.id" "register jimeng MCP"
  body_override=""
}

# --- Artifacts (ordered: create → read → delete) ---------------------------
test_local_artifacts_a_create() {
  body_override='{"projectId":"api-test","id":"test-artifact","content":"hello api test","mimeType":"text/plain"}' \
    test_endpoint_json local POST "/api/local/artifacts" 200 ".id" "create artifact"
  body_override=""
}

test_local_artifacts_b_get() {
  test_endpoint_json local GET "/api/local/artifacts/test-artifact?projectId=api-test" 200 ".content" "get artifact"
}

test_local_artifacts_c_delete() {
  test_endpoint local DELETE "/api/local/artifacts/test-artifact?projectId=api-test" 200 "delete artifact"
}

# --- Projects ---------------------------------------------------------------
test_local_project_delete() {
  test_endpoint local DELETE "/api/local/projects/api-test" 200 "delete project"
}

# --- Diagnostics ------------------------------------------------------------
test_local_diagnostics_export() {
  body_override='{"reason":"api-test-suite"}' \
    test_endpoint_json local POST "/api/local/diagnostics" 200 ".path" "export diagnostics"
  body_override=""
}

# --- Error Handling ---------------------------------------------------------
test_local_method_not_allowed() {
  test_endpoint local POST "/api/local/health" 405 "POST on GET-only"
}

test_local_not_found() {
  test_endpoint local GET "/api/local/does-not-exist" 404 "404 handling"
}
