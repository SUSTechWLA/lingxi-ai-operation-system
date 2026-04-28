# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Service
- Single Go binary on port 8080 (all modules combined in a modular monolith)

### Core Commands

#### Start backend + infrastructure
```bash
# 1. Copy and configure environment
cp .env.example .env
# Edit .env — add your OPENAI_API_KEY

# 2. Start Docker infrastructure + backend
./scripts/startup.sh    # One-click (infra → build backend → run backend)
```

#### Build sandbox (Rust)
```bash
# Prerequisites: Rust toolchain (install via: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh)

make sandbox-build       # Build Rust sandbox service
./build/lingxi-sandbox & # Start sandbox gRPC server on :50051

# To enable sandbox for tool execution:
# Set SANDBOX_ENABLED=true in .env, restart backend
```

#### Manual backend startup
```bash
# Terminal 1: Infrastructure
docker compose up -d

# Terminal 2: Backend (port 8080)
go build -o build/lingxi-ai-os cmd/lingxi-ai-os/main.go
./build/lingxi-ai-os
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
./scripts/test-apis.sh
curl http://localhost:8080/api/health
```

## Architecture Overview

Go modular monolith — all modules run in a single process on port 8080:

1. **NL-Translator** - Converts natural language prompts into executable DAG task graphs using LLMs
2. **Orchestrator** - Core task scheduler managing task lifecycle, dependencies, and event distribution
3. **Context** - Persists task history and provides audit/snapshot capabilities
4. **Worker** - Executes operations via plugin-based tool architecture
5. **Publish** - Frontend-facing API for content creation, AI generation/polish, and multi-platform publishing

### Communication Flow
- Modules communicate via internal Go function calls (same process)
- Event-driven architecture using Redpanda (Kafka-compatible) for async coordination
- Outbox pattern: events written to DB first, relayed to Kafka by background goroutine (no event loss)
- Persistence with PostgreSQL (pgx), caching with Redis (go-redis)

## Module Details

### internal/orchestrator
Core task scheduling engine.

Key components:
- `OrchestratorService` - Task creation, DAG submission, validation
- `StateService` - Unified state convergence for tasks and nodes; includes `TryMakeReady` for immediate dependency check
- `StateMachine` - Handles node success/failure, retry logic; publishes events via outbox
- `DependencyChecker` - Event-driven: checks downstream dependencies after node execution; evaluates conditions before making child nodes ready
- `RetryPolicy` - Exponential backoff (1s -> 2s -> 4s -> ... -> 60s max)
- `Scheduler` - Fallback recovery: 30s ticker, only processes CREATED nodes stuck for >1 minute
- `TaskExecutionControl` - Pause/resume/retry operations; persists pause reason
- `DAGValidator` - Cycle detection, duplicate node checks

### internal/translator
Natural language to DAG translation via LLM.

Key components:
- `NlToDagService` - Calls OpenAI API with system prompt to generate DAG from natural language
- `TranslateAndSubmit` - Translates then submits DAG to orchestrator in one step

### internal/context
Audit and context management.

Key components:
- `ContextService` - Record context, get task history, snapshot/restore nodes
- `HandleEvent` - Auto-record context from Kafka events (ai.node.executed, ai.node.failed, etc.)

### internal/worker
Tool execution gateway with sandbox support.

Architecture layers:
- **Tool layer** (`internal/worker/tool/`) — Plugin interface + registry
- **Executor layer** (`internal/worker/executor/`) — Execution backends (direct / sandbox)

#### Tool Interface
- `Tool` — Base interface: Name, Description, Type, Execute, ValidateParameters
- `BuildableTool` — Tool that produces an `ExecutionRequest` (for executor routing to sandbox)
- `ExecutableTool` — Tool that self-executes inline (for API-call-style tools)
- `ToolRegistry` — Plugin registration and lookup by name

