# Plan 1 final-review fix report

Date: 2026-07-27

Base: `1c191ff85329fa8d8ed7f920cd5db7e625a86e53`

Scope: the four Important findings in `final-review-fix-brief.md` only. The
Translator polling-timeout classification and product-ID versus summary-ID
mapping Minor findings remain deferred and unchanged.

## Implemented fixes

1. Request/task correlation is authoritative at the event-bus boundary.
   Initial and dependency-released node events no longer synthesize
   node-specific trace roots, while explicit event correlation remains a
   fallback when context correlation is absent.
2. Agent DAG submission now closes an `agent.bootstrap` workflow stage and
   emits `agent.run.started`, not a false `agent.run.completed`. Completed,
   failed, and cancelled Agent terminals are reconstructed from the durable
   terminal outbox, retain their original event ID and runtime snapshot, and
   are delivered once to observability before acknowledgement. The first
   durable terminal wins, including concurrent/repeated terminal attempts.
   The terminal relay now runs even when video creation is disabled.
3. WorkflowRun creation freezes and validates the canonical registry snapshot
   before task/DAG/run side effects. The real registry snapshot ID and SHA are
   carried by task input, DAG execution input, the WorkflowRun, and its
   manifest. Runtime preparation preserves event-specific fields and uses
   process-global runtime only for missing values; workflow transitions and
   relay summaries retain workflow/snapshot/prompt versions.
4. Every Translator POST, polling GET, final submission, and status GET carries
   a W3C `traceparent` with the caller trace and a distinct child span.

## RED evidence

All Go commands below used writable sandbox caches:

```text
GOCACHE=/private/tmp/tangying-go-cache
GOTMPDIR=/private/tmp/tangying-go-tmp
```

Authoritative trace and Translator propagation:

```sh
go test ./internal/core/eventbus ./internal/core/orchestrator/service ./internal/core/translator/service -run 'TestPublishWithContextTreatsPresentContextCorrelationAsAuthoritative|TestNodeReadyEventsPreserveOneAuthoritativeTraceFromInitialToChild|TestTranslatorOutboundRequestsPropagateW3CChildTraceparent' -count=1
```

Result before production changes: **FAIL as expected**.

- Event bus retained `node-specific-trace-root`.
- The initial orchestrator event retained `task-1-initial`.
- Translator POST `/api/node` had an empty `traceparent`.

True Agent terminal lifecycle:

```sh
go test ./internal/core/agentruntime -run 'TestRunnerDAGSubmissionCompletesBootstrapWithoutAgentTerminal|TestRunnerDurableTerminalOutboxEmitsExactlyOneTrueAgentTerminal|TestRunnerTerminalCallbackRetryEmitsOneIdempotentObservabilityEvent' -count=1
```

Result before production changes: **FAIL as expected**.

- DAG submission emitted a false `agent.run.completed`.
- Durable completed/failed terminal observations were absent.
- Repeated cancellation invoked terminal callbacks twice.
- Retry delivery produced no durable terminal observability event.

Runtime preservation:

```sh
go test ./internal/core/observability -run TestEmitterAndRelayPreserveEventRuntimeVersionsWhileGlobalFillsMissing -count=1
```

Result before production changes: **FAIL as expected**. The relay summary kept
only global app/git values and lost the event workflow, snapshot, and prompt
versions.

## Focused GREEN evidence

```sh
go test ./internal/core/eventbus ./internal/core/orchestrator/service -run 'TestPublishWithContextTreatsPresentContextCorrelationAsAuthoritative|TestPublishWithContextPopulatesMissingCorrelationWithoutStaleValues|TestConsumerHandlerReceivesReconstructedCorrelationContext|TestNodeReadyEventsPreserveOneAuthoritativeTraceFromInitialToChild' -count=1
```

Result: **PASS** (`eventbus 0.570s`, `orchestrator/service 0.955s`).

```sh
go test ./internal/core/agentruntime -run 'TestRunnerDAGSubmissionCompletesBootstrapWithoutAgentTerminal|TestRunnerDurableTerminalOutboxEmitsExactlyOneTrueAgentTerminal|TestRunnerTerminalCallbackRetryEmitsOneIdempotentObservabilityEvent|TestRunnerBackgroundPersistsConsistentCancellationAndTimeoutTerminals|TestRunnerStartEmitsBootstrapFailureWithoutPrivateRequestText|TestRunnerDeadlineFailsInnerAndOuterWithTimeoutCodes|TestRunnerRepairCancellationUsesCancelledEventsWithoutError|TestRunnerFirstDurableTerminalWinsOverReplacementAttempt|TestRunnerTerminalEventRetriesUntilAcknowledged' -count=1
```

