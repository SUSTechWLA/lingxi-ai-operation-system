# AGENTS.md

This file provides guidance to coding agents (Codex / Claude / ZCode) when working with code in this repository.

> 📖 **For the full product architecture, every module, every API, and deployment details, read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) first.** This file is a quick-start reference; ARCHITECTURE.md is the authoritative, source-verified document.

## Quick Reference

### Service
- Single Go binary on port 8080 (all modules combined in a modular monolith)
- Go module: `github.com/tangying-ai/aios-core`

### Core Commands

#### Start backend + infrastructure
```bash
# 1. Copy and configure environment
cd aios-core && cp .env.example .env
# Edit .env — add your OPENAI_API_KEY

# 2. Start Docker infrastructure + backend
./aios-core/scripts/startup.sh    # One-click (infra → build backend → run backend)
```

#### Build sandbox (Rust)
```bash
# Prerequisites: Rust toolchain (install via: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh)

make sandbox-build       # Build Rust sandbox service
./build/tangying-sandbox & # Start sandbox gRPC server on :50051

# To enable sandbox for tool execution:
# Set SANDBOX_ENABLED=true in .env, restart backend
```

#### Manual backend startup
```bash
# Terminal 1: Infrastructure
cd aios-core && docker compose up -d

# Terminal 2: Backend (port 8080)
cd aios-core && go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./aios-core/build/tangying-ai-os
```

#### Frontend (development or Electron build)
```bash
cd frontend
npm install

# Dev server (hot reload, proxies /api to :8080)
npm run dev            # http://localhost:3000

# Electron production build (uses dist/ directly, APIs call localhost:8080)
npm run build          # Build Vite
npm run electron:build # Build .dmg/.exe via electron-builder
```

#### Development
```bash
make build    # Build binary
make run      # Build + run
make test     # Run tests
make tidy     # Tidy dependencies
make fmt      # Format code
```

#### Test APIs
```bash
./aios-core/scripts/test-apis.sh
curl http://localhost:8080/api/health
```

## Architecture Overview

Go modular monolith on a single process (:8080, Gin). The codebase is split into two layers under `aios-core/internal/`:

1. **`internal/core/` — Generic engine** (business-agnostic). Provides DAG orchestration, tool execution, workflow templates, the skill runtime, the model gateway, artifact versioning, and infra (event bus / outbox / media / translator / context / config).
2. **`internal/agents/` — Business agents** (domain logic built on top of `core`). One package per business line: `video`, `bid`, `chat`, `publish`.

> This layering means "add a new business line" ≈ "add an agent package + a skill package", without touching the engine. See ARCHITECTURE.md §2–§5.

### Communication Flow
- In-process Go function calls between modules
- Event-driven async coordination via Redpanda (Kafka-compatible) through the **Outbox pattern** (events written to DB first, relayed by a background goroutine — no event loss)
- Persistence: PostgreSQL (pgx); cache/session: Redis (go-redis); objects: MinIO

## Project Structure (current)

