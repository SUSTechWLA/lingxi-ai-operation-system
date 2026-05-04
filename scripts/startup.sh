#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
section() { echo -e "\n${BLUE}========================================${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

if [ -f "$PROJECT_DIR/.env" ]; then
    export $(cat "$PROJECT_DIR/.env" | grep -v '^#' | xargs)
fi

BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
    section "Shutting down"
    [ -n "$FRONTEND_PID" ] && kill $FRONTEND_PID 2>/dev/null && info "Frontend stopped"
    [ -n "$BACKEND_PID" ] && kill $BACKEND_PID 2>/dev/null && info "Backend stopped"
    info "All services stopped"
}
trap cleanup EXIT

section "🐝 Lingxi AI OS — One-Click Start"

# ============================================
# Step 1: Docker infrastructure
# ============================================
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

# Check all 5 containers
all_ok=true
for container in lingxi-postgres lingxi-redis lingxi-redpanda lingxi-minio lingxi-qdrant; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} (not running)"
        all_ok=false
    fi
done

if [ "$all_ok" = false ]; then
    warn "Some containers are not running. Check 'docker compose logs'."
fi

# ============================================
# Step 2: Build backend
# ============================================
section "Step 2: Build Backend"

mkdir -p build
info "Building Go binary..."
go build -o build/lingxi-ai-os cmd/lingxi-ai-os/main.go
info "Backend build complete"

# ============================================
# Step 3: Build sandbox (if Rust toolchain available)
# ============================================
section "Step 3: Build Sandbox Service"

if command -v cargo &> /dev/null || [ -f "$HOME/.cargo/env" ]; then
    [ -f "$HOME/.cargo/env" ] && source "$HOME/.cargo/env"
    if command -v cargo &> /dev/null; then
        info "Building Rust sandbox service..."
        cd sandbox
        cargo build --release --quiet 2>&1 || warn "Sandbox build failed (non-fatal)"
        cd "$PROJECT_DIR"
        if [ -f sandbox/target/release/lingxi-sandbox ]; then
            cp sandbox/target/release/lingxi-sandbox build/
            info "Sandbox build complete"
        fi
    else
        warn "Rust toolchain not found, skipping sandbox build (set SANDBOX_ENABLED=false)"
    fi
else
    warn "Rust toolchain not found, skipping sandbox build (set SANDBOX_ENABLED=false)"
fi

# ============================================
# Step 4: Start backend
# ============================================
section "Step 4: Start Backend (port 8080)"

# Kill any process occupying port 8080
if lsof -ti :8080 &>/dev/null; then
    info "Port 8080 is in use, killing existing process..."
    lsof -ti :8080 | xargs kill -9 2>/dev/null
    sleep 1
    info "Port 8080 freed"
fi

./build/lingxi-ai-os &
BACKEND_PID=$!

echo -n "Waiting for backend..."
for i in $(seq 1 30); do
    if curl -s http://localhost:8080/api/health >/dev/null 2>&1; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    echo -n "."
    sleep 1
done

# ============================================
# Step 5: Start frontend
# ============================================
section "Step 5: Start Frontend (port 3000)"

cd frontend

if [ ! -d "node_modules" ]; then
    info "Installing frontend dependencies..."
    npm install
fi

info "Starting Vite dev server..."
npm run dev &
FRONTEND_PID=$!
cd "$PROJECT_DIR"

echo -n "Waiting for frontend..."
for i in $(seq 1 20); do
    if curl -s http://localhost:3000 >/dev/null 2>&1; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    echo -n "."
    sleep 1
done

# ============================================
# Worker sandbox overview
# ============================================
section "Worker Module Overview"

echo -e ""
echo -e "  ${CYAN}Execution Modes:${NC}"
echo -e "   ${GREEN}1${NC} DirectExecutor — Local subprocess execution (default)"
echo -e "   ${GREEN}2${NC} SandboxExecutor — Rust gRPC sandbox with resource isolation"
echo -e ""
echo -e "  ${CYAN}Built-in Tools:${NC}"
echo -e "   • Bash     — Command whitelist + dangerous pattern filter"
echo -e "   • Python   — python3 -c execution via DirectExecutor"
echo -e "   • LLM API  — OpenAI chat/completions calls"
echo -e "   • Polisher — Text polish for social media titles/descriptions"
echo -e "   • Media Analyzer  — Analyze images/videos for tags and suggestions"
echo -e "   • Content Generator — Generate full content from media analysis"
echo -e "   • Content Checker — Compliance check for sensitive/ad words"
echo -e "   • Platform Adapter — Adapt content for 7 social media platforms"
echo -e ""
echo -e "  ${CYAN}Sandbox enables:${NC}"
echo -e "   • Resource isolation (memory, CPU, disk, PID limits via setrlimit)"
echo -e "   • gRPC-based remote execution (config: SANDBOX_ADDRESS)"
echo -e "   • Temp directory isolation with automatic cleanup"
echo -e "   • Timeout enforcement at sandbox level"
echo -e "   • Fallback to DirectExecutor when sandbox unavailable (SANDBOX_FALLBACK=true)"
echo -e ""
echo -e "  ${CYAN}To enable sandbox:${NC}"
echo -e "   1. Set SANDBOX_ENABLED=true in .env"
echo -e "   2. Start the sandbox service: ./build/lingxi-sandbox &"
echo -e "   3. Restart the backend"

# ============================================
# Running
# ============================================
section "All Services Running!"

echo -e "${GREEN}Backend:  http://localhost:8080${NC}"
echo -e "${GREEN}Frontend: http://localhost:3000${NC}"
echo -e ""
echo -e "  ${CYAN}API Endpoints:${NC}"
echo -e "   /api/health                 — Health check"
echo -e "   /api/publish                — Content publishing with media upload"
echo -e "   /api/ai/generate            — AI content generation from text"
echo -e "   /api/ai/generate-from-media — AI content generation from images/videos"
echo -e "   /api/ai/polish              — AI text polish (title/description)"
echo -e "   /api/media/*                — Media management (upload/list/tags)"
echo -e "   /api/trace/*                — Task trace query"
echo -e "   /api/task/*                 — Task orchestration"
echo -e "   /api/translate/*            — NL-to-DAG translation"
echo -e ""
echo -e "Press Ctrl+C to stop all services"
echo ""

wait
