# API Reference

Base URL: `http://localhost:8080`

## Response Format

All endpoints return JSON with the following standard format:

**Success:**
```json
{
  "code": 200,
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

1. [Health](#1-health)
2. [Publish Module (Frontend-facing)](#2-publish-module-frontend-facing)
   - [POST /api/publish — Submit publish task](#post-apipublish)
   - [POST /api/ai/generate — AI generate content](#post-apiaigenerate)
   - [POST /api/ai/generate-from-media — AI generate from media](#post-apiaigenerate-from-media)
   - [POST /api/ai/polish — AI polish text](#post-apiaipolish)
3. [Trace — Task Trace Query](#3-trace--task-trace-query)
   - [GET /api/trace/recent — Get recent trace](#get-apitracerecent)
   - [GET /api/trace/:taskId — Get task trace](#get-apitracetaskid)
4. [Orchestrator Module](#4-orchestrator-module)
   - [POST /api/task/create — Create task](#post-apitaskcreate)
   - [POST /api/task/:taskId/dag — Submit DAG](#post-apitasktaskiddag)
   - [GET /api/task/:taskId — Get task](#get-apitasktaskid)
   - [GET /api/task/:taskId/context — Get context](#get-apitasktaskidcontext)
   - [POST /api/task/:taskId/pause — Pause task](#post-apitasktaskidpause)
   - [POST /api/task/:taskId/fail — Fail task](#post-apitasktaskidfail)
   - [POST /api/task/:taskId/resume — Resume task](#post-apitasktaskidresume)
   - [GET /api/task/:taskId/pause-reason — Get pause reason](#get-apitasktaskidpause-reason)
5. [Node Operations](#5-node-operations)
   - [POST /api/node/:nodeId/success — Report success](#post-apinodenodeidsuccess)
   - [POST /api/node/:nodeId/failure — Report failure](#post-apinodenodeidfailure)
   - [POST /api/node/:nodeId/retry — Retry node](#post-apinodenodeidretry)
   - [GET /api/node/:nodeId/snapshot/latest — Get snapshot](#get-apinodenodeidsnapshotlatest)
   - [POST /api/node/:nodeId/restore — Restore from snapshot](#post-apinodenodeidrestore)
6. [NL-Translator](#6-nl-translator)
   - [POST /api/translate — Translate NL to DAG](#post-apitranslate)
   - [POST /api/translate/submit — Translate and submit](#post-apitranslatesubmit)
7. [NL-Driven DAG Submission](#7-nl-driven-dag-submission)
   - [POST /api/node — Submit DAG directly](#post-apinode)
8. [Context](#8-context)
   - [GET /api/context/:taskId — Get task context](#get-apicontexttaskid)
   - [GET /api/context/:taskId/node/:nodeId/snapshot/latest — Get node snapshot](#get-apicontexttaskidnodenodeidsnapshotlatest)
   - [POST /api/context/:taskId/node/:nodeId/restore — Restore node](#post-apicontexttaskidnodenodeidrestore)
   - [POST /api/context/record — Record context manually](#post-apicontextrecord)
9. [Media Management API](#9-media-management-api)
   - [POST /api/media/upload — Upload media files](#post-apimediaupload)
   - [GET /api/media/list — List media assets](#get-apimedialist)
   - [GET /api/media/:id — Get media by ID](#get-apimediaid)
   - [PUT /api/media/:id/tags — Update media tags](#put-apimediaidtags)
10. [Skill / AI Assistant Dialog API](#10-skill--ai-assistant-dialog-api)
11. [Tool Registry API](#11-tool-registry-api)
12. [Built-in Tools](#12-built-in-tools)
13. [Error Responses](#13-error-responses)

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
| `content_type` | string | No | `"image"` or `"video"` |
| `platforms` | string | No | JSON array of platform IDs, e.g. `["douyin","xiaohongshu"]` |
| `images` | File[] | No | Image files (jpg, png, webp, gif) |
| `videos` | File[] | No | Video files (mp4, mov, avi, mkv) |
| `cover` | File | No | Cover image file |

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
  "code": 200,
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
  "code": 200,
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
  "code": 200,
  "message": "success",
  "data": {
    "title": "这组神仙风景也太治愈了！",
    "description": "收录了不同时节不同地点的宝藏自然风景..."
  }
}
```

