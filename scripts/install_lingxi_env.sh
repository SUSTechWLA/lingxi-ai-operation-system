#!/bin/bash
# 灵犀AI OS - 一键安装脚本 (Mac/Linux)
# 检查并安装必要的依赖，配置环境变量，构建项目

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

section "灵犀AI OS - 一键安装"

# ============ 步骤 1: 检查 Java ============
section "步骤 1: 检查 Java 17+"

if command -v java &> /dev/null; then
    JAVA_VERSION=$(java -version 2>&1 | head -1 | cut -d'"' -f2 | cut -d'.' -f1)
    if [ "$JAVA_VERSION" -ge 17 ]; then
        info "Java 版本满足要求: $(java -version 2>&1 | head -1)"
    else
        error "Java 版本过低 (当前: $JAVA_VERSION)，需要 Java 17+"
        echo "安装方式:"
        echo "  macOS:   brew install openjdk@17"
        echo "  Linux:   sudo apt install openjdk-17-jdk"
        echo "  手动:    https://adoptium.net/"
        exit 1
    fi
else
    error "Java 未安装，需要 Java 17+"
    echo "安装方式:"
    echo "  macOS:   brew install openjdk@17"
    echo "  Linux:   sudo apt install openjdk-17-jdk"
    echo "  手动:    https://adoptium.net/"
    exit 1
fi

# ============ 步骤 2: 检查 Maven ============
section "步骤 2: 检查 Maven"

if command -v mvn &> /dev/null; then
    MVN_VERSION=$(mvn -version 2>&1 | head -1 | cut -d' ' -f3)
    info "Maven 已安装: $MVN_VERSION"
else
    error "Maven 未安装"
    echo "安装方式:"
    echo "  macOS:   brew install maven"
    echo "  Linux:   sudo apt install maven"
    echo "  手动:    https://maven.apache.org/download.cgi"
    exit 1
fi

# ============ 步骤 3: 检查 Docker ============
section "步骤 3: 检查 Docker"

if command -v docker &> /dev/null; then
    if docker info &> /dev/null; then
        DOCKER_VERSION=$(docker --version | cut -d' ' -f3 | tr -d ',')
        info "Docker 已安装并运行: $DOCKER_VERSION"
    else
        warn "Docker 已安装但未运行，请先启动 Docker Desktop"
    fi
else
    error "Docker 未安装"
    echo "安装方式:"
    echo "  macOS:   brew install --cask docker"
    echo "  Linux:   https://docs.docker.com/engine/install/"
    exit 1
fi

# ============ 步骤 4: 检查 Docker Compose ============
section "步骤 4: 检查 Docker Compose"

if docker compose version &> /dev/null; then
    COMPOSE_VERSION=$(docker compose version 2>&1 | cut -d' ' -f4 | tr -d 'v')
    info "Docker Compose 已安装: $COMPOSE_VERSION"
elif command -v docker-compose &> /dev/null; then
    warn "检测到旧版 docker-compose，建议升级到 Docker Compose V2"
else
    error "Docker Compose 未安装"
    echo "安装方式: Docker Compose V2 已包含在 Docker Desktop 中"
    exit 1
fi

# ============ 步骤 5: 检查 curl ============
section "步骤 5: 检查 curl"

if command -v curl &> /dev/null; then
    info "curl 已安装"
else
    error "curl 未安装"
    echo "安装方式:"
    echo "  macOS:   xcode-select --install"
    echo "  Linux:   sudo apt install curl"
    exit 1
fi

# ============ 步骤 6: 配置环境变量 ============
section "步骤 6: 配置环境变量"

if [ -f "$PROJECT_DIR/.env" ]; then
    info ".env 文件已存在"
else
    if [ -f "$PROJECT_DIR/.env.example" ]; then
        cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"
        info "已从 .env.example 复制创建 .env 文件"
    else
        warn ".env.example 不存在，创建默认 .env 文件"
        cat > "$PROJECT_DIR/.env" << 'ENVEOF'
