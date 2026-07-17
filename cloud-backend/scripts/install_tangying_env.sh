#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }
section() { echo -e "\n${BLUE}========================================${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

section "AIOS - Environment Setup"

# Step 1: Check Go
section "Step 1: Check Go 1.23+"

if command -v go &> /dev/null; then
    info "Go installed: $(go version)"
else
    error "Go not installed, need Go 1.23+"
    echo "Install:"
    echo "  macOS:  brew install go"
    echo "  Linux:  https://go.dev/dl/"
    exit 1
fi

# Step 2: Check Docker
section "Step 2: Check Docker"

if command -v docker &> /dev/null; then
    if docker info &> /dev/null; then
        info "Docker running: $(docker --version)"
    else
        warn "Docker installed but not running, start Docker Desktop"
    fi
else
    error "Docker not installed"
    exit 1
fi

# Step 3: Configure .env
section "Step 3: Configure Environment"

if [ -f "$PROJECT_DIR/.env" ]; then
    info ".env file exists"
else
    cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"
    info "Created .env from .env.example"
    warn "Please edit .env and set POSTGRES_PASSWORD / AUTH_TOKEN_SECRET as needed"
fi

# Step 4: Start infrastructure
section "Step 4: Start Docker Infrastructure"

docker compose up -d

echo "Waiting for PostgreSQL..."
for i in $(seq 1 30); do
    if docker exec tangying-postgres pg_isready &> /dev/null 2>&1; then
        echo -e " ${GREEN}ready${NC}"
        break
    fi
    echo -n "."
    sleep 2
done

echo ""
echo "Infrastructure status:"
for container in tangying-postgres tangying-redis tangying-redpanda tangying-minio tangying-qdrant; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} (not running)"
    fi
done

# Step 5: Install Go dependencies
section "Step 5: Install Go Dependencies"

go mod tidy
info "Go dependencies installed"

# Step 6: Build
section "Step 6: Build"

mkdir -p build
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
info "Build complete: build/tangying-ai-os"

section "Setup Complete!"
echo ""
echo -e "${GREEN}Next steps:${NC}"
echo "  1. Edit .env and set POSTGRES_PASSWORD / AUTH_TOKEN_SECRET as needed"
echo "     vim $PROJECT_DIR/.env"
echo "     Model providers are configured in the desktop app local settings"
echo ""
echo "  2. Run the service"
echo "     make run"
echo ""
echo "  3. Test APIs"
echo "     ./scripts/test-apis.sh"
