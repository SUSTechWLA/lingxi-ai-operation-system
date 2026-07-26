#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SKIP_INSTALL="false"
GOCACHE="${GOCACHE:-$ROOT_DIR/.run/go-build-cache}"
ELECTRON_BUILDER_CACHE="${ELECTRON_BUILDER_CACHE:-$ROOT_DIR/.run/electron-builder-cache}"
export GOCACHE
export ELECTRON_BUILDER_CACHE

if [[ "${1:-}" == "--skip-install" ]]; then
  SKIP_INSTALL="true"
elif [[ $# -gt 0 ]]; then
  echo "Usage: bash scripts/build-local-desktop.sh [--skip-install]" >&2
  exit 2
fi

mkdir -p "$ROOT_DIR/frontend/resources/bin" "$GOCACHE" "$ELECTRON_BUILDER_CACHE"

cd "$ROOT_DIR/local-backend"
go build -o "$ROOT_DIR/frontend/resources/bin/tangying-local-agent" ./cmd/local-agent

cd "$ROOT_DIR/frontend"
if [[ "$SKIP_INSTALL" != "true" ]]; then
  npm ci
fi
npm run build
npm run electron:build
