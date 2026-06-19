# 08 Core Test Plan

## Automated Tests

- Orchestrator service tests for CONTROL node pause/no dispatch and approval downstream wakeup.
- Workflow compiler tests for default agent routing, declared external tool routing, long-running metadata, optional stage dependencies, and stable template IDs.
- Existing workflow/skillruntime/orchestrator tests.
- Full `go test ./...`.

## Manual/CLI Verification

- `go run ./cmd/skill2workflow --skill-root skills --output /tmp/aios-skill-dags`.
- Inspect generated DAG for `create-opinion-videos`.

## Not Covered In This Loop

- Real external media providers.
- End-to-end rendering with HyperFrames/Seedance/audio DSP.
