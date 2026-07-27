#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
compose_file="$repo_root/cloud-backend/docker-compose.yml"
runtime_env="$repo_root/cloud-backend/.env.one-click"
compose_project="cloud-backend"
runtime_dir="$repo_root/.run/one-click"
hyperframes_pid_file="$runtime_dir/hyperframes.pid"
hyperframes_log_file="$runtime_dir/hyperframes.log"
hyperframes_error_log_file="$runtime_dir/hyperframes.error.log"
hyperframes_launch_label="com.tangying.ai-os.hyperframes"
command_name="up"
dry_run="false"
skip_install="false"
skip_package="false"
open_app="false"
observability_source_migration_from=""
observability_source_migration_option_seen="false"

usage() {
  cat <<'USAGE'
Usage: bash scripts/one-click-deploy.sh [up|status|down] [options]

Commands:
  up                 Install locked dependencies, start the Docker backend,
                     wait for health, and package the desktop client (default).
  status             Show Compose state and probe cloud/renderer health.
  down               Stop and remove service containers; named data volumes remain.

Options:
  --dry-run          Print the execution plan without changing the machine.
  --skip-install     Reuse existing Node dependencies.
  --skip-package     Start services without packaging the desktop client.
  --open             Open the packaged app after a successful build (macOS only).
  --migrate-observability-source-from development
                     Explicitly approve the authenticated development-to-production
                     terminal-envelope migration. No source is inferred.
  -h, --help         Show this help.

Examples:
  bash scripts/one-click-deploy.sh up
  bash scripts/one-click-deploy.sh up --migrate-observability-source-from development
  bash scripts/one-click-deploy.sh up --open
  bash scripts/one-click-deploy.sh status
  bash scripts/one-click-deploy.sh down
USAGE
}

log() {
  printf '[tangying] %s\n' "$*"
}

die() {
  printf '[tangying] ERROR: %s\n' "$*" >&2
  exit 1
}

compose() {
  docker compose --project-name "$compose_project" --env-file "$runtime_env" -f "$compose_file" "$@"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command is missing: $1"
}

runtime_env_owner() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    stat -f '%u' "$1"
  else
    stat -c '%u' "$1"
  fi
}

runtime_env_mode() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    stat -f '%Lp' "$1"
  else
    stat -c '%a' "$1"
  fi
}

runtime_env_group() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    stat -f '%g' "$1"
  else
    stat -c '%g' "$1"
  fi
}

valid_previous_observability_source_environments() {
  [[ "$1" == "" || "$1" == "development" ]]
}

valid_observability_sealing_key() {
  local key="$1" transport decoded_file canonical byte_count distinct_count key_hex key_hex_length
  local block_chars position block repeated="false" decode_ok="true" placeholder="false"
  [[ "$key" == base64:* ]] || return 1
  transport="${key#base64:}"
  [[ -n "$transport" && "$transport" =~ ^[A-Za-z0-9+/]+={0,2}$ ]] || return 1
  decoded_file="$(mktemp "${TMPDIR:-/tmp}/tangying-observability-key.XXXXXX")"
  chmod 600 "$decoded_file"
  if ! printf '%s' "$transport" | openssl base64 -d -A >"$decoded_file" 2>/dev/null; then
    decode_ok="false"
  fi
  canonical="$(openssl base64 -A -in "$decoded_file" 2>/dev/null || true)"
  byte_count="$(wc -c <"$decoded_file" | tr -d ' ')"
  distinct_count="$(od -An -tu1 "$decoded_file" | tr -s '[:space:]' '\n' | sed '/^$/d' | sort -u | wc -l | tr -d ' ')"
  key_hex="$(od -An -tx1 "$decoded_file" | tr -d '[:space:]')"
  if LC_ALL=C grep -Eiq 'replace-with|placeholder|changeme|development-only' "$decoded_file"; then
    placeholder="true"
  fi
  rm -f -- "$decoded_file"
  if [[ "$decode_ok" != "true" || "$canonical" != "$transport" || "$byte_count" -lt 32 || "$byte_count" -gt 64 || "$distinct_count" -lt 16 || "$placeholder" == "true" ]]; then
    return 1
  fi
  key_hex_length="${#key_hex}"
  for ((block_chars = 2; block_chars <= key_hex_length / 2; block_chars += 2)); do
    ((key_hex_length % block_chars == 0)) || continue
    block="${key_hex:0:block_chars}"
    repeated="true"
    for ((position = block_chars; position < key_hex_length; position += block_chars)); do
      if [[ "${key_hex:position:block_chars}" != "$block" ]]; then
        repeated="false"
        break
      fi
    done
    [[ "$repeated" != "true" ]] || return 1
  done
  return 0
}

