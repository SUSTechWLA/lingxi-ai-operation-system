#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
FAILURES=0
WARNINGS=0

info() { printf '\033[1;34m[INFO]\033[0m %s\n' "$*"; }
ok() { printf '\033[1;32m[OK]\033[0m %s\n' "$*"; }
warn() { WARNINGS=$((WARNINGS + 1)); printf '\033[1;33m[WARN]\033[0m %s\n' "$*"; }
fail() { FAILURES=$((FAILURES + 1)); printf '\033[1;31m[FAIL]\033[0m %s\n' "$*"; }

run_check() {
  local label="$1"
  shift
  info "$label"
  if "$@"; then
    ok "$label"
  else
    fail "$label"
  fi
}

require_command() {
  local name="$1"
  local hint="$2"
  if command -v "$name" >/dev/null 2>&1; then
    ok "$name found: $(command -v "$name")"
  else
    fail "$name is missing. $hint"
  fi
}

check_port() {
  local port="$1"
  local label="$2"
  if command -v lsof >/dev/null 2>&1; then
    if lsof -iTCP:"$port" -sTCP:LISTEN -n -P >/dev/null 2>&1; then
      if [[ "${BETA_SMOKE_STRICT_PORTS:-}" == "1" ]]; then
        fail "$label port $port is already in use"
      else
        warn "$label port $port is already in use; this is fine if the service is intentionally running"
      fi
    else
      ok "$label port $port is free"
    fi
  else
    warn "lsof is missing; skipped $label port $port check"
  fi
}

check_node_module_dir() {
  local dir="$1"
  local name="$2"
  if [[ ! -d "$dir/node_modules" ]]; then
    fail "$name dependencies are missing. Run: cd $dir && npm ci"
    return 1
  fi
  return 0
}

check_release_env() {
  if [[ "${GIN_MODE:-}" != "release" && "${APP_ENV:-}" != "production" ]]; then
    warn "Production env validation skipped because GIN_MODE=release or APP_ENV=production is not set"
    return 0
  fi
  local required=(
    AUTH_TOKEN_SECRET
    OBSERVABILITY_SEALING_KEY
    OBSERVABILITY_SEALING_DOMAIN
    OBSERVABILITY_SOURCE_ENVIRONMENT
    POSTGRES_PASSWORD
    MINIO_SECRET_KEY
    CORS_ALLOWED_ORIGINS
    SANDBOX_ENABLED
    SANDBOX_ADDRESS
  )
  for key in "${required[@]}"; do
    if [[ -z "${!key:-}" ]]; then
      fail "$key is required in release/production mode"
    fi
  done
  if [[ "${CORS_ALLOWED_ORIGINS:-}" == "*" ]]; then
    fail "CORS_ALLOWED_ORIGINS must not be '*' in release/production mode"
  fi
  if [[ "${SANDBOX_ENABLED:-}" != "true" ]]; then
    fail "SANDBOX_ENABLED must be true in release/production mode"
  fi
  if [[ "${SANDBOX_FALLBACK:-false}" == "true" ]]; then
    fail "SANDBOX_FALLBACK must be false in release/production mode"
  fi
}

info "Tangying AIOS closed beta smoke check"
info "Repo: $ROOT_DIR"

require_command go "Install Go 1.24+; cloud backend currently targets Go 1.25."
require_command node "Install Node.js 24 for frontend and HyperFrames render service."
require_command npm "Install npm with Node.js."
require_command python3 "Install Python 3.11+."
require_command ffmpeg "Install FFmpeg and ensure it is on PATH."

check_port 8080 "cloud-backend"
check_port 18080 "local-backend/local-agent"
check_port 3000 "frontend dev server"
check_port 8787 "HyperFrames render service"

check_release_env

run_check "cloud-backend go test ./..." bash -c "cd '$ROOT_DIR/cloud-backend' && go test ./..."
run_check "creator authenticated creation view handler" bash -c "cd '$ROOT_DIR/cloud-backend' && go test ./internal/agents/video/handler -run 'TestCreatorViewHandler(RequiresAuthentication|VerifiesOwnerBeforeReturningView)' -count=1"
run_check "creator strict duration and target assembly integration" bash -c "cd '$ROOT_DIR/cloud-backend' && go test ./internal/agents/video/service -run 'TestCreatorStudio|TestCreationServiceRejectsShotAtFifteenSeconds|TestCreationServiceAcceptsFourteenSecondShot|TestListShotPagePaginatesOneHundredShotsWithStableCursor' -count=1"
run_check "local-backend go test ./..." bash -c "cd '$ROOT_DIR/local-backend' && go test ./..."
run_check "mcp/video_qa unittest" bash -c "cd '$ROOT_DIR' && python3 -m unittest discover -s mcp/video_qa -p 'test*.py'"
run_check "fallback E2E fixture" bash "$ROOT_DIR/scripts/beta-fallback-fixture.sh"

if check_node_module_dir "$ROOT_DIR/frontend" "frontend"; then
  run_check "frontend npm run lint" bash -c "cd '$ROOT_DIR/frontend' && npm run lint"
  run_check "frontend creator studio contract" bash -c "cd '$ROOT_DIR/frontend' && npm run test:creator"
  run_check "frontend developer console gate" bash -c "cd '$ROOT_DIR/frontend' && npm run test:developer-build"
  run_check "frontend npm run build" bash -c "cd '$ROOT_DIR/frontend' && npm run build"
fi

if check_node_module_dir "$ROOT_DIR/hyperframes-render-service" "HyperFrames render service"; then
  run_check "hyperframes-render-service npm run build" bash -c "cd '$ROOT_DIR/hyperframes-render-service' && npm run build"
fi

if [[ "$FAILURES" -gt 0 ]]; then
  printf '\nClosed beta smoke check failed: %d failure(s), %d warning(s).\n' "$FAILURES" "$WARNINGS"
  exit 1
fi

printf '\nClosed beta smoke check passed with %d warning(s).\n' "$WARNINGS"
