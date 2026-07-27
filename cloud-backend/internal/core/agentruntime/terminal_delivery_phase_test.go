package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type terminalReplayEmitter struct {
	mu         sync.Mutex
	failures   int
	payloads   [][]byte
	ownerIDs   []string
	emitter    *observability.Emitter
	persistent observability.PersistentPreparedEventEmitter
}

type nonPreparedTerminalEmitter struct {
	applications int
}

func (e *nonPreparedTerminalEmitter) Emit(context.Context, observability.Event) error {
	e.applications++
	return nil
}

func newTerminalReplayEmitter(t *testing.T, failures int) *terminalReplayEmitter {
	t.Helper()
	replay := &terminalReplayEmitter{failures: failures}
	emitter, err := observability.NewPersistentEmitter(
		observability.Source{Service: "cloud", Component: "http-server", Environment: "test"},
		observability.Runtime{AppVersion: "test-app", GitCommit: "test-commit"}, replay, 8,
		observability.PersistentSealingConfig{
			Domain: "agent-terminal-test-v1",
			Key:    []byte("0123456789abcdef0123456789abcdef-extra-test-key"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := emitter.ForPersistentComponent(observability.ComponentAgentRuntime)
	if err != nil {
		t.Fatal(err)
	}
	replay.emitter = emitter
	replay.persistent = persistent
	t.Cleanup(func() { _ = emitter.Close(context.Background()) })
	return replay
}

func newPersistentAgentEmitterForTest(
	t *testing.T,
	source observability.Source,
	runtime observability.Runtime,
	sink observability.Sink,
	capacity int,
) (*observability.Emitter, observability.PersistentPreparedEventEmitter) {
	t.Helper()
	emitter, err := observability.NewPersistentEmitter(source, runtime, sink, capacity, observability.PersistentSealingConfig{
		Domain: "agent-terminal-test-v1",
		Key:    []byte("0123456789abcdef0123456789abcdef-extra-test-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := emitter.ForPersistentComponent(observability.ComponentAgentRuntime)
	if err != nil {
		emitter.Close(context.Background())
		t.Fatal(err)
	}
	return emitter, persistent
}

func (e *terminalReplayEmitter) Write(ctx context.Context, event observability.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ownerID, _ := trustedcontext.UserID(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.payloads = append(e.payloads, payload)
	e.ownerIDs = append(e.ownerIDs, ownerID)
	if e.failures > 0 {
		e.failures--
		return errors.New("observability sink unavailable")
	}
	return nil
}

func (*terminalReplayEmitter) Close(context.Context) error { return nil }

func (e *terminalReplayEmitter) Emit(ctx context.Context, event observability.Event) error {
	return e.persistent.Emit(ctx, event)
}

func (e *terminalReplayEmitter) EmitAndWait(ctx context.Context, event observability.Event) error {
	return e.persistent.EmitAndWait(ctx, event)
}

func (e *terminalReplayEmitter) ValidatePersistentConfiguration() error {
	return e.persistent.ValidatePersistentConfiguration()
}

func (e *terminalReplayEmitter) FreezeAndSeal(ctx context.Context, event observability.Event) (observability.SealedPreparedEvent, error) {
	return e.persistent.FreezeAndSeal(ctx, event)
}

func (e *terminalReplayEmitter) RestorePreparedEvent(sealed observability.SealedPreparedEvent) (observability.PreparedEvent, error) {
	return e.persistent.RestorePreparedEvent(sealed)
}

func (e *terminalReplayEmitter) RestorePreparedEventFor(ctx context.Context, sealed observability.SealedPreparedEvent, event observability.Event) (observability.PreparedEvent, error) {
	return e.persistent.RestorePreparedEventFor(ctx, sealed, event)
}

func (e *terminalReplayEmitter) ReplayPreparedAndWait(ctx context.Context, prepared observability.PreparedEvent) error {
	return e.persistent.ReplayPreparedAndWait(ctx, prepared)
}

func (e *terminalReplayEmitter) snapshot() ([][]byte, []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	payloads := make([][]byte, len(e.payloads))
	for index := range e.payloads {
		payloads[index] = append([]byte(nil), e.payloads[index]...)
	}
	return payloads, append([]string(nil), e.ownerIDs...)
}

type lostTerminalAckStore struct {
	*memoryRunStore
	loseNextAck bool
}

type lostTerminalObservabilityMarkStore struct {
	*memoryRunStore
	loseNextMark bool
}

func (s *lostTerminalObservabilityMarkStore) MarkTerminalObservabilityDelivered(
	ctx context.Context,
	delivery TerminalEventDelivery,
) (bool, error) {
	if !s.loseNextMark {
		return s.memoryRunStore.MarkTerminalObservabilityDelivered(ctx, delivery)
	}
	s.loseNextMark = false
	_, _ = s.memoryRunStore.ReleaseTerminalEvent(ctx, delivery)
	return false, nil
}

type idempotentTerminalReplaySink struct {
	mu          sync.Mutex
	invocations [][]byte
	applied     map[string]struct{}
}

func (s *idempotentTerminalReplaySink) Write(_ context.Context, event observability.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invocations = append(s.invocations, payload)
	if s.applied == nil {
		s.applied = make(map[string]struct{})
	}
	s.applied[event.EventID] = struct{}{}
	return nil
}

func (*idempotentTerminalReplaySink) Close(context.Context) error { return nil }

type terminalReplaySink struct {
	mu       sync.Mutex
	failures int
	events   []observability.Event
	owners   []string
}

func (s *terminalReplaySink) Write(ctx context.Context, event observability.Event) error {
	ownerID, _ := trustedcontext.UserID(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	s.owners = append(s.owners, ownerID)
	if s.failures > 0 {
		s.failures--
		return errors.New("repository sink unavailable")
	}
	return nil
}

func (*terminalReplaySink) Close(context.Context) error { return nil }

func (s *terminalReplaySink) snapshot(t *testing.T) ([][]byte, []string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	payloads := make([][]byte, len(s.events))
	for index, event := range s.events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		payloads[index] = payload
	}
	return payloads, append([]string(nil), s.owners...)
}

func (s *lostTerminalAckStore) AckTerminalEvent(ctx context.Context, delivery TerminalEventDelivery) (bool, error) {
	if !s.loseNextAck {
		return s.memoryRunStore.AckTerminalEvent(ctx, delivery)
	}
	s.loseNextAck = false
	_, _ = s.memoryRunStore.ReleaseTerminalEvent(ctx, delivery)
	return false, nil
}

func TestRunnerCallbackSuccessThenObservabilityFailureDoesNotRepeatCallback(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_callback")
	emitter := newTerminalReplayEmitter(t, 1)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(emitter)

	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err == nil {
		t.Fatal("first delivery succeeded despite observability failure")
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 1 {
		t.Fatalf("callback count = %d, want one durable callback phase", callbackCount)
	}
}

func TestRunnerRejectsNonPreparedEmitterBeforeTerminalCallback(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_non_prepared_config")
	emitter := &nonPreparedTerminalEmitter{}
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(emitter)

	if err := runner.ValidateConfiguration(); err == nil || !strings.Contains(err.Error(), "persistent prepared observability") {
		t.Fatalf("configuration error = %v", err)
	}
	err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, ""))
	if err == nil || !strings.Contains(err.Error(), "persistent prepared observability") {
		t.Fatalf("terminal delivery error = %v", err)
	}
	if callbackCount != 0 || emitter.applications != 0 {
		t.Fatalf("callback=%d emitter=%d, want configuration failure before side effects", callbackCount, emitter.applications)
	}
}

func TestRunnerRejectsTypedNilPersistentEmitterBeforeTerminalCallback(t *testing.T) {
	var emitter *terminalReplayEmitter
	callbackCount := 0
	runner := NewRunner(nil, newMemoryRunStore(), nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(emitter)

	err := runner.ValidateConfiguration()
	if err == nil || !strings.Contains(err.Error(), "persistent prepared observability") {
		t.Fatalf("typed-nil configuration error = %v", err)
	}
	if callbackCount != 0 {
		t.Fatalf("callback count = %d, want zero", callbackCount)
	}
}

func TestRunnerTerminalReplayUsesByteIdenticalFrozenObservabilityEnvelope(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_replay")
	emitter := newTerminalReplayEmitter(t, 1)
	runner := NewRunner(nil, store, nil, nil, nil).WithObservability(emitter)

	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err == nil {
		t.Fatal("first delivery succeeded despite observability failure")
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	payloads, owners := emitter.snapshot()
	if len(payloads) != 2 {
		t.Fatalf("observability attempts = %d, want 2", len(payloads))
	}
	if !bytes.Equal(payloads[0], payloads[1]) {
		t.Fatalf("terminal observability replay changed\nfirst:  %s\nsecond: %s", payloads[0], payloads[1])
	}
	if len(owners) != 2 || owners[0] != run.UserID || owners[1] != run.UserID {
		t.Fatalf("replay owners = %v, want frozen owner %q", owners, run.UserID)
	}
}

func TestRunnerTerminalReplayThroughEmitterKeepsPreparedEnvelopeByteIdentical(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_prepared_replay")
	sink := &terminalReplaySink{failures: 1}
	emitter, persistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "wrong-default", Environment: "test"},
		observability.Runtime{AppVersion: "app-frozen", GitCommit: "commit-frozen"},
		sink,
		8,
	)
	runner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(persistent)

	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err == nil {
		t.Fatal("first delivery succeeded despite repository sink failure")
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err == nil {
		t.Fatal("emitter close did not report the injected repository failure")
	}

	payloads, owners := sink.snapshot(t)
	if len(payloads) != 2 {
		t.Fatalf("repository attempts = %d, want 2", len(payloads))
	}
	if !bytes.Equal(payloads[0], payloads[1]) {
		t.Fatalf("prepared terminal observability replay changed\nfirst:  %s\nsecond: %s", payloads[0], payloads[1])
	}
	if len(owners) != 2 || owners[0] != run.UserID || owners[1] != run.UserID {
		t.Fatalf("prepared replay owners = %v, want %q twice", owners, run.UserID)
	}
}

func TestRunnerTamperedPreparedEnvelopeFailsClosedBeforeCallback(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_tampered_prepared")
	sink := &terminalReplaySink{}
	emitter, persistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "agent-runtime", Environment: "test"},
		observability.Runtime{AppVersion: "app-frozen", GitCommit: "commit-frozen"}, sink, 4,
	)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(persistent)

	event := terminalEventFromRun(run, "")
	event.EventID = "evt_agent_terminal_tampered_prepared"
	event.CallbackIdempotencyKey = event.EventID
	event.RunID, event.TaskID, event.UserID = run.ID, run.TaskID, run.UserID
	event.TraceID, event.ToolRegistrySnapshotID = run.TraceID, run.ToolRegistrySnapshotID
	event.Status, event.OccurredAt = run.Status, run.UpdatedAt
	prepared, err := runner.freezeTerminalObservability(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	tampered := prepared.Clone()
	tampered.Payload = bytes.Replace(prepared.Payload, []byte(`"ownerUserId":"owner-phase"`), []byte(`"ownerUserId":"owner-other"`), 1)
	if bytes.Equal(prepared.Payload, tampered.Payload) {
		t.Fatal("test did not alter the prepared owner")
	}
	event.PreparedObservability = &tampered
	if err := store.SaveRunTerminal(context.Background(), run, event); err != nil {
		t.Fatal(err)
	}

	err = runner.DeliverPendingTerminalEventsOnce(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("tampered delivery error = %v, want signature mismatch", err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	payloads, _ := sink.snapshot(t)
	if callbackCount != 0 || len(payloads) != 0 || store.terminalDelivered(run.ID) {
		t.Fatalf("callback=%d sink=%d delivered=%v, want fail closed before side effects", callbackCount, len(payloads), store.terminalDelivered(run.ID))
	}
}

func TestRunnerRejectsValidPreparedEnvelopeCopiedFromAnotherOutboxRowBeforeCallback(t *testing.T) {
	store := newMemoryRunStore()
	sink := &terminalReplaySink{}
	emitter, persistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "agent-runtime", Environment: "test"},
		observability.Runtime{AppVersion: "app-frozen", GitCommit: "commit-frozen"}, sink, 4,
	)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(persistent)

	firstRun := terminalDeliveryTestRun("agr_phase_source_row")
	firstEvent := terminalEventFromRun(firstRun, "")
	firstEvent.EventID = "evt_agent_terminal_source_row"
	firstEvent.CallbackIdempotencyKey = firstEvent.EventID
	firstEvent.RunID, firstEvent.TaskID, firstEvent.UserID = firstRun.ID, firstRun.TaskID, firstRun.UserID
	firstEvent.TraceID, firstEvent.ToolRegistrySnapshotID = firstRun.TraceID, firstRun.ToolRegistrySnapshotID
	firstEvent.Status, firstEvent.OccurredAt = firstRun.Status, firstRun.UpdatedAt
	sealed, err := runner.freezeTerminalObservability(context.Background(), firstEvent)
	if err != nil {
		t.Fatal(err)
	}

	secondRun := terminalDeliveryTestRun("agr_phase_target_row")
	secondEvent := terminalEventFromRun(secondRun, "")
	secondEvent.EventID = "evt_agent_terminal_target_row"
	secondEvent.CallbackIdempotencyKey = secondEvent.EventID
	secondEvent.RunID, secondEvent.TaskID, secondEvent.UserID = secondRun.ID, secondRun.TaskID, secondRun.UserID
	secondEvent.TraceID, secondEvent.ToolRegistrySnapshotID = secondRun.TraceID, secondRun.ToolRegistrySnapshotID
	secondEvent.Status, secondEvent.OccurredAt = secondRun.Status, secondRun.UpdatedAt
	secondEvent.PreparedObservability = sealed
	if err := store.SaveRunTerminal(context.Background(), secondRun, secondEvent); err != nil {
		t.Fatal(err)
	}

	err = runner.DeliverPendingTerminalEventsOnce(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "caller binding mismatch") {
		t.Fatalf("cross-row prepared delivery error = %v", err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	payloads, _ := sink.snapshot(t)
	if callbackCount != 0 || len(payloads) != 0 || store.terminalDelivered(secondRun.ID) {
		t.Fatalf("callback=%d sink=%d delivered=%v, want fail closed before side effects", callbackCount, len(payloads), store.terminalDelivered(secondRun.ID))
	}
}

func TestRunnerLegacyTerminalRowFreezesSQLIdentityAcrossProcessRestart(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_legacy_restart")
	const legacySQLID = "evt_agent_terminal_legacy_agr_phase_legacy_restart"
	store.runs[run.ID] = run
	store.terminal[run.ID] = RunTerminalEvent{
		RunID: run.ID, TaskID: run.TaskID, UserID: run.UserID, TraceID: run.TraceID,
		ToolRegistrySnapshotID: run.ToolRegistrySnapshotID, Status: run.Status,
		Context: map[string]interface{}{"projectId": "prj_phase"}, OccurredAt: run.UpdatedAt,
	}
	store.terminalEventID[run.ID] = legacySQLID

	firstSink := &terminalReplaySink{failures: 1}
	firstEmitter, firstPersistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "agent-runtime", Environment: "first-process"},
		observability.Runtime{AppVersion: "app-first", GitCommit: "commit-first"},
		firstSink,
		8,
	)
	callbackCount := 0
	callbackKey := ""
	firstRunner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(_ context.Context, event RunTerminalEvent) error {
			callbackCount++
			callbackKey = event.CallbackIdempotencyKey
			return nil
		}).
		WithObservability(firstPersistent)
	if err := firstRunner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err == nil {
		t.Fatal("first process succeeded despite repository sink failure")
	}
	if err := firstEmitter.Close(context.Background()); err == nil {
		t.Fatal("first emitter close did not report the injected failure")
	}

	secondSink := &terminalReplaySink{}
	secondEmitter, secondPersistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "agent-runtime", Environment: "first-process"},
		observability.Runtime{AppVersion: "app-first", GitCommit: "commit-first"},
		secondSink,
		8,
	)
	secondRunner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(secondPersistent)
	if err := secondRunner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := secondEmitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	firstPayloads, firstOwners := firstSink.snapshot(t)
	secondPayloads, secondOwners := secondSink.snapshot(t)
	if len(firstPayloads) != 1 || len(secondPayloads) != 1 || !bytes.Equal(firstPayloads[0], secondPayloads[0]) {
		t.Fatalf("legacy replay changed across restart\nfirst:  %q\nsecond: %q", firstPayloads, secondPayloads)
	}
	if callbackCount != 1 || callbackKey != legacySQLID {
		t.Fatalf("callback count=%d key=%q, want one application keyed by %q", callbackCount, callbackKey, legacySQLID)
	}
	if len(firstOwners) != 1 || len(secondOwners) != 1 || firstOwners[0] != run.UserID || secondOwners[0] != run.UserID {
		t.Fatalf("legacy owners first=%v second=%v", firstOwners, secondOwners)
	}
	if !store.terminalDelivered(run.ID) {
		t.Fatal("legacy terminal row was not acknowledged after restart")
	}
}

func TestRunnerAckFalseIsClaimLoss(t *testing.T) {
	store := &lostTerminalAckStore{memoryRunStore: newMemoryRunStore(), loseNextAck: true}
	run := terminalDeliveryTestRun("agr_phase_ack")
	runner := NewRunner(nil, store, nil, nil, nil).WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
		return nil
	}).WithObservability(newTerminalReplayEmitter(t, 0))

	err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, ""))
	if err == nil || !strings.Contains(err.Error(), "terminal event claim lost") {
		t.Fatalf("AckTerminalEvent(false) error = %v, want terminal event claim lost", err)
	}
}