---

### POST /api/ai/polish

Polish existing text (title or description) using AI (synchronous — waits for completion).

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
  "code": 200,
  "message": "success",
  "data": {
    "content": "这周末就别宅家啦，我们一起去爬山...",
    "taskId": "20260426232624-b0989898",
    "traceUrl": "/api/trace/20260426232624-b0989898"
  }
}
```

---

### POST /api/ai/polish/submit

Submit a polish task and return immediately with taskId + nodeId (asynchronous). Used by the frontend to support cancel during long-running polish operations.

**Content-Type:** `application/json`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `text` | string | **Yes** | Text to polish |
| `type` | string | No | `"title"` or `"description"` (default: `"description"`) |

**Example:**
```bash
curl -X POST http://localhost:8080/api/ai/polish/submit \
  -H "Content-Type: application/json" \
  -d '{"text":"这个周末我们去爬山","type":"description"}'
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "taskId": "20260426232624-b0989898",
    "nodeId": "polish-xxxxx",
    "message": "Polish task submitted",
    "traceUrl": "/api/trace/20260426232624-b0989898"
  }
}
```

---

### GET /api/ai/polish/result

Query the result of an async polish task. Used together with `POST /api/ai/polish/submit`.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `taskId` | string | **Yes** | Task ID from submit response |
| `nodeId` | string | No | Node ID (optional, queries latest if omitted) |

**Example:**
```bash
curl "http://localhost:8080/api/ai/polish/result?taskId=20260426232624-b0989898&nodeId=polish-xxxxx"
```

**Response** `200` (still running):
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "taskId": "20260426232624-b0989898",
    "nodeId": "polish-xxxxx",
    "status": "RUNNING",
    "traceUrl": "/api/trace/20260426232624-b0989898"
  }
}
```

