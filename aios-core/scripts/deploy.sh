#!/bin/bash
# ============================================================================
#  躺营 AI OS — 一键部署脚本
#  适用系统: Ubuntu 22.04/24.04, Debian 12
#
#  用法:
#    bash scripts/deploy.sh           # 一键安装部署
#    bash scripts/deploy.sh --update  # 更新（仅构建+重启）
#    bash scripts/deploy.sh --help    # 查看帮助
#
#  部署后访问: http://<服务器IP>/
# ============================================================================
set -euo pipefail

# ── 颜色 ──────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── 全局变量 ──────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
INSTALL_DIR="/opt/tangying"
LOG_FILE="/tmp/tangying-deploy-$(date +%Y%m%d-%H%M%S).log"
START_TIME=$(date +%s)

MODE="full"        # full | update
FORCE_INSTALL=false
DEPLOY_USER=""
SERVER_IP=""

# ── 输出函数 ──────────────────────────────────────────
info()    { echo -e "${GREEN}[✓]${NC} $1" | tee -a "$LOG_FILE"; }
warn()    { echo -e "${YELLOW}[!]${NC} $1" | tee -a "$LOG_FILE"; }
error()   { echo -e "${RED}[✗]${NC} $1" | tee -a "$LOG_FILE"; exit 1; }
section() { echo -e "\n${BLUE}${BOLD}══ $1 ══${NC}" | tee -a "$LOG_FILE"; }
step()    { echo -e "${CYAN}▶ ${1}${NC}" | tee -a "$LOG_FILE"; }
bare()    { echo -e "  $1" | tee -a "$LOG_FILE"; }

usage() {
    cat <<'EOF'
躺营 AI OS — 一键部署脚本

用法:
  sudo bash scripts/deploy.sh             一键安装部署（裸服务器从零开始）
  sudo bash scripts/deploy.sh --update    更新模式（仅构建+重启服务）

部署前准备:
  1. 将项目代码放到服务器上（git clone / scp / rsync）
  2. 准备 OPENAI_API_KEY（或其他兼容 API 的 Key）

部署后:
  访问 http://<服务器IP>/  即可使用

支持系统: Ubuntu 22.04 / 24.04, Debian 12
EOF
    exit 0
}

# ── 参数解析 ──────────────────────────────────────────
for arg in "$@"; do
    case "$arg" in
        --update)   MODE="update" ;;
        --force)    FORCE_INSTALL=true ;;
        --help|-h)  usage ;;
    esac
done

# ── 检查 root 权限 ────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    error "请用 sudo 运行此脚本: sudo bash scripts/deploy.sh"
fi

# 确定运行用户（实际登录用户，非 root）
if [ -n "${SUDO_USER:-}" ]; then
    DEPLOY_USER="$SUDO_USER"
else
    DEPLOY_USER="$(logname 2>/dev/null || echo 'root')"
fi

# 自动检测服务器 IP
detect_ip() {
    if command -v curl &>/dev/null; then
        SERVER_IP=$(curl -4sf ifconfig.me 2>/dev/null || true)
    fi
    if [ -z "$SERVER_IP" ]; then
        SERVER_IP=$(ip -4 addr show scope global 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || echo "YOUR_SERVER_IP")
    fi
}

# ── Step 0: 系统检测 ──────────────────────────────────
detect_os() {
    section "Step 0: 系统检测"

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_ID="$ID"
        OS_VERSION="$VERSION_ID"
        OS_NAME="$PRETTY_NAME"
    else
        error "无法识别操作系统"
    fi

    case "$OS_ID" in
        ubuntu)
            if [ "$OS_VERSION" != "22.04" ] && [ "$OS_VERSION" != "24.04" ]; then
                warn "Untested Ubuntu version: $OS_VERSION (建议 22.04 或 24.04)"
            fi
            ;;
        debian)
            if [ "$OS_VERSION" != "12" ]; then
                warn "Untested Debian version: $OS_VERSION (建议 Debian 12)"
            fi
            ;;
        *)
            error "不支持的操作系统: $OS_NAME (需要 Ubuntu 22.04/24.04 或 Debian 12)"
            ;;
    esac

    info "系统: $OS_NAME | 用户: $DEPLOY_USER | 模式: $MODE"
    detect_ip
    bare "项目目录: $PROJECT_DIR"
    bare "部署目录: $INSTALL_DIR"
    bare "服务器 IP: $SERVER_IP"
}

