#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
section() { echo -e "\n${BLUE}========================================${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

if [ -f "$PROJECT_DIR/.env" ]; then
    export $(cat "$PROJECT_DIR/.env" | grep -v '^#' | xargs)
fi

APP_PID=""

cleanup() {
    section "Shutting down"
    [ -n "$APP_PID" ] && kill $APP_PID 2>/dev/null && info "Service stopped"
}
trap cleanup EXIT

section "Lingxi AI OS - Starting"

# Step 1: Docker infrastructure
section "Step 1: Check Docker Infrastructure"

if ! docker info &> /dev/null; then
    info "Starting Docker Desktop..."
    open -a Docker 2>/dev/null || true
fi

if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^lingxi-postgres$"; then
    info "Starting infrastructure containers..."
    docker compose up -d
    sleep 5
fi

for container in lingxi-postgres lingxi-redis lingxi-redpanda; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} (not running)"
    fi
done

# Step 2: Build
section "Step 2: Build"

mkdir -p build
info "Building..."
go build -o build/lingxi-ai-os cmd/lingxi-ai-os/main.go
info "Build complete"

# Step 3: Run
section "Step 3: Start Service"

./build/lingxi-ai-os &
APP_PID=$!

echo -n "Waiting for service..."
for i in $(seq 1 30); do
    if curl -s http://localhost:8080/api/health >/dev/null 2>&1; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    echo -n "."
    sleep 1
done

section "Service Running!"
echo -e "${GREEN}Service: http://localhost:8080${NC}"
echo ""
echo "Press Ctrl+C to stop"
echo ""

wait
