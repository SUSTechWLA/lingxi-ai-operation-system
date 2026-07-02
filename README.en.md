# Tangying AI Operation System

**Language / 语言:** [中文](README.md) | English

Tangying AI Operation System is a local desktop + cloud orchestration system for video creation workflows. Starting from a one-line video idea, it generates reviewable, traceable, refillable, and exportable multi-stage artifacts, while the client handles asset management, external model handoff, local execution, and final video review.

Project Wiki:

- [Chinese Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki)
- [English Wiki](https://github.com/SUSTechWLA/tangying-ai-operation-system/wiki/English)

## Core Capabilities

- **Two video workflows**: talking-head / knowledge videos and cinematic AIGC shot videos.
- **Cloud orchestration**: the cloud backend owns task planning, agent runtime, review gates, persistence, and diagnostics.
- **Local execution**: the local backend owns local files, artifacts, cache, logs, rendering, and desktop tool execution.
- **External model handoff**: users do not have to configure third-party model APIs inside the system. They can copy prompts and reference information to external model websites, then upload generated results back into the project.
- **Human review gates**: scripts, storyboards, previews, renders, and delivery packages can be viewed, approved, rejected, edited, or regenerated where supported.

## Runtime Boundaries

```text
frontend/                       React + Electron desktop client
local-backend/                  Local desktop agent and local runner
cloud-backend/                  Go AIOS Core cloud backend and orchestration
hyperframes-render-service/     Optional local render service
```

The local runtime owns local files, cache, artifacts, logs, diagnostics, and desktop execution. It must not depend on PostgreSQL, Redis, Kafka, MinIO, Docker, or LLM API keys.

The cloud backend owns API integration, dynamic agent planning, orchestration, persistence, remote configuration, and cloud-side diagnostics.

## Core Flow

```text
User prompt
  -> POST /api/agent/runs
  -> LLMPlanner / client model config
  -> PlanGuard
  -> PlanCompiler
  -> transient DAG
  -> artifact review gates
  -> local runner material generation / render handoff
  -> final video artifact visible in client
```

Intermediate artifacts are reviewable. Review gates can approve, reject, edit, or regenerate where supported.

## Local Development

Start the local backend:

```bash
bash scripts/start-local-backend.sh
```

Start the desktop frontend:

```bash
bash scripts/start-frontend.sh
```

Start the cloud backend:

```bash
bash scripts/start-cloud-backend.sh
```

Build the desktop app:

```bash
bash scripts/build-local-desktop.sh
```

The macOS build output is written under:

```text
frontend/release/
```

## Verification

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
cd ../frontend && npm run build
```

For cloud backend concurrency checks:

```bash
cd cloud-backend && go test -race ./...
```

## API Entry Points

Cloud backend:

```text
http://localhost:8080/docs
http://localhost:8080/openapi.json
```

Local agent:

```text
http://localhost:18080/api/local/docs
http://localhost:18080/api/local/openapi.json
```

## Release Branch Scope

The `release` branch keeps core runtime code, build files, runtime skill configuration, and the project README. Auxiliary documentation, smoke assets, eval fixtures, and development-only scripts are intentionally excluded from release.