validate_existing_runtime_env_metadata() {
  [[ -f "$runtime_env" && ! -L "$runtime_env" ]] || die "runtime environment must be a regular non-symlink file"
  local owner group mode
  owner="$(runtime_env_owner "$runtime_env")" || die "runtime environment owner inspection failed"
  group="$(runtime_env_group "$runtime_env")" || die "runtime environment group inspection failed"
  mode="$(runtime_env_mode "$runtime_env")" || die "runtime environment mode inspection failed"
  [[ "$owner" =~ ^[0-9]+$ && "$owner" == "$(id -u)" ]] || die "runtime environment must be owned by the current user"
  [[ "$group" =~ ^[0-9]+$ ]] || die "runtime environment group is invalid"
  [[ "$mode" =~ ^[0-7]{3}$ ]] || die "runtime environment mode is invalid"
}

secure_runtime_env_replacement() {
  local replacement="$1"
  if ! chmod 600 "$replacement"; then
    rm -f -- "$replacement"
    die "runtime environment replacement chmod failed"
  fi
  if ! chown "$(id -u):$(id -g)" "$replacement"; then
    rm -f -- "$replacement"
    die "runtime environment replacement chown failed"
  fi
  if [[ "$(runtime_env_owner "$replacement")" != "$(id -u)" ||
        "$(runtime_env_group "$replacement")" != "$(id -g)" ||
        "$(runtime_env_mode "$replacement")" != "600" ]]; then
    rm -f -- "$replacement"
    die "runtime environment replacement permission verification failed"
  fi
}

replace_runtime_env_atomically() {
  local replacement="$1"
  local expected_existing="$2"
  secure_runtime_env_replacement "$replacement"
  if [[ "${TANGYING_DEPLOY_TEST:-}" == "1" &&
        "${TANGYING_DEPLOY_TEST_ATOMIC_RENAME_FAIL:-}" == "1" ]]; then
    rm -f -- "$replacement"
    die "atomic runtime environment replacement failed"
  fi
  if [[ "$expected_existing" == "false" &&
        ( -e "$runtime_env" || -L "$runtime_env" ) ]]; then
    rm -f -- "$replacement"
    die "refusing to overwrite runtime environment"
  fi
  if ! mv -f -- "$replacement" "$runtime_env"; then
    rm -f -- "$replacement"
    die "atomic runtime environment replacement failed"
  fi
}

