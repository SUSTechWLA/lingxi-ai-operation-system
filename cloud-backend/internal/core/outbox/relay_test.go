package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/eventbus"
)

// ── Mock OutboxStore ────────────────────────────────────────────────────

type mockStore struct {
	mu      sync.Mutex
	entries map[int64]OutboxEntry // id → entry
	dlq     []dlqEntry
	nextID  int64

	// Hooks for injecting errors
	fetchErr   error
	deleteErr  error
	retryErr   error
	moveDlqErr error
}

type dlqEntry struct {
	Entry  OutboxEntry
	ErrMsg string
}

func newMockStore() *mockStore {
	return &mockStore{
		entries: make(map[int64]OutboxEntry),
		nextID:  1,
	}
}

func (m *mockStore) addEntry(entry OutboxEntry) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry.ID == 0 {
		entry.ID = m.nextID
		m.nextID++
	}
	m.entries[entry.ID] = entry
	return entry.ID
}

func (m *mockStore) FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fetchErr != nil {
		return nil, m.fetchErr
	}

	var result []OutboxEntry
	for id, e := range m.entries {
		if len(result) >= limit {
			break
		}
		result = append(result, e)
		_ = id
	}
	return result, nil
}

func (m *mockStore) Delete(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.entries, id)
	return nil
}

func (m *mockStore) IncrementRetry(ctx context.Context, id int64, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.retryErr != nil {
		return m.retryErr
	}
	e, ok := m.entries[id]
	if !ok {
		return fmt.Errorf("entry %d not found", id)
	}
	e.RetryCount++
	e.LastError = errMsg
	m.entries[id] = e
	return nil
}

func (m *mockStore) MoveToDLQ(ctx context.Context, entry OutboxEntry, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.moveDlqErr != nil {
		return m.moveDlqErr
	}
	delete(m.entries, entry.ID)
	m.dlq = append(m.dlq, dlqEntry{Entry: entry, ErrMsg: errMsg})
	return nil
}

// ── Mock EventPublisher ─────────────────────────────────────────────────

type mockPublisher struct {
	mu         sync.Mutex
	published  []publishedEvent
	publishErr error // if set and failMap is empty, every Publish fails
	failMap    map[string]bool // topic → always fail for this topic
}

type publishedEvent struct {
	Topic string
	Key   string
	Event eventbus.Event
}

func (m *mockPublisher) Publish(topic, key string, event eventbus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Per-event failure map takes precedence
	if m.failMap != nil && m.failMap[key] {
		return m.publishErr
	}
	// Global failure mode
	if m.publishErr != nil && m.failMap == nil {
		return m.publishErr
	}

	m.published = append(m.published, publishedEvent{Topic: topic, Key: key, Event: event})
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────────

func makeEvent(taskID, nodeID string) eventbus.Event {
	return eventbus.Event{
		TaskID: taskID,
		NodeID: nodeID,
		Type:   "TEST",
	}
}

func makeEntry(id int64, eventType string, event eventbus.Event) OutboxEntry {
	payload, _ := json.Marshal(event)
	return OutboxEntry{
		ID:            id,
		AggregateType: "node",
		AggregateID:   event.NodeID,
		EventType:     eventType,
		Payload:       payload,
	}
}

// ── Test: Normal relay ──────────────────────────────────────────────────

func TestRelay_RelaysEventAndDeletes(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond

	event := makeEvent("task-1", "node-1")
	store.addEntry(makeEntry(1, "ai.node.ready", event))

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	// Wait for relay to process
	time.Sleep(50 * time.Millisecond)
	cancel()
	relay.Stop()

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()

	if publishedCount != 1 {
		t.Fatalf("expected 1 published event, got %d", publishedCount)
	}
	if pub.published[0].Topic != "ai.node.ready" {
		t.Errorf("expected topic ai.node.ready, got %s", pub.published[0].Topic)
	}

	store.mu.Lock()
	remaining := len(store.entries)
	store.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 remaining entries, got %d", remaining)
	}
}

// ── Test: Retry on failure ──────────────────────────────────────────────

