# API Reference

Base URL: `http://localhost:8080`

All endpoints accept and return `application/json`.

---

## Health

### GET /api/health

Check service health.

**Response** `200`
```json
{
  "status": "UP",
  "service": "ai-orchestrator"
}
```

**Example**
```bash
curl http://localhost:8080/api/health
```

---

## Task Operations

### POST /api/task/create

Create a new task.

**Request Body**
```json
{
  "userId": "user-123"
}
```

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId": "test-user"}'
```

### POST /api/task/:taskId/dag

Submit a DAG (nodes + edges) to an existing task.

**Path Parameters**
| Name | Description |
|------|-------------|
| taskId | Task ID returned from create |

**Request Body**
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

**Node Fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | string | Yes | Unique node identifier within the task |
| type | string | Yes | `LLM` or `TOOL` |
| name | string | Yes | Node name (TOOL type: used as tool name for routing) |
| input | object | No | Node input parameters |
| condition | string | No | Conditional expression (e.g., `"nodeA.status == success"`) |
| maxRetry | int | No | Maximum retry count (default: 3) |
| priority | int | No | Node priority |
| workerGroup | string | No | Worker group assignment |

**Condition Format**: `"nodeId.status == success"` or `"nodeId.status == failed"`. When condition is not met, the node is automatically marked as SKIPPED.

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "DAG submitted successfully"
}
```

**Example**
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

### GET /api/task/:taskId

Get task details including all nodes and their statuses.

**Path Parameters**
| Name | Description |
|------|-------------|
| taskId | Task ID |

**Response** `200`
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

**Node Statuses**: `CREATED`, `READY`, `RUNNING`, `SUCCESS`, `FAILED`, `RETRYING`, `SKIPPED`

**Example**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3
```

### GET /api/task/:taskId/context

Get all context records for a task.

**Path Parameters**
| Name | Description |
|------|-------------|
| taskId | Task ID |

**Response** `200`
```json
[
  {
    "id": "ctx-001",
    "taskId": "20260423150000-a1b2c3",
    "nodeId": "n1",
    "type": "NODE_SUCCESS",
    "message": "Node succeeded",
    "createdAt": "2026-04-23T15:00:01Z"
  }
]
```

**Example**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3/context
```

### POST /api/task/:taskId/pause

Pause a running task.

**Path Parameters**
| Name | Description |
|------|-------------|
| taskId | Task ID |

**Request Body** (optional)
```json
{
  "reason": "Manual pause for debugging"
}
```

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "Task paused successfully"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/task/20260423150000-a1b2c3/pause \
  -H "Content-Type: application/json" \
  -d '{"reason": "debugging"}'
```

### POST /api/task/:taskId/resume

Resume a paused task.

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "message": "Task resumed successfully"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/task/20260423150000-a1b2c3/resume
```

### GET /api/task/:taskId/pause-reason

Get the reason a task was paused.

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "reason": "Manual pause for debugging"
}
```

**Example**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3/pause-reason
```

---

## Node Operations

### POST /api/node/:nodeId/success

Report a node execution as successful.

**Path Parameters**
| Name | Description |
|------|-------------|
| nodeId | Node ID |

**Request Body**
```json
{
  "result": "Query returned 42 rows",
  "data": {"rows": 42}
}
```

**Response** `200`
```json
{
  "message": "Node success recorded"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/node/n1/success \
  -H "Content-Type: application/json" \
  -d '{"result": "success", "data": {}}'
```

### POST /api/node/:nodeId/failure

Report a node execution as failed.

**Path Parameters**
| Name | Description |
|------|-------------|
| nodeId | Node ID |

**Request Body**
```json
{
  "errorMessage": "Connection timeout after 30s"
}
```

**Response** `200`
```json
{
  "message": "Node failure recorded"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/node/n1/failure \
  -H "Content-Type: application/json" \
  -d '{"errorMessage": "timeout"}'
```

### POST /api/node/:nodeId/retry

Retry a failed node. Resets node status to CREATED and re-initializes it.

**Path Parameters**
| Name | Description |
|------|-------------|
| nodeId | Node ID |

**Response** `200`
```json
{
  "nodeId": "n1",
  "message": "Node retry initiated"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/node/n1/retry
```

### GET /api/node/:nodeId/snapshot/latest

Get the latest snapshot for a node.

