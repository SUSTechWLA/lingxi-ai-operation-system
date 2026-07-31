# Local Observability Store and Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Build the canonical local JSONL journal, rebuildable SQLite index, retention and crash recovery, secure query API, cloud synchronization, scoped diagnostic export, and standard read-only diagnostic tools.

**Architecture:** The Local Agent owns one asynchronous observability service. Accepted events are allowlist-redacted, appended to a rotating hash-chained journal, indexed in SQLite WAL mode, and exposed through authenticated scoped queries. Cloud events synchronize by event ID; AI access uses registered read-only Local Agent tools.

**Tech Stack:** Go 1.24, modernc.org/sqlite v1.54.0, github.com/klauspost/compress v1.19.0, net/http, Electron Node.js runtime, TypeScript HyperFrames service.

## Global Constraints

- JSONL is the forensic source of truth; SQLite is disposable and fully rebuildable.
- Default local capacity is 2 GB.
- Retain successful ordinary events for 14 days, failed-run evidence for 30 days, and audit events for 180 days.
- Active runs, unresolved crashes, and incomplete recovery evidence are never automatically deleted.
- Critical state, approval, error, and checkpoint events are durably flushed.
- Noncritical events may buffer for at most approximately one second.
- Local write and query APIs require a per-session capability and trusted origin.
- AI tools are read-only, scoped, paginated, redacted, and audited.
- Diagnostic creation and upload are not available through read-only AI tools.

---

## File structure

- Create local-backend/internal/observability for event types, redaction, journal, index, service, queries, retention, and recovery.
- Create Local Agent observability handlers, cloud synchronization, and diagnostic bundle modules.
- Modify Electron launch and frontend local-agent requests for a per-session capability.
- Instrument Local Runner, local tools, Electron subprocesses, and HyperFrames.
- Register six local diagnostic tools through existing tool manifests and Local Runner commands.

### Task 1: Local event model and leak prevention

**Files:**
- Create: local-backend/internal/observability/event.go
- Create: local-backend/internal/observability/event_test.go
- Create: local-backend/internal/observability/redaction.go
- Create: local-backend/internal/observability/redaction_test.go
- Modify: local-backend/go.mod
- Create: local-backend/go.sum

**Interfaces:**
- Produces Event, SearchQuery, EventPage, TraceView, Timeline, ErrorExplanation, and DiagnosticScope.
- Produces ValidateAndRedact(Event) (Event, error).
- Produces ScanForSecrets([]byte) []Leak.

- [ ] **Step 1: Pin the storage dependencies**

Run:

~~~bash
cd local-backend
go get modernc.org/sqlite@v1.54.0
go get github.com/klauspost/compress@v1.19.0
~~~

Expected: go.mod and go.sum contain the pinned modules. Keep the modernc.org/libc version selected by the sqlite module unchanged.

- [ ] **Step 2: Write failing contract and leak tests**

Marshal the local Event and compare its required keys with contracts/observability/v1/event.schema.json. Inject unique Authorization, cookie, token, password, API key, prompt, user input, provider error, and absolute-path values.

~~~go
func TestValidateAndRedactDropsForbiddenEvidence(t *testing.T) {
    event := validEvent()
    event.Evidence.Attributes = map[string]any{
        "authorization": "Bearer synthetic-secret",
        "artifactId": "art_1",
    }
    got, err := ValidateAndRedact(event)
    if err != nil { t.Fatal(err) }
    data, _ := json.Marshal(got)
    if bytes.Contains(data, []byte("synthetic-secret")) { t.Fatal("secret leaked") }
    if !bytes.Contains(data, []byte("art_1")) { t.Fatal("safe reference was removed") }
}
~~~

- [ ] **Step 3: Run tests**

Run: cd local-backend && go test ./internal/observability -run 'TestEvent|TestValidateAndRedact|TestScanForSecrets' -count=1

Expected: FAIL because the package is incomplete.

- [ ] **Step 4: Implement explicit types and an allowlist redactor**

Use the shared JSON names exactly. Validate identity, timestamps, severity, event type, message key, source, correlation, and privacy. Remove forbidden attributes. Scan results contain classification and location only, never the secret itself.

- [ ] **Step 5: Run tests and commit**

Run: cd local-backend && go test ./internal/observability -count=1

Expected: PASS.

~~~bash
git add local-backend/go.mod local-backend/go.sum local-backend/internal/observability
git commit -m "feat: add local observability event model"
~~~

