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

func TestEventRunIDAllowsRequestOnlyRelayWithoutInventingRun(t *testing.T) {
	event := validEvent()
	event.EventID = "evt-request-only"
	event.EventType = EventTypeRequestAccepted
	event.Correlation.WorkflowRunID = ""
	runID, err := eventRunID(event)
	if err != nil || runID != "" {
		t.Fatalf("runID=%q err=%v", runID, err)
	}
}

func TestEventRunIDRejectsRunlessDomainLifecycle(t *testing.T) {
	for _, eventType := range []EventType{EventTypeToolCallStarted, EventTypeLocalJobFailed, EventTypeTaskCreated} {
		event := validEvent()
		event.EventType = eventType
		event.Correlation = Correlation{TraceID: "trc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
		if _, err := eventRunID(event); err == nil {
			t.Errorf("%s accepted without run", eventType)
		}
	}
}

func TestEventRunIDUsesAgentOrTaskFallbackForStageLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		correlation Correlation
		want        string
	}{
		{"agent run owns agent stage", Correlation{AgentRunID: "agr_agent"}, "agr_agent"},
		{"task owns task scoped stage", Correlation{TaskID: "tsk_task"}, "tsk_task"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			event.EventType = EventTypeWorkflowStageStarted
			event.Correlation = tt.correlation
			runID, err := eventRunID(event)
			if err != nil || runID != tt.want {
				t.Fatalf("runID=%q err=%v, want %q", runID, err, tt.want)
			}
		})
	}
}

func TestEventRunIDAllowsOnlyExactTranslatorBootstrapLifecycle(t *testing.T) {
	allowed := []struct {
		eventType  EventType
		messageKey string
	}{
		{EventTypeLLMCallStarted, "llm.call.started"},
		{EventTypeLLMCallCompleted, "llm.call.completed"},
		{EventTypeLLMCallFailed, "llm.call.failed"},
		{EventType("llm.call.cancelled"), "llm.call.cancelled"},
		{EventTypeWorkflowStageStarted, "translator.dag.started"},
		{EventTypeWorkflowStageCompleted, "translator.dag.completed"},
		{EventTypeWorkflowStageFailed, "translator.dag.failed"},
		{EventTypeWorkflowStageCancelled, "translator.dag.cancelled"},
		{EventTypeRecoveryFallbackStarted, "recovery.fallback.started"},
		{EventTypeRecoveryFallbackCompleted, "recovery.fallback.completed"},
	}
	for _, tt := range allowed {
		event := validEvent()
		event.Source.Component = "translator"
		event.EventType = tt.eventType
		event.MessageKey = tt.messageKey
		event.Correlation = Correlation{TraceID: "trc_bootstrap", SpanID: "spn_bootstrap", StageID: "translator-llm-dag"}
		if runID, err := eventRunID(event); err != nil || runID != "" {
			t.Errorf("%s/%s runID=%q err=%v", tt.eventType, tt.messageKey, runID, err)
		}
	}
}

func TestEventRunIDDoesNotWidenRunlessAllowlistForCancelledTypes(t *testing.T) {
	for _, eventType := range []EventType{"llm.call.cancelled", "verify.check.cancelled"} {
		event := validEvent()
		event.EventType = eventType
		event.MessageKey = string(eventType)
		event.Correlation = Correlation{TraceID: "trc_cancelled", SpanID: "spn_cancelled"}
		if _, err := eventRunID(event); err == nil {
			t.Errorf("%s accepted without exact translator bootstrap scope or run identity", eventType)
		}
	}
}