```
aios-core/
├── cmd/
│   ├── tangying-ai-os/main.go     # Entry point, wiring, graceful shutdown
│   └── skill2workflow/main.go     # CLI: compile a Skill package → workflow DAG
├── internal/
│   ├── core/                      # GENERIC ENGINE
│   │   ├── orchestrator/          # DAG scheduling (service: state machine, dependency checker, scheduler, retry, DAG validator, task control)
│   │   ├── workflow/              # Workflow templates + runs + skill_compiler (CompileSkillToDAG)
│   │   ├── skillruntime/          # Skill package loader/registry + LLM skill router (manifest.go, registry.go, handler.go, router.go)
│   │   ├── worker/                # Tool execution (tool interface, 14+ builtin tools, executor: direct/sandbox, node_executor)
│   │   ├── modelgateway/          # Unified model gateway (fingerprint cache + retry + providers, incl. fake)
│   │   ├── artifact/              # Versioned artifacts (model, repository, service, materializer, handler)
│   │   ├── localrunner/           # Electron local task protocol (reserved, tables + service, no HTTP yet)
│   │   ├── translator/            # NL → DAG via LLM
│   │   ├── context/               # Audit/snapshot from Kafka events
│   │   ├── media/                 # MinIO-backed media assets
│   │   ├── model/                 # Data models + repository (pgx)
│   │   ├── outbox/                # Outbox relay (SaveEvent + Relay goroutine)
│   │   ├── eventbus/              # sarama Kafka producer/consumer + Topic constants
│   │   ├── config/                # Viper config (.env), VideoConfig feature flags
│   │   ├── database/              # pgx pool + RunMigrations (CREATE IF NOT EXISTS)
│   │   ├── logger/                # Zap (dev/prod)
│   │   └── common/                # llmutil, jsonx, metadata
│   └── agents/                    # BUSINESS AGENTS
│       ├── video/                 # Video creation (model, repository, service, handler) — Project → Run
│       ├── bid/                   # Tender/bid doc generation (project → chapter → approve → export)
│       ├── chat/                  # AI conversational assistant (session → plan DAG → execute → extract)
│       └── publish/               # Content publish + AI generate/polish + tools + trace
├── skills/                        # Skill packages (6): create-opinion-videos, aigc-shot-video,
│                                  #   video-creator, film-shot-reconstruction, voice-post-production, voice-visual-video
├── deploy/                        # Cloud deployment: docker-compose.cloud.yml, nginx.conf, .env.cloud.example, Dockerfile
└── sandbox/                       # Rust gRPC sandbox service (resource isolation via setrlimit)
frontend/                          # React + TS + Tailwind + Zustand + Vite
├── src/
│   ├── pages/                     # CreatorWorkbenchPage (main), DesktopPage; PublishPage is legacy/unused
│   ├── components/                # Sidebar + page components
│   ├── services/api.ts            # All /api calls
│   └── stores/appStore.ts         # Zustand (publish-related state; creator page self-manages state)
electron/                          # main.js, preload.js, package.json (electron-builder)
docs/ARCHITECTURE.md               # ★ Authoritative architecture doc
```

## Key Concepts

### Layering rule
`internal/core` must NOT import from `internal/agents`. Agents depend on core; core depends only on itself and stdlib/3rd-party. This keeps the engine reusable across business lines. (Example: `skillruntime` injects a `CompileFunc` from `workflow` to avoid an import cycle — see `skillruntime/handler.go`.)

### DAG Task Graph
- **Task** = directed acyclic graph. **Node** = one operation (TOOL / LLM / CONTROL). **Edge** = dependency.
- Nodes carry an optional `condition` for conditional branching.
- Node status: `CREATED → READY → RUNNING → SUCCESS | FAILED` (+ `SKIPPED`, `RETRYING`). A parent is satisfied when it is `SUCCESS` **or** `SKIPPED`.
- **CONTROL nodes** are human-approval gates: becoming READY auto-pauses the task; `POST /api/node/:id/success` resumes it.

### Skill → Workflow compilation
A `skill.yaml` compiles to a DAG automatically (`workflow/skill_compiler.go`):
- normal stage → 1 TOOL node
- `approval_required: true` → TOOL exec node + CONTROL node chain
- `optional: true` → an extra `_skip` CONTROL bypass
Skills are loaded at startup (`skillruntime/registry.go`), and when `VIDEO_CREATION_ENABLED=true` each healthy skill is auto-registered as a workflow template.

### Tool name routing
- Explicit: `payload["tool"]` wins
- TOOL type → `payload["name"]`
- LLM type → always `llm_api`

### Idempotency
`idempotencyKey = taskId + "-" + nodeId`, used as the Kafka message key; state transitions check "already in target state → skip".

### Event topics (current)
| Topic | Producer | Consumer |
|-------|----------|----------|
| `ai.node.ready` | StateService | Worker (NodeExecutor) |
| `ai.node.result` | Worker (with Status: SUCCESS/FAILED) | Orchestrator (StateMachine + DependencyChecker) + Context |
| `ai.progress` | Worker (heartbeat/progress/checkpoint) | ProgressConsumer |

> ⚠️ The older `ai.node.executed` / `ai.node.failed` topics have been merged into `ai.node.result` (status field). Update any stale references.

### Outbox pattern
`SaveEvent()` writes to the `outbox` table; a `Relay` goroutine (100ms ticker) publishes to Kafka and deletes on success.

## API Endpoints (summary)

