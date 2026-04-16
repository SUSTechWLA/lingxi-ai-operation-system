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
section "$1"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONTEXT_PID=""
ORCH_PID=""
TRANSLATOR_PID=""

cleanup() {
    section "清理进程"
    info "停止所有模块进程..."
    [ -n "$CONTEXT_PID" ] && kill $CONTEXT_PID 2>/dev/null && info "已停止 ai-context"
    [ -n "$ORCH_PID" ] && kill $ORCH_PID 2>/dev/null && info "已停止 ai-orchestrator"
    [ -n "$TRANSLATOR_PID" ] && kill $TRANSLATOR_PID 2>/dev/null && info "已停止 nl-translator"
    info "清理完成"
}

trap cleanup EXIT

wait_for_url() {
    local url=$1
    local name=$2
    local max_attempts=30
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

cd "$PROJECT_DIR"

section "步骤 1: Docker 基础设施"

if ! command -v docker &> /dev/null; then
    error "Docker 未安装"
    exit 1
fi

if ! docker info &> /dev/null; then
    info "启动 Docker Desktop..."
    open -a Docker 2>/dev/null || true
    echo -n "等待 Docker 启动..."
    for i in {1..30}; do
        if docker info &> /dev/null; then
            echo -e " ${GREEN}✓${NC}"
            break
        fi
        echo -n "."
        sleep 2
    done
fi

info "清理旧容器..."
docker ps -a --format "{{.Names}}" | grep -E "^lingxi-" | xargs -r docker stop 2>/dev/null || true
docker ps -a --format "{{.Names}}" | grep -E "^lingxi-" | xargs -r docker rm 2>/dev/null || true

info "启动 Docker 服务..."
docker compose up -d
sleep 5

if wait_for_url "http://localhost:5432" "PostgreSQL"; then
    info "PostgreSQL 就绪 (localhost:5432)"
fi

section "步骤 2: 启动 AI-Context 模块 (8082)"
cd "$PROJECT_DIR/ai-context"
nohup mvn spring-boot:run > /tmp/ai-context.log 2>&1 &
CONTEXT_PID=$!
info "ai-context 启动中 (PID: $CONTEXT_PID)..."

section "步骤 3: 启动 AI-Orchestrator 模块 (8080)"
cd "$PROJECT_DIR/ai-orchestrator"
nohup mvn spring-boot:run > /tmp/ai-orchestrator.log 2>&1 &
ORCH_PID=$!
info "ai-orchestrator 启动中 (PID: $ORCH_PID)..."

section "步骤 4: 启动 NL-Translator 模块 (8081)"
cd "$PROJECT_DIR/ai-nl-translator"
nohup mvn spring-boot:run > /tmp/nl-translator.log 2>&1 &
TRANSLATOR_PID=$!
info "nl-translator 启动中 (PID: $TRANSLATOR_PID)..."

section "步骤 5: 等待所有模块就绪"
wait_for_url "http://localhost:8082/api/health" "AI-Context" || warn "AI-Context 启动失败"
wait_for_url "http://localhost:8080/api/task/test" "AI-Orchestrator" || warn "AI-Orchestrator 启动失败"
wait_for_url "http://localhost:8081/api/translate" "NL-Translator" || warn "NL-Translator 启动失败"

section "步骤 6: 运行 API 测试"

passed=0
failed=0

test_api() {
    local name=$1
    local expected_code=$2
    shift 2
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" "$@")
    if echo "$response" | grep -q "$expected_code"; then
        echo -e "  ${GREEN}✓${NC} $name (HTTP $response)"
        passed=$((passed + 1))
    else
        echo -e "  ${RED}✗${NC} $name (期望: $expected_code, 实际: $response)"
        failed=$((failed + 1))
    fi
}

info "6.1 AI-Context 模块测试"
test_api "Health" "200" -X GET http://localhost:8082/api/health
test_api "记录TaskCreated" "200" -X POST http://localhost:8082/api/context/task-created -H "Content-Type: application/json" -d '{"taskId":"test-001"}'

info "6.2 AI-Orchestrator 模块测试"
ORCH_TASK_RESP=$(curl -s -X POST http://localhost:8080/api/task/create -H "Content-Type: application/json" -d '{"input":"测试任务"}')
TEST_TASK_ID=$(echo "$ORCH_TASK_RESP" | python3 -c "import sys, json; print(json.load(sys.stdin).get('taskId', ''))" 2>/dev/null || echo "")
if [ -n "$TEST_TASK_ID" ]; then
    echo -e "  ${GREEN}✓${NC} 创建任务成功: $TEST_TASK_ID"
    passed=$((passed + 1))
    test_api "提交DAG" "200" -X POST "http://localhost:8080/api/task/${TEST_TASK_ID}/dag" -H "Content-Type: application/json" -d '{"nodes":[{"id":"n1","type":"LLM","name":"test"}],"edges":[]}'
    sleep 2
    test_api "查询任务" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}"
    test_api "查询上下文" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}/context"
else
    echo -e "  ${RED}✗${NC} 创建任务失败"
    failed=$((failed + 1))
fi

info "6.3 NL-Translator 模块测试"
test_api "翻译端点" "500" -X POST http://localhost:8081/api/translate -H "Content-Type: application/json" -d '{"prompt":"test"}'
if [ -n "$TEST_TASK_ID" ]; then
    test_api "任务查询代理" "200" -X GET "http://localhost:8081/api/task/${TEST_TASK_ID}"
fi

info "6.4 模块间通信测试"
if [ -n "$TEST_TASK_ID" ]; then
    CTX_COUNT=$(curl -s "http://localhost:8080/api/task/${TEST_TASK_ID}/context" | python3 -c "import sys, json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    if [ "$CTX_COUNT" -gt 0 ]; then
        echo -e "  ${GREEN}✓${NC} Orchestrator -> AI-Context 通信正常 (上下文记录数: $CTX_COUNT)"
        passed=$((passed + 1))
    else
        echo -e "  ${RED}✗${NC} Orchestrator -> AI-Context 通信异常"
        failed=$((failed + 1))
    fi
fi

section "测试总结"
echo -e "通过: ${GREEN}$passed${NC}"
echo -e "失败: ${RED}$failed${NC}"

if [ $failed -eq 0 ]; then
    echo -e "\n${GREEN}✅ 所有测试通过！${NC}"
    echo ""
    echo "服务端口:"
    echo "  - AI-Context:      http://localhost:8082"
    echo "  - AI-Orchestrator: http://localhost:8080"
    echo "  - NL-Translator:   http://localhost:8081"
    exit 0
else
    echo -e "\n${RED}❌ 部分测试失败${NC}"
    echo ""
    echo "日志文件:"
    echo "  - /tmp/ai-context.log"
    echo "  - /tmp/ai-orchestrator.log"
    echo "  - /tmp/nl-translator.log"
    exit 1
fi
