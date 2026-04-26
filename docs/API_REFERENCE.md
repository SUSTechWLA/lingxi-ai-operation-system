# API Reference

Base URL: `http://localhost:8080`

## Response Format

All endpoints return JSON with the following standard format:

**Success:**
```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

**Error:**
```json
{
  "code": 400,
  "message": "error description",
  "data": null
}
```

Common HTTP status codes:
- `200` — Success
- `400` — Bad Request (missing or invalid parameters)
- `500` — Internal Server Error

---

## Table of Contents

1. [Health](#health)
2. [Publish Module (Frontend-facing)](#publish-module-frontend-facing)
   - [POST /api/publish — Submit publish task](#post-apipublish)
   - [POST /api/ai/generate — AI generate content](#post-apiaigenerate)
   - [POST /api/ai/generate-from-media — AI generate from media](#post-apiaigenerate-from-media)
   - [POST /api/ai/polish — AI polish text](#post-apiaipolish)
   - [GET /api/weather/query — Query weather](#get-apiweatherquery)
2. [Trace — Task trace query](#2-trace-task-trace-query)
   - [GET /api/trace/recent — Get recent trace](#get-apitracerecent)
   - [GET /api/trace/:taskId — Get task trace](#get-apitracetaskid)
3. [Orchestrator Module](#3-orchestrator-module)
   - [POST /api/task/create — Create task](#post-apitaskcreate)
   - [POST /api/task/:taskId/dag — Submit DAG](#post-apitasktaskiddag)
   - [GET /api/task/:taskId — Get task](#get-apitasktaskid)
   - [GET /api/task/:taskId/context — Get context](#get-apitasktaskidcontext)
   - [POST /api/task/:taskId/pause — Pause task](#post-apitasktaskidpause)
   - [POST /api/task/:taskId/resume — Resume task](#post-apitasktaskidresume)
   - [GET /api/task/:taskId/pause-reason — Get pause reason](#get-apitasktaskidpause-reason)
4. [Node Operations](#4-node-operations)
   - [POST /api/node/:nodeId/success — Report success](#post-apinodenodeidsuccess)
   - [POST /api/node/:nodeId/failure — Report failure](#post-apinodenodeidfailure)
   - [POST /api/node/:nodeId/retry — Retry node](#post-apinodenodeidretry)
   - [GET /api/node/:nodeId/snapshot/latest — Get snapshot](#get-apinodenodeidsnapshotlatest)
   - [POST /api/node/:nodeId/restore — Restore from snapshot](#post-apinodenodeidrestore)
5. [NL-Translator](#5-nl-translator)
   - [POST /api/translate — Translate NL to DAG](#post-apitranslate)
   - [POST /api/translate/submit — Translate and submit](#post-apitranslatesubmit)
6. [NL-Driven DAG Submission](#6-nl-driven-dag-submission)
   - [POST /api/node — Submit DAG directly](#post-apinode)
7. [Context](#7-context)
   - [GET /api/context/:taskId — Get task context](#get-apicontexttaskid)
   - [GET /api/context/:taskId/node/:nodeId/snapshot/latest — Get node snapshot](#get-apicontexttaskidnodenodeidsnapshotlatest)
   - [POST /api/context/:taskId/node/:nodeId/restore — Restore node](#post-apicontexttaskidnodenodeidrestore)
   - [POST /api/context/record — Record context manually](#post-apicontextrecord)
8. [Built-in Tools](#8-built-in-tools)
9. [Error Responses](#9-error-responses)

---

## 1. Health

### GET /api/health

Check if the service is running.

**Example:**
```bash
curl http://localhost:8080/api/health
```

**Response** `200`:
```json
{
  "service": "ai-orchestrator",
  "status": "UP"
}
```

---

## 2. Publish Module (Frontend-facing)

### POST /api/publish

Submit content for multi-platform publishing. Creates an AI task that polishes the content before publishing.

**Content-Type:** `multipart/form-data`

**Form Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | **Yes** | Content title |
| `description` | string | **Yes** | Content description |
| `keywords` | string | No | Comma-separated keywords |
| `platforms` | string | No | JSON array of platform IDs, e.g. `["douyin","xiaohongshu"]` |
| `images` | File[] | No | Image files (jpg, png, webp, gif) |
| `videos` | File[] | No | Video files (mp4, mov, avi, mkv) |

**Example:**
```bash
curl -X POST http://localhost:8080/api/publish \
  -F "title=今日美食推荐" \
  -F "description=推荐几家好吃的餐厅" \
  -F "keywords=美食,餐厅,推荐" \
  -F 'platforms=["douyin","xiaohongshu"]' \
  -F "images=@photo.jpg"
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "taskId": "20260426091349-a8a8a8a8",
    "message": "内容已提交发布任务"
  }
}
```

**Response** `400` (missing required fields):
```json
{
  "code": 400,
  "message": "title and description are required",
  "data": null
}
```

---

### POST /api/ai/generate

Generate title and description from a text prompt using AI.

**Content-Type:** `application/json`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | **Yes** | Text prompt describing the content idea |

**Example:**
```bash
curl -X POST http://localhost:8080/api/ai/generate \
  -H "Content-Type: application/json" \
  -d '{"prompt":"周末户外活动推荐"}'
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "title": "周末别宅家！8个低门槛户外活动直接抄作业",
    "description": "整理了适配不同人数、预算的周末户外玩法..."
  }
}
```

---

### POST /api/ai/generate-from-media

Generate title and description from uploaded media files (images/videos) plus a text prompt.

**Content-Type:** `multipart/form-data`

**Form Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | No | Additional text description |
| `images` | File[] | No | Image files for context |
| `videos` | File[] | No | Video files for context |

**Example:**
```bash
curl -X POST http://localhost:8080/api/ai/generate-from-media \
  -F "prompt=风景照片" \
  -F "images=@scenery.jpg"
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "title": "这组神仙风景也太治愈了！",
    "description": "收录了不同时节不同地点的宝藏自然风景..."
  }
}
```

---

### POST /api/ai/polish

Polish existing text (title or description) using AI.

**Content-Type:** `application/json`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `text` | string | **Yes** | Text to polish |
| `type` | string | No | `"title"` or `"description"` (default: `"description"`) |

**Example:**
```bash
curl -X POST http://localhost:8080/api/ai/polish \
  -H "Content-Type: application/json" \
  -d '{"text":"这个周末我们去爬山","type":"description"}'
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "content": "这周末就别宅家啦，我们一起去爬山...",
    "taskId": "20260426232624-b0989898",
    "traceUrl": "/api/trace/20260426232624-b0989898"
  }
}
```

---

### GET /api/weather/query

Query weather information for a city. Currently returns simulated data.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `city` | string | **Yes** | City name (e.g. "北京", "上海") |

**Example:**
```bash
curl "http://localhost:8080/api/weather/query?city=北京"
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "city": "北京",
    "temperature": "22°C",
    "condition": "Sunny",
    "humidity": "45%",
    "wind": "Light breeze, 8 km/h",
    "forecast": "Clear skies expected throughout the day"
  }
}
```

**Response** `400` (missing city):
```json
{
  "code": 400,
  "message": "city parameter is required",
  "data": null
}
```

---

## 2. Trace — Task Trace Query

Trace endpoints provide complete task lifecycle data (task details + all context entries) for debugging and auditing.

### GET /api/trace/recent

Query the most recently executed task's full trace.

**Example:**
```bash
curl http://localhost:8080/api/trace/recent
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task": {
      "taskId": "20260426231320-68686850",
      "status": "SUCCESS",
      "input": {"source": "nl-translator"},
      "output": null,
      "createdAt": "2026-04-26T23:13:20.960495Z",
      "nodes": [...]
    },
    "contexts": [
      {
        "id": 9185846,
        "contextType": "TASK_CREATED",
        "taskId": "20260426231320-68686850",
        "sourceModule": "Orchestrator",
        "message": "任务创建成功，等待 DAG 提交",
        "createdAt": "0001-01-01T00:00:00Z"
      },
      {
        "id": 9185850,
        "contextType": "NODE_SCHEDULED",
        "taskId": "20260426231320-68686850",
        "nodeId": "polish-xxx",
        "sourceModule": "ContextService",
        "sourceTopic": "ai.node.result",
        "metadata": {"startedAt": "2026-04-26T23:13:21.048766+08:00"},
        "message": "Kafka 事件记录：节点进入运行状态"
      },
      {
        "id": 9185851,
        "contextType": "NODE_SUCCESS",
        "taskId": "20260426231320-68686850",
        "nodeId": "polish-xxx",
        "sourceModule": "ContextService",
        "sourceTopic": "ai.node.result",
        "metadata": {"durationMs": 12543, "exitCode": 0},
        "message": "Kafka 事件记录：节点执行成功"
      }
    ]
  }
}
```

**Response** `404` (no tasks found):
```json
{
  "code": 404,
  "message": "no tasks found",
  "data": null
}
```

---

### GET /api/trace/:taskId

Query a specific task's full trace by task ID.

**Example:**
```bash
curl http://localhost:8080/api/trace/20260426231320-68686850
```

**Response** `200` (same format as /api/trace/recent):
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task": {...},
    "contexts": [...]
  }
}
```

