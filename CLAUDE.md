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
- Persistence with PostgreSQL (pgx), caching with Redis (go-redis)

## Module Details

### internal/orchestrator
Core task scheduling engine.

Key components:
- `OrchestratorService` - Task creation, DAG submission, validation
- `StateService` - Unified state convergence for tasks and nodes
- `StateMachine` - Handles node success/failure, retry logic, event publishing
- `DependencyChecker` - Event-driven: checks downstream dependencies after node execution
- `RetryPolicy` - Exponential backoff (1s -> 2s -> 4s -> ... -> 60s max)
- `Scheduler` - Recovers CREATED nodes after restart (1s ticker goroutine)
- `TaskExecutionControl` - Pause/resume/retry operations
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
- `BashTool` - Execute shell commands with timeout (exec.CommandContext)
- `LlmApiTool` - Call OpenAI chat/completions API
- `NodeExecutor` - Determine tool, validate, execute with timeout, publish result events

## Project Structure

```
cmd/lingxi-ai-os/main.go    # Entry point, wiring, graceful shutdown
internal/
  config/                    # Viper-based config with .env support
  database/                  # pgx pool + schema migrations
  eventbus/                  # Sarama Kafka producer/consumer
  logger/                    # Zap logger (dev/prod modes)
  model/                     # Data models + repositories
  redis/                     # go-redis client
  orchestrator/
    handler/                 # Gin HTTP handlers
    service/                 # Business logic + state machine
  translator/
    handler/                 # Gin HTTP handlers
    service/                 # NL-to-DAG translation
  context/
    handler/                 # Gin HTTP handlers
    service/                 # Context/snapshot management
  worker/
    service/                 # Node execution engine
    tool/                    # Tool interface + registry
      builtin/               # BashTool, LlmApiTool
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
- Orchestrator handles execution order and state management

### Event Topics
- `ai.node.ready` - DependencyChecker publishes when node is ready (contains idempotencyKey)
- `ai.node.result` - Worker publishes on node completion (contains idempotencyKey)
- `ai.node.executed` - StateMachine publishes on success, DependencyChecker consumes
- `ai.node.failed` - StateMachine publishes on permanent failure
- `ai.task.completed` - Published on task completion
- `ai.task.failed` - Published on task failure

### Node Status Flow
- `CREATED` -> `READY` (dependencies met) -> `RUNNING` -> `SUCCESS` or `FAILED`
- `RETRYING` - Intermediate during exponential backoff retry
- Failed with retries: `FAILED` -> `RETRYING` -> `CREATED` (Scheduler recovers to `READY`)

### Idempotency
idempotencyKey = taskId + "-" + nodeId, used as Kafka message key for deduplication.

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
