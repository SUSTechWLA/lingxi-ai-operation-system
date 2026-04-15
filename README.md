# 灵犀AI原生操作系统 (AIOS)

> **核心定位**: Orchestrator 是灵犀AI原生OS的绝对核心，相当于现代操作系统的CPU + 内核调度器 + 进程管理器。

## 项目概览

灵犀AI原生OS是一个多语言、模块化的AI操作系统，基于深度架构设计文档实现。

### 当前实现模块架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              用户                                         │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │ 自然语言
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    NL Translator 模块（用户访问入口）                     │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ TranslateController                                               │  │
│  │   - POST /api/translate           (仅翻译，返回DAG)               │  │
│  │   - POST /api/translate-and-submit (翻译并提交)                   │  │
│  │   - GET /api/task/{taskId}       (查询任务状态)                   │  │
│  └──────────────┬────────────────────────────────────────────────────┘  │
│                 │                                                         │
│  ┌──────────────▼───────────────────┐  ┌───────────────────────────┐  │
│  │ NlToDagService                   │  │ OpenAiClientService       │  │
│  │   - 自然语言转 DAG               │  │   - OpenAI API 调用        │  │
│  └──────────────┬───────────────────┘  └───────────────────────────┘  │
└─────────────────┼─────────────────────────────────────────────────────────┘
                  │ 内部调用
                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│              AI Orchestrator 模块（内部服务）                            │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │ TaskController                                                   │  │
│  │   - POST /api/node  (接收DAG创建任务)                             │  │
│  │   - GET /api/task/{id} (查询任务状态)                              │  │
│  └──────────────┬────────────────────────────────────────────────────┘  │
│                 │                                                         │
│  ┌──────────────▼───────────────────┐  ┌───────────────────────────┐  │
│  │ OrchestratorService              │  │ StateMachineService       │  │
│  │   - 任务编排调度                  │  │   - 状态机管理              │  │
│  └──────────────┬───────────────────┘  └───────────────┬───────────┘  │
│                 │                                         │               │
│  ┌──────────────▼───────────────────┐  ┌───────────────▼───────────┐  │
│  │ EventProducer                    │  │ EventConsumer              │  │
│  │   - 事件发送                      │  │   - 事件消费                │  │
│  └──────────────┬───────────────────┘  └───────────────┬───────────┘  │
└─────────────────┼────────────────────────────────────────┼───────────────┘
                  │                                │
                  ▼                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                              基础设施                                    │
│  ┌───────────────────────┐  ┌───────────────────────────┐             │
│  │ Redis 7.x             │  │ Redpanda / Kafka          │             │
│  │   - 状态存储           │  │   - 事件总线               │             │
│  └───────────────────────┘  └───────────────────────────┘             │
└─────────────────────────────────────────────────────────────────────────┘
```

### 完整系统架构图（远景规划）
```mermaid
flowchart TB
    %% ================= 横切能力（全链路覆盖）=================
    X["全链路安全体系<br>认证 / 鉴权 / 加密 / 审计 / 合规"]
    Y["可观测体系<br>日志 / 指标 / Trace / 告警"]
    Z["配置中心 & 服务治理<br>配置管理 / 服务注册发现 / 限流 / 熔断 / 降级"]

    %% ================= 接入层 =================
    subgraph L1 ["接入层"]
        A1["CLI / SDK"]
        A2["Web / 前端"]
        A3["IoT设备"]
    end

    %% ================= 网关层 =================
    subgraph G1 ["API Gateway / BFF层"]
        G["统一网关<br>协议转换 / 路由 / 聚合 / 跨域"]
    end

    %% ================= 编排与控制层（全请求唯一入口）=================
    subgraph O1 ["编排与控制层（总控中枢）"]
        O["Orchestrator<br>请求分发 / 服务调度 / 流程编排"]
        NL["NL Translator<br>自然语言转 DAG<br>【当前已实现】"]
        WF["Workflow Engine<br>状态机 / 重试 / 回滚 / Saga事务<br>【编排层内置核心组件】"]
    end

    %% ================= Agent智能层 =================
    subgraph L2 ["Agent智能层（Python）"]
        B["Agent Service<br>LLM交互 / 意图理解 / 任务规划 / 工具匹配 / 记忆管理"]
    end

    %% ================= 业务服务层 =================
    subgraph L3 ["业务服务层（Java）"]
        C1["用户与权限服务<br>用户管理 / 凭证管理 / OAuth授权 / 权限控制"]
        C2["第三方平台服务<br>工具注册 / 统一调度 / 适配器管理"]
        subgraph C2M ["平台适配器（SPI热插拔）"]
            C21["抖音适配器"]
            C22["小红书适配器"]
            C23["淘宝适配器"]
            C24["扩展平台适配器"]
        end
        C3["审计与合规服务<br>审计日志 / 合规审核 / 报表生成"]
    end

    %% ================= 基础设施服务层 =================
    subgraph L4 ["基础设施服务层"]
        D1["向量检索服务（C++）<br>向量写入 / 相似性检索 / 索引管理"]
        D2["加密服务（C++）<br>密钥管理 / 证书服务 / 加解密SDK"]
        D3["实时计算服务（C++/Flink）<br>事件清洗 / 聚合 / 告警 / 流处理"]
    end

    %% ================= 数据与中间件层 =================
    subgraph L5 ["数据与中间件层"]
        E1[("PostgreSQL<br>业务数据 / 审计日志")]
        E2[("Milvus<br>向量数据库")]
        E3[("Redis<br>缓存 / 会话 / 限流 / 分布式锁")]
        E4[("Kafka<br>事件流中枢 / 消息队列")]
        E5[("MinIO<br>对象存储 / 加密凭证 / 大文件")]
    end

    %% ================= 核心调用链路（严格单向依赖）=================
    %% 接入 → 网关
    A1 & A2 & A3 --> G

    %% 网关 → 编排层（唯一入口）
    G --> O

    %% 编排层 → 核心能力调度
    O --> NL
    O --> B
    O --> C1
    O --> C2
    O --> WF

    %% Agent层 → 仅依赖编排层和基础设施，不直接调用业务服务
    B --> O
    B --> D1
    B --> E3

    %% 业务服务 → 平台适配器
    C2 --> C2M

    %% 业务服务 → 数据存储
    C1 --> E1
    C1 --> E3
    C2 --> E3
    C3 --> E1

    %% 基础设施服务 → 数据存储
    D1 --> E2
    D2 --> E5
    D3 --> E4

    %% ================= 事件驱动闭环（全异步解耦）=================
    O & B & C1 & C2 -->|发布事件| E4
    E4 -->|消费事件| D3
    D3 -->|标准化事件| E4
    E4 -->|消费审计事件| C3

    %% ================= 横切能力全链路覆盖 =================
    X --> G
    X --> O
    X --> B
    X --> C1
    X --> C2
    X --> D1
    X --> D2

    Y --> G
    Y --> O
    Y --> B
    Y --> C1
    Y --> C2
    Y --> D1
    Y --> D3

    Z --> G
    Z --> O
    Z --> B
    Z --> C1
    Z --> C2
    Z --> D1
    Z --> D2