func TestRunnerSinkSuccessThenObservabilityPhaseMarkLossReplaysOneEffectiveEvent(t *testing.T) {
	store := &lostTerminalObservabilityMarkStore{memoryRunStore: newMemoryRunStore(), loseNextMark: true}
	run := terminalDeliveryTestRun("agr_phase_observability_mark_loss")
	sink := &idempotentTerminalReplaySink{}
	emitter, persistent := newPersistentAgentEmitterForTest(t,
		observability.Source{Service: "cloud", Component: "agent-runtime", Environment: "test"},
		observability.Runtime{AppVersion: "frozen-app", GitCommit: "frozen-commit"}, sink, 4,
	)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(persistent)

	err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, ""))
	if err == nil || !strings.Contains(err.Error(), "claim lost during mark observability delivered") {
		t.Fatalf("first delivery error = %v, want observability phase claim loss", err)
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if callbackCount != 1 {
		t.Fatalf("callback count = %d, want completed callback phase not replayed", callbackCount)
	}
	if len(sink.invocations) != 2 || len(sink.applied) != 1 {
		t.Fatalf("sink invocations=%d effective applications=%d, want 2/1", len(sink.invocations), len(sink.applied))
	}
	if !bytes.Equal(sink.invocations[0], sink.invocations[1]) {
		t.Fatalf("phase-mark replay changed prepared event\nfirst:  %s\nsecond: %s", sink.invocations[0], sink.invocations[1])
	}
	if !store.terminalDelivered(run.ID) {
		t.Fatal("terminal event remained pending after phase-mark recovery")
	}
}