Result: **PASS** (`agentruntime 0.687s`).

```sh
go test ./internal/core/workflow -run 'TestCreateRunFreezesRealToolSnapshotBeforeTaskAndUsesItForExecutionAndManifest|TestCreateRunRequiresSnapshotProviderBeforeCreatingTask|TestUpdateStageStatusEmitsSingleMappedTransition|TestWorkflowFailuresNormalizeStableCodesThroughEmitter' -count=1
go test ./internal/core/observability -run TestEmitterAndRelayPreserveEventRuntimeVersionsWhileGlobalFillsMissing -count=1
go test ./cmd/tangying-ai-os -run 'TestNewHTTPRouter|TestRunStatusSyncer' -count=1
```

Result: **PASS** (`workflow 0.582s`, `observability 0.447s`,
`cmd/tangying-ai-os 0.643s`).

```sh
go test ./internal/core/translator/service ./internal/core/observability -run 'TestTranslatorOutboundRequestsPropagateW3CChildTraceparent|TestEmitterAndRelayPreserveEventRuntimeVersionsWhileGlobalFillsMissing' -count=1
```

Result: **PASS** (`translator/service 1.056s`, `observability 0.584s`).

## Regression, race, aggregate, and full-suite evidence

Related-package regression:

```sh
go test ./internal/core/eventbus ./internal/core/orchestrator/service ./internal/core/observability ./internal/core/agentruntime ./internal/core/workflow ./internal/core/translator/... ./cmd/tangying-ai-os -count=1
```

Result: all packages except `agentruntime` passed. `agentruntime` was
environment-blocked before assertions by the pre-existing
`TestClientProviderPlannerUsesRequestTextProvider` calling
`httptest.NewServer`: `listen tcp6 [::1]:0: bind: operation not permitted`.
The same package with only that listener test skipped passed:

```sh
go test ./internal/core/agentruntime -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
```

Result: **PASS** (`agentruntime 0.386s`).

Relevant race gate:

```sh
go test -race ./internal/core/eventbus ./internal/core/orchestrator/service ./internal/core/observability ./internal/core/agentruntime ./internal/core/workflow ./internal/core/translator/service -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
```

Result: **PASS** for all six packages.

Plan 1 aggregate gate:

```sh
npm run test:observability-foundation
```

Result: the JavaScript observability contract passed and all reported Go
packages passed except `agentruntime`, which was blocked by the same sandbox
listener denial above. The equivalent aggregate Go package set with that
single listener test skipped passed:

```sh
go test ./internal/core/observability ./internal/core/agentruntime ./internal/core/workflow ./internal/core/localrunner ./internal/core/translator/... ./internal/core/worker/service -skip '^TestClientProviderPlannerUsesRequestTextProvider$' -count=1
```

Result: **PASS** for all seven packages.

Full cloud suite:

```sh
go test ./... -count=1
```

Result: all reported packages passed except `agentruntime` and
`worker/tool/builtin`. They were environment-blocked by pre-existing
`httptest.NewServer` tests with the same `bind: operation not permitted`
failure. A repository search found 13 tests that open listeners. The full
suite with exactly those 13 listener tests skipped passed:

```sh
go test ./... -skip '^(TestClientProviderPlannerUsesRequestTextProvider|TestLlmApiTool_Execute_UsesIntegerMaxTokensAndJSONMode|TestKnowledgeResearcherUsesClientModelProviderParam|TestImageAssetGeneratorUsesClientOpenAICompatibleImageProvider|TestTextImageToVideoGeneratorUsesClientOpenAICompatibleVideoProvider|TestVideoScriptGeneratorNormalizesLongScriptSpansFromModel|TestCaptionSplitterUsesLocalDeterministicOutput|TestExecuteDynamicAgentPromptToolExposesShotAssetPackages|TestExecuteDynamicAgentPromptToolSplitsShotsLocally|TestExecuteDynamicAgentPromptToolRetriesStructuredJSONOutput|TestReferenceAssetPlannerCinematicUsesDeterministicDataWithModelProvider|TestKeyframePromptGeneratorCinematicUsesDeterministicDataWithModelProvider|TestVideoScriptGeneratorCinematicBackfillsInvalidModelOutput)$' -count=1
```

Result: **PASS** for every cloud package.

Static checks:

```sh
go vet ./...
git diff --check
```

Result: **PASS**, no output.

## Environment notes

- The default macOS Go cache under `~/Library/Caches` is outside the managed
  sandbox. Initial invocation failed before compilation with a cache permission
  error, so all reported Go evidence uses the writable `/private/tmp` cache
  paths shown above.
