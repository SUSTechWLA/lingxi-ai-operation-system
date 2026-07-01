# Tangying AI Operation System

躺营 AI 自媒体运营助手是一个面向视频创作工作流的本地桌面 + 云端编排系统。当前核心能力是从一句话视频需求出发，生成可审核的多阶段产物，并在客户端查看最终视频。

## Runtime Boundaries

```text
frontend/                  React + Electron desktop client
local-backend/             Local desktop agent and local runner
cloud-backend/             Go AIOS Core cloud backend and orchestration
hyperframes-render-service/ Optional local render service
```

Local runtime owns local files, cache, artifacts, logs, diagnostics, and desktop execution. It must not depend on PostgreSQL, Redis, Kafka, MinIO, Docker, or LLM API keys.

Cloud backend owns API integration, dynamic agent planning, orchestration, persistence, remote configuration, and cloud-side diagnostics.

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

The release branch keeps core runtime code, build files, runtime skill configuration, and this project README. Auxiliary documentation, smoke assets, eval fixtures, and development-only scripts are intentionally excluded.
