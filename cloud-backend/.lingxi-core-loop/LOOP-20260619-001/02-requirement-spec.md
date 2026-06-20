# 02 Requirement Spec

## Functional Requirements

- REQ-001: Core must load production skill packages for the 4 target business skills.
- REQ-002: Skill stages must express default agent stages, external tool stages, optional stages, approval gates, and long-running metadata.
- REQ-003: Compiler must emit executable DAG nodes using the existing `external` bridge contract.
- REQ-004: Approval/CONTROL nodes must pause workflow and never dispatch to worker automatically.
- REQ-005: Manual node success for approval must wake downstream dependencies.
- REQ-006: Skill workflow templates must have stable IDs derived from skill name and version.
- REQ-007: Startup auto-registration must be idempotent.
- REQ-008: The 3 video workflows must preserve human review gates before expensive image/video/render stages.

## Non-Goals

- Do not implement video generation providers, HyperFrames internals, FFmpeg internals, audio DSP internals, or Codex skill execution.
- Do not add IAM, online approval center, or customer frontend code.
- Do not put video-specific business rules inside orchestrator or worker core.
