# AGENTS.md

> **架构版本：v4.0** — 视频创作 Agent，Dynamic Agent Runtime (`LLMPlanner → PlanGuard → PlanCompiler → Transient DAG`)，含质量门禁体系和 Artifact Review 闭环。

This repository is split by runtime boundary:

```text
frontend/       # React + Electron UI
local-backend/  # local desktop agent, no DB/Docker dependency
cloud-backend/  # Go AIOS Core cloud backend
```

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first for the full architecture.

## Local Runtime

The local runtime is for desktop users. It should not depend on PostgreSQL, Redis, Kafka, MinIO, Docker, or LLM API keys.

```bash
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

Local data lives under the OS application data directory and includes cache, projects, artifacts, logs, and diagnostics.

## Cloud Runtime

The cloud backend owns LLM/API integration, remote configuration, orchestration, persistence, cloud logs, and diagnostics analysis.

```bash
cd cloud-backend
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os ./cmd/tangying-ai-os
./build/tangying-ai-os
```

Cloud backend runtime docs: [cloud-backend/README.md](cloud-backend/README.md).

## Frontend

```bash
cd frontend
npm install
npm run dev
```

Build the desktop app with the cloud API base configured:

```bash
VITE_CLOUD_API_BASE=https://your-cloud.example.com/api \
TANGYING_CLOUD_API_BASE=https://your-cloud.example.com/api \
bash scripts/build-local-desktop.sh
```

## Verification

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...           # includes agentruntime unit tests
cd ../cloud-backend && go test -race ./...     # race detector
cd ../frontend && npm run build
```

## Dynamic Agent Runtime (v3.2)

The cloud backend now supports a dynamic agent path that does NOT require workflow_templates:

```text
POST /api/agent/runs  {"message": "请帮我根据端午节的来历创作一个口播知识分享视频"}
    → LLMPlanner generates AgentPlan JSON (via ModelGateway)
    → PlanGuard validates (tools, params, types, references, risk)
    → PlanCompiler inserts quality gates + approval CONTROL nodes
    → Transient DAG submitted to Orchestrator
    → Executes with quality gates blocking on failure, reviews pausing for approval
    → Artifact reviews synced to artifact_reviews table (PENDING/APPROVED/REJECTED)
```

Key agent API:
- `POST /api/agent/runs` — start a dynamic agent run
- `GET /api/agent/runs/:id` — query run status + plan
- `GET /api/agent/runs/:id/trace` — DAG execution trace
- `GET /api/agent/runs/:id/reviews` — list pending reviews
- `POST /api/agent/runs/:id/reviews/:rid/approve|reject` — approve/reject

## Boundary Rules

- Do not add database, Docker, Kafka, Redis, or MinIO dependencies to `local-backend`.
- Do not store LLM API keys in the local desktop package.
- Local logs must be uploaded only after user action/authorization.
- Keep cloud business orchestration in `cloud-backend`.

## API Documentation (OpenAPI)

API docs are generated from the authoritative OpenAPI spec — not hand-written.
The spec lives alongside the routes in code and is the single source of truth.

### Live Swagger UIs

- **Cloud backend:** start the server and open `http://localhost:8080/docs`
  - Raw spec: `http://localhost:8080/openapi.json`
- **Local agent:** start the agent and open `http://localhost:18080/api/local/docs`
  - Raw spec: `http://localhost:18080/api/local/openapi.json`

### Regenerating docs

```bash
# Cloud backend (markdown + TypeScript types)
cd cloud-backend && make gen-docs

# Local agent (markdown only)
cd local-backend && go run ./cmd/gen-local-apidocs
```

### Keeping docs in sync

When you add/change/remove an API endpoint:

1. Update the handler's `RegisterRoutes` (Gin) or `routes()` (net/http).
2. Update `cloud-backend/internal/core/apispec/cloud_spec.go` (cloud) or
   `local-backend/internal/localagent/openapi.go` (local) to match.
3. Run `make gen-docs` to regenerate the markdown and TypeScript types.
4. Run `make api-docs-check` to verify nothing drifted (add to pre-commit/CI).

The generated markdown files (`cloud-backend/docs/API_REFERENCE.md`,
`local-backend/docs/API_REFERENCE.md`) and the generated TypeScript file
(`frontend/src/utils/api-types.generated.ts`) carry a `DO NOT EDIT` banner —
they are always overwritten by the generator. The frontend's hand-crafted
`frontend/src/utils/types.ts` remains the authoritative TS source and should
be manually reconciled when the generated file changes.
