# Observability Foundation and Cloud Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Establish the versioned event contract, privacy and error rules, end-to-end trace propagation, immutable tool-registry snapshots, and redacted cloud event relay.

**Architecture:** A shared JSON contract defines the wire format. The cloud backend wraps Zap and Gin with correlation-aware helpers, stores only allowlisted summaries plus a short-lived delivery outbox, and attaches one immutable tool-registry snapshot to each Agent and workflow run.

**Tech Stack:** Go 1.25, Gin, Zap, PostgreSQL/pgx, JSON Schema 2020-12, Node.js contract checks.

## Global Constraints

- The architecture is local-first; cloud storage contains no raw prompts, media, secrets, or unredacted tool arguments.
- Every production task has one unique end-to-end traceId.
- Every critical stage has a start event and one terminal event.
- Event types and error codes are stable contracts; localized text is selected through messageKey.
- A running task's normalized tool and MCP snapshot remains byte-for-byte unchanged.
- New tool and MCP registrations affect only new tasks.
- SECRET data never enters logs, summaries, responses, or diagnostic packages.
- Existing unrelated worktree changes must be preserved.

---

## File structure

- Create contracts/observability/v1/event.schema.json for the canonical event envelope.
- Create contracts/observability/v1/error-codes.json for stable errors.
- Create scripts/verify-observability-contract.mjs for dependency-free contract checks.
- Create cloud-backend/internal/core/observability/ for event types, context, errors, redaction, emitter, middleware, repository, and handlers.
- Create cloud-backend/internal/core/agentruntime/tool_snapshot.go for canonical registry snapshots.
- Modify cloud-backend/internal/core/database/database.go for summary and relay tables.
- Modify cloud-backend/cmd/tangying-ai-os/main.go for composition and graceful flush.
- Instrument Agent, workflow, translator, orchestrator, Kafka event bus, worker, and Local Runner cloud boundaries.

### Task 1: Canonical event and error contracts

**Files:**
- Create: contracts/observability/v1/event.schema.json
- Create: contracts/observability/v1/error-codes.json
- Create: contracts/observability/v1/example-stage-failed.json
- Create: scripts/verify-observability-contract.mjs
- Modify: package.json
- Test: scripts/verify-observability-contract.mjs

**Interfaces:**
- Produces event fields schemaVersion, eventId, occurredAt, ingestedAt, producerSequence, severity, eventType, messageKey, source, correlation, execution, runtime, evidence, error, and privacy.
- Produces error entries containing code, class, retryable, userMessageKey, and suggestedActionKey.

- [ ] **Step 1: Write the failing contract verifier**

~~~js
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const schema = JSON.parse(await readFile('contracts/observability/v1/event.schema.json', 'utf8'))
const errors = JSON.parse(await readFile('contracts/observability/v1/error-codes.json', 'utf8'))
const fixture = JSON.parse(await readFile('contracts/observability/v1/example-stage-failed.json', 'utf8'))

assert.equal(schema.$schema, 'https://json-schema.org/draft/2020-12/schema')
for (const field of schema.required) assert.ok(Object.hasOwn(fixture, field), 'missing ' + field)
assert.match(fixture.eventId, /^evt_/)
assert.match(fixture.correlation.traceId, /^trc_/)
assert.ok(errors.some((item) => item.code === fixture.error.code))
assert.equal(new Set(errors.map((item) => item.code)).size, errors.length)
~~~

- [ ] **Step 2: Run the verifier**

Run: node scripts/verify-observability-contract.mjs

Expected: FAIL with ENOENT for event.schema.json.

- [ ] **Step 3: Add the schema, registry, fixture, and root script**

The schema sets additionalProperties to false at the top level and defines enums for severity, execution.status, and privacy.classification. Include every approved error code, including MCP.CONNECTION.UNAVAILABLE, ARTIFACT.HASH.MISMATCH, and RENDER.FFMPEG.CODEC_UNSUPPORTED.

Add this root script:

~~~json
"test:observability-contract": "node scripts/verify-observability-contract.mjs"
~~~

- [ ] **Step 4: Verify the contract**

Run: npm run test:observability-contract

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add contracts/observability/v1 scripts/verify-observability-contract.mjs package.json
git commit -m "feat: define observability event contract"
~~~

