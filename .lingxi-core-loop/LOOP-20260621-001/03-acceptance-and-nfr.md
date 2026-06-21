# Acceptance And NFR

## Acceptance Criteria

- AC-001 Register returns user profile, access token, refresh token, and expiry.
- AC-002 Login returns the same shape and updates last login time.
- AC-003 `/api/auth/me` returns the current user only when a valid access token is present.
- AC-004 Refresh revokes the old refresh token and returns a replacement.
- AC-005 Logout revokes the submitted refresh token for the authenticated user.
- AC-006 Missing, malformed, expired, or wrong-type access tokens return 401.
- AC-007 Disabled users cannot log in or use protected APIs.
- AC-008 New media, video projects, and bid projects use the authenticated user's ID.
- AC-009 User A cannot read/list/update/delete User B's protected resources.
- AC-010 Frontend retries one refresh on 401 and then returns to login if refresh fails.
- AC-011 Local model provider GET responses do not expose full API keys.

## Non-Functional Requirements

- NFR-001 Passwords use a slow password hash.
- NFR-002 Refresh tokens are persisted only as hashes.
- NFR-003 Access tokens are short-lived.
- NFR-004 Existing generated API docs remain generator-owned.
- NFR-005 No database or cloud-service dependency is added to `local-backend`.
- NFR-006 Tests cover auth, middleware, and resource isolation.

## Risks

- RISK-001 Existing rows owned by `default` need a compatibility strategy.
- RISK-002 Protecting every legacy route at once can break development flows. First pass protects the user-owned resource sets in scope.
- RISK-003 Browser storage is acceptable for first-version development/Electron but should be replaced by HttpOnly cookies or secure Electron storage later.