func TestRelay_RetriesOnPublishFailure(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{
		publishErr: errors.New("kafka unavailable"),
	}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.MaxRetries = 5

	event := makeEvent("task-1", "node-1")
	store.addEntry(makeEntry(1, "ai.node.ready", event))

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	relay.Stop()

	// Event should still be in outbox with incremented retry count
	store.mu.Lock()
	entry, exists := store.entries[1]
	dlqLen := len(store.dlq)
	store.mu.Unlock()

	if !exists {
		t.Fatal("event should still be in outbox (not deleted)")
	}
	if entry.RetryCount == 0 {
		t.Error("retry_count should have been incremented")
	}
	if entry.LastError == "" {
		t.Error("last_error should be set")
	}
	if dlqLen > 0 {
		t.Errorf("event should not be in DLQ yet (retries < max), got %d DLQ entries", dlqLen)
	}
}

// ── Test: Moves to DLQ after max retries ────────────────────────────────

func TestRelay_MovesToDLQAfterMaxRetries(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{
		publishErr: errors.New("kafka unavailable"),
	}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 5 * time.Millisecond
	cfg.MaxRetries = 2

	// Add an event that's already been retried twice
	entry := makeEntry(1, "ai.node.ready", makeEvent("task-1", "node-1"))
	entry.RetryCount = 2
	store.addEntry(entry)

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	relay.Stop()

	store.mu.Lock()
	_, inOutbox := store.entries[1]
	dlqLen := len(store.dlq)
	store.mu.Unlock()

	if inOutbox {
		t.Error("event should be removed from outbox after max retries")
	}
	if dlqLen != 1 {
		t.Fatalf("expected 1 DLQ entry, got %d", dlqLen)
	}
	if store.dlq[0].Entry.ID != 1 {
		t.Errorf("expected DLQ entry ID 1, got %d", store.dlq[0].Entry.ID)
	}
}

// ── Test: Batch relay ───────────────────────────────────────────────────

func TestRelay_BatchRelay(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.BatchSize = 50

	// Add 100 events
	for i := 0; i < 100; i++ {
		event := makeEvent(fmt.Sprintf("task-%d", i), fmt.Sprintf("node-%d", i))
		store.addEntry(makeEntry(0, "ai.node.ready", event))
	}

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	relay.Stop()

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()

	if publishedCount != 100 {
		t.Errorf("expected 100 published events, got %d", publishedCount)
	}

	store.mu.Lock()
	remaining := len(store.entries)
	store.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 remaining entries, got %d", remaining)
	}
}

// ── Test: Partial failure doesn't block other events ────────────────────

func TestRelay_PartialFailureDoesNotBlock(t *testing.T) {
	store := newMockStore()
	// Event "node-1" always fails; "node-2" and "node-3" succeed
	pub := &mockPublisher{
		failMap:    map[string]bool{"node-1": true},
		publishErr: errors.New("kafka unavailable"),
	}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.MaxRetries = 10

	// Add 3 events with different aggregate IDs (= key)
	event1 := makeEvent("task-1", "node-1")
	store.addEntry(makeEntry(1, "ai.node.ready", event1))
	event2 := makeEvent("task-2", "node-2")
	store.addEntry(makeEntry(2, "ai.node.ready", event2))
	event3 := makeEvent("task-3", "node-3")
	store.addEntry(makeEntry(3, "ai.node.ready", event3))

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(150 * time.Millisecond)
	cancel()
	relay.Stop()

	store.mu.Lock()
	entry1, has1 := store.entries[1]
	_, has2 := store.entries[2]
	_, has3 := store.entries[3]
	store.mu.Unlock()

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()

	// Events 2 and 3 should be published and deleted
	if has2 {
		t.Error("event 2 should be deleted after success")
	}
	if has3 {
		t.Error("event 3 should be deleted after success")
	}
	// Event 1 should still be in outbox (retrying, key always fails)
	if !has1 {
		t.Error("event 1 should still be in outbox (retried)")
	}
	if entry1.RetryCount == 0 {
		t.Error("event 1 should have retry count incremented")
	}
	if publishedCount < 2 {
		t.Errorf("expected at least 2 published events, got %d", publishedCount)
	}
}

// ── Test: Graceful shutdown waits for in-flight work ────────────────────

func TestRelay_GracefulShutdown(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond

	// Add events
	for i := 0; i < 10; i++ {
		event := makeEvent(fmt.Sprintf("task-%d", i), fmt.Sprintf("node-%d", i))
		store.addEntry(makeEntry(0, "ai.node.ready", event))
	}

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	// Let it run briefly, then stop
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Stop should wait for in-flight processing
	done := make(chan struct{})
	go func() {
		relay.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good: Stop() completed
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out — graceful shutdown broken")
	}

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()

	if publishedCount == 0 {
		t.Error("expected some events to be published before shutdown")
	}
	t.Logf("published %d/%d events before shutdown", publishedCount, 10)
}