func TestRunnerEmitSuccessThenAckLossDoesNotRepeatCompletedPhases(t *testing.T) {
	store := &lostTerminalAckStore{memoryRunStore: newMemoryRunStore(), loseNextAck: true}
	run := terminalDeliveryTestRun("agr_phase_ack_replay")
	emitter := newTerminalReplayEmitter(t, 0)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		}).
		WithObservability(emitter)

	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err == nil || !strings.Contains(err.Error(), "terminal event claim lost") {
		t.Fatalf("first delivery error=%v", err)
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	payloads, _ := emitter.snapshot()
	if callbackCount != 1 || len(payloads) != 1 {
		t.Fatalf("callback applications=%d observability applications=%d, want one each", callbackCount, len(payloads))
	}
	if !store.terminalDelivered(run.ID) {
		t.Fatal("terminal row remained pending after acknowledgement retry")
	}
}

func TestRunnerConcurrentTerminalReconcilersClaimOneDelivery(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_concurrent")
	emitter := newTerminalReplayEmitter(t, 0)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var callbackCount atomic.Int32
	runner := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount.Add(1)
			entered <- struct{}{}
			<-release
			return nil
		}).
		WithObservability(emitter)
	enqueueTerminalDeliveryForTest(t, runner, store, run, "evt_agent_terminal_concurrent")

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- runner.DeliverPendingTerminalEventsOnce(context.Background(), 1)
	}()
	<-entered
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- runner.DeliverPendingTerminalEventsOnce(context.Background(), 1)
	}()
	if err := <-secondResult; err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
	payloads, _ := emitter.snapshot()
	if callbackCount.Load() != 1 || len(payloads) != 1 || !store.terminalDelivered(run.ID) {
		t.Fatalf("callbacks=%d events=%d delivered=%v", callbackCount.Load(), len(payloads), store.terminalDelivered(run.ID))
	}
}