### Task 2: Cloud event types, error normalization, and redaction

**Files:**
- Create: cloud-backend/internal/core/observability/event.go
- Create: cloud-backend/internal/core/observability/event_test.go
- Create: cloud-backend/internal/core/observability/error.go
- Create: cloud-backend/internal/core/observability/error_test.go
- Create: cloud-backend/internal/core/observability/redaction.go
- Create: cloud-backend/internal/core/observability/redaction_test.go

**Interfaces:**
- Produces func (e Event) Validate() error.
- Produces func NormalizeError(code string, err error, component string, causedBy string) *EventError.
- Produces func Redact(e Event) Event and func ContainsSecret(value any) bool.

- [ ] **Step 1: Write failing tests**

~~~go
func TestNormalizeErrorFingerprintIgnoresDynamicMessage(t *testing.T) {
    a := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9001"), "mcp-client", "evt_a")
    b := NormalizeError("MCP.CONNECTION.UNAVAILABLE", errors.New("dial port 9002"), "mcp-client", "evt_b")
    if a.Fingerprint != b.Fingerprint {
        t.Fatalf("fingerprints differ: %q %q", a.Fingerprint, b.Fingerprint)
    }
}

func TestRedactNeverKeepsSecretValues(t *testing.T) {
    event := validEvent()
    event.Evidence.Attributes = map[string]any{
        "authorization": "Bearer secret-value",
        "artifactId": "art_1",
    }
    got := Redact(event)
    if ContainsSecret(got) {
        t.Fatal("redacted event still contains a secret")
    }
}
~~~

- [ ] **Step 2: Run focused tests**

Run: cd cloud-backend && go test ./internal/core/observability -count=1

Expected: FAIL because the package is incomplete.

- [ ] **Step 3: Implement explicit contract structs and allowlist redaction**

Use explicit Event, Correlation, Execution, Runtime, Evidence, EventError, and Privacy structs. Redact evidence by allowed field names. Remove authorization, cookie, token, password, apiKey, secret, prompt, and userInput fields instead of masking them into reusable text.

- [ ] **Step 4: Run tests**

Run: cd cloud-backend && go test ./internal/core/observability -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add cloud-backend/internal/core/observability
git commit -m "feat: add cloud observability event policies"
~~~

### Task 3: Trace middleware and nonblocking emitter

**Files:**
- Create: cloud-backend/internal/core/observability/context.go
- Create: cloud-backend/internal/core/observability/context_test.go
- Create: cloud-backend/internal/core/observability/emitter.go
- Create: cloud-backend/internal/core/observability/emitter_test.go
- Create: cloud-backend/internal/core/observability/middleware.go
- Create: cloud-backend/internal/core/observability/middleware_test.go
- Modify: cloud-backend/internal/core/logger/logger.go
- Modify: cloud-backend/internal/core/eventbus/eventbus.go
- Modify: cloud-backend/cmd/tangying-ai-os/main.go

**Interfaces:**
- Produces WithCorrelation(context.Context, Correlation) context.Context.
- Produces CorrelationFromContext(context.Context) Correlation.
- Produces Sink with Write(context.Context, Event) error and Close(context.Context) error.
- Produces NewEmitter(Source, Runtime, Sink, int) *Emitter.
- Produces Middleware(*Emitter) gin.HandlerFunc.

- [ ] **Step 1: Write the incoming trace test**

~~~go
func TestMiddlewarePreservesIncomingTraceparent(t *testing.T) {
    r := gin.New()
    r.Use(Middleware(NewEmitter(testSource(), Runtime{}, &memorySink{}, 16)))
    r.GET("/x", func(c *gin.Context) {
        c.String(http.StatusOK, CorrelationFromContext(c.Request.Context()).TraceID)
    })
    req := httptest.NewRequest(http.MethodGet, "/x", nil)
    req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)
    if rec.Body.String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
        t.Fatalf("trace id = %q", rec.Body.String())
    }
}
~~~

Also test that a full queue discards DEBUG before audit, error, state-transition, or checkpoint events.

- [ ] **Step 2: Run focused tests**

Run: cd cloud-backend && go test ./internal/core/observability -run 'TestMiddleware|TestEmitter' -count=1

Expected: FAIL.