# ── 辅助: 检测命令是否存在 ────────────────────────────
has_cmd() { command -v "$1" &>/dev/null; }

# ── 辅助: 检测包是否已安装 ────────────────────────────
pkg_installed() {
    dpkg -l "$1" &>/dev/null && return 0 || return 1
}

# ── 辅助: 检查版本是否满足要求 ────────────────────────
version_ge() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

# ── Step 1: 系统依赖 ──────────────────────────────────
install_system_deps() {
    section "Step 1: 安装系统依赖"

    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq

    local pkgs="curl wget git build-essential ca-certificates gnupg lsb-release software-properties-common"
    local missing=""
    for pkg in $pkgs; do
        pkg_installed "$pkg" || missing="$missing $pkg"
    done

    if [ -n "$missing" ]; then
        step "安装: $missing"
        apt-get install -y -qq $missing
    fi
    info "系统依赖就绪"
}

# ── Step 2: Docker ─────────────────────────────────────
install_docker() {
    section "Step 2: 安装 Docker"

    if has_cmd docker && docker info &>/dev/null; then
        info "Docker 已安装: $(docker --version)"
        return 0
    fi

    step "安装 Docker Engine..."
    install -m 0755 -d /etc/apt/keyrings 2>/dev/null || true

    if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
        curl -fsSL https://download.docker.com/linux/$OS_ID/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        chmod a+r /etc/apt/keyrings/docker.gpg
    fi

    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$OS_ID $(lsb_release -cs) stable" \
        > /etc/apt/sources.list.d/docker.list

    apt-get update -qq
    apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    # 将部署用户加入 docker 组
    usermod -aG docker "$DEPLOY_USER" 2>/dev/null || true

    systemctl enable docker --now
    info "Docker 安装完成: $(docker --version)"
}

# ── Step 3: Go ─────────────────────────────────────────
install_go() {
    section "Step 3: 安装 Go"

    local GO_MIN="1.23.0"
    local GO_VER="1.23.4"
    local GO_TAR="go${GO_VER}.linux-amd64.tar.gz"
    local GO_URL="https://go.dev/dl/${GO_TAR}"

    if has_cmd go; then
        local cur_ver=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+\.[0-9]+')
        if version_ge "$cur_ver" "$GO_MIN"; then
            info "Go 已安装: $(go version)"
            return 0
        fi
        warn "Go 版本过低 ($cur_ver < $GO_MIN)，重新安装..."
    fi

    step "下载 Go $GO_VER..."
    curl -fsSL "$GO_URL" -o "/tmp/$GO_TAR"

    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/$GO_TAR"
    rm -f "/tmp/$GO_TAR"

    # 确保 Go 在 PATH 中
    for rc in /root/.bashrc "/home/$DEPLOY_USER/.bashrc" /etc/profile; do
        if [ -f "$rc" ] && ! grep -q '/usr/local/go/bin' "$rc" 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> "$rc"
        fi
    done
    export PATH=$PATH:/usr/local/go/bin

    info "Go 安装完成: $(go version)"
}

# ── Step 4: Node.js ────────────────────────────────────
install_nodejs() {
    section "Step 4: 安装 Node.js"

    local NODE_MIN="18.0.0"

    if has_cmd node; then
        local cur_ver=$(node --version | grep -oP '\K[0-9]+\.[0-9]+\.[0-9]+')
        if version_ge "$cur_ver" "$NODE_MIN"; then
            info "Node.js 已安装: $(node --version)"
            return 0
        fi
        warn "Node.js 版本过低 ($cur_ver < $NODE_MIN)，重新安装..."
    fi

    step "通过 NodeSource 安装 Node.js 20 LTS..."

    if [ ! -f /etc/apt/keyrings/nodesource.gpg ]; then
        curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
            | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
    fi

    local NODE_MAJOR=20
    echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list

    apt-get update -qq
    apt-get install -y -qq nodejs

    info "Node.js 安装完成: $(node --version)"
}

