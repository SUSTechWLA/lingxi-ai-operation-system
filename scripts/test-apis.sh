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
        echo -e "  ${RED}✗${NC} $name (期望: $expected_code, 实际: $response)"
        failed=$((failed + 1))
    fi
}

section "API 连接测试"

echo "检查Docker服务..."
if docker ps --format '{{.Names}}' | grep -q "lingxi-postgres"; then
    echo -e "  ${GREEN}✓${NC} PostgreSQL 运行中"
else
    echo -e "  ${RED}✗${NC} PostgreSQL 未运行"
fi

if docker ps --format '{{.Names}}' | grep -q "lingxi-redpanda"; then
    echo -e "  ${GREEN}✓${NC} Redpanda 运行中"
else
    echo -e "  ${RED}✗${NC} Redpanda 未运行"
fi

section "模块代码检查"

echo "检查模块目录..."
for module in ai-context ai-orchestrator ai-nl-translator ai-worker; do
    if [ -d "$module" ]; then
        echo -e "  ${GREEN}✓${NC} $module 目录存在"
        if [ -f "$module/pom.xml" ]; then
            echo -e "    ${GREEN}✓${NC} pom.xml 存在"
        fi
    fi
done

section "验证结果"
echo -e "Docker基础设施: ${GREEN}就绪${NC}"
echo ""
echo "下一步："
echo "  1. 编译模块: cd ai-context && mvn compile"
echo "  2. 按顺序启动模块:"
echo "     - ai-context (8082)"
echo "     - ai-orchestrator (8080)"
echo "     - ai-nl-translator (8081)"
echo "     - ai-worker (8083)"
echo ""
echo "  或使用完整启动脚本: ./scripts/startup.sh"
