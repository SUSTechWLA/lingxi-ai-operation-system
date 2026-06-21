# Core Discovery

## Architecture Inputs

`docs/ARCHITECTURE.md` defines the repository split:

- `frontend/`: React and Electron UI.
- `local-backend/`: local desktop agent, no DB or Docker dependency.
- `cloud-backend/`: AIOS Core cloud backend.

The architecture states that the local desktop side owns user-provided base model Provider settings. Cloud owns remote configuration, orchestration, account system, cloud logs, and diagnostics analysis.

## Relevant Existing Capabilities

- `local-backend/internal/localagent/server.go` already exposes `GET/PUT /api/local/model-providers`.
- `local-backend/internal/localagent/server_test.go` verifies that full API keys are stored locally and GET responses are masked.
- `cloud-backend/internal/core/database/database.go` already has `user_id` on `media_assets`, `video_projects`, and `bid_projects`, with current defaults of `default`.
- `frontend/src/pages/DesktopPage.tsx` already renders local model provider fields for text, image, and video capabilities.

## Current Gaps

- No cloud Auth API, users table, refresh token table, or auth middleware exists.
- Several handlers accept or default frontend-provided user identifiers.
- `video_projects` are created with hard-coded `UserID: "default"`.
- `bid_projects` accept `req.UserID`.
- `media` upload/list reads `userId` from form/query.
- Frontend has no login/register gate and axios does not attach tokens.

## Boundary Finding

Model provider API key storage is already on the correct side of the runtime boundary. The required upgrade should preserve this behavior and avoid adding cloud model-key persistence in the first version.
