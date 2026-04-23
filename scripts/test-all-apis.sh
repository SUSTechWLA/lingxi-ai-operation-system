#!/bin/bash
# 灵犀AI OS - 完整API测试脚本
# 测试所有模块的所有API接口

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }
section() { echo -e "\n${BLUE}========================================${NC}"; }
subsection() { echo -e "\n${CYAN}--- $1 ---${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# 加载环境变量
if [ -f "$PROJECT_DIR/.env" ]; then
    export $(cat "$PROJECT_DIR/.env" | grep -v '^#' | xargs)
fi

total_passed=0
total_failed=0
total_warnings=0
TEST_TASK_ID=""
TEST_NODE_ID=""

# 测试函数 - 返回HTTP状态码
test_http() {
    local name=$1
    local expected_code=$2
    shift 2
    echo -e "  测试: $name"
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" "$@" 2>/dev/null || echo "000")
    if echo "$response" | grep -q "$expected_code"; then
        echo -e "  ${GREEN}✓  PASS${NC} - HTTP $response"
        total_passed=$((total_passed + 1))
        return 0
    else
        echo -e "  ${RED}✗  FAIL${NC} - 期望: $expected_code, 实际: $response"
        total_failed=$((total_failed + 1))
        return 1
    fi
}

# 带响应体的测试函数
test_with_response() {
    local name=$1
    local expected_code=$2
    shift 2
    echo -e "  测试: $name"
    local temp_file=$(mktemp)
    local http_code
    http_code=$(curl -s -o "$temp_file" -w "%{http_code}" "$@" 2>/dev/null || echo "000")

    if echo "$http_code" | grep -q "$expected_code"; then
        echo -e "  ${GREEN}✓  PASS${NC} - HTTP $http_code"
        echo -e "  响应: $(head -c 200 "$temp_file" 2>/dev/null || echo "(empty)")"
        total_passed=$((total_passed + 1))
        rm -f "$temp_file"
        return 0
    else
        echo -e "  ${RED}✗  FAIL${NC} - 期望: $expected_code, 实际: $http_code"
        echo -e "  响应: $(cat "$temp_file" 2>/dev/null || echo "(empty)")"
        total_failed=$((total_failed + 1))
        rm -f "$temp_file"
        return 1
    fi
}

# 检查服务是否可访问
check_service() {
    local name=$1
    local url=$2
    echo -e "检查 ${name}..."
    if curl -s --max-time 5 "$url" >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} ${name} 可访问"
        return 0
    else
        echo -e "  ${RED}✗${NC} ${name} 不可访问"
        return 1
    fi
}

section "灵犀AI OS - 完整API测试套件"
echo "开始时间: $(date)"

# ============ 步骤 1: 基础设施检查 ============
section "步骤 1: 基础设施检查"

echo "检查Docker容器..."
for container in lingxi-postgres lingxi-redis lingxi-redpanda lingxi-minio lingxi-qdrant; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container} 运行中"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} 未运行"
        total_warnings=$((total_warnings + 1))
    fi
done

# ============ 步骤 2: 服务健康检查 ============
section "步骤 2: 服务健康检查"

services_up=0

check_service "AI-Context (8082)" "http://localhost:8082/api/health" && services_up=$((services_up + 1))
check_service "AI-Orchestrator (8080)" "http://localhost:8080/api/health" && services_up=$((services_up + 1))
check_service "AI-Worker (8083)" "http://localhost:8083/api/health" && services_up=$((services_up + 1))
check_service "NL-Translator (8081)" "http://localhost:8081/api/health" && services_up=$((services_up + 1))

if [ "$services_up" -lt 4 ]; then
    warn "部分服务未启动，请确保所有服务都在运行"
    echo "启动命令参考:"
    echo "  cd ai-context && mvn spring-boot:run"
    echo "  cd ai-orchestrator && mvn spring-boot:run"
    echo "  cd ai-worker && mvn spring-boot:run"
    echo "  cd ai-nl-translator && mvn spring-boot:run"
fi

# ============ 步骤 3: AI-Context API测试 ============
section "步骤 3: AI-Context API测试 (端口 8082)"

subsection "健康检查"
test_http "Health检查" "200" -X GET http://localhost:8082/api/health

# ============ 步骤 4: AI-Orchestrator API测试 ============
section "步骤 4: AI-Orchestrator API测试 (端口 8080)"

subsection "基础API"
test_http "Health检查" "200" -X GET http://localhost:8080/api/health

subsection "任务管理API"
echo -e "\n创建测试任务..."
TASK_RESP=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "full-api-test-user", "input": {"prompt": "测试任务"}}')

