package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type relayCall struct {
	query string
	args  []interface{}
}

type fakeRelayDB struct {
	row      relayRow
	rows     *relayRows
	execTag  pgconn.CommandTag
	execErr  error
	calls    []relayCall
	queryErr error
}

func (db *fakeRelayDB) QueryRow(_ context.Context, query string, args ...interface{}) rowScanner {
	db.calls = append(db.calls, relayCall{query: query, args: append([]interface{}(nil), args...)})
	return db.row
}

func (db *fakeRelayDB) Query(_ context.Context, query string, args ...interface{}) (eventRows, error) {
	db.calls = append(db.calls, relayCall{query: query, args: append([]interface{}(nil), args...)})
	return db.rows, db.queryErr
}

func (db *fakeRelayDB) Exec(_ context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	db.calls = append(db.calls, relayCall{query: query, args: append([]interface{}(nil), args...)})
	return db.execTag, db.execErr
}

type relayRow []interface{}

func (row relayRow) Scan(dest ...interface{}) error {
	if len(row) != len(dest) {
		return fmt.Errorf("destinations=%d values=%d", len(dest), len(row))
	}
	for index := range row {
		target := reflect.ValueOf(dest[index])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", index)
		}
		value := reflect.ValueOf(row[index])
		if !value.IsValid() {
			target.Elem().Set(reflect.Zero(target.Elem().Type()))
			continue
		}
		if !value.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("value %d type %s cannot assign to %s", index, value.Type(), target.Elem().Type())
		}
		target.Elem().Set(value)
	}
	return nil
}

type relayRows struct {
	values  []relayRow
	index   int
	scanErr error
	err     error
	closed  bool
}

func (rows *relayRows) Next() bool { return rows.index < len(rows.values) }
func (rows *relayRows) Scan(dest ...interface{}) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	row := rows.values[rows.index]
	rows.index++
	return row.Scan(dest...)
}
func (rows *relayRows) Close()     { rows.closed = true }
func (rows *relayRows) Err() error { return rows.err }

func TestRepositorySaveSummaryRedactsValidatesAndUsesFixedExpiry(t *testing.T) {
	event := validEvent()
	event.Evidence.Attributes = map[string]any{"prompt": "do not persist me"}
	rawCopy := event
	db := &fakeRelayDB{row: relayRow{true}}
	repo := newRepositoryWithRelayDB(db)

	if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
		t.Fatal(err)
	}
	if len(db.calls) != 1 {
		t.Fatalf("calls = %d", len(db.calls))
	}
	call := db.calls[0]
	if call.args[1] != "user_a" {
		t.Fatalf("user scope = %#v", call.args)
	}
	payload, ok := call.args[5].([]byte)
	if !ok {
		t.Fatalf("payload type = %T", call.args[5])
	}
	if strings.Contains(string(payload), "do not persist me") || strings.Contains(string(payload), `"attributes":`) {
		t.Fatalf("unsafe payload persisted: %s", payload)
	}
	var persisted Event
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	if err := persisted.Validate(); err != nil {
		t.Fatalf("persisted event invalid: %v", err)
	}
	if !reflect.DeepEqual(event.Evidence.Attributes, rawCopy.Evidence.Attributes) {
		t.Fatal("SaveSummary mutated caller event")
	}
	if got, want := call.args[7], event.IngestedAt.Add(72*time.Hour); got != want {
		t.Fatalf("expires_at = %v, want %v", got, want)
	}
	if strings.Contains(call.query, "error_message") || strings.Contains(call.query, "prompt") {
		t.Fatalf("save SQL exposes forbidden columns: %s", call.query)
	}
}

