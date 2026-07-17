#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$ROOT_DIR/frontend/resources/bin"

cd "$ROOT_DIR/local-backend"
go build -o "$ROOT_DIR/frontend/resources/bin/tangying-local-agent" ./cmd/local-agent

cd "$ROOT_DIR/frontend"
npm install
npm run build
npm run electron:build
