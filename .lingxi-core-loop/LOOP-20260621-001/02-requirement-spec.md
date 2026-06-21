# Requirement Spec

## Functional Requirements

- REQ-001 Users can register with email, password, and optional nickname.
- REQ-002 Users can log in with email and password.
- REQ-003 Cloud returns an access token and refresh token after successful register/login.
- REQ-004 Cloud supports refresh token rotation.
- REQ-005 Cloud supports logout by revoking the active refresh token.
- REQ-006 Cloud supports `GET /api/auth/me`.
- REQ-007 Auth middleware injects `user_id` and optional `device_id` into request context.
- REQ-008 Protected cloud resource handlers use context `user_id`, not request body or query `userId`.
- REQ-009 Media assets, video projects, and bid projects are scoped by authenticated user.
- REQ-010 Frontend blocks cloud product UI until it has a valid login state.
- REQ-011 Frontend attaches `Authorization: Bearer <access_token>` to cloud API calls.
- REQ-012 Local model provider API keys remain stored only by the local desktop agent.

## Non-Requirements

- REQ-NON-001 No RBAC.
- REQ-NON-002 No OAuth or SSO.
- REQ-NON-003 No team or organization model.
- REQ-NON-004 No cloud storage of user model provider API keys.