**Response** `404`:
```json
{
  "code": 404,
  "message": "task not found",
  "data": null
}
```

---

### Trace Data Schema

Each trace response contains two sections: `task` (the full task with nodes) and `contexts` (ordered lifecycle events).

**Task Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `taskId` | string | Task unique identifier |
| `status` | string | One of: `CREATED`, `RUNNING`, `SUCCESS`, `FAILED`, `PAUSED` |
| `input` | object | Task input parameters |
| `output` | object | Task output / result (null if not completed) |
| `createdAt` | string | ISO 8601 timestamp |
| `nodes` | Node[] | All nodes belonging to this task |

**Node Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Node ID |
| `type` | string | `TOOL` or `LLM` |
| `name` | string | Tool name (e.g. `polisher`, `llm_api`) |
| `status` | string | One of: `CREATED`, `READY`, `RUNNING`, `SUCCESS`, `FAILED`, `RETRYING`, `SKIPPED` |
| `input` | object | Node input parameters |
| `output` | object | Node output — includes execution metrics (`startedAt`, `durationMs`, `exitCode`, `error`, `resourceUsage`) |
| `condition` | string | Conditional expression (if any) |
| `retryCount` | int | Number of retries attempted |
| `maxRetry` | int | Maximum allowed retries |
| `idempotencyKey` | string | Kafka idempotency key (`taskId + "-" + nodeId`) |