**Path Parameters**
| Name | Description |
|------|-------------|
| nodeId | Node ID |

**Response** `200`
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

**Response** `404` - No snapshot found
```json
{
  "error": "No snapshot found"
}
```

**Example**
```bash
curl http://localhost:8080/api/node/n1/snapshot/latest
```

### POST /api/node/:nodeId/restore

Restore a node from its latest snapshot.

**Response** `200`
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

**Example**
```bash
curl -X POST http://localhost:8080/api/node/n1/restore
```

---

## NL-Translator

### POST /api/translate

Translate a natural language prompt into a DAG structure.

**Request Body**
```json
{
  "prompt": "Query Beijing weather and generate a summary report"
}
```

**Response** `200`
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

**Example**
```bash
curl -X POST http://localhost:8080/api/translate \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Query Beijing weather and generate a summary"}'
```

### POST /api/translate/submit

Translate a natural language prompt and immediately submit the DAG to the orchestrator.

**Request Body**
```json
{
  "prompt": "Query Beijing weather and generate a summary report"
}
```

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED",
  "message": "DAG submitted successfully"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/translate/submit \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Query Beijing weather and generate a summary"}'
```

### GET /api/task/:taskId/status

Get task status via the translator module.

**Path Parameters**
| Name | Description |
|------|-------------|
| taskId | Task ID |

**Response** `200`
```json
{
  "task": {
    "id": "20260423150000-a1b2c3",
    "status": "RUNNING"
  },
  "nodes": [...]
}
```

**Example**
```bash
curl http://localhost:8080/api/task/20260423150000-a1b2c3/status
```

---

## Context

### GET /api/context/:taskId

Get all context records for a task (same as GET /api/task/:taskId/context).

**Example**
```bash
curl http://localhost:8080/api/context/20260423150000-a1b2c3
```

### GET /api/context/:taskId/node/:nodeId/snapshot/latest

Get the latest snapshot for a specific node.

**Example**
```bash
curl http://localhost:8080/api/context/20260423150000-a1b2c3/node/n1/snapshot/latest
```

### POST /api/context/:taskId/node/:nodeId/restore

Restore a node from its latest snapshot.

**Example**
```bash
curl -X POST http://localhost:8080/api/context/20260423150000-a1b2c3/node/n1/restore
```

### POST /api/context/record

Manually record a context entry.

**Request Body**
```json
{
  "taskId": "20260423150000-a1b2c3",
  "nodeId": "n1",
  "type": "CUSTOM",
  "message": "Manual context record"
}
```

**Context Types**: `TASK_CREATED`, `DAG_VALIDATED`, `DAG_SUBMITTED`, `NODE_READY`, `NODE_SCHEDULED`, `NODE_SUCCESS`, `NODE_FAILED`, `NODE_RETRY`, `NODE_SKIPPED`, `TASK_SUCCESS`, `TASK_FAILED`, `SNAPSHOT`, `CUSTOM`

**Response** `200`
```json
{
  "message": "Context recorded"
}
```

**Example**
```bash
curl -X POST http://localhost:8080/api/context/record \
  -H "Content-Type: application/json" \
  -d '{"taskId": "20260423150000-a1b2c3", "nodeId": "n1", "type": "CUSTOM", "message": "test"}'
```

---

## NL-Driven DAG Submission

### POST /api/node

Submit a complete DAG in one step (used by NL-Translator). Creates a task and submits the DAG in a single request.

**Request Body**
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

**Response** `200`
```json
{
  "taskId": "20260423150000-a1b2c3",
  "status": "CREATED",
  "message": "DAG submitted successfully"
}
```

**Example: Conditional branching**
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

---

## Built-in Tools

| Tool Name | Type | Description | Input Parameters |
|-----------|------|-------------|-----------------|
| `bash` | CUSTOM | Sandboxed shell execution | `command` (string, required) |
| `llm_api` | LLM | OpenAI-compatible chat API | `prompt` / `message` / `content` (string, required) |
| `weather` | CUSTOM | Weather query (simulated) | `city` or `location` (string, required) |

---

## Error Responses

All endpoints may return:

**400 Bad Request**
```json
{
  "error": "invalid request body"
}
```

**404 Not Found**
```json
{
  "error": "Task not found"
}
```

**500 Internal Server Error**
```json
{
  "error": "database connection failed"
}
```
