# ADR-001 Skill Workflow Runtime

## Decision

Production AIOS skills are versioned runtime packages under `skills/<name>/<version>/` and compile into DAG templates. Codex `SKILL.md` files remain source methodology, not the production runtime format.

## Consequences

- Core can load, validate, compile, and version-lock skills without depending on Codex internals.
- Media-specific actions are modeled as external tools.
- Stage gates and long-task behavior become reusable platform features.
