package observability

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memorySink struct {
	mu     sync.Mutex
	events []Event
	closed bool
}

func (s *memorySink) Write(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *memorySink) Close(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *memorySink) snapshot() ([]Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...), s.closed
}

type blockingSink struct {
	memorySink
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingSink) Write(ctx context.Context, event Event) error {
	s.once.Do(func() {
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
		}
	})
	return s.memorySink.Write(ctx, event)
}

func TestEmitterNormalizesW3CIdentifiersDeterministically(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	event := validEvent()
	event.Correlation = Correlation{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
	}

	if err := emitter.Emit(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, _ := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("sink events = %d, want 1", len(events))
	}
	got := events[0].Correlation
	if got.TraceID != "trc_4bf92f3577b34da6a3ce929d0e0e4736" ||
		got.SpanID != "spn_00f067aa0ba902b7" ||
		got.ParentSpanID != "spn_b7ad6b7169203331" {
		t.Fatalf("wire correlation = %#v", got)
	}
}

func TestEmitterFillsMissingCorrelationFieldsFromContext(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	ctx := WithCorrelation(context.Background(), Correlation{
		TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SpanID:  "00f067aa0ba902b7",
		TaskID:  "tsk_context",
	})
	event := validEvent()
	event.Correlation = Correlation{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}

	if err := emitter.Emit(ctx, event); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, _ := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("sink events = %d, want 1", len(events))
	}
	got := events[0].Correlation
	if got.TraceID != "trc_4bf92f3577b34da6a3ce929d0e0e4736" ||
		got.SpanID != "spn_00f067aa0ba902b7" ||
		got.TaskID != "tsk_context" {
		t.Fatalf("merged correlation = %#v", got)
	}
}

func TestEmitterFullQueueDiscardsDebugBeforeProtectedEvent(t *testing.T) {
	sink := &blockingSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 1)

	inFlight := validEvent()
	inFlight.EventID = "evt_inflight"
	if err := emitter.Emit(context.Background(), inFlight); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("sink did not start first write")
	}

	debug := validEvent()
	debug.EventID = "evt_debug"
	debug.Severity = SeverityDebug
	debug.EventType = EventTypeWorkflowStageProgress
	debug.MessageKey = "workflow.stage.progress"
	debug.Execution.Status = ExecutionStatusInProgress
	if err := emitter.Emit(context.Background(), debug); err != nil {
		t.Fatal(err)
	}

	audit := validEvent()
	audit.EventID = "evt_audit"
	audit.Severity = SeverityInfo
	audit.EventType = EventTypeAgentDecisionRecorded
	audit.MessageKey = "agent.decision.recorded"
	audit.Execution.Status = ExecutionStatusCompleted
	if err := emitter.Emit(context.Background(), audit); err != nil {
		t.Fatalf("protected Emit() error = %v", err)
	}

	close(sink.release)
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, closed := sink.snapshot()
	if !closed {
		t.Fatal("sink was not closed")
	}
	if len(events) != 2 {
		t.Fatalf("sink events = %#v, want in-flight and protected", events)
	}
	if events[0].EventID != "evt_inflight" || events[1].EventID != "evt_audit" {
		t.Fatalf("event order = %q, %q", events[0].EventID, events[1].EventID)
	}
}

func TestEmitterCloseFlushesQueuedEvents(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	for i, id := range []string{"evt_one", "evt_two", "evt_three"} {
		event := validEvent()
		event.EventID = id
		event.ProducerSequence = int64(i)
		if err := emitter.Emit(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, closed := sink.snapshot()
	if len(events) != 3 || !closed {
		t.Fatalf("after Close events = %d, sink closed = %v", len(events), closed)
	}
}

func TestEmitterAuditEventDisplacesUnprotectedInfo(t *testing.T) {
	sink := &blockingSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 1)

	inFlight := validEvent()
	inFlight.EventID = "evt_inflight"
	if err := emitter.Emit(context.Background(), inFlight); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("sink did not start first write")
	}

	progress := validEvent()
	progress.EventID = "evt_progress"
	progress.Severity = SeverityInfo
	progress.EventType = EventTypeWorkflowStageProgress
	progress.MessageKey = "workflow.stage.progress"
	progress.Execution.Status = ExecutionStatusInProgress
	if err := emitter.Emit(context.Background(), progress); err != nil {
		t.Fatal(err)
	}

	audit := validEvent()
	audit.EventID = "evt_request"
	audit.Severity = SeverityInfo
	audit.EventType = EventTypeRequestAccepted
	audit.MessageKey = "request.accepted"
	audit.Execution.Status = ExecutionStatusCompleted
	if err := emitter.Emit(context.Background(), audit); err != nil {
		t.Fatalf("audit Emit() error = %v", err)
	}

	close(sink.release)
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, _ := sink.snapshot()
	if len(events) != 2 || events[1].EventID != "evt_request" {
		t.Fatalf("sink events = %#v, want in-flight and request audit", events)
	}
}

func testSource() Source {
	return Source{
		Service:     "cloud-backend",
		Component:   "http-server",
		Environment: "test",
	}
}
