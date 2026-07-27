package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

type ownerCapturingSink struct {
	owners []string
	events []Event
	err    error
}

func (s *ownerCapturingSink) Write(ctx context.Context, event Event) error {
	owner, _ := trustedcontext.UserID(ctx)
	s.owners = append(s.owners, owner)
	s.events = append(s.events, event)
	return s.err
}
func (s *ownerCapturingSink) Close(context.Context) error { return s.err }

func TestEmitterSnapshotsTrustedOwnerAndScopesProducerComponent(t *testing.T) {
	sink := &ownerCapturingSink{}
	emitter := NewEmitter(Source{Service: "cloud", Component: "http-server", Environment: "test"}, Runtime{}, sink, 4)
	ctx := trustedcontext.WithUserID(context.Background(), "user-1")
	event := validEvent()
	event.Severity = SeverityInfo
	if err := emitter.ForComponent(ComponentWorker).Emit(ctx, event); err != nil {
		t.Fatal(err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := emitter.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.owners[0] != "user-1" {
		t.Fatalf("event=%d owners=%v", len(sink.events), sink.owners)
	}
	if got := sink.events[0].Source.Component; got != string(ComponentWorker) {
		t.Fatalf("component=%q", got)
	}
}

func TestCompositeSinkAttemptsEverySinkAndJoinsErrors(t *testing.T) {
	a := &ownerCapturingSink{err: errors.New("a")}
	b := &ownerCapturingSink{err: errors.New("b")}
	event := validEvent()
	event.Severity = SeverityInfo
	err := NewCompositeSink(a, b).Write(context.Background(), event)
	if len(a.events) != 1 || len(b.events) != 1 {
		t.Fatalf("writes a=%d b=%d", len(a.events), len(b.events))
	}
	if !errors.Is(err, a.err) || !errors.Is(err, b.err) {
		t.Fatalf("joined error=%v", err)
	}
}

func TestUserOwnedEmitterEventReachesScopedPullAndAck(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := newStatefulRelayDB(func() time.Time { return now })
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return now })
	emitter := NewEmitter(Source{Service: "cloud", Component: "http-server", Environment: "test"}, Runtime{}, NewRepositorySink(repo), 4)
	event := validEvent()
	event.EventID = "evt_owned"
	event.OccurredAt = now
	event.EventType = EventTypeRequestAccepted
	event.Correlation.WorkflowRunID = ""
	if err := emitter.Emit(trustedcontext.WithUserID(context.Background(), "user-1"), event); err != nil {
		t.Fatal(err)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := emitter.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	page, err := repo.Pull(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != "evt_owned" {
		t.Fatalf("events=%+v", page.Events)
	}
	if len(db.summaries) != 0 {
		t.Fatalf("request event invented summaries=%v", db.summaries)
	}
	if _, err := repo.Acknowledge(context.Background(), "user-1", []string{"evt_owned"}); err != nil {
		t.Fatal(err)
	}
	other, err := repo.Pull(context.Background(), "user-2", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Events) != 0 {
		t.Fatalf("cross-owner events=%+v", other.Events)
	}
}
