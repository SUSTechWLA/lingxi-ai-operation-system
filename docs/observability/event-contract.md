# Observability event contract

The cloud backend emits the versioned `1.0` lifecycle envelope defined in
[`contracts/observability/v1/event.schema.json`](../../contracts/observability/v1/event.schema.json).
The schema and stable error registry are checked without network access by
`npm run test:observability-contract`; the complete foundation gate is
`npm run test:observability-foundation`.

## Lifecycle and correlation

Critical operations emit one start event and exactly one terminal event. The
cloud creation path covers Agent setup, planning, plan validation and
correction, plan compilation, DAG submission, translator LLM/DAG execution,
workflow run and stage transitions, cloud tool calls, retries, Verify/Correct
tools, translator fallback selection, and local-job completion or failure
callbacks.

The outer `agent.run.*` pair describes creation and submission of an Agent run;
successful submission leaves the persisted run in `RUNNING`. Internal Agent
planning, compilation, and DAG submission use `workflow.stage.*` with fixed
stage identifiers. Plan validation uses `agent.plan.validation.*`; correction
uses `correct.operation.*`, and the configured plan judge owns the genuine
`verify.check.*` boundary.

Agent deadlines are failures, not cancellations: the nested boundary uses
`WORKFLOW.STAGE.TIMEOUT` and the outer run uses `AGENT.RUN.TIMEOUT`.
Explicit cancellation and a persisted run-cancellation request remain
`WARN`/`CANCELLED` terminals without an error object.
Translator LLM cancellation uses `llm.call.cancelled`, and Agent plan-judge
cancellation uses `verify.check.cancelled`; both are closed v1 event types with
`WARN` severity, `CANCELLED` execution status, and `error: null`.
Canonical examples are
[`example-llm-call-cancelled.json`](../../contracts/observability/v1/example-llm-call-cancelled.json)
and
[`example-verify-check-cancelled.json`](../../contracts/observability/v1/example-verify-check-cancelled.json).

HTTP middleware accepts W3C `traceparent`. Kafka propagation reconstructs the
same correlation context. Producers may supply existing domain IDs (`wfr-`,
UUIDs, or legacy values); the emitter converts them to schema-valid product IDs.
Arbitrary caller identifiers are deterministically SHA-256 mapped and are not
placed on the wire unchanged.

## Privacy boundary

Events contain IDs/references, byte counts, safe status values, attempts,
durations, stable error metadata, and SHA-256 input hashes only. They never
contain request text, prompts, the translator system/full prompt, workflow
input/output, local-job payload/output/diagnostics, tool arguments/results,
tokens, credentials, or user comments. Zap diagnostics likewise omit raw
`userInput` and translator `prompt` fields. Agent tool-trace logs contain only
bounded safe tool names and counts; background, translator, and worker failure
logs use registry code/class and a static code-plus-component fingerprint, not
raw error text or a hash of provider/request content.

`ERROR` severity is reserved for terminal events carrying a normalized code
from the stable registry; both the Go validator and JSON schema reject an
`ERROR` event without structured error metadata. Cancellation remains a
`WARN`/`CANCELLED` terminal event. The JSON Schema `error.code` enum is required
to be exactly identical, including order and duplicate absence, to the stable
registry.

Emitter failure never fails a user workflow. Queue rejection or validation
failure produces a bounded structured diagnostic containing only component,
event type, and `observability.emit.failed`. The shared emitter retains its
bounded-queue policy: low-priority progress can be evicted; protected lifecycle
events return an explicit overflow diagnostic rather than disappearing
silently.

Local-job terminal observability is a separate durable callback phase. The
terminal event ID, occurrence time, duration, attempt, correlation, and failure
code are derived from the persisted job row. Queue rejection or sink failure
leaves that phase pending for authenticated reconciliation while product result
and follow-up callbacks continue independently; a sink acknowledgement is
required before the observability phase is marked delivered. The repository is
the required sink. Structured logging is an explicitly best-effort secondary
sink: it is always attempted and failures are counted, but a logger outage does
not prevent durable acknowledgement; a repository failure always leaves the
phase pending for retry.

## Tool snapshot

Each new Agent or workflow run records the immutable tool-registry snapshot ID
and SHA-256 in its run manifest. Canonical tool entries and their string lists
are sorted before serialization, so equivalent registries produce byte-identical
snapshots. A request-scoped snapshot remains unchanged through planning,
validation, correction, compilation, and submission.

## Cloud relay

Authenticated clients use:

- `GET /api/observability/events` for scoped, cursor-paginated delivery;
- `POST /api/observability/events/ack` for idempotent acknowledgement;
- `GET /api/observability/runs/:runId/summary` for a scoped run summary.

Relay rows are redacted before persistence and expire after 72 hours. The
delivery cursor is the stable `occurredAt` plus `eventId` ordering, and pulls
are capped at 500 events.

The translator begins its LLM, stage, and fallback lifecycle before the
orchestrator returns a task identity. Only component `translator`, stage
`translator-llm-dag`, and the fixed emitted LLM/stage/fallback event-type plus
message-key pairs are accepted as runless bootstrap events. They are stored in
the authenticated owner's outbox with no fabricated run summary. Lookalike
components, stages, types, or message keys remain rejected.
