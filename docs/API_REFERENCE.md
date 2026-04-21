# 灵犀AIOS 完整API参考文档

> **版本**: 1.0.0
> **最后更新**: 2026-04-21

## 目录

- [系统架构概述](#系统架构概述)
- [NL-Translator API (8081)](#nl-translator-api-端口-8081)
- [AI-Orchestrator API (8080)](#ai-orchestrator-api-端口-8080)
- [AI-Worker API (8083)](#ai-worker-api-端口-8083)
- [AI-Context API (8082)](#ai-context-api-端口-8082)
- [事件Topic说明](#事件topic说明)
- [数据模型](#数据模型)

---

## 系统架构概述

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户请求流程                                │
└─────────────────────────────────────────────────────────────────┘

用户自然语言
    │
    ▼
┌──────────────┐     REST API      ┌────────────────┐
│ NL-Translator│ ────────────────▶ │ AI-Orchestrator│
│  (8081)      │   DAG JSON        │   (8080)       │
└──────────────┘                    └───────┬────────┘
                                              │
                           ┌──────────────────┼──────────────────┐
                           │                  │                  │
                           ▼                  ▼                  ▼
                    ┌──────────┐     ┌──────────────┐   ┌──────────┐
                    │AI-Context │     │   Redpanda   │   │ AI-Worker│
                    │  (8082)  │     │  Event Bus   │   │  (8083)  │
                    └──────────┘     └──────────────┘   └──────────┘
                       (记录历史)         (事件驱动)        (执行工具)
```

---

## NL-Translator API (端口: 8081)

自然语言翻译模块，负责将用户的自然语言转换为DAG任务图。

### 基础信息
- **Base URL**: `http://localhost:8081`
- **Content-Type**: `application/json`

### API端点

#### 1. 健康检查
```http
GET /api/health
```

**响应示例**:
```json
{
  "status": "UP",
  "service": "nl-translator"
}
```

---

#### 2. 自然语言转DAG
```http
POST /api/translate
Content-Type: application/json

{
  "prompt": "写一篇关于AI的文章"
}
```

**响应示例**:
```json
{
  "prompt": "写一篇关于AI的文章",
  "dag": {
    "nodes": [
      {
        "id": "node-llm-001",
        "type": "LLM",
        "name": "write_article",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于AI的文章",
            "system_prompt": "你是一个专业的文章作者"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": []
  }
}
```

---

#### 3. 翻译并提交任务
```http
POST /api/translate-and-submit
Content-Type: application/json

{
  "prompt": "写一篇关于AI的文章并生成摘要"
}
```

**响应示例**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "RUNNING",
  "message": "Task created and DAG submitted"
}
```

---

#### 4. 查询任务状态
```http
GET /api/task/{taskId}
```

**响应示例**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "SUCCESS",
  "nodes": [
    {
      "id": "node-1",
      "status": "SUCCESS",
      "output": {"text": "文章内容..."}
    }
  ]
}
```

---

## AI-Orchestrator API (端口: 8080)

任务编排模块，核心调度引擎。

### 基础信息
- **Base URL**: `http://localhost:8080`
- **Content-Type**: `application/json`

### API端点

#### 1. 健康检查
```http
GET /api/health
```

**响应**:
```json
{
  "status": "UP",
  "service": "ai-orchestrator"
}
```

---

#### 2. 创建任务
```http
POST /api/task/create
Content-Type: application/json

{
  "userId": "user-001",
  "input": {
    "prompt": "测试任务"
  }
}
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "CREATED"
}
```

---

#### 3. 提交DAG
```http
POST /api/task/{taskId}/dag
Content-Type: application/json

{
  "nodes": [
    {
      "id": "node-1",
      "type": "LLM",
      "name": "write_article",
      "input": {
        "tool": "llm",
        "parameters": {
          "prompt": "写一篇关于AI的文章"
        }
      },
      "priority": 5,
      "maxRetry": 3
    },
    {
      "id": "node-2",
      "type": "LLM",
      "name": "summarize",
      "input": {
        "tool": "llm",
        "parameters": {
          "prompt": "总结以下文章: {{parent.node-1.text}}"
        }
      },
      "maxRetry": 3
    }
  ],
  "edges": [
    {"from": "node-1", "to": "node-2"}
  ]
}
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "message": "DAG submitted successfully"
}
```

**DAG请求字段说明**:
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| nodes | Array | 是 | 节点列表 |
| nodes[].id | String | 是 | 节点唯一ID |
| nodes[].type | String | 是 | 节点类型: LLM 或 TOOL |
| nodes[].name | String | 是 | 节点名称/工具名 |
| nodes[].input | Object | 是 | 节点输入数据 |
| nodes[].priority | Integer | 否 | 优先级(1-10), 默认5 |
| nodes[].maxRetry | Integer | 否 | 最大重试次数, 默认3 |
| edges | Array | 否 | 依赖边列表 |
| edges[].from | String | 是 | 源节点ID |
| edges[].to | String | 是 | 目标节点ID |

---

#### 4. 查询任务详情
```http
GET /api/task/{taskId}
```

**响应示例**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "userId": "user-001",
  "status": "RUNNING",
  "input": {"prompt": "测试任务"},
  "output": null,
  "createdAt": "2026-04-21T10:30:00",
  "nodes": [
    {
      "id": "node-1",
      "taskId": "550e8400-e29b-41d4-a716-446655440000",
      "type": "LLM",
      "name": "write_article",
      "status": "SUCCESS",
      "input": {"tool": "llm", "parameters": {...}},
      "output": {"text": "文章内容..."},
      "errorMessage": null,
      "retryCount": 0,
      "maxRetry": 3,
      "priority": 5,
      "workerGroup": "default",
      "version": 2,
      "createdAt": "2026-04-21T10:30:01"
    },
    {
      "id": "node-2",
      "type": "LLM",
      "name": "summarize",
      "status": "RUNNING",
      "input": {...},
      "output": null,
      "retryCount": 0,
      "maxRetry": 3
    }
  ]
}
```

**任务状态枚举**:
- `CREATED`: 已创建
- `RUNNING`: 运行中
- `PAUSED`: 已暂停
- `SUCCESS`: 成功
- `FAILED`: 失败

**节点状态枚举**:
- `CREATED`: 已创建
- `READY`: 就绪(依赖满足)
- `RUNNING`: 运行中
- `SUCCESS`: 成功
- `FAILED`: 失败

---

#### 5. 查询任务上下文
```http
GET /api/task/{taskId}/context
```

**响应示例**:
```json
[
  {
    "id": "ctx-001",
    "taskId": "550e8400-e29b-41d4-a716-446655440000",
    "nodeId": null,
    "contextType": "TASK_CREATED",
    "payload": {...},
    "createdAt": "2026-04-21T10:30:00"
  },
  {
    "id": "ctx-002",
    "taskId": "550e8400-e29b-41d4-a716-446655440000",
    "nodeId": "node-1",
    "contextType": "NODE_SUCCESS",
    "payload": {...},
    "createdAt": "2026-04-21T10:30:05"
  }
]
```

---

#### 6. 暂停任务
```http
POST /api/task/{taskId}/pause
Content-Type: application/json

{
  "reason": "需要人工检查"
}
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "message": "Task paused successfully"
}
```

---

#### 7. 恢复任务
```http
POST /api/task/{taskId}/resume
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "message": "Task resumed successfully"
}
```

---

#### 8. 获取暂停原因
```http
GET /api/task/{taskId}/pause-reason
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "节点 node-1 执行失败: 网络超时"
}
```

---

#### 9. 重试失败节点
```http
POST /api/node/{nodeId}/retry
```

**响应**:
```json
{
  "nodeId": "node-1",
  "message": "Node retry initiated"
}
```

---

#### 10. 标记节点成功(外部回调)
```http
POST /api/node/{nodeId}/success
Content-Type: application/json

{
  "result": "执行结果数据"
}
```

**响应**:
```json
{
  "message": "Node success recorded"
}
```

---

#### 11. 标记节点失败(外部回调)
```http
POST /api/node/{nodeId}/failure
Content-Type: application/json

{
  "errorMessage": "执行失败原因"
}
```

**响应**:
```json
{
  "message": "Node failure recorded"
}
```

---

#### 12. 获取节点最新快照
```http
GET /api/node/{nodeId}/snapshot/latest
```

**响应**:
```json
{
  "nodeId": "node-1",
  "snapshot": {
    "id": "node-1",
    "type": "LLM",
    "name": "write_article",
    "status": "SUCCESS",
    "input": {...},
    "output": {...},
    "retryCount": 0,
    "maxRetry": 3,
    "version": 2
  }
}
```

---

#### 13. 从快照恢复节点
```http
POST /api/node/{nodeId}/restore
```

**响应**:
```json
{
  "nodeId": "node-1",
  "message": "Node snapshot retrieved",
  "snapshot": {...}
}
```

---

## AI-Worker API (端口: 8083)

工具执行模块，负责执行具体的工具调用。

### 基础信息
- **Base URL**: `http://localhost:8083`
- **Content-Type**: `application/json`

### API端点

#### 1. 健康检查
```http
GET /api/health
```

**响应**:
```json
{
  "status": "UP",
  "service": "ai-worker"
}
```

---

#### 2. 获取已注册工具列表
```http
GET /api/tools
```

**响应示例**:
```json
{
  "count": 4,
  "tools": [
    {
      "name": "llm",
      "description": "OpenAI LLM API 工具",
      "type": "BUILTIN"
    },
    {
      "name": "database",
      "description": "数据库查询工具(SELECT only)",
      "type": "BUILTIN"
    },
    {
      "name": "file",
      "description": "文件系统操作工具",
      "type": "BUILTIN"
    },
    {
      "name": "bash",
      "description": "Bash命令执行工具",
      "type": "BUILTIN"
    }
  ]
}
```

---

#### 3. 注册外部工具
```http
POST /worker/register
Content-Type: application/json

{
  "endpoint": "http://localhost:8092",
  "workerGroup": "default"
}
```

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "toolName": "weather_query",
    "toolVersion": "1.0.0",
    "status": "REGISTERED"
  }
}
```

**外部工具必须实现的接口**:

1. **GET /info** - 获取工具元数据
```json
{
  "toolName": "weather_query",
  "toolVersion": "1.0.0",
  "description": "查询天气信息",
  "parameters": {
    "type": "object",
    "properties": {
      "city": {
        "type": "string",
        "description": "城市名称"
      }
    },
    "required": ["city"]
  }
}
```

2. **POST /run** - 执行工具
```json
// 请求
{
  "taskId": "task-001",
  "nodeId": "node-001",
  "input": {
    "city": "北京"
  }
}