Standard response: `{"code": 200, "message": "success", "data": {...}}`. **Full list with params/bodies: see [ARCHITECTURE.md §8](docs/ARCHITECTURE.md#8-完整-api-接口清单).**

| Group | Base route | Notes |
|-------|-----------|-------|
| Skill runtime | `/api/skills`, `/api/skills/route`, `/api/skills/:name/:version/compile` | NL routing + catalog + compile |
| Workflow | `/api/workflows` | Template CRUD + instantiate |
| Video projects | `/api/video-projects`, `/api/video-projects/:id/workflow-runs`, `/api/video-projects/:id/artifacts` | Requires `VIDEO_CREATION_ENABLED=true` |
| Artifacts | `/api/artifacts/:id` (+ `/content`, `/history`, `/revise`) | Versioned, with materialize |
| Orchestrator | `/api/task/*`, `/api/node/:id/success|failure` | Task + manual approval |
| Publish / AI | `/api/publish`, `/api/ai/generate(-from-media)`, `/api/ai/polish(/submit|/result)` | |
| AI chat | `/api/chat/sessions/*` | ⚠️ NOT `/api/skill/dialog/...` (old path) |
| Tools | `/api/tools(/:name|/register)` | Builtin + external registry |
| Media | `/api/media/*` | Upload/list/get/tags |
| Bid | `/api/bid/projects/*`, `/api/bid/templates` | Tender doc generation |
| Trace | `/api/trace/recent`, `/api/trace/:taskId` | Audit |
| Health | `/api/health` | Docker/Electron liveness |

## Infrastructure

Docker-based local development:
- PostgreSQL 16 — primary database
- Redis 7 — cache/session
- Redpanda — event streaming (Kafka compatible)
- MinIO — object storage
- Qdrant — vector database (provisioned, not yet wired)

```bash
cd aios-core && docker compose up -d    # Start
docker compose down                      # Stop
docker compose logs -f                   # View logs
```

## Environment Configuration

```bash
cd aios-core && cp .env.example .env
```

Key variables (see `.env.example` for all):
- `OPENAI_API_KEY` / `OPENAI_BASE_URL` — LLM access (required)
- `VIDEO_CREATION_ENABLED` — enable video creation agent + skill auto-registration
- `MODEL_PROVIDER_MODE` — `fake` (test, no API key needed) or `real`
- `SKILL_ROOT` — skill package root (default `skills`)
- `SANDBOX_ENABLED` — route BuildableTools through the Rust sandbox
- `LOCAL_RUNNER_ENABLED` — Electron local runner protocol

## Testing

```bash
go test ./...                # All Go tests
go test -race ./...          # With race detector
go test ./internal/core/...  # Core engine only
./aios-core/scripts/test-apis.sh   # API integration tests
```

Existing test coverage: artifact, skillruntime, modelgateway, localrunner, config, video model/service, workflow compiler/run, skill router, retry_policy, dag_validator, node executor, video creation external tools. Repository (DB) and HTTP handler integration tests still need adding.

## Development Workflow

1. Ensure Docker is running and infrastructure is up (`docker compose up -d`)
2. `make run` to build and start
3. Follow existing Go patterns; respect the core↔agents layering rule
4. Write tests for new functionality
5. `go build ./...` and `./aios-core/scripts/test-apis.sh` before submitting
6. **If code changes contradict ARCHITECTURE.md, update the doc** (it is source-verified)

## Adding a new business line

1. Write `skills/{name}/1.0.0/skill.yaml` + `stages/*.md` (optional `schemas/*.json`)
2. (Optional) Add `internal/agents/{name}/` (model/repository/service/handler) reusing orchestrator + workflow
3. Register the handler in `cmd/tangying-ai-os/main.go` (feature-gate if needed)
4. On startup the skill auto-loads → compiles → registers as a workflow template; the LLM skill router picks it up automatically (if healthy + correct category)

## Common Issues

- **NL-Translator 503**: Missing/invalid `OPENAI_API_KEY`
- **Video routes 404**: `VIDEO_CREATION_ENABLED` not set to `true`
- **Port conflict**: ensure 8080 is free
- **Docker not running**: start Docker Desktop first
- **Old consumer group incompatibility** (migrated from Java version): `rpk group delete <group-name>`
- **Go version**: requires Go 1.25+ (`brew install go`)
