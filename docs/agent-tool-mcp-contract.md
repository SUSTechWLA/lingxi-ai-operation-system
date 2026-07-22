# Agent, Tool, and MCP Contract

Tangying AIOS uses one contract from planning through local execution. The
contract keeps model context, tool definitions, safety decisions, result
validation, and recovery behavior separate, while allowing any conforming MCP
provider to register dynamically.

## Five-layer runtime contract

### 1. Context

Context gives the model the information required to make a decision: the user
request, project and shot state, accepted artifacts, relevant retrieved
knowledge, available capabilities, and the current workflow state. The runtime
keeps the full request context for authorized provider authentication, but the
planner prompt receives a sanitized projection. API keys, authorization
headers, tokens, cookies, passwords, and equivalent secret-bearing fields must
never enter a prompt, tool description, log, diagnostic archive, or public API
response.

Context is evidence, not authority. A value appearing in context does not grant
permission to run a tool or cross a local/cloud boundary.

### 2. Tools

Every tool has one canonical definition:

- a stable, unique `name`;
- a concise `description` of observable behavior;
- an `inputSchema` expressed as JSON Schema with `type: object`, explicit
  `properties`, and a valid `required` list;
- an optional output/result schema;
- execution-plane, capability, approval, timeout, and provider-binding
  metadata.

`LLMToolDefinition` and its four adapters are a provider-boundary library:
OpenAI Responses, OpenAI Chat Completions, Anthropic, and Gemini wire formats
can be derived without changing the canonical registry. The current production
planner does **not** pass these definitions as provider-native function tools;
it asks a Chat Completions model for one JSON `AgentPlan` DAG. The adapter
library is therefore a validated integration boundary for future native tool
calling, not a claim that native calling is active today. If a provider cannot
represent a schema feature without changing its meaning, the adapter fails
closed instead of silently weakening the contract.

`strict` means the model-generated arguments must satisfy the declared schema;
it does not authorize execution. Authorization remains a separate constraint
decision.

### 3. Constrain

Constraints establish what may happen before a tool runs. The Guard evaluates:

- policy and capability allowlists;
- local versus cloud execution boundaries;
- user-device and filesystem scope;
- provider and tool enable/disable rules;
- explicit approval requirements;
- secret handling and header redaction;
- dependency ordering and quality-gate relationships;
- time, retry, and resource limits.

Unknown dependencies, cycles, forged validation gates, unsupported transports,
invalid schemas, and missing approvals are blocking errors. Registration alone
never grants a provider unrestricted execution.

### 4. Verify

Verification decides whether an executed step produced the promised result. A
step is checked against its result schema, declared artifacts, status, quality
gate, and downstream invariants. Guard only accepts a quality gate that the
runtime derived from the step it validates; caller-supplied or duplicated gate
metadata cannot bypass the relationship.

Verification is independent of an executor returning exit code zero. Missing
artifacts, malformed structured results, failed quality thresholds, or a result
that cannot be attributed to the selected provider remain failures.

Explicit canonical schemas are compiled at registration using Draft 7 or Draft
2020-12 semantics. Remote `$ref` loading is disabled, schema size and nesting
are bounded, and invalid registration is rejected before database or registry
mutation. At execution, unresolved exact output references fail with
`INPUT_REFERENCE_UNRESOLVED`; resolved arguments and outputs fail with
`INPUT_SCHEMA_INVALID` or `OUTPUT_SCHEMA_INVALID`. MCP tool wrappers validate
their `structuredContent` against the remote tool's `outputSchema`; native
local tools validate the root output object.

### 5. Correct

Correction is scoped recovery after a verified failure. It may retry a
transient call within budget, repair only the invalid plan fields, choose an
allowed fallback, or roll back a reversible local operation. Every repair is
revalidated by the same schema and Guard rules. Exhausted retry budgets,
authorization failures, ambiguous destructive actions, and invalid repair
plans fail closed and are surfaced for user action.

## Current planning and execution loop

The current runtime is a workflow loop, not a provider-native assistant
`tool_calls` loop:

1. The planner receives sanitized context and compact canonical tool
   candidates, then a Chat Completions request returns one JSON `AgentPlan`.
2. `PlanCompiler.PreparePlan` normalizes that plan, and `PlanGuard` validates
   dependencies, policy, canonical input schemas, and precise output
   references.
3. The compiler creates a DAG. Every executable node carries pure
   `contractArguments` separately from transport, intent, artifact, and routing
   metadata.
4. Executors resolve references, validate the resolved logical input, run the
   cloud or local tool, validate its canonical output, and publish a tool result
   event.
5. Quality gates, retry policy, and repair planning verify or correct the
   workflow result.

The runtime does not currently send provider-native assistant tool-call IDs or
tool result messages back into another assistant turn. Assistant messages are
allowed by the wider Agent model to contain content or `tool_calls`, but that
optional multi-turn provider behavior is not implemented by this JSON-DAG
planner and must not be inferred from the adapter types.