**Context Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Auto-increment ID |
| `contextType` | string | Event type (see below) |
| `taskId` | string | Associated task ID |
| `nodeId` | string | Associated node ID (empty for task-level events) |
| `sourceModule` | string | Originating module: `Orchestrator`, `StateMachine`, or `ContextService` |
| `sourceTopic` | string | Kafka topic this event was consumed from (only for `ContextService` entries) |
| `metadata` | object | Additional data (inputPreview, outputPreview, execution metrics) |
| `message` | string | Human-readable description |
| `createdAt` | string | Timestamp |

**Context Types (ordered lifecycle):**

| # | Type | Source | Description |
|---|------|--------|-------------|
| 1 | `TASK_CREATED` | Orchestrator | Task created, awaiting DAG submission |
| 2 | `DAG_VALIDATED` | Orchestrator | DAG structure validated (no cycles, no duplicates) |
| 3 | `DAG_SUBMITTED` | Orchestrator | DAG submitted, nodes written to DB |
| 4 | `NODE_READY` | StateMachine | Node dependencies met, transitioned to READY |
| 5 | `NODE_SCHEDULED` | ContextService | Worker picked up the node, execution started |
| 6 | `NODE_SUCCESS` | StateMachine | Node execution succeeded (RUNNING → SUCCESS) |
| 6 | `NODE_SUCCESS` | ContextService | Kafka event: node execution result (with durationMs, exitCode) |
| 7 | `TASK_SUCCESS` | StateMachine | All nodes completed, task finished |

---

## 3. Orchestrator Module

### POST /api/task/create

Create a new empty task.

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userId` | string | No | User identifier |

**Example:**
```bash
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "test-user"}'
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED"
}
```

---

### POST /api/task/:taskId/dag

Submit a DAG (nodes + edges) to an existing task.

**Path Parameters:**

| Name | Description |
|------|-------------|
| `taskId` | Task ID returned from create |

**Request Body:**

```json
{
  "nodes": [
    {
      "id": "node-1",
      "type": "TOOL",
      "name": "weather",
      "input": {"city": "Beijing"}
    },
    {
      "id": "node-2",
      "type": "LLM",
      "name": "Summarize",
      "input": {"prompt": "Generate travel advice based on weather"}
    },
    {
      "id": "node-3",
      "type": "TOOL",
      "name": "error-handler",
      "condition": "node-1.status == failed",
      "input": {"message": "Weather query failed"}
    }
  ],
  "edges": [
    {"from": "node-1", "to": "node-2"},
    {"from": "node-1", "to": "node-3"}
  ]
}
```

**Node Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | **Yes** | Unique node identifier within the task |
| `type` | string | **Yes** | `"LLM"` or `"TOOL"` |
| `name` | string | **Yes** | Node name; for TOOL type, used for tool routing |
| `input` | object | No | Node input parameters |
| `condition` | string | No | Conditional expression (e.g. `"nodeA.status == success"`) |
| `maxRetry` | int | No | Maximum retry count (default: 3) |
| `priority` | int | No | Node priority |
| `workerGroup` | string | No | Worker group assignment |

**Condition Format:** `"nodeId.status == success"` or `"nodeId.status == failed"`. When condition is not met, the node is automatically marked as SKIPPED.

**Example:**
```bash
TASK_ID="20260423150000-a1b2c3"
curl -X POST http://localhost:8080/api/task/${TASK_ID}/dag \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "n1", "type": "TOOL", "name": "weather", "input": {"city": "Beijing"}},
      {"id": "n2", "type": "LLM", "name": "Summarize", "input": {"prompt": "Generate travel advice"}}
    ],
    "edges": [
      {"from": "n1", "to": "n2"}
    ]
  }'
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "DAG submitted successfully"
}
```

---

### GET /api/task/:taskId

Get task details including all nodes and their statuses.

**Example:**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3
```

