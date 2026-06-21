# User Login And Model Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-version user login, token authentication, user resource isolation, and frontend login flow while preserving local-only model provider secrets.

**Architecture:** `cloud-backend/internal/core/auth` owns auth APIs, token service, middleware, repositories, and context helpers. Existing user-owned services consume `auth.UserIDFromContext(ctx)` and scope database queries by user. Frontend uses a small auth service and axios interceptors; local model provider settings stay in `local-backend`.

**Tech Stack:** Go 1.25, Gin, pgx, `golang.org/x/crypto/bcrypt`, HMAC-SHA256 tokens, React 18, TypeScript 5.5, axios.

## Global Constraints

- Do not add PostgreSQL, Redis, Kafka, MinIO, Docker, or LLM API key dependencies to `local-backend`.
- Do not store user model provider API keys in cloud PostgreSQL.
- Do not trust frontend-supplied `user_id` values for protected resources.
- Keep responses in the existing `{ code, message, data }` envelope.
- Update OpenAPI source and regenerate generated docs/types after route changes.

---

### Task 1: Cloud Auth Core

**Files:**
- Create: `cloud-backend/internal/core/auth/model.go`
- Create: `cloud-backend/internal/core/auth/password.go`
- Create: `cloud-backend/internal/core/auth/token.go`
- Create: `cloud-backend/internal/core/auth/context.go`
- Create: `cloud-backend/internal/core/auth/repository.go`
- Create: `cloud-backend/internal/core/auth/service.go`
- Test: `cloud-backend/internal/core/auth/auth_test.go`
- Modify: `cloud-backend/internal/core/database/database.go`

**Interfaces:**
- Produces: `Service.Register(ctx, RegisterRequest, ClientInfo) (*AuthResponse, error)`
- Produces: `Service.Login(ctx, LoginRequest, ClientInfo) (*AuthResponse, error)`
- Produces: `Service.Refresh(ctx, RefreshRequest, ClientInfo) (*AuthResponse, error)`
- Produces: `Service.Logout(ctx, userID string, refreshToken string) error`
- Produces: `UserIDFromContext(ctx context.Context) (string, bool)`

- [ ] **Step 1: Write failing auth package tests**

```bash
cd cloud-backend && go test ./internal/core/auth -run 'TestPassword|TestToken|TestService' -count=1
```

Expected: FAIL because `internal/core/auth` does not exist.

- [ ] **Step 2: Implement auth package**

Use bcrypt password hashing, HMAC-SHA256 signed access tokens, random refresh tokens, SHA-256 refresh token hashes, and repository methods backed by `users`, `refresh_tokens`, and `devices`.

- [ ] **Step 3: Add migrations**

Add `users`, `refresh_tokens`, and `devices` tables to `database.RunMigrations`. Add missing `workflow_runs.user_id` and `model_calls.user_id` columns with safe defaults.

- [ ] **Step 4: Run auth tests**

```bash
cd cloud-backend && go test ./internal/core/auth -count=1
```

Expected: PASS.

### Task 2: Auth HTTP And Middleware

**Files:**
- Create: `cloud-backend/internal/core/auth/handler.go`
- Create: `cloud-backend/internal/core/auth/middleware.go`
- Test: `cloud-backend/internal/core/auth/handler_test.go`
- Modify: `cloud-backend/cmd/tangying-ai-os/main.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_spec.go`
- Modify: `cloud-backend/internal/core/apispec/cloud_schemas.go`

**Interfaces:**
- Consumes: Task 1 `auth.Service`
- Produces: `POST /api/auth/register`
- Produces: `POST /api/auth/login`
- Produces: `POST /api/auth/refresh`
- Produces: `POST /api/auth/logout`
- Produces: `GET /api/auth/me`
- Produces: `auth.Middleware.RequireAuth() gin.HandlerFunc`

- [ ] **Step 1: Write failing handler and middleware tests**

```bash
cd cloud-backend && go test ./internal/core/auth -run 'TestHandler|TestMiddleware' -count=1
```

Expected: FAIL because handlers and middleware are missing.