### Task 2: Append-only journal, rotation, and checksum chain

**Files:**
- Create: local-backend/internal/observability/journal.go
- Create: local-backend/internal/observability/journal_test.go

**Interfaces:**
- Produces OpenJournal(root string, clock Clock) (*Journal, error).
- Produces Journal.Append(Event, FlushMode) error.
- Produces Journal.Rotate, Replay, and Close.

- [ ] **Step 1: Write rotation, replay, and concurrency tests**

~~~go
func TestJournalRotatesAndChainsChecksums(t *testing.T) {
    root := t.TempDir()
    journal, err := OpenJournal(root, fixedClock())
    if err != nil { t.Fatal(err) }
    journal.maxBytes = 512
    for i := 0; i < 20; i++ {
        if err := journal.Append(testEvent(i), FlushCritical); err != nil { t.Fatal(err) }
    }
    if err := journal.Close(); err != nil { t.Fatal(err) }
    manifest := readManifest(t, root)
    if len(manifest.Files) < 2 { t.Fatalf("files = %d", len(manifest.Files)) }
    if manifest.Files[1].PreviousSHA256 != manifest.Files[0].SHA256 {
        t.Fatal("checksum chain broken")
    }
}
~~~

Also start 50 goroutines, append unique event IDs, replay, and assert exactly 50 intact records.

- [ ] **Step 2: Run journal tests**

Run: cd local-backend && go test ./internal/observability -run 'TestJournal|TestConcurrentAppend' -count=1

Expected: FAIL.

- [ ] **Step 3: Implement safe append and rotation**

Use one writer goroutine. Write one event per JSON line under observability/events. Flush critical events with file.Sync. Rotate at 64 MB or UTC-day change, compress closed files with Zstandard, calculate SHA-256, and atomically replace the manifest using write-then-rename.

If the journal cannot be opened, append to observability/spool/pending-events.jsonl. A journal error writes to the independent crash spool and never recursively calls the journal.

- [ ] **Step 4: Run race tests**

Run: cd local-backend && go test -race ./internal/observability -run 'TestJournal|TestConcurrentAppend' -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/observability/journal.go local-backend/internal/observability/journal_test.go
git commit -m "feat: add append-only observability journal"
~~~

### Task 3: SQLite index and deterministic timeline queries

**Files:**
- Create: local-backend/internal/observability/index.go
- Create: local-backend/internal/observability/index_test.go
- Create: local-backend/internal/observability/crypto.go
- Create: local-backend/internal/observability/crypto_test.go
- Create: local-backend/internal/observability/query.go
- Create: local-backend/internal/observability/query_test.go

**Interfaces:**
- Produces OpenIndex(path string) (*Index, error).
- Produces Insert, Search, Trace, Timeline, ExplainError, and VerifyArtifact methods.
- Produces EncryptEnvelope and DecryptEnvelope using an OS-protected data key supplied at Local Agent startup.

- [ ] **Step 1: Write deduplication, isolation, and clock-skew tests**

~~~go
func TestTimelineUsesCausalityBeforeClockTime(t *testing.T) {
    index := openTestIndex(t)
    child := testEvent(2)
    child.Correlation.ParentSpanID = "spn_parent"
    child.OccurredAt = time.Unix(1, 0)
    parent := testEvent(1)
    parent.Correlation.SpanID = "spn_parent"
    parent.OccurredAt = time.Unix(2, 0)
    for _, event := range []Event{child, parent, child} {
        if err := index.Insert(event); err != nil { t.Fatal(err) }
    }
    got, err := index.Timeline(parent.Correlation.WorkflowRunID)
    if err != nil { t.Fatal(err) }
    if len(got.Events) != 2 || got.Events[0].EventID != parent.EventID {
        t.Fatalf("timeline = %#v", got.Events)
    }
}
~~~

Test that a project-scoped search cannot return an event from another project and that limits above 500 are clamped. Test that the SQLite envelope column does not contain the plaintext event while an authorized query decrypts it correctly.

- [ ] **Step 2: Run index tests**

Run: cd local-backend && go test ./internal/observability -run 'TestIndex|TestTimeline|TestSearch|TestExplainError' -count=1

Expected: FAIL.

- [ ] **Step 3: Implement WAL schema and bounded queries**

Store indexed columns for event identity, time, severity, event type, source, every correlation identifier, status, error code, fingerprint, classification, and an AES-256-GCM encrypted redacted JSON envelope. Use event_id as primary key and ON CONFLICT DO NOTHING. The data key comes from TANGYING_OBSERVABILITY_KEY and is never stored in SQLite or the journal manifest.