write_runtime_env() {
  local xtrace_was_set="false"
  if [[ "$-" == *x* ]]; then
    xtrace_was_set="true"
    set +x
  fi
  umask 077
  case "$observability_source_migration_from" in
    "") ;;
    development) ;;
    *) die "only supported observability source migration is development" ;;
  esac
  if [[ -e "$runtime_env" || -L "$runtime_env" ]]; then
    validate_existing_runtime_env_metadata
    local observability_key_count=0 observability_domain_count=0 observability_source_count=0
    local observability_previous_count=0 observability_approval_count=0 tool_registration_token_count=0
    local observability_sealing_key="" observability_sealing_domain=""
    local observability_source_environment="" observability_previous_environments=""
    local observability_source_migration_approved="" line normalized_line
    local had_crlf="false" upgraded="false"
    local existing_mode existing_group
    local runtime_env_lines=()
    existing_mode="$(runtime_env_mode "$runtime_env")"
    existing_group="$(runtime_env_group "$runtime_env")"
    while IFS= read -r line || [[ -n "$line" ]]; do
      normalized_line="$line"
      if [[ "$normalized_line" == *$'\r'* ]]; then
        if [[ "$normalized_line" != *$'\r' ]]; then
          die "runtime environment contains unsupported carriage returns; use LF or CRLF line endings only"
        fi
        normalized_line="${normalized_line%$'\r'}"
        if [[ "$normalized_line" == *$'\r'* ]]; then
          die "runtime environment contains unsupported carriage returns; use LF or CRLF line endings only"
        fi
        had_crlf="true"
      fi
      runtime_env_lines+=("$normalized_line")
      case "$normalized_line" in
        OBSERVABILITY_SEALING_KEY=*)
          observability_key_count=$((observability_key_count + 1))
          observability_sealing_key="${normalized_line#OBSERVABILITY_SEALING_KEY=}"
          ;;
        OBSERVABILITY_SEALING_DOMAIN=*)
          observability_domain_count=$((observability_domain_count + 1))
          observability_sealing_domain="${normalized_line#OBSERVABILITY_SEALING_DOMAIN=}"
          ;;
        OBSERVABILITY_SOURCE_ENVIRONMENT=*)
          observability_source_count=$((observability_source_count + 1))
          observability_source_environment="${normalized_line#OBSERVABILITY_SOURCE_ENVIRONMENT=}"
          ;;
        OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=*)
          observability_previous_count=$((observability_previous_count + 1))
          observability_previous_environments="${normalized_line#OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=}"
          ;;
        OBSERVABILITY_SOURCE_MIGRATION_APPROVED=*)
          observability_approval_count=$((observability_approval_count + 1))
          observability_source_migration_approved="${normalized_line#OBSERVABILITY_SOURCE_MIGRATION_APPROVED=}"
          ;;
        TOOL_REGISTRATION_INTERNAL_TOKEN=*)
          tool_registration_token_count=$((tool_registration_token_count + 1))
          ;;
      esac
    done <"$runtime_env"

    if [[ "$observability_key_count" != "1" ]]; then
      die "existing OBSERVABILITY_SEALING_KEY is missing or duplicated; do not auto-rotate: restore the prior key, or drain pending terminal events before rotation"
    fi
    [[ "$observability_domain_count" -le 1 ]] || die "existing OBSERVABILITY_SEALING_DOMAIN is duplicated"
    [[ "$observability_source_count" -le 1 ]] || die "existing OBSERVABILITY_SOURCE_ENVIRONMENT is duplicated"
    [[ "$observability_previous_count" -le 1 ]] || die "existing OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS is duplicated"
    [[ "$observability_approval_count" -le 1 ]] || die "existing OBSERVABILITY_SOURCE_MIGRATION_APPROVED is duplicated"
    [[ "$tool_registration_token_count" -le 1 ]] || die "existing TOOL_REGISTRATION_INTERNAL_TOKEN is duplicated"
    if ! valid_observability_sealing_key "$observability_sealing_key"; then
      die "existing OBSERVABILITY_SEALING_KEY is invalid; do not auto-rotate: re-encode the exact prior key bytes as base64:, or drain pending terminal events before rotation"
    fi
    if [[ "$observability_domain_count" == "1" &&
          "$observability_sealing_domain" != "cloud-agent-terminal-v1" ]]; then
      die "existing OBSERVABILITY_SEALING_DOMAIN is invalid for one-click; expected cloud-agent-terminal-v1"
    fi
    if [[ "$observability_source_count" == "1" &&
          "$observability_source_environment" != "production" ]]; then
      die "existing OBSERVABILITY_SOURCE_ENVIRONMENT is invalid for one-click; expected production"
    fi
    if ! valid_previous_observability_source_environments "$observability_previous_environments"; then
      die "existing OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS is invalid for one-click"
    fi
    local desired_previous_environment desired_migration_approval
    if [[ "$observability_approval_count" == "1" ]]; then
      case "$observability_source_migration_approved" in
        none)
          if [[ "$observability_previous_count" == "1" &&
                -n "$observability_previous_environments" ]]; then
            die "observability source migration marker none requires an empty previous source"
          fi
          desired_previous_environment=""
          desired_migration_approval="none"
          ;;
        development-\>production)
          if [[ "$observability_previous_count" != "1" ||
                "$observability_previous_environments" != "development" ]]; then
            die "observability source migration approved marker requires previous source development"
          fi
          desired_previous_environment="development"
          desired_migration_approval="development->production"
          ;;
        *)
          die "observability source migration approval marker is invalid"
          ;;
      esac
    elif [[ "$observability_previous_environments" == "development" ]]; then
      if [[ "$observability_source_migration_from" != "development" ]]; then
        die "unapproved development observability lineage; rerun with --migrate-observability-source-from development"
      fi
      desired_previous_environment="development"
      desired_migration_approval="development->production"
    else
      desired_previous_environment=""
      desired_migration_approval="none"
    fi

    if [[ "$observability_source_migration_from" == "development" ]]; then
      desired_previous_environment="development"
      desired_migration_approval="development->production"
    fi

    local output_lines=() output_line
    for line in "${runtime_env_lines[@]}"; do
      output_line="$line"
      case "$line" in
        OBSERVABILITY_SEALING_DOMAIN=*)
          output_line="OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1"
          ;;
        OBSERVABILITY_SOURCE_ENVIRONMENT=*)
          output_line="OBSERVABILITY_SOURCE_ENVIRONMENT=production"
          ;;
        OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=*)
          output_line="OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=${desired_previous_environment}"
          ;;
        OBSERVABILITY_SOURCE_MIGRATION_APPROVED=*)
          output_line="OBSERVABILITY_SOURCE_MIGRATION_APPROVED=${desired_migration_approval}"
          ;;
      esac
      output_lines+=("$output_line")
      [[ "$output_line" == "$line" ]] || upgraded="true"
    done
    if [[ "$observability_domain_count" == "0" ]]; then
      output_lines+=("OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1")
      upgraded="true"
    fi
    if [[ "$observability_source_count" == "0" ]]; then
      output_lines+=("OBSERVABILITY_SOURCE_ENVIRONMENT=production")
      upgraded="true"
    fi
    if [[ "$observability_previous_count" == "0" ]]; then
      output_lines+=("OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=${desired_previous_environment}")
      upgraded="true"
    fi
    if [[ "$observability_approval_count" == "0" ]]; then
      output_lines+=("OBSERVABILITY_SOURCE_MIGRATION_APPROVED=${desired_migration_approval}")
      upgraded="true"
    fi
    if [[ "$tool_registration_token_count" == "0" ]]; then
      output_lines+=("TOOL_REGISTRATION_INTERNAL_TOKEN=$(openssl rand -hex 32)")
      upgraded="true"
    fi
    if [[ "$had_crlf" == "true" || "$existing_mode" != "600" ||
          "$existing_group" != "$(id -g)" ]]; then
      upgraded="true"
    fi

    if [[ "$upgraded" == "true" ]]; then
      local replacement
      replacement="$(mktemp "${runtime_env}.tmp.XXXXXX")"
      : >"$replacement"
      for line in "${output_lines[@]}"; do
        if ! printf '%s\n' "$line" >>"$replacement"; then
          rm -f -- "$replacement"
          die "runtime environment replacement write failed"
        fi
      done
      replace_runtime_env_atomically "$replacement" "true"
      log "upgraded private runtime environment after complete validation without rotating durable observability sealing"
    fi
    if [[ "$desired_migration_approval" == "development->production" &&
          "$observability_source_migration_from" == "development" ]]; then
      log "approved observability source migration: development->production (sealing key unchanged)"
    fi
  else
    if [[ -n "$observability_source_migration_from" ]]; then
      die "observability source migration requires the prior sealing identity; restore the existing environment and sealing key before retrying"
    fi
    local auth_secret observability_sealing_key tool_registration_token postgres_password minio_access_key minio_secret_key desktop_data_root
    auth_secret="$(openssl rand -hex 32)"
    while :; do
      observability_sealing_key="base64:$(openssl rand -base64 32 | tr -d '\r\n')"
      valid_observability_sealing_key "$observability_sealing_key" && break
    done
    tool_registration_token="$(openssl rand -hex 32)"
    postgres_password="$(openssl rand -hex 24)"
    minio_access_key="$(openssl rand -hex 12)"
    minio_secret_key="$(openssl rand -hex 24)"
    if [[ "$(uname -s)" == "Darwin" ]]; then
      desktop_data_root="${TANGYING_DESKTOP_DATA_ROOT:-${HOME:?}/Library/Application Support/tangying-frontend/local-agent}"
    else
      desktop_data_root="${TANGYING_DESKTOP_DATA_ROOT:-${XDG_DATA_HOME:-${HOME:?}/.local/share}/tangying-frontend/local-agent}"
    fi
    mkdir -p "$desktop_data_root"
    local replacement
    replacement="$(mktemp "${runtime_env}.tmp.XXXXXX")"
    if ! {
      printf 'POSTGRES_DB=tangying_db\n'
      printf 'POSTGRES_USER=postgres\n'
      printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password"
      printf 'MINIO_ACCESS_KEY=%s\n' "$minio_access_key"
      printf 'MINIO_SECRET_KEY=%s\n' "$minio_secret_key"
      printf 'AUTH_TOKEN_SECRET=%s\n' "$auth_secret"
      printf 'TOOL_REGISTRATION_INTERNAL_TOKEN=%s\n' "$tool_registration_token"
      printf 'OBSERVABILITY_SEALING_KEY=%s\n' "$observability_sealing_key"
      printf 'OBSERVABILITY_SEALING_DOMAIN=cloud-agent-terminal-v1\n'
      printf 'OBSERVABILITY_SOURCE_ENVIRONMENT=production\n'
      printf 'OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS=\n'
      printf 'OBSERVABILITY_SOURCE_MIGRATION_APPROVED=none\n'
      printf 'CORS_ALLOWED_ORIGINS=http://localhost:3000,http://127.0.0.1:3000,null\n'
      printf 'TANGYING_DESKTOP_DATA_ROOT=%s\n' "$desktop_data_root"
    } >"$replacement"; then
      rm -f -- "$replacement"
      die "runtime environment replacement write failed"
    fi
    replace_runtime_env_atomically "$replacement" "false"
    log "created private runtime environment: cloud-backend/.env.one-click"
  fi
  if [[ "$xtrace_was_set" == "true" ]]; then
    set -x
  fi
}