**Response** `200`:
```json
{
  "task": {
    "id": "20260423150000-a1b2c3",
    "status": "RUNNING",
    "userId": "test-user",
    "pauseReason": ""
  },
  "nodes": [
    {
      "id": "n1",
      "type": "TOOL",
      "name": "weather",
      "status": "SUCCESS",
      "condition": "",
      "output": {"city": "Beijing", "temperature": "22°C", "condition": "Sunny"}
    },
    {
      "id": "n2",
      "type": "TOOL",
      "name": "error-handler",
      "status": "SKIPPED",
      "condition": "n1.status == failed"
    }
  ]
}
```

**Node Statuses:** `CREATED`, `READY`, `RUNNING`, `SUCCESS`, `FAILED`, `RETRYING`, `SKIPPED`

---

### GET /api/task/:taskId/context

Get all context records for a task.

**Example:**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3/context
```

**Response** `200`:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 9185846,
      "contextType": "TASK_CREATED",
      "taskId": "20260423150000-a1b2c3",
      "sourceModule": "Orchestrator",
      "message": "任务创建成功，等待 DAG 提交",
      "createdAt": "0001-01-01T00:00:00Z"
    },
    {
      "id": 9185850,
      "contextType": "NODE_SCHEDULED",
      "taskId": "20260423150000-a1b2c3",
      "nodeId": "n1",
      "sourceModule": "ContextService",
      "sourceTopic": "ai.node.result",
      "metadata": {
        "startedAt": "2026-04-23T15:00:01.123456+08:00"
      },
      "message": "Kafka 事件记录：节点进入运行状态"
    }
  ]
}
```

---

### POST /api/task/:taskId/pause

Pause a running task.

**Example:**
```bash
curl -X POST http://localhost:8080/api/task/20260423150000-a1b2c3/pause \
  -H "Content-Type: application/json" \
  -d '{"reason": "debugging"}'
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "Task paused successfully"
}
```

---

### POST /api/task/:taskId/resume

Resume a paused task.

**Example:**
```bash
curl -X POST http://localhost:8080/api/task/20260423150000-a1b2c3/resume
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "Task resumed successfully"
}
```

---

### GET /api/task/:taskId/pause-reason

Get the reason a task was paused.

**Example:**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3/pause-reason
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "reason": "Manual pause for debugging"
}
```

---

## 4. Node Operations

### POST /api/node/:nodeId/success

Report a node execution as successful.

**Example:**
```bash
curl -X POST http://localhost:8080/api/node/n1/success \
  -H "Content-Type: application/json" \
  -d '{"result": "success", "data": {}}'
```

**Response** `200`:
```json
{
  "message": "Node success recorded"
}
```

---

### POST /api/node/:nodeId/failure

Report a node execution as failed.

**Example:**
```bash
curl -X POST http://localhost:8080/api/node/n1/failure \
  -H "Content-Type: application/json" \
  -d '{"errorMessage": "timeout"}'