Order a timeline by causal edges and producer sequence; timestamps break only otherwise-equal positions. Search uses a stable occurred_at plus event_id cursor.

- [ ] **Step 4: Run index and rebuild tests**

Run: cd local-backend && go test ./internal/observability -run 'TestIndex|TestTimeline|TestSearch|TestExplainError|TestRebuild' -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/observability/index.go local-backend/internal/observability/index_test.go local-backend/internal/observability/crypto.go local-backend/internal/observability/crypto_test.go local-backend/internal/observability/query.go local-backend/internal/observability/query_test.go
git commit -m "feat: index local observability events"
~~~

### Task 4: Service, retention, and crash recovery

**Files:**
- Create: local-backend/internal/observability/service.go
- Create: local-backend/internal/observability/service_test.go
- Create: local-backend/internal/observability/retention.go
- Create: local-backend/internal/observability/retention_test.go
- Create: local-backend/internal/observability/recovery.go
- Create: local-backend/internal/observability/recovery_test.go

**Interfaces:**
- Produces OpenService(Config) (*Service, error).
- Produces Emit, Flush, RunMaintenance, Recover, and Close.
- Produces RecoveryReport with rebuilt count and interrupted-run classifications.

- [ ] **Step 1: Write retention and restart tests**

Create a 15-day successful event, 29-day failed event, 179-day audit event, active run, unfinished span, and journal event missing from SQLite. Assert maintenance removes only the expired successful event and recovery reindexes the missing event.

- [ ] **Step 2: Run service tests**

Run: cd local-backend && go test ./internal/observability -run 'TestService|TestRetention|TestRecovery' -count=1

Expected: FAIL.

- [ ] **Step 3: Implement async ingestion and protected cleanup**

Use one bounded queue. Validate before enqueue. Critical events wait for journal durability; ordinary events return after bounded enqueue. At more than 2 GB, remove eligible data in the approved priority order and emit one content-free cleanup audit event.

Recovery replays journal files, restores index rows, identifies unterminated spans and runs, checks local artifact references, and classifies runs as RECOVERABLE, USER_ACTION_REQUIRED, or TERMINAL.

- [ ] **Step 4: Run service and race tests**

Run: cd local-backend && go test -race ./internal/observability -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/observability
git commit -m "feat: add observability retention and recovery"
~~~

### Task 5: Secure Local Agent API and desktop capability

**Files:**
- Create: local-backend/internal/localagent/observability_handlers.go
- Create: local-backend/internal/localagent/observability_handlers_test.go
- Modify: local-backend/internal/localagent/server.go
- Modify: local-backend/internal/localagent/openapi.go
- Modify: local-backend/cmd/local-agent/main.go
- Modify: frontend/electron/local-agent-runtime.cjs
- Modify: frontend/electron/local-agent-runtime.test.cjs
- Create: frontend/electron/observability-key-runtime.cjs
- Create: frontend/electron/observability-key-runtime.test.cjs
- Modify: frontend/electron/main.cjs
- Modify: frontend/electron/preload.cjs
- Modify: frontend/src/utils/electron.d.ts
- Modify: frontend/src/services/localAgent.ts

**Interfaces:**
- Produces POST and GET /api/local/observability/events.
- Produces GET /api/local/observability/traces/:traceId.
- Produces GET /api/local/observability/runs/:runId/timeline.
- Produces GET /api/local/observability/errors/:fingerprint.
- Requires X-Tangying-Local-Capability.

- [ ] **Step 1: Write authorization, origin, and scope tests**

~~~go
func TestObservabilityQueryRejectsMissingCapability(t *testing.T) {
    server := newObservabilityTestServer(t, "cap_test")
    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/local/observability/events?projectId=prj_1", nil)
    server.Handler().ServeHTTP(rec, req)
    if rec.Code != http.StatusUnauthorized { t.Fatalf("status = %d", rec.Code) }
}
~~~

Test a valid capability, a hostile Origin, no scope, and a different project.

- [ ] **Step 2: Run tests**

Run: cd local-backend && go test ./internal/localagent -run Observability -count=1

Expected: FAIL.

- [ ] **Step 3: Implement composition, routes, and capability flow**

Generate a random 32-byte application-session capability in Electron. Pass it through TANGYING_LOCAL_CAPABILITY and attach it through the existing frontend local-agent request helper. Never persist or log it.

