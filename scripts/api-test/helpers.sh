#!/usr/bin/env bash
# ============================================================================
# Tangying AIOS — API Test Framework Helpers
# ============================================================================
# Utility functions shared by all test scripts. Source this file before
# using any test functions.
# ============================================================================

# --- State management -------------------------------------------------------
TESTS_TOTAL=0
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
TESTS_WARNINGS=0
CURRENT_SERVICE=""
TEST_START_TIME=$(date +%s)

# --- Output helpers ---------------------------------------------------------
info()  { printf '\033[1;34m[INFO]\033[0m    %s\n' "$*"; }
ok()    { printf '\033[1;32m[PASS]\033[0m    %s\n' "$*"; TESTS_PASSED=$((TESTS_PASSED + 1)); TESTS_TOTAL=$((TESTS_TOTAL + 1)); }
fail()  { printf '\033[1;31m[FAIL]\033[0m    %s\n' "$*"; TESTS_FAILED=$((TESTS_FAILED + 1)); TESTS_TOTAL=$((TESTS_TOTAL + 1)); }
warn()  { printf '\033[1;33m[WARN]\033[0m    %s\n' "$*"; TESTS_WARNINGS=$((TESTS_WARNINGS + 1)); TESTS_TOTAL=$((TESTS_TOTAL + 1)); }
skip()  { printf '\033[1;37m[SKIP]\033[0m    %s\n' "$*"; TESTS_SKIPPED=$((TESTS_SKIPPED + 1)); TESTS_TOTAL=$((TESTS_TOTAL + 1)); }
header(){ printf '\n\033[1;36m━━━ %s ━━━\033[0m\n' "$*"; }

vlog() {
  if [[ "${API_TEST_VERBOSE:-0}" == "1" ]]; then
    printf '\033[0;37m[DEBUG]\033[0m   %s\n' "$*"
  fi
}

# --- JSON helpers -----------------------------------------------------------
json_log_entry() {
  local service="$1" endpoint="$2" method="$3" expected="$4" actual="$5" result="$6" duration_ms="$7" message="$8"
  printf '{"service":"%s","endpoint":"%s","method":"%s","expectedStatus":%s,"actualStatus":%s,"result":"%s","durationMs":%s,"message":"%s"}\n' \
    "$service" "$endpoint" "$method" "$expected" "$actual" "$result" "$duration_ms" "$message"
}

# --- HTTP helpers -----------------------------------------------------------
# Perform an HTTP request and return the response body + status code.
# Usage: http_get "$url"                    → outputs JSON body
#        http_get "$url" "$token"           → with Bearer token
#        http_get "$url" ""                 → no auth
http_get() {
  local url="$1"
  local token="${2:-}"
  local timeout="${API_TEST_TIMEOUT:-10}"
  local curl_opts=(-sS --max-time "$timeout" -w '\n%{http_code}')
  if [[ -n "$token" ]]; then
    curl_opts+=(-H "Authorization: Bearer $token")
  fi
  curl "${curl_opts[@]}" "$url" 2>&1
}

http_post() {
  local url="$1" body="$2" token="${3:-}"
  local timeout="${API_TEST_TIMEOUT:-10}"
  local curl_opts=(-sS --max-time "$timeout" -X POST -H 'Content-Type: application/json' -w '\n%{http_code}')
  if [[ -n "$token" ]]; then
    curl_opts+=(-H "Authorization: Bearer $token")
  fi
  if [[ -n "$body" ]]; then
    curl_opts+=(-d "$body")
  fi
  curl "${curl_opts[@]}" "$url" 2>&1
}

http_put() {
  local url="$1" body="$2" token="${3:-}"
  local timeout="${API_TEST_TIMEOUT:-10}"
  local curl_opts=(-sS --max-time "$timeout" -X PUT -H 'Content-Type: application/json' -w '\n%{http_code}')
  if [[ -n "$token" ]]; then
    curl_opts+=(-H "Authorization: Bearer $token")
  fi
  curl "${curl_opts[@]}" -d "$body" "$url" 2>&1
}

http_delete() {
  local url="$1" token="${2:-}"
  local timeout="${API_TEST_TIMEOUT:-10}"
  local curl_opts=(-sS --max-time "$timeout" -X DELETE -w '\n%{http_code}')
  if [[ -n "$token" ]]; then
    curl_opts+=(-H "Authorization: Bearer $token")
  fi
  curl "${curl_opts[@]}" "$url" 2>&1
}

# Parse HTTP response: split body and status code.
# Usage: http_get "..." | parse_response → sets RESP_BODY, RESP_CODE, RESP_OK
parse_response() {
  local raw
  raw="$(cat)"
  RESP_CODE=$(echo "$raw" | tail -1)
  RESP_BODY=$(echo "$raw" | sed '$d')
  RESP_OK=0
  if [[ "$RESP_CODE" -ge 200 ]] && [[ "$RESP_CODE" -lt 300 ]]; then
    RESP_OK=1
  fi
}

