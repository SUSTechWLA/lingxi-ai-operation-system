# Core Gap Decision

## Decision

`CORE_CHANGE_CANDIDATE`

## Rationale

The requirement cannot be solved purely by frontend changes or workflow configuration. The cloud backend needs a general account identity layer, authenticated request context, and user resource isolation. These are platform-wide mechanisms and belong to the cloud AIOS Core boundary.

## Core Admission Check

- General platform capability: yes.
- No customer-specific business rules: yes.
- Reusable across media, video, bid, workflow, logs, and future model profiles: yes.
- Cannot be implemented safely by frontend-provided `user_id`: yes.
- Stable boundary: yes, cloud auth and local provider secret storage stay separate.
- Testable: yes.
- Compatible and rollback-capable: yes, first version can preserve default rows and protect scoped APIs incrementally.

## Rejected Alternatives

- Frontend-supplied `user_id`: rejected because it is not trustworthy.
- Cloud model API key storage: rejected for first version because current architecture keeps user-provided provider secrets in the local desktop runtime.
- Full IAM/RBAC: rejected as out of scope for first version.
