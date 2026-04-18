# 灵犀AI OS Orchestrator 模块完整指南

> **状态**: ✅ 已实现 - 本文档描述当前已实现的模块
> **最后更新**: 2026-04-18

---

## 目录

1. [什么是 Orchestrator？](#什么是-orchestrator)
2. [核心功能](#核心功能)
3. [技术栈](#技术栈)
4. [项目结构](#项目结构)
5. [完整 API 接口](#完整-api-接口)
6. [类定义详解](#类定义详解)
7. [工作流程](#工作流程)
8. [快速开始](#快速开始)

---

## 什么是 Orchestrator？

**Orchestrator（编排器）** 是灵犀AI OS的核心调度模块，负责：

- 接收 NL-Translator 转换的 DAG 任务请求
- 按依赖关系调度执行节点
- 管理任务和节点的生命周期状态
- 通过事件驱动与 Worker 通信
- 通过 Context 记录操作历史

### 整体架构图

```mermaid
graph TB
    User[用户/NL-Translator] -->|"POST /api/task/create"| TaskController

    subgraph "Orchestrator 模块"
        TaskController -->|创建任务| OrchestratorService
        OrchestratorService -->|校验DAG| DAGValidator
        OrchestratorService -->|保存| TaskRepository
        OrchestratorService -->|发送事件| EventProducer
        Scheduler -->|查询就绪节点| NodeRepository
        Scheduler -->|发布事件| EventProducer
        StateMachine -->|更新状态| NodeRepository
    end

    subgraph "事件总线 (Redpanda)"
        Topic_NodeReady["ai.node.ready"]
        Topic_NodeResult["ai.node.result"]
    end

    subgraph "数据存储"
        TaskRepository[(Task)]
        NodeRepository[(Node)]
        NodeDependencyRepository[(NodeDependency)]
    end

    EventProducer -->|"ai.node.ready"| Topic_NodeReady
    Topic_NodeResult -->|消费| EventConsumer
    EventConsumer -->|更新状态| StateMachine

    TaskRepository -.->|"CRUD"| PostgreSQL
    NodeRepository -.->|"CRUD"| PostgreSQL
    NodeDependencyRepository -.->|"CRUD"| PostgreSQL
```

---

## 核心功能

### 1. DAG 任务编排
- 接收结构化的 DAG 请求（节点 + 边）
- 节点支持多种类型：LLM、TOOL 等
- 支持节点优先级和重试配置
- 边定义节点间的依赖关系

### 2. 节点调度
- 定时扫描就绪节点（每秒一次）
- 乐观锁防止重复调度
- 支持任务暂停/恢复
- 支持节点重试

### 3. 状态管理
- 使用 PostgreSQL 持久化存储
- 任务状态：CREATED → RUNNING → SUCCESS/FAILED
- 节点状态：CREATED → READY → RUNNING → SUCCESS/FAILED

### 4. 事件驱动
- 通过 Redpanda 发布节点就绪事件
- 监听节点结果事件更新状态
- Worker 消费事件执行具体任务

---

## 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| Java | 17+ | 开发语言 |
| Spring Boot | 3.2.5 | 应用框架 |
| Spring Kafka | 3.1.x | Redpanda 客户端 |
| Spring Data JPA | 3.2.x | PostgreSQL 访问 |
| Redpanda | latest | 事件总线 |
| PostgreSQL | 16 | 持久化存储 |
| Maven | 3.9.x | 构建工具 |

---

## 项目结构

```
ai-orchestrator/
├── pom.xml
├── docker-compose.yml
└── src/main/java/com/lingxi/ai/orchestrator/
    ├── OrchestratorApplication.java           # 启动入口
    ├── controller/
    │   └── TaskController.java               # 任务API
    ├── service/
    │   ├── OrchestratorService.java          # 核心编排
    │   ├── Scheduler.java                    # 节点调度器
    │   ├── StateMachine.java                 # 状态机
    │   ├── StateMachineService.java           # 状态机服务
    │   ├── DAGValidator.java                 # DAG校验
    │   └── TaskExecutionControl.java         # 任务执行控制
    ├── event/
    │   ├── EventProducer.java                # 事件发送
    │   └── EventConsumer.java                # 事件消费
    ├── repository/
    │   ├── TaskRepository.java               # 任务仓储
    │   ├── NodeRepository.java               # 节点仓储
    │   └── NodeDependencyRepository.java     # 依赖仓储
    ├── entity/
    │   ├── Task.java                        # 任务实体
    │   ├── Node.java                        # 节点实体
    │   ├── NodeDependency.java               # 依赖实体
    │   ├── TaskStatus.java                   # 任务状态枚举
    │   ├── NodeStatus.java                   # 节点状态枚举
    │   └── NodeType.java                     # 节点类型枚举
    ├── model/
    │   ├── DAGRequest.java                  # DAG请求模型
    │   ├── NodeTaskEvent.java                # 节点任务事件
    │   └── NodeResultEvent.java              # 节点结果事件
    ├── context/
    │   └── ContextService.java              # 上下文服务代理
    └── config/
        ├── KafkaConfig.java                  # Kafka配置
        └── RedisConfig.java                  # Redis配置
```

---

## 完整 API 接口

### 基础信息
- **基础URL**: `http://localhost:8080`
- **内容类型**: `application/json`

### API 列表

#### 1. 任务管理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/task/create` | 创建新任务 |
| POST | `/api/task/{taskId}/dag` | 提交 DAG |
| GET | `/api/task/{taskId}` | 查询任务详情 |
| POST | `/api/task/{taskId}/pause` | 暂停任务 |
| POST | `/api/task/{taskId}/resume` | 恢复任务 |
| GET | `/api/task/{taskId}/pause-reason` | 获取暂停原因 |
| GET | `/api/task/{taskId}/context` | 获取任务上下文 |

#### 2. 节点管理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/node/{nodeId}/success` | 标记节点成功（外部回调） |
| POST | `/api/node/{nodeId}/failure` | 标记节点失败（外部回调） |
| POST | `/api/node/{nodeId}/retry` | 重试节点 |
| GET | `/api/node/{nodeId}/snapshot/latest` | 获取节点最新快照 |
| POST | `/api/node/{nodeId}/restore` | 从快照恢复 |

#### 3. 系统

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |

### API 详细说明

#### 1. 创建任务

```bash
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"input": {"prompt": "写一篇关于AI的文章"}}'
```

响应：
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "CREATED"
}
```

#### 2. 提交 DAG

```bash
curl -X POST http://localhost:8080/api/task/550e8400-e29b-41d4-a716-446655440000/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "node-1",
        "type": "LLM",
        "name": "write_article",
        "input": {"topic": "AI"},
        "priority": 5,
        "maxRetry": 3
      },
      {
        "id": "node-2",
        "type": "LLM",
        "name": "summarize",
        "input": {},
        "priority": 5
      }
    ],
    "edges": [
      {"from": "node-1", "to": "node-2"}
    ]
  }'
```

响应：
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "message": "DAG submitted successfully"
}
```

#### 3. 查询任务

```bash
curl http://localhost:8080/api/task/550e8400-e29b-41d4-a716-446655440000
```

响应：
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "RUNNING",
  "input": {"prompt": "写一篇关于AI的文章"},
  "output": null,
  "createdAt": "2024-01-15T10:30:00",
  "nodes": [
    {
      "id": "node-1",
      "taskId": "550e8400-e29b-41d4-a716-446655440000",
      "type": "LLM",
      "name": "write_article",
      "status": "SUCCESS",
      "input": {"topic": "AI"},
      "output": {"content": "这是一篇关于AI的文章..."},
      "errorMessage": null,
      "retryCount": 0,
      "maxRetry": 3,
      "priority": 5,
      "workerGroup": "default",
      "version": 2,
      "createdAt": "2024-01-15T10:30:01"
    },
    {
      "id": "node-2",
      "taskId": "550e8400-e29b-41d4-a716-446655440000",
      "type": "LLM",
      "name": "summarize",
      "status": "RUNNING",
      "input": {},
      "output": null,
      "errorMessage": null,
      "retryCount": 0,
      "maxRetry": 3,
      "priority": 5,
      "workerGroup": "default",
      "version": 1,
      "createdAt": "2024-01-15T10:30:01"
    }
  ]
}
```

#### 4. 暂停/恢复任务

```bash
# 暂停任务
curl -X POST http://localhost:8080/api/task/550e8400-e29b-41d4-a716-446655440000/pause \
  -H "Content-Type: application/json" \
  -d '{"reason": "Manual pause by user"}'

# 恢复任务
curl -X POST http://localhost:8080/api/task/550e8400-e29b-41d4-a716-446655440000/resume
```

#### 5. 重试节点

```bash
curl -X POST http://localhost:8080/api/node/node-1/retry
```

#### 6. 查询任务上下文

```bash
curl http://localhost:8080/api/task/550e8400-e29b-41d4-a716-446655440000/context
```

响应：返回 AI-Context 模块记录的操作历史

#### 7. 健康检查

```bash
curl http://localhost:8080/api/health
```

响应：
```json
{"status": "UP", "service": "ai-orchestrator"}
```

---

## 类定义详解

### 实体类

#### Task（任务实体）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | String | 任务ID（UUID） |
| userId | String | 用户ID |
| status | TaskStatus | 任务状态 |
| input | Map | 输入数据 |
| output | Map | 输出数据 |
| createdAt | LocalDateTime | 创建时间 |

#### Node（节点实体）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | String | 节点ID |
| taskId | String | 所属任务ID |
| type | NodeType | 节点类型（LLM/TOOL） |
| name | String | 节点名称/工具名 |
| status | NodeStatus | 节点状态 |
| input | Map | 输入数据 |
| output | Map | 输出数据 |
| errorMessage | String | 错误信息 |
| retryCount | int | 当前重试次数 |
| maxRetry | int | 最大重试次数 |
| priority | int | 优先级（1-10） |
| workerGroup | String | Worker分组 |
| version | int | 乐观锁版本号 |
| createdAt | LocalDateTime | 创建时间 |

### 枚举类

#### TaskStatus
```java
public enum TaskStatus {
    CREATED,   // 已创建
    RUNNING,   // 运行中
    SUCCESS,   // 成功
    FAILED     // 失败
}
```

#### NodeStatus
```java
public enum NodeStatus {
    CREATED,   // 已创建
    READY,     // 就绪（依赖满足）
    RUNNING,   // 运行中
    SUCCESS,   // 成功
    FAILED     // 失败
}
```

#### NodeType
```java
public enum NodeType {
    LLM,       // 大语言模型
    TOOL       // 工具调用
}
```

### 事件模型

#### NodeTaskEvent
```java
@Data
public class NodeTaskEvent {
    private String taskId;
    private String nodeId;
    private String type;         // 节点类型
    private Map<String, Object> payload;  // 执行负载
    private String traceId;
}
```

#### NodeResultEvent
```java
@Data
public class NodeResultEvent {
    private String taskId;
    private String nodeId;
    private NodeStatus status;
    private Map<String, Object> output;
    private String traceId;
    private String errorMessage;
}
```

---

## 工作流程

### 完整执行时序图

```mermaid
sequenceDiagram
    participant NL as NL-Translator
    participant TC as TaskController
    participant OS as OrchestratorService
    participant DV as DAGValidator
    participant TR as TaskRepository
    participant NR as NodeRepository
    participant DR as NodeDependencyRepository
    participant EP as EventProducer
    participant RP as Redpanda
    participant Sch as Scheduler
    participant SM as StateMachine
    participant Worker as Worker

    NL->>TC: POST /api/task/create
    TC->>OS: createTask(input)
    OS->>TR: save(task)
    OS->>TR: update status=RUNNING
    OS-->>TC: taskId

    NL->>TC: POST /api/task/{taskId}/dag
    TC->>OS: submitDAG(taskId, dagRequest)
    OS->>DV: validate(dagRequest)
    DV-->>OS: valid
    loop 创建节点
        OS->>NR: save(node)
    end
    loop 创建依赖边
        OS->>DR: save(dependency)
    end
    OS->>TR: update status=RUNNING
    OS-->>TC: DAG submitted

    Note over Sch: 每秒轮询
    Sch->>NR: findReadyNodes()
    NR-->>Sch: [node-1]
    Sch->>NR: updateStatusWithLock(node-1)
    Sch->>EP: publishNodeReady(node-1)
    EP->>RP: 发送 ai.node.ready

    RP->>Worker: 消费事件
    Worker->>Worker: 执行任务
    Worker->>RP: 发送 ai.node.result

    RP->>SM: handleNodeResult(event)
    alt node success
        SM->>NR: update node SUCCESS
        SM->>NR: findReadyNodes() 再次检查
        Note over SM: 如果node-2依赖满足，更新为READY
    else node failure
        SM->>NR: update node FAILED
        SM->>TR: update task FAILED
    end
```

### 节点调度流程

```
┌─────────────────────────────────────────────────────────────┐
│                    Scheduler (每秒执行)                     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ 查询就绪节点     │
                    │ findReadyNodes() │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ 检查任务是否暂停 │
                    │ canScheduleNodes│
                    └────────┬────────┘
                             │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
         [暂停]           [可调度]         [其他]
         跳过             继续             跳过
                             │
                             ▼
                    ┌─────────────────┐
                    │ 尝试获取锁       │
                    │ updateWithLock  │
                    └────────┬────────┘
                             │
              ┌───────────────┴───────────────┐
              │               │               │
              ▼               ▼               ▼
           [成功]          [失败]          [其他]
         更新为RUNNING     跳过             跳过
              │
              ▼
    ┌─────────────────┐
    │ 发布NodeReady事件│
    │ publishNodeReady│
    └────────┬────────┘
             │
             ▼
    ┌─────────────────┐
    │ 记录节点快照     │
    │ recordNodeSnapshot│
    └─────────────────┘
```

### Redpanda Topic 设计

| Topic | 说明 | 生产者 | 消费者 |
|-------|------|--------|--------|
| `ai.node.ready` | 节点就绪，Worker可以执行 | Orchestrator | Worker |
| `ai.node.result` | 节点执行结果 | Worker | Orchestrator |

---

## 快速开始

### 步骤 1：启动基础设施

```bash
cd /Users/wanglian/Projects/lingxi-ai-operation-system
docker compose up -d
```

### 步骤 2：编译并启动

```bash
cd ai-orchestrator
mvn spring-boot:run
```

### 步骤 3：测试完整流程

```bash
# 1. 创建任务
TASK_RESP=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"input": {"prompt": "测试任务"}}')
TASK_ID=$(echo $TASK_RESP | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")
echo "Task ID: $TASK_ID"

# 2. 提交 DAG
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "n1", "type": "LLM", "name": "test_node", "input": {}}
    ],
    "edges": []
  }'

# 3. 查询任务
sleep 2
curl "http://localhost:8080/api/task/${TASK_ID}"
```

---

## 下一步

- 了解 [NL-Translator 模块](./NL_TRANSLATOR_GUIDE.md)
- 了解 [AI-Context 模块](./AI_CONTEXT_GUIDE.md)
- 了解 [Worker 模块](./WORKER_GUIDE.md)