# 灵犀AI OS 环境变量配置
OPENAI_API_KEY=your-api-key-here
OPENAI_BASE_URL=https://ark.cn-beijing.volces.com/api/coding/v3
OPENAI_MODEL=doubao-seed-2.0-pro
OPENAI_TEMPERATURE=0.7
OPENAI_MAX_TOKENS=2000
OPENAI_TIMEOUT=30000
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
POSTGRES_HOST=localhost
POSTGRES_DB=lingxi_db
POSTGRES_USER=wanglian
POSTGRES_PASSWORD=123
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
ORCHESTRATOR_URL=http://localhost:8080
CONTEXT_SERVICE_URL=http://localhost:8082
ENVEOF
    fi
    warn "请编辑 .env 文件，配置 OPENAI_API_KEY 等必要参数"
    echo "  编辑命令: vim $PROJECT_DIR/.env"
fi

# ============ 步骤 7: 启动 Docker 基础设施 ============
section "步骤 7: 启动 Docker 基础设施"

if ! docker info &> /dev/null; then
    warn "Docker 未运行，尝试启动..."
    open -a Docker 2>/dev/null || true
    echo -n "等待 Docker 启动"
    for i in {1..30}; do
        if docker info &> /dev/null; then
            echo -e " ${GREEN}✓${NC}"
            break
        fi
        echo -n "."
        sleep 2
    done
    if ! docker info &> /dev/null; then
        error "Docker 启动失败，请手动启动 Docker Desktop 后重试"
        exit 1
    fi
fi

info "启动基础设施容器..."
docker compose up -d

echo -n "等待 PostgreSQL 就绪"
for i in {1..30}; do
    if docker exec lingxi-postgres pg_isready -U wanglian &> /dev/null; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    if docker ps --format '{{.Names}}' | grep -q "^lingxi-postgres$"; then
        echo -e " ${GREEN}✓${NC}"
        break
    fi
    echo -n "."
    sleep 2
done

echo ""
echo "基础设施状态:"
for container in lingxi-postgres lingxi-redis lingxi-redpanda lingxi-minio lingxi-qdrant; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        echo -e "  ${GREEN}✓${NC} ${container}"
    else
        echo -e "  ${YELLOW}⚠${NC}  ${container} (未运行)"
    fi
done

# ============ 步骤 8: 构建所有模块 ============
section "步骤 8: 构建所有模块"

build_module() {
    local module=$1
    info "构建 $module..."
    cd "$PROJECT_DIR/$module"
    if mvn clean install -DskipTests -q 2>&1; then
        echo -e "  ${GREEN}✓${NC} $module 构建成功"
    else
        echo -e "  ${RED}✗${NC} $module 构建失败，请检查日志"
        return 1
    fi
}

BUILD_FAILED=0
build_module "ai-context" || BUILD_FAILED=1
build_module "ai-orchestrator" || BUILD_FAILED=1
build_module "ai-nl-translator" || BUILD_FAILED=1
build_module "ai-worker" || BUILD_FAILED=1

cd "$PROJECT_DIR"

if [ $BUILD_FAILED -ne 0 ]; then
    error "部分模块构建失败，请检查错误信息"
    echo "可以单独构建失败的模块:"
    echo "  cd <module-dir> && mvn clean install -DskipTests"
    exit 1
fi

# ============ 安装完成 ============
section "安装完成！"

echo ""
echo -e "${GREEN}✅ 环境安装和项目构建完成！${NC}"
echo ""
echo "后续步骤:"
echo "  1. 编辑 .env 文件，配置 OPENAI_API_KEY"
echo "     vim $PROJECT_DIR/.env"
echo ""
echo "  2. 启动所有服务"
echo "     ./scripts/startup.sh"
echo ""
echo "  3. 或手动启动各模块"
echo "     cd ai-context && mvn spring-boot:run"
echo "     cd ai-orchestrator && mvn spring-boot:run"
echo "     cd ai-nl-translator && mvn spring-boot:run"
echo "     cd ai-worker && mvn spring-boot:run"
echo ""
echo "  4. 测试 API"
echo "     ./scripts/test-all-apis.sh"
