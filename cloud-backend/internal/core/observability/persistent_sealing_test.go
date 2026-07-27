package observability

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

const (
	testPersistentDomain = "cloud-agent-terminal-test-v1"
	testPersistentKey    = "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXpBQkNERUY="
)

func TestPersistentPreparedEventRestoresAcrossRestartInSameSealingDomain(t *testing.T) {
	parentSource := Source{Service: "cloud-backend", Component: "http-server", Environment: "test"}
	producerRuntime := Runtime{
		AppVersion: "app-v1", GitCommit: "commit-v1", WorkflowVersion: "workflow-v1",
		PromptTemplateVersion: "prompt-v1",
	}
	first, firstComponent := newPersistentTestComponent(t, parentSource, producerRuntime, testPersistentKey, testPersistentDomain, nil)
	event := persistentTestEvent("evt_persistent_restart")
	sealed, err := firstComponent.FreezeAndSeal(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), event,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	sink := &preparedOwnerSink{}
	restartedRuntime := Runtime{AppVersion: "app-v2", GitCommit: "commit-v2", WorkflowVersion: "workflow-v2", PromptTemplateVersion: "prompt-v2"}
	second, secondComponent := newPersistentTestComponent(t, parentSource, restartedRuntime, testPersistentKey, testPersistentDomain, sink)
	prepared, err := secondComponent.RestorePreparedEvent(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondComponent.ReplayPreparedAndWait(
		trustedcontext.WithUserID(context.Background(), "owner-attacker"), prepared,
	); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.owners) != 1 || sink.owners[0] != "owner-frozen" {
		t.Fatalf("sink owners = %v, want frozen owner", sink.owners)
	}
	if len(sink.events) != 1 {
		t.Fatalf("sink events = %d, want one", len(sink.events))
	}
	got := sink.events[0]
	if got.Source != (Source{Service: "cloud-backend", Component: "agent-runtime", Environment: "test"}) {
		t.Fatalf("restored source = %#v", got.Source)
	}
	if got.Runtime.AppVersion != "app-v1" || got.Runtime.GitCommit != "commit-v1" ||
		got.Runtime.WorkflowVersion != "workflow-v1" || got.Runtime.PromptTemplateVersion != "prompt-v1" ||
		got.Runtime.ToolRegistrySnapshotID != "snapshot-frozen" {
		t.Fatalf("restored full runtime = %#v", got.Runtime)
	}
}

func TestPersistentPreparedEventRejectsForeignSealingDomainsAndProducerIdentity(t *testing.T) {
	parentSource := Source{Service: "cloud-backend", Component: "http-server", Environment: "test"}
	producerRuntime := Runtime{AppVersion: "app-v1", GitCommit: "commit-v1", WorkflowVersion: "workflow-v1", PromptTemplateVersion: "prompt-v1"}
	original, originalComponent := newPersistentTestComponent(t, parentSource, producerRuntime, testPersistentKey, testPersistentDomain, nil)
	sealed, err := originalComponent.FreezeAndSeal(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), persistentTestEvent("evt_persistent_foreign"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close(context.Background())

	tests := []struct {
		name      string
		source    Source
		runtime   Runtime
		key       string
		domain    string
		component Component
	}{
		{name: "wrong key", source: parentSource, runtime: producerRuntime, key: "base64:RkVEQ0JBWnl4d3Z1dHNycXBvbm1sa2ppaGdmZWRjYmE=", domain: testPersistentDomain, component: ComponentAgentRuntime},
		{name: "foreign domain", source: parentSource, runtime: producerRuntime, key: testPersistentKey, domain: "foreign-terminal-domain-v1", component: ComponentAgentRuntime},
		{name: "foreign parent source", source: Source{Service: "other-service", Component: "http-server", Environment: "test"}, runtime: producerRuntime, key: testPersistentKey, domain: testPersistentDomain, component: ComponentAgentRuntime},
		{name: "foreign component adapter", source: parentSource, runtime: producerRuntime, key: testPersistentKey, domain: testPersistentDomain, component: ComponentWorker},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			emitter, err := NewPersistentEmitter(test.source, test.runtime, nil, 1, PersistentSealingConfig{
				Domain: test.domain, Key: test.key,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer emitter.Close(context.Background())
			component, err := emitter.ForPersistentComponent(test.component)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := component.RestorePreparedEvent(sealed); err == nil {
				t.Fatal("foreign persistent emitter restored sealed event")
			}
		})
	}
}

func TestPersistentPreparedEventRejectsPublicSHAForgery(t *testing.T) {
	emitter, component := newPersistentTestComponent(
		t,
		Source{Service: "cloud-backend", Component: "http-server", Environment: "test"},
		Runtime{AppVersion: "app-v1", GitCommit: "commit-v1"},
		testPersistentKey,
		testPersistentDomain,
		nil,
	)
	defer emitter.Close(context.Background())
	sealed, err := component.FreezeAndSeal(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), persistentTestEvent("evt_persistent_sha_forge"),
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(sealed.Payload)
	forged := sealed
	forged.Payload = replaceSealedMACForTest(t, sealed.Payload, hex.EncodeToString(digest[:]))
	if _, err := component.RestorePreparedEvent(forged); err == nil {
		t.Fatal("public SHA checksum forged a persistent replay capability")
	}
}

func TestPersistentPreparedEventRejectsSourceRuntimeBindingTamperBeforeReplay(t *testing.T) {
	emitter, component := newPersistentTestComponent(
		t,
		Source{Service: "cloud-backend", Component: "http-server", Environment: "test"},
		Runtime{AppVersion: "app-v1", GitCommit: "commit-v1", WorkflowVersion: "workflow-v1", PromptTemplateVersion: "prompt-v1"},
		testPersistentKey,
		testPersistentDomain,
		nil,
	)
	defer emitter.Close(context.Background())
	sealed, err := component.FreezeAndSeal(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), persistentTestEvent("evt_persistent_binding_tamper"),
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*SealedPreparedEvent)
	}{
		{name: "source service", mutate: func(value *SealedPreparedEvent) { value.Binding.Source.Service = "other-service" }},
		{name: "source environment", mutate: func(value *SealedPreparedEvent) { value.Binding.Source.Environment = "production" }},
		{name: "runtime app", mutate: func(value *SealedPreparedEvent) { value.Binding.Runtime.AppVersion = "app-v2" }},
		{name: "runtime git", mutate: func(value *SealedPreparedEvent) { value.Binding.Runtime.GitCommit = "commit-v2" }},
		{name: "runtime workflow", mutate: func(value *SealedPreparedEvent) { value.Binding.Runtime.WorkflowVersion = "workflow-v2" }},
		{name: "runtime prompt", mutate: func(value *SealedPreparedEvent) { value.Binding.Runtime.PromptTemplateVersion = "prompt-v2" }},
		{name: "runtime tool snapshot", mutate: func(value *SealedPreparedEvent) { value.Binding.Runtime.ToolRegistrySnapshotID = "snapshot-other" }},
		{name: "schema", mutate: func(value *SealedPreparedEvent) { value.Binding.SchemaVersion = "2.0" }},
		{name: "privacy", mutate: func(value *SealedPreparedEvent) { value.Binding.Privacy.Classification = PrivacyPublic }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := sealed.Clone()
			test.mutate(&tampered)
			if _, err := component.RestorePreparedEvent(tampered); err == nil {
				t.Fatal("tampered trusted binding restored")
			}
		})
	}
}