#### Built-in Tools (`internal/worker/tool/builtin/`)
- `BashTool` — Sandboxed shell: command whitelist + dangerous pattern filter + `/tmp/lingxi-sandbox` workdir. Implements `BuildableTool`.
- `PythonTool` — python3 -c execution with resource limits. Implements `BuildableTool`.
- `LlmApiTool` — OpenAI chat/completions API calls. Implements `ExecutableTool`.
- `PolisherTool` — Text polish for social media titles/descriptions via LLM. Implements `ExecutableTool`.
- `MediaAnalyzerTool` — Media analysis: extracts tags, suggestions, and summaries from uploaded images/videos via LLM. Implements `ExecutableTool`.
- `ContentGeneratorTool` — Full content package generation based on media analysis, platform, and style keywords. Implements `ExecutableTool`.
- `ContentCheckerTool` — Content compliance check: sensitive words, advertising law violations, platform-specific rules. Implements `ExecutableTool`.
- `PlatformAdapterTool` — Cross-platform content adaptation: adjusts tone, format, and length for 7 social media platforms. Implements `ExecutableTool`.

#### Executor Layer (`internal/worker/executor/`)
- `Executor` interface — `Execute(ctx, ExecutionRequest) (ExecutionResult, error)`
- `DirectExecutor` — Runs subprocess locally with temp workdir and env injection
- `SandboxExecutor` — gRPC client to Rust sandbox service (`sandbox/`). Implements resource isolation (memory, CPU, disk, PID limits via setrlimit), timeout enforcement, and temp directory cleanup. When `SANDBOX_ENABLED=true`, `BuildableTool` requests route through this instead of `DirectExecutor`
- `sandboxpb/` — Generated Go protobuf/gRPC code from `sandbox/proto/sandbox.proto`
- `ExecutionRequest` — Unified request: Command, Args, Env, WorkDir, TimeoutSec, Limits (memory, CPU, disk, PID), InputFiles, Stdin
- `ExecutionResult` — Unified result: ExitCode, Stdout, Stderr, TimedOut, ResourceUsage, OutputRef

#### Node Execution (`internal/worker/service/`)
- `NodeExecutor` — Routes to tool by name, selects executor (sandbox if enabled + BuildableTool, else direct), enforces timeout, publishes result/failure events to Kafka

### internal/outbox
Event reliability layer.

Key components:
- `Relay` - Background goroutine (100ms ticker) that reads pending outbox entries, publishes to Kafka, deletes on success
- `SaveEvent` - Writes events to outbox table (called by StateService, StateMachine, DependencyChecker)

### internal/publish
Frontend-facing content publishing API.

Key components:
- `PublishHandler` - Gin HTTP handlers for `/api/publish`, `/api/ai/generate`, `/api/ai/generate-from-media`, `/api/ai/polish`
- `PublishService` - Orchestrates content publishing via DAG task creation, AI content generation and text polishing via OpenAI
- `TraceHandler` - Gin HTTP handlers for `/api/trace/recent`, `/api/trace/:taskId` (task lifecycle audit)

API contract (standard response format):
```json
{"code": 0, "message": "success", "data": {...}}
```

### Frontend (frontend/)
React + TypeScript + TailwindCSS + Zustand (no router — simple state-driven page switching).

#### Pages (in `pages/`)
- `PublishPage.tsx` — 创作发布: upload media, write/edit title/description/keywords, AI generate/polish, select platforms, publish
- `DesktopPage.tsx` — 桌面工具 (Electron only): system status monitoring, backend health check, command execution panel