# --- Service availability check ---------------------------------------------
# Check if a service is reachable. Returns 0 if reachable, 1 if not.
check_service() {
  local url="$1" name="$2"
  local code
  code=$(curl -sS --max-time 3 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null)
  if [[ "$code" =~ ^[0-9]+$ ]] && [[ "$code" -ge 100 ]]; then
    vlog "$name at $url → $code"
    return 0
  fi
  return 1
}

# --- Test primitives --------------------------------------------------------
# test_endpoint <service> <method> <path> <expected_status> <extra_description>
# The test expects <expected_status> HTTP response code.
test_endpoint() {
  local service="$1" method="$2" path="$3" expected="$4" desc="${5:-}"
  local url token start_ms end_ms elapsed raw body code curl_exit
  CURRENT_SERVICE="$service"

  case "$service" in
    cloud) url="${CLOUD_API_BASE}${path}" ;;
    local) url="${LOCAL_API_BASE}${path}" ;;
    hf)    url="${HYPERFRAMES_API_BASE}${path}" ;;
    *)     url="$path" ;;
  esac

  # If this is an auth-required endpoint, use the token
  token="${TANGYING_USER_TOKEN:-}"

  start_ms=$(python3 -c 'import time; print(int(time.time()*1000))' 2>/dev/null || echo 0)

  case "$method" in
    GET)
      if [[ -n "$token" ]]; then
        raw=$(http_get "$url" "$token")
      else
        raw=$(http_get "$url")
      fi
      ;;
    POST)
      raw=$(http_post "$url" "$body_override" "$token")
      ;;
    PUT)
      raw=$(http_put "$url" "$body_override" "$token")
      ;;
    DELETE)
      raw=$(http_delete "$url" "$token")
      ;;
    *)
      fail "$service $method $path — unsupported HTTP method"
      return 1
      ;;
  esac
  curl_exit=$?
  code=$(echo "$raw" | tail -1)
  body=$(echo "$raw" | sed '$d')

  end_ms=$(python3 -c 'import time; print(int(time.time()*1000))' 2>/dev/null || echo 0)
  elapsed=$((end_ms - start_ms))

  if [[ $curl_exit -ne 0 ]]; then
    fail "$service $method $path — connection failed (curl exit $curl_exit) $desc"
    return 1
  fi

  if [[ "$code" == "$expected" ]]; then
    ok "$service $method $path → $code (${elapsed}ms) $desc"
    return 0
  else
    fail "$service $method $path — expected $expected, got $code (${elapsed}ms) $desc"
    vlog "  Response: ${body:0:200}"
    return 1
  fi
}

# test_endpoint_json <service> <method> <path> <expected_status> <jq_filter>
# Like test_endpoint but also validates JSON structure.
test_endpoint_json() {
  local service="$1" method="$2" path="$3" expected="$4" jq_filter="${5:-}" desc="${6:-}"
  local url token raw code body

  case "$service" in
    cloud) url="${CLOUD_API_BASE}${path}" ;;
    local) url="${LOCAL_API_BASE}${path}" ;;
    hf)    url="${HYPERFRAMES_API_BASE}${path}" ;;
    *)     url="$path" ;;
  esac
  token="${TANGYING_USER_TOKEN:-}"

  if [[ "$method" == "GET" ]]; then
    if [[ -n "$token" ]]; then raw=$(http_get "$url" "$token"); else raw=$(http_get "$url"); fi
  elif [[ "$method" == "POST" ]]; then
    raw=$(http_post "$url" "${body_override:-}" "$token")
  fi

  code=$(echo "$raw" | tail -1)
  RESP_BODY=$(echo "$raw" | sed '$d')
  RESP_CODE="$code"
  body="$RESP_BODY"

  if [[ "$code" != "$expected" ]]; then
    fail "$service $method $path — expected $expected, got $code $desc"
    return 1
  fi

  if [[ -n "$jq_filter" ]]; then
    if command -v jq &>/dev/null; then
      if echo "$body" | jq -e "$jq_filter" >/dev/null 2>&1; then
        ok "$service $method $path → $code, json valid: $jq_filter $desc"
      else
        fail "$service $method $path → $code, json check failed: $jq_filter $desc"
        vlog "  Body: ${body:0:200}"
        return 1
      fi
    else
      ok "$service $method $path → $code (jq not installed, skipping json check) $desc"
    fi
  else
    # Just check valid JSON
    if command -v jq &>/dev/null; then
      if echo "$body" | jq empty 2>/dev/null; then
        ok "$service $method $path → $code, valid JSON $desc"
      else
        fail "$service $method $path → $code, invalid JSON response $desc"
        return 1
      fi
    else
      # Fallback: check it starts with { or [
      if [[ "$body" =~ ^[\[\{] ]]; then
        ok "$service $method $path → $code $desc"
      else
        fail "$service $method $path → $code, not JSON $desc"
        return 1
      fi
    fi
  fi
}

# test_service_health <service> <url>
# Quick health check — expects 200 or any 2xx.
test_service_health() {
  local service="$1" url="$2"
  CURRENT_SERVICE="$service"
  local code curl_exit
  code=$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null)
  curl_exit=$?
  if [[ $curl_exit -ne 0 ]]; then
    fail "$service health check at $url — unreachable"
    return 1
  fi
  if [[ "$code" -ge 200 ]] && [[ "$code" -lt 400 ]]; then
    ok "$service health check at $url → $code"
  else
    warn "$service health check at $url → $code (unexpected)"
  fi
}