func TestEventRunIDRejectsTranslatorBootstrapLookalikes(t *testing.T) {
	base := validEvent()
	base.Source.Component = "translator"
	base.EventType = EventTypeLLMCallStarted
	base.MessageKey = "llm.call.started"
	base.Correlation = Correlation{TraceID: "trc_bootstrap", SpanID: "spn_bootstrap", StageID: "translator-llm-dag"}
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{"component", func(event *Event) { event.Source.Component = "translator-lookalike" }},
		{"stage", func(event *Event) { event.Correlation.StageID = "translator-other" }},
		{"message", func(event *Event) { event.MessageKey = "llm.call.completed" }},
		{"type", func(event *Event) { event.EventType = EventTypeToolCallStarted; event.MessageKey = "tool.call.started" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := base
			tt.mutate(&event)
			if _, err := eventRunID(event); err == nil {
				t.Fatalf("lookalike accepted: %+v", event)
			}
		})
	}
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
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return event.IngestedAt })

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

func TestRepositorySaveUsesCloudAcceptanceClockAndPostgresMicrosecondPrecision(t *testing.T) {
	acceptedAt := time.Date(2026, 7, 27, 15, 4, 5, 987_654_321, time.FixedZone("CST", 8*60*60))
	occurredAt := time.Date(2026, 7, 26, 23, 4, 5, 123_456_789, time.FixedZone("west", -7*60*60))
	event := validEvent()
	event.OccurredAt = occurredAt
	event.IngestedAt = acceptedAt.Add(365 * 24 * time.Hour)
	callerCopy := event
	db := &fakeRelayDB{row: relayRow{true}}
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return acceptedAt })

	if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
		t.Fatal(err)
	}
	call := db.calls[0]
	wantOccurred := time.Date(2026, 7, 27, 6, 4, 5, 123_456_000, time.UTC)
	wantAccepted := time.Date(2026, 7, 27, 7, 4, 5, 987_654_000, time.UTC)
	if call.args[4] != wantOccurred || call.args[6] != wantAccepted || call.args[7] != wantAccepted.Add(72*time.Hour) {
		t.Fatalf("timestamps = occurred:%v accepted:%v expires:%v", call.args[4], call.args[6], call.args[7])
	}
	var persisted Event
	if err := json.Unmarshal(call.args[5].([]byte), &persisted); err != nil {
		t.Fatal(err)
	}
	postgresOccurred := time.UnixMicro(call.args[4].(time.Time).UnixMicro()).UTC()
	postgresAccepted := time.UnixMicro(call.args[6].(time.Time).UnixMicro()).UTC()
	if !persisted.OccurredAt.Equal(postgresOccurred) || !persisted.IngestedAt.Equal(postgresAccepted) {
		t.Fatalf("JSON timestamps do not survive PostgreSQL microsecond round trip: payload=%s columns=%v/%v", call.args[5], postgresOccurred, postgresAccepted)
	}
	if !event.OccurredAt.Equal(callerCopy.OccurredAt) || !event.IngestedAt.Equal(callerCopy.IngestedAt) {
		t.Fatal("SaveSummary mutated caller timestamps")
	}
}

func TestRepositoryRejectsOccurredAtOutsideAcceptanceWindow(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	for _, occurredAt := range []time.Time{
		{},
		now.Add(maxEventFutureSkew + time.Microsecond),
		now.Add(-maxEventAge - time.Microsecond),
	} {
		event := validEvent()
		event.OccurredAt = occurredAt
		db := &fakeRelayDB{row: relayRow{true}}
		err := newRepositoryWithRelayDBAndClock(db, func() time.Time { return now }).SaveSummary(context.Background(), "user_a", event)
		if err == nil || len(db.calls) != 0 {
			t.Fatalf("occurredAt %v accepted, calls=%d", occurredAt, len(db.calls))
		}
	}
}

func TestRepositoryPullRejectsCursorOutsideCursorHistoryWindow(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newRepositoryWithRelayDBAndClock(&fakeRelayDB{rows: &relayRows{}}, func() time.Time { return now })
	for _, occurredAt := range []time.Time{
		{},
		now.Add(maxEventFutureSkew + time.Microsecond),
		now.Add(-maxCursorAge - time.Microsecond),
	} {
		cursor, err := encodeCursor(occurredAt, "evt_one")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Pull(context.Background(), "user_a", cursor, 10); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor time %v error=%v", occurredAt, err)
		}
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
	event := validEvent()
	err := newRepositoryWithRelayDBAndClock(db, func() time.Time { return event.OccurredAt.Add(time.Hour) }).SaveSummary(context.Background(), "user_a", event)
	if !errors.Is(err, ErrEventConflict) {
		t.Fatalf("error = %v, want ErrEventConflict", err)
	}
}

