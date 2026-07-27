# Task 1 implementer report

Base: `10bea8bba4a7579457b7b6f4dcca7acab80e2e6d`

## Implementation summary

- Split Agent terminal delivery into durable monotonic callback,
  observability, and final-ack phases. Every phase update is a
  run/event/claim-token CAS; final ack additionally requires both prior phase
  timestamps. A false phase mark, release, or final acknowledgement is claim
  loss and is returned as an error.
- Freeze `RunTerminalEvent`, explicit `callbackIdempotencyKey`, authenticated
  owner, and the fully prepared observability event before side effects. The
  emitter now supports prepare-once and byte-identical frozen replay, including
  event/occurrence/ingest time, producer sequence, source, runtime, trace/span,
  correlation, execution, error, evidence, and privacy fields.
- Recover legacy JSON without an event ID from authoritative SQL
  `terminal_event_id`, derive the span deterministically from that ID, and
  persist the completed frozen payload under the active claim before callback
  or emit. Identity mismatches fail closed.
- Upgrade/fresh migration adds both phase columns, deterministically replaces
  prior `legacy_terminal_*` IDs (or missing IDs) with a schema-valid JSON ID or
  `evt_agent_terminal_legacy_` plus `MD5(run_id)`, backfills already-delivered
  rows, installs identity/phase-order constraints, and remains a required
  fail-closed startup migration.
- Built-in creator-project terminal receivers now no-op when the same run and
  target terminal state were already applied. Shot-regeneration failure was
  already target-terminal idempotent and its existing consistency tests remain
  in the gate.
- Documented the honest guarantee: physical callback invocation can repeat if
  a process crashes after the receiver commits but before phase marking. The
  stable terminal Event ID is injected as `callbackIdempotencyKey`; idempotent
  receivers collapse the replay, providing effectively-once application. The
  observability repository similarly collapses byte-identical replay by Event
  ID.

No Workflow snapshot or worker execution production code was changed.

## Tests added or amended

- `cloud-backend/internal/core/agentruntime/terminal_delivery_phase_test.go`
  - callback success then emit failure does not repeat callback
  - callback payload and full observability envelope are byte-identical
  - real emitter prepared envelope remains byte-identical
  - legacy SQL identity/runtime survives process restart
  - Ack false is claim loss
  - emit success then ack loss does not repeat callback or emit
  - concurrent reconcilers produce one claimed delivery
  - expired-lease crash window reuses one receiver idempotency identity
  - callback receives the explicit stable idempotency key
- `repository_manifest_test.go` and `repository_terminal_test.go`
  - phase scan state, legacy SQL identity recovery, frozen claimed JSON, exact
    CAS predicates, and ack phase prerequisites
- `video_project_revision_test.go`
  - migration phase/identity/backfill guards and required failure propagation
- `project_service_test.go`
  - completed/stopped callback replay performs only one project CAS
- Existing first-terminal-wins, success/failure/cancellation, planning failure,
  callback retry, and shot-regeneration terminal tests were rerun.

## TDD evidence

Initial RED:

```text
$ go test ./internal/core/agentruntime -run 'TestRunner(CallbackSuccessThenObservabilityFailureDoesNotRepeatCallback|TerminalReplayUsesByteIdenticalFrozenObservabilityEnvelope|AckFalseIsClaimLoss|TerminalCallbackReceivesStableExplicitIdempotencyKey)$' -count=1
--- FAIL: TestRunnerCallbackSuccessThenObservabilityFailureDoesNotRepeatCallback
    callback count = 2, want one durable callback phase
--- FAIL: TestRunnerTerminalReplayUsesByteIdenticalFrozenObservabilityEnvelope
    first spanId=5dd50f5faa92da4d, second spanId=c9514f453a2f2dd7
--- FAIL: TestRunnerAckFalseIsClaimLoss
    AckTerminalEvent(false) error = <nil>
--- FAIL: TestRunnerTerminalCallbackReceivesStableExplicitIdempotencyKey
    idempotencyKey=""
FAIL
```

Prepared-envelope RED:

```text
--- FAIL: TestRunnerTerminalReplayThroughEmitterKeepsPreparedEnvelopeByteIdentical
first:  ingestedAt=...324705Z producerSequence=0
second: ingestedAt=...324751Z producerSequence=1
FAIL
```

Receiver and migration fail-closed RED:

```text
--- FAIL: TestProjectServiceTerminalCallbackReplayIsIdempotentForSameRun/completed
    CAS calls=2
--- FAIL: TestProjectServiceTerminalCallbackReplayIsIdempotentForSameRun/stopped
    CAS calls=2
# database test build
undefined: ensureAgentTerminalOutbox
```

All of those tests were then observed GREEN.

## Verification commands and actual results

Focused and race:

```text
$ GOCACHE=/tmp/codex-observability-go-cache go test -race ./internal/core/agentruntime -run '<10 terminal phase tests>' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.605s

$ GOCACHE=/tmp/codex-observability-go-cache go test ./internal/core/agentruntime -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.623s

$ GOCACHE=/tmp/codex-observability-go-cache go test ./internal/core/database ./internal/core/observability ./internal/agents/video/service -count=1
ok github.com/tangying-ai/aios-core/internal/core/database 0.551s
ok github.com/tangying-ai/aios-core/internal/core/observability 0.955s
ok github.com/tangying-ai/aios-core/internal/agents/video/service 1.578s
```

Observability foundation:

```text
$ npm run test:observability-foundation
test:observability-contract: PASS
observability: PASS
workflow: PASS
localrunner: PASS
translator/handler: PASS
translator/service: PASS
worker/service: PASS
agentruntime: FAIL only at httptest.NewServer: listen tcp6 [::1]:0: operation not permitted

$ npm run test:observability-contract && cd cloud-backend && \
  go test ./internal/core/observability ./internal/core/workflow ./internal/core/localrunner ./internal/core/translator/... ./internal/core/worker/service -count=1 && \
  go test ./internal/core/agentruntime -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 0.359s
ok github.com/tangying-ai/aios-core/internal/core/workflow 0.518s
ok github.com/tangying-ai/aios-core/internal/core/localrunner 0.679s
ok github.com/tangying-ai/aios-core/internal/core/translator/handler 1.126s
ok github.com/tangying-ai/aios-core/internal/core/translator/service 0.895s
ok github.com/tangying-ai/aios-core/internal/core/worker/service 1.367s
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.038s
```

Cloud full suite and sandbox isolation:

```text
$ cd cloud-backend && go test ./... -count=1
Two packages stopped at their first httptest.NewServer test; errors:
TestClientProviderPlannerUsesRequestTextProvider: listen tcp6 [::1]:0: operation not permitted
TestLlmApiTool_Execute_UsesIntegerMaxTokensAndJSONMode: listen tcp6 [::1]:0: operation not permitted
A source inventory found 13 listener-binding tests total. All packages reached
outside the two stopped packages reported ok/no-test-files.

$ cd cloud-backend && go test ./... -skip '<exact 13 httptest.NewServer test names>' -count=1
All cloud packages: PASS (exit 0); agentruntime 2.006s, database 1.792s,
observability 1.445s, video/service 1.926s, worker/tool/builtin 1.654s,
workflow 1.515s.
```

Static gates:

```text
$ cd cloud-backend && GOCACHE=/tmp/codex-observability-go-cache go vet ./...
(no output, exit 0)

$ git diff --check
(no output, exit 0)
```

## Fix Round 4 — forward-compatible terminal CAS and signed-source upgrade

This round starts from `916145e74586322e930633bbe9646dc5363f45a0` and
addresses only the two residual Important Task 1 findings, plus the directly
related CRLF one-click upgrade edge. Workflow snapshot execution remains out of
scope.

### Lossless bounded terminal JSON migration

- The root cause was a closed Go struct decode followed by a full struct
  marshal in `FreezeTerminalEvent`. A claimed migration therefore discarded
  every future top-level, prepared-envelope, binding, source, runtime, and
  privacy member before the CAS.