# ── Step 5: Nginx ──────────────────────────────────────
install_nginx() {
    section "Step 5: 安装 Nginx"

    if has_cmd nginx; then
        info "Nginx 已安装: $(nginx -v 2>&1)"
        return 0
    fi

    apt-get install -y -qq nginx
    systemctl enable nginx --now
    info "Nginx 安装完成"
}

# ── Step 6: Rust (可选) ────────────────────────────────
install_rust() {
    section "Step 6: 安装 Rust（沙箱编译需要）"

    if has_cmd cargo; then
        info "Rust 已安装: $(rustc --version)"
        return 0
    fi

    if [ "${SANDBOX_ENABLED:-false}" = "false" ] && [ "$FORCE_INSTALL" = false ]; then
        info "SANDBOX_ENABLED=false，跳过 Rust 安装（如需要可设置 SANDBOX_ENABLED=true 并重新运行）"
        return 0
    fi

    step "安装 Rust 工具链..."
    su - "$DEPLOY_USER" -c 'curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable'
    info "Rust 安装完成"
}

# ── Step 7: .env 配置 ──────────────────────────────────
setup_env() {
    section "Step 7: 配置环境变量"

    if [ -f "$PROJECT_DIR/.env" ] && grep -q 'OPENAI_API_KEY=your-api-key-here' "$PROJECT_DIR/.env"; then
        warn ".env 文件存在但 API Key 未配置"
        echo ""
        echo -e "  ${BOLD}请输入你的 OPENAI_API_KEY:${NC}"
        read -rp "  > " api_key
        if [ -n "$api_key" ]; then
            sed -i "s|OPENAI_API_KEY=.*|OPENAI_API_KEY=$api_key|" "$PROJECT_DIR/.env"
            info "API Key 已更新"
        fi
    elif [ ! -f "$PROJECT_DIR/.env" ]; then
        step "从 .env.example 创建 .env..."
        cp "$PROJECT_DIR/.env.example" "$PROJECT_DIR/.env"

        echo ""
        echo -e "  ${BOLD}请输入你的 OPENAI_API_KEY (必填):${NC}"
        read -rp "  > " api_key
        if [ -n "$api_key" ]; then
            sed -i "s|OPENAI_API_KEY=.*|OPENAI_API_KEY=$api_key|" "$PROJECT_DIR/.env"
        fi

        echo ""
        echo -e "  ${BOLD}请输入 OPENAI_BASE_URL (直接回车使用默认 OpenAI):${NC}"
        read -rp "  > " base_url
        if [ -n "$base_url" ]; then
            sed -i "s|OPENAI_BASE_URL=.*|OPENAI_BASE_URL=$base_url|" "$PROJECT_DIR/.env"
        fi

        echo ""
        echo -e "  ${BOLD}请输入 OPENAI_MODEL (直接回车使用默认 gpt-4):${NC}"
        read -rp "  > " model
        if [ -n "$model" ]; then
            sed -i "s|OPENAI_MODEL=.*|OPENAI_MODEL=$model|" "$PROJECT_DIR/.env"
        fi

        # 生成随机数据库密码
        local db_pass=$(openssl rand -base64 16 2>/dev/null || head -c 16 /dev/urandom | base64)
        sed -i "s|POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$db_pass|" "$PROJECT_DIR/.env"
        sed -i "s|MINIO_SECRET_KEY=.*|MINIO_SECRET_KEY=$db_pass|" "$PROJECT_DIR/.env"

        info ".env 配置完成"
    else
        info ".env 已存在，跳过配置"
    fi

    # 加载环境变量
    set -a
    source "$PROJECT_DIR/.env" 2>/dev/null || true
    set +a
}

