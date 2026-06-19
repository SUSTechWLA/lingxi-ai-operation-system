# 04c Workflow Spec

## Skill Manifest Extensions

Stage fields added:

- `kind`: `TOOL`, `CONTROL`, `APPROVAL`, or omitted for default agent stage.
- `tool`: external tool name for `TOOL` stages.
- `input`: static parameters merged into `input.parameters`.
- `long_running`: marks node for heartbeat/progress support.
- `heartbeat_timeout_sec`: per-node timeout override.

## Compiler Contract

- Default stage: `TOOL` node named `external`, `parameters.tool=skill_stage_agent`.
- Tool stage: `TOOL` node named `external`, `parameters.tool=<stage.tool>`.
- Approval required: execution node followed by `CONTROL` node.
- Optional stage: creates a `skip` CONTROL branch and an `exec` branch. Upstream connects to both branch entries; downstream connects to skip and execution/approval outputs.
- Stable template ID: `TemplateIDForSkill(name, version)`.
