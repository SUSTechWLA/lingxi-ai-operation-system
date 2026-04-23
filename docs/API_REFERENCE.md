# 灵犀AIOS 完整API参考文档

> **版本**: 2.1.0
> **最后更新**: 2026-04-23

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
  "prompt": "查询北京天气并生成总结报告"
}
```

**响应示例**:
```json
{
  "prompt": "查询北京天气并生成总结报告",
  "dag": {
    "nodes": [
      {
        "id": "node-tool-001",
        "type": "TOOL",
        "name": "weather_query",
        "input": {
          "tool": "weather_query",
          "parameters": {
            "city": "北京",
            "type": "realtime"
          }
        },
        "maxRetry": 3
      },
      {
        "id": "node-llm-001",
        "type": "LLM",
        "name": "summarize",
        "input": {
          "tool": "llm",
          "parameters": {
            "prompt": "根据以下天气数据生成简要总结报告: {{parent.node-tool-001.output}}"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": [
      {"from": "node-tool-001", "to": "node-llm-001"}
    ]
  }
}
```

---

#### 3. 翻译并提交任务
```http
POST /api/translate-and-submit
Content-Type: application/json

{
  "prompt": "查询北京天气并生成总结报告"
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
      "id": "node-tool-001",
      "status": "SUCCESS",
      "output": {"city": "北京", "temperature": 25, "weather": "晴", "humidity": 60}
    },
    {
      "id": "node-llm-001",
      "status": "SUCCESS",
      "output": {"text": "北京今日天气晴朗，气温25°C，湿度60%..."}
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
      "id": "query-weather",
      "type": "TOOL",
      "name": "weather_query",
      "input": {
        "tool": "weather_query",
        "parameters": {
          "city": "北京",
          "type": "realtime"
        }
      },
      "priority": 5,
      "maxRetry": 3
    },
    {
      "id": "summarize",
      "type": "LLM",
      "name": "summarize",
      "input": {
        "tool": "llm",
        "parameters": {
          "prompt": "根据以下天气数据生成简要总结报告: {{parent.query-weather.output}}"
        }
      },
      "maxRetry": 3
    }
  ],
  "edges": [
    {"from": "query-weather", "to": "summarize"}
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
  "input": {"prompt": "查询北京天气并生成总结报告"},
  "output": null,
  "createdAt": "2026-04-23T10:30:00",
  "nodes": [
    {
      "id": "query-weather",
      "taskId": "550e8400-e29b-41d4-a716-446655440000",
      "type": "TOOL",
      "name": "weather_query",
      "status": "SUCCESS",
      "input": {"tool": "weather_query", "parameters": {"city": "北京", "type": "realtime"}},
      "output": {"city": "北京", "temperature": 25, "humidity": 60, "windSpeed": 12, "weather": "晴"},
      "errorMessage": null,
      "retryCount": 0,
      "maxRetry": 3,
      "priority": 5,
      "workerGroup": "default",
      "version": 2,
      "createdAt": "2026-04-23T10:30:01"
    },
    {
      "id": "summarize",
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
- `RETRYING`: 重试中（指数退避等待）
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

#### 14. 一步创建任务并提交DAG
```http
POST /api/node
Content-Type: application/json

{
  "nodes": [
    {
      "nodeId": "query-weather",
      "type": "TOOL",
      "name": "weather_query",
      "input": {
        "tool": "weather_query",
        "parameters": {
          "city": "上海",
          "type": "realtime"
        }
      }
    }
  ],
  "edges": []
}
```

**响应**:
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "CREATED",
  "message": "DAG submitted successfully"
}
```

**说明**: 此端点专为 NL-Translator 设计，将创建任务和提交 DAG 合并为一步操作。支持 `nodeId` 或 `id` 字段作为节点标识。

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
  "endpoint": "http://localhost:8090",
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

**外部工具必须实现的接口**（以 `weather_tool.py` 为例）:

1. **GET /tool/info** - 获取工具元数据
```json
{
  "toolName": "weather_query",
  "toolVersion": "1.0.0",
  "description": "查询指定城市的实时天气信息，支持温度、湿度、风力等数据。",
  "inputSchema": {
    "type": "object",
    "properties": {
      "city": {
        "type": "string",
        "description": "要查询的城市名称，如'北京'、'上海'、'广州'、'深圳'"
      },
      "type": {
        "type": "string",
        "description": "查询类型，可选值：realtime(实时天气)/forecast(天气预报)",
        "default": "realtime"
      }
    },
    "required": ["city"]
  },
  "outputSchema": {
    "type": "object",
    "properties": {
      "city": {"type": "string", "description": "城市名称"},
      "temperature": {"type": "number", "description": "实时温度，单位摄氏度"},
      "humidity": {"type": "number", "description": "相对湿度，百分比"},
      "windSpeed": {"type": "number", "description": "风速，单位公里/小时"},
      "weather": {"type": "string", "description": "天气状况描述"},
      "queryTime": {"type": "string", "description": "查询时间"}
    }
  }
}
```

2. **POST /tool/execute** - 执行工具
```json
// 请求
{
  "taskId": "task-001",
  "nodeId": "node-001",
  "traceId": "trace-001",
  "input": {
    "city": "北京",
    "type": "realtime"
  }
}

// 响应
{
  "code": 200,
  "message": "success",
  "data": {
    "city": "北京",
    "temperature": 25,
    "humidity": 60,
    "windSpeed": 12,
    "weather": "晴",
    "queryTime": "2026-04-23 14:30:00",
    "type": "realtime"
  }
}
```

3. **GET /tool/health** - 健康检查
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "status": "UP",
    "version": "1.0.0"
  }
}
```

---

#### 4. 注销外部工具
```http
POST /worker/unregister
Content-Type: application/json

{
  "endpoint": "http://localhost:8090"
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
        "description": "查询指定城市的实时天气信息",
        "endpoint": "http://localhost:8090",
        "status": "HEALTHY",
        "registeredAt": "2026-04-23T10:30:00"
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
    "description": "查询指定城市的实时天气信息",
    "endpoint": "http://localhost:8090",
    "status": "HEALTHY",
    "toolInfo": {...},
    "registeredAt": "2026-04-23T10:30:00",
    "lastHealthCheckAt": "2026-04-23T10:35:00"
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
    "prompt": "根据以下天气数据生成简要总结报告: 北京25°C晴天，上海28°C多云",
    "system_prompt": "你是一个气象分析师",
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
  "nodeId": "query-weather",
  "snapshot": {
    "id": "query-weather",
    "type": "TOOL",
    "name": "weather_query",
    "status": "SUCCESS",
    "input": {...},
    "output": {"city": "北京", "temperature": 25, "weather": "晴"},
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
  "nodeId": "query-weather",
  "type": "TOOL",
  "name": "weather_query"
}
```

---

#### 7. 记录节点成功 (内部API)
```http
POST /api/context/node-success
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather"
}
```

---

#### 8. 记录节点失败 (内部API)
```http
POST /api/context/node-failed
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather",
  "errorMessage": "城市不存在"
}
```

---

#### 9. 记录节点快照 (内部API)
```http
POST /api/context/node-snapshot
Content-Type: application/json

{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather",
  "type": "TOOL",
  "name": "weather_query",
  "status": "SUCCESS",
  "input": {...},
  "output": {"city": "北京", "temperature": 25, "weather": "晴"},
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
| `ai.task.success` | Orchestrator | Context | 任务成功事件 |
| `ai.task.failed` | Orchestrator | Context | 任务失败事件 |
| `ai.node.ready` | Orchestrator (DependencyChecker) | Worker | 节点就绪事件（含 idempotencyKey） |
| `ai.node.result` | Worker | Orchestrator | 节点结果事件（含 idempotencyKey） |
| `ai.node.executed` | Orchestrator (StateMachine) | Context, DependencyChecker | 节点执行完成事件（事件驱动调度） |
| `ai.node.failed` | Orchestrator (StateMachine) | Context | 节点永久失败事件 |
| `ai.context.events` | All | Context | 上下文事件 |

---

### 事件格式

#### 1. ai.node.ready (节点就绪事件)
```json
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather",
  "type": "TOOL",
  "name": "weather_query",
  "payload": {
    "tool": "weather_query",
    "parameters": {
      "city": "北京",
      "type": "realtime"
    }
  },
  "traceId": "trace-001",
  "idempotencyKey": "550e8400-e29b-41d4-a716-446655440000-query-weather",
  "timestamp": "2026-04-23T10:30:00"
}
```

---

#### 2. ai.node.result (节点结果事件)
```json
// 成功（天气查询节点）
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather",
  "status": "SUCCESS",
  "output": {
    "city": "北京",
    "temperature": 25,
    "humidity": 60,
    "windSpeed": 12,
    "weather": "晴",
    "queryTime": "2026-04-23 14:30:00"
  },
  "traceId": "trace-001",
  "errorMessage": null,
  "idempotencyKey": "550e8400-e29b-41d4-a716-446655440000-query-weather"
}

// 失败
{
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "query-weather",
  "status": "FAILED",
  "output": null,
  "errorMessage": "城市不存在",
  "traceId": "trace-001",
  "idempotencyKey": "550e8400-e29b-41d4-a716-446655440000-query-weather"
}
```

---

#### 3. ai.node.executed (节点执行完成事件)
```json
{
  "event_type": "ai.node.executed",
  "taskId": "550e8400-e29b-41d4-a716-446655440000",
  "nodeId": "node-1",
  "status": "SUCCESS"
}
```

**说明**: 由 StateMachine 在节点成功后发布，DependencyChecker 消费此事件检查下游依赖并触发新的 `ai.node.ready` 事件。

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
    READY,     // 就绪（依赖满足）
    RUNNING,   // 运行中
    RETRYING,  // 重试中（指数退避等待）
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

### Node 实体新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `idempotencyKey` | String(128, unique) | 幂等键，格式为 `taskId + "-" + nodeId`，由 Orchestrator 在 DAG 提交时自动生成。用于 Kafka 消息去重和节点执行幂等保证 |

---

## 完整使用示例

### 示例1: 查询单城市天气（单节点 TOOL 任务）

```bash
# 0. 先注册外部天气工具
cd examples
pip install fastapi uvicorn
python weather_tool.py &
sleep 2

curl -X POST http://localhost:8083/worker/register \
  -H "Content-Type: application/json" \
  -d '{"endpoint": "http://localhost:8090"}'

# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

echo "Task ID: $TASK_ID"

# 2. 提交DAG（查询北京天气）
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "query-beijing",
        "type": "TOOL",
        "name": "weather_query",
        "input": {
          "tool": "weather_query",
          "parameters": {
            "city": "北京",
            "type": "realtime"
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

### 示例2: 查询天气并生成总结报告（TOOL + LLM 串行任务）

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

# 2. 提交包含两个节点的DAG：先查天气，再用LLM总结
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "query-weather",
        "type": "TOOL",
        "name": "weather_query",
        "input": {
          "tool": "weather_query",
          "parameters": {
            "city": "北京",
            "type": "realtime"
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
            "prompt": "根据以下天气数据生成简要总结报告: {{parent.query-weather.output}}"
          }
        },
        "maxRetry": 3
      }
    ],
    "edges": [
      {"from": "query-weather", "to": "summarize"}
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
  -d '{"prompt": "查询北京天气并生成总结报告"}')

TASK_ID=$(echo $RESPONSE | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

echo "Task ID: $TASK_ID"

# 查询任务状态
sleep 10
curl "http://localhost:8081/api/task/${TASK_ID}"
```

---

### 示例4: 并行查询多个城市天气

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-user"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['taskId'])")

# 2. 提交两个并行节点（无 edges 即为并行）
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {
        "id": "query-beijing",
        "type": "TOOL",
        "name": "weather_query",
        "input": {
          "tool": "weather_query",
          "parameters": {
            "city": "北京",
            "type": "realtime"
          }
        },
        "maxRetry": 2
      },
      {
        "id": "query-shanghai",
        "type": "TOOL",
        "name": "weather_query",
        "input": {
          "tool": "weather_query",
          "parameters": {
            "city": "上海",
            "type": "realtime"
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