// 响应
{
  "code": 200,
  "message": "success",
  "data": {
    "city": "北京",
    "temperature": 25,
    "weather": "晴",
    "humidity": 60
  }
}
```

3. **GET /health** - 健康检查
```json
{
  "status": "UP"
}
```

---

#### 4. 注销外部工具
```http
POST /worker/unregister
Content-Type: application/json

{
  "endpoint": "http://localhost:8092"
}
```

**响应**:
```json
{
  "code": 200,
  "message": "success"
}
```

---

#### 5. 获取外部工具列表
```http
GET /worker/tools
```

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "count": 1,
    "tools": [
      {
        "toolName": "weather_query",
        "toolVersion": "1.0.0",
        "description": "查询天气信息",
        "endpoint": "http://localhost:8092",
        "status": "HEALTHY",
        "registeredAt": "2026-04-21T10:30:00"
      }
    ]
  }
}
```

---

#### 6. 获取指定外部工具详情
```http
GET /worker/tools/{toolName}
```

**响应示例**:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "toolName": "weather_query",
    "toolVersion": "1.0.0",
    "description": "查询天气信息",
    "endpoint": "http://localhost:8092",
    "status": "HEALTHY",
    "toolInfo": {...},
    "registeredAt": "2026-04-21T10:30:00",
    "lastHealthCheckAt": "2026-04-21T10:35:00"
  }
}
```

---

### 内置工具使用说明

#### 1. LLM工具
**工具名称**: `llm`

**参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| prompt | String | 是 | 提示词 |
| system_prompt | String | 否 | 系统提示词 |
| model | String | 否 | 模型名称, 默认gpt-4 |
| temperature | Double | 否 | 温度参数, 默认0.7 |
| max_tokens | Integer | 否 | 最大token数 |

**示例**:
```json
{
  "tool": "llm",
  "parameters": {
    "prompt": "写一篇关于AI的文章",
    "system_prompt": "你是一个专业的科技作者",
    "model": "gpt-4",
    "temperature": 0.7
  }
}
```

---

#### 2. 数据库工具
**工具名称**: `database`

**参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| sql | String | 是 | SQL查询语句(仅限SELECT) |

**示例**:
```json
{
  "tool": "database",
  "parameters": {
    "sql": "SELECT COUNT(*) as count FROM ai_task"
  }
}
```

---

#### 3. 文件工具
**工具名称**: `file`

**参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| operation | String | 是 | 操作类型: read, write, list, delete |
| path | String | 是 | 文件路径 |
| content | String | 否 | 写入内容(write时需要) |
| recursive | Boolean | 否 | 是否递归(list时使用) |

**示例**:
```json
// 写入文件
{
  "tool": "file",
  "parameters": {
    "operation": "write",
    "path": "test.txt",
    "content": "Hello, AIOS!"
  }
}