compose_service_is_running() {
  local service_name="$1"
  compose ps --services --filter status=running 2>/dev/null | grep -Fxq "$service_name"
}

port_is_listening() {
  local port="$1"
  lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
}

assert_managed_port() {
  local port="$1"
  local service_name="$2"
  if port_is_listening "$port" && ! compose_service_is_running "$service_name"; then
    die "port is already occupied by a non-Tangying service: $port; stop it explicitly and retry"
  fi
}

managed_hyperframes_is_running() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    launchctl print "gui/$(id -u)/$hyperframes_launch_label" >/dev/null 2>&1 && port_is_listening 8787
    return
  fi
  [[ -f "$hyperframes_pid_file" ]] || return 1
  local pid command_line
  pid="$(sed -n '1p' "$hyperframes_pid_file")"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  command_line="$(ps -o command= -p "$pid")"
  [[ "$command_line" == *"$repo_root/hyperframes-render-service/dist/server.js"* ]]
}

start_hyperframes() {
  if managed_hyperframes_is_running; then
    log "HyperFrames host service already managed"
    return
  fi
  if port_is_listening 8787; then
    die "port is already occupied by a non-Tangying service: 8787; stop it explicitly and retry"
  fi
  local desktop_data_root
  desktop_data_root="$(sed -n 's/^TANGYING_DESKTOP_DATA_ROOT=//p' "$runtime_env")"
  [[ -n "$desktop_data_root" ]] || die "TANGYING_DESKTOP_DATA_ROOT is missing from runtime environment"
  mkdir -p "$runtime_dir"
  local pid="" node_path
  node_path="$(command -v node)"
  if [[ "$(uname -s)" == "Darwin" ]]; then
    launchctl remove "$hyperframes_launch_label" >/dev/null 2>&1 || true
    launchctl submit \
      -l "$hyperframes_launch_label" \
      -o "$hyperframes_log_file" \
      -e "$hyperframes_error_log_file" \
      -- /usr/bin/env \
      PORT=8787 \
      HOST=127.0.0.1 \
      HYPERFRAMES_PROJECT_ROOT="$desktop_data_root" \
      HYPERFRAMES_OUTPUT_ROOT="$desktop_data_root" \
      "$node_path" "$repo_root/hyperframes-render-service/dist/server.js"
  else
    nohup env \
      PORT=8787 \
      HOST=127.0.0.1 \
      HYPERFRAMES_PROJECT_ROOT="$desktop_data_root" \
      HYPERFRAMES_OUTPUT_ROOT="$desktop_data_root" \
      "$node_path" "$repo_root/hyperframes-render-service/dist/server.js" >"$hyperframes_log_file" 2>"$hyperframes_error_log_file" &
    pid=$!
    printf '%s\n' "$pid" >"$hyperframes_pid_file"
  fi
  for _attempt in $(seq 1 40); do
    if curl --fail --silent "http://127.0.0.1:8787/health" >/dev/null; then
      log "HyperFrames host service started${pid:+ (pid $pid)}"
      return
    fi
    if [[ -n "$pid" ]] && ! kill -0 "$pid" 2>/dev/null; then
      die "HyperFrames exited during startup; inspect .run/one-click/hyperframes.log"
    fi
    sleep 0.5
  done
  die "HyperFrames did not become healthy; inspect .run/one-click/hyperframes.log"
}

