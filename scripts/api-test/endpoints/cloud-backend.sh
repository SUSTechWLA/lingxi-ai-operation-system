#!/usr/bin/env bash
# ============================================================================
# Cloud Backend API — Endpoint Tests (authenticated)
# ============================================================================
# Cloud backend uses envelope: { code, message, data }
# Auth: Bearer token in TANGYING_USER_TOKEN
# Register returns 200 (not 201), same as login.
# ============================================================================

# --- Public Endpoints --------------------------------------------------------
test_cloud_health() {
  test_endpoint_json cloud GET "/api/health" 200 ".code" "root health"
}

test_cloud_health_ready() {
  test_endpoint_json cloud GET "/api/health/ready" 200 ".status" "readiness probe"
}

test_cloud_openapi_json() {
  test_endpoint cloud GET "/openapi.json" 200 "openapi json spec"
}

test_cloud_openapi_yaml() {
  test_endpoint cloud GET "/openapi.yaml" 200 "openapi yaml spec"
}

test_cloud_docs() {
  test_endpoint cloud GET "/docs" 200 "swagger docs ui"
}

# --- Auth (register first, then login, then use token) ----------------------
test_cloud_auth_01_register() {
  body_override='{"email":"apitest-'"${TEST_PROJECT_ID}"'@t.com","password":"Test1234!","nickname":"api-test"}' \
    test_endpoint_json cloud POST "/api/auth/register" 200 ".data.access_token" "register"
  body_override=""
}

test_cloud_auth_02_login() {
  body_override='{"email":"apitest-'"${TEST_PROJECT_ID}"'@t.com","password":"Test1234!"}' \
    test_endpoint_json cloud POST "/api/auth/login" 200 ".data.access_token" "login"
  body_override=""
  if command -v jq &>/dev/null && [[ -n "${RESP_BODY:-}" ]]; then
    local token
    token=$(echo "$RESP_BODY" | jq -r '.data.access_token // empty' 2>/dev/null)
    if [[ -n "$token" ]] && [[ "$token" != "null" ]]; then
      TANGYING_USER_TOKEN="$token"
      export TANGYING_USER_TOKEN
    fi
  fi
}

test_cloud_auth_03_login_invalid() {
  body_override='{"email":"nobody@nope.com","password":"nope"}' \
    test_endpoint cloud POST "/api/auth/login" 401 "invalid credentials"
  body_override=""
}

test_cloud_auth_04_refresh() {
  body_override='{"refresh_token":"invalid"}' \
    test_endpoint cloud POST "/api/auth/refresh" 401 "token refresh"
  body_override=""
}

# --- Protected Endpoints (require auth token from login above) ---------------
test_cloud_auth_05_me() {
  test_endpoint_json cloud GET "/api/auth/me" 200 ".data.id" "current user"
}

test_cloud_tools_list() {
  test_endpoint cloud GET "/api/tools" 200 "list tools"
}

test_cloud_skills_list() {
  test_endpoint cloud GET "/api/skills" 200 "list skills"
}

test_cloud_skills_catalog() {
  test_endpoint cloud GET "/api/skills/catalog" 200 "skills catalog"
}

test_cloud_skills_route() {
  body_override='{"brief":"科技口播视频"}' \
    test_endpoint cloud POST "/api/skills/route" 200 "route skill"
  body_override=""
}

test_cloud_workflows_list() {
  test_endpoint cloud GET "/api/workflows" 200 "list workflows"
}

test_cloud_skill_capabilities() {
  test_endpoint cloud GET "/api/skill-capabilities" 200 "skill capabilities"
}

test_cloud_video_role_agents() {
  test_endpoint cloud GET "/api/video/role-agents" 200 "role agents"
}

test_cloud_video_preflight() {
  test_endpoint cloud GET "/api/video/preflight?pipeline=knowledge-video" 200 "video preflight"
}

test_cloud_video_projects_create() {
  body_override='{"name":"API Smoke '"${TEST_PROJECT_ID}"'","description":"Test project","mode":"voice_visual"}' \
    test_endpoint_json cloud POST "/api/video-projects" 200 ".data.project.id" "create project"
  body_override=""
}

test_cloud_video_projects_list() {
  test_endpoint cloud GET "/api/video-projects" 200 "list projects"
}

test_cloud_trace_recent() {
  test_endpoint cloud GET "/api/trace/recent" 200 "recent trace"
}

# --- Error paths ------------------------------------------------------------
test_cloud_404_path() {
  test_endpoint cloud GET "/api/definitely-not-a-real-endpoint" 404 "404 handling"
}

test_cloud_auth_06_required() {
  local saved_token="$TANGYING_USER_TOKEN"
  TANGYING_USER_TOKEN=""
  test_endpoint cloud GET "/api/video-projects" 401 "auth required"
  TANGYING_USER_TOKEN="$saved_token"
}
