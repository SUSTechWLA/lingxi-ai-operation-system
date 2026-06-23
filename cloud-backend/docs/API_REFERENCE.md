<!-- GENERATED — do not edit.
     Regenerate: cd cloud-backend && make gen-docs
     Source of truth: internal/core/apispec/cloud_spec.go
-->

# Tangying AIOS Cloud API

Version: 0.1.0

## Base URLs

- `http://localhost:8080` — Local development server

## Table of Contents

1. [AI](#1-ai)
2. [Agent Runs](#2-agent-runs)
3. [Artifacts](#3-artifacts)
4. [Auth](#4-auth)
5. [Bid](#5-bid)
6. [Chat](#6-chat)
7. [Context](#7-context)
8. [Health](#8-health)
9. [Media](#9-media)
10. [Node](#10-node)
11. [Orchestrator](#11-orchestrator)
12. [Publish](#12-publish)
13. [Skill Capabilities](#13-skill-capabilities)
14. [Skills](#14-skills)
15. [Stages](#15-stages)
16. [Tools](#16-tools)
17. [Trace](#17-trace)
18. [Translate](#18-translate)
19. [Video Projects](#19-video-projects)
20. [Workflow Runs](#20-workflow-runs)
21. [Workflows](#21-workflows)

---

## 1. AI

### POST /api/ai/generate

Generate content from text prompt

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "prompt": "string",
}
```

**Responses:**

- **200** — Generated content (JSON)

---

### POST /api/ai/generate-from-media

Generate content from media files + prompt

**Request body:** Optional (Content-Type: `multipart/form-data`)

```json
{
  "prompt": "string",
}
```

**Responses:**

- **200** — Generated content (JSON)

---

### POST /api/ai/polish

Polish text via AI (synchronous)

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "text": "string",
  "type": "string",
}
```

**Responses:**

- **200** — Polished result (JSON)

---

### GET /api/ai/polish/result

Query async polish result

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | query | `string` | **Yes** | Task ID from submit response |
| `nodeId` | query | `string` | No | Node ID (optional) |

**Responses:**

- **200** — Polish result or status (JSON)

---

### POST /api/ai/polish/submit

Submit async polish task

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "text": "string",
  "type": "string",
}
```

**Responses:**

- **200** — Task submitted (JSON)

---

## 2. Agent Runs

### POST /api/agent/runs

Start a dynamic agent run from natural language

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/AgentStartRunRequest" }
```

**Responses:**

- **200** — Agent run started (JSON)
- **400** — Invalid plan or request (JSON)

---

### GET /api/agent/runs/:runId

Get dynamic agent run state

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | **Yes** | Agent run identifier |

**Responses:**

- **200** — Agent run detail (JSON)
- **404** — Not found (JSON)

---

### GET /api/agent/runs/:runId/reviews

List review gates for a dynamic agent run

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | **Yes** | Agent run identifier |

**Responses:**

- **200** — Review gates (JSON)
- **404** — Not found (JSON)

---

### POST /api/agent/runs/:runId/reviews/:reviewId/approve

Approve a dynamic agent review gate

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | **Yes** | Agent run identifier |
| `reviewId` | path | `string` | **Yes** | Review node identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Approved (JSON)

---

### POST /api/agent/runs/:runId/reviews/:reviewId/reject

Reject a dynamic agent review gate

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | **Yes** | Agent run identifier |
| `reviewId` | path | `string` | **Yes** | Review node identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Rejected (JSON)

---

### GET /api/agent/runs/:runId/trace

Get dynamic agent task trace

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | **Yes** | Agent run identifier |

**Responses:**

- **200** — Agent task trace (JSON)
- **404** — Not found (JSON)

---

## 3. Artifacts

### GET /api/artifacts/:id

Get artifact by ID

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Artifact identifier |

**Responses:**

- **200** — Artifact (JSON)
- **404** — Not found (JSON)

---

### GET /api/artifacts/:id/content

Get artifact content

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Artifact identifier |

**Responses:**

- **200** — Content + metadata (JSON)
- **404** — Not found (JSON)

---

### GET /api/artifacts/:id/history

Get artifact revision history

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Artifact identifier |

**Responses:**

- **200** — Version history (JSON)

---

### POST /api/artifacts/:id/revise

Create an artifact revision

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Artifact identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "message": "string",
}
```

**Responses:**

- **200** — Revised (JSON)

---

### GET /api/video-projects/:id/artifacts

List project artifacts

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Artifacts list (JSON)

---

## 4. Auth

### POST /api/auth/login

Log in with email and password

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/AuthLoginRequest" }
```

**Responses:**

- **200** — Authenticated (JSON)
- **401** — Invalid credentials (JSON)

---

### POST /api/auth/logout

Revoke the current refresh token

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/AuthLogoutRequest" }
```

**Responses:**

- **200** — Logged out (JSON)
- **401** — Missing or invalid access token (JSON)

---

### GET /api/auth/me

Get the current authenticated user

**Responses:**

- **200** — Current user (JSON)
- **401** — Missing or invalid access token (JSON)

---

### POST /api/auth/refresh

Rotate refresh token and return new tokens

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/AuthRefreshRequest" }
```

**Responses:**

- **200** — Refreshed (JSON)
- **401** — Invalid refresh token (JSON)

---

### POST /api/auth/register

Register a user with email and password

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/AuthRegisterRequest" }
```

**Responses:**

- **200** — Registered and authenticated (JSON)
- **400** — Invalid request (JSON)
- **409** — Email already registered (JSON)

---

## 5. Bid

### GET /api/bid/projects

List bid projects

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `status` | query | `string` | No | Filter by status |
| `userId` | query | `string` | No | Filter by user |
| `offset` | query | `integer` | No | Pagination offset |
| `limit` | query | `integer` | No | Page size |

**Responses:**

- **200** — Projects list (JSON)

---

### POST /api/bid/projects

Create a bid project

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Created (JSON)

---

### GET /api/bid/projects/:id

Get project detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Project + chapters (JSON)
- **404** — Not found (JSON)

---

### PUT /api/bid/projects/:id

Update a project

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Updated (JSON)

---

### DELETE /api/bid/projects/:id

Delete a project

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Deleted (JSON)

---

### POST /api/bid/projects/:id/chapters/:chId/approve

Approve a chapter

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `chId` | path | `string` | **Yes** | Chapter identifier |

**Responses:**

- **200** — Approved (JSON)

---

### POST /api/bid/projects/:id/chapters/:chId/regenerate

Trigger chapter regeneration

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `chId` | path | `string` | **Yes** | Chapter identifier |

**Responses:**

- **200** — Regeneration triggered (JSON)

---

### POST /api/bid/projects/:id/chapters/:chId/reject

Reject a chapter

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `chId` | path | `string` | **Yes** | Chapter identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "comment": "string",
}
```

**Responses:**

- **200** — Rejected (JSON)

---

### POST /api/bid/projects/:id/export

Export project to document

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{
  "format": "string",
}
```

**Responses:**

- **200** — Export started (JSON)

---

### GET /api/bid/projects/:id/export/status

Get export status

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Export status (JSON)

---

### POST /api/bid/projects/:id/pause

Pause generation

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{
  "reason": "string",
}
```

**Responses:**

- **200** — Paused (JSON)

---

### GET /api/bid/projects/:id/progress

Get generation progress

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Progress data (JSON)

---

### POST /api/bid/projects/:id/resume

Resume generation

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Resumed (JSON)

---

### POST /api/bid/projects/:id/start

Start bid generation

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Started (JSON)

---

### GET /api/bid/projects/:id/trace

Get project trace redirect

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Trace URL (JSON)

---

### POST /api/bid/projects/:id/upload-tender

Upload tender document

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** **Required** (Content-Type: `multipart/form-data`)

```json
{
  "file": "string",
}
```

**Responses:**

- **200** — Uploaded (JSON)

---

### GET /api/bid/templates

List bid templates

**Responses:**

- **200** — Templates list (JSON)

---

## 6. Chat

### GET /api/chat/sessions/:session_id

Get session state

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `session_id` | path | `string` | **Yes** | Session identifier |

**Responses:**

- **200** — Session state (JSON)
- **404** — Not found (JSON)

---

### POST /api/chat/sessions/:session_id/chat

Send a chat message

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `session_id` | path | `string` | **Yes** | Session identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "message": "string",
}
```

**Responses:**

- **200** — Assistant reply (JSON)
- **410** — Session terminated (JSON)

---

### GET /api/chat/sessions/:session_id/progress

Get session progress

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `session_id` | path | `string` | **Yes** | Session identifier |

**Responses:**

- **200** — Progress state (JSON)

---

### POST /api/chat/sessions/:session_id/terminate

Terminate session

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `session_id` | path | `string` | **Yes** | Session identifier |

**Responses:**

- **200** — Terminated (JSON)

---

### POST /api/chat/sessions/create

Create a new chat session

**Request body:** Optional (Content-Type: `application/json`)

```json
{
  "description": "string",
  "keywords": ["string"],
  "media_count": 0,
  "media_ids": ["string"],
  "platforms": ["string"],
  "title": "string",
  ...
}
```

**Responses:**

- **200** — Session created (JSON)

---

## 7. Context

### GET /api/context/:taskId

Get context records for task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Context records (JSON)

---

### POST /api/context/:taskId/node/:nodeId/restore

Restore node

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Responses:**

- **200** — Restored (JSON)

---

### GET /api/context/:taskId/node/:nodeId/snapshot/latest

Get node snapshot

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Responses:**

- **200** — Snapshot found (JSON)

---

### POST /api/context/record

Record a context event manually

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "message": "string",
  "nodeId": "string",
  "taskId": "string",
  "type": "string",
}
```

**Responses:**

- **200** — Recorded (JSON)

---

## 8. Health

### GET /api/health

Liveness check

**Responses:**

- **200** — Service is up (JSON)

---

### GET /api/health/ready

Readiness check with dependencies

**Responses:**

- **200** — All dependencies healthy (JSON)
- **503** — One or more dependency down (JSON)

---

## 9. Media

### GET /api/media/:id

Get media asset by ID

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Media identifier |

**Responses:**

- **200** — Media asset (JSON)
- **404** — Not found (JSON)

---

### PUT /api/media/:id/tags

Update media tags

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Media identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "tags": ["string"],
}
```

**Responses:**

- **200** — Tags updated (JSON)

---

### GET /api/media/list

List media assets

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `userId` | query | `string` | No | User identifier |
| `offset` | query | `integer` | No | Pagination offset |
| `limit` | query | `integer` | No | Page size |
| `tag` | query | `string` | No | Filter by tag |

**Responses:**

- **200** — Media list (JSON)

---

### POST /api/media/upload

Upload media files

**Request body:** Optional (Content-Type: `multipart/form-data`)

```json
{
  "userId": "string",
}
```

**Responses:**

- **200** — Uploaded (JSON)
- **400** — No files (JSON)

---

## 10. Node

### POST /api/node

Submit DAG in one step (create + submit)

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/DAGRequest" }
```

**Responses:**

- **200** — DAG submitted (JSON)

---

### POST /api/node/:nodeId/failure

Report node execution failure

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "errorMessage": "string",
}
```

**Responses:**

- **200** — Recorded (JSON)

---

### POST /api/node/:nodeId/restore

Restore node from snapshot

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Responses:**

- **200** — Restored (JSON)

---

### POST /api/node/:nodeId/retry

Retry a failed node

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Responses:**

- **200** — Retry initiated (JSON)

---

### GET /api/node/:nodeId/snapshot/latest

Get latest node snapshot

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Responses:**

- **200** — Snapshot found (JSON)
- **404** — No snapshot (JSON)

---

### POST /api/node/:nodeId/success

Report node execution success

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `nodeId` | path | `string` | **Yes** | Node identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Recorded (JSON)

---

## 11. Orchestrator

### GET /api/task/:taskId

Get task with node details

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Task + nodes (JSON)

---

### GET /api/task/:taskId/context

Get context records for task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Context records (JSON)

---

### POST /api/task/:taskId/dag

Submit a DAG to a task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{ "$ref": "#/components/schemas/DAGRequest" }
```

**Responses:**

- **200** — DAG submitted (JSON)

---

### POST /api/task/:taskId/fail

Immediately fail a task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Failed (JSON)

---

### POST /api/task/:taskId/pause

Pause a running task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{
  "reason": "string",
}
```

**Responses:**

- **200** — Paused (JSON)

---

### GET /api/task/:taskId/pause-reason

Get task pause reason

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Pause reason (JSON)

---

### GET /api/task/:taskId/progress

Get task execution progress

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Task progress (JSON)

---

### POST /api/task/:taskId/resume

Resume a paused task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Resumed (JSON)

---

### POST /api/task/create

Create a new empty task

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Task created (JSON)

---

## 12. Publish

### POST /api/publish

Submit content for multi-platform publishing

**Request body:** Optional (Content-Type: `multipart/form-data`)

```json
{
  "content_type": "string",
  "description": "string",
  "keywords": "string",
  "platforms": "string",
  "title": "string",
}
```

**Responses:**

- **200** — Task created (JSON)
- **400** — Missing required fields (JSON)

---

## 13. Skill Capabilities

### GET /api/skill-capabilities

List agent capability packages

**Responses:**

- **200** — Capability packages (JSON)

---

### GET /api/skill-capabilities/:id

Get capability package detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Capability package identifier |

**Responses:**

- **200** — Capability package (JSON)
- **404** — Not found (JSON)

---

## 14. Skills

### GET /api/skills

List all loaded skills

**Responses:**

- **200** — Skills list (JSON)

---

### GET /api/skills/:name/:version

Get skill detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `name` | path | `string` | **Yes** | Skill name |
| `version` | path | `string` | **Yes** | Skill version |

**Responses:**

- **200** — Skill detail (JSON)
- **404** — Not found (JSON)

---

### POST /api/skills/:name/:version/compile

Compile skill to DAG

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `name` | path | `string` | **Yes** | Skill name |
| `version` | path | `string` | **Yes** | Skill version |

**Responses:**

- **200** — Compiled DAG (JSON)

---

### GET /api/skills/catalog

Get skill catalog

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `includeHidden` | query | `boolean` | No | Include hidden skills |

**Responses:**

- **200** — Catalog (JSON)

---

### POST /api/skills/route

Route a brief to a skill

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "brief": "string",
}
```

**Responses:**

- **200** — Route result (JSON)

---

## 15. Stages

### POST /api/video-projects/:id/stages/:stage/approve

Approve a stage

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `stage` | path | `string` | **Yes** | Stage name |

**Request body:** Optional (Content-Type: `application/json`)

```json
{
  "comment": "string",
  "output": {},
  "runId": "string",
}
```

**Responses:**

- **200** — Approved (JSON)
- **409** — Stage not ready (JSON)

---

## 16. Tools

### GET /api/tools

List all registered tools

**Responses:**

- **200** — Tool manifests (JSON)

---

### GET /api/tools/:name

Get tool details

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `name` | path | `string` | **Yes** | Tool name |

**Responses:**

- **200** — Tool manifest (JSON)
- **404** — Tool not found (JSON)

---

### DELETE /api/tools/:name

Deregister an external tool

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `name` | path | `string` | **Yes** | Tool name |

**Responses:**

- **200** — Deregistered (JSON)
- **404** — Not found (JSON)

---

### POST /api/tools/register

Register an external tool

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Registered (JSON)

---

## 17. Trace

### GET /api/trace/:taskId

Get task trace by ID

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Trace data (JSON)
- **404** — Task not found (JSON)

---

### GET /api/trace/recent

Get most recent task trace

**Responses:**

- **200** — Trace data (JSON)
- **404** — No tasks found (JSON)

---

## 18. Translate

### GET /api/task/:taskId/status

Query translate task status

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `taskId` | path | `string` | **Yes** | Task identifier |

**Responses:**

- **200** — Task status (JSON)

---

### POST /api/translate

Translate NL prompt to DAG

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "prompt": "string",
}
```

**Responses:**

- **200** — DAG structure (JSON)

---

### POST /api/translate/submit

Translate NL prompt and submit DAG

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "prompt": "string",
}
```

**Responses:**

- **200** — Result (JSON)

---

## 19. Video Projects

### GET /api/video-projects

List video projects

**Responses:**

- **200** — Projects list (JSON)

---

### POST /api/video-projects

Create a video project

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Created (JSON)

---

### GET /api/video-projects/:id

Get project detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Project (JSON)
- **404** — Not found (JSON)

---

### PATCH /api/video-projects/:id

Update a project

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Updated (JSON)

---

### DELETE /api/video-projects/:id

Archive a project (soft delete)

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Responses:**

- **200** — Archived (JSON)

---

## 20. Workflow Runs

### POST /api/video-projects/:id/workflow-runs

Create a workflow run

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{
  "input": {},
  "templateId": "string",
  "templateVersion": "string",
}
```

**Responses:**

- **200** — Run created (JSON)

---

### GET /api/video-projects/:id/workflow-runs/:rid

Get run detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `rid` | path | `string` | **Yes** | Run identifier |

**Responses:**

- **200** — Run detail (JSON)
- **404** — Not found (JSON)

---

### POST /api/video-projects/:id/workflow-runs/:rid/cancel

Cancel a run

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `rid` | path | `string` | **Yes** | Run identifier |

**Responses:**

- **200** — Cancelled (JSON)

---

### POST /api/video-projects/:id/workflow-runs/:rid/pause

Pause a run

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Project identifier |
| `rid` | path | `string` | **Yes** | Run identifier |

**Responses:**

- **200** — Paused (JSON)

---

## 21. Workflows

### GET /api/workflows

List workflow templates

**Responses:**

- **200** — Templates (JSON)

---

### POST /api/workflows

Create a workflow template

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Created (JSON)

---

### GET /api/workflows/:id

Get template detail

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Template identifier |

**Responses:**

- **200** — Template (JSON)
- **404** — Not found (JSON)

---

### PUT /api/workflows/:id

Update a template

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Template identifier |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Updated (JSON)

---

### DELETE /api/workflows/:id

Delete a template

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Template identifier |

**Responses:**

- **200** — Deleted (JSON)

---

### POST /api/workflows/:id/instantiate

Instantiate template → task

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | **Yes** | Template identifier |

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

**Responses:**

- **200** — Task created (JSON)

---

