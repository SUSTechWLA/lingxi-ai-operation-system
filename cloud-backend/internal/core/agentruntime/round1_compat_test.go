package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

const round1TestSealingKey = "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXpBQkNERUY="

func loadRound1TerminalFixture(t *testing.T) RunTerminalEvent {
	t.Helper()
	payload, err := os.ReadFile("testdata/round1_terminal_event.json")
	if err != nil {
		t.Fatal(err)
	}
	var event RunTerminalEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode literal Round-1 terminal fixture: %v", err)
	}
	if len(event.legacyPreparedObservability) == 0 || event.PreparedObservability != nil {
		t.Fatal("literal Round-1 fixture was not retained as pending legacy data")
	}
	return event
}

func TestRound1TerminalJSONDecodeAndRemarshalDoesNotClearPendingPreparedEvent(t *testing.T) {
	event := loadRound1TerminalFixture(t)
	originalLegacy := append([]byte(nil), event.legacyPreparedObservability...)
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunTerminalEvent
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.legacyPreparedObservability, originalLegacy) || decoded.PreparedObservability != nil {
		t.Fatal("database JSON decode/remarshal cleared or converted a pending Round-1 event")
	}
}

func TestRunnerMigratesRound1OnlyAfterClaimCASAndSurvivesRestart(t *testing.T) {
	store := newMemoryRunStore()
	event := loadRound1TerminalFixture(t)
	store.terminal[event.RunID] = event
	store.terminalEventID[event.RunID] = event.EventID
	store.terminalLeaseUntil[event.RunID] = event.OccurredAt

	firstEmitter, firstPersistent := newRound1PersistentEmitter(t, round1TestSealingKey, nil)
	callbackCount := 0
	firstRunner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(firstPersistent).
		WithTerminalCallback(func(_ context.Context, callbackEvent RunTerminalEvent) error {
			callbackCount++
			stored := store.terminal[event.RunID]
			if stored.PreparedObservability == nil || len(stored.legacyPreparedObservability) != 0 ||
				callbackEvent.PreparedObservability == nil {
				t.Fatal("callback ran before the claimed Round-1 replacement was durably frozen")
			}
			return errors.New("simulated crash after migration CAS")
		})
	if err := firstRunner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err == nil {
		t.Fatal("simulated callback crash unexpectedly succeeded")
	}
	if err := firstEmitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stored := store.terminal[event.RunID]; stored.PreparedObservability == nil || len(stored.legacyPreparedObservability) != 0 {
		t.Fatal("Round-1 replacement was not retained for retry")
	}

	wrongEmitter, wrongPersistent := newRound1PersistentEmitter(
		t, "base64:RkVEQ0JBWnl4d3Z1dHNycXBvbm1sa2ppaGdmZWRjYmE=", nil,
	)
	wrongRunner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(wrongPersistent).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		})
	if err := wrongRunner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("wrong-key restart error = %v", err)
	}
	if err := wrongEmitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 1 {
		t.Fatalf("wrong-key restart reached callback, count=%d", callbackCount)
	}

	sink := &terminalReplaySink{}
	restartedEmitter, restartedPersistent := newRound1PersistentEmitter(t, round1TestSealingKey, sink)
	restartedRunner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(restartedPersistent).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		})
	if err := restartedRunner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := restartedEmitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if callbackCount != 2 || !store.terminalDelivered(event.RunID) {
		t.Fatalf("same-key restart callback count=%d delivered=%v", callbackCount, store.terminalDelivered(event.RunID))
	}
	payloads, owners := sink.snapshot(t)
	if len(payloads) != 1 || len(owners) != 1 || owners[0] != event.UserID {
		t.Fatalf("migrated sink payloads=%d owners=%v", len(payloads), owners)
	}
}

