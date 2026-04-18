# 🚀 灵犀AIOS - 完整部署与使用指南

## 概述

灵犀AIOS已实现完整功能（阶段一、二、三），支持：
- **阶段一**：单节点LLM任务，端到端流程
- **阶段二**：多节点串行任务、失败重试、快照管理、数据库工具
- **阶段三**：并行节点调度、工具超时控制、执行隔离、文件工具、全链路追踪

## 架构概览

```
用户入口 → NL-Translator(8081) → Orchestrator(8080) → AI-Worker(8083)
                                      ↓                          ↓
                              事件驱动调度(Redpanda)       LLM/数据库工具
                                      ↓                          ↓
                              AI-Context(8082) ←──────────────┘
                              (上下文记录/快照管理)
```

## 模块说明

### 1. ai-orchestrator (8080) - 任务编排中心
- 职责：DAG解析、依赖管理、状态流转、任务调度
- 核心特性：
  - 支持多节点串行/并行任务调度
  - 失败暂停机制：节点失败时暂停任务，后续节点停止调度
  - 失败重试：支持自动/手动重试失败节点
  - 任务暂停/恢复API

### 2. ai-worker (8083) - 统一工具网关
- 职责：工具注册、发现与执行
- 已实现工具：
  - **LLM工具**：对接OpenAI API，支持文本生成
  - **数据库工具**：支持PostgreSQL SELECT查询
  - **文件工具**：支持本地文件读写（带安全路径限制）
- 核心功能：
  - 消费 `ai.node.ready` Topic事件
  - 执行对应工具
  - 发布 `ai.node.result` Topic事件
  - 自动保存执行前/后快照
  - **工具超时控制**：默认120秒超时，可配置
  - **执行隔离**：独立线程池，防止单个工具阻塞其他任务
  - **全链路追踪**：traceId贯穿整个执行链路

### 3. ai-context (8082) - 全链路可追溯中心
- 职责：事件存储、快照管理、历史查询
- 核心特性：事件消费者，监听所有任务/节点事件

### 4. ai-nl-translator (8081) - 自然语言转DAG
- 职责：将用户自然语言转换为可执行DAG

## 前置条件

1. **JDK 17+**
2. **PostgreSQL** (数据库: lingxi_db)
3. **Redpanda/Kafka** (端口: 9092)
4. **OpenAI API Key** (使用LLM工具时需要)

## 快速开始

### 1. 启动中间件

```bash
# 启动PostgreSQL
docker run -d --name lingxi-postgres \
  -e POSTGRES_DB=lingxi_db \
  -e POSTGRES_USER=wanglian \
  -e POSTGRES_PASSWORD=123 \
  -p 5432:5432 \
  postgres:15

# 启动Redpanda
docker run -d --name lingxi-redpanda \
  -p 9092:9092 \
  docker.redpandadata.com/redpanda:latest \
  redpanda start --overprovisioned --smp 1 --memory 1G --reserve-memory 0M --node-id 0 --check=false
```

### 2. 配置环境变量

```bash
export OPENAI_API_KEY=your-openai-api-key-here
```

### 3. 启动服务（按顺序）

```bash
# 1. 启动AI-Context
cd ai-context
mvn spring-boot:run

# 2. 启动Orchestrator
cd ai-orchestrator
mvn spring-boot:run

# 3. 启动AI-Worker
cd ai-worker
mvn spring-boot:run

# 4. 启动NL-Translator
cd ai-nl-translator
mvn spring-boot:run
```

## 验证流程

### 场景一：单节点LLM任务（阶段一）

#### 1. 创建任务并提交DAG

```bash
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}'

# 使用返回的taskId，提交DAG
curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "node-llm-001",
        "type": "LLM",
        "name": "write_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于人工智能未来发展的文章，300字以内",
            "system_prompt": "你是一个专业的科技文章作者"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": []
  }'
```

#### 2. 查询任务状态

```bash
curl http://localhost:8080/api/task/{taskId}
```

#### 3. 查询上下文记录

```bash
curl http://localhost:8082/api/task/{taskId}/context
```

---

### 场景二：两节点串行任务（阶段二）

创建"写文章 → 生成摘要"串行任务：

```bash
# 1. 创建任务
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-002"}'

# 2. 提交包含两个节点的DAG
curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "write-article",
        "type": "LLM",
        "name": "write_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于人工智能未来发展的文章，500字左右",
            "system_prompt": "你是一个专业的科技文章作者"
          }
        },
        "maxRetry": 3
      },
      {
        "id": "summarize",
        "type": "LLM",
        "name": "summarize_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "请为以下文章生成摘要：{{parent.write-article.text}}",
            "system_prompt": "你是一个专业的文章摘要专家"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": [
      {"from": "write-article", "to": "summarize"}
    ]
  }'
```

---

### 场景三：失败重试与暂停恢复（阶段二）

#### 手动重试失败节点

```bash
# 1. 查询任务暂停原因
curl http://localhost:8080/api/task/{taskId}/pause-reason

# 2. 手动重试失败节点
curl -X POST http://localhost:8080/api/node/{nodeId}/retry

# 3. 或暂停任务
curl -X POST http://localhost:8080/api/task/{taskId}/pause \
  -H "Content-Type: application/json" \
  -d '{"reason": "需要人工检查"}'

# 4. 恢复任务
curl -X POST http://localhost:8080/api/task/{taskId}/resume
```