func TestRunnerExpiredLeaseReplaysStableCallbackIdempotencyIdentity(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_expired_lease")
	emitter := newTerminalReplayEmitter(t, 0)
	runner := NewRunner(nil, store, nil, nil, nil).WithObservability(emitter)
	const eventID = "evt_agent_terminal_expired_lease"
	enqueueTerminalDeliveryForTest(t, runner, store, run, eventID)

	crashedClaim, err := store.ClaimTerminalEvents(
		context.Background(), 1, time.Now().Add(-time.Second), "claim-before-crash",
	)
	if err != nil || len(crashedClaim) != 1 {
		t.Fatalf("crashed claim=%#v error=%v", crashedClaim, err)
	}
	applications := map[string]struct{}{}
	callbackInvocations := 0
	apply := func(event RunTerminalEvent) {
		callbackInvocations++
		applications[event.CallbackIdempotencyKey] = struct{}{}
	}
	// The receiver committed its side effect, then the process crashed before
	// the callback phase could be marked. A later lease holder must receive the
	// same explicit identity so the receiver can collapse the replay.
	apply(crashedClaim[0].Event)
	restarted := NewRunner(nil, store, nil, nil, nil).
		WithTerminalCallback(func(_ context.Context, event RunTerminalEvent) error {
			apply(event)
			return nil
		}).
		WithObservability(emitter)
	if err := restarted.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if callbackInvocations != 2 || len(applications) != 1 {
		t.Fatalf("callback invocations=%d effective applications=%d", callbackInvocations, len(applications))
	}
	if _, ok := applications[eventID]; !ok {
		t.Fatalf("stable callback identities=%v, want %q", applications, eventID)
	}
	payloads, _ := emitter.snapshot()
	if len(payloads) != 1 || !store.terminalDelivered(run.ID) {
		t.Fatalf("observability events=%d delivered=%v", len(payloads), store.terminalDelivered(run.ID))
	}
}

