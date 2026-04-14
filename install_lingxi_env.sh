#!/bin/bash
set -e  # 遇到错误立即退出

# ================= 防休眠（关键！合盖/熄屏继续安装） =================
caffeinate -s -i -d $$ &
CAFFEINATE_PID=$!
trap "kill $CAFFEINATE_PID 2>/dev/null" EXIT

# ================= 颜色输出函数 =================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ================= 0. 前置检查 =================
info "=========================================="
info "灵犀AI原生系统 - 开发环境一键安装"
info "=========================================="

# 检查是否在macOS上运行
if [[ "$OSTYPE" != "darwin"* ]]; then
    error "此脚本仅支持 macOS"
    exit 1
fi

# 确保 Apple Silicon Homebrew 路径在 PATH 中
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"

# 检查 Homebrew 是否已安装
if ! command -v brew &> /dev/null; then
    error "Homebrew 未安装，正在自动安装..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    # 再次确保 Homebrew 路径生效
    export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
fi
info "Homebrew 已安装，继续..."

# ================= 1. 基础系统工具 =================
info ""
info "=========================================="
info "步骤 1/8: 安装基础系统工具"
info "=========================================="

# 安装 Xcode Command Line Tools（如果未安装）
if ! xcode-select -p &> /dev/null; then
    info "安装 Xcode Command Line Tools..."
    xcode-select --install
    warn "请在弹出的窗口中点击“安装”，完成后重新运行此脚本"
    exit 0
else
    info "Xcode Command Line Tools 已安装"
fi

# 补充 Homebrew 基础工具
brew install git wget curl jq tree htop make pkg-config 2>/dev/null || true
info "基础工具安装完成"

# ================= 2. Python 环境 =================
info ""
info "=========================================="
info "步骤 2/8: 安装 Python 环境 (pyenv + Python 3.11)"
info "=========================================="

if ! command -v pyenv &> /dev/null; then
    info "安装 pyenv..."
    brew install pyenv
else
    info "pyenv 已安装"
fi

# 配置 shell（仅添加一次）
SHELL_CONFIG="$HOME/.zshrc"
if ! grep -q 'PYENV_ROOT' "$SHELL_CONFIG"; then
    info "配置 pyenv 环境变量..."
    echo '' >> "$SHELL_CONFIG"
    echo '# Pyenv Configuration' >> "$SHELL_CONFIG"
    echo 'export PYENV_ROOT="$HOME/.pyenv"' >> "$SHELL_CONFIG"
    echo 'export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PYENV_ROOT/bin:$PATH"' >> "$SHELL_CONFIG"
    echo 'eval "$(pyenv init -)"' >> "$SHELL_CONFIG"
else
    info "pyenv 环境变量已配置"
fi

# 重新加载环境变量（当前会话）
export PYENV_ROOT="$HOME/.pyenv"
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PYENV_ROOT/bin:$PATH"
eval "$(pyenv init -)"

# 安装 Python 3.11.9（如果未安装）
if ! pyenv versions | grep -q '3.11.9'; then
    info "安装 Python 3.11.9（这可能需要几分钟）..."
    pyenv install 3.11.9
else
    info "Python 3.11.9 已安装"
fi
pyenv global 3.11.9
info "Python 环境配置完成: $(python --version)"

# 安装 Python gRPC 工具
pip install --upgrade pip
pip install grpcio grpcio-tools protobuf
info "Python gRPC 工具安装完成"

# ================= 3. Java 环境（修正版） =================
info ""
info "=========================================="
info "步骤 3/8: 安装 Java 环境 (jenv + OpenJDK 21)"
info "=========================================="

if ! command -v jenv &> /dev/null; then
    info "安装 jenv..."
    brew install jenv
else
    info "jenv 已安装"
fi

# 配置 shell（仅添加一次）
if ! grep -q 'jenv init' "$SHELL_CONFIG"; then
    info "配置 jenv 环境变量..."
    echo '' >> "$SHELL_CONFIG"
    echo '# Jenv Configuration' >> "$SHELL_CONFIG"
    echo 'export PATH="$HOME/.jenv/bin:$PATH"' >> "$SHELL_CONFIG"
    echo 'eval "$(jenv init -)"' >> "$SHELL_CONFIG"
else
    info "jenv 环境变量已配置"
fi

# 重新加载环境变量（当前会话）
export PATH="$HOME/.jenv/bin:$PATH"
eval "$(jenv init -)"

# 安装 OpenJDK 21（适配 Apple Silicon 路径）
BREW_JAVA_PATH="/opt/homebrew/opt/openjdk@21"
JAVA_HOME="$BREW_JAVA_PATH/libexec/openjdk.jdk/Contents/Home"
if [[ ! -d "$BREW_JAVA_PATH" ]]; then
    info "安装 OpenJDK 21..."
    brew install openjdk@21