// 读取文件
{
  "tool": "file",
  "parameters": {
    "operation": "read",
    "path": "test.txt"
  }
}

// 列出文件
{
  "tool": "file",
  "parameters": {
    "operation": "list",
    "path": ".",
    "recursive": false
  }
}
```

---

#### 4. Bash工具
**工具名称**: `bash`

**参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| command | String | 是 | 要执行的命令 |
| timeout | Integer | 否 | 超时秒数, 默认60 |

**示例**:
```json
{
  "tool": "bash",
  "parameters": {
    "command": "ls -la /tmp",
    "timeout": 60
  }
}
```

---

## AI-Context API (端口: 8082)

上下文管理模块，记录操作历史和快照。

### 基础信息
- **Base URL**: `http://localhost:8082`
- **Content-Type**: `application/json`

### API端点

#### 1. 健康检查
```http
GET /api/health
```

**响应**:
```json
{
  "status": "UP",
  "service": "ai-context"
}
```

---

#### 2. 获取任务上下文
```http
GET /api/task/{taskId}/context
```

**响应示例**:
```json
[
  {
    "id": "ctx-001",
    "taskId": "550e8400-e29b-41d4-a716-446655440000",
    "nodeId": null,
    "contextType": "TASK_CREATED",
    "payload": {...},
    "createdAt": "2026-04-21T10:30:00"
  },
  {
    "id": "ctx-002",
    "taskId": "550e8400-e29b-41d4-a716-446655440000",
    "nodeId": "node-1",
    "contextType": "NODE_SCHEDULED",
    "payload": {...},
    "createdAt": "2026-04-21T10:30:01"
  },
  {
    "id": "ctx-003",
    "taskId": "550e8400-e29b-41d4-a716-446655440000",
    "nodeId": "node-1",
    "contextType": "NODE_SUCCESS",
    "payload": {...},
    "createdAt": "2026-04-21T10:30:05"
  }
]
```

**上下文类型枚举**:
- `TASK_CREATED`: 任务创建
- `DAG_SUBMITTED`: DAG提交
- `DAG_VALIDATED`: DAG验证
- `NODE_SCHEDULED`: 节点调度
- `NODE_READY`: 节点就绪
- `NODE_SUCCESS`: 节点成功
- `NODE_FAILED`: 节点失败
- `NODE_RETRY`: 节点重试
- `NODE_SNAPSHOT`: 节点快照
- `TASK_SUCCESS`: 任务成功
- `TASK_FAILED`: 任务失败

---

#### 3. 获取节点最新快照
```http
GET /api/node/{nodeId}/snapshot/latest
```

**响应**:
```json
{
  "nodeId": "node-1",
  "snapshot": {
    "id": "node-1",
    "type": "LLM",
    "name": "write_article",
    "status": "SUCCESS",
    "input": {...},
    "output": {...},
    "retryCount": 0,
    "maxRetry": 3,
    "priority": 5,
    "workerGroup": "default",
    "version": 2,
    "errorMessage": null
  }
}
```

---

#### 4. 记录任务创建 (内部API)
```http
POST /api/context/task-created
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### 5. 记录DAG提交 (内部API)
```http
POST /api/context/dag-submitted
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### 6. 记录节点调度 (内部API)
```http
POST /api/context/node-scheduled
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "type": "LLM",
  "name": "write_article"
}
```

---

#### 7. 记录节点成功 (内部API)
```http
POST /api/context/node-success
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1"
}
```

---

#### 8. 记录节点失败 (内部API)
```http
POST /api/context/node-failed
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "errorMessage": "执行失败原因"
}
```

---

#### 9. 记录节点快照 (内部API)
```http
POST /api/context/node-snapshot
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "type": "LLM",
  "name": "write_article",
  "status": "SUCCESS",
  "input": {...},
  "output": {...},
  "retryCount": 0,
  "maxRetry": 3,
  "priority": 5,
  "workerGroup": "default",
  "version": 2,
  "errorMessage": null
}
```

---

## 事件Topic说明

系统使用Redpanda(Kafka兼容)作为事件总线。

### Topic列表

| Topic | 生产者 | 消费者 | 说明 |
|-------|--------|--------|------|
| `ai.task.created` | Orchestrator | Context | 任务创建事件 |
| `ai.node.ready` | Orchestrator | Worker | 节点就绪事件 |
| `ai.node.result` | Worker | Orchestrator | 节点结果事件 |
| `ai.context.events` | All | Context | 上下文事件 |

---

### 事件格式

#### 1. ai.node.ready (节点就绪事件)
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "type": "LLM",
  "name": "write_article",
  "payload": {
    "tool": "llm",
    "parameters": {
      "prompt": "写一篇关于AI的文章"
    }
  },
  "traceId": "trace-001",
  "timestamp": "2026-04-21T10:30:00"
}
```

---

#### 2. ai.node.result (节点结果事件)
```json
// 成功
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "status": "SUCCESS",
  "output": {
    "text": "文章内容..."
  },
  "traceId": "trace-001",
  "timestamp": "2026-04-21T10:30:05"
}

// 失败
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "status": "FAILED",
  "output": null,
  "errorMessage": "网络超时",
  "traceId": "trace-001",
  "timestamp": "2026-04-21T10:30:05"
}
```

---

## 数据模型

### 任务状态枚举 (TaskStatus)
```java
public enum TaskStatus {
    CREATED,   // 已创建
    RUNNING,   // 运行中
    PAUSED,    // 已暂停
    SUCCESS,   // 成功
    FAILED     // 失败
}
```

### 节点状态枚举 (NodeStatus)
```java
public enum NodeStatus {
    CREATED,   // 已创建
    READY,     // 就绪
    RUNNING,   // 运行中
    SUCCESS,   // 成功
    FAILED     // 失败
}
```

### 节点类型枚举 (NodeType)
```java
public enum NodeType {
    LLM,       // 大语言模型节点
    TOOL       // 工具调用节点
}
```

### 工具类型枚举 (ToolType)
```java
public enum ToolType {
    BUILTIN,   // 内置工具
    EXTERNAL   // 外部工具
}
```

---

## 完整使用示例

### 示例1: 单节点LLM任务

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

echo "Task ID: $TASK_ID"

# 2. 提交DAG
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
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
            "prompt": "写一篇关于人工智能未来发展的短文，300字以内"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": []
  }'

# 3. 等待并查询状态
sleep 5
curl "http://localhost:8080/api/task/${TASK_ID}"

# 4. 查看上下文
curl "http://localhost:8080/api/task/${TASK_ID}/context"
```

---

### 示例2: 两节点串行任务

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

# 2. 提交包含两个节点的DAG
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
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
            "prompt": "写一篇关于人工智能的文章，500字左右"
          }
        },
        "maxRetry": 3
      },
      {
        "id": "summarize",
        "type": "LLM",
        "name": "summarize",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "请为以下文章生成摘要: {{parent.write-article.text}}"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": [
      {"from": "write-article", "to": "summarize"}
    ]
  }'

# 3. 查询任务状态
sleep 10
curl "http://localhost:8080/api/task/${TASK_ID}"
```

---

### 示例3: 使用NL-Translator一键提交

```bash
# 直接通过自然语言提交任务
RESPONSE=$(curl -s -X POST http://localhost:8081/api/translate-and-submit \
  -H "Content-Type: application/json" \
  -d '{"prompt": "写一篇关于AI的文章并生成摘要"}')

TASK_ID=$(echo $RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

echo "Task ID: $TASK_ID"

# 查询任务状态
sleep 10
curl "http://localhost:8081/api/task/${TASK_ID}"
```

---

### 示例4: 并行节点任务

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

# 2. 提交两个并行节点(没有edges即为并行)
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "write-ai",
        "type": "LLM",
        "name": "write_ai",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于AI的短文，200字以内"
          }
        },
        "maxRetry": 2
      },
      {
        "id": "write-space",
        "type": "LLM",
        "name": "write_space",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "写一篇关于太空探索的短文，200字以内"
          }
        },
        "maxRetry": 2
      }
    ],
    "edges": []
  }'

# 3. 两个节点会被同时调度执行
sleep 8
curl "http://localhost:8080/api/task/${TASK_ID}"
```

---

## 配置说明

### 环境变量

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `POSTGRES_HOST` | PostgreSQL主机 | localhost |
| `POSTGRES_PORT` | PostgreSQL端口 | 5432 |
| `POSTGRES_DB` | PostgreSQL数据库名 | lingxi_db |
| `POSTGRES_USER` | PostgreSQL用户名 | wanglian |
| `POSTGRES_PASSWORD` | PostgreSQL密码 | - |
| `KAFKA_BOOTSTRAP_SERVERS` | Kafka/Redpanda服务器 | localhost:9092 |
| `REDIS_HOST` | Redis主机 | localhost |
| `REDIS_PORT` | Redis端口 | 6379 |
| `OPENAI_API_KEY` | OpenAI API密钥 | - |
| `OPENAI_BASE_URL` | OpenAI API基础URL | https://api.openai.com/v1 |
| `OPENAI_MODEL` | OpenAI模型 | gpt-4 |
| `CONTEXT_SERVICE_URL` | Context服务URL | http://localhost:8082 |

---

## 错误处理

### HTTP状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
| 503 | 服务不可用(如API key未配置) |

### 错误响应格式

```json
{
  "error": "错误描述",
  "type": "错误类型",
  "hint": "可选的解决建议"
}
```

---

## 附录

### 相关文档

- [MVP_SETUP_GUIDE.md](./MVP_SETUP_GUIDE.md) - 完整部署与使用指南
- [ORCHESTRATOR_GUIDE.md](./ORCHESTRATOR_GUIDE.md) - Orchestrator模块详解
- [WORKER_GUIDE.md](./WORKER_GUIDE.md) - Worker模块详解
- [NL_TRANSLATOR_GUIDE.md](./NL_TRANSLATOR_GUIDE.md) - NL-Translator模块详解
- [AI_CONTEXT_GUIDE.md](./AI_CONTEXT_GUIDE.md) - AI-Context模块详解

### 支持与反馈

如有问题，请提交Issue或联系开发团队。
