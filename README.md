# 灵犀AI原生操作系统 (AIOS)

> **核心理念**: 用户通过**自然语言**直接操作硬件、中间件和应用，**绕过复杂的 GUI 前端交互**，为下一代操作系统做准备。

---

## 📚 阅读指南

本文档面向**零基础读者**，帮助你快速理解整个系统。

### 什么是 AIOS？

**AIOS 是新一代自然语言驱动的操作系统**。

#### 传统操作系统的交互方式

```
用户 ──▶ 点击图标 ──▶ 打开应用 ──▶ 点击菜单 ──▶ 选择功能 ──▶ 填写表单 ──▶ 执行操作
         (复杂的 GUI 交互链路)
```

#### AIOS 的交互方式

```
用户 ──▶ 说一句话 ──▶ 系统自动执行
         (自然语言直达目标)
```

#### 对比示例

| 场景 | 传统 GUI 操作 | AIOS 自然语言操作 |
|------|--------------|------------------|
| 查询数据库 | 打开工具 → 连接服务器 → 选数据库 → 写SQL → 执行 → 查看结果 | "查询用户表中最近一周的注册数据" |
| 部署应用 | 打开终端 → SSH登录 → 拉取代码 → 编译 → 配置 → 启动 | "把最新版本部署到测试环境" |
| 分析日志 | 打开日志平台 → 选时间范围 → 输入过滤条件 → 导出 → 分析 | "分析过去一小时的所有错误日志" |
| 发送邮件 | 打开邮箱 → 新建 → 填写收件人 → 写主题 → 写正文 → 添加附件 → 发送 | "给张三发一封项目进度报告邮件" |

### 核心价值

- 🎯 **降低使用门槛**: 不需要学习复杂的 GUI 操作，会说话就会用
- ⚡ **提升效率**: 一句话完成原本需要多步操作的任务
- 🔧 **统一交互**: 硬件、中间件、应用都用同一种方式操作
- 🚀 **面向未来**: 为下一代自然语言操作系统奠定基础

---

## 🏗️ 系统架构图

### 当前已实现模块

``` mermaid
graph TD
    subgraph "用户入口层 (Entry Layer)"
        A[🌐 Web / CLI / SDK]
    end

    subgraph "自然语言翻译层 (NL-Translator)"
    direction TB
        B["NL-Translator (Port: 8081)"]
        B1["意图解析: '写文章并生成摘要'"] --> B2["生成任务图 (DAG)"]
    end

    subgraph "任务编排层 (AI-Orchestrator)"
    direction TB
        C["Orchestrator (Port: 8080)"]
        C1["Scheduler (调度)"]
        C2["StateMachine (状态机)"]
    end

    subgraph "上下文管理层 (AI-Context)"
        D["AI-Context (Port: 8082)"]
        D1["操作历史回溯"]
        D2["节点快照恢复"]
    end

    A -->|"自然语言请求"| B
    B -->|"REST API (JSON DAG)"| C
    C <-->|"状态同步 & 记录"| D
```

### 🔧 工具执行层 (AI-Worker) - 规划中

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          工具执行层 (AI-Worker)                              │
│                              端口: 8083 (规划)                              │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐            │  │
│  │  │  LLM    │  │  搜索   │  │  数据库  │  │  文件   │   ...       │  │
│  │  │  工具   │  │  工具   │  │   工具  │  │   工具  │            │  │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘            │  │
│  │       ↑            ↑            ↑            ↑                     │  │
│  │       │            │            │            │                       │  │
│  │       └────────────┴────────────┴────────────┘                       │  │
│  │                          │                                           │  │
│  │                    统一工具网关                                         │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  工具注册表：                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  工具名称        │ 类型     │ 描述              │ API 端点          │  │
│  │  ───────────────│──────────│──────────────────│──────────────────│  │
│  │  openai_llm     │ LLM      │ OpenAI GPT模型    │ /api/tools/llm   │  │
│  │  google_search   │ Search   │ 谷歌搜索          │ /api/tools/search │  │
│  │  postgres_query │ Database │ PostgreSQL查询    │ /api/tools/db     │  │
│  │  file_read      │ File     │ 读取文件          │ /api/tools/file   │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 🔗 模块间通信详解