func TestRepositorySaveSummaryRejectsUnsafeIdentityAndPayload(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		mutate func(*Event)
	}{
		{"missing user", "", func(*Event) {}},
		{"secret classification", "user_a", func(event *Event) { event.Privacy.Classification = PrivacySecret }},
		{"missing run identity", "user_a", func(event *Event) {
			event.Correlation.AgentRunID = ""
			event.Correlation.WorkflowRunID = ""
		}},
		{"ambiguous run identity", "user_a", func(event *Event) { event.Correlation.AgentRunID = "agr_other" }},
		{"oversized event identity", "user_a", func(event *Event) { event.EventID = "evt_" + strings.Repeat("a", 129) }},
		{"oversized run identity", "user_a", func(event *Event) { event.Correlation.WorkflowRunID = "wfr_" + strings.Repeat("a", 129) }},
		{"oversized trace identity", "user_a", func(event *Event) { event.Correlation.TraceID = "trc_" + strings.Repeat("a", 129) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := validEvent()
			test.mutate(&event)
			db := &fakeRelayDB{row: relayRow{true}}
			err := newRepositoryWithRelayDB(db).SaveSummary(context.Background(), test.userID, event)
			if err == nil {
				t.Fatal("unsafe event was accepted")
			}
			if len(db.calls) != 0 {
				t.Fatalf("database called for rejected event: %#v", db.calls)
			}
		})
	}
}

func TestRepositorySaveSummaryRejectsCrossOwnerEventIDCollision(t *testing.T) {
	db := &fakeRelayDB{row: relayRow{false}}
	err := newRepositoryWithRelayDB(db).SaveSummary(context.Background(), "user_a", validEvent())
	if !errors.Is(err, ErrEventConflict) {
		t.Fatalf("error = %v, want ErrEventConflict", err)
	}
}

func TestRepositoryPullScopesStableCursorAndClosesRows(t *testing.T) {
	first := validEvent()
	second := validEvent()
	second.EventID = "evt_zz"
	second.OccurredAt = first.OccurredAt
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	rows := &relayRows{values: []relayRow{
		{first.EventID, first.OccurredAt, firstJSON},
		{second.EventID, second.OccurredAt, secondJSON},
	}}
	db := &fakeRelayDB{rows: rows}
	repo := newRepositoryWithRelayDB(db)

	page, err := repo.Pull(context.Background(), "user_a", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.closed {
		t.Fatal("rows were not closed")
	}
	if len(page.Events) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %#v", page)
	}
	call := db.calls[0]
	if call.args[0] != "user_a" || call.args[len(call.args)-1] != 3 {
		t.Fatalf("pull args = %#v", call.args)
	}
	if !strings.Contains(call.query, "(occurred_at, event_id)") ||
		!strings.Contains(call.query, "delivered_at IS NULL") ||
		!strings.Contains(call.query, "expires_at >") {
		t.Fatalf("pull query is not stable/active: %s", call.query)
	}

	nextDB := &fakeRelayDB{rows: &relayRows{}}
	if _, err := newRepositoryWithRelayDB(nextDB).Pull(context.Background(), "user_a", page.NextCursor, 2); err != nil {
		t.Fatal(err)
	}
	nextArgs := nextDB.calls[0].args
	if nextArgs[1] != second.OccurredAt || nextArgs[2] != second.EventID {
		t.Fatalf("decoded cursor args = %#v", nextArgs)
	}
}

func TestRepositoryPullRejectsMalformedCursorPayloadAndRowsError(t *testing.T) {
	repo := newRepositoryWithRelayDB(&fakeRelayDB{rows: &relayRows{}})
	for _, cursor := range []string{"not-base64", strings.Repeat("x", maxCursorBytes+1)} {
		if _, err := repo.Pull(context.Background(), "user_a", cursor, 10); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor error = %v", err)
		}
	}

	badJSONRows := &relayRows{values: []relayRow{{"evt_bad", time.Now().UTC(), []byte(`{"eventId":`)}}}
	if _, err := newRepositoryWithRelayDB(&fakeRelayDB{rows: badJSONRows}).Pull(context.Background(), "user_a", "", 10); err == nil {
		t.Fatal("malformed payload was accepted")
	}
	if !badJSONRows.closed {
		t.Fatal("rows not closed after malformed payload")
	}

	rowsErr := errors.New("rows failed")
	if _, err := newRepositoryWithRelayDB(&fakeRelayDB{rows: &relayRows{err: rowsErr}}).Pull(context.Background(), "user_a", "", 10); !errors.Is(err, rowsErr) {
		t.Fatalf("rows error = %v", err)
	}
}