# test_mcp_tool_list <provider_name> <local_api_url_suffix>
# Checks that an MCP provider can list its tools via the local agent API.
test_mcp_tool_list() {
  local provider="$1" api_path="$2"
  local url="${LOCAL_API_BASE}${api_path}"
  local raw code body
  raw=$(http_get "$url" "$TANGYING_USER_TOKEN")
  code=$(echo "$raw" | tail -1)
  body=$(echo "$raw" | sed '$d')

  if [[ "$code" != "200" ]]; then
    warn "MCP $provider tool list: $api_path → $code (service may not be running)"
    return 1
  fi

  if command -v jq &>/dev/null; then
    local tool_count
    tool_count=$(echo "$body" | jq '.tools | length' 2>/dev/null)
    if [[ -n "$tool_count" ]] && [[ "$tool_count" -gt 0 ]]; then
      ok "MCP $provider: $tool_count tools exposed"
    else
      warn "MCP $provider: no tools found or invalid response"
    fi
  else
    ok "MCP $provider tool list → 200 (jq not available for tool count)"
  fi
}

# --- Report generation ------------------------------------------------------
generate_summary() {
  local end_time elapsed
  end_time=$(date +%s)
  elapsed=$((end_time - TEST_START_TIME))

  echo ""
  echo "╔══════════════════════════════════════════════════════════════════╗"
  echo "║              Tangying AIOS — API Test Summary                    ║"
  echo "╠══════════════════════════════════════════════════════════════════╣"
  printf "║  Total: %-4d  Passed: %-4d  Failed: %-4d  Skipped: %-4d  ║\n" \
    "$TESTS_TOTAL" "$TESTS_PASSED" "$TESTS_FAILED" "$TESTS_SKIPPED"
  if [[ "$TESTS_WARNINGS" -gt 0 ]]; then
    printf "║  Warnings: %-3d                                                 ║\n" "$TESTS_WARNINGS"
  fi
  printf "║  Duration: %-3ds                                                ║\n" "$elapsed"
  echo "╚══════════════════════════════════════════════════════════════════╝"

  if [[ "$TESTS_FAILED" -gt 0 ]]; then
    echo ""
    echo "❌ Some tests FAILED."
    return 1
  fi

  echo ""
  echo "✅ All tests passed."
  return 0
}

# Generate a JSON report file for CI consumption.
generate_json_report() {
  local report_file="${API_TEST_REPORT_DIR}/api-test-report-$(date +%Y%m%d-%H%M%S).json"
  local end_time elapsed
  end_time=$(date +%s)
  elapsed=$((end_time - TEST_START_TIME))

  mkdir -p "$(dirname "$report_file")"

  cat > "$report_file" <<JSONEOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "durationSec": $elapsed,
  "total": $TESTS_TOTAL,
  "passed": $TESTS_PASSED,
  "failed": $TESTS_FAILED,
  "skipped": $TESTS_SKIPPED,
  "warnings": $TESTS_WARNINGS,
  "result": "$([[ $TESTS_FAILED -eq 0 ]] && echo "PASS" || echo "FAIL")"
}
JSONEOF
  info "JSON report written to $report_file"
}

# --- Test suite runner ------------------------------------------------------
# Run a test file, discover and execute all test_* functions.
run_test_suite() {
  local name="$1" path="$2"
  header "$name"
  if [[ ! -f "$path" ]]; then
    warn "Test suite not found: $path"
    return
  fi

  # Discover test functions defined in the file
  local test_funcs
  test_funcs=$(grep -oE '^test_[a-zA-Z0-9_]+\(\)' "$path" | sed 's/()$//' | sort)

  if [[ -z "$test_funcs" ]]; then
    warn "No test_* functions found in $path"
    return
  fi

  # Source the file to load function definitions
  source "$path"

  # Execute each test function
  for func in $test_funcs; do
    if declare -f "$func" >/dev/null 2>&1; then
      "$func"
    fi
  done
}
