# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Service
- Single Go binary on port 8080 (all modules combined in a modular monolith)

### Core Commands

#### Start all services
```bash
# Copy environment config
cp .env.example .env

# Edit .env and add your OPENAI_API_KEY

# One-click startup (builds + starts infrastructure + runs)
./scripts/startup.sh
```

#### Manual startup
```bash
# Start infrastructure
docker compose up -d

# Build and run
go build -o build/lingxi-ai-os cmd/lingxi-ai-os/main.go
./build/lingxi-ai-os
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

# Health check
curl http://localhost:8080/api/health
```

## Architecture Overview

Go modular monolith — all modules run in a single process on port 8080:

1. **NL-Translator** - Converts natural language prompts into executable DAG task graphs using LLMs
2. **Orchestrator** - Core task scheduler managing task lifecycle, dependencies, and event distribution
3. **Context** - Persists task history and provides audit/snapshot capabilities
4. **Worker** - Executes operations via plugin-based tool architecture

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
Tool execution gateway.

Key components:
- `Tool` interface - Name, Description, Type, Execute, ValidateParameters
- `ToolRegistry` - Plugin registration and lookup
- `BashTool` - Sandboxed shell execution: command whitelist + dangerous pattern filtering + /tmp/ai-sandbox working directory
- `LlmApiTool` - Call OpenAI chat/completions API (accepts prompt/message/content fields)
- `WeatherTool` - Example TOOL-type plugin for weather queries
- `NodeExecutor` - Determine tool (TOOL type uses node name, LLM type uses llm_api), validate, execute with timeout, publish result events

### internal/outbox
Event reliability layer.

Key components:
- `Relay` - Background goroutine (100ms ticker) that reads pending outbox entries, publishes to Kafka, deletes on success
- `SaveEvent` - Writes events to outbox table (called by StateService, StateMachine, DependencyChecker)

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
  worker/
    service/                 # Node execution engine
    tool/                    # Tool interface + registry
      builtin/               # BashTool(sandboxed), LlmApiTool, WeatherTool
frontend/                    # Frontend project (React/Vue, etc.)
  src/
    components/             # UI components
    pages/                   # Pages
    hooks/                   # Custom hooks
    utils/                   # Utility functions
    services/                # API services
    stores/                  # State management
    styles/                  # Styles
    assets/                  # Static assets
  public/                    # Public assets
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

## Common Issues

- **NL-Translator 503**: Missing or invalid OPENAI_API_KEY
- **Port conflict**: Ensure port 8080 is not in use
- **Docker not running**: Start Docker Desktop first
- **Go not installed**: Need Go 1.23+, install via `brew install go`
- **Old consumer group incompatibility**: If switching from Java version, delete old Kafka consumer groups via `rpk group delete <group-name>`