Generate the separate persistent observability data key once, encrypt it with Electron safeStorage, store only the encrypted blob under the Electron user-data directory, and pass the decrypted key to the Local Agent as TANGYING_OBSERVABILITY_KEY. Never expose the data key through preload or renderer runtime configuration.

Queries require projectId or workflowRunId, enforce indexed ownership, cap results at 500, and return contract DTOs instead of database rows.

- [ ] **Step 4: Run Local Agent, Electron, and frontend checks**

Run:

~~~bash
cd local-backend && go test ./internal/localagent ./internal/observability -count=1
cd ../frontend && node --test electron/local-agent-runtime.test.cjs && npm run build
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/localagent local-backend/cmd/local-agent frontend/electron frontend/src/services/localAgent.ts frontend/src/utils/electron.d.ts
git commit -m "feat: expose secure local observability API"
~~~

### Task 6: Cloud sync and local execution instrumentation

**Files:**
- Create: local-backend/internal/localagent/observability_sync.go
- Create: local-backend/internal/localagent/observability_sync_test.go
- Modify: local-backend/internal/localrunner/loop.go
- Modify: local-backend/internal/localrunner/loop_test.go
- Modify: local-backend/internal/localtool/registry.go
- Modify: cloud-backend/internal/core/localrunner/model.go
- Create: frontend/electron/observability-runtime.cjs
- Create: frontend/electron/observability-runtime.test.cjs
- Modify: frontend/electron/main.cjs
- Create: hyperframes-render-service/src/observability.ts
- Modify: hyperframes-render-service/src/types.ts
- Modify: hyperframes-render-service/src/server.ts
- Modify: hyperframes-render-service/src/render.ts
- Modify: hyperframes-render-service/src/queue.ts

**Interfaces:**
- Consumes cloud event pull and acknowledgement routes.
- Produces Local Runner, tool, Electron subprocess, and HyperFrames lifecycle events.

- [ ] **Step 1: Write sync deduplication and adapter tests**

Test that two cloud pages containing the same eventId create one local record, acknowledgement occurs only after durable append, and a failed append preserves the cursor. Test that Electron stdout and stderr become bounded structured lines with secrets removed.

- [ ] **Step 2: Run focused tests**

Run:

~~~bash
cd local-backend && go test ./internal/localagent ./internal/localrunner -run 'Observability|Sync|Lifecycle' -count=1
cd ../frontend && node --test electron/observability-runtime.test.cjs
~~~

Expected: FAIL.

- [ ] **Step 3: Implement synchronization and adapters**

Pull with authenticated user credentials, insert by eventId, and acknowledge after journal and index acceptance. Back off with jitter while offline. Include traceId, spanId, parentSpanId, workflowRunId, taskId, nodeId, shotId, toolCallId, attempt, and duration in the cloud LocalJob payload and local execution events.

HyperFrames emits render.queued, render.started, coalesced render.progress, render.completed, and render.failed as one-line JSON. Do not include raw projectDir or outputPath.

- [ ] **Step 4: Run builds and tests**

Run:

~~~bash
cd local-backend && go test ./internal/localagent ./internal/localrunner ./internal/localtool -count=1
cd ../frontend && node --test electron/*.test.cjs && npm run build
cd ../hyperframes-render-service && npm run build
~~~

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend frontend/electron hyperframes-render-service/src
git commit -m "feat: synchronize and instrument local execution"
~~~

### Task 7: Scoped diagnostic preview and export

**Files:**
- Create: local-backend/internal/localagent/diagnostic_bundle.go
- Create: local-backend/internal/localagent/diagnostic_bundle_test.go
- Modify: local-backend/internal/localagent/server.go
- Modify: local-backend/internal/localagent/server_test.go
- Modify: local-backend/internal/localagent/openapi.go

**Interfaces:**
- Produces POST /api/local/diagnostics/preview.
- Revises POST /api/local/diagnostics to require a preview token.
- Produces manifest entries with relative path, classification, size, and SHA-256.

- [ ] **Step 1: Replace broad-export tests with scoped tests**

Create two projects and tasks. Preview one task and assert the other project's identifiers, artifacts, reports, and events are absent. Create the ZIP with the preview token and assert every member is in the manifest and passes the leak scanner.

- [ ] **Step 2: Run diagnostic tests**

Run: cd local-backend && go test ./internal/localagent -run 'DiagnosticPreview|ScopedDiagnostic|DiagnosticLeak' -count=1

Expected: FAIL because the current exporter walks broad directories.

- [ ] **Step 3: Implement exact-manifest export**

Preview resolves exact files and event ranges, returns classifications and sizes, and stores a short-lived privileged UI token bound to scope and manifest hash. Creation requires the token and unchanged manifest. Scan again while creating the ZIP and fail closed on a leak. Name files diagnostic-YYYYMMDD-HHMMSS-scope.zip.

- [ ] **Step 4: Run Local Agent tests**

Run: cd local-backend && go test ./internal/localagent -count=1

Expected: PASS.

- [ ] **Step 5: Commit**

~~~bash
git add local-backend/internal/localagent
git commit -m "feat: scope local diagnostic exports"
~~~

### Task 8: Register standard read-only AI tools and gate the phase

**Files:**
- Create: local-backend/internal/localtool/observability_queries.go
- Create: local-backend/internal/localtool/observability_queries_test.go
- Modify: local-backend/internal/localtool/bootstrap.go
- Modify: local-backend/internal/localtool/registry.go
- Modify: cloud-backend/internal/core/localrunner/model.go
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/logs_search.tool.yaml
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/traces_get.tool.yaml
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/runs_timeline.tool.yaml
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/errors_explain.tool.yaml
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/artifacts_verify.tool.yaml
- Create: cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools/diagnostics_preview.tool.yaml
- Create: docs/observability/local-store.md
- Modify: package.json
- Modify: README.md

**Interfaces:**
- Produces logical tools logs.search, traces.get, runs.timeline, errors.explain, artifacts.verify, and diagnostics.preview.
- Produces local commands OBSERVABILITY_LOGS_SEARCH, OBSERVABILITY_TRACES_GET, OBSERVABILITY_RUNS_TIMELINE, OBSERVABILITY_ERRORS_EXPLAIN, OBSERVABILITY_ARTIFACTS_VERIFY, and OBSERVABILITY_DIAGNOSTICS_PREVIEW.

- [ ] **Step 1: Write read-only scope and pagination tests**

Assert limits above 500 are clamped, cross-project identifiers are rejected, results remain redacted, each call emits diagnostic.query.completed, and no executor exposes create, upload, delete, retry, regenerate, or mutation behavior.

- [ ] **Step 2: Run tool and manifest tests**

Run:

~~~bash
cd local-backend && go test ./internal/localtool -run Observability -count=1
cd ../cloud-backend && go test ./internal/core/worker/tool ./internal/core/skillcapability -count=1
~~~

Expected: FAIL.

- [ ] **Step 3: Implement executors and manifests**

Use executionPlane local, requiresUserDevice true, sideEffect false, idempotent true, riskLevel low, and required scope parameters. Register through RegisterDefaultExecutors and add matching commands to local and cloud allowlists. diagnostics.preview calls the pure query-service preview method and returns a manifest only; it does not create the privileged UI preview token used for ZIP creation.

- [ ] **Step 4: Add and run the aggregate gate**

Add:

~~~json
"test:observability-local": "cd local-backend && go test -race ./internal/observability ./internal/localagent ./internal/localrunner ./internal/localtool -count=1 && cd ../frontend && node --test electron/*.test.cjs && npm run build && cd ../hyperframes-render-service && npm run build"
~~~

Run: npm run test:observability-local

Expected: PASS.

- [ ] **Step 5: Document and commit**

Document the local directory, rotation, checksum chain, index rebuild, retention, capability header, diagnostic preview, and six read-only tools.

~~~bash
git add local-backend cloud-backend/internal/core/localrunner/model.go cloud-backend/skill-capabilities/video/codex-video-skill/1.0.0/tools package.json README.md docs/observability/local-store.md
git commit -m "feat: add local observability diagnostics"
~~~

## Phase acceptance gate

- Concurrent ingest survives the race detector.
- Deleting SQLite and restarting reconstructs the timeline from JSONL.
- Rotation occurs at 64 MB or day boundary and journals form a checksum chain.
- Retention tiers and the 2 GB limit pass with an injected clock.
- Unauthenticated and cross-project queries are rejected.
- Cloud sync is idempotent and acknowledges only durable events.
- Diagnostic preview and ZIP contain only the selected scope.
- Six AI tools are read-only, dynamically registered, and enter only new task snapshots.
- Local Agent, Electron, and HyperFrames builds pass.