- [ ] **Step 3: Implement context, emitter, and middleware**

Parse and return W3C traceparent. Generate a trace when absent. Attach product identifiers through context helpers. Add TraceID, SpanID, and ParentSpanID to Kafka events and reconstruct correlation context before a consumer invokes its handler. Replace gin.Default() with gin.New(), then install gin.Recovery(), existing CORS, and observability middleware. Do not log request bodies or headers.

- [ ] **Step 4: Run package and server tests**

Run: cd cloud-backend && go test ./internal/core/observability ./cmd/tangying-ai-os -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add cloud-backend/internal/core/observability cloud-backend/internal/core/logger/logger.go cloud-backend/internal/core/eventbus/eventbus.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: propagate cloud trace context"
~~~

### Task 4: Immutable tool-registry and run manifests

**Files:**
- Create: cloud-backend/internal/core/agentruntime/tool_snapshot.go
- Create: cloud-backend/internal/core/agentruntime/tool_snapshot_test.go
- Modify: cloud-backend/internal/core/agentruntime/runner.go
- Modify: cloud-backend/internal/core/agentruntime/repository.go
- Modify: cloud-backend/internal/core/workflow/run_model.go
- Modify: cloud-backend/internal/core/workflow/run_repository.go
- Modify: cloud-backend/internal/core/database/database.go

**Interfaces:**
- Produces ToolSnapshot with ID, SHA256, CanonicalJSON, and CreatedAt.
- Produces BuildToolSnapshot([]*tool.ToolManifest) (ToolSnapshot, error).
- Adds TraceID, ToolRegistrySnapshotID, RunManifest, ParentRunID, and ReplayFromStageID to runs.

- [ ] **Step 1: Write the deterministic snapshot test**

~~~go
func TestBuildToolSnapshotIsByteStable(t *testing.T) {
    a := []*tool.ToolManifest{{Name: "zeta"}, {Name: "alpha"}}
    b := []*tool.ToolManifest{{Name: "alpha"}, {Name: "zeta"}}
    left, err := BuildToolSnapshot(a)
    if err != nil { t.Fatal(err) }
    right, err := BuildToolSnapshot(b)
    if err != nil { t.Fatal(err) }
    if left.SHA256 != right.SHA256 || string(left.CanonicalJSON) != string(right.CanonicalJSON) {
        t.Fatal("equivalent registries produced different snapshots")
    }
}
~~~

- [ ] **Step 2: Run the test**

Run: cd cloud-backend && go test ./internal/core/agentruntime -run TestBuildToolSnapshot -count=1

Expected: FAIL because BuildToolSnapshot is undefined.

- [ ] **Step 3: Implement canonical sorting, hashing, and persistence**

Sort tools by logical name, sort every string list, serialize map keys deterministically, and hash exact bytes with SHA-256. Build once before planning and store the ID in run metadata. Add idempotent columns for trace_id, tool_registry_snapshot_id, run_manifest, parent_run_id, and replay_from_stage_id.

- [ ] **Step 4: Run Agent and workflow tests**

Run: cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/workflow -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add cloud-backend/internal/core/agentruntime cloud-backend/internal/core/workflow cloud-backend/internal/core/database/database.go
git commit -m "feat: persist immutable run manifests"
~~~

### Task 5: Redacted cloud summary and delivery outbox

**Files:**
- Create: cloud-backend/internal/core/observability/repository.go
- Create: cloud-backend/internal/core/observability/repository_test.go
- Create: cloud-backend/internal/core/observability/handler.go
- Create: cloud-backend/internal/core/observability/handler_test.go
- Modify: cloud-backend/internal/core/database/database.go
- Modify: cloud-backend/cmd/tangying-ai-os/main.go
- Modify: cloud-backend/internal/core/apispec/cloud_spec_test.go

**Interfaces:**
- Produces Repository.SaveSummary, Pull, and Acknowledge.
- Produces GET /api/observability/events.
- Produces POST /api/observability/events/ack.
- Produces GET /api/observability/runs/:runId/summary.

- [ ] **Step 1: Write authenticated-user and pagination tests**