func TestRunnerTerminalCallbackReceivesStableExplicitIdempotencyKey(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_callback_key")
	var callbackPayload map[string]any
	runner := NewRunner(nil, store, nil, nil, nil).WithTerminalCallback(func(_ context.Context, event RunTerminalEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return json.Unmarshal(payload, &callbackPayload)
	}).WithObservability(newTerminalReplayEmitter(t, 0))

	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err != nil {
		t.Fatal(err)
	}
	eventID, _ := callbackPayload["eventId"].(string)
	idempotencyKey, _ := callbackPayload["callbackIdempotencyKey"].(string)
	if eventID == "" || idempotencyKey != eventID {
		t.Fatalf("callback identity eventId=%q idempotencyKey=%q, want the frozen event ID", eventID, idempotencyKey)
	}
}

func TestRunnerTerminalCallbackReplayUsesByteIdenticalFrozenPayload(t *testing.T) {
	store := newMemoryRunStore()
	run := terminalDeliveryTestRun("agr_phase_callback_payload")
	var payloads [][]byte
	runner := NewRunner(nil, store, nil, nil, nil).WithTerminalCallback(func(_ context.Context, event RunTerminalEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		payloads = append(payloads, payload)
		if len(payloads) == 1 {
			return errors.New("receiver unavailable after reading payload")
		}
		return nil
	}).WithObservability(newTerminalReplayEmitter(t, 0))
	if err := runner.persistAndDeliverTerminal(context.Background(), run, terminalEventFromRun(run, "")); err == nil {
		t.Fatal("first callback attempt unexpectedly succeeded")
	}
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 2 || !bytes.Equal(payloads[0], payloads[1]) {
		t.Fatalf("callback replay changed\nfirst:  %q\nsecond: %q", payloads[0], payloads[1])
	}
}

func terminalDeliveryTestRun(id string) *Run {
	now := time.Date(2026, time.July, 28, 1, 2, 3, 456000000, time.UTC)
	return &Run{
		ID: id, TaskID: "tsk_phase", UserID: "owner-phase", Status: RunStatusSuccess,
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", ToolRegistrySnapshotID: "snapshot-phase",
		Metadata:  map[string]interface{}{"requestContext": map[string]interface{}{"projectId": "prj_phase"}},
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now,
	}
}

func enqueueTerminalDeliveryForTest(
	t *testing.T,
	runner *Runner,
	store *memoryRunStore,
	run *Run,
	eventID string,
) {
	t.Helper()
	event := terminalEventFromRun(run, "")
	event.EventID = eventID
	event.CallbackIdempotencyKey = eventID
	event.TaskID = run.TaskID
	event.TraceID = run.TraceID
	event.ToolRegistrySnapshotID = run.ToolRegistrySnapshotID
	event.OccurredAt = run.UpdatedAt
	frozen, err := runner.freezeTerminalObservability(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	event.PreparedObservability = frozen
	if err := store.SaveRunTerminal(context.Background(), run, event); err != nil {
		t.Fatal(err)
	}
}