- Listener failures are sandbox infrastructure failures, not code/test
  assertion failures. No production or test code was weakened to bypass them.

## Fix Round 3 — Round-1 compatibility and production key bootstrap

Round 2 changed `preparedObservability` from the Round-1 base64 JSON string to
an HMAC v2 object. Pending Round-1 rows therefore failed JSON decoding. The
production key path also accepted human-selected text, while one-click emitted
raw hex and did not converge existing env-file permissions.

### Controlled compatibility state machine

- `RunTerminalEvent` recognizes the historical string only as private pending
  bytes. Decode/remarshal retains it, so ordinary database JSON handling cannot
  silently clear or convert a pending event.
- Migration runs after a repository claim and exact SQL event/run/callback
  identity plus claim-token checks. The private decoder enforces the Round-1
  size, canonical JSON, v1 version and SHA checksum, registry, redaction and
  no-secret contracts, then proves trusted owner, source/component, tool
  snapshot, and the complete terminal-derived Event.
- A valid legacy Event is resealed with the configured persistent HMAC key as
  v2. The replacement is persisted through the existing run/event/claim CAS
  before callback or sink execution. Conversion/persist error or claim loss is
  fail-closed. A callback crash retries the persisted v2; same-key restart
  succeeds and wrong-key restart fails before callback.
- Tests use literal Round-1 terminal JSON with the historical base64 transport
  and canonical checksum. Coverage includes checksum/tamper/owner/SQL identity,
  claim loss, persist failure, callback ordering, crash/retry, same/wrong-key
  restart, and decode/remarshal retention.

### One production key format and secure one-click bootstrap

- Config validation and the HMAC sealer share one parser: canonical `base64:`
  decoding to 32-64 bytes. Explicit checks reject whitespace, malformed or
  noncanonical base64, short/overlong material, fewer than 16 distinct bytes,
  exact repeated character/block patterns, repeated hex text, and known
  placeholder text. Errors never echo the supplied value; no estimated entropy
  claim is made.
- Startup passes that same text to the sealer and treats either
  `GIN_MODE=release` or `APP_ENV=production|prod|release` as production. Compose
  sets both production indicators, the internal tool token, and enabled,
  non-fallback sandbox configuration.
- One-click generates 32 random key bytes in the accepted format and preserves
  a valid existing key byte-for-byte. Missing/invalid existing keys stop with
  restore-or-drain-before-rotation guidance. The script rejects symlink and
  non-regular targets, verifies current-user ownership, applies `0600` before
  secret access, verifies owner/mode after writes, and suppresses/restores
  xtrace around secret handling.
- The runbook documents generation, exact-byte legacy transport conversion,
  pending-row drain, coordinated rotation, wrong-key recovery and binary/secret
  rollback. The example placeholder remains production-invalid.

### TDD and verification evidence

Initial key parser RED:

```text
undefined: ParsePersistentSealingKey
cannot use test.value (variable of type string) as []byte value in struct literal
FAIL github.com/tangying-ai/aios-core/internal/core/observability [build failed]
```

The literal Round-1 fixture initially could not unmarshal a JSON string into a
v2 object. Pre-fix one-click behavior generated raw hex, retained an existing
`0644` mode, accepted invalid existing key text, and exposed secret assignments
under `bash -x`. These contracts were observed RED before the production fixes.

```text
$ go test -race ./internal/core/observability ./internal/core/agentruntime \
  -run 'Test(ParsePersistent|PersistentSealer|PersistentPrepared|Round1|Runner.*Round1|Runner.*Terminal|Repository.*Terminal)' -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 1.479s
ok github.com/tangying-ai/aios-core/internal/core/agentruntime 1.682s

$ go test ./internal/core/observability ./internal/core/config ./cmd/tangying-ai-os -count=1
ok github.com/tangying-ai/aios-core/internal/core/observability 13.861s
ok github.com/tangying-ai/aios-core/internal/core/config 0.872s
ok github.com/tangying-ai/aios-core/cmd/tangying-ai-os 0.535s

$ GOFLAGS='-skip=^TestClientProviderPlannerUsesRequestTextProvider$' npm run test:observability-foundation
contract plus observability, agentruntime, workflow, localrunner, translator,
and worker/service: PASS

$ go test ./... -skip '<the exact 13 pre-existing listener tests>' -count=1
all cloud packages: PASS

$ bash -n scripts/one-click-deploy.sh
(no output, exit 0)

$ python3 -m unittest scripts.test_one_click_deploy -v
Ran 13 tests: OK

$ go vet ./...
(no output, exit 0)

$ git diff --check
(no output, exit 0)
```

No Workflow snapshot or worker-execution production code changed in this round.
