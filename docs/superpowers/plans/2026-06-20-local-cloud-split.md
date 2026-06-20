# Local Cloud Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the repository into frontend, local backend, and cloud backend entry points so desktop users can run without local databases while cloud services own LLM/config/integration concerns.

**Architecture:** Keep the existing Go AIOS Core as the cloud backend and move it to `cloud-backend/`. Add a lightweight `local-backend/` Go module with no PostgreSQL, Redis, Kafka, MinIO, or Docker dependency; it manages local paths, logs, command execution, and diagnostic packages. Keep `frontend/` as the UI/Electron app and route local-only operations to the local agent while cloud API calls remain configurable.

**Tech Stack:** Go 1.25 compatible modules, React/Vite/Electron, shell scripts, Docker Compose for cloud-only infrastructure.

---

### Task 1: Directory Migration

**Files:**
- Move: `aios-core/` to `cloud-backend/`
- Keep: `frontend/`
- Create: `local-backend/`
- Create: `scripts/`

- [ ] Move the existing cloud backend directory.
- [ ] Update root scripts and documentation references from `aios-core` to `cloud-backend`.
- [ ] Keep generated dependency directories ignored.

### Task 2: Local Agent

**Files:**
- Create: `local-backend/go.mod`
- Create: `local-backend/cmd/local-agent/main.go`
- Create: `local-backend/internal/localagent/server.go`
- Create: `local-backend/internal/localagent/server_test.go`

- [ ] Write tests for health, local paths, log writing, and diagnostics zip creation.
- [ ] Implement a minimal HTTP server on `127.0.0.1:18080`.
- [ ] Ensure it uses user data directories and does not import database or cloud infrastructure packages.

### Task 3: Entrypoint Scripts

**Files:**
- Create: `scripts/start-frontend.sh`
- Create: `scripts/start-local-backend.sh`
- Create: `scripts/start-cloud-backend.sh`
- Create: `scripts/build-local-desktop.sh`

- [ ] Add focused scripts for each runtime.
- [ ] Keep cloud Docker infrastructure only under `cloud-backend`.
- [ ] Build local agent as part of desktop packaging preparation.

### Task 4: Frontend API Boundary

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/utils/electron.d.ts`
- Modify: `frontend/electron/main.cjs`
- Modify: `frontend/electron/preload.cjs`

- [ ] Make cloud API base configurable with `VITE_CLOUD_API_BASE`.
- [ ] Expose local agent URL to the renderer.
- [ ] Keep packaged desktop from assuming a local cloud backend on port 8080.

### Task 5: Docs

**Files:**
- Create: `docs/LOCAL_USAGE.md`
- Create: `docs/CLOUD_DEPLOYMENT.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/ARCHITECTURE.md`

- [ ] Document local user flow: install, open app, local files/logs/diagnostics.
- [ ] Document cloud flow: env, deploy, logs, diagnostics analysis.
- [ ] Document the new repo map and runtime boundary.

### Task 6: Verification

**Commands:**
- `cd local-backend && go test ./...`
- `cd cloud-backend && go test ./...`
- `cd frontend && npm run build`
- `bash scripts/start-local-backend.sh --check`
- `bash scripts/start-cloud-backend.sh --check`

- [ ] Run all checks and record failures.
- [ ] Fix only migration-related failures.