- `RunTerminalEvent` now retains a bounded, duplicate-safe ordered raw-object
  baseline only when unknown members exist. Marshal performs a surgical merge:
  unchanged known fields keep their original bytes, changed known fields are
  authoritative, and unknown values survive recursively with exact arrays,
  objects, nulls, and large integer spelling.
- Validation runs before the normal Go decode and rejects duplicate object keys
  at every depth, noncanonical case aliases of known keys, trailing data,
  payloads over 256 KiB, depth over 32, and more than 16K values. This bounds the
  preservation state and prevents an unknown field from hiding a later known
  alias.
- Direct decode/remarshal and repository CAS tests use `futureTop`, `futureV2`,
  future binding/source/runtime/privacy members, and integers beyond JavaScript
  precision. The CAS changes the known sealed payload and proves every future
  member remains present.

### Explicit authenticated source-environment migration

- The pre-fix one-click environment had no `GIN_MODE` or `APP_ENV`, so durable
  envelopes were signed as `development`. The newer compose runtime forced
  `production`, and using runtime mode as the signed source made both v1 and v2
  pending rows permanently unrestorable.
- `OBSERVABILITY_SOURCE_ENVIRONMENT` is now an explicit stable signed identity,
  independent from runtime validation mode. Production requires it explicitly;
  development retains the historical `development` default. The optional
  `OBSERVABILITY_PREVIOUS_SOURCE_ENVIRONMENTS` is a canonical, duplicate-free,
  at-most-eight-entry allowlist.
- A v2 migration first opens and authenticates the canonical HMAC envelope,
  domain, owner, trusted terminal-row binding, component,
  and parent/event service and component. Only an allowlisted historical
  environment can change. A v1 bridge similarly verifies its digest and exact
  claimed-row value before producing the current HMAC envelope.
- Runner persists the resealed payload through the existing run/event/claim CAS
  before callback. Claim loss forbids callback. A current-source v2 envelope is
  a byte-identical no-op; after a successful CAS, a restart without the
  historical allowlist restores and delivers the current-source envelope.
- Tests cover development v1 and v2 to production, production no-op, post-CAS
  callback crash plus no-allowlist restart, lost claim, unlisted staging,
  foreign service/component, and tampered HMAC.

### One-click upgrade and CRLF behavior

- Compose passes the explicit current and previous source variables. Existing
  one-click environments add `production` plus temporary `development`; fresh
  environments write `production` with an explicitly empty allowlist so later
  reruns cannot accidentally enable migration.
- Existing CRLF env files are normalized atomically to LF after owner/mode
  checks, without rotating or printing the sealing key. Embedded carriage
  returns, duplicate variables, invalid source identifiers, duplicate previous
  values, and leading/trailing/consecutive comma ambiguity fail closed.
- The beta runbook documents drain, allowlist removal, and rollback rules.

### Fix Round 4 TDD evidence

Initial RED was observed before production changes:

```text
TestRunTerminalEventJSONPreservesFutureTopAndPreparedV2Fields:
  future top-level and prepared v2 fields were absent after remarshal
TestRepositoryTerminalMigrationCASPreservesUnknownJSON:
  claimed CAS payload discarded futureTop/futureV2
TestRunTerminalEventJSONRejectsDuplicateKeysAtEveryDepth:
  duplicate keys were accepted
TestRunTerminalEventJSONEnforcesSizeAndDepthBounds:
  oversized and deeply nested payloads were accepted

TestRunnerMigratesPreviousSourceV2BeforeCallbackAndRestart:
  prepared observability parent source domain mismatch

scripts/test_one_click_deploy.py:
  6 failures for missing source wiring, CRLF key parsing, and duplicate handling
test_ambiguous_previous_source_list_stops_upgrade:
  development, was accepted
test_runtime_env_is_private_and_existing_valid_key_is_preserved:
  fresh env omitted the explicit empty previous-source allowlist
```

Focused GREEN and race:

```text
$ go test -race ./internal/core/observability \
    -run 'Test(Persistent|LegacyPreparedEvent)' -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 1.548s

$ go test -race ./internal/core/agentruntime \
    -run 'Test(Runner.*(Terminal|Source|Round1)|RunTerminalEventJSON|RepositoryTerminalMigrationCAS)' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.751s

$ go test ./internal/core/config ./cmd/tangying-ai-os -count=1
ok github.com/tangying-ai/aios-core/internal/core/config
ok github.com/tangying-ai/aios-core/cmd/tangying-ai-os

$ go test ./internal/core/database \
    -run 'Test(AgentTerminal|NormalizeAgent|ValidAgent|EnsureAgent)' -count=1 -v
all always-run migration tests PASS; PostgreSQL harness SKIP because no test DSN is configured
```

Repository gates (using a repository-local `GOCACHE` because the sandbox does
not permit the default user cache path):

```text
$ npm run test:observability-contract
PASS

$ GOFLAGS='-skip=^TestClientProviderPlannerUsesRequestTextProvider$' \
    npm run test:observability-foundation
contract plus observability, agentruntime, workflow, localrunner, translator,
and worker/service: PASS

$ go test ./... -skip '<exact 13 existing httptest.NewServer test names>' -count=1
all cloud packages PASS

$ go vet ./...
(no output, exit 0)

$ bash -n scripts/one-click-deploy.sh scripts/beta-smoke-check.sh
(no output, exit 0)

$ python3 scripts/test_one_click_deploy.py
Ran 16 tests: OK

$ git diff --check
(no output, exit 0)
```

The unmodified foundation run reaches and passes every non-listener package but
still fails at the unchanged sandbox boundary:

```text
TestClientProviderPlannerUsesRequestTextProvider:
httptest: failed to listen on a port: listen tcp6 [::1]:0:
bind: operation not permitted
```

No listener test was modified or treated as passing, and no Task 2 code was
changed.

## Fix Round 2 — authenticated restart capability and fail-closed wiring

Round 1 still exposed a package-level decoder backed only by a public SHA-256
digest. Any ordinary package could construct canonical bytes, recompute the
digest, and obtain a replay capability. The Agent Runner also retained raw
emitter fallbacks. This round removes both residual trust bypasses without
starting Task 2.

### Implementation

- Removed the exported prepared-event marshal/decoder surface. Only a concrete
  persistent component emitter can freeze, restore, bind, and replay an opaque
  capability. External negative-compilation fixtures prove that neither a raw
  `Event` nor the removed decoder can mint a replay capability.
- Replaced the public checksum with a canonical versioned HMAC-SHA-256 envelope.
  The private sealing key/domain, owner, parent source, producer runtime,
  component, and complete redacted event are authenticated. Restore enforces
  payload size, canonical JSON, version, signature, sealing domain, parent
  source, component, event source, registry/redaction, and exact trusted
  binding before returning a capability.
- The trusted binding covers owner, Event ID, complete source and correlation,
  complete historical runtime, schema, privacy/redaction list, severity, event
  type, execution status, error code, and occurred time. A second component-
  owned expected-event check binds the authenticated event to the exact Agent
  outbox row before callback. A valid envelope copied from another row fails
  before callback or sink.
- A restart using the same key/domain and parent/component identity restores the
  historical producer runtime even when the new process has a different app,
  Git, workflow, or prompt version. Wrong keys, foreign domains, foreign parent
  sources, and foreign component adapters fail closed.
- Agent Runner now stores only `PersistentPreparedEventEmitter`, rejects plain
  and typed-nil emitters through explicit startup validation, and has no raw
  `Emit`/`EmitAndWait` fallback. Both pre-callback validation and sink replay go
  through component-owned restore. Legacy missing trace/time values are frozen
  deterministically before side effects.
- Cloud startup constructs a persistent emitter, requests the Agent component,
  and validates the Runner before route registration. Production config rejects
  missing, short, or documented placeholder sealing keys without including the
  supplied value in errors.
- One-click deployment generates a separate random sealing key once, persists
  it in the private runtime env, and upgrades existing env files only when the
  variables are absent. Compose requires the key, release smoke checks require
  key/domain, and the beta runbook documents stable restart and drain-before-
  rotation behavior. No key value is logged, emitted, or recorded here.

