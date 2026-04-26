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
# Step 3: Start backend
# ============================================
section "Step 3: Start Backend (port 8080)"

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
# Step 4: Start frontend
# ============================================
section "Step 4: Start Frontend (port 3000)"

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
echo -e "   ${GREEN}1${NC} DirectExecutor — Local subprocess with sandbox dir"
echo -e "   ${GREEN}2${NC} SandboxExecutor — gRPC sandbox (stub, ready for Rust integration)"
echo -e ""
echo -e "  ${CYAN}Built-in Tools:${NC}"
echo -e "   • Bash     — Command whitelist + dangerous pattern filter + /tmp/lingxi-sandbox"
echo -e "   • Python   — python3 -c execution via DirectExecutor"
echo -e "   • LLM API  — OpenAI chat/completions calls"
echo -e "   • Weather  — demo/template tool"
echo -e ""
echo -e "  ${CYAN}Sandbox enables:${NC}"
echo -e "   • Resource isolation (memory, CPU, disk limits)"
echo -e "   • Remote gRPC execution endpoint (config: SANDBOX_ADDRESS)"
echo -e "   • Fallback to DirectExecutor when sandbox unavailable (SANDBOX_FALLBACK=true)"
echo -e ""

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
echo -e "   /api/task/*                 — Task orchestration"
echo -e "   /api/translate/*            — NL-to-DAG translation"
echo -e ""
echo -e "Press Ctrl+C to stop all services"
echo ""

wait
