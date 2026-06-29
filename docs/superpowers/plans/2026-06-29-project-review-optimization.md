# Project Review Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tighten backend security boundaries and improve the director studio's professional UI quality, responsiveness, and maintainability.

**Architecture:** Keep local-backend local-only and key-safe, wrap cloud runtime routes with existing auth middleware without changing handler behavior in unit tests, and make frontend changes inside the current DirectorStudioPage structure to avoid a risky broad split. Use focused tests for backend behavior and existing frontend build/director checks for UI logic.

**Tech Stack:** Go net/http and Gin, React 18, Vite, Tailwind CSS, ESLint 9 flat config, react-markdown.

## Global Constraints

- Do not add database, Docker, Kafka, Redis, or MinIO dependencies to `local-backend`.
- Do not store LLM API keys in the local desktop package.
- Keep cloud business orchestration in `cloud-backend`.
- Generated OpenAPI docs must remain in sync.
- Use tests before production code changes where behavior changes.

---

### Task 1: Local Secret Exposure Boundary

**Files:**
- Modify: `local-backend/internal/localagent/server_test.go`
- Modify: `local-backend/internal/localagent/server.go`

**Interfaces:**
- Consumes: `Server.Handler() http.Handler`
- Produces: local CORS responses that echo allowed local origins and rejects full-key reads from browser-style requests.

- [ ] **Step 1: Write failing local-backend tests**

Add tests proving `include_key=true` does not expose API keys when an `Origin` header is present and CORS no longer returns `*`.

- [ ] **Step 2: Run tests to verify failure**

Run: `cd local-backend && go test ./internal/localagent`
Expected: FAIL before implementation.

- [ ] **Step 3: Implement local boundary**

Restrict key inclusion to localhost requests without browser `Origin`; echo only allowed local origins for CORS.

- [ ] **Step 4: Verify**

Run: `cd local-backend && go test ./internal/localagent`.

### Task 2: Cloud Route Auth Wrapping

**Files:**
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Modify: relevant cloud handler route registration methods only if needed for optional middleware.
- Add or modify focused tests in existing packages.

**Interfaces:**
- Consumes: `authMiddleware.RequireAuth() gin.HandlerFunc`
- Produces: agent runtime, artifact, model-provider config, and publish-compatible mutation routes protected by auth.

- [ ] **Step 1: Add tests for optional middleware route registration**

Tests should prove handlers can install middleware without breaking existing tests that call `RegisterRoutes(r)`.

- [ ] **Step 2: Run focused cloud tests to verify failure**

Run targeted `go test` commands for changed packages.

- [ ] **Step 3: Implement optional middleware and wire it in `main.go`**

Keep public docs/health/auth routes unchanged.

- [ ] **Step 4: Verify**

Run targeted package tests, then `cd cloud-backend && go test ./... && make api-docs-check`.

### Task 3: Frontend Workbench Polish And Safety

**Files:**
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/tailwind.config.js`
- Add: `frontend/eslint.config.js`

**Interfaces:**
- Consumes: `serviceStatus`, `react-markdown`, existing director logic exports.
- Produces: responsive director layout, truthful local status, safe Markdown rendering, working ESLint command.

- [ ] **Step 1: Verify current failing lint**

Run: `cd frontend && npm run lint`
Expected: FAIL due missing ESLint 9 flat config.

- [ ] **Step 2: Implement frontend changes**

Remove fixed director min-width, improve responsive grid classes, replace `dangerouslySetInnerHTML` with `ReactMarkdown`, wire sidebar status to `serviceStatus`, and add ESLint 9 flat config.

- [ ] **Step 3: Verify frontend**

Run: `cd frontend && npm run lint && npm run test:director && npm run build`.

### Task 4: Full Verification

**Files:**
- No direct file changes.

- [ ] **Step 1: Run full test/build suite**

Run:
`cd local-backend && go test ./... && go vet ./...`
`cd cloud-backend && go test ./... && go vet ./... && make api-docs-check`
`cd frontend && npm run lint && npm run test:director && npm run build`

- [ ] **Step 2: Report residual risks**

Include any remaining visual tradeoffs, large-file refactor follow-up, and routes intentionally left public.