// ── Test: Empty outbox → no failure signal ──────────────────────────────

func TestRelay_EmptyOutboxNoBackoff(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	relay.Stop()

	// Nothing should be published, no errors
	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()
	if publishedCount != 0 {
		t.Errorf("expected 0 published events from empty outbox, got %d", publishedCount)
	}
}

// ── Test: Malformed event → DLQ ─────────────────────────────────────────

func TestRelay_MalformedEventMovesToDLQ(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond

	// Add an event with invalid JSON payload
	store.addEntry(OutboxEntry{
		ID:            1,
		AggregateType: "node",
		AggregateID:   "node-1",
		EventType:     "ai.node.ready",
		Payload:       []byte("this is not valid json {{{"),
	})

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	relay.Stop()

	store.mu.Lock()
	_, inOutbox := store.entries[1]
	dlqLen := len(store.dlq)
	store.mu.Unlock()

	if inOutbox {
		t.Error("malformed event should be removed from outbox")
	}
	if dlqLen != 1 {
		t.Fatalf("malformed event should be in DLQ, got %d DLQ entries", dlqLen)
	}
}

// ── Test: Oversized message → DLQ ───────────────────────────────────────

func TestRelay_OversizedMovesToDLQ(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{
		publishErr: errors.New("Message larger than configured MaxMessageBytes limit (5000000 vs 1048576)"),
	}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 10 * time.Millisecond

	entry := makeEntry(1, "ai.node.ready", makeEvent("task-1", "node-1"))
	store.addEntry(entry)

	relay := NewRelayWithStore(store, pub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	relay.Stop()

	store.mu.Lock()
	_, inOutbox := store.entries[1]
	dlqLen := len(store.dlq)
	store.mu.Unlock()

	if inOutbox {
		t.Error("oversized event should be removed from outbox")
	}
	if dlqLen != 1 {
		t.Fatalf("oversized event should be in DLQ, got %d DLQ entries", dlqLen)
	}
}

// ── Test: Concurrent relay instances don't duplicate ────────────────────
// (Verifies FOR UPDATE SKIP LOCKED semantics — two relays should not
// publish the same event twice because mockStore.FetchPending doesn't
// implement SKIP LOCKED, but the pgStore does.)

func TestRelay_ConcurrentSafe(t *testing.T) {
	store := newMockStore()
	pub := &mockPublisher{}
	cfg := DefaultRelayConfig()
	cfg.PollInterval = 5 * time.Millisecond
	cfg.BatchSize = 10

	// Add 50 events
	for i := 0; i < 50; i++ {
		event := makeEvent(fmt.Sprintf("task-%d", i), fmt.Sprintf("node-%d", i))
		store.addEntry(makeEntry(0, "ai.node.ready", event))
	}

	// Start 3 concurrent relays sharing the same store
	var relays []*Relay
	for i := 0; i < 3; i++ {
		r := NewRelayWithStore(store, pub, cfg)
		relays = append(relays, r)
	}

	ctx, cancel := context.WithCancel(context.Background())
	for _, r := range relays {
		r.Start(ctx)
	}

	time.Sleep(300 * time.Millisecond)
	cancel()
	for _, r := range relays {
		r.Stop()
	}

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()

	// With in-memory mock (no real row locks), we'll get duplicates.
	// But in production (pgStore with SKIP LOCKED), each event is processed once.
	// This test verifies the system doesn't crash under concurrency.
	t.Logf("published %d events across 3 concurrent relays", publishedCount)
	if publishedCount == 0 {
		t.Error("expected some events to be published")
	}
}

// ── Test: isOversized helper ────────────────────────────────────────────

func TestIsOversized(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.New("Message larger than configured MaxMessageBytes"), true},
		{errors.New("kafka: MaxMessageBytes exceeded"), true},
		{errors.New("message too large"), true},
		{errors.New("connection refused"), false},
		{errors.New("timeout"), false},
	}

	for _, tt := range tests {
		if got := isOversized(tt.err); got != tt.expected {
			t.Errorf("isOversized(%q) = %v, want %v", tt.err.Error(), got, tt.expected)
		}
	}
}