```
### 核心特性

- ✨ **7层深度架构**: 网关层 → 规划层 → 调度层 → 执行层 → 状态层 → 事件层 → 资源层
- 🧠 **自然语言驱动**: NLU自动理解用户意图，生成任务DAG
- 🔍 **全链路可观测**: 结构化日志 + 分布式追踪 + 指标收集
- 📜 **可追溯性**: 事件溯源、状态历史、日志回溯
- 🌐 **多语言支持**: Java/Python/C++ 各有所长
- ⚡ **内核级调度**: 多级反馈队列调度算法

### 模块架构

```
lingxi-ai-operation-system/
├── ai-orchestrator/          # 任务编排层（核心调度）
│   └── 职责：接收 Node/DAG，执行编排调度
├── nl-translator/             # 自然语言翻译层
│   └── 职责：通过 LLM API 将自然语言转换为 Node/DAG
└── docs/                      # 文档
```

## 快速开始

### 前置要求

- Java 17+
- Maven 3.9+
- Docker 和 Docker Compose
- OpenAI API Key（用于 nl-translator）

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

### 启动 NL Translator 模块

NL Translator 模块负责将自然语言转换为 Node/DAG。

```bash
# 在新的终端中，进入 nl-translator 目录
cd nl-translator

# 设置 OpenAI API Key
export OPENAI_API_KEY=your-api-key-here

# 编译项目
mvn clean compile

# 启动应用（端口 8081）
mvn spring-boot:run
```

### 测试 API

用户只能通过 NL Translator 模块访问系统：

```bash
# 1. 翻译并提交任务（自然语言 → DAG → 执行）
TASK_ID=$(curl -s -X POST http://localhost:8081/api/translate-and-submit \
  -H "Content-Type: application/json" \
  -d '{"prompt":"写一篇关于AI的文章并生成摘要"}' | python3 -c "import sys, json; print(json.load(sys.stdin)['taskId'])")

echo "创建任务成功，Task ID: $TASK_ID"

# 2. 等待执行完成
sleep 5

# 3. 通过 nl-translator 查询任务状态
curl -s http://localhost:8081/api/task/$TASK_ID
```

#### 仅翻译不提交（可选）

如果只需要获取 DAG 而不立即执行：

```bash
# 仅翻译，返回 DAG 结构
curl -s -X POST http://localhost:8081/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt":"写一篇关于AI的文章并生成摘要"}'
```

> **重要说明**：
> - 用户不直接访问 Orchestrator 模块
> - Orchestrator 仅作为内部服务被 NL Translator 调用
> - 所有用户请求都通过 NL Translator 模块统一处理

### 详细文档

想了解更多？请查看完整的模块指南：

📖 **[Orchestrator 模块完整指南](./ORCHESTRATOR_GUIDE.md)**

这份指南包含：
- 什么是 Orchestrator？
- 核心功能详解
- 所有类的定义和说明
- 完整的工作流程
- 快速上手教程

📖 **[NL Translator 模块完整指南](./NL_TRANSLATOR_GUIDE.md)**

这份指南包含：
- 什么是 NL Translator？
- 如何配置 OpenAI API
- 自然语言到 DAG 的转换原理
- API 接口使用说明
- 快速上手教程

## 项目结构

```
lingxi-ai-operation-system/
├── README.md                           # 本文档
├── ORCHESTRATOR_GUIDE.md              # Orchestrator 模块完整指南
├── NL_TRANSLATOR_GUIDE.md            # NL Translator 模块指南
├── create_orchestrator.md             # Orchestrator SDD 设计文档
├── docker-compose.yml                  # 根目录 Docker 配置
├── ai-orchestrator/                    # 任务编排层 ✨
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
├── nl-translator/                       # 自然语言翻译层 ✨
│   ├── pom.xml                         # Maven 配置
│   └── src/main/java/com/lingxi/ai/translator/
│       ├── NlTranslatorApplication.java
│       ├── controller/                 # API 控制器
│       ├── service/                    # 业务服务
│       ├── model/                      # 数据模型
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