stop_hyperframes() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    if launchctl print "gui/$(id -u)/$hyperframes_launch_label" >/dev/null 2>&1; then
      launchctl remove "$hyperframes_launch_label"
      log "HyperFrames host service stopped"
    fi
    return
  fi
  if ! managed_hyperframes_is_running; then
    return
  fi
  local pid
  pid="$(sed -n '1p' "$hyperframes_pid_file")"
  kill -TERM "$pid"
  for _attempt in $(seq 1 20); do
    if ! kill -0 "$pid" 2>/dev/null; then
      rm -f "$hyperframes_pid_file"
      log "HyperFrames host service stopped"
      return
    fi
    sleep 0.25
  done
  die "managed HyperFrames process did not stop cleanly (pid $pid)"
}

health_probe() {
  local label="$1"
  local url="$2"
  if curl --fail --silent --show-error "$url" >/dev/null; then
    log "$label: healthy ($url)"
    return 0
  fi
  log "$label: unavailable ($url)"
  return 1
}

find_desktop_app() {
  find "$repo_root/frontend/release" -maxdepth 3 -type d -name '*.app' -print -quit 2>/dev/null
}

find_desktop_installer() {
  find "$repo_root/frontend/release" -maxdepth 1 -type f -name '*.dmg' -print -quit 2>/dev/null
}

