# 05 Core Impact Analysis

## Packages Changed

- `internal/core/skillruntime`: stage manifest fields.
- `internal/core/workflow`: compiler, template model/repository/service/seed, tests.
- `internal/core/orchestrator/service`: CONTROL node gating and downstream wakeup.
- `internal/core/model` and `internal/core/database`: context type constants/check constraint.
- `cmd/skill2workflow` and `cmd/tangying-ai-os`: CLI parsing and startup upsert.
- `skills/*`: runtime packages for target skills.

## Compatibility

- Old manifests without `kind`, `tool`, or long-running fields still compile.
- Existing workflow create API can omit `id` and `version`.
- Existing random-ID create behavior remains for client-created templates.

## Risks

- External tools must be registered before real workflow execution.
- Kafka consumer still calls dependency checker after `StateMachine.OnSuccess`; the new internal call is idempotent but produces duplicate checks.