### TDD evidence

Initial RED was observed before production changes:

```text
TestPublicPackageDecoderCannotMintPreparedCapability:
  public package decoder still mints PreparedEvent

persistent_sealing_test.go:
  undefined: NewPersistentEmitter
  undefined: PersistentSealingConfig
  undefined: SealedPreparedEvent
  undefined: PersistentPreparedEventEmitter

TestRunnerRejectsNonPreparedEmitterBeforeTerminalCallback:
  runner.ValidateConfiguration undefined

TestValidateForModeRejectsMissingObservabilitySealingKey:
  production missing sealing key error = <nil>
```

The resulting attack/boundary tests cover public-decoder and raw-Event negative
compilation, public-SHA forgery, same-domain restart, wrong/missing key, foreign
domain/source/component, complete source/runtime/schema/privacy binding tamper,
cross-row valid-envelope substitution, pre-callback failure, typed nil, payload
size/version bounds, byte-identical crash-window replay, and non-persistent
Runner startup rejection.

### Fresh Fix Round 2 verification

```text
$ go test -race ./internal/core/observability -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 13.909s

$ go test -race ./internal/core/agentruntime \
    -run 'Test.*Terminal.*|TestRunnerRejects.*Emitter.*|TestCancelRunPausesTaskAndMarksProjectStopped' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.470s

$ go test ./internal/core/config ./cmd/tangying-ai-os -count=1
ok github.com/tangying-ai/aios-core/internal/core/config 0.242s
ok github.com/tangying-ai/aios-core/cmd/tangying-ai-os 0.574s

$ go test ./internal/core/database \
    -run 'Test(AgentTerminal|NormalizeAgent|ValidAgent|EnsureAgent)' -count=1 -v
all always-run migration identity/constraint/startup-error tests PASS;
live PostgreSQL fresh/upgrade test SKIP because no test DSN is configured

$ GOFLAGS='-skip=^TestClientProviderPlannerUsesRequestTextProvider$' \
    npm run test:observability-foundation
contract plus observability, agentruntime, workflow, localrunner, translator,
and worker/service: PASS

$ go test ./... -skip '<exact 13 existing httptest listener tests>' -count=1
all cloud packages PASS, including agentruntime, config, database,
observability, worker/tool/builtin, and workflow

$ go vet ./...
(no output, exit 0)

$ bash -n scripts/one-click-deploy.sh scripts/beta-smoke-check.sh
(no output, exit 0)

$ python3 scripts/test_one_click_deploy.py
Ran 10 tests: OK

$ git diff --check
(no output, exit 0)
```

The unskipped listener probe still fails only at the sandbox boundary:

```text
TestClientProviderPlannerUsesRequestTextProvider:
httptest: failed to listen on a port: listen tcp6 [::1]:0:
bind: operation not permitted
```

The exact listener-test skip is therefore an explicit environment separation,
not a product-test waiver. The full listener-free cloud suite exited zero.

## Limitation

The sandbox cannot bind loopback listeners, so the unmodified listener-based
tests cannot execute here. This is separated from product failures by an exact
test-name skip run; no listener test was rewritten or silently treated as
passing. Strict physical exactly-once callback invocation is also impossible
without a transactional receiver boundary; the implemented and tested contract
is stable-key effectively-once application.

## Fix Round 1 — Important review findings

Commit `fcd1c4fae60c21f6c9f474a176bab8ee43fc700d` was reviewed with two
Important findings. This round corrects both without changing Workflow snapshot
or worker execution code.

### Migration identity hardening

- The upgrade now normalizes every JSON-backed row whose SQL identity is NULL,
  blank, surrounded by whitespace, legacy-prefixed, regex-invalid, or otherwise
  outside the v1 event-ID contract. A valid exact JSON `eventId` is preferred;
  otherwise the migration derives `evt_agent_terminal_legacy_<run-id>` when the
  run ID is safe and bounded, with a deterministic MD5 run-ID fallback.