print_up_plan() {
  if [[ "$skip_install" != "true" ]]; then
    log "PLAN frontend: npm ci"
    log "PLAN hyperframes-render-service: npm ci"
  fi
  log "PLAN cloud-backend: go build for linux"
  log "PLAN HyperFrames host service on 127.0.0.1:8787"
  log "PLAN docker compose up --detach --build --wait"
  log "PLAN probe http://127.0.0.1:8080/api/health/ready"
  log "PLAN probe http://127.0.0.1:8787/health"
  if [[ "$skip_package" != "true" ]]; then
    log "PLAN scripts/build-local-desktop.sh"
  fi
}

run_up() {
  if [[ "$dry_run" == "true" ]]; then
    print_up_plan
    return
  fi

  for dependency in docker curl openssl lsof; do
    require_command "$dependency"
  done
  docker info >/dev/null 2>&1 || die "Docker is installed but the daemon is not running"
  require_command node
  require_command npm
  require_command go
  if [[ "$skip_package" != "true" ]]; then
    require_command ffmpeg
    require_command ffprobe
  fi

  write_runtime_env
  assert_managed_port 8080 cloud-backend
  if port_is_listening 8787 && ! managed_hyperframes_is_running; then
    die "port is already occupied by a non-Tangying service: 8787; stop it explicitly and retry"
  fi

  if [[ "$skip_install" != "true" ]]; then
    log "installing locked frontend dependencies"
    npm ci --prefix "$repo_root/frontend"
    log "installing locked HyperFrames dependencies"
    npm ci --prefix "$repo_root/hyperframes-render-service"
  fi

  log "building HyperFrames host service"
  npm --prefix "$repo_root/hyperframes-render-service" run build
  start_hyperframes

  local go_arch
  go_arch="$(go env GOHOSTARCH)"
  log "building cloud backend for linux/$go_arch"
  mkdir -p "$repo_root/cloud-backend/build"
  (
    cd "$repo_root/cloud-backend"
    CGO_ENABLED=0 GOOS=linux GOARCH="$go_arch" go build -o build/tangying-ai-os ./cmd/tangying-ai-os
  )

  log "building and starting the Docker backend"
  compose up --detach --build --wait
  health_probe "cloud API" "http://127.0.0.1:8080/api/health/ready"
  health_probe "HyperFrames" "http://127.0.0.1:8787/health"

  if [[ "$skip_package" != "true" ]]; then
    log "packaging the desktop client"
    bash "$repo_root/scripts/build-local-desktop.sh" --skip-install
    local desktop_app desktop_installer
    desktop_app="$(find_desktop_app)"
    desktop_installer="$(find_desktop_installer)"
    [[ -n "$desktop_app" ]] || die "desktop build completed without a packaged .app"
    [[ -n "$desktop_installer" ]] || die "desktop build completed without a packaged .dmg"
    log "desktop_app=$desktop_app"
    log "desktop_installer=$desktop_installer"
    if [[ "$open_app" == "true" ]]; then
      [[ "$(uname -s)" == "Darwin" ]] || die "--open is supported only on macOS"
      open "$desktop_app"
    fi
  fi

  log "deployment ready"
}

