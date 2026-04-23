# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🔑 Quick Reference

### Service Ports
- AI-Context: http://localhost:8082
- AI-Orchestrator: http://localhost:8080
- NL-Translator: http://localhost:8081
- AI-Worker: http://localhost:8083

### Core Commands

#### Start all services
```bash
# Copy environment config
cp .env.example .env

# Edit .env and add your OPENAI_API_KEY

# One-click startup
./scripts/startup.sh
```

#### Manual startup
```bash
# Start infrastructure
Docker compose up -d

# Start individual modules
cd ai-context && mvn spring-boot:run
cd ai-orchestrator && mvn spring-boot:run
cd ai-nl-translator && mvn spring-boot:run
cd ai-worker && mvn spring-boot:run
```

#### Test APIs
```bash
# Run full API test suite
./scripts/test-apis.sh

# Test specific module
curl http://localhost:8080/api/health
```

## 🏗️ Architecture Overview

This is a natural language-driven operating system built on a microservices architecture:

1. **NL-Translator (8081)** - Converts user natural language prompts into executable DAG task graphs using LLMs
2. **AI-Orchestrator (8080)** - Core task scheduler that manages task lifecycle, dependencies, and event distribution
3. **AI-Context (8082)** - Persists task history and provides audit/snapshot capabilities
4. **AI-Worker (8083)** - Executes actual operations by interfacing with external tools, services, and hardware

### Communication Flow
- Services communicate via REST APIs
- Event-driven architecture using Redpanda (Kafka-compatible) event bus
- Persistence with PostgreSQL, caching with Redis

## 📦 Module Details

### ai-orchestrator
Core task scheduling engine built with Spring Boot 3.2.5 and Java 17.

Key responsibilities:
- Receive DAG tasks from NL-Translator
- Manage task and node state via StateService (unified state convergence)
- Event-driven scheduling via DependencyChecker (checks dependencies after each node execution)
- Exponential backoff retry via RetryPolicy (1s → 2s → 4s → ... → 60s max)
- Idempotent execution with idempotencyKey (taskId + "-" + nodeId)
- System recovery via Scheduler (recovers CREATED nodes after restart)
- Publish events to Redpanda event bus
- Update context via AI-Context service

Build & run:
```bash
cd ai-orchestrator
mvn clean install
mvn spring-boot:run
```

### ai-nl-translator
Natural language processing layer that converts user prompts to structured DAGs.

Key features:
- LLM-powered intent recognition
- DAG generation and validation
- REST API endpoint for translation

Build & run:
```bash
cd ai-nl-translator
mvn clean install
mvn spring-boot:run
```

### ai-context
Audit and context management service.

Key features:
- Persist task and node state to PostgreSQL
- Store operation history and snapshots
- Provide recovery capabilities
- Auto-record context from Kafka events (including ai.node.executed)

Build & run:
```bash
cd ai-context
mvn clean install
mvn spring-boot:run
```

### ai-worker
Tool execution gateway that performs actual operations.

Key features:
- Plugin-based tool architecture
- Integration with external APIs and services
- Consumes events from Redpanda (ai.node.ready)
- Executes database operations, HTTP requests, LLM calls, etc.
- Idempotent result publishing (idempotencyKey as Kafka message key)
- Timeout control for tool execution

Build & run:
```bash
cd ai-worker
mvn clean install
mvn spring-boot:run
```

## 🐳 Infrastructure

The project uses Docker for local development:
- PostgreSQL 16 - Primary database
- Redis 7 - Caching and session storage
- Redpanda - Event streaming platform (Kafka compatible)
- MinIO - Object storage
- Qdrant - Vector database

Manage infrastructure:
```bash
# Start services
docker compose up -d

# Stop services
docker compose down

# View logs
docker compose logs -f
```

## 🧪 Testing

### Run tests for all modules
```bash
# Run unit tests
mvn test

# Run integration tests
mvn verify
```

### API Testing
The test-apis.sh script provides comprehensive API testing:
```bash
./scripts/test-apis.sh
```

## 📚 Documentation

- Full API reference: `docs/API_REFERENCE.md`
- Orchestrator guide: `docs/guides/ORCHESTRATOR_GUIDE.md`
- NL-Translator guide: `docs/guides/NL_TRANSLATOR_GUIDE.md`
- Context management guide: `docs/guides/AI_CONTEXT_GUIDE.md`
- Worker guide: `docs/guides/WORKER_GUIDE.md`

## 📝 Environment Configuration

Copy and customize the environment file:
```bash
cp .env.example .env
```

Required configuration:
- `OPENAI_API_KEY` - Your OpenAI API key for LLM operations
- `OPENAI_BASE_URL` - Base URL for LLM service

## 🎯 Key Concepts

### DAG Task Graph
Tasks are represented as directed acyclic graphs where:
- Nodes represent individual operations (LLM calls, tool executions)
- Edges represent dependencies between nodes
- Orchestrator handles execution order and state management

### Event Topics
- `ai.node.ready` - Published by DependencyChecker when a node is ready for execution (contains idempotencyKey)
- `ai.node.result` - Published by Worker when a node completes execution (contains idempotencyKey)
- `ai.node.executed` - Published by StateMachine after node success, consumed by DependencyChecker for downstream scheduling
- `ai.node.failed` - Published by StateMachine on permanent node failure
- `ai.task.created` - Published on task creation
- `ai.task.success` / `ai.task.failed` - Published on task completion

### Node Status Flow
- `CREATED` → `READY` (dependencies met) → `RUNNING` → `SUCCESS` or `FAILED`
- `RETRYING` - Intermediate status during exponential backoff retry
- Failed nodes with retries remaining transition: `FAILED` → `RETRYING` → `CREATED` (Scheduler recovers to `READY`)

## 🔧 Development Workflow

1. Ensure Docker is running and infrastructure is up
2. Start individual services as needed
3. Make changes following the existing code patterns
4. Write tests for new functionality
5. Test with test-apis.sh before submitting

## 📌 Common Issues

- **NL-Translator 503 error**: Missing or invalid OPENAI_API_KEY configuration
- **Port conflicts**: Ensure ports 8080, 8081, 8082, 8083 are not in use
- **Docker not running**: Start Docker Desktop before running services
