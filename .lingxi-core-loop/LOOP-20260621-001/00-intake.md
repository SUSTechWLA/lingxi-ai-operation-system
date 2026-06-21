# Intake

## Request

Upgrade the current system according to `.lingxi-core-loop/增加用户登陆和模型配置功能.md`.

## Selected Scope

- User registration and login.
- Token authentication, refresh, logout, and current user information.
- Server-side request context injection of `user_id`.
- User isolation for cloud resources that already carry user ownership.
- Frontend login state and token handling.
- Preserve local-only model provider API key storage.

## Explicitly Out Of Scope

- RBAC, teams, organizations, OAuth, SSO, admin console, enterprise approval, and cloud storage of user model provider secrets.

## Initial Boundary

Cloud account and user resource isolation belong in `cloud-backend`. Local model provider API keys remain in `local-backend` and must not gain cloud database or service dependencies.