- The replacement CHECK requires a non-NULL, trim-stable,
  `^evt_[A-Za-z0-9_-]+$`, at-most-96-byte identity whenever terminal JSON is
  present. `ClaimTerminalEvents` independently rejects invalid SQL identities
  and rejects a valid SQL/JSON identity conflict instead of selecting one.
- Always-run table tests cover NULL/blank/whitespace/legacy/invalid SQL,
  valid/invalid/missing JSON, safe and hashed fallback, conflicting valid IDs,
  and the 96-byte boundary. A transaction-scoped PostgreSQL fresh/upgrade
  harness executes the actual migration and strict constraint when
  `AGENT_TERMINAL_TEST_DATABASE_URL` (or `LOCALRUNNER_TEST_DATABASE_URL`) is
  available. This sandbox has no PostgreSQL server/DSN, so that one test was
  observed as an explicit SKIP; required-migration failure propagation remains
  always-run and PASS.

### Opaque prepared-event capability

- Raw `observability.Event` is no longer accepted by the durable replay API.
  `PrepareDurableEvent` returns the sealed `PreparedEvent` capability and
  `ReplayPreparedAndWait` accepts only that capability. A negative compilation
  fixture proves an external raw Event does not implement the private capability
  method.
- Durable persistence stores canonical versioned envelope bytes containing the
  frozen trusted owner and fully prepared/redacted Event plus a SHA-256
  corruption digest. The only decoder rejects unknown/trailing/noncanonical
  JSON, version/digest changes, invalid registry contracts, secret material,
  incomplete redaction, and trusted event/owner/component/source/correlation/
  runtime/privacy/status/error/time binding mismatches.
- Agent terminal JSON now stores those stable prepared bytes. A process restart
  decodes and replays the first process's source, runtime, timestamps, sequence,
  owner, and component; it cannot re-prepare with the second process's defaults.
  Invalid persisted bytes fail closed before callback side effects.
- Tests cover byte-stable encode/decode/re-encode, frozen owner replay despite a
  hostile replay context, redaction and secret rejection, owner/event-ID/runtime/
  source/component/privacy tampering, external binding mismatches, and restart
  replay under changed source/runtime defaults.
- A new crash-window test commits the sink write, loses the observability phase
  mark/claim, then retries. It observes two byte-identical physical sink calls,
  one effective Event-ID application, one callback application, and final ack.

### Fix Round 1 TDD and verification evidence

Initial RED was observed before production changes:

```text
internal/core/observability/prepared_event_test.go: undefined:
  PreparedDurableEventEmitter, MarshalPreparedEvent,
  DecodePreparedEvent, PreparedEventBinding
internal/core/database/video_project_revision_test.go: undefined:
  normalizeAgentTerminalEventID, ValidAgentTerminalEventID
```

Focused GREEN and race:

```text
$ go test -race ./internal/core/agentruntime -run 'Test.*Terminal.*|TestRepositoryClaimTerminalEvents.*' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.792s

$ go test -race ./internal/core/observability -run 'TestPrepared|TestDecodePrepared|TestRawEventCannotCompile' -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 7.379s

$ go test ./internal/core/database -run 'Test(AgentTerminal|NormalizeAgent|ValidAgent|EnsureAgent)' -count=1 -v
all always-run cases PASS; TestAgentTerminalOutboxMigrationFreshAndUpgradePostgres SKIP (no DSN)
```

Package and repository gates:

```text
$ go test ./internal/core/agentruntime -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 0.402s

$ go test ./internal/core/database ./internal/core/observability ./internal/agents/video/service -count=1
all PASS

$ npm run test:observability-foundation
contract and every package PASS except the unchanged agentruntime listener test:
httptest.NewServer: listen tcp6 [::1]:0: operation not permitted

$ npm run test:observability-contract && <foundation packages with the one exact listener test skipped>
all PASS

$ go test ./... -count=1
all reached packages PASS except the same two packages stopped by their first
unchanged httptest listener bind (agentruntime and worker/tool/builtin)

$ go test ./... -skip '<exact 13 httptest.NewServer test names>' -count=1
all cloud packages PASS

$ go vet ./...
(no output, exit 0)

$ git diff --check
(no output, exit 0)
```
