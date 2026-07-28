package observability

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var errSinkWrite = errors.New("sink write failed")

type errorSink struct{}

func (*errorSink) Write(context.Context, Event) error { return errSinkWrite }
func (*errorSink) Close(context.Context) error        { return nil }

func TestEmitAndWaitReturnsSinkAcknowledgement(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, &errorSink{}, 2)
	err := emitter.EmitAndWait(context.Background(), validEvent())
	if !errors.Is(err, errSinkWrite) {
		t.Fatalf("err=%v", err)
	}
	_ = emitter.Close(context.Background())
}

type cancelSink struct {
	started chan struct{}
	once    sync.Once
}

func (s *cancelSink) Write(ctx context.Context, _ Event) error {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	return ctx.Err()
}

func (*cancelSink) Close(context.Context) error { return nil }

type gatedContextSink struct {
	memorySink
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *gatedContextSink) Write(ctx context.Context, event Event) error {
	s.once.Do(func() { close(s.started) })
	<-s.release
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.memorySink.Write(ctx, event)
}

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

func TestEmitterHashesUnsafeCorrelationDeterministicallyWithoutLeak(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 8)
	unsafeTrace := "trc_sk_live_Bearer_secret"
	unsafeSpan := "spn_sk_live_Bearer_secret"
	for _, id := range []string{"evt_unsafe_one", "evt_unsafe_two"} {
		event := validEvent()
		event.EventID = id
		event.Correlation.TraceID = unsafeTrace
		event.Correlation.SpanID = unsafeSpan
		if err := emitter.Emit(context.Background(), event); err != nil {
			t.Fatalf("Emit() error = %v", err)
		}
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, _ := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("sink events = %d", len(events))
	}
	if events[0].Correlation.TraceID != events[1].Correlation.TraceID || events[0].Correlation.SpanID != events[1].Correlation.SpanID {
		t.Fatalf("equal unsafe inputs did not correlate deterministically: %#v %#v", events[0].Correlation, events[1].Correlation)
	}
	data, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{unsafeTrace, unsafeSpan, "sk_live", "Bearer", "secret"} {
		if strings.Contains(string(data), literal) {
			t.Fatalf("unsafe correlation literal %q leaked: %s", literal, data)
		}
	}
}

func TestProtectedEventPolicyCoversClosedEventRegistry(t *testing.T) {
	for eventType := range eventTypes {
		event := Event{EventType: eventType, Severity: SeverityDebug}
		wantProtected := eventType != EventTypeWorkflowStageProgress
		if got := protectedEvent(event); got != wantProtected {
			t.Errorf("protectedEvent(%q) = %v, want %v", eventType, got, wantProtected)
		}
	}
	for _, eventType := range []EventType{
		EventTypeAgentResultSubmitted,
		EventTypeVerifyCheckPassed,
		EventTypeArtifactValidated,
		EventTypeArtifactMaterialized,
		EventTypeArtifactExported,
	} {
		if !protectedEvent(Event{EventType: eventType, Severity: SeverityDebug}) {
			t.Errorf("reviewer-named lifecycle event %q is not protected", eventType)
		}
	}
	if protectedEvent(Event{EventType: EventTypeWorkflowStageProgress, Severity: SeverityInfo}) {
		t.Error("INFO progress-only event should be evictable")
	}
	for _, severity := range []Severity{SeverityWarn, SeverityError} {
		if !protectedEvent(Event{EventType: EventTypeWorkflowStageProgress, Severity: severity}) {
			t.Errorf("%s progress event must be protected", severity)
		}
	}
}

func TestEmitterNilSinkIsSafe(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, nil, 1)
	if err := emitter.Emit(context.Background(), validEvent()); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestEmitterCloseReturnsAsyncSinkError(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, &errorSink{}, 1)
	if err := emitter.Emit(context.Background(), validEvent()); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Close(context.Background()); !errors.Is(err, errSinkWrite) {
		t.Fatalf("Close() error = %v, want %v", err, errSinkWrite)
	}
}

func TestEmitterCloseCancellationUnblocksSinkWrite(t *testing.T) {
	sink := &cancelSink{started: make(chan struct{})}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 1)
	if err := emitter.Emit(context.Background(), validEvent()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("sink write did not start")
	}
	closeCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := emitter.Close(closeCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close() error = %v, want context canceled", err)
	}
	select {
	case <-emitter.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after Close context cancellation")
	}
}

func TestEmitterProducerCancellationDoesNotAbortAcceptedEvent(t *testing.T) {
	sink := &gatedContextSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 1)
	producerCtx, cancelProducer := context.WithCancel(WithCorrelation(context.Background(), Correlation{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
	}))
	if err := emitter.Emit(producerCtx, validEvent()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("sink write did not start")
	}
	cancelProducer()
	close(sink.release)

	if err := emitter.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	events, _ := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("delivered events = %d, want 1", len(events))
	}
}

func TestEmitterConcurrentCloseIsIdempotentAndEmitAfterCloseFails(t *testing.T) {
	emitter := NewEmitter(testSource(), Runtime{}, &memorySink{}, 4)
	if err := emitter.Emit(context.Background(), validEvent()); err != nil {
		t.Fatal(err)
	}
	var emitWG sync.WaitGroup
	emitErrs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		emitWG.Add(1)
		go func() {
			defer emitWG.Done()
			event := validEvent()
			event.EventType = EventTypeWorkflowStageProgress
			event.MessageKey = "workflow.stage.progress"
			event.Severity = SeverityDebug
			event.Execution.Status = ExecutionStatusInProgress
			emitErrs <- emitter.Emit(context.Background(), event)
		}()
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- emitter.Close(context.Background())
		}()
	}
	emitWG.Wait()
	close(emitErrs)
	for err := range emitErrs {
		if err != nil && !errors.Is(err, ErrEmitterClosed) && !errors.Is(err, ErrQueueFull) {
			t.Errorf("concurrent Emit() error = %v", err)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Close() error = %v", err)
		}
	}
	if err := emitter.Emit(context.Background(), Event{}); !errors.Is(err, ErrEmitterClosed) {
		t.Fatalf("Emit() after Close error = %v, want ErrEmitterClosed", err)
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

func TestEmitterProtectedOnlyQueueReturnsExplicitOverflow(t *testing.T) {
	sink := &blockingSink{started: make(chan struct{}), release: make(chan struct{})}
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
	queued := validEvent()
	queued.EventID = "evt_protected_one"
	if err := emitter.Emit(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	incoming := validEvent()
	incoming.EventID = "evt_protected_two"
	if err := emitter.Emit(context.Background(), incoming); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("protected-only overflow error = %v, want ErrQueueFull", err)
	}
	close(sink.release)
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
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
