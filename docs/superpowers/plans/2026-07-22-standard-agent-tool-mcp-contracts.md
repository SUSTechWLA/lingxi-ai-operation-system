# Standard Agent Tool and MCP Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Agent runtime's Context/Tools/Constrain/Verify/Correct loop explicit and make all tool and MCP definitions portable across standard LLM APIs and MCP clients.

**Architecture:** Keep `ToolManifest` as the product metadata envelope, add lossless JSON Schema fields as the canonical interface, and derive provider wire formats through isolated adapters. Replace the handwritten MCP protocol transport with the official Go SDK while retaining the generic local runner boundary and existing provider configuration.

**Tech Stack:** Go 1.25, official MCP Go SDK, JSON Schema, Python FastMCP, Go/Python/Node test suites, GitHub Actions.

---

### Task 1: Canonical tool schema and LLM adapters

**Files:**
- Modify: `cloud-backend/internal/core/worker/tool/manifest.go`
- Create: `cloud-backend/internal/core/worker/tool/llm_definition.go`
- Create: `cloud-backend/internal/core/worker/tool/llm_definition_test.go`
- Modify: `cloud-backend/internal/core/worker/tool/mcp_manifest.go`
- Modify: `cloud-backend/internal/core/worker/tool/mcp_manifest_test.go`

- [ ] **Step 1: Write failing tests for lossless schemas and provider adapters**

Define a nested input schema containing `items`, `oneOf`, nullable types, `$defs`, and `additionalProperties`. Assert that OpenAI Responses, OpenAI Chat Completions, Anthropic, and Gemini adapters preserve it and use their documented field names. Assert invalid root schemas and empty tool names are rejected.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `cd cloud-backend && go test ./internal/core/worker/tool -run 'TestLLMTool|TestMCPProvider'`

Expected: failure because the canonical schema fields and adapter functions do not exist.

- [ ] **Step 3: Implement the canonical definition and adapters**

Add `InputSchema` and `OutputSchema` to `ToolManifest`, a validated `LLMToolDefinition`, and pure adapter functions for the four LLM API shapes. Preserve canonical maps via deep-copy JSON round trips. Keep legacy parameter projections as compatibility views.

- [ ] **Step 4: Run the focused tests and verify GREEN**

Run: `cd cloud-backend && go test ./internal/core/worker/tool`

Expected: all package tests pass.

- [ ] **Step 5: Commit**

Commit message: `feat: add canonical llm tool contracts`

### Task 2: Persist canonical schemas without breaking existing rows

**Files:**
- Modify: `cloud-backend/internal/core/database/database.go`
- Modify: `cloud-backend/internal/core/model/model.go`
- Modify: `cloud-backend/internal/core/model/repository/tool_manifest_repo.go`
- Modify: `cloud-backend/internal/core/worker/tool/tool_manifest_service.go`
- Modify: `cloud-backend/internal/core/worker/tool/tool_manifest_service_test.go`

- [ ] **Step 1: Write failing round-trip tests**

Assert `manifestToRecord` and record restoration preserve full canonical schemas while legacy manifests still derive valid object schemas.

- [ ] **Step 2: Verify RED**

Run: `cd cloud-backend && go test ./internal/core/worker/tool ./internal/core/model/repository`

- [ ] **Step 3: Add additive JSONB persistence**

Add `input_schema` and `output_schema` JSONB columns with object defaults, update SQL insert/select/scan code, and add backward-compatible derivation from `parameters`/`output` for old records.

- [ ] **Step 4: Verify GREEN**

Run: `cd cloud-backend && go test ./internal/core/worker/tool ./internal/core/model/repository ./internal/core/database`

- [ ] **Step 5: Commit**

Commit message: `feat: persist canonical tool schemas`

### Task 3: Standard MCP client and dynamic registration

**Files:**
- Modify: `local-backend/go.mod`
- Modify: `local-backend/go.sum`
- Replace: `local-backend/internal/localmcp/client.go`
- Modify: `local-backend/internal/localmcp/types.go`
- Modify: `local-backend/internal/localmcp/client_test.go`
- Modify: `local-backend/internal/localagent/mcp_providers.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call.go`

- [ ] **Step 1: Write failing protocol and registration tests**

Cover stdio and Streamable HTTP initialization, stable protocol negotiation, provider headers, paginated discovery, logical/remote name mapping, all tool metadata, all result content types, `structuredContent`, `_meta`, `isError`, and session closure.

- [ ] **Step 2: Verify RED**

Run: `cd local-backend && go test ./internal/localmcp ./internal/localagent ./internal/localtool`

- [ ] **Step 3: Integrate the official MCP Go SDK**

Pin a non-release-candidate SDK version that supports stable MCP `2025-11-25`. Build transports solely from provider config, connect once per client, let the SDK handle lifecycle/Streamable HTTP/SSE/session behavior, and map SDK structures into lossless local types. Keep `LOCAL_MCP_TOOL_CALL` as the only executor.

- [ ] **Step 4: Verify GREEN**

Run: `cd local-backend && go test ./internal/localmcp ./internal/localagent ./internal/localtool`

- [ ] **Step 5: Commit**

Commit message: `feat: use standard mcp client contracts`

### Task 4: Agent five-layer conformance

**Files:**
- Create: `cloud-backend/internal/core/agentruntime/contract.go`
- Create: `cloud-backend/internal/core/agentruntime/contract_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/llm_planner.go`
- Modify: `cloud-backend/internal/core/agentruntime/runner.go`

- [ ] **Step 1: Write failing conformance tests**

Assert the production runtime explicitly identifies Context, Tools, Constrain, Verify, and Correct components; planner input includes system instructions, user request, runtime context/memory/RAG and canonical tool definitions; guard rejection can trigger bounded repair; verification and correction policies remain connected to compiled steps.

- [ ] **Step 2: Verify RED**

Run: `cd cloud-backend && go test ./internal/core/agentruntime -run 'TestAgentContract|TestLLMPlanner'`

- [ ] **Step 3: Implement the explicit contract**

Add a small immutable conformance descriptor and validation function backed by existing runtime components. Extend planner payload construction to include sanitized request context and canonical schemas without exposing hidden chain-of-thought. Preserve existing run behavior.

- [ ] **Step 4: Verify GREEN**

Run: `cd cloud-backend && go test ./internal/core/agentruntime`

- [ ] **Step 5: Commit**

Commit message: `feat: enforce agent runtime capability loop`

### Task 5: MCP server contracts, documentation, and CI

**Files:**
- Modify: `mcp/ip_avatar_3d/requirements.txt`
- Modify: `mcp/jimeng/requirements.txt`
- Modify: `mcp/video_qa/requirements.txt`
- Create: `scripts/test_mcp_contracts.py`
- Create: `scripts/test_mcp_contracts_test.py`
- Modify: `mcp/README.md`
- Create: `docs/agent-tool-mcp-contract.md`
- Modify: `README.md`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Write failing repository contract tests**

The test must discover every `mcp/*/server.py`, require an official Python MCP SDK constraint, verify stdio entry points do not write non-protocol data to stdout, and ensure CI invokes cloud/local Go tests plus the MCP contract suite.

- [ ] **Step 2: Verify RED**

Run: `python3 -m unittest scripts/test_mcp_contracts_test.py`

- [ ] **Step 3: Normalize server dependency and documentation contracts**

Pin compatible MCP 1.x ranges, document provider registration examples for stdio and Streamable HTTP, document the five Agent layers and LLM adapters, and add the contract test to CI.

- [ ] **Step 4: Verify GREEN**

Run: `python3 -m unittest scripts/test_mcp_contracts_test.py && python3 scripts/test_mcp_contracts.py`

- [ ] **Step 5: Commit**

Commit message: `docs: standardize agent and mcp integration`

### Task 6: Full verification and release delivery

**Files:**
- Delete before PR: `docs/superpowers/specs/2026-07-22-standard-agent-tool-mcp-contracts-design.md`
- Delete before PR: `docs/superpowers/plans/2026-07-22-standard-agent-tool-mcp-contracts.md`

- [ ] **Step 1: Run focused and full verification**

Run cloud and local `go test ./...`, all MCP Python tests, frontend tests/lint/build, Electron runtime tests, HyperFrames build/test, deployment and beta readiness tests, beta smoke, release version/tree checks, and `git diff --check`.

- [ ] **Step 2: Review all changes**

Run independent spec-compliance and code-quality reviews. Fix every Critical and Important issue, then re-run affected and full verification.

- [ ] **Step 3: Remove release-forbidden planning artifacts**

Delete the two `docs/superpowers` files so the release tree contains only product documentation and code; retain the approved design in commit history.

- [ ] **Step 4: Push and create the PR**

Push `hotfix/standard-agent-tool-mcp-contracts`, create a PR targeting `release`, and include standards, compatibility, and test evidence in the body.

- [ ] **Step 5: Wait for CI and merge**

Wait until all required checks finish. Fix failures through additional commits. Under the user's explicit authorization, merge the PR even if branch protection requires an administrative merge, but never merge with failing checks.

- [ ] **Step 6: Verify remote release**

Fetch `develop_go/release`, verify the merge commit contains all expected files and passes release-tree/version checks, and report the PR URL and merge SHA.