# ── Step 8: Docker 基础设施 ────────────────────────────
start_infrastructure() {
    section "Step 8: 启动 Docker 基础设施"

    cd "$PROJECT_DIR"

    # 注入 .env 变量到 docker compose
    docker compose up -d

    step "等待容器就绪..."
    local timeout=60
    local elapsed=0
    local required="tangying-postgres tangying-redis tangying-redpanda tangying-minio tangying-qdrant"

    while [ $elapsed -lt $timeout ]; do
        local all_ready=true
        for c in $required; do
            if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${c}$"; then
                all_ready=false
                break
            fi
        done
        if $all_ready; then
            break
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    echo ""
    for c in $required; do
        if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${c}$"; then
            info "$c 运行中"
        else
            warn "$c 未运行，请检查 docker compose logs"
        fi
    done

    # 等待 PostgreSQL 完全就绪
    step "等待 PostgreSQL 准备接受连接..."
    for i in $(seq 1 15); do
        if docker exec tangying-postgres pg_isready -U postgres &>/dev/null; then
            info "PostgreSQL 就绪"
            break
        fi
        sleep 2
    done
}

# ── Step 9: 构建 Go 后端 ───────────────────────────────
build_backend() {
    section "Step 9: 构建 Go 后端"

    export PATH=$PATH:/usr/local/go/bin

    cd "$PROJECT_DIR"

    # 如果部署到了 /opt/tangying 但项目目录不在那里，创建软链接
    if [ "$PROJECT_DIR" != "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR" ]; then
        mkdir -p "$(dirname "$INSTALL_DIR")"
        ln -sf "$PROJECT_DIR" "$INSTALL_DIR"
        info "创建项目链接: $INSTALL_DIR → $PROJECT_DIR"
    fi

    mkdir -p build
    step "编译中..."
    go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
    info "后端构建完成: build/tangying-ai-os"
}

# ── Step 10: 构建前端 ──────────────────────────────────
build_frontend() {
    section "Step 10: 构建前端 (Production)"

    cd "$PROJECT_DIR/../frontend"

    step "安装依赖..."
    npm install --silent

    step "构建中 (Vite production build)..."
    npm run build

    info "前端构建完成: frontend/dist/"

    # 确保 nginx 可以读取
    chown -R "$DEPLOY_USER:$DEPLOY_USER" dist/ 2>/dev/null || true
    cd "$PROJECT_DIR"
}

# ── Step 11: 构建沙箱 (可选) ───────────────────────────
build_sandbox() {
    section "Step 11: 构建 Rust 沙箱"

    if [ "${SANDBOX_ENABLED:-false}" != "true" ]; then
        info "SANDBOX_ENABLED 不为 true，跳过沙箱构建"
        return 0
    fi

    if ! has_cmd cargo; then
        warn "Rust 未安装，跳过沙箱构建"
        return 0
    fi

    cd "$PROJECT_DIR"

    # 加载 cargo 环境
    local cargo_home="/home/$DEPLOY_USER/.cargo"
    [ -f "$cargo_home/env" ] && source "$cargo_home/env"

    if [ -f sandbox/Cargo.toml ]; then
        step "编译 Rust 沙箱..."
        (cd sandbox && cargo build --release 2>&1 | tail -5)
        if [ -f sandbox/target/release/tangying-sandbox ]; then
            cp sandbox/target/release/tangying-sandbox build/
            info "沙箱构建完成: build/tangying-sandbox"
        fi
    fi
    cd "$PROJECT_DIR"
}

# ── Step 12: 配置 Nginx ────────────────────────────────
configure_nginx() {
    section "Step 12: 配置 Nginx"

    local NGINX_CONF="/etc/nginx/sites-available/tangying"
    local FRONTEND_DIST="$PROJECT_DIR/../frontend/dist"

    if [ ! -d "$FRONTEND_DIST" ]; then
        error "前端构建目录不存在: $FRONTEND_DIST"
    fi

    # 生成 Nginx 配置
    if [ -f "$SCRIPT_DIR/nginx-tangying.conf" ]; then
        cp "$SCRIPT_DIR/nginx-tangying.conf" "$NGINX_CONF"
        sed -i "s|__FRONTEND_DIST__|$FRONTEND_DIST|g" "$NGINX_CONF"
    else
        # Inline fallback
        cat > "$NGINX_CONF" << NGINX_EOF
server {
    listen 80;
    server_name _;
    root $FRONTEND_DIST;
    index index.html;

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 300s;
        client_max_body_size 500M;
    }

    location / {
        try_files \$uri \$uri/ /index.html;
    }
}
NGINX_EOF
    fi

    # 启用站点
    rm -f /etc/nginx/sites-enabled/default
    ln -sf "$NGINX_CONF" /etc/nginx/sites-enabled/tangying

    # 验证配置
    if nginx -t 2>/dev/null; then
        systemctl reload nginx
        info "Nginx 配置完成，已重载"
    else
        error "Nginx 配置验证失败，请检查 $NGINX_CONF"
    fi
}

