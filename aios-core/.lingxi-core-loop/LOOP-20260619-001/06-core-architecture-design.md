# 06 Core Architecture Design

## Design

1. Extend `StageDefinition` with optional execution metadata.
2. Compile every executable skill stage through the existing `external` bridge.
3. Keep external tool names inside `input.parameters.tool` so `DetermineToolName` selects the bridge, not the external tool name.
4. Treat CONTROL nodes as orchestration wait states: READY + task PAUSED + review context, no worker dispatch.
5. Trigger dependency checking from `StateMachine.OnSuccess` so manual approval and Kafka success share the same continuation path.
6. Use `TemplateIDForSkill` and `workflow.Service.Upsert` for stable startup registration.

## Rollback

- Revert changed Go files and remove added `skills/<target>/1.0.0` packages.
- Existing workflow templates created through API remain unaffected.
