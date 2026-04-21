#!/bin/bash
# 灵犀AI OS - API测试脚本
# 测试所有模块的API接口

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

passed=0
failed=0

test_api() {
    local name=$1
    local expected_code=$2
    shift 2
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" "$@" 2>/dev/null || echo "000")
    if echo "$response" | grep -q "$expected_code"; then
        echo -e "  ${GREEN}✓${NC} $name (HTTP $response)"
        passed=$((passed + 1))
        return 0
    else
        echo -e "  ${RED}✗${NC} $name (期望: $expected_code, 实际: $response)"
        failed=$((failed + 1))
        return 1
    fi
}

section "灵犀AI OS - API 测试"

section "步骤 1: 检查基础设施"

echo "检查Docker容器..."
if docker ps --format '{{.Names}}' | grep -q "lingxi-postgres"; then
    echo -e "  ${GREEN}✓${NC} PostgreSQL 运行中"
else
    echo -e "  ${RED}✗${NC} PostgreSQL 未运行，请先启动: docker compose up -d"
fi

if docker ps --format '{{.Names}}' | grep -q "lingxi-redpanda"; then
    echo -e "  ${GREEN}✓${NC} Redpanda 运行中"
else
    echo -e "  ${YELLOW}⚠${NC}  Redpanda 未运行，部分功能可能受限"
fi

section "步骤 2: AI-Context 模块测试 (端口 8082)"
test_api "Health检查" "200" -X GET http://localhost:8082/api/health

section "步骤 3: AI-Orchestrator 模块测试 (端口 8080)"
test_api "Health检查" "200" -X GET http://localhost:8080/api/health

echo ""
echo "创建测试任务..."
TASK_RESP=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "api-test-user"}')
TEST_TASK_ID=$(echo "$TASK_RESP" | python3 -c "import sys, json; print(json.load(sys.stdin).get('taskId', ''))" 2>/dev/null || echo "")

if [ -n "$TEST_TASK_ID" ]; then
    echo -e "  ${GREEN}✓${NC} 创建任务成功: $TEST_TASK_ID"
    passed=$((passed + 1))

    echo ""
    echo "提交DAG..."
    TEST_NODE_ID="api-test-node-$(date +%s)"
    test_api "提交DAG" "200" -X POST "http://localhost:8080/api/task/${TEST_TASK_ID}/dag" \
        -H "Content-Type: application/json" \
        -d "{\"nodes\":[{\"id\":\"${TEST_NODE_ID}\",\"type\":\"LLM\",\"name\":\"test\",\"input\":{}}],\"edges\":[]}"

    sleep 2
    test_api "查询任务" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}"
    test_api "查询上下文" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}/context"
else
    echo -e "  ${RED}✗${NC} 创建任务失败"
    failed=$((failed + 1))
fi

section "步骤 4: AI-Worker 模块测试 (端口 8083)"
test_api "Health检查" "200" -X GET http://localhost:8083/api/health
test_api "获取工具列表" "200" -X GET http://localhost:8083/api/tools

section "步骤 5: NL-Translator 模块测试 (端口 8081)"
test_api "Health检查" "200" -X GET http://localhost:8081/api/health

echo ""
echo "测试自然语言转DAG..."
TRANSLATE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8081/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt":"test"}' 2>/dev/null || echo "000")

if [ "$TRANSLATE_CODE" = "503" ]; then
    echo -e "  ${YELLOW}⚠${NC}  NL-Translator 需要配置 OpenAI API Key (HTTP 503)"
    echo -e "     设置环境变量: export OPENAI_API_KEY=your-key"
    passed=$((passed + 1))
elif [ "$TRANSLATE_CODE" = "200" ]; then
    echo -e "  ${GREEN}✓${NC} NL-Translator 正常工作 (HTTP 200)"
    passed=$((passed + 1))
else
    echo -e "  ${RED}✗${NC} NL-Translator 异常 (HTTP $TRANSLATE_CODE)"
    failed=$((failed + 1))
fi

if [ -n "$TEST_TASK_ID" ]; then
    test_api "任务查询代理" "200" -X GET "http://localhost:8081/api/task/${TEST_TASK_ID}"
fi

section "测试总结"
echo -e "通过: ${GREEN}$passed${NC}"
echo -e "失败: ${RED}$failed${NC}"

if [ $failed -eq 0 ]; then
    echo -e "\n${GREEN}✅ 所有API测试通过！${NC}"
    echo ""
    echo "服务地址:"
    echo "  - AI-Context:      http://localhost:8082"
    echo "  - AI-Orchestrator: http://localhost:8080"
    echo "  - NL-Translator:   http://localhost:8081"
    echo "  - AI-Worker:       http://localhost:8083"
    echo ""
    echo "日志文件:"
    echo "  - 查看各模块日志: tail -f /tmp/{module-name}.log"
    exit 0
else
    echo -e "\n${RED}❌ 部分测试失败${NC}"
    echo ""
    echo "故障排查:"
    echo "  1. 确认Docker容器运行: docker ps"
    echo "  2. 确认各模块已启动"
    echo "  3. 查看模块日志: tail -100 /tmp/{module-name}.log"
    echo ""
    echo "日志文件位置:"
    echo "  - AI-Context:      /tmp/ai-context.log"
    echo "  - AI-Orchestrator: /tmp/ai-orchestrator.log"
    echo "  - NL-Translator:   /tmp/nl-translator.log"
    echo "  - AI-Worker:       /tmp/ai-worker.log"
    exit 1
fi
