#!/bin/bash
set -e

cd "$(dirname "$0")/../frontend"

echo "================================================"
echo "  AIOS 视频创作智能体平台 - Electron 打包脚本"
echo "================================================"
echo ""

# Step 1: Install dependencies (skip if node_modules exists)
if [ ! -d "node_modules" ]; then
  echo "[1/3] 安装依赖..."
  npm install
else
  echo "[1/3] 依赖已安装，跳过"
fi

# Step 2: Build frontend
echo "[2/3] 构建前端..."
npm run build

# Step 3: Build Electron
echo "[3/3] 打包 Electron 客户端..."
node node_modules/.bin/electron-builder --mac

echo ""
echo "================================================"
echo "  ✅ 打包完成！"
echo "  输出: release/AIOS 视频创作智能体平台-0.0.1-arm64.dmg"
echo "================================================"