``` mermaid
sequenceDiagram
    autonumber
    actor User as 用户
    participant NL as NL-Translator (8081)
    participant Orch as Orchestrator (8080)
    participant Red as Redpanda (Event Bus)
    participant Worker as Worker Service
    participant Context as AI-Context (8082)

    User->>NL: POST /api/translate (自然语言)
    NL->>NL: 调用 LLM 生成 DAG 结构
    NL->>Orch: POST /api/task/create (发送DAG)
    Orch->>Context: 记录 TaskCreated 初始上下文
    Orch->>Red: 发布 ai.task.created 事件
    
    Note over Orch, Red: Scheduler 开始调度
    Orch->>Red: 发布 ai.node.ready (Node 1)
    Red-->>Worker: 消费 Ready 事件
    Worker->>Worker: 执行任务 (LLM/Tool)
    Worker->>Red: 发布 ai.node.result
    Red-->>Orch: 监听结果
    Orch->>Context: 更新节点快照
    Orch->>Orch: 检查 DAG 是否完成
    Orch-->>User: 返回任务最终状态
```

### 📊 数据流向图

``` mermaid
graph LR
    Input[用户输入] --> NL[NL-Translator]
    NL --> GPT[(GPT-4)]
    GPT --> DAG[DAG 结构]
    
    subgraph "核心数据处理"
        DAG --> Orch[Orchestrator]
        Orch <--> DB[(PostgreSQL<br/>持久化存储)]
        Orch <--> Bus{Redpanda<br/>事件总线}
        Orch <--> Context[AI-Context<br/>历史快照]
    end
    
    Bus -->|"ai.node.*"| Worker[Worker 执行器]
    Worker --> Bus
```

---

## 🎯 模块职责

### NL-Translator (端口 8081)
**做什么**：把"人话"翻译成"机器能懂的任务图"

```java
// 就像翻译软件
输入: "写文章并生成摘要"
输出: DAG结构 {
  nodes: [
    {id: "1", type: "LLM", name: "write_article"},
    {id: "2", type: "LLM", name: "summarize"}
  ],
  edges: [
    {from: "1", to: "2"}  // 2依赖1的结果
  ]
}
```

### AI-Orchestrator (端口 8080)
**做什么**：任务的"操作系统内核"，负责任务调度和状态管理

- 接收 DAG
- 按依赖顺序调度执行
- 管理节点状态（CREATED → RUNNING → SUCCESS/FAILED）
- 处理并发和重试

### AI-Context (端口 8082)
**做什么**：任务的"黑匣子"，记录一切，支持回溯

- 记录每个操作
- 保存节点快照（用于故障恢复）
- 查询历史状态

### AI-Worker (规划中, 端口 8083)
**做什么**：真正的"工具人"，执行具体任务

- 连接外部 API（搜索、数据库、文件等）
- 执行 LLM 调用
- 返回执行结果

---

## 🚀 快速开始

### 一键启动所有服务

```bash
cd /Users/wanglian/Projects/lingxi-ai-operation-system
./scripts/startup.sh
```

### 手动启动（分步骤）

```bash
# 1. 启动基础设施
docker compose up -d

# 2. 启动上下文服务
cd ai-context && mvn spring-boot:run &

# 3. 启动编排服务
cd ai-orchestrator && mvn spring-boot:run &

# 4. 启动翻译服务
cd ai-nl-translator && mvn spring-boot:run &
```

### 测试你的第一个任务

```bash
# 通过 NL-Translator 创建任务
curl -X POST http://localhost:8081/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "写一篇关于AI的文章"}'

# 查询任务状态
curl http://localhost:8080/api/task/{返回的taskId}

# 查看执行上下文
curl http://localhost:8080/api/task/{taskId}/context
```

---