func TestPersistentPreparedEventRejectsMissingKeyTypedNilAndPayloadBounds(t *testing.T) {
	if _, err := NewPersistentEmitter(testSource(), Runtime{}, nil, 1, PersistentSealingConfig{
		Domain: testPersistentDomain,
	}); err == nil {
		t.Fatal("missing persistent sealing key accepted")
	}
	emitter, component := newPersistentTestComponent(t, testSource(), Runtime{}, testPersistentKey, testPersistentDomain, nil)
	defer emitter.Close(context.Background())
	sealed, err := component.FreezeAndSeal(
		trustedcontext.WithUserID(context.Background(), "owner-frozen"), persistentTestEvent("evt_persistent_bounds"),
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]SealedPreparedEvent{
		"empty":     {Binding: sealed.Binding},
		"oversized": {Payload: bytes.Repeat([]byte("x"), maxSealedPreparedEventBytes+1), Binding: sealed.Binding},
		"version":   {Payload: bytes.Replace(sealed.Payload, []byte(persistentPreparedEventVersion), []byte("observability.prepared.v9"), 1), Binding: sealed.Binding},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := component.RestorePreparedEvent(candidate); err == nil {
				t.Fatal("invalid sealed payload restored")
			}
		})
	}
	var nilPrepared *preparedEvent
	if err := component.ReplayPreparedAndWait(context.Background(), nilPrepared); err == nil {
		t.Fatal("typed-nil PreparedEvent replayed")
	}
}

func newPersistentTestComponent(
	t *testing.T,
	source Source,
	runtime Runtime,
	key string,
	domain string,
	sink Sink,
) (*Emitter, PersistentPreparedEventEmitter) {
	t.Helper()
	emitter, err := NewPersistentEmitter(source, runtime, sink, 4, PersistentSealingConfig{
		Domain: domain, Key: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	component, err := emitter.ForPersistentComponent(ComponentAgentRuntime)
	if err != nil {
		emitter.Close(context.Background())
		t.Fatal(err)
	}
	return emitter, component
}

func persistentTestEvent(eventID string) Event {
	event := validEvent()
	event.EventID = eventID
	event.Source = Source{}
	event.Runtime = Runtime{ToolRegistrySnapshotID: "snapshot-frozen"}
	return event
}

func replaceSealedMACForTest(t *testing.T, payload []byte, replacement string) []byte {
	t.Helper()
	marker := []byte(`"hmacSha256":"`)
	start := bytes.Index(payload, marker)
	if start < 0 {
		t.Fatalf("sealed payload lacks HMAC: %s", payload)
	}
	start += len(marker)
	end := bytes.IndexByte(payload[start:], '"')
	if end < 0 {
		t.Fatalf("sealed payload has unterminated HMAC: %s", payload)
	}
	result := append([]byte(nil), payload...)
	return append(append(result[:start], replacement...), result[start+end:]...)
}
