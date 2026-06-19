# 04 Core Gap Decision

Decision: `CORE_CHANGE_CANDIDATE`.

## Why Core Change Is Required

- Existing configuration/workflow alone cannot fix compiled DAGs pointing at non-existent `skill_stage` tool.
- Existing CONTROL node behavior is a platform orchestration bug: review gates dispatch to worker and manual approval does not wake downstream.
- Stable skill workflow registration requires deterministic template IDs and idempotent startup registration.
- Stage long-running metadata and external tool routing are generic platform concerns reusable beyond these 4 skills.

## Why Business Logic Is Not In Core

The implementation only adds runtime package manifests, compiler semantics, orchestration behavior, and external capability contracts. Video/audio algorithms remain external tools.
