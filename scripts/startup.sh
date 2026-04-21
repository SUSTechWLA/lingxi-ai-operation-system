#!/bin/bash
# 灵犀AI OS - 一键启动脚本
# 按顺序启动所有服务

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

# 加载环境变量
if [ -f "$PROJECT_DIR/.env" ]; then
    export $(cat "$PROJECT_DIR/.env" | grep -v '^#' | xargs)
fi

CONTEXT_PID=""
ORCH_PID=""
TRANSLATOR_PID=""
WORKER_PID=""

cleanup() {
    section "清理进程"
    info "停止所有模块进程..."
    [ -n "$CONTEXT_PID" ] && kill $CONTEXT_PID 2>/dev/null && info "已停止 ai-context"
    [ -n "$ORCH_PID" ] && kill $ORCH_PID 2>/dev/null && info "已停止 ai-orchestrator"
    [ -n "$TRANSLATOR_PID" ] && kill $TRANSLATOR_PID 2>/dev/null && info "已停止 ai-nl-translator"
    [ -n "$WORKER_PID" ] && kill $WORKER_PID 2>/dev/null && info "已停止 ai-worker"
    info "清理完成"
}

trap cleanup EXIT

wait_for_url() {
    local url=$1
    local name=$2
    local max_attempts=60
    local attempt=1
    echo -n "等待 $name..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null | grep -qE "^[24]"; then
            echo -e " ${GREEN}✓${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo -e " ${RED}✗ 超时${NC}"
    return 1
}

wait_for_docker() {
    local max_attempts=30
    local attempt=1
    echo -n "等待 Docker 就绪..."
    while [ $attempt -le $max_attempts ]; do
        if docker info &> /dev/null; then
            echo -e " ${GREEN}✓${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    echo -e " ${RED}✗ 超时${NC}"
    return 1
}

section "灵犀AI OS - 一键启动"
echo "项目目录: $PROJECT_DIR"

section "步骤 1: 检查 Docker 基础设施"

if ! command -v docker &> /dev/null; then
    error "Docker 未安装，请先安装 Docker"
    exit 1
fi

if ! docker info &> /dev/null; then
    info "Docker 未运行，正在启动 Docker Desktop..."
    open -a Docker 2>/dev/null || true
    wait_for_docker || { error "Docker 启动失败"; exit 1; }
fi

echo "检查 Docker 容器..."
if ! docker ps --format '{{.Names}}' | grep -q "^lingxi-postgres$"; then
    info "基础设施容器未运行，正在启动..."
    docker compose up -d
    sleep 5
fi

echo "基础设施状态:"
for container in lingxi-postgres lingxi-redis lingxi-redpanda lingxi-minio lingxi-qdrant; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} (未运行)"
    fi
done

section "步骤 2: 启动 AI-Context 模块 (8082)"
cd "$PROJECT_DIR/ai-context"
rm -f /tmp/ai-context.log
nohup mvn spring-boot:run > /tmp/ai-context.log 2>&1 &
CONTEXT_PID=$!
info "ai-context 启动中 (PID: $CONTEXT_PID)..."

section "步骤 3: 启动 AI-Orchestrator 模块 (8080)"
cd "$PROJECT_DIR/ai-orchestrator"
rm -f /tmp/ai-orchestrator.log
nohup mvn spring-boot:run > /tmp/ai-orchestrator.log 2>&1 &
ORCH_PID=$!
info "ai-orchestrator 启动中 (PID: $ORCH_PID)..."

section "步骤 4: 启动 AI-Worker 模块 (8083)"
cd "$PROJECT_DIR/ai-worker"
rm -f /tmp/ai-worker.log
nohup mvn spring-boot:run > /tmp/ai-worker.log 2>&1 &
WORKER_PID=$!
info "ai-worker 启动中 (PID: $WORKER_PID)..."

section "步骤 5: 启动 NL-Translator 模块 (8081)"
cd "$PROJECT_DIR/ai-nl-translator"
rm -f /tmp/nl-translator.log
nohup mvn spring-boot:run > /tmp/nl-translator.log 2>&1 &
TRANSLATOR_PID=$!
info "ai-nl-translator 启动中 (PID: $TRANSLATOR_PID)..."

section "步骤 6: 等待所有模块就绪"
wait_for_url "http://localhost:8082/api/health" "AI-Context"
wait_for_url "http://localhost:8080/api/health" "AI-Orchestrator"
wait_for_url "http://localhost:8083/api/health" "AI-Worker"
wait_for_url "http://localhost:8081/api/health" "NL-Translator"

section "步骤 7: 运行 API 测试"
sleep 3
"$SCRIPT_DIR/test-apis.sh" || true

section "启动完成！"
echo -e "${GREEN}✅ 所有服务已成功启动！${NC}"
echo ""
echo "服务地址:"
echo "  - AI-Context:      http://localhost:8082"
echo "  - AI-Orchestrator: http://localhost:8080"
echo "  - NL-Translator:   http://localhost:8081"
echo "  - AI-Worker:       http://localhost:8083"
echo ""
echo "日志文件:"
echo "  - AI-Context:      tail -f /tmp/ai-context.log"
echo "  - AI-Orchestrator: tail -f /tmp/ai-orchestrator.log"
echo "  - NL-Translator:   tail -f /tmp/nl-translator.log"
echo "  - AI-Worker:       tail -f /tmp/ai-worker.log"
echo ""
echo "配置文件位置:"
echo "  - .env:  项目根目录下的 .env 文件"
echo ""
echo "按 Ctrl+C 停止所有服务"
echo ""

wait
