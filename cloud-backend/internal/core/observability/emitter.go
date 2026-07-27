package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrEmitterClosed = errors.New("observability emitter is closed")
	ErrQueueFull     = errors.New("observability emitter queue is full")
)

type Sink interface {
	Write(context.Context, Event) error
	Close(context.Context) error
}

type queuedEvent struct {
	ctx   context.Context
	event Event
}

type Emitter struct {
	source   Source
	runtime  Runtime
	sink     Sink
	capacity int
	sequence atomic.Int64

	mu       sync.Mutex
	queue    []queuedEvent
	closing  bool
	closeCtx context.Context
	firstErr error

	wake      chan struct{}
	closeWake chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewEmitter(source Source, runtime Runtime, sink Sink, capacity int) *Emitter {
	if capacity < 1 {
		capacity = 1
	}
	emitter := &Emitter{
		source:    source,
		runtime:   runtime,
		sink:      sink,
		capacity:  capacity,
		wake:      make(chan struct{}, 1),
		closeWake: make(chan struct{}),
		done:      make(chan struct{}),
	}
	go emitter.run()
	return emitter
}

// Emit validates and queues an event without waiting for the sink. When the
// bounded queue is full, DEBUG is always the first eviction candidate.
// Protected events are never silently lost: if no lower-priority event can be
// evicted, Emit returns ErrQueueFull.
func (e *Emitter) Emit(ctx context.Context, event Event) error {
	if e == nil || e.sink == nil {
		return errors.New("observability emitter requires a sink")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := e.prepare(ctx, event)
	if err != nil {
		return err
	}
	item := queuedEvent{ctx: context.WithoutCancel(ctx), event: prepared}

	e.mu.Lock()
	if e.closing {
		e.mu.Unlock()
		return ErrEmitterClosed
	}
	if len(e.queue) >= e.capacity {
		victim := evictionCandidate(e.queue, prepared)
		if victim < 0 {
			e.mu.Unlock()
			return ErrQueueFull
		}
		copy(e.queue[victim:], e.queue[victim+1:])
		e.queue = e.queue[:len(e.queue)-1]
	}
	e.queue = append(e.queue, item)
	e.mu.Unlock()
	e.signal(e.wake)
	return nil
}

func (e *Emitter) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.closing = true
		e.closeCtx = ctx
		e.mu.Unlock()
		close(e.closeWake)
	})

	select {
	case <-e.done:
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.firstErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Emitter) prepare(ctx context.Context, event Event) (Event, error) {
	now := time.Now().UTC()
	event.SchemaVersion = "1.0"
	if event.EventID == "" {
		event.EventID = "evt_" + randomHex(16)
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	event.IngestedAt = now
	event.ProducerSequence = e.sequence.Add(1) - 1
	event.Source = e.source
	event.Runtime = e.runtime
	event.Correlation = mergeCorrelation(event.Correlation, CorrelationFromContext(ctx))
	event.Correlation.TraceID = canonicalTraceID(event.Correlation.TraceID)
	event.Correlation.SpanID = canonicalSpanID(event.Correlation.SpanID)
	event.Correlation.ParentSpanID = canonicalSpanID(event.Correlation.ParentSpanID)
	if event.Execution.Attempt == 0 {
		event.Execution.Attempt = 1
	}
	if event.Privacy.Classification == "" {
		event.Privacy.Classification = PrivacyInternal
	}
	if event.Privacy.RedactedFields == nil {
		event.Privacy.RedactedFields = []string{}
	}
	event = Redact(event)
	if ContainsSecret(event) {
		return Event{}, errors.New("observability event contains secret material")
	}
	if err := event.Validate(); err != nil {
		return Event{}, fmt.Errorf("validate observability event: %w", err)
	}
	return event, nil
}

func mergeCorrelation(primary, fallback Correlation) Correlation {
	fields := []struct {
		primary  *string
		fallback string
	}{
		{&primary.TraceID, fallback.TraceID},
		{&primary.SpanID, fallback.SpanID},
		{&primary.ParentSpanID, fallback.ParentSpanID},
		{&primary.SessionID, fallback.SessionID},
		{&primary.ProjectID, fallback.ProjectID},
		{&primary.TaskID, fallback.TaskID},
		{&primary.WorkflowRunID, fallback.WorkflowRunID},
		{&primary.AgentRunID, fallback.AgentRunID},
		{&primary.StageID, fallback.StageID},
		{&primary.ShotID, fallback.ShotID},
		{&primary.ArtifactID, fallback.ArtifactID},
		{&primary.ToolCallID, fallback.ToolCallID},
		{&primary.ProviderJobID, fallback.ProviderJobID},
	}
	for _, field := range fields {
		if *field.primary == "" {
			*field.primary = field.fallback
		}
	}
	return primary
}

func (e *Emitter) run() {
	defer close(e.done)
	for {
		item, ok, closing := e.take()
		if ok {
			if err := e.sink.Write(item.ctx, item.event); err != nil {
				e.recordError(err)
			}
			continue
		}
		if closing {
			e.mu.Lock()
			closeCtx := e.closeCtx
			e.mu.Unlock()
			if closeCtx == nil {
				closeCtx = context.Background()
			}
			if err := e.sink.Close(closeCtx); err != nil {
				e.recordError(err)
			}
			return
		}
		select {
		case <-e.wake:
		case <-e.closeWake:
		}
	}
}

func (e *Emitter) take() (queuedEvent, bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.queue) == 0 {
		return queuedEvent{}, false, e.closing
	}
	item := e.queue[0]
	copy(e.queue, e.queue[1:])
	e.queue = e.queue[:len(e.queue)-1]
	return item, true, false
}

func (e *Emitter) recordError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.firstErr == nil {
		e.firstErr = err
	}
}

func (e *Emitter) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func evictionCandidate(queue []queuedEvent, incoming Event) int {
	for i, item := range queue {
		if item.event.Severity == SeverityDebug && !protectedEvent(item.event) {
			return i
		}
	}
	if protectedEvent(incoming) {
		for i, item := range queue {
			if !protectedEvent(item.event) {
				return i
			}
		}
	}
	return -1
}

func protectedEvent(event Event) bool {
	if event.Severity == SeverityError || event.EventType == EventTypeWorkflowStageCheckpointed {
		return true
	}
	switch event.EventType {
	case EventTypeRequestAccepted,
		EventTypeRequestAuthenticationSucceeded,
		EventTypeRequestAuthenticationFailed,
		EventTypeAgentDecisionRecorded,
		EventTypeUserDecisionRecorded:
		return true
	}
	value := string(event.EventType)
	for _, suffix := range []string{
		".queued", ".started", ".completed", ".failed", ".cancelled",
		".paused", ".resumed", ".created", ".requested",
	} {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

// Raw W3C identifiers remain in request and Kafka contexts. The canonical v1
// event schema requires product prefixes, so the wire copy receives a
// deterministic prefix and can be converted back by removing it.
func canonicalTraceID(value string) string {
	if strings.HasPrefix(value, "trc_") || value == "" {
		return value
	}
	return "trc_" + strings.ToLower(value)
}

func canonicalSpanID(value string) string {
	if strings.HasPrefix(value, "spn_") || value == "" {
		return value
	}
	return "spn_" + strings.ToLower(value)
}

func randomHex(bytes int) string {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(data)
}
