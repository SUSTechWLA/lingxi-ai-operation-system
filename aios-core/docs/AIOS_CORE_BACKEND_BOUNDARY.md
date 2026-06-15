# AIOS Core Backend Boundary

This repository now treats the Go service as the AIOS Core backend and the React/Electron apps as replaceable clients.

## Backend Core

The backend Core is the Go module rooted at this repository:

- `cmd/tangying-ai-os/` starts the single Core process on port `8080`.
- `internal/orchestrator/`, `internal/worker/`, `internal/skill/`, `internal/translator/`, `internal/context/`, `internal/outbox/`, `internal/eventbus/`, `internal/database/`, `internal/redis/`, and `internal/config/` are platform Core packages.
- `internal/publish/` and `internal/media/` are current product-facing adapters. They can remain while new customer solutions use Core APIs, workflow definitions, and external tool manifests instead of changing Core for each business case.
- `sandbox/` is a separate Rust execution service. Build and run it independently when sandbox execution is enabled.

## Client Boundary

`frontend/` and `electron/` are client projects, not Go backend packages. Each has its own `go.mod` boundary only to keep `go test ./...` and `go list ./...` focused on Core packages when local `node_modules` contains Go files.

Frontend teams should treat the backend as an HTTP API service:

- Use `/api/health` for readiness.
- Use `/api/task/*`, `/api/translate/*`, `/api/skill/dialog/*`, `/api/tools`, `/api/media/*`, and `/api/trace/*` as Core-facing integration surfaces.
- Do not rely on Go package internals from client code.
- Do not require customer-specific UI logic to be added to Core.

## External Tool Boundary

Tool teams own tool implementation. Core owns only the contract:

- tool manifest registration and discovery;
- input/output schemas;
- timeout, retry, idempotency, and error mapping;
- health checks and black-box acceptance;
- trace propagation and logs.

Business rules, algorithms, model choices, and tool-internal services stay outside Core.

## Commands

Backend-only development:

```bash
make backend-build
make backend-test
./scripts/startup.sh --backend-only
```

Full local demo with the current React client:

```bash
./scripts/startup.sh
```

Frontend client development:

```bash
cd frontend
npm install
npm run dev
```