## 📂 项目结构

```
lingxi-ai-operation-system/
│
├── 🌐 用户接口层
├── nl-translator/                    # 自然语言 → DAG 翻译
│   └── 职责：解析用户意图，生成可执行的任务图
│
├── ⚙️ 核心编排层
├── ai-orchestrator/                 # 任务编排引擎
│   └── 职责：调度任务、管理状态、事件驱动
│
├── 📝 上下文层
├── ai-context/                     # 上下文管理
│   └── 职责：记录历史、支持回溯、快照恢复
│
├── 🔧 工具执行层 (规划中)
├── ai-worker/                      # 工具网关
│   └── 职责：执行具体工具、连接外部API
│
└── 🗄️ 基础设施
    ├── PostgreSQL 16                # 持久化存储
    ├── Redis 7.x                    # 缓存
    ├── Redpanda                     # 事件总线 (Kafka兼容)
    └── MinIO/Qdrant                 # 对象存储/向量检索
```

---

## 🔮 未来架构展望

``` mermaid
graph TB
    User((用户)) --> Gateway[API Gateway<br/>认证/限流/监控]
    
    subgraph "Service Mesh"
        Gateway --> NL[NL-Translator]
        Gateway --> Workflow[Workflow Engine]
        Gateway --> Agent[Agent Service]
    end
    
    NL & Workflow & Agent --> Core[Orchestrator 核心引擎]
    
    subgraph "Infrastructure"
        Core --- Bus((Redpanda Event Bus))
        Core --- DB[(PostgreSQL)]
        Core --- Cache[(Redis)]
    end
    
    Bus --- Worker1[Worker A]
    Bus --- Worker2[Worker B]
    Bus --- Worker3[Worker C]
    
    subgraph "External Resources"
        Worker1 --- LLM[OpenAI/Claude]
        Worker2 --- Web[Google Search]
        Worker3 --- Data[(External DB/File)]
    end
```

---

## 🎓 学习路径

### 第一阶段：跑起来
1. 运行 `./scripts/startup.sh`
2. 调用 API 创建任务
3. 观察任务执行过程

### 第二阶段：理解原理
1. 阅读 [ORCHESTRATOR_GUIDE.md](docs/guides/ORCHESTRATOR_GUIDE.md) - 理解任务编排
2. 阅读 [NL_TRANSLATOR_GUIDE.md](docs/guides/NL_TRANSLATOR_GUIDE.md) - 理解自然语言处理
3. 阅读 [AI-CONTEXT_GUIDE.md](docs/guides/AI-CONTEXT_GUIDE.md) - 理解上下文管理

### 第三阶段：二次开发
1. 添加新的节点类型
2. 实现自定义 Worker
3. 扩展 NL-Translator

---

## 📖 文档索引

| 文档 | 内容 | 难度 |
|------|------|------|
| [ORCHESTRATOR_GUIDE.md](docs/guides/ORCHESTRATOR_GUIDE.md) | 任务编排详解 | ⭐⭐ |
| [NL_TRANSLATOR_GUIDE.md](docs/guides/NL_TRANSLATOR_GUIDE.md) | 自然语言翻译详解 | ⭐⭐ |
| [AI-CONTEXT_GUIDE.md](docs/guides/AI-CONTEXT_GUIDE.md) | 上下文管理详解 | ⭐ |
| [WORKER_GUIDE.md](docs/guides/WORKER_GUIDE.md) | 工具执行层详解 | ⭐⭐⭐ |

---

## ❓ 常见问题

**Q: 为什么需要 NL-Translator？**
A: 让用户可以用自然语言描述任务，降低使用门槛。

**Q: 为什么需要 Orchestrator？**
A: 管理复杂任务中的节点依赖、状态流转、错误处理。

**Q: 为什么需要 AI-Context？**
A: 提供可追溯性，支持故障恢复和调试。

**Q: AI-Worker 是什么？**
A: 未来规划中的模块，负责真正执行工具调用（搜索、数据库等）。

---

## License

MIT License