func TestRepositoryAcknowledgeIsUserScopedIdempotentAndBounded(t *testing.T) {
	db := &fakeRelayDB{execTag: pgconn.NewCommandTag("UPDATE 2")}
	repo := newRepositoryWithRelayDB(db)
	count, err := repo.Acknowledge(context.Background(), "user_a", []string{"evt_one", "evt_two"})
	if err != nil || count != 2 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if db.calls[0].args[0] != "user_a" {
		t.Fatalf("ack args = %#v", db.calls[0].args)
	}
	for _, fragment := range []string{"user_id=$1", "delivered_at IS NULL", "event_id = ANY"} {
		if !strings.Contains(db.calls[0].query, fragment) {
			t.Fatalf("ack query missing %q: %s", fragment, db.calls[0].query)
		}
	}
	if _, err := repo.Acknowledge(context.Background(), "user_a", []string{"other"}); err == nil {
		t.Fatal("invalid event ID accepted")
	}
	if _, err := repo.Acknowledge(context.Background(), "user_a", []string{"evt_" + strings.Repeat("a", 129)}); err == nil {
		t.Fatal("oversized event ID accepted")
	}
	if _, err := repo.Acknowledge(context.Background(), "user_a", make([]string, maxAckEventIDs+1)); !errors.Is(err, ErrTooManyEventIDs) {
		t.Fatalf("oversized ack error = %v", err)
	}
}

func TestRepositoryRunSummaryIsUserAndRunScoped(t *testing.T) {
	now := time.Now().UTC()
	duration := int64(42)
	correlation := []byte(`{"traceId":"trc_one","spanId":"spn_one","workflowRunId":"wfr_run"}`)
	versions := []byte(`{"appVersion":"1.2.3","workflowVersion":"v7"}`)
	db := &fakeRelayDB{row: relayRow{
		"wfr_run", ExecutionStatusCompleted, &duration, []string{strings.Repeat("a", 64)}, correlation, versions, now,
	}}
	summary, err := newRepositoryWithRelayDB(db).RunSummary(context.Background(), "user_a", "wfr_run")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunID != "wfr_run" || summary.DurationMs == nil || *summary.DurationMs != duration {
		t.Fatalf("summary = %#v", summary)
	}
	if db.calls[0].args[0] != "user_a" || db.calls[0].args[1] != "wfr_run" {
		t.Fatalf("summary args = %#v", db.calls[0].args)
	}
	if strings.Contains(db.calls[0].query, "redacted_payload") {
		t.Fatalf("summary reads event payload wholesale: %s", db.calls[0].query)
	}
}

func TestRepositoryRunSummaryRejectsMalformedStoredState(t *testing.T) {
	now := time.Now().UTC()
	base := relayRow{
		"wfr_run", ExecutionStatusCompleted, (*int64)(nil), []string{}, []byte(`{"traceId":"trc_one","spanId":"spn_one","workflowRunId":"wfr_run"}`), []byte(`{}`), now,
	}
	tests := []struct {
		name   string
		mutate func(relayRow)
	}{
		{"unknown status", func(row relayRow) { row[1] = ExecutionStatus("BROKEN") }},
		{"negative duration", func(row relayRow) { value := int64(-1); row[2] = &value }},
		{"invalid fingerprint", func(row relayRow) { row[3] = []string{"raw error text"} }},
		{"unknown correlation field", func(row relayRow) {
			row[4] = []byte(`{"traceId":"trc_one","spanId":"spn_one","workflowRunId":"wfr_run","prompt":"secret"}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := append(relayRow(nil), base...)
			test.mutate(row)
			if _, err := newRepositoryWithRelayDB(&fakeRelayDB{row: row}).RunSummary(context.Background(), "user_a", "wfr_run"); err == nil {
				t.Fatal("malformed stored summary was accepted")
			}
		})
	}
}