**Response** `200` (completed):
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "taskId": "20260426232624-b0989898",
    "nodeId": "polish-xxxxx",
    "status": "SUCCESS",
    "content": "这周末就别宅家啦，我们一起去爬山...",
    "traceUrl": "/api/trace/20260426232624-b0989898"
  }
}
```

**Node Status Values:** `CREATED`, `READY`, `RUNNING`, `SUCCESS`, `FAILED`

---

## 3. Trace — Task Trace Query

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
  "code": 200,
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
  "code": 200,
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
| 7 | `NODE_FAILED` | StateMachine | Node execution permanently failed (RUNNING → FAILED) |
| 7 | `AI_CANCELLED` | Frontend / Handler | User cancelled AI operation via frontend |
| 8 | `TASK_SUCCESS` | StateMachine | All nodes completed, task finished |
| 8 | `TASK_FAILED` | StateMachine | Task marked as failed |

---

## 4. Orchestrator Module

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
      "name": "bash",
      "input": {"command": "echo 'Hello World'"}
    },
    {
      "id": "node-2",
      "type": "LLM",
      "name": "Summarize",
      "input": {"prompt": "Generate a summary of the task output"}
    },
    {
      "id": "node-3",
      "type": "TOOL",
      "name": "bash",
      "condition": "node-1.status == failed",
      "input": {"command": "echo 'Task failed'"}
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
      {"id": "n1", "type": "TOOL", "name": "bash", "input": {"command": "echo 'Hello'"}},
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
      "name": "bash",
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
  "code": 200,
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

### POST /api/task/:taskId/fail

Immediately mark a task as FAILED. Used by the frontend cancel mechanism to ensure the backend task stops processing when the user cancels an AI operation.

**Example:**
```bash
curl -X POST http://localhost:8080/api/task/20260423150000-a1b2c3/fail
```

**Response** `200`:
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "Task failed successfully"
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

## 5. Node Operations

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

## 6. NL-Translator

### POST /api/translate

Translate a natural language prompt into a DAG structure.

**Example:**
```bash
curl -X POST http://localhost:8080/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Run a hello world command and generate a summary"}'
```

**Response** `200`:
```json
{
  "nodes": [
    {"id": "n1", "type": "TOOL", "name": "bash", "input": {"command": "echo 'Hello'"}},
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
  -d '{"prompt": "Run a hello world command and generate a summary"}'
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

## 7. NL-Driven DAG Submission

### POST /api/node

Submit a complete DAG in one step. Creates a task and submits the DAG in a single request.

**Example — basic:**
```bash
curl -X POST http://localhost:8080/api/node \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "n1", "type": "TOOL", "name": "bash", "input": {"command": "echo 'Hello'"}},
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
      {"id": "bash-1", "type": "TOOL", "name": "bash", "input": {"command": "echo 'Hello'"}},
      {"id": "success-1", "type": "LLM", "name": "summary", "condition": "bash-1.status == success", "input": {"prompt": "Generate a summary"}},
      {"id": "fail-1", "type": "TOOL", "name": "bash", "condition": "bash-1.status == failed", "input": {"command": "echo 'Task failed'"}}
    ],
    "edges": [
      {"from": "bash-1", "to": "success-1"},
      {"from": "bash-1", "to": "fail-1"}
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

## 8. Context

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

**Context Types:** `TASK_CREATED`, `DAG_VALIDATED`, `DAG_SUBMITTED`, `NODE_READY`, `NODE_SCHEDULED`, `NODE_SUCCESS`, `NODE_FAILED`, `NODE_RETRY`, `NODE_SKIPPED`, `TASK_SUCCESS`, `TASK_FAILED`, `AI_CANCELLED`, `SNAPSHOT`, `CUSTOM`

**Response** `200`:
```json
{
  "message": "Context recorded"
}
```

---

## 9. Media Management API

### POST /api/media/upload

Upload media files (images/videos). Files are stored in MinIO and metadata is saved to the database.

**Content-Type:** `multipart/form-data`

**Form Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userId` | string | No | User identifier (default: "default") |
| `files` | File[] | **Yes** | One or more image/video files |

**Example:**
```bash
curl -X POST http://localhost:8080/api/media/upload \
  -F "userId=test-user" \
  -F "files=@photo.jpg"
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "media-1714294410000-photo",
      "userId": "test-user",
      "originalName": "photo.jpg",
      "mimeType": "image/jpeg",
      "size": 1024000,
      "minioPath": "test-user/2026/04/28/media-1714294410000-photo.jpg",
      "tags": [],
      "createdAt": "2026-04-28T12:00:00Z",
      "updatedAt": "2026-04-28T12:00:00Z"
    }
  ]
}
```

---

### GET /api/media/list

List uploaded media assets with pagination and optional tag filtering.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `userId` | string | No | User identifier (default: "default") |
| `offset` | int | No | Pagination offset (default: 0) |
| `limit` | int | No | Page size (default: 20) |
| `tag` | string | No | Filter by tag name |

**Example:**
```bash
curl "http://localhost:8080/api/media/list?userId=test-user&offset=0&limit=10&tag=风景"
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [...],
    "total": 1
  }
}
```

---

### GET /api/media/:id

Get a single media asset by ID.

**Example:**
```bash
curl http://localhost:8080/api/media/media-1714294410000-photo
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": "media-1714294410000-photo",
    "userId": "test-user",
    "originalName": "photo.jpg",
    "mimeType": "image/jpeg",
    "size": 1024000,
    "minioPath": "test-user/2026/04/28/media-1714294410000-photo.jpg",
    "tags": [],
    "createdAt": "2026-04-28T12:00:00Z",
    "updatedAt": "2026-04-28T12:00:00Z"
  }
}
```

**Response** `404`:
```json
{
  "code": 404,
  "message": "media asset not found: ...",
  "data": null
}
```

---

### PUT /api/media/:id/tags

Update tags for a media asset.

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tags` | string[] | **Yes** | New tag array |

**Example:**
```bash
curl -X PUT http://localhost:8080/api/media/media-1714294410000-photo/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["风景", "旅行", "故宫"]}'
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": null
}
```

---


## 10. Skill / AI Assistant Dialog API

AI 对话助手 API，每次对话 = 一个 Task，经 Orchestrator → Worker 执行，Context 全程追踪。

### POST /api/skill/dialog/session/create

创建新的对话会话。可传入当前页面上下文（标题、简介、关键词、已上传素材）辅助 AI 理解。

**Content-Type:** `application/json`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string | No | 用户标识 |
| `title` | string | No | 当前页面标题 |
| `description` | string | No | 当前页面简介 |
| `body` | string | No | 当前页面正文 |
| `keywords` | string[] | No | 当前页面关键词 |
| `media_count` | int | No | 已上传素材数量 |
| `media_names` | string[] | No | 素材文件名列表 |
| `media_ids` | string[] | No | 素材 ID 列表 |

**Example:**
```bash
curl -X POST http://localhost:8080/api/skill/dialog/session/create \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user001",
    "title": "我的创作页面",
    "description": "一个关于美食的创作",
    "keywords": ["美食", "探店"],
    "media_count": 1,
    "media_names": ["photo.jpg"],
    "media_ids": ["media-xxx"]
  }'
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": { "session_id": "uuid-string" }
}
```

---

### GET /api/skill/dialog/session/:session_id

获取会话完整状态（消息历史、媒体上下文、任务列表），用于恢复对话。

**Example:**
```bash
curl http://localhost:8080/api/skill/dialog/session/uuid-string
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "session_id": "uuid-string",
    "messages": [...],
    "media_context": {...},
    "task_ids": ["20260502131237-b8b8b8b8"],
    "terminated": false,
    "created_at": "2026-05-02T13:12:37Z"
  }
}
```

---

### POST /api/skill/dialog/session/:session_id/chat

发送对话消息。系统会结合会话历史 + 媒体上下文 + 工具清单生成 DAG，通过 Orchestrator 执行后返回结果。

**Content-Type:** `application/json`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | **Yes** | 用户消息 |

**Example:**
```bash
curl -X POST http://localhost:8080/api/skill/dialog/session/uuid-string/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "根据上传的图片生成标题和简介"}'
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "reply": "内容已生成！",
    "fields": {
      "title": "生成的标题",
      "description": "生成的简介",
      "keywords": ["关键词1", "关键词2"],
      "task_id": "20260502131237-b8b8b8b8"
    }
  }
}
```

---

### GET /api/skill/dialog/session/:session_id/progress

查询会话当前执行进度。

**Example:**
```bash
curl http://localhost:8080/api/skill/dialog/session/uuid-string/progress
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "status": "EXECUTING",
    "task_id": "20260502131237-b8b8b8b8"
  }
}
```

**Status Values:** `IDLE`, `EXECUTING`, `TERMINATED`

---

### POST /api/skill/dialog/session/:session_id/terminate

终止会话。

**Example:**
```bash
curl -X POST http://localhost:8080/api/skill/dialog/session/uuid-string/terminate
```

---

## 11. Tool Registry API

工具注册表 API，用于查询、注册、注销工具。工具清单存储在 DB（`tool_manifests` 表）中，Redis 缓存加速查询（TTL 5分钟），启动时自动同步 builtin 工具。AI 助手通过此 API 获取系统可用工具完整信息。

### GET /api/tools — 列出所有工具

返回所有已注册工具（builtin + external）的完整 Manifest。

**Example:**
```bash
curl http://localhost:8080/api/tools
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "name": "chat_generate",
      "description": "Conversational content generation with full message history support",
      "type": "builtin",
      "parameters": { "messages": { "type": "array", "description": "...", "required": true } },
      "output": { "content": { "type": "string", "description": "..." } },
      "sandbox": false
    }
  ]
}
```

---

### GET /api/tools/:name — 获取单个工具详情

**Example:**
```bash
curl http://localhost:8080/api/tools/chat_generate
```

---

### POST /api/tools/register — 注册外部工具

向系统注册一个外部工具。工具信息持久化到 DB、加入内存注册表，并立即使 Redis 缓存失效。

**Content-Type:** `application/json`

**Request Body (ToolManifest):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | 工具全局唯一名 |
| `description` | string | **Yes** | 工具功能描述（AI 据此判断何时使用） |
| `type` | string | **Yes** | `external`（外部 HTTP 工具） |
| `endpoint` | string | **Yes** | HTTP 端点完整 URL |
| `version` | string | No | 版本号 |
| `timeout` | int | No | 超时毫秒数（默认 30000） |
| `parameters` | object | **Yes** | 参数定义（ParamDef map） |
| `output` | object | **Yes** | 输出字段定义（ParamDef map） |
| `sandbox` | bool | No | 是否需要沙箱隔离 |
| `examples` | array | No | 输入输出示例 |

**Example:**
```bash
curl -X POST http://localhost:8080/api/tools/register \
  -H "Content-Type: application/json" \
  -d '{
    "name": "image_processor",
    "description": "处理图片文件，返回图片信息",
    "type": "external",
    "endpoint": "http://localhost:9001/image",
    "timeout": 10000,
    "parameters": {
      "image_url": { "type": "string", "description": "图片URL", "required": true }
    },
    "output": {
      "width": { "type": "string", "description": "宽度" },
      "height": { "type": "string", "description": "高度" }
    }
  }'
