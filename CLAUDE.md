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
./build/tangying-sandbox & # Start sandbox gRPC server on :50051

# To enable sandbox for tool execution:
# Set SANDBOX_ENABLED=true in .env, restart backend
```

#### Manual backend startup
```bash
# Terminal 1: Infrastructure
docker compose up -d

# Terminal 2: Backend (port 8080)
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
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
6. **Media** - MinIO-backed media asset management with upload, tag filtering, and presigned URL retrieval
7. **Skill** - AI conversational assistant with multi-turn dialog, LLM-driven DAG planning, Redis-backed session state, and tool manifest knowledge base

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
- `ManifestProvider` — Optional interface: `Manifest() ToolManifest` exposes full parameter/ output schema, sandbox requirements, and examples for AI tool discovery
- `ExternalToolProvider` — Interface for tools that can execute registered external tools by name
- `ToolRegistry` — Plugin registration and lookup by name; also manages external tool manifests (RegisterExternal, DeregisterExternal, ListManifests)
- `ToolManifest` (`manifest.go`) — Full tool specification: Name, Description, Type, Endpoint, Timeout, Parameters (map of ParamDef), Output (map of ParamDef), Sandbox flag, Examples

#### Built-in Tools (`internal/worker/tool/builtin/`)
- `BashTool` — Sandboxed shell: command whitelist + dangerous pattern filter + `/tmp/tangying-sandbox` workdir. Implements `BuildableTool`.
- `PythonTool` — python3 -c execution with resource limits. Implements `BuildableTool`.
- `LlmApiTool` — OpenAI chat/completions API calls. Implements `ExecutableTool`. Defined in `builtin.go`.
- `PolisherTool` — Text polish for social media titles/descriptions via LLM. Implements `ExecutableTool`.
- `MediaAnalyzerTool` — Media analysis: extracts tags, suggestions, and summaries from uploaded images/videos via LLM. Implements `ExecutableTool`.
- `ContentGeneratorTool` — Full content package generation based on media analysis, platform, and style keywords. Implements `ExecutableTool`.
- `ContentCheckerTool` — Content compliance check: sensitive words, advertising law violations, platform-specific rules. Implements `ExecutableTool`.
- `PlatformAdapterTool` — Cross-platform content adaptation: adjusts tone, format, and length for 7 social media platforms. Implements `ExecutableTool`.
- `ChatGenerateTool` — Conversational content generation with full multi-turn message history via OpenAI. Implements `ExecutableTool`.
- `ChatReviseTool` — Revise or generate content fields (title, description, keywords) from natural language instructions. Implements `ExecutableTool`.
- `ExternalTool` — Bridge to registered external tool services via HTTP. Routes DAG pipeline calls to external endpoints registered through `/api/tools/register`. Implements `ExecutableTool` + `ExternalToolProvider`.
- `VideoMetadataTool` — Downloads video from MinIO (via presigned URL) and extracts metadata: duration, resolution, frame rate, codec, audio track info. Caches video locally for downstream tools (`/tmp/tangying-video-cache`). Implements `ExecutableTool`.
- `VideoAnalyzerTool` — Extracts keyframes via ffmpeg scene detection and transcribes audio via Whisper. Outputs base64 data URLs for keyframes and dialogue transcript text. Implements `ExecutableTool`.
- `VideoCopyGeneratorTool` — Generates platform-adapted short-video titles, copy, and keywords from video metadata, keyframe analysis, and audio transcripts. Uses multimodal LLM. Supports douyin/xiaohongshu/bilibili/kuaishou platforms. Implements `ExecutableTool`.

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
{"code": 200, "message": "success", "data": {...}}
```

### internal/media
Media asset management backed by MinIO object storage.

Key components:
- `MediaHandler` - Gin HTTP handlers for `/api/media/upload`, `/api/media/list`, `/api/media/:id`, `/api/media/:id/tags`
- `MediaService` - CRUD operations on `media_assets` table, tag-based filtering with PostgreSQL JSONB `@>` queries
- `StorageService` - Wraps `minio-go` client: auto-creates bucket on init, upload with content-type detection, presigned GET URLs (24h TTL), delete

Media asset model: ID, UserID, OriginalName, MimeType, Size, MinioPath, Tags (JSONB array), EmbeddingID, timestamps.

### internal/skill (AI Assistant Dialog)
Conversational AI assistant with multi-turn dialog, LLM-driven DAG planning, and tool manifest knowledge base.

Key components:
- `SessionHandler` - `/api/skill/dialog/session/*` endpoints for creating sessions, sending messages, querying progress, and terminating
- `SessionManager` - Redis-backed session state with message history (50-message cap, 30min TTL), media context (presigned URLs), and task tracking
- `PlanService` - Builds LLM prompt from conversation history + media context + tool manifests → generates DAG (JSON mode)
- `ResultAssembler` - Creates orchestrator tasks, submits DAGs (with node ID scoping), polls for completion, extracts title/description/keywords from node outputs
- `LLMClient` - Typed OpenAI chat completion client with JSON schema response format
- `ToolManifestService` - DB-persisted + Redis-cached tool knowledge base; syncs builtin tools on startup, formats manifests for LLM DAG prompts
- `prompts/` - System prompt templates for DAG generation and content planning

### internal/publish (Tools)
- `ToolHandler` - `/api/tools` endpoints for listing/querying/registering/deregistering tool manifests (builtin + external)

### Frontend (frontend/)
React + TypeScript + TailwindCSS + Zustand (no router — simple state-driven page switching).

#### Pages (in `pages/`)
- `PublishPage.tsx` — 创作发布: upload media, write/edit title/description/keywords, AI generate/polish, select platforms, publish
- `DesktopPage.tsx` — 桌面工具 (Electron only): system status monitoring, backend health check, command execution panel

#### Key components (in `components/`)
- `Sidebar.tsx` — Navigation sidebar with 创作发布 / 桌面工具 tabs
- `UploadCard.tsx` — Drag-and-drop video/image upload with content type selection
- `TitleInput.tsx` — Title input with AI polish button
- `DescriptionInput.tsx` — Description textarea with AI polish button
- `KeywordInput.tsx` — Tag-based keyword input
- `AIHelperPanel.tsx` — AI generate and polish controls
- `AIAssistantTab.tsx` — Conversational AI chat panel with multi-turn history, media context, and skill dialog integration
- `BlockingOverlay.tsx` — Full-screen loading overlay during AI operations with cancel button
- `ContentTypeSelector.tsx` — Content type selection (video/article/image)
- `MediaLibraryPanel.tsx` — Side panel for browsing and filtering uploaded media assets
- `PlatformSelector.tsx` — Multi-platform toggle selector (10 platforms)
- `PublishButton.tsx` — Publish action button (calls `/api/publish`)
- `DesktopToolbar.tsx` — Electron desktop toolbar
- `CommandPanel.tsx` — Command execution panel (Electron IPC, in DesktopPage)
- `appStore.ts` — Zustand store (title, description, keywords, body, media, platforms, cover, aiLoadingMessage, chatSessionId, contentType)
- `api.ts` — Axios service calling all `/api/publish`, `/api/ai/*`, `/api/trace/*`, `/api/skill/dialog/*`, `/api/media/*`, `/api/tools`

#### Electron vs Web
- Production: Packaged as Electron .dmg/.exe via `electron-builder` (config in `electron/package.json`); bundles `frontend/dist/` as static assets
- `electron/main.js` — Main process: creates BrowserWindow, loads frontend dist or dev server
- `electron/preload.js` — Preload script for secure IPC between renderer and main process
- Development: `npm run dev` serves at port 3000 with Vite proxy forwarding `/api` to `:8080`
- The desktop tools tab (`DesktopPage`, `DesktopToolbar`, `CommandPanel`) is always visible
- API base URL: auto-detects Electron → `http://localhost:8080/api`, otherwise `/api` (Vite proxy)

Key UI features:
- **Media-based AI generation**: When images/videos are uploaded, AI generate uses `/api/ai/generate-from-media` (multipart) instead of text-only `/api/ai/generate`; video pipeline involves metadata extraction → frame analysis → audio transcription → copy generation
- **Conversational AI assistant**: `AIAssistantTab` provides multi-turn chat via `/api/skill/dialog/session/*` endpoints, with media context (presigned URLs for multimodal vision) and LLM-driven DAG planning
- **Media library panel**: Browse, filter by tag, and reuse previously uploaded media assets
- **Content type selector**: Choose between video, article, image content types before generation
- **AI loading overlay**: Full-screen `BlockingOverlay` with progress bar+spinner and cancel button during AI operations; cancel terminates the backend task (via `/api/task/:taskId/fail`) and records context
- **Debug trace button**: Floating button (bottom-right) to query recent task lifecycle
- **Trace endpoints**: `/api/trace/recent` and `/api/trace/:taskId` for full task+context audit data
- **Async polish**: Title/description polish uses submit/poll pattern (`/api/ai/polish/submit` + `/api/ai/polish/result`) for cancel support

## Project Structure

```
cmd/tangying-ai-os/main.go    # Entry point, wiring, graceful shutdown
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
  media/
    handler.go               # Media upload/list/get/update-tags HTTP handlers
    service.go               # Media CRUD + MinIO storage
    storage.go               # MinIO client wrapper (bucket auto-create, presigned URLs)
  publish/
    handler/                 # Publish/AI/Tool HTTP handlers
      handler.go             # Publish, AI generate, AI polish (sync+async), AI generate-from-media
      trace_handler.go       # Task trace query (/api/trace/recent, /api/trace/:taskId)
      tool_handler.go        # Tool registry query/register/deregister
      handler_test.go
    service/                 # Content publishing + AI generate/polish
      service.go             # PublishContent, AIGenerateContent, AIGenerateFromMedia, AIPolishText
  worker/
    service/                 # Node execution engine
    tool/                    # Tool interface + registry + manifest
      tool.go                # Tool, BuildableTool, ExecutableTool, ManifestProvider, ExternalToolProvider
      manifest.go            # ToolManifest spec (parameters, output, examples)
      builtin/               # BashTool, PythonTool, LlmApiTool, PolisherTool, MediaAnalyzer, ContentGenerator, ContentChecker, PlatformAdapter, ChatGenerateTool, ChatReviseTool, ExternalTool, VideoMetadataTool, VideoAnalyzerTool, VideoCopyGeneratorTool
    executor/                # DirectExecutor, SandboxExecutor, types
  skill/                     # AI conversational assistant (dialog + DAG planning)
    handler/
      session_handler.go     # Session create/get/chat/progress/terminate HTTP handlers
    service/
      session_manager.go     # Redis-backed session state (30min TTL, 50-msg cap)
      plan_service.go        # LLM DAG generation from conversation context + tool manifests
      result_assembler.go    # Task creation, DAG submission, polling, field extraction
      llm_client.go          # Typed OpenAI chat completion client (JSON schema mode)
      tool_manifest_service.go # DB+Redis tool knowledge base (sync, cache, format for LLM)
    prompts/
      prompts.go             # System prompt templates for DAG generation
  common/                    # Shared utility packages
    llmutil/                 # OpenAI client helpers
    jsonx/                   # JSON parsing/schema utilities
    metadata/                # Metadata extraction utilities
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
    components/             # UI components (Sidebar, UploadCard, TitleInput, DescriptionInput, KeywordInput, AIHelperPanel, AIAssistantTab, BlockingOverlay, ContentTypeSelector, MediaLibraryPanel, PlatformSelector, PublishButton, DesktopToolbar, CommandPanel)
    pages/                   # Pages (PublishPage, DesktopPage)
    stores/                  # Zustand state management (appStore)
    services/                # API services (api.ts)
    utils/                   # Types (types.ts) and Electron utilities (electron.ts)
electron/                    # Electron desktop wrapper
  main.js                    # Electron main process
  preload.js                 # Preload script for IPC
  package.json               # electron-builder config (outputs .dmg/.exe)
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
- TOOL type: `payload["name"]` (the node's name) determines the tool (e.g., "bash", "python")
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
- `POST /api/ai/polish/submit` - Submit async polish task (returns taskId+nodeId immediately for cancel support)
- `GET /api/ai/polish/result` - Query async polish result by taskId+nodeId
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

### Media
- `POST /api/media/upload` - Upload images/videos (multipart, stored in MinIO)
- `GET /api/media/list?userId=&offset=&limit=&tag=` - List assets with tag filtering
- `GET /api/media/:id` - Get single media asset
- `PUT /api/media/:id/tags` - Update asset tags

### Skill / AI Assistant Dialog
- `POST /api/skill/dialog/session/create` - Create a new conversation session (with optional page context: title, description, keywords, media IDs)
- `GET /api/skill/dialog/session/:id` - Get session state (message history + media context + task IDs)
- `POST /api/skill/dialog/session/:id/chat` - Send a message → LLM plans DAG → orchestrator executes → returns generated fields
- `GET /api/skill/dialog/session/:id/progress` - Query current execution progress (IDLE/EXECUTING/TERMINATED)
- `POST /api/skill/dialog/session/:id/terminate` - Terminate a session

### Tool Registry
- `GET /api/tools` - List all tool manifests (builtin + external) with full parameter/output schemas
- `GET /api/tools/:name` - Get specific tool manifest
- `POST /api/tools/register` - Register an external tool (HTTP endpoint + manifest)
- `DELETE /api/tools/:name` - Deregister an external tool

## Common Issues

- **NL-Translator 503**: Missing or invalid OPENAI_API_KEY
- **Port conflict**: Ensure port 8080 is not in use
- **Docker not running**: Start Docker Desktop first
- **Go not installed**: Need Go 1.23+, install via `brew install go`
- **Old consumer group incompatibility**: If switching from Java version, delete old Kafka consumer groups via `rpk group delete <group-name>`