```

**Response** `200`:
```json
{
  "message": "Node failure recorded"
}
```

---

### POST /api/node/:nodeId/retry

Retry a failed node. Resets node status to CREATED and re-initializes it.

**Example:**
```bash
curl -X POST http://localhost:8080/api/node/n1/retry
```

**Response** `200`:
```json
{
  "nodeId": "n1",
  "message": "Node retry initiated"
}
```

---

### GET /api/node/:nodeId/snapshot/latest

Get the latest snapshot for a node.

**Example:**
```bash
curl http://localhost:8080/api/node/n1/snapshot/latest
```

**Response** `200`:
```json
{
  "nodeId": "n1",
  "snapshot": {
    "id": "ctx-001",
    "type": "SNAPSHOT",
    "message": "Node state snapshot",
    "data": {}
  }
}
```

**Response** `404`:
```json
{
  "error": "No snapshot found"
}
```

---

### POST /api/node/:nodeId/restore

Restore a node from its latest snapshot.

**Example:**
```bash
curl -X POST http://localhost:8080/api/node/n1/restore
```

**Response** `200`:
```json
{
  "nodeId": "n1",
  "message": "Node snapshot retrieved",
  "snapshot": {
    "id": "ctx-001",
    "type": "SNAPSHOT",
    "data": {}
  }
}
```

---

## 5. NL-Translator

### POST /api/translate

Translate a natural language prompt into a DAG structure.

**Example:**
```bash
curl -X POST http://localhost:8080/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Query Beijing weather and generate a summary"}'
```

**Response** `200`:
```json
{
  "nodes": [
    {"id": "n1", "type": "TOOL", "name": "weather", "input": {"city": "Beijing"}},
    {"id": "n2", "type": "LLM", "name": "Summarize", "input": {"prompt": "Generate travel advice"}}
  ],
  "edges": [
    {"from": "n1", "to": "n2"}
  ]
}
```

---

### POST /api/translate/submit

Translate a natural language prompt and immediately submit the DAG to the orchestrator.

**Example:**
```bash
curl -X POST http://localhost:8080/api/translate/submit \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Query Beijing weather and generate a summary"}'
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED",
  "message": "DAG submitted successfully"
}
```

---

## 6. NL-Driven DAG Submission

### POST /api/node

Submit a complete DAG in one step. Creates a task and submits the DAG in a single request.

**Example — basic:**
```bash
curl -X POST http://localhost:8080/api/node \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "n1", "type": "TOOL", "name": "weather", "input": {"city": "Beijing"}},
      {"id": "n2", "type": "LLM", "name": "Summarize", "input": {"prompt": "Generate travel advice"}}
    ],
    "edges": [
      {"from": "n1", "to": "n2"}
    ]
  }'
```

**Example — conditional branching:**
```bash
curl -X POST http://localhost:8080/api/node \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "weather-1", "type": "TOOL", "name": "weather", "input": {"city": "Beijing"}},
      {"id": "success-1", "type": "LLM", "name": "travel-advice", "condition": "weather-1.status == success", "input": {"prompt": "Generate travel advice"}},
      {"id": "fail-1", "type": "TOOL", "name": "bash", "condition": "weather-1.status == failed", "input": {"command": "echo weather failed"}}
    ],
    "edges": [
      {"from": "weather-1", "to": "success-1"},
      {"from": "weather-1", "to": "fail-1"}
    ]
  }'
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED",
  "message": "DAG submitted successfully"
}
```

---

## 7. Context

### GET /api/context/:taskId

Get all context records for a task (same as GET /api/task/:taskId/context).

**Example:**
```bash
curl http://localhost:8080/api/context/20260423150000-a1b2c3
```

---

### GET /api/context/:taskId/node/:nodeId/snapshot/latest

Get the latest snapshot for a specific node.

**Example:**
```bash
curl http://localhost:8080/api/context/20260423150000-a1b2c3/node/n1/snapshot/latest
```

---

### POST /api/context/:taskId/node/:nodeId/restore

Restore a node from its latest snapshot.

**Example:**
```bash
curl -X POST http://localhost:8080/api/context/20260423150000-a1b2c3/node/n1/restore
```

---

### POST /api/context/record

Manually record a context entry.

**Example:**
```bash
curl -X POST http://localhost:8080/api/context/record \
  -H "Content-Type: application/json" \
  -d '{"taskId": "20260423150000-a1b2c3", "nodeId": "n1", "type": "CUSTOM", "message": "test"}'
```

**Context Types:** `TASK_CREATED`, `DAG_VALIDATED`, `DAG_SUBMITTED`, `NODE_READY`, `NODE_SCHEDULED`, `NODE_SUCCESS`, `NODE_FAILED`, `NODE_RETRY`, `NODE_SKIPPED`, `TASK_SUCCESS`, `TASK_FAILED`, `SNAPSHOT`, `CUSTOM`

**Response** `200`:
```json
{
  "message": "Context recorded"
}
```

---

## 8. Built-in Tools

| Tool Name | Type | Description | Input Parameters |
|-----------|------|-------------|-----------------|
| `llm_api` | LLM | OpenAI-compatible chat API | `prompt` / `message` / `content` (string, required) |
| `bash` | CUSTOM | Sandboxed shell execution | `command` (string, required) |
| `polisher` | CUSTOM | Text polish via LLM | `text` (string, required), `polishType` (`"title"` or `"description"`) |

---

## 9. Error Responses

**400 Bad Request:**
```json
{
  "code": 400,
  "message": "invalid request body",
  "data": null
}
```

**500 Internal Server Error:**
```json
{
  "code": 500,
  "message": "database connection failed",
  "data": null
}
```
