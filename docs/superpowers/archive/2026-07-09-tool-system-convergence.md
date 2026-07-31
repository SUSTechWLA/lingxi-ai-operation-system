# Tool System Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Converge tool boundaries around `ToolManifest`, strengthen fresh-knowledge gating, and keep MCP providers behind `LOCAL_MCP_TOOL_CALL`.

**Architecture:** Keep existing registries in place and add only a light catalog facade where needed. Planner and retriever operate on logical `ToolManifest` entries; compiler and local runner route MCP provider tools to the existing local MCP execution path.

**Tech Stack:** Go 1.25 cloud backend, Go 1.24 local backend, existing JSON-RPC MCP stdio/http client, Markdown docs.

## Global Constraints

- Do not introduce a parallel `ToolDescriptor`.
- Do not introduce a parallel `ToolRetriever`.
- Planner must not call provider CLIs, HTTP services, FFmpeg, Jimeng, Dreamina, or Video QA directly.
- New CLI and AIGC provider capabilities must be exposed through standard MCP `tools/list` and `tools/call`.
- `LOCAL_MCP_TOOL_CALL` remains the standard MCP execution entry.
- Tests must prove non-news creative video requests do not select fresh knowledge or news tools.

---

### Task 1: Policy And Retriever Tests

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/knowledge_policy_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/tool_retriever_test.go`

**Interfaces:**
- Consumes: `DefaultKnowledgePolicy(message, domain string) *KnowledgePolicy`
- Consumes: `NewHybridToolRetriever(manifests).Retrieve(ctx, ToolRetrieveRequest)`
- Produces: failing tests for no-web, fiction, rewrite/creative, current-event, and when-not-to-use behavior.

- [ ] Write tests for the six required `KnowledgePolicy` examples.
- [ ] Run `go test ./internal/core/agentruntime -run 'TestDefaultKnowledgePolicy|TestToolRetriever'`.
- [ ] Implement only enough policy/retriever behavior to pass.
- [ ] Re-run the same tests.

### Task 2: Manifest Boundary And Catalog Facade

**Files:**
- Modify: `cloud-backend/internal/core/worker/tool/manifest.go`
- Create: `cloud-backend/internal/core/agentruntime/tool_catalog.go`
- Modify: `cloud-backend/internal/core/agentruntime/llm_planner.go`

**Interfaces:**
- Produces: `Boundary`, `WhenToUse`, `WhenNotToUse`, and `ProviderBinding` on `ToolManifest`.
- Produces: a lightweight catalog facade that still exposes `GetManifest` and can list by capability/provider/local/MCP boundary.

- [ ] Add tests through retriever/guard behavior rather than a new descriptor model.
- [ ] Add manifest fields and constants with comments.
- [ ] Add facade helpers without changing existing registry ownership.
- [ ] Keep existing `toolManifestCatalog` call sites compatible.

### Task 3: Guard Policy Tests And Guard Enforcement

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/knowledge_policy_test.go`
- Create or modify: `cloud-backend/internal/core/agentruntime/plan_guard_policy_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_guard.go`

**Interfaces:**
- Consumes: `PlanGuard.Validate(plan)`
- Produces: readable rejection errors for no-web, fresh forbidden, publish/delete approval, high-cost AIGC approval, file-write artifact/pathguard, disabled provider/tool.

- [ ] Add failing tests for required guard scenarios.
- [ ] Implement manifest and knowledge-policy based checks.
- [ ] Re-run guard tests.

### Task 4: MCP Discovery And Manifest Conversion

**Files:**
- Modify: `local-backend/internal/localmcp/types.go`
- Modify: `local-backend/internal/localmcp/client.go`
- Modify: `local-backend/internal/localmcp/client_test.go`

**Interfaces:**
- Consumes: `ProviderConfig`, `Client.ListTools`, `Client.CallTool`
- Produces: enabled/disabled tool filtering, timeout/approval config, and a local logical MCP tool view that includes provider binding metadata for the cloud manifest schema.

- [ ] Add failing tests for `enabledTools`, `disabledTools`, `toolPrefix`, `toolNameMap`, unavailable provider errors.
- [ ] Implement filtering and timeout-aware client calls.
- [ ] Re-run localmcp tests.

### Task 5: Compiler And Local MCP Execution

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call_test.go`

**Interfaces:**
- Consumes: `ToolManifest.ExecutionPlane`, `Boundary`, `ProviderBinding`
- Produces: `LOCAL_MCP_TOOL_CALL` payloads with providerId, toolName, logicalToolName, input/arguments, timeout, traceId, artifactPolicy.

- [ ] Add failing compiler test for MCP provider tool compilation.
- [ ] Add localtool test that fallback results are explicit and provenance stays present.
- [ ] Implement compiler payload normalization and local output markers.
- [ ] Re-run compiler and localtool tests.

### Task 6: Documentation And Verification

**Files:**
- Modify: `docs/mcp-providers.md`
- Create: `docs/tool-boundary.md`
- Create: `docs/fresh-knowledge-policy.md`
- Create: `docs/local-tool-vs-mcp-provider.md`

**Interfaces:**
- Produces: operator docs for Agent Core, native/local/MCP/HTTP/queue/legacy boundaries, provider onboarding, fresh-knowledge gating, Jimeng, Video QA, trace, and migration.

- [ ] Update docs without removing existing IP Avatar 3D notes.
- [ ] Run focused cloud tests.
- [ ] Run focused local tests.
- [ ] Run broader `go test` for touched packages if focused tests pass.