else
    info "OpenJDK 21 已安装"
fi

# 创建系统级 Java 软链接（让 /usr/libexec/java_home 能识别）
sudo ln -sfn "$BREW_JAVA_PATH/libexec/openjdk.jdk" "/Library/Java/JavaVirtualMachines/openjdk-21.jdk" 2>/dev/null || true

# 添加到 jenv 管理
if ! jenv versions | grep -q '21'; then
    info "将 Java 21 添加到 jenv..."
    jenv add "$JAVA_HOME"
fi
jenv global 21
info "Java 环境配置完成: $(java -version 2>&1 | head -n 1)"

# ================= 4. Java 构建工具 =================
info ""
info "=========================================="
info "步骤 4/8: 安装 Java 构建工具 (Maven + Gradle)"
info "=========================================="

brew install maven gradle 2>/dev/null || true
info "Maven 版本: $(mvn --version | head -n 1)"
info "Gradle 版本: $(gradle --version | grep 'Gradle ')"

# ================= 5. C++ 环境 =================
info ""
info "=========================================="
info "步骤 5/8: 安装 C++ 开发环境"
info "=========================================="

brew install cmake ninja openssl boost 2>/dev/null || true
info "C++ 工具安装完成"
info "Clang 版本: $(clang --version | head -n 1)"
info "CMake 版本: $(cmake --version | head -n 1)"

# ================= 6. Protobuf & gRPC =================
info ""
info "=========================================="
info "步骤 6/8: 安装 Protobuf & gRPC 工具链"
info "=========================================="

brew install protobuf grpc 2>/dev/null || true
info "Protobuf 版本: $(protoc --version)"

# ================= 7. Docker 中间件配置 =================
info ""
info "=========================================="
info "步骤 7/8: 配置 Docker 中间件环境"
info "=========================================="

# 检查 Docker Desktop 是否运行
if ! docker info &> /dev/null; then
    warn "Docker Desktop 未运行，请先启动 Docker Desktop，然后按任意键继续..."
    read -n 1 -s
    if ! docker info &> /dev/null; then
        error "Docker 仍未运行，跳过中间件配置，请稍后手动启动"
    else
        info "Docker 已启动，继续..."
    fi
fi

# 创建工作目录
PROJECT_DIR="$HOME/Projects/lingxi-ai-system"
DOCKER_DIR="$PROJECT_DIR/docker"
mkdir -p "$DOCKER_DIR"
info "工作目录: $PROJECT_DIR"

# 生成 docker-compose.yml
cat > "$DOCKER_DIR/docker-compose.yml" << 'DOCKERCOMPOSE'
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    container_name: lingxi-postgres
    environment:
      POSTGRES_USER: wanglian
      POSTGRES_PASSWORD: 123
      POSTGRES_DB: lingxi_db
    ports:
      - "5432:5432"

  redis:
    image: redis:7-alpine
    container_name: lingxi-redis
    ports:
      - "6379:6379"

  # ✅ Kafka 替代（更轻量 & Mac友好）
  redpanda:
    image: redpandadata/redpanda:latest
    container_name: lingxi-redpanda
    command:
      - redpanda start
      - --overprovisioned
      - --smp 1
      - --memory 512M
      - --reserve-memory 0M
    ports:
      - "9092:9092"

  minio:
    image: minio/minio:latest
    container_name: lingxi-minio
    environment:
      MINIO_ROOT_USER: wanglian
      MINIO_ROOT_PASSWORD: 12345678
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"

  # ⭐ 向量数据库（替代 Milvus）
  qdrant:
    image: qdrant/qdrant:latest
    container_name: lingxi-qdrant
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - ./qdrant_storage:/qdrant/storage   # 👈 加这个
DOCKERCOMPOSE

info "Docker Compose 配置已生成: $DOCKER_DIR/docker-compose.yml"

# ================= 8. 完成提示 =================
info ""
info "=========================================="
info "安装完成！"
info "=========================================="
info ""
info "【重要】请执行以下命令以应用环境变量："
info "  source ~/.zshrc"
info ""
info "【下一步操作】"
info "1. 应用环境变量后，启动 Docker 中间件："
info "   cd $DOCKER_DIR"
info "   docker-compose up -d"
info ""
info "2. 验证环境："
info "   python --version"
info "   java -version"
info "   mvn --version"
info "   protoc --version"
info ""
info "3. 连接信息（已配置在 docker-compose.yml 中）："
info "   PostgreSQL: localhost:5432, 用户: lingxi, 密码: lingxi123456"
info "   Redis: localhost:6379"
info "   Kafka: localhost:9092"
info "   Milvus: localhost:19530"
info "   MinIO: localhost:9001, 用户: minioadmin, 密码: minioadmin"
info ""
info "项目目录已创建: $PROJECT_DIR"
info "=========================================="