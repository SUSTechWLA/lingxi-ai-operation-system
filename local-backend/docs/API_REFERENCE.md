<!-- GENERATED — do not edit. Run: cd local-backend && go run ./cmd/gen-local-apidocs -->

# Tangying Local Agent API

Version: 0.1.0

## Base URL

- `http://localhost:9090` — Local desktop agent

---

### POST /api/local/artifacts

Store a local artifact

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

- **400** — Invalid payload
- **200** — Stored

---

### GET /api/local/artifacts/:id

Get a local artifact by ID

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | Yes |  |
| `projectId` | query | `string` | Yes |  |

- **404** — Not found
- **200** — Artifact data

---

### DELETE /api/local/artifacts/:id

Delete a local artifact

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | Yes |  |
| `projectId` | query | `string` | Yes |  |

- **200** — Deleted

---

### POST /api/local/biaoshu-artifacts/read

Read a local bid-writing artifact file

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

- **400** — Invalid or untrusted file path
- **200** — Artifact content

---

### GET /api/local/biaoshu-projects

List local bid-writing projects

- **200** — Project list

---

### PUT /api/local/biaoshu-projects/:runId

Create or update a local bid-writing project

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `runId` | path | `string` | Yes |  |

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

- **200** — Project stored
- **400** — Invalid payload

---

### POST /api/local/diagnostics

Create a diagnostics ZIP

**Request body:** Optional (Content-Type: `application/json`)

```json
{}
```

- **200** — Diagnostics ZIP created

---

### GET /api/local/health

Agent health status

- **200** — OK — agent is running

---

### POST /api/local/logs

Write a log entry

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

- **200** — Logged
- **400** — Invalid payload

---

### GET /api/local/model-providers

Get model provider settings

- **200** — Provider settings

---

### PUT /api/local/model-providers

Update model provider settings

**Request body:** **Required** (Content-Type: `application/json`)

```json
{}
```

- **200** — Updated
- **400** — Invalid payload

---

### GET /api/local/paths

Get data directory paths

- **200** — Paths returned

---

### DELETE /api/local/projects/:id

Delete a local project and its artifacts

**Parameters:**

| Name | In | Type | Required | Description |
|------|----|------|----------|-------------|
| `id` | path | `string` | Yes |  |

- **200** — Deleted

---

