# User Login And Model Configuration Design

## Goal

Implement first-version cloud account login and user resource isolation while keeping user-provided model provider secrets in the local desktop runtime.

## Scope

- Add email/password registration and login to `cloud-backend`.
- Add access token, refresh token rotation, logout, and current-user APIs.
- Add request-context helpers so business handlers read `user_id` from the authenticated request.
- Protect user-owned resources in video projects, bid projects, and media assets.
- Add frontend login/register flow and axios authentication handling.
- Keep local model provider API keys in `local-backend` only; do not upload them to cloud storage or PostgreSQL.

## Out Of Scope

- RBAC, teams, organizations, OAuth, SSO, admin console, and cloud approval workflows.
- Cloud storage for user model API keys.
- Database, Redis, Kafka, MinIO, Docker, or LLM API key dependencies in `local-backend`.
- Enterprise multi-tenant policy beyond per-user row isolation.

## Architecture

Cloud authentication lives under `cloud-backend/internal/core/auth`. The package owns password hashing, token issuing, refresh token persistence, middleware, and context helpers. Auth routes are public. User-owned API groups use middleware to validate the access token and inject `user_id` and optional `device_id` into `context.Context`.

The first protected resource set is deliberately narrow: media assets, video projects, and bid projects. Those modules already carry `user_id` columns or defaults, so the change is to stop accepting user IDs from request payloads and query strings, scope repository operations by authenticated user, and preserve existing default data by migrating blank values to `default`.

Frontend authentication is implemented in the existing React app without adding a separate state framework. Tokens are stored in a small auth service. Axios attaches access tokens and attempts one refresh on 401. The app shows a login/register screen until `/auth/me` succeeds.

Local model provider settings remain in `local-backend/internal/localagent`. The existing behavior of storing full API keys locally and returning only masked previews remains the accepted design. The system page can display login state next to the local provider settings, but the provider API key is never sent to `cloud-backend`.

## API

Public routes:

- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/refresh`

Authenticated routes:

- `POST /api/auth/logout`
- `GET /api/auth/me`

Request and response bodies follow the existing `{ code, message, data }` envelope.

## Data

New cloud tables:

- `users`: id, email, password_hash, nickname, avatar_url, status, timestamps, last_login_at.
- `refresh_tokens`: id, user_id, token_hash, device_id, user_agent, ip_address, expires_at, revoked_at, replaced_by_token_id, created_at.
- `devices`: id, user_id, device_name, device_type, platform, last_seen_at, timestamps.

Existing user-owned tables:

- `media_assets.user_id`
- `video_projects.user_id`
- `bid_projects.user_id`
- `ai_task.user_id`
- `workflow_runs.user_id`
- `model_calls.user_id`

The first implementation must at least enforce scoping for media, video projects, and bid projects. Other tables get migration columns where missing and may be wired incrementally as part of workflow start paths.

## Security

- The frontend must not send trusted `user_id` values for user-owned cloud resources.
- Passwords are stored with a slow one-way hash.
- Refresh tokens are stored as SHA-256 hashes, never as plaintext.
- Access tokens are signed and short-lived.
- Refresh tokens are rotated on refresh and revoked on logout.
- Disabled or deleted users cannot authenticate.
- Local provider API keys remain local and masked in GET responses.

## Error Handling

- Invalid login credentials return the same message for unknown email and wrong password.
- Missing or malformed authorization returns 401.
- Expired access token returns 401 and lets the frontend attempt refresh.
- Authenticated access to another user's resource returns 404 to avoid leaking existence.
- Refresh token reuse after rotation returns 401.

## Testing

- Unit tests for password hashing, token claims, refresh token hashing, and context helpers.
- Handler tests for register/login/refresh/logout/me.
- Middleware tests for missing token, bad token, expired token, disabled user, and successful context injection.
- Resource isolation tests for media, video project, and bid project handlers/services.
- Frontend tests or build verification for auth service and axios interceptor integration.
- Existing local-backend model provider tests must continue to pass.

## Acceptance

- A new user can register and immediately receive tokens.
- A registered user can log in, call `/auth/me`, refresh tokens, and log out.
- Cloud user-owned resources are created with the authenticated user's ID.
- User A cannot list, fetch, update, or delete User B's protected media, video project, or bid project resources.
- Frontend starts unauthenticated, lets a user register or log in, and then calls cloud APIs with `Authorization`.
- Local model provider API keys remain only in the local agent config file and are not returned in full by GET.
