# 灵犀AI OS - 脚本使用指南

本目录包含灵犀AI OS的所有辅助脚本。

## 📁 脚本说明

### 1. `install_lingxi_env.sh` (Mac/Linux)
**用途**: 首次安装时配置开发环境

**功能**:
- 检查并安装必要的依赖（JDK 17+, Maven, Docker等）
- 配置环境变量
- 创建必要的目录

**使用方法**:
```bash
./scripts/install_lingxi_env.sh
```

---

### 2. `install_lingxi_env_win.sh` (Windows)
**用途**: Windows系统的环境安装脚本

**功能**:
- 检查Windows环境依赖
- 配置Windows环境变量

**使用方法**:
```cmd
scripts\install_lingxi_env_win.sh
```

---

### 3. `startup.sh` ⭐
**用途**: 一键启动所有服务（推荐日常使用）

**功能**:
- 自动检查并启动Docker基础设施
- 按顺序启动所有模块：
  1. AI-Context (8082)
  2. AI-Orchestrator (8080)
  3. AI-Worker (8083)
  4. NL-Translator (8081)
- 等待所有服务就绪
- 自动运行API测试
- 按 `Ctrl+C` 可停止所有服务

**使用方法**:
```bash
# 从项目根目录执行
./scripts/startup.sh
```

**日志文件**:
- `/tmp/ai-context.log`
- `/tmp/ai-orchestrator.log`
- `/tmp/ai-worker.log`
- `/tmp/nl-translator.log`

---

### 4. `test-apis.sh`
**用途**: 测试所有API接口

**功能**:
- 检查基础设施状态
- 测试各模块的健康检查接口
- 创建测试任务并验证流程
- 测试工具注册与发现

**使用方法**:
```bash
# 确保服务已启动后执行
./scripts/test-apis.sh
```

**测试内容**:
1. Docker基础设施检查
2. AI-Context 模块测试
3. AI-Orchestrator 模块测试（创建任务、提交DAG等）
4. AI-Worker 模块测试（工具列表）
5. NL-Translator 模块测试

---

## 🚀 快速开始

### 首次使用
```bash
# 1. 安装环境
./scripts/install_lingxi_env.sh

# 2. 启动所有服务
./scripts/startup.sh
```

### 日常使用
```bash
# 直接启动（环境已配置好）
./scripts/startup.sh
```

### 单独测试
```bash
# 1. 先启动服务
./scripts/startup.sh

# 2. 新开终端，运行API测试
./scripts/test-apis.sh
```

---

## 🔧 故障排查

### 端口被占用
```bash
# 查看端口占用
lsof -ti :8080  # 检查8080端口

# 手动停止服务
for port in 8080 8081 8082 8083; do
  pid=$(lsof -ti :$port 2>/dev/null)
  [ -n "$pid" ] && kill -9 $pid
done
```

### Docker问题
```bash
# 查看容器状态
docker ps

# 重启基础设施
docker compose down
docker compose up -d
```

### 查看日志
```bash
# 实时查看某个服务的日志
tail -f /tmp/ai-orchestrator.log

# 查看最后100行
tail -100 /tmp/ai-worker.log
```

---

## 📋 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| AI-Context | 8082 | 上下文管理 |
| AI-Orchestrator | 8080 | 任务编排 |
| AI-Worker | 8083 | 工具执行 |
| NL-Translator | 8081 | 自然语言翻译 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |
| Redpanda | 9092 | 事件总线 |
| MinIO | 9000/9001 | 对象存储 |
| Qdrant | 6333/6334 | 向量数据库 |

---

## 📚 相关文档

- 完整API参考: `../docs/API_REFERENCE.md`
- 架构指南: `../docs/guides/`
- 部署指南: `../docs/guides/MVP_SETUP_GUIDE.md`