func TestRunnerRejectsRound1DigestOwnerAndSQLIdentityConflictsBeforeCallback(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RunTerminalEvent, *memoryRunStore)
		want   string
	}{
		{name: "digest", want: "digest mismatch", mutate: func(event *RunTerminalEvent, _ *memoryRunStore) {
			marker := []byte(`"sha256":"`)
			position := bytes.Index(event.legacyPreparedObservability, marker) + len(marker)
			event.legacyPreparedObservability[position] = '0'
		}},
		{name: "owner", want: "owner binding mismatch", mutate: func(event *RunTerminalEvent, _ *memoryRunStore) {
			event.UserID = "owner-conflict"
		}},
		{name: "terminal binding tamper", want: "terminal binding mismatch", mutate: func(event *RunTerminalEvent, _ *memoryRunStore) {
			event.TaskID = "tsk_tampered"
		}},
		{name: "sql event identity", want: "SQL identity mismatch", mutate: func(_ *RunTerminalEvent, store *memoryRunStore) {
			store.terminalEventID["agr_legacy_compat"] = "evt_agent_terminal_sql_conflict"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryRunStore()
			event := loadRound1TerminalFixture(t)
			store.terminal[event.RunID] = event
			store.terminalEventID[event.RunID] = event.EventID
			test.mutate(&event, store)
			store.terminal[event.RunID] = event
			emitter, persistent := newRound1PersistentEmitter(t, round1TestSealingKey, nil)
			callbackCount := 0
			runner := NewRunner(nil, store, nil, nil, nil).
				WithObservability(persistent).
				WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
					callbackCount++
					return nil
				})
			err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
			if callbackCount != 0 {
				t.Fatalf("invalid Round-1 event reached callback %d times", callbackCount)
			}
			if err := emitter.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type loseRound1FreezeStore struct{ *memoryRunStore }

func (store *loseRound1FreezeStore) FreezeTerminalEvent(context.Context, TerminalEventDelivery) (bool, error) {
	return false, nil
}

type failRound1FreezeStore struct{ *memoryRunStore }

func (store *failRound1FreezeStore) FreezeTerminalEvent(context.Context, TerminalEventDelivery) (bool, error) {
	return false, errors.New("database unavailable during migration CAS")
}

func TestRunnerRound1ClaimLossForbidsCallback(t *testing.T) {
	base := newMemoryRunStore()
	event := loadRound1TerminalFixture(t)
	base.terminal[event.RunID] = event
	base.terminalEventID[event.RunID] = event.EventID
	store := &loseRound1FreezeStore{memoryRunStore: base}
	emitter, persistent := newRound1PersistentEmitter(t, round1TestSealingKey, nil)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(persistent).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		})
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "claim lost") {
		t.Fatalf("claim-loss error = %v", err)
	}
	if callbackCount != 0 {
		t.Fatalf("claim-loss migration reached callback %d times", callbackCount)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerRound1PersistFailureForbidsCallback(t *testing.T) {
	base := newMemoryRunStore()
	event := loadRound1TerminalFixture(t)
	base.terminal[event.RunID] = event
	base.terminalEventID[event.RunID] = event.EventID
	store := &failRound1FreezeStore{memoryRunStore: base}
	emitter, persistent := newRound1PersistentEmitter(t, round1TestSealingKey, nil)
	callbackCount := 0
	runner := NewRunner(nil, store, nil, nil, nil).
		WithObservability(persistent).
		WithTerminalCallback(func(context.Context, RunTerminalEvent) error {
			callbackCount++
			return nil
		})
	if err := runner.DeliverPendingTerminalEventsOnce(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("persist-failure error = %v", err)
	}
	if callbackCount != 0 {
		t.Fatalf("persist-failure migration reached callback %d times", callbackCount)
	}
	if stored := base.terminal[event.RunID]; stored.PreparedObservability != nil || len(stored.legacyPreparedObservability) == 0 {
		t.Fatal("failed migration CAS changed the pending Round-1 payload")
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func newRound1PersistentEmitter(t *testing.T, key string, sink observability.Sink) (*observability.Emitter, observability.PersistentPreparedEventEmitter) {
	t.Helper()
	emitter, err := observability.NewPersistentEmitter(
		observability.Source{Service: "cloud", Component: "http-server", Environment: "test"},
		observability.Runtime{AppVersion: "current-app", GitCommit: "current-commit"}, sink, 8,
		observability.PersistentSealingConfig{Domain: "agent-terminal-test-v1", Key: key},
	)
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := emitter.ForPersistentComponent(observability.ComponentAgentRuntime)
	if err != nil {
		t.Fatal(err)
	}
	return emitter, persistent
}