#### Key components (in `components/`)
- `Sidebar.tsx` — Navigation sidebar with 创作发布 / 桌面工具 tabs
- `UploadCard.tsx` — Drag-and-drop video/image upload
- `TitleInput.tsx` — Title input with AI polish button
- `DescriptionInput.tsx` — Description textarea with AI polish button
- `KeywordInput.tsx` — Tag-based keyword input
- `AIHelperPanel.tsx` — AI generate and polish controls
- `PlatformSelector.tsx` — Multi-platform toggle selector
- `PublishButton.tsx` — Publish action button (calls `/api/publish`)
- `CommandPanel.tsx` — Command execution panel (Electron IPC, in DesktopPage)
- `appStore.ts` — Zustand store (title, description, keywords, media, platforms)
- `api.ts` — Axios service calling all `/api/publish`, `/api/ai/*`, `/api/trace/*`

#### Electron vs Web
- Production: Packaged as Electron .dmg/.exe via `electron-builder`
- Development: `npm run dev` serves at port 3000 with Vite proxy forwarding `/api` to `:8080`
- The desktop tools tab is always visible (designed for Electron usage)
- API base URL: auto-detects Electron → `http://localhost:8080/api`, otherwise `/api` (Vite proxy)

Key UI features:
- **Media-based AI generation**: When images/videos are uploaded, AI generate uses `/api/ai/generate-from-media` (multipart) instead of text-only `/api/ai/generate`
- **AI loading overlay**: Full-screen loading animation with progress bar+spinner during AI operations
- **Result popup**: Centered modal with success/error icon and auto-dismiss
- **Debug trace button**: Floating button (bottom-right) to query recent task lifecycle
- **Trace endpoints**: `/api/trace/recent` and `/api/trace/:taskId` for full task+context audit data

## Project Structure

```
cmd/lingxi-ai-os/main.go    # Entry point, wiring, graceful shutdown
internal/
  config/                    # Viper-based config with .env support
  database/                  # pgx pool + schema migrations
  eventbus/                  # IBM/sarama Kafka producer/consumer
  logger/                    # Zap logger (dev/prod modes)
  model/                     # Data models + repositories
  outbox/                    # Outbox pattern (relay.go + SaveEvent)
  redis/                     # go-redis client
  orchestrator/
    handler/                 # Gin HTTP handlers
    service/                 # Business logic + state machine + condition evaluation
  translator/
    handler/                 # Gin HTTP handlers
    service/                 # NL-to-DAG translation
  context/
    handler/                 # Gin HTTP handlers
    service/                 # Context/snapshot management
  publish/
    handler/                 # Publish/AI/Weather HTTP handlers
      handler.go             # Publish, AI generate, AI polish, AI generate-from-media
      trace_handler.go       # Task trace query (/api/trace/recent, /api/trace/:taskId)
      handler_test.go
    service/                 # Content publishing + AI generate/polish
      service.go             # PublishContent, AIGenerateContent, AIGenerateFromMedia, AIPolishText
  worker/
    service/                 # Node execution engine
    tool/                    # Tool interface + registry
      builtin/               # BashTool, PythonTool, LlmApiTool, PolisherTool
    executor/                # DirectExecutor, SandboxExecutor (stub), types
sandbox/                     # Rust sandbox service (gRPC server for isolated execution)
  src/
    main.rs                  # gRPC server entry point (tonic + tokio)
    sandbox.rs               # Sandbox execution with setrlimit resource isolation
  proto/
    sandbox.proto            # Protobuf/gRPC service definition
  Cargo.toml                 # Rust dependencies (tonic, prost, tokio, libc)
  build.rs                   # Proto compilation via tonic-build
frontend/                    # React + TypeScript + TailwindCSS
  src/
    components/             # UI components (Sidebar, UploadCard, TitleInput, AIHelperPanel, etc.)
    pages/                   # Pages (PublishPage)
    stores/                  # Zustand state management (appStore)
    services/                # API services (api.ts)
    utils/                   # Types and utilities
```

## Infrastructure

Docker-based local development:
- PostgreSQL 16 - Primary database
- Redis 7 - Caching
- Redpanda - Event streaming (Kafka compatible)
- MinIO - Object storage
- Qdrant - Vector database

```bash
docker compose up -d    # Start
docker compose down     # Stop
docker compose logs -f  # View logs
```

