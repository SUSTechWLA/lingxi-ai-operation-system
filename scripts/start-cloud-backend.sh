#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CLOUD_DIR="$ROOT_DIR/cloud-backend"

if [[ "${1:-}" == "--check" ]]; then
  cd "$CLOUD_DIR"
  go test ./...
  exit 0
fi

cd "$CLOUD_DIR"
if [[ ! -f .env && -f .env.example ]]; then
  cp .env.example .env
fi

export SKILL_ROOT="${SKILL_ROOT:-$CLOUD_DIR/skills}"
export AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG="${AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG:-true}"

docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
exec ./build/tangying-ai-os