# ── Step 13: systemd 服务 ──────────────────────────────
install_services() {
    section "Step 13: 安装 systemd 服务"

    # ── 后端服务 ──
    local back_src="$SCRIPT_DIR/tangying-backend.service"
    local back_dst="/etc/systemd/system/tangying-backend.service"

    if [ -f "$back_src" ]; then
        cp "$back_src" "$back_dst"
    else
        cat > "$back_dst" << SYSTEMD_EOF
[Unit]
Description=Tangying AI OS Backend
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=$DEPLOY_USER
WorkingDirectory=$PROJECT_DIR
EnvironmentFile=$PROJECT_DIR/.env
ExecStart=$PROJECT_DIR/build/tangying-ai-os
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SYSTEMD_EOF
    fi
    sed -i "s|__USER__|$DEPLOY_USER|g" "$back_dst"
    sed -i "s|__PROJECT_DIR__|$PROJECT_DIR|g" "$back_dst"

    systemctl daemon-reload
    systemctl enable tangying-backend
    systemctl restart tangying-backend

    info "后端服务已安装: systemctl status tangying-backend"

    # ── 沙箱服务 (可选) ──
    if [ "${SANDBOX_ENABLED:-false}" = "true" ] && [ -f "$PROJECT_DIR/build/tangying-sandbox" ]; then
        local sand_src="$SCRIPT_DIR/tangying-sandbox.service"
        local sand_dst="/etc/systemd/system/tangying-sandbox.service"

        if [ -f "$sand_src" ]; then
            cp "$sand_src" "$sand_dst"
        else
            cat > "$sand_dst" << SYSTEMD_EOF
[Unit]
Description=Tangying AI OS Sandbox (Rust gRPC)
After=network.target tangying-backend.service

[Service]
Type=simple
User=$DEPLOY_USER
WorkingDirectory=$PROJECT_DIR
ExecStart=$PROJECT_DIR/build/tangying-sandbox
Restart=always
RestartSec=3
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SYSTEMD_EOF
        fi
        sed -i "s|__USER__|$DEPLOY_USER|g" "$sand_dst"
        sed -i "s|__PROJECT_DIR__|$PROJECT_DIR|g" "$sand_dst"

        systemctl daemon-reload
        systemctl enable tangying-sandbox
        systemctl restart tangying-sandbox
        info "沙箱服务已安装: systemctl status tangying-sandbox"
    else
        systemctl stop tangying-sandbox 2>/dev/null || true
        systemctl disable tangying-sandbox 2>/dev/null || true
    fi
}

# ── Step 14: 防火墙 ────────────────────────────────────
configure_firewall() {
    section "Step 14: 配置防火墙"

    if has_cmd ufw; then
        ufw allow 80/tcp comment 'tangying-nginx' 2>/dev/null || true
        ufw allow 443/tcp comment 'tangying-nginx-ssl' 2>/dev/null || true
        ufw allow 22/tcp comment 'ssh' 2>/dev/null || true

        if ufw status | grep -q 'Status: active'; then
            info "防火墙规则已更新"
        else
            warn "ufw 未启用，建议执行: sudo ufw enable"
        fi
    else
        warn "ufw 未安装，请手动配置防火墙开放 80 端口"
    fi
}