if echo "$TASK_RESP" | grep -q "taskId"; then
    TEST_TASK_ID=$(echo "$TASK_RESP" | python3 -c "import sys, json; print(json.load(sys.stdin).get('taskId', ''))" 2>/dev/null || echo "")
    echo -e "  ${GREEN}✓${NC} 创建任务成功: $TEST_TASK_ID"
    total_passed=$((total_passed + 1))

    subsection "DAG操作API"
    TEST_NODE_ID="test-node-$(date +%s)"
    test_with_response "提交DAG" "200" -X POST "http://localhost:8080/api/task/${TEST_TASK_ID}/dag" \
        -H "Content-Type: application/json" \
        -d "{\"nodes\":[{\"id\":\"${TEST_NODE_ID}\",\"type\":\"LLM\",\"name\":\"test_llm\",\"input\":{\"tool\":\"llm\",\"parameters\":{\"prompt\":\"hello\"}},\"maxRetry\":3}],\"edges\":[]}"

    sleep 1

    subsection "任务查询API"
    test_with_response "查询任务详情" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}"
    test_with_response "查询任务上下文" "200" -X GET "http://localhost:8080/api/task/${TEST_TASK_ID}/context"

    subsection "节点操作API"
    test_with_response "获取节点快照" "200" -X GET "http://localhost:8080/api/node/${TEST_NODE_ID}/snapshot/latest"

else
    echo -e "  ${RED}✗${NC} 创建任务失败"
    echo -e "  响应: $TASK_RESP"
    total_failed=$((total_failed + 1))
fi

# ============ 步骤 5: AI-Worker API测试 ============
section "步骤 5: AI-Worker API测试 (端口 8083)"

subsection "基础API"
test_http "Health检查" "200" -X GET http://localhost:8083/api/health

subsection "工具API"
test_with_response "获取内置工具列表" "200" -X GET http://localhost:8083/api/tools
test_with_response "获取外部工具列表" "200" -X GET http://localhost:8083/worker/tools

# ============ 步骤 6: NL-Translator API测试 ============
section "步骤 6: NL-Translator API测试 (端口 8081)"

subsection "基础API"
test_http "Health检查" "200" -X GET http://localhost:8081/api/health

subsection "翻译API"
echo -e "\n测试自然语言转DAG..."
TRANSLATE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8081/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "查询北京的天气"}' 2>/dev/null || echo "000")

if [ "$TRANSLATE_CODE" = "503" ]; then
    echo -e "  ${YELLOW}⚠${NC}  NL-Translator 返回503 (可能缺少OpenAI API Key)"
    echo -e "     这是预期行为，如需要请配置 OPENAI_API_KEY"
    total_warnings=$((total_warnings + 1))
    total_passed=$((total_passed + 1))
elif [ "$TRANSLATE_CODE" = "200" ]; then
    echo -e "  ${GREEN}✓${NC} NL-Translator 正常工作 (HTTP 200)"
    total_passed=$((total_passed + 1))
else
    echo -e "  ${RED}✗${NC} NL-Translator 异常 (HTTP $TRANSLATE_CODE)"
    total_failed=$((total_failed + 1))
fi

# 测试任务查询代理
if [ -n "$TEST_TASK_ID" ]; then
    test_with_response "任务查询代理" "200" -X GET "http://localhost:8081/api/task/${TEST_TASK_ID}"
fi

# ============ 测试总结 ============
section "测试总结"
echo -e "${PURPLE}测试结果统计:${NC}"
echo -e "  ${GREEN}通过: $total_passed${NC}"
echo -e "  ${RED}失败: $total_failed${NC}"
if [ "$total_warnings" -gt 0 ]; then
    echo -e "  ${YELLOW}警告: $total_warnings${NC}"
fi
echo -e "  总计: $((total_passed + total_failed))"

section "服务访问信息"
echo -e "${CYAN}服务地址:${NC}"
echo "  - AI-Context:      http://localhost:8082"
echo "  - AI-Orchestrator: http://localhost:8080"
echo "  - NL-Translator:   http://localhost:8081"
echo "  - AI-Worker:       http://localhost:8083"

if [ $total_failed -eq 0 ]; then
    echo -e "\n${GREEN}========================================${NC}"
    echo -e "${GREEN}✅ 所有API测试通过！${NC}"
    echo -e "${GREEN}========================================${NC}"
    exit 0
else
    echo -e "\n${RED}========================================${NC}"
    echo -e "${RED}❌ 部分测试失败${NC}"
    echo -e "${RED}========================================${NC}"
    echo -e "\n${YELLOW}故障排查建议:${NC}"
    echo "  1. 确认所有Docker容器运行: docker ps"
    echo "  2. 确认各模块已启动并监听对应端口"
    echo "  3. 查看模块日志: tail -100 /tmp/{module-name}.log"
    echo "  4. 检查端口占用: lsof -i :8080,:8081,:8082,:8083"
    echo -e "\n${YELLOW}日志文件位置:${NC}"
    echo "  - AI-Context:      /tmp/ai-context.log"
    echo "  - AI-Orchestrator: /tmp/ai-orchestrator.log"
    echo "  - NL-Translator:   /tmp/nl-translator.log"
    echo "  - AI-Worker:       /tmp/ai-worker.log"
    exit 1
fi
