package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type preparedOwnerSink struct {
	mu     sync.Mutex
	owners []string
	events []Event
}

func (s *preparedOwnerSink) Write(ctx context.Context, event Event) error {
	owner, _ := trustedcontext.UserID(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owners = append(s.owners, owner)
	s.events = append(s.events, event)
	return nil
}

func (*preparedOwnerSink) Close(context.Context) error { return nil }

func TestPreparedEventRoundTripIsStableAndReplaysFrozenOwner(t *testing.T) {
	sink := &preparedOwnerSink{}
	emitter := NewEmitter(testSource(), Runtime{GitCommit: "freeze-commit"}, sink, 2)
	component := emitter.ForComponent(ComponentAgentRuntime).(PreparedDurableEventEmitter)
	event := validEvent()
	event.EventID = "evt_prepared_roundtrip"
	event.Source = Source{}
	event.Evidence.Attributes = map[string]any{"password": "must-not-persist"}
	ctx := trustedcontext.WithUserID(context.Background(), "owner-frozen")

	prepared, err := component.PrepareDurableEvent(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	first, err := MarshalPreparedEvent(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first, []byte("must-not-persist")) {
		t.Fatalf("prepared bytes contain redacted secret: %s", first)
	}
	binding := preparedBindingForTest(event, "owner-frozen")
	decoded, err := DecodePreparedEvent(first, binding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalPreparedEvent(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("prepared round trip changed\nfirst:  %s\nsecond: %s", first, second)
	}
	wrongContext := trustedcontext.WithUserID(context.Background(), "owner-attacker")
	if err := component.ReplayPreparedAndWait(wrongContext, decoded); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.owners) != 1 || sink.owners[0] != "owner-frozen" {
		t.Fatalf("sink owners = %v, want frozen owner", sink.owners)
	}
	if len(sink.events) != 1 || sink.events[0].Source.Component != string(ComponentAgentRuntime) {
		t.Fatalf("sink events = %#v", sink.events)
	}
}

func TestDecodePreparedEventRejectsTamperedEnvelopeFields(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{GitCommit: "freeze-commit"}, nil, 1)
	defer emitter.Close(context.Background())
	component := emitter.ForComponent(ComponentAgentRuntime).(PreparedDurableEventEmitter)
	event := validEvent()
	event.EventID = "evt_prepared_tamper"
	prepared, err := component.PrepareDurableEvent(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), event,
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := MarshalPreparedEvent(prepared)
	if err != nil {
		t.Fatal(err)
	}
	binding := preparedBindingForTest(event, "owner-frozen")

	tests := []struct {
		name   string
		mutate func(*preparedEventEnvelope)
	}{
		{name: "owner", mutate: func(envelope *preparedEventEnvelope) { envelope.OwnerUserID = "owner-attacker" }},
		{name: "top level event id", mutate: func(envelope *preparedEventEnvelope) { envelope.Event.EventID = "evt_attacker" }},
		{name: "runtime", mutate: func(envelope *preparedEventEnvelope) { envelope.Event.Runtime.GitCommit = "attacker" }},
		{name: "source", mutate: func(envelope *preparedEventEnvelope) { envelope.Event.Source.Service = "attacker" }},
		{name: "component", mutate: func(envelope *preparedEventEnvelope) {
			envelope.Event.Source.Component = "worker-tool-executor"
		}},
		{name: "privacy", mutate: func(envelope *preparedEventEnvelope) { envelope.Event.Privacy.Classification = PrivacyPublic }},
		{name: "secret", mutate: func(envelope *preparedEventEnvelope) { envelope.Event.Privacy.Classification = PrivacySecret }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var envelope preparedEventEnvelope
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Fatal(err)
			}
			test.mutate(&envelope)
			tampered, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodePreparedEvent(tampered, binding); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
				t.Fatalf("tampered prepared envelope error = %v, want digest mismatch", err)
			}
		})
	}
}

func TestPrepareDurableEventRejectsSecretClassification(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, nil, 1)
	defer emitter.Close(context.Background())
	component := emitter.ForComponent(ComponentAgentRuntime).(PreparedDurableEventEmitter)
	event := validEvent()
	event.EventID = "evt_prepared_secret"
	event.Privacy.Classification = PrivacySecret
	_, err := component.PrepareDurableEvent(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), event,
	)
	if err == nil || !strings.Contains(err.Error(), "secret material") {
		t.Fatalf("secret prepared event error = %v", err)
	}
}

func TestDecodePreparedEventRejectsExternalBindingMismatches(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, nil, 1)
	defer emitter.Close(context.Background())
	component := emitter.ForComponent(ComponentAgentRuntime).(PreparedDurableEventEmitter)
	event := validEvent()
	event.EventID = "evt_prepared_binding"
	prepared, err := component.PrepareDurableEvent(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), event,
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := MarshalPreparedEvent(prepared)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*PreparedEventBinding)
	}{
		{name: "owner", mutate: func(binding *PreparedEventBinding) { binding.OwnerUserID = "owner-other" }},
		{name: "top level id", mutate: func(binding *PreparedEventBinding) { binding.EventID = "evt_other" }},
		{name: "runtime", mutate: func(binding *PreparedEventBinding) { binding.Runtime.ToolRegistrySnapshotID = "snapshot-other" }},
		{name: "component", mutate: func(binding *PreparedEventBinding) { binding.Component = ComponentWorker }},
		{name: "privacy", mutate: func(binding *PreparedEventBinding) { binding.Privacy.Classification = PrivacyPublic }},
		{name: "status", mutate: func(binding *PreparedEventBinding) { binding.ExecutionStatus = ExecutionStatusCompleted }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := preparedBindingForTest(event, "owner-frozen")
			test.mutate(&binding)
			if _, err := DecodePreparedEvent(payload, binding); err == nil {
				t.Fatal("mismatched prepared binding accepted")
			}
		})
	}
}

func TestRawEventCannotCompileAsPreparedReplayCapability(t *testing.T) {
	command := exec.Command("go", "test", "./testdata/raw_prepared_replay")
	command.Dir = "."
	command.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "go-cache"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("raw Event unexpectedly compiled as PreparedEvent: %s", output)
	}
	if !strings.Contains(string(output), "does not implement observability.PreparedEvent") {
		t.Fatalf("compile error did not enforce opaque capability:\n%s", output)
	}
}

func preparedBindingForTest(event Event, owner string) PreparedEventBinding {
	errorCode := ""
	if event.Error != nil {
		errorCode = event.Error.Code
	}
	privacy := event.Privacy
	privacy.RedactedFields = nil
	return PreparedEventBinding{
		EventID: event.EventID, OwnerUserID: owner, Component: ComponentAgentRuntime,
		Correlation: event.Correlation, Runtime: event.Runtime, Privacy: privacy,
		EventType: event.EventType, ExecutionStatus: event.Execution.Status,
		ErrorCode: errorCode, OccurredAt: event.OccurredAt,
	}
}
