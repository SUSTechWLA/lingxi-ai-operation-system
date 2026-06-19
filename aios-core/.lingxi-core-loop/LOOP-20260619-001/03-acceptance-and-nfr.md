# 03 Acceptance And NFR

## Acceptance Criteria

- AC-001: `go run ./cmd/skill2workflow --skill-root skills` compiles all target runtime packages.
- AC-002: Default agent stages compile to `TOOL` nodes named `external` with `parameters.tool=skill_stage_agent`.
- AC-003: Declared tool stages compile to `TOOL` nodes named `external` with the declared external tool under `parameters.tool`.
- AC-004: Long-running stage metadata appears in compiled DAG nodes.
- AC-005: Optional stage exec branch depends on previous stage output; approval cannot be reached by bypassing exec.
- AC-006: CONTROL nodes become READY, pause the task, and do not publish `ai.node.ready`.
- AC-007: Approval success wakes downstream nodes and resumes a paused workflow task.
- AC-008: Skill auto-registration uses deterministic ID `wf-<skill>-<version>` and upserts instead of creating duplicates.

## NFR

- NFR-001: Existing workflow API remains backward compatible.
- NFR-002: Existing `skill.yaml` packages without new fields still compile.
- NFR-003: External tool internals stay outside Core.
- NFR-004: All changed Go packages pass tests; full `go test ./...` must pass.