```

**Response** `200`:
```json
{
  "code": 200,
  "message": "tool registered successfully",
  "data": { "name": "image_processor", "type": "external" }
}
```

---

### DELETE /api/tools/:name — 注销外部工具

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/tools/image_processor
```

---

## 12. Built-in Tools

当前系统内置 14 个工具，启动时自动同步到 `tool_manifests` 表：

| Tool Name | Type | Description | Key Parameters |
|-----------|------|-------------|----------------|
| `llm_api` | builtin | Call LLM API for chat completions | `prompt` / `message` / `content` |
| `bash` | builtin | Sandboxed shell execution | `command` (string, required) |
| `python` | builtin | Python3 code execution | `source` (string, required) |
| `polisher` | builtin | Polish text via LLM | `text` (string, required), `polishType` (`title`/`description`) |
| `media_analyzer` | builtin | Analyze images/videos → tags + suggestions | `media_ids`, `file_names`, `prompt` |
| `content_generator` | builtin | Generate full content package from media | `prompt`, `platform`, `style`, `media_ids` |
| `content_checker` | builtin | Compliance check (sensitive words, ad law) | `content`, `title`, `platform` |
| `platform_adapter` | builtin | Adapt content for 7 social platforms | `source_content`, `target_platform`, `title` |
| `chat_generate` | builtin | Multi-turn conversational content generation | `messages` (array, required) |
| `chat_revise` | builtin | Revise title/desc/keywords via NL instruction | `message` (string, required) |
| `video_metadata` | builtin | Download video + extract metadata (duration, resolution, frame rate, codec, audio) | `media_id` (string, required) |
| `video_analyzer` | builtin | Extract keyframes (ffmpeg scene detection) + transcribe audio (Whisper) | `cached_video_path` or `media_id` |
| `video_copy_generator` | builtin | Generate platform-adapted short-video titles, copy, and keywords via multimodal LLM | `platform` (douyin/xiaohongshu/bilibili/kuaishou) |
| `external` | builtin | Proxy to registered external tools | `tool` (external tool name) |

---

## 13. Error Responses

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