run_status() {
  if [[ "$dry_run" == "true" ]]; then
    log "PLAN docker compose ps"
    log "PLAN probe http://127.0.0.1:8080/api/health/ready"
    log "PLAN probe http://127.0.0.1:8787/health"
    return
  fi
  require_command docker
  require_command curl
  [[ -f "$runtime_env" ]] || die "runtime environment is missing; run the up command first"
  compose ps
  local failed="false"
  health_probe "cloud API" "http://127.0.0.1:8080/api/health/ready" || failed="true"
  health_probe "HyperFrames" "http://127.0.0.1:8787/health" || failed="true"
  [[ "$failed" == "false" ]] || exit 1
}

run_down() {
  if [[ "$dry_run" == "true" ]]; then
    log "PLAN docker compose down --remove-orphans"
    return
  fi
  require_command docker
  [[ -f "$runtime_env" ]] || die "runtime environment is missing; nothing managed to stop"
  compose down --remove-orphans
  stop_hyperframes
  log "services stopped; named data volumes were preserved"
}

if [[ "${BASH_SOURCE[0]}" != "$0" ]]; then
  return 0
fi

if [[ $# -gt 0 && "$1" != -* ]]; then
  command_name="$1"
  shift
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) dry_run="true" ;;
    --skip-install) skip_install="true" ;;
    --skip-package) skip_package="true" ;;
    --open) open_app="true" ;;
    --migrate-observability-source-from)
      [[ "$observability_source_migration_option_seen" == "false" ]] || die "--migrate-observability-source-from was specified more than once"
      [[ $# -ge 2 && "$2" != -* ]] || die "--migrate-observability-source-from requires a source"
      observability_source_migration_option_seen="true"
      observability_source_migration_from="$2"
      [[ "$observability_source_migration_from" == "development" ]] || die "only supported source is development"
      shift
      ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
  shift
done

if [[ -n "$observability_source_migration_from" && "$command_name" != "up" ]]; then
  die "--migrate-observability-source-from is supported only with the up command"
fi

case "$command_name" in
  up) run_up ;;
  status) run_status ;;
  down) run_down ;;
  help) usage ;;
  *) die "unknown command: $command_name" ;;
esac