# ── Step 15: 验证部署 ──────────────────────────────────
verify_deployment() {
    section "Step 15: 验证部署"

    local failed=0
    local max_wait=30

    # 1. Docker 容器
    step "检查 Docker 容器..."
    local containers=$(docker ps --format '{{.Names}}' 2>/dev/null | grep -c tangying || true)
    if [ "$containers" -ge 5 ]; then
        info "所有 5 个 Docker 容器运行中"
    else
        warn "Docker 容器数量: $containers/5"
        failed=1
    fi

    # 2. 后端健康检查
    step "等待后端服务启动..."
    for i in $(seq 1 $max_wait); do
        if curl -sf http://localhost:8080/api/health >/dev/null 2>&1; then
            info "后端健康检查通过"
            break
        fi
        if [ $i -eq $max_wait ]; then
            warn "后端未响应 (可稍后检查: journalctl -u tangying-backend -f)"
            failed=1
        fi
        sleep 1
    done

    # 3. Nginx 代理
    step "检查 Nginx 代理..."
    if curl -sf http://localhost/api/health >/dev/null 2>&1; then
        info "Nginx 代理 /api/* → 后端: 正常"
    else
        warn "Nginx 代理未生效 (检查: nginx -t)"
        failed=1
    fi

    # 4. 前端静态文件
    step "检查前端..."
    if curl -sf http://localhost/ >/dev/null 2>&1; then
        info "前端页面可访问"
    else
        warn "前端页面不可达 (检查 Nginx 配置)"
        failed=1
    fi

    # 5. 磁盘空间
    step "磁盘空间..."
    local disk_usage
    disk_usage=$(df -h / | awk 'NR==2 {print $5}' | sed 's/%//')
    if [ "$disk_usage" -lt 90 ]; then
        info "磁盘使用率: ${disk_usage}%"
    else
        warn "磁盘空间不足: ${disk_usage}%"
    fi

    return $failed
}

# ── 部署完成 ───────────────────────────────────────────
print_summary() {
    local end_time=$(date +%s)
    local duration=$((end_time - START_TIME))
    local minutes=$((duration / 60))
    local seconds=$((duration % 60))

    section "部署完成!"
    echo ""
    echo -e "  ${GREEN}${BOLD}躺营 AI OS 已成功部署${NC}"
    echo ""
    echo -e "  ${BOLD}访问地址:${NC}  ${CYAN}http://${SERVER_IP}/${NC}"
    echo -e "  ${BOLD}部署耗时:${NC}  ${minutes}分${seconds}秒"
    echo -e "  ${BOLD}部署日志:${NC}  $LOG_FILE"
    echo ""
    echo -e "  ${BOLD}━━━ 常用管理命令 ━━━${NC}"
    echo ""
    echo -e "  ${CYAN}后端服务:${NC}"
    echo -e "    systemctl status tangying-backend  # 查看状态"
    echo -e "    systemctl restart tangying-backend # 重启"
    echo -e "    journalctl -u tangying-backend -f  # 查看日志"
    echo ""
    echo -e "  ${CYAN}Docker 基础设施:${NC}"
    echo -e "    cd $INSTALL_DIR && docker compose ps     # 查看容器"
    echo -e "    cd $INSTALL_DIR && docker compose logs -f # 查看日志"
    echo ""
    echo -e "  ${CYAN}更新部署:${NC}"
    echo -e "    cd $INSTALL_DIR && git pull"
    echo -e "    sudo bash scripts/deploy.sh --update"
    echo ""
    echo -e "  ${BOLD}⚠  首次使用前，请确保已正确配置 .env 中的 OPENAI_API_KEY${NC}"
    echo ""
}

# ── 主流程 ─────────────────────────────────────────────
main() {
    echo -e "${BLUE}${BOLD}"
    echo "  ╔══════════════════════════════════════╗"
    echo "  ║   躺营 AI OS — 一键部署脚本         ║"
    echo "  ║   Ubuntu 22.04/24.04 · Debian 12   ║"
    echo "  ╚══════════════════════════════════════╝"
    echo -e "${NC}"

    # ── Full mode (首次部署) ──
    if [ "$MODE" = "full" ]; then
        detect_os
        install_system_deps
        install_docker
        install_go
        install_nodejs
        install_nginx
        setup_env
        source "$PROJECT_DIR/.env" 2>/dev/null || true
        install_rust
        start_infrastructure
        build_backend
        build_frontend
        build_sandbox
        configure_nginx
        install_services
        configure_firewall
        verify_deployment || true
        print_summary

    # ── Update mode (更新部署) ──
    else
        detect_os
        source "$PROJECT_DIR/.env" 2>/dev/null || true
        start_infrastructure
        build_backend
        build_frontend
        build_sandbox
        install_services
        systemctl reload nginx
        verify_deployment || true
        print_summary
    fi
}

main "$@"
