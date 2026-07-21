# Standard Agent Tool and MCP Contracts Design

**Status:** Approved by the user's 2026-07-22 requirements

## Goal

Make Tangying AIOS expose one provider-neutral agent/tool contract and accept dynamically registered MCP providers through the same tool catalog, while preserving the existing video-production orchestration and release behavior.

## Standards baseline

- Tool schemas use JSON Schema as the canonical source. MCP schemas default to JSON Schema 2020-12 when `$schema` is absent.
- MCP targets the latest stable protocol revision available on 2026-07-22: `2025-11-25`, while negotiating compatible older revisions.
- MCP transports are standard `stdio` and Streamable HTTP. Provider-specific runner commands are forbidden.
- LLM tool wire formats are adapters, not the canonical model: OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, and Gemini function declarations are generated from the same schema without losing nested keywords.

## Agent runtime contract

The runtime must expose and test five layers:

1. **Context** supplies perception: stable system instructions, user request, compacted project/role memory, environment and artifact state, RAG knowledge, prior execution outputs, and the selected tool catalog.
2. **Tools** supply actions: every tool has a stable name, description, canonical input schema, optional output schema, execution boundary, and routing metadata. MCP tools enter through `tools/list` and execute through `tools/call`.
3. **Constrain** supplies boundaries: allowed/forbidden tools, capability policy, budgets, cost/risk limits, approval requirements, execution plane, and artifact policy are checked before execution.
4. **Verify** judges results: schema validation, quality checkers, artifact/review gates, and terminal status checks run after tool execution.
5. **Correct** recovers: plan repair, bounded retry, corrective tools, timeout/error classification, and explicit fallback or human-review transitions handle failures without silently declaring success.

The current DAG runtime already has substantial Context, Constrain, Verify, and Correct components. This change makes their contract explicit and regression-tested; it does not replace the product workflow with an unrelated chat framework.

## Canonical tool model

`ToolManifest` remains the rich product envelope, but gains canonical `inputSchema` and `outputSchema` fields. The legacy `parameters` and `output` projections remain for backward-compatible planner/UI behavior and are derived from canonical schemas when possible.

A provider-neutral `LLMToolDefinition` contains:

- `name`
- `description`
- JSON Schema `inputSchema`
- optional JSON Schema `outputSchema`
- optional strictness and behavioral annotations

Adapters must produce:

- OpenAI Responses function tools (`type`, `name`, `description`, `parameters`, `strict`)
- OpenAI Chat Completions function tools (`type`, nested `function`)
- Anthropic tools (`name`, `description`, `input_schema`, `strict`)
- Gemini function declarations (`name`, `description`, `parameters`)

Strict mode is enabled only when the canonical schema meets the target provider's supported subset; adapters must never mutate the canonical schema.

## MCP registration and execution

Provider registration keeps configuration-only routing: `id`, transport, endpoint or command, arguments, environment, headers, timeout, tool prefix/name map, allow/deny lists, and approval mode.

The local agent must use the official MCP Go SDK for lifecycle and transport correctness. Registration performs a connection and initialization, then paginated `tools/list`; each result becomes a canonical `ToolManifest` with its complete schema and metadata. Invocation uses the provider binding's original remote tool name.

All standard MCP tool result content is preserved: text, image, audio, resource link, embedded resource, `structuredContent`, `isError`, annotations, and `_meta`. Application-level tool errors remain visible to the agent; protocol errors remain transport failures.

## Compatibility and safety

- Existing `LOCAL_MCP_TOOL_CALL` remains the single generic local execution command.
- Existing logical tool prefixes and mappings remain compatible.
- Existing Python FastMCP servers remain independent MCP servers and receive contract tests.
- No provider-specific cloud orchestration branch is added.
- Existing database rows remain readable. Canonical schema columns are additive and backfilled from legacy projections or preserved provider schemas.
- Tool annotations from remote servers are treated as untrusted hints; local approval/risk policy remains authoritative.

## Verification

The release is accepted only if tests prove:

- complex JSON Schema survives manifest persistence and all LLM adapters;
- the five agent layers are present and connected in the production runtime;
- dynamically registered MCP tools appear in the common catalog and compile to the generic local command;
- stdio and Streamable HTTP lifecycle/version negotiation work;
- pagination, session headers, JSON and SSE responses, errors, and all content block types are preserved;
- current MCP servers pass protocol-level discovery and call tests;
- all existing Go, Python, frontend, Electron, deployment, beta-smoke, release-tree, lint, and build checks pass.

## Delivery

The work is based on the current `release` branch plus the previously completed dark-theme contrast commit. It is delivered through a `hotfix/*` PR. After required checks pass, the PR is merged into `release` under the user's explicit authorization.
