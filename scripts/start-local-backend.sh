#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOCAL_DIR="$ROOT_DIR/local-backend"

if [[ "${1:-}" == "--check" ]]; then
  cd "$LOCAL_DIR"
  go test ./...
  exit 0
fi

cd "$LOCAL_DIR"
exec go run ./cmd/local-agent \
  -addr "${TANGYING_LOCAL_AGENT_ADDR:-127.0.0.1:18080}" \
  -cloud-api-base "${TANGYING_CLOUD_API_BASE:-${VITE_CLOUD_API_BASE:-}}"