---

### 场景四：使用数据库工具（阶段二）

```bash
# 创建数据库查询任务
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-003"}'

curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "query-data",
        "type": "TOOL",
        "name": "query_database",
        "input": {
          "tool": "database",
          "parameters": {
            "sql": "SELECT COUNT(*) as count FROM ai_task"
          }
        },
        "maxRetry": 1
      }
    ],
    "edges": []
  }'
```

---

### 场景五：并行节点任务（阶段三）

创建两个独立的并行节点任务（同时执行）：

```bash
# 1. 创建任务
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-004"}'

# 2. 提交包含两个并行节点的DAG（没有edges = 并行执行）
curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "write-ai-article",
        "type": "LLM",
        "name": "write_ai_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于AI的短文，200字以内",
            "system_prompt": "你是一个科普作家"
          }
        },
        "maxRetry": 2
      },
      {
        "id": "write-space-article",
        "type": "LLM",
        "name": "write_space_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于太空探索的短文，200字以内",
            "system_prompt": "你是一个科普作家"
          }
        },
        "maxRetry": 2
      }
    ],
    "edges": []
  }'

# 两个节点会被同时调度到Worker执行
```

---

### 场景六：使用文件工具（阶段三）

```bash
# 1. 写入文件
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-005"}'

curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "write-file",
        "type": "TOOL",
        "name": "write_file",
        "input": {
          "tool": "file",
          "parameters": {
            "operation": "write",
            "path": "test.txt",
            "content": "Hello, 灵犀AIOS!\n这是测试文件内容。"
          }
        },
        "maxRetry": 1
      }
    ],
    "edges": []
  }'

# 2. 读取文件
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-006"}'

curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "read-file",
        "type": "TOOL",
        "name": "read_file",
        "input": {
          "tool": "file",
          "parameters": {
            "operation": "read",
            "path": "test.txt"
          }
        },
        "maxRetry": 1
      }
    ],
    "edges": []
  }'

# 3. 列出文件
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-007"}'

curl -X POST http://localhost:8080/api/task/{taskId}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "list-files",
        "type": "TOOL",
        "name": "list_files",
        "input": {
          "tool": "file",
          "parameters": {
            "operation": "list",
            "path": ".",
            "recursive": false
          }
        },
        "maxRetry": 1
      }
    ],
    "edges": []
  }'
```

---

### 场景七：自定义超时配置（阶段三）

在ai-worker的application.yml中配置：

```yaml
worker:
  thread-pool:
    core-size: 10        # 核心线程数
    max-size: 50         # 最大线程数
    queue-capacity: 100  # 队列容量
  tool:
    timeout-seconds: 120  # 工具执行超时时间（秒）
```

超过超时时间的工具会自动失败，触发重试逻辑。

---

### 其他实用API

```bash
# 查看AI-Worker已注册工具
curl http://localhost:8083/api/tools

# 查看服务健康状态
curl http://localhost:8080/api/health
curl http://localhost:8083/api/health
curl http://localhost:8082/api/health

# 获取节点最新快照
curl http://localhost:8080/api/node/{nodeId}/snapshot/latest
```

## 失败控制机制说明

### 核心设计

当任务包含多个节点时：

1. **节点失败 → 任务状态变为 `PAUSED`
2. **后续节点暂停调度**（Scheduler检查任务状态，暂停时跳过
3. **失败节点可以重试（自动或手动）
4. **重试成功后 → 任务自动恢复，后续节点继续执行**
5. **超过最大重试次数 → 任务失败**

### 状态流转

```
任务状态: CREATED → RUNNING → (节点失败) → PAUSED → (重试成功) → RUNNING → SUCCESS
                                 ↓
                           (重试耗尽) → FAILED
```

## 事件Topic说明

| Topic | 生产者 | 消费者 | 说明 |
|-------|--------|--------|------|
| `ai.task.created` | Orchestrator | Context | 任务创建 |
| `ai.node.ready` | Orchestrator | Worker | 节点就绪待执行 |
| `ai.node.result` | Worker | Orchestrator | 节点执行结果 |
| `ai.context.events` | All | Context | 上下文事件 |

## 数据库表结构

系统使用以下核心表：
- `ai_task` - 任务表
- `ai_node` - 节点表
- `ai_node_dependency` - 节点依赖表
- `ai_context` - 上下文记录表

## 故障排查

### 查看服务健康状态

```bash
# Orchestrator
curl http://localhost:8080/api/health

# AI-Worker
curl http://localhost:8083/api/health

# AI-Context
curl http://localhost:8082/api/health
```

### 常见问题

1. **Redpanda连接失败**
   - 确认Redpanda容器正常运行：`docker ps`
   - 检查端口9092是否被占用

2. **OpenAI API调用失败**
   - 确认`OPENAI_API_KEY`环境变量已设置
   - 检查网络连接与API额度

3. **任务卡在RUNNING状态**
   - 查看AI-Worker日志确认是否收到事件
   - 检查Redpanda Topic是否有消息积压

4. **任务处于PAUSED状态**
   - 查看暂停原因：`GET /api/task/{taskId}/pause-reason`
   - 手动重试失败节点或恢复任务

5. **数据库工具查询失败**
   - 确认只使用SELECT查询（安全限制）
   - 检查数据库连接配置
