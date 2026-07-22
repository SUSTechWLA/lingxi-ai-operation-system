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

Provider adapters translate this canonical definition at the API edge. OpenAI
uses the function tool wrapper and strict schema semantics, Anthropic uses
`name` / `description` / `input_schema`, and Gemini uses function declarations
with its supported schema representation. Provider-specific spelling must not
leak back into the canonical registry. If a provider cannot represent a schema
feature without changing its meaning, conformance fails closed instead of
silently weakening the contract.

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

### 5. Correct

Correction is scoped recovery after a verified failure. It may retry a
transient call within budget, repair only the invalid plan fields, choose an
allowed fallback, or roll back a reversible local operation. Every repair is
revalidated by the same schema and Guard rules. Exhausted retry budgets,
authorization failures, ambiguous destructive actions, and invalid repair
plans fail closed and are surfaced for user action.

## Conversation and tool-result loop

An Agent request is composed from the system instruction, tool definitions,
user messages, prior assistant messages that are safe to retain, and tool
results. Retrieved private or current knowledge may be injected into the user
or context portion with source and scope metadata. An assistant turn can return
user-facing content or a structured tool call. The runtime executes an approved
call and appends the structured result to the next model turn, closing the
observe-decide-act-verify loop.

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

Provider responses preserve all standard text, image, audio, resource, and
embedded-resource content. Structured content and unknown raw extension fields
are retained so a newer conforming server does not lose information when
passing through an older application layer. `tools/list` pagination must be
consumed until `nextCursor` is empty.

Provider configuration may contain environment variables and HTTP headers.
Secret header values are write-only: callers can store or replace them, while
GET, status, logs, and diagnostics expose at most header names or redacted
metadata. Error messages must not echo command environments, authorization
values, or prompt secrets.

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
