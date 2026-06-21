# Delivery Report

## Decision

This change required Core updates because trusted user identity and user resource isolation cannot be implemented safely by frontend-supplied `user_id`.

## Delivered Behavior

- Added cloud email/password registration, login, refresh-token rotation, logout, and current-user API.
- Added auth middleware that validates bearer access tokens and injects `user_id` plus optional `device_id` into request context.
- Added PostgreSQL migrations for `users`, `refresh_tokens`, `devices`, `workflow_runs.user_id`, and `model_calls.user_id`.
- Scoped media assets, video projects, bid projects, and new workflow runs to authenticated users.
- Added frontend login/register screen, token persistence, axios bearer-token injection, one refresh retry on 401, and sidebar logout/user display.
- Preserved local-only model provider secret storage in `local-backend`.
- Updated OpenAPI source, generated markdown docs, and generated frontend API types.

## Evidence

- `cd cloud-backend && make api-docs-check && go test ./... && go vet ./...` passed.
- `cd local-backend && go test ./... && go vet ./...` passed.
- `cd frontend && npm run build` passed.
- `git diff --check` passed.

## Risks And Follow-Up

- First-version frontend token storage uses browser/Electron storage for development usability. A production web deployment should move refresh tokens to HttpOnly Secure SameSite cookies or Electron secure storage.
- Existing rows owned by `default` remain for compatibility. A later migration can assign legacy rows to real users after product/account migration decisions.
- RBAC, teams, OAuth, SSO, and cloud model API key storage remain out of scope.
