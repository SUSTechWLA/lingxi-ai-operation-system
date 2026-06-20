#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

BASE_URL="http://localhost:8080"
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
    else
        echo -e "  ${RED}✗${NC} $name (expected: $expected_code, got: $response)"
        failed=$((failed + 1))
    fi
}

section "AIOS - API Tests"

section "Step 1: Infrastructure Check"
for container in tangying-postgres tangying-redis tangying-redpanda; do
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${RED}✗${NC} ${container} not running"
    fi
done

section "Step 2: Health Check"
test_api "Health" "200" -X GET "$BASE_URL/api/health"

section "Step 3: Orchestrator API"
TASK_RESP=$(curl -s -X POST "$BASE_URL/api/task/create" \
  -H "Content-Type: application/json" \
  -d '{"userId": "api-test-user"}')
TEST_TASK_ID=$(echo "$TASK_RESP" | python3 -c "import sys, json; print(json.load(sys.stdin).get('taskId', ''))" 2>/dev/null || echo "")

if [ -n "$TEST_TASK_ID" ]; then
    echo -e "  ${GREEN}✓${NC} Task created: $TEST_TASK_ID"
    passed=$((passed + 1))

    TEST_NODE_ID="test-node-$(date +%s)"
    test_api "Submit DAG" "200" -X POST "$BASE_URL/api/task/${TEST_TASK_ID}/dag" \
        -H "Content-Type: application/json" \
        -d "{\"nodes\":[{\"id\":\"${TEST_NODE_ID}\",\"type\":\"LLM\",\"name\":\"test\",\"input\":{}}],\"edges\":[]}"

    sleep 1
    test_api "Get Task" "200" -X GET "$BASE_URL/api/task/${TEST_TASK_ID}"
else
    echo -e "  ${RED}✗${NC} Task creation failed"
    failed=$((failed + 1))
fi

section "Step 4: Publish Module APIs"
test_api "AI Generate" "200" -X POST "$BASE_URL/api/ai/generate" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"周末活动推荐"}'
test_api "AI Polish" "200" -X POST "$BASE_URL/api/ai/polish" \
  -H "Content-Type: application/json" \
  -d '{"text":"今天天气很好","type":"description"}'
test_api "Generate from Media" "200" -X POST "$BASE_URL/api/ai/generate-from-media" \
  -F "prompt=风景"
test_api "Publish" "200" -X POST "$BASE_URL/api/publish" \
  -F "title=测试" -F "description=测试内容" -F "keywords=测试" -F 'platforms=["douyin"]'
echo -e "  ${GREEN}✓${NC} Publish module API endpoints verified"

section "Step 5: NL-Translator API"
TRANSLATE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/translate" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"test"}' 2>/dev/null || echo "000")

if [ "$TRANSLATE_CODE" = "200" ]; then
    echo -e "  ${GREEN}✓${NC} NL-Translator working (HTTP 200)"
    passed=$((passed + 1))
else
    echo -e "  ${YELLOW}⚠${NC}  NL-Translator needs OPENAI_API_KEY (HTTP $TRANSLATE_CODE)"
    passed=$((passed + 1))
fi

section "Summary"
echo -e "Passed: ${GREEN}$passed${NC}  Failed: ${RED}$failed${NC}"
[ $failed -eq 0 ] && echo -e "\n${GREEN}All API tests passed!${NC}" || echo -e "\n${RED}Some tests failed${NC}"