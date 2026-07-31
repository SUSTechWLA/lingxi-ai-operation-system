# Observability Product Surfaces and Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Deliver the creator timeline, independent developer diagnostics console, scoped support flow, and production security, performance, recovery, and release gates.

**Architecture:** Pure frontend view models transform the local query API into two separate surfaces. Creators see understandable stages, evidence, and corrective actions; the developer route exposes redacted traces and technical detail. Final gates validate privacy, scope isolation, replay, performance, CI, and the packaged desktop client.

**Tech Stack:** React 18, TypeScript 5.5, Vite 5, esbuild logic checks, Electron 33, Go tests and benchmarks, existing desktop packaging scripts.

## Global Constraints

- Creator pages never display raw JSON, stacks, tool arguments, provider payloads, or internal storage records.
- The developer console is a separate route and authorization surface.
- Completed tasks and stages remain enterable and show versioned artifacts and decisions.
- Regeneration creates a derived run, reuses unchanged upstream artifacts, invalidates only the selected stage and downstream dependencies, and preserves history.
- Diagnostic packages require preview and explicit user approval.
- Normal users receive localized reasons and actions from stable message keys.
- Performance budgets are less than 2 percent representative CPU overhead, enqueue P95 below 1 ms, and a 10,000-event timeline P95 below 1 second.
- Observability failure does not fail the creation pipeline.
- Existing creator-review and media-preview behavior must not regress.

---

## File structure

- Create frontend/src/features/observability for types, view models, creator components, and developer components.
- Create frontend/src/services/observability.ts for authenticated local queries.
- Create frontend/scripts/observability-logic-check.mjs for pure model and rendered-markup checks.
- Modify frontend/src/pages/DirectorStudioPage.tsx and App.tsx for the two product surfaces.
- Add privacy, replay, failure-isolation, and benchmark tests to local and cloud observability packages.
- Add one repository-wide gate, CI step, support runbook, and packaged-client smoke test.

### Task 1: Frontend client and deterministic view models

**Files:**
- Create: frontend/src/features/observability/types.ts
- Create: frontend/src/features/observability/timelineModel.ts
- Create: frontend/src/features/observability/developerModel.ts
- Create: frontend/src/features/observability/messages.zh-CN.ts
- Create: frontend/src/services/observability.ts
- Create: frontend/scripts/observability-logic-check.mjs
- Modify: frontend/package.json

**Interfaces:**
- Produces fetchRunTimeline, searchEvents, fetchTrace, explainError, previewDiagnostics, and createDiagnostics.
- Produces buildCreatorTimeline(Timeline) CreatorTimelineItem[].
- Produces buildTraceTree(Event[]) TraceTreeNode[] and groupErrors(Event[]) ErrorGroup[].

- [ ] **Step 1: Write the failing logic harness**

~~~js
const events = [
  fixture('stage.started', { stageId: 'script', status: 'RUNNING' }),
  fixture('mcp.call.failed', {
    eventId: 'evt_mcp',
    stageId: 'script',
    status: 'FAILED',
    error: { code: 'MCP.CONNECTION.UNAVAILABLE', rootCause: true },
  }),
  fixture('stage.failed', {
    stageId: 'script',
    status: 'FAILED',
    error: { code: 'WORKFLOW.STAGE.FAILED', causedByEventId: 'evt_mcp' },
  }),
]
const timeline = buildCreatorTimeline({ runId: 'wfr_1', events })
assert.equal(timeline.length, 1)
assert.equal(timeline[0].status, 'failed')
assert.equal(timeline[0].errorCode, 'MCP.CONNECTION.UNAVAILABLE')
assert.doesNotMatch(JSON.stringify(timeline), /stack|authorization|toolArguments/)
~~~

The script uses esbuild like frontend/scripts/director-studio-logic-check.mjs.

- [ ] **Step 2: Run the harness**

Run: cd frontend && node scripts/observability-logic-check.mjs

Expected: FAIL because feature modules do not exist.

- [ ] **Step 3: Implement bounded clients and pure transformations**

Every request supplies X-Tangying-Local-Capability, explicit project or run scope, AbortSignal, and limit no greater than 500. Creator transformation groups low-level events by stage, follows root-cause links, selects localized message keys, and retains artifact references without technical payloads. Developer transformation keeps redacted correlation, duration, attempts, and evidence. messages.zh-CN.ts maps every initial messageKey and suggestedActionKey from the shared error registry to plain creator-facing Chinese copy; unknown keys use one generic safe fallback.

- [ ] **Step 4: Run logic and build checks**

Run: cd frontend && node scripts/observability-logic-check.mjs && npm run build

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add frontend/src/features/observability frontend/src/services/observability.ts frontend/scripts/observability-logic-check.mjs frontend/package.json
git commit -m "feat: add observability frontend models"
~~~

### Task 2: Creator task timeline and recovery actions

**Files:**
- Create: frontend/src/features/observability/CreatorRunTimeline.tsx
- Create: frontend/src/features/observability/CreatorFailureCard.tsx
- Create: frontend/src/features/observability/StageEvidencePanel.tsx
- Modify: frontend/src/pages/DirectorStudioPage.tsx
- Modify: frontend/src/pages/directorStudioLogic.ts
- Modify: frontend/scripts/observability-logic-check.mjs

**Interfaces:**
- Consumes CreatorTimelineItem[] from Task 1.
- Consumes existing artifact preview, impact preview, and regeneration APIs.
- Produces creator-visible status, duration, evidence, repair attempts, suggestion, and Regenerate from this step.

- [ ] **Step 1: Add rendered-markup and action tests**

Bundle CreatorRunTimeline.tsx and render through react-dom/server. Assert localized stage title, plain-language reason, suggestion, and regeneration button. Assert no traceId, spanId, stack, raw JSON, or tool arguments.

Add a pure test proving the regeneration request contains parentRunId, replayFromStageId, and an idempotency key while unrelated Shot IDs remain absent.

- [ ] **Step 2: Run the test**

Run: cd frontend && npm run test:observability

Expected: FAIL on missing components.

- [ ] **Step 3: Implement the creator timeline**

Show one row per creator-relevant stage. Expanded rows reuse reviewable artifact previews and decisions. Failures show one root cause, repair attempts, and one recommended action. Completed tasks use the same component in read-only mode and keep stages selectable.

Before regeneration, request and display the exact impact. On approval, create a derived run and refresh. Never infer success from a percentage; use terminal state and verified artifacts.

- [ ] **Step 4: Run frontend regressions**

Run: cd frontend && npm run test:director && npm run test:observability && npm run build

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add frontend/src/features/observability/CreatorRunTimeline.tsx frontend/src/features/observability/CreatorFailureCard.tsx frontend/src/features/observability/StageEvidencePanel.tsx frontend/src/pages/DirectorStudioPage.tsx frontend/src/pages/directorStudioLogic.ts frontend/scripts/observability-logic-check.mjs
git commit -m "feat: show creator run timeline"
~~~

### Task 3: Independent developer diagnostic console

**Files:**
- Create: frontend/src/features/observability/DeveloperDiagnosticsPage.tsx
- Create: frontend/src/features/observability/TraceTree.tsx
- Create: frontend/src/features/observability/TraceWaterfall.tsx
- Create: frontend/src/features/observability/EventDetail.tsx
- Create: frontend/src/features/observability/DiagnosticExportDialog.tsx
- Modify: frontend/src/App.tsx
- Modify: frontend/scripts/observability-logic-check.mjs

**Interfaces:**
- Consumes search, trace, error, preview, and create clients from Task 1.
- Produces hash route #/developer/diagnostics.

- [ ] **Step 1: Add route and rendered-content tests**

Assert resolveAppRoute('#/developer/diagnostics') selects the developer page and normal creator navigation omits it. Render a trace fixture and assert root error, propagated error, retries, duration, provider, cached tokens, artifact references, and redaction labels are visible.

- [ ] **Step 2: Run the test**

Run: cd frontend && npm run test:observability

Expected: FAIL.

- [ ] **Step 3: Implement filters, trace views, and export confirmation**

Support project, task, run, Shot, stage, component, time, severity, and error filters. Load summaries first and fetch details when a span is selected. The export dialog requires scope, lists every manifest entry and classification, and enables creation only after explicit confirmation.

Gate the route with VITE_ENABLE_DEVELOPER_CONSOLE=1. Do not add it to creator navigation.

- [ ] **Step 4: Run frontend checks**

Run: cd frontend && npm run test:director && npm run test:observability && npm run build

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add frontend/src/features/observability frontend/src/App.tsx frontend/scripts/observability-logic-check.mjs
git commit -m "feat: add developer diagnostic console"
~~~

### Task 4: Explicit one-time encrypted support upload

**Files:**
- Create: cloud-backend/internal/core/observability/support_upload.go
- Create: cloud-backend/internal/core/observability/support_upload_test.go
- Modify: cloud-backend/internal/core/observability/handler.go
- Modify: cloud-backend/internal/core/apispec/cloud_spec_test.go
- Modify: cloud-backend/cmd/tangying-ai-os/main.go
- Create: local-backend/internal/localagent/diagnostic_upload.go
- Create: local-backend/internal/localagent/diagnostic_upload_test.go
- Modify: local-backend/internal/localagent/openapi.go
- Modify: frontend/src/services/observability.ts
- Modify: frontend/src/features/observability/DiagnosticExportDialog.tsx
- Modify: frontend/scripts/observability-logic-check.mjs

**Interfaces:**
- Produces POST /api/observability/support-uploads and POST /api/observability/support-uploads/:uploadId/complete.
- Produces POST /api/local/diagnostics/upload, which requires the privileged UI preview token and local capability.
- Produces a one-time RSA public key, signed upload URL, upload ID, and expiry.

- [ ] **Step 1: Write consent, expiry, and encryption tests**

Assert no support session or network request occurs before the explicit confirmation action. Create a session, encrypt a synthetic ZIP, and assert uploaded bytes do not contain ZIP magic or fixture plaintext. Assert a completed or expired upload ID cannot be reused and cross-user completion is rejected.

- [ ] **Step 2: Run focused tests**

Run:

~~~bash
cd cloud-backend && go test ./internal/core/observability -run SupportUpload -count=1
cd ../local-backend && go test ./internal/localagent -run DiagnosticUpload -count=1
cd ../frontend && npm run test:observability
~~~

Expected: FAIL because the support upload flow does not exist.

- [ ] **Step 3: Implement one-time encrypted upload**

The cloud creates a 24-hour one-time session with an RSA-3072 keypair and a signed object-storage PUT URL. The Local Agent generates a random AES-256-GCM key, encrypts the already-approved ZIP, wraps the AES key with RSA-OAEP-SHA256, uploads only ciphertext and the wrapped key, and marks the session complete. The cloud deletes the private key when support access expires.

The frontend first shows the exact approved manifest and a separate upload confirmation. It never uploads automatically after ZIP creation. Events record upload ID, byte count, checksum, and expiry only.

- [ ] **Step 4: Run cloud, local, and frontend tests**

Run the commands from Step 2.

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add cloud-backend/internal/core/observability cloud-backend/internal/core/apispec/cloud_spec_test.go cloud-backend/cmd/tangying-ai-os/main.go local-backend/internal/localagent frontend/src/services/observability.ts frontend/src/features/observability/DiagnosticExportDialog.tsx frontend/scripts/observability-logic-check.mjs
git commit -m "feat: add explicit encrypted support upload"
~~~

### Task 5: Replay, downstream invalidation, and privacy end-to-end tests

**Files:**
- Create: local-backend/internal/observability/leak_e2e_test.go
- Create: local-backend/internal/observability/replay_e2e_test.go
- Modify: cloud-backend/internal/core/workflow/run_service_test.go
- Modify: cloud-backend/internal/core/workflow/checkpoint_service.go
- Create: cloud-backend/internal/core/workflow/checkpoint_service_test.go
- Modify: frontend/scripts/observability-logic-check.mjs

**Interfaces:**
- Verifies parentRunId, replayFromStageId, artifact hashes, and exact downstream invalidation.
- Verifies all storage and presentation sinks reject synthetic secrets.

- [ ] **Step 1: Write a derived-run fixture**

Create three stages with versioned artifacts and a verified checkpoint after stage one. Regenerate stage two. Assert a new run ID, preserved stage-one hash, new stage-two version, invalidated stage-three artifact, preserved original run, and new Verify events.

- [ ] **Step 2: Write one synthetic leak corpus across all sinks**

Inject unique values into Authorization, cookie, API key, prompt, user input, provider error, subprocess stderr, and absolute path fields. Emit through cloud redaction, relay, local journal, SQLite, query response, creator model, developer model, and diagnostic export. Scan bytes after every boundary.

- [ ] **Step 3: Run focused tests**

Run:

~~~bash
cd local-backend && go test ./internal/observability -run 'TestReplay|TestAllSinksRejectSyntheticSecrets' -count=1
cd ../cloud-backend && go test ./internal/core/workflow -run 'TestDerivedRun|TestCheckpoint' -count=1
cd ../frontend && npm run test:observability
~~~

Expected: FAIL until every integration boundary is wired.

- [ ] **Step 4: Complete lineage and invalidation wiring**

Persist lineage before queueing. Create the derived run transactionally, resolve the latest verified checkpoint, copy only unchanged upstream references, mark selected and downstream artifacts stale, and rerun verification. Never mutate the original run or artifacts.

- [ ] **Step 5: Run the focused tests again**

Run the commands from Step 3.

Expected: PASS.

- [ ] **Step 6: Commit**

~~~bash
git add local-backend/internal/observability cloud-backend/internal/core/workflow frontend/scripts/observability-logic-check.mjs
git commit -m "test: verify observability replay and privacy"
~~~

### Task 6: Performance, backpressure, and failure isolation

**Files:**
- Create: local-backend/internal/observability/benchmark_test.go
- Create: local-backend/internal/observability/failure_isolation_test.go
- Modify: cloud-backend/internal/core/observability/emitter_test.go
- Modify: local-backend/internal/observability/service_test.go

**Interfaces:**
- Verifies enqueue P95 below 1 ms, 10,000-event timeline P95 below 1 second, and continued creation when sinks fail.

- [ ] **Step 1: Add benchmarks and failure fixtures**

Add BenchmarkEventEnqueue and BenchmarkTimeline10k. Add a sink that returns errors, full queue, unwritable journal, and corrupt SQLite fixture. Assert normal creation continues, critical events enter the spool, and recovery rebuilds the index.

- [ ] **Step 2: Run tests and benchmarks**

Run:

~~~bash
cd local-backend
go test ./internal/observability -run 'TestFailureIsolation|TestBackpressure|TestCorruptIndexRecovery' -count=1
go test ./internal/observability -run '^$' -bench 'BenchmarkEventEnqueue|BenchmarkTimeline10k' -benchmem -count=3
~~~

Expected: failure tests PASS and benchmark reports are captured.

- [ ] **Step 3: Tune only measured bottlenecks**

Keep one writer, batch noncritical SQLite inserts, preserve synchronous critical journal flush, and add indexes only for measured query predicates. Do not weaken durability or privacy.

- [ ] **Step 4: Run race and benchmark gates**

Run:

~~~bash
cd local-backend
go test -race ./internal/observability -count=1
go test ./internal/observability -run '^$' -bench 'BenchmarkEventEnqueue|BenchmarkTimeline10k' -benchmem -count=5
~~~

Expected: tests PASS and median results meet the budgets on the documented baseline machine.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/observability cloud-backend/internal/core/observability/emitter_test.go
git commit -m "test: enforce observability performance budgets"
~~~

### Task 7: CI, support runbook, packaging, and release verification

**Files:**
- Create: scripts/test-observability-e2e.sh
- Modify: package.json
- Modify: .github/workflows/ci.yml
- Modify: README.md
- Modify: docs/observability/event-contract.md
- Modify: docs/observability/local-store.md
- Create: docs/observability/support-runbook.md

**Interfaces:**
- Produces npm run test:observability.
- Produces an operator runbook for trace search, root cause, index rebuild, diagnostics, and checkpoint recovery.

- [ ] **Step 1: Add the repository-wide gate**

scripts/test-observability-e2e.sh runs:

~~~bash
npm run test:observability-contract
cd cloud-backend && go test ./... -count=1
cd ../local-backend && go test -race ./... -count=1
cd ../frontend && node --test electron/*.test.cjs
npm run test:director
npm run test:observability
npm run build
cd ../hyperframes-render-service && npm run build
~~~

Add:

~~~json
"test:observability": "bash scripts/test-observability-e2e.sh"
~~~

- [ ] **Step 2: Add the gate to active CI**

Use the repository's existing Node and Go setup. Run npm run test:observability. Upload benchmark output and synthetic failing fixtures only; never upload real user logs.

- [ ] **Step 3: Write the support runbook**

Provide exact UI paths and commands for locating a run, copying a trace ID, identifying the root event, checking retries, verifying an artifact hash, rebuilding SQLite from JSONL, previewing diagnostics, and resuming from the latest verified checkpoint.

- [ ] **Step 4: Run the full gate**

Run: npm run test:observability

Expected: PASS.

- [ ] **Step 5: Build the packaged client**

Run: npm run desktop:build

Expected: the platform package appears under frontend/release and the bundled Local Agent starts with a per-session capability.

- [ ] **Step 6: Run the packaged smoke scenario**

Sign in, open a completed project, inspect a stage timeline and media artifact, enable developer diagnostics, inspect a trace, preview a task-scoped package, decline upload, regenerate one stage, and verify unrelated Shots retain accepted versions.

Expected: all actions succeed; creator pages contain no raw JSON; the package stays local; the derived run has new run and trace lineage.

- [ ] **Step 7: Commit**

~~~bash
git add scripts/test-observability-e2e.sh package.json .github/workflows/ci.yml README.md docs/observability
git commit -m "chore: gate observability release readiness"
~~~

## Final acceptance gate

- One creator action is traceable through frontend, cloud, Agent, workflow, tool, Local Agent, renderer, artifact, Verify, and Correct events.
- Root failures are distinguished from propagated failures and retries.
- Creators see understandable status, artifacts, repair attempts, and one next action.
- Developers see the redacted trace tree, waterfall, groups, versions, retries, and evidence.
- AI tools return scoped, paginated, redacted, read-only results and audit access.
- Completed stages reopen and regenerate through derived runs without unrelated Shot changes.
- Diagnostics contain exactly the approved manifest and no synthetic secret.
- Index rebuild, crash recovery, offline sync, backpressure, and failed-sink tests pass.
- Performance budgets, CI, packaging, and the smoke scenario pass.