The product must not claim to expose or persist a model's private chain of
thought. It records auditable decisions instead: sanitized inputs, selected
tool, validated arguments, approval decision, execution result, verification
outcome, retry count, and final user-facing explanation.

## MCP provider contract

MCP providers follow the standard lifecycle:

1. Client sends `initialize` and negotiates the protocol version and
   capabilities.
2. Client sends `notifications/initialized`.
3. Client discovers tools with paginated `tools/list`.
4. Client invokes an allowed tool with `tools/call`.
5. Client closes the session and transport within its deadline.

The local Agent supports standard `stdio` and Streamable HTTP transports.
Standard input/output servers reserve stdout for MCP JSON-RPC; diagnostics go
to stderr. Streamable HTTP sessions preserve negotiated session state and do
not open a legacy standalone SSE channel unless the negotiated protocol
requires it. Initialize, listing, calls, pagination, and shutdown all have
bounded contexts.

Enabled providers are discovered at runtime. Their remote tools are mapped to
canonical manifests and executed through the single
`LOCAL_MCP_TOOL_CALL` command. Adding a provider must not add a new
provider-specific planner, compiler branch, JSON-RPC client, or runner command.
The production client uses the official MCP SDK instead of handwritten protocol
messages.

Discovery does not mutate the cloud-global tool catalog. The runner advertises
only the safe `tools/list` projection (provider ID, normalized logical and
remote names, schemas, standard annotation hints, timeout, approval mode, and a
SHA-256 catalog revision). At the start of one Agent run, the cloud resolves
online catalogs using the authenticated user and device plus an optional
runner selection. It combines those virtual manifests with the static catalog
in an immutable request snapshot used by the Planner, Guard, and Compiler.
Nothing from that snapshot is written to the global registry, database, or
Redis.

Two devices advertising the same logical tool name are ambiguous unless the
request selects one authenticated device/runner. A catalog returned for another
user or device fails closed. The compiled hidden local gateway carries
`targetRunnerId`, `catalogRevision`, `providerId`, `remoteToolName`,
`logicalToolName`, and logical `arguments`. Dispatch rechecks ownership,
online state, revision, and exact advertised binding. A changed revision fails
with `MCP_CATALOG_STALE`; the run must replan instead of silently calling a
different tool version. Bundled providers such as the IP Avatar enter planning
through this real runner advertisement, not a hardcoded cloud-global manifest.
After that check, the cloud-owned local job stores the selected non-secret
input/output schemas and binding revision as an immutable contract snapshot.
Completion validates MCP `structuredContent` against that snapshot, so a later
heartbeat cannot change the result contract and no global registry lookup is
needed. MCP `isError=true`, a mismatched snapshot, or invalid structured output
fails the job instead of publishing node success.

Provider responses preserve all standard text, image, audio, resource, and
embedded-resource content. Structured content and unknown raw extension fields
are retained so a newer conforming server does not lose information when
passing through an older application layer. `tools/list` pagination must be
consumed until `nextCursor` is empty.

Provider configuration may contain environment variables and HTTP headers.
Environment and header values are write-only: callers can store, replace, or
explicitly clear them, while GET, status, logs, and diagnostics expose at most
sorted key names and configured/not-configured metadata. Omitting `env` or
`headers` during an update preserves the stored values, so sending a sanitized
GET response back to PUT cannot erase credentials; an explicit empty object
clears that secret map. Error messages must not echo command environments,
authorization values, or prompt secrets.

Provider registration fails closed. When `approvalMode` is omitted or blank,
the Local Agent normalizes it to `before_execute`. A provider may bypass that
review gate only by explicitly setting `approvalMode` to `none`; invalid values
are rejected and cannot overwrite the last valid configuration.

Cloud-global tool mutation is a separate internal control-plane operation.
`POST /api/tools/register` and `DELETE /api/tools/:name` require the
`X-Internal-Tool-Token` header to match
`TOOL_REGISTRATION_INTERNAL_TOKEN`; an ordinary authenticated user cannot add a
manifest to the shared catalog. An empty server token disables these mutations.

## Registration example

The Local Agent accepts a provider list at
`PUT /api/local/mcp-providers`. A stdio provider uses `command` and `args`; a
Streamable HTTP provider uses `endpoint`. See the copyable examples in the
repository [README](../README.md#agenttoolmcp-标准契约) and the detailed
[MCP provider guide](mcp-providers.md).

## Repository conformance check

Install the supported official Python SDK and run the contract smoke test:

```bash
python3 -m pip install 'mcp>=1.27,<2'
python3 scripts/test_mcp_contracts.py
```

The test starts all three bundled Python servers over real stdio, completes
`initialize` and paginated `tools/list`, validates their names, descriptions,
and input schemas, then closes them without calling Blender, FFmpeg, Dreamina,
the network, or any generation service. CI runs the same check on Python 3.11.