- [ ] **Step 2: Implement handler and middleware**

Register public auth routes, protect logout/me, parse `DeviceID`, `User-Agent`, and client IP, and inject user/device into request context.

- [ ] **Step 3: Wire main server**

Instantiate auth repository/service and register auth routes before protected groups. Update CORS allowed headers to include `DeviceID`.

- [ ] **Step 4: Update OpenAPI source**

Add Auth tag, auth routes, and schema names. Do not hand-edit generated markdown or TypeScript.

- [ ] **Step 5: Run tests**

```bash
cd cloud-backend && go test ./internal/core/auth ./internal/core/apispec -count=1
```

Expected: PASS.

### Task 3: Cloud Resource Isolation

**Files:**
- Modify: `cloud-backend/internal/core/media/handler.go`
- Modify: `cloud-backend/internal/core/media/service.go`
- Modify: `cloud-backend/internal/agents/video/handler/project_handler.go`
- Modify: `cloud-backend/internal/agents/video/service/project_service.go`
- Modify: `cloud-backend/internal/agents/video/repository/project_repo.go`
- Modify: `cloud-backend/internal/agents/bid/handler/handler.go`
- Modify: `cloud-backend/internal/agents/bid/service/service.go`
- Modify: `cloud-backend/internal/agents/bid/repository/repository.go`
- Tests: package-level tests for each modified module

**Interfaces:**
- Consumes: Task 2 auth context helpers
- Produces: project/media/bid operations scoped by authenticated user

- [ ] **Step 1: Write failing isolation tests**

Tests must show that a request with user A cannot list or fetch user B resources, and that creates use user A even if the payload contains a different user ID.

- [ ] **Step 2: Implement scoped repository methods**

Add `FindByIDForUser`, `FindAllForUser`, `UpdateForUser`, and delete/update scoping where needed.

- [ ] **Step 3: Update handlers/services**

Read `user_id` from context and return 401 if missing. Remove trusted use of query/body `userId`.

- [ ] **Step 4: Run scoped tests**

```bash
cd cloud-backend && go test ./internal/core/media ./internal/agents/video/... ./internal/agents/bid/... -count=1
```

Expected: PASS.

### Task 4: Frontend Auth Flow

**Files:**
- Create: `frontend/src/services/auth.ts`
- Create: `frontend/src/components/AuthScreen.tsx`
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/Sidebar.tsx`
- Modify: `frontend/src/utils/types.ts`

**Interfaces:**
- Consumes: Task 2 Auth APIs
- Produces: logged-out login/register screen
- Produces: axios access-token attachment and refresh retry

- [ ] **Step 1: Write failing frontend type/build check**

```bash
cd frontend && npm run build
```

Expected: FAIL after adding references to auth exports before implementation, or use focused TypeScript compile failures during development.

- [ ] **Step 2: Implement auth service**

Store tokens in memory plus existing browser storage for development/Electron first version. Provide register, login, refresh, logout, me, and subscribe helpers.

- [ ] **Step 3: Implement UI**

Add a compact login/register screen and display current user/logout in the sidebar. Keep the first app screen as the usable product once authenticated.

- [ ] **Step 4: Run frontend build**

```bash
cd frontend && npm run build
```

Expected: PASS.

### Task 5: Docs, Generated API, And Verification

**Files:**
- Modify generated docs/types only through generators:
  - `cloud-backend/docs/API_REFERENCE.md`
  - `frontend/src/utils/api-types.generated.ts`

**Interfaces:**
- Consumes: Tasks 1-4
- Produces: synchronized docs and final verification evidence

- [ ] **Step 1: Regenerate cloud API docs and types**

```bash
cd cloud-backend && make gen-docs
```

Expected: command exits 0 and generated files include Auth routes.

- [ ] **Step 2: Check API docs drift**

```bash
cd cloud-backend && make api-docs-check
```

Expected: PASS.

- [ ] **Step 3: Run backend tests**

```bash
cd local-backend && go test ./...
cd ../cloud-backend && go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run frontend build**

```bash
cd frontend && npm run build
```

Expected: PASS.