## Testing

```bash
go test ./...                    # All tests
go test ./internal/orchestrator/ # Orchestrator only
./scripts/test-apis.sh           # API integration tests
```

## Environment Configuration

```bash
cp .env.example .env
```

Required:
- `OPENAI_API_KEY` - API key for LLM operations
- `OPENAI_BASE_URL` - Base URL for LLM service

## Key Concepts

### DAG Task Graph
Tasks are directed acyclic graphs:
- Nodes = individual operations (LLM calls, tool executions)
- Edges = dependencies between nodes
- Nodes can have `condition` field for conditional branching (e.g., `"nodeA.status == success"`)
- Orchestrator handles execution order and state management

### Event Topics
- `ai.node.ready` - DependencyChecker publishes when node is ready (contains idempotencyKey)
- `ai.node.result` - Worker publishes on node completion (contains idempotencyKey)
- `ai.node.executed` - StateMachine publishes on success, DependencyChecker consumes
- `ai.node.failed` - StateMachine publishes on permanent failure
- `ai.task.completed` - Published on task completion
- `ai.task.failed` - Published on task failure

All events go through the outbox table first, then relayed to Kafka.

### Node Status Flow
- `CREATED` -> `READY` (dependencies met, via TryMakeReady or DependencyChecker) -> `RUNNING` -> `SUCCESS` or `FAILED`
- `SKIPPED` - Condition not met (counts as satisfied for downstream dependencies and task completion)
- `RETRYING` - Intermediate during exponential backoff retry
- Failed with retries: `FAILED` -> `RETRYING` -> `CREATED` (Scheduler 30s fallback recovers to `READY`)

### Outbox Pattern
Events are first written to the `outbox` DB table via `SaveEvent()`. A background Relay goroutine (100ms ticker) reads pending entries, publishes to Kafka, and deletes on success. This guarantees no event loss even if Kafka is temporarily unavailable.

### Idempotency
idempotencyKey = taskId + "-" + nodeId, used as Kafka message key for deduplication.

### Tool Name Routing
- TOOL type: `payload["name"]` (the node's name) determines the tool (e.g., "weather", "bash")
- LLM type: always routes to "llm_api"
- Explicit override: `payload["tool"]` takes highest priority

## Development Workflow

1. Ensure Docker is running and infrastructure is up
2. `make run` to build and start
3. Make changes following existing Go patterns
4. Write tests for new functionality
5. `./scripts/test-apis.sh` before submitting

## API Endpoints

### Publish Module (frontend-facing)
- `POST /api/publish` - Submit content for publishing (multipart: title, description, keywords, platforms, images, videos)
- `POST /api/ai/generate` - AI-generate title and description from text prompt
- `POST /api/ai/generate-from-media` - AI-generate from images/videos + prompt (multipart)
- `POST /api/ai/polish` - AI-polish existing text (title or description)
- `GET /api/trace/recent` - Get most recent task trace (full task + context audit data)
- `GET /api/trace/:taskId` - Get specific task trace

### Orchestrator
- `POST /api/task/create` - Create a new task
- `POST /api/task/:taskId/dag` - Submit DAG for a task
- `GET /api/task/:taskId` - Get task details
- `POST /api/node` - Submit DAG from NL translation (creates task + submits DAG)

### Translator
- `POST /api/translate` - Translate natural language to DAG
- `POST /api/translate/submit` - Translate and submit in one step

### Context
- `GET /api/task/:taskId/context` - Get task context history

## Common Issues

- **NL-Translator 503**: Missing or invalid OPENAI_API_KEY
- **Port conflict**: Ensure port 8080 is not in use
- **Docker not running**: Start Docker Desktop first
- **Go not installed**: Need Go 1.23+, install via `brew install go`
- **Old consumer group incompatibility**: If switching from Java version, delete old Kafka consumer groups via `rpk group delete <group-name>`