~~~go
func TestPullEventsUsesAuthenticatedUserScope(t *testing.T) {
    repo := &fakeRepository{page: EventPage{Events: []Event{{EventID: "evt_1"}}}}
    handler := NewHandler(repo)
    r := gin.New()
    r.GET("/events", func(c *gin.Context) { c.Set("userID", "user_a"); c.Next() }, handler.Pull)
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/events?limit=50", nil))
    if repo.lastUserID != "user_a" { t.Fatalf("user scope = %q", repo.lastUserID) }
}
~~~

- [ ] **Step 2: Run handler tests**

Run: cd cloud-backend && go test ./internal/core/observability -run 'TestPull|TestAcknowledge|TestRunSummary' -count=1

Expected: FAIL.

- [ ] **Step 3: Implement storage and routes**

Create observability_event_outbox with unique event_id, user_id, correlation indexes, redacted payload, delivered_at, and expires_at. Create observability_run_summaries keyed by user and run. Outbox rows expire after 72 hours. Pull uses a stable occurred_at plus event_id cursor and caps limit at 500. Save rejects an event when ContainsSecret remains true.

- [ ] **Step 4: Run observability and API tests**

Run: cd cloud-backend && go test ./internal/core/observability ./internal/core/apispec -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add cloud-backend/internal/core/observability cloud-backend/internal/core/database/database.go cloud-backend/internal/core/apispec/cloud_spec_test.go cloud-backend/cmd/tangying-ai-os/main.go
git commit -m "feat: add redacted observability relay"
~~~

### Task 6: Instrument cloud critical paths and gate the phase

**Files:**
- Modify: cloud-backend/internal/core/agentruntime/runner.go
- Modify: cloud-backend/internal/core/agentruntime/runner_test.go
- Modify: cloud-backend/internal/core/translator/service/translator.go
- Modify: cloud-backend/internal/core/workflow/run_service.go
- Modify: cloud-backend/internal/core/workflow/decision_log.go
- Modify: cloud-backend/internal/core/localrunner/handler.go
- Modify: cloud-backend/internal/core/worker/service/executor.go
- Create: docs/observability/event-contract.md
- Modify: README.md
- Modify: package.json

**Interfaces:**
- Consumes Emitter.Emit and correlation context.
- Produces paired lifecycle events for task, run, stage, Agent, LLM, tool, local job, Verify, Correct, retry, fallback, and terminal outcomes.

- [ ] **Step 1: Add lifecycle and raw-content tests**

Capture a failed Agent run and assert that agent.run.failed exists while the serialized event set does not contain the private request text. Add the same test pattern for translator prompts and local-job payloads.

- [ ] **Step 2: Run focused tests**

Run: cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/translator/... ./internal/core/workflow ./internal/core/localrunner -count=1

Expected: FAIL on missing events or leaked content.

- [ ] **Step 3: Instrument critical boundaries**

Emit start and terminal events around planning, validation, compilation, DAG submission, stage transition, LLM, tool, local job, Verify, Correct, retry, and fallback. Include duration, attempt, stable error code, references, and hashes. Remove raw userInput and prompt fields from Zap calls. Reject decision-log writes that lack a resolved workflowRunId.

- [ ] **Step 4: Add the aggregate gate and documentation**

Add:

~~~json
"test:observability-foundation": "npm run test:observability-contract && cd cloud-backend && go test ./internal/core/observability ./internal/core/agentruntime ./internal/core/workflow ./internal/core/localrunner ./internal/core/translator/... -count=1"
~~~

Document the schema, identifiers, privacy boundary, tool snapshot, and relay endpoints.

- [ ] **Step 5: Run the phase gates**

Run:

~~~bash
npm run test:observability-foundation
cd cloud-backend && go test ./... -count=1
~~~

Expected: PASS.

- [ ] **Step 6: Commit**

~~~bash
git add cloud-backend package.json README.md docs/observability/event-contract.md
git commit -m "feat: instrument cloud creation lifecycle"
~~~

## Phase acceptance gate

- Contract verification passes without network access.
- Equivalent tool registries produce byte-identical snapshots.
- Trace context survives HTTP, Agent, workflow, and local-job boundaries.
- Cloud storage rejects secret-bearing events.
- Pull is authenticated, scoped, paginated, and acknowledgement is idempotent.
- Critical cloud lifecycle operations have paired events.
- Existing cloud tests pass.