func TestRepositoryPullScopesStableCursorAndClosesRows(t *testing.T) {
	first := validEvent()
	second := validEvent()
	lookahead := validEvent()
	second.EventID = "evt_zz"
	lookahead.EventID = "evt_zzz"
	second.OccurredAt = first.OccurredAt
	lookahead.OccurredAt = first.OccurredAt
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	lookaheadJSON, _ := json.Marshal(lookahead)
	rows := &relayRows{values: []relayRow{
		{first.EventID, first.OccurredAt, firstJSON},
		{second.EventID, second.OccurredAt, secondJSON},
		{lookahead.EventID, lookahead.OccurredAt, lookaheadJSON},
	}}
	db := &fakeRelayDB{rows: rows}
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return first.OccurredAt.Add(time.Hour) })

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
	if _, err := newRepositoryWithRelayDBAndClock(nextDB, func() time.Time { return first.OccurredAt.Add(time.Hour) }).Pull(context.Background(), "user_a", page.NextCursor, 2); err != nil {
		t.Fatal(err)
	}
	nextArgs := nextDB.calls[0].args
	if nextArgs[1] != second.OccurredAt || nextArgs[2] != second.EventID {
		t.Fatalf("decoded cursor args = %#v", nextArgs)
	}
}

func TestRepositoryPullExactlyFullFinalPageHasNoCursor(t *testing.T) {
	first := validEvent()
	second := validEvent()
	second.EventID = "evt_zz"
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	rows := &relayRows{values: []relayRow{
		{first.EventID, first.OccurredAt, firstJSON},
		{second.EventID, second.OccurredAt, secondJSON},
	}}
	page, err := newRepositoryWithRelayDB(&fakeRelayDB{rows: rows}).Pull(context.Background(), "user_a", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || page.NextCursor != "" {
		t.Fatalf("final exactly-full page = %#v", page)
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
		{"unsorted fingerprints", func(row relayRow) {
			row[3] = []string{strings.Repeat("b", 64), strings.Repeat("a", 64)}
		}},
		{"unknown correlation field", func(row relayRow) {
			row[4] = []byte(`{"traceId":"trc_one","spanId":"spn_one","workflowRunId":"wfr_run","prompt":"secret"}`)
		}},
		{"too many fingerprints", func(row relayRow) {
			fingerprints := make([]string, maxRunErrorFingerprints+1)
			for index := range fingerprints {
				fingerprints[index] = fmt.Sprintf("%064x", index+1)
			}
			row[3] = fingerprints
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

func TestSaveSummarySQLKeepsTerminalStateMonotonicAndFingerprintSetBounded(t *testing.T) {
	for _, fragment := range []string{
		"observability_event_outbox.redacted_payload - 'ingestedAt'",
		"EXCLUDED.redacted_payload - 'ingestedAt'",
		"RETURNING (xmax = 0) AS inserted",
		"WHERE $3 <> '' AND EXISTS (SELECT 1 FROM accepted WHERE inserted)",
		"last_event_id",
		"observability_run_summaries.status NOT IN ('COMPLETED','FAILED','CANCELLED','SKIPPED')",
		"EXCLUDED.status IN ('COMPLETED','FAILED','CANCELLED','SKIPPED')",
		"EXCLUDED.last_event_id > observability_run_summaries.last_event_id",
		"COALESCE(EXCLUDED.duration_ms, observability_run_summaries.duration_ms)",
		"ORDER BY fingerprint",
		"LIMIT 128",
	} {
		if !strings.Contains(saveSummarySQL, fragment) {
			t.Errorf("save summary SQL missing %q: %s", fragment, saveSummarySQL)
		}
	}
}
