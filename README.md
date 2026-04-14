# 灵犀AI原生操作系统 (AIOS)

> **核心定位**: Orchestrator 是灵犀AI原生OS的绝对核心，相当于现代操作系统的CPU + 内核调度器 + 进程管理器。

## 项目概览

灵犀AI原生OS是一个多语言、模块化的AI操作系统，基于深度架构设计文档实现。

### 核心特性

- ✨ **7层深度架构**: 网关层 → 规划层 → 调度层 → 执行层 → 状态层 → 事件层 → 资源层
- 🧠 **自然语言驱动**: NLU自动理解用户意图，生成任务DAG
- 🔍 **全链路可观测**: 结构化日志 + 分布式追踪 + 指标收集
- 📜 **可追溯性**: 事件溯源、状态历史、日志回溯
- 🌐 **多语言支持**: Java/Python/C++ 各有所长
- ⚡ **内核级调度**: 多级反馈队列调度算法

## 快速开始

### 前置要求

- Java 17+
- Maven 3.9+
- Docker 和 Docker Compose

### 启动 Orchestrator 模块

Orchestrator 模块是一个基于 Redpanda 事件驱动的分布式 AI 编排内核。

```bash
# 进入 Orchestrator 目录
cd ai-orchestrator

# 启动依赖服务（Redis + Redpanda）
docker-compose up -d redis redpanda

# 编译项目
mvn clean compile

# 启动应用
mvn spring-boot:run
```

### 测试 API

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task \
  -H "Content-Type: application/json" \
  -d '{"prompt":"写文章并生成摘要"}' | python3 -c "import sys, json; print(json.load(sys.stdin)['taskId'])")

echo "创建任务成功，Task ID: $TASK_ID"

# 2. 等待 8 秒让任务执行
sleep 8

# 3. 查询任务状态
curl -s http://localhost:8080/api/task/$TASK_ID
```

### 详细文档

想了解更多？请查看完整的 Orchestrator 模块指南：

📖 **[Orchestrator 模块完整指南](./ORCHESTRATOR_GUIDE.md)**

这份指南包含：
- 什么是 Orchestrator？
- 核心功能详解
- 所有类的定义和说明
- 完整的工作流程
- 快速上手教程

## 项目结构

```
lingxi-ai-operation-system/
├── README.md                           # 本文档
├── ORCHESTRATOR_GUIDE.md              # Orchestrator 模块完整指南
├── create_orchestrator.md             # Orchestrator SDD 设计文档
├── docker-compose.yml                  # 根目录 Docker 配置
├── ai-orchestrator/                    # Orchestrator 模块 ✨
│   ├── pom.xml                         # Maven 配置
│   ├── docker-compose.yml              # Orchestrator 依赖配置
│   └── src/main/java/com/lingxi/ai/orchestrator/
│       ├── OrchestratorApplication.java
│       ├── controller/                 # API 控制器
│       ├── service/                    # 业务服务
│       ├── model/                      # 数据模型
│       ├── event/                      # 事件处理
│       ├── repository/                 # 数据访问
│       └── config/                     # 配置类
└── ...
```

## 技术栈

| 层级 | 技术选型 |
|------|----------|
| 开发语言 | Java 17+ |
| Web框架 | Spring Boot 3.2.5 |
| 状态存储 | Redis 7.x |
| 事件总线 | Redpanda (Kafka 兼容) |
| 构建工具 | Maven 3.9.x |

## 与现代操作系统的类比

| 现代操作系统 | 灵犀AI原生OS Orchestrator |
|--------------|---------------------------|
| CPU | 任务调度、指令执行 |
| 内核调度器 | 优先级调度、资源分配、时间片轮转 |
| 进程管理器 | 任务创建、状态管理、上下文切换 |
| 系统调用接口 | 统一API网关、请求接入 |
| 设备驱动 | 工具/服务调用适配器 |
| 文件系统 | 状态持久化、上下文存储 |

## License

MIT License
