package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// statefulRelayDB is a semantic PostgreSQL substitute for the relay boundary.
// It models JSONB equality without ingestedAt, TIMESTAMPTZ microsecond storage,
// the atomic outbox/summary acceptance decision, tuple paging, expiry, and
// user-scoped acknowledgement. SQL-shape tests separately lock the statement
// to the same contract when a live PostgreSQL server is unavailable.
type statefulRelayDB struct {
	now           func() time.Time
	outbox        map[string]*statefulOutboxRow
	summaries     map[string]*statefulSummaryRow
	summaryWrites int
}

type statefulOutboxRow struct {
	userID, runID, traceID string
	eventID                string
	occurredAt, ingestedAt time.Time
	expiresAt              time.Time
	payload                []byte
	delivered              bool
}

type statefulSummaryRow struct {
	summary     RunSummary
	lastEventID string
}

type errorScanner struct{ err error }

func (scanner errorScanner) Scan(...interface{}) error { return scanner.err }

func newStatefulRelayDB(now func() time.Time) *statefulRelayDB {
	return &statefulRelayDB{
		now:       now,
		outbox:    map[string]*statefulOutboxRow{},
		summaries: map[string]*statefulSummaryRow{},
	}
}

func (db *statefulRelayDB) QueryRow(_ context.Context, query string, args ...interface{}) rowScanner {
	switch {
	case strings.Contains(query, "INSERT INTO observability_event_outbox"):
		return db.save(args)
	case strings.Contains(query, "FROM observability_run_summaries"):
		key := args[0].(string) + "\x00" + args[1].(string)
		stored := db.summaries[key]
		if stored == nil {
			return errorScanner{err: pgx.ErrNoRows}
		}
		correlation, _ := json.Marshal(stored.summary.Correlation)
		versions, _ := json.Marshal(stored.summary.Versions)
		return relayRow{
			stored.summary.RunID,
			stored.summary.Status,
			cloneInt64(stored.summary.DurationMs),
			append([]string(nil), stored.summary.ErrorFingerprints...),
			correlation,
			versions,
			stored.summary.UpdatedAt,
		}
	default:
		return errorScanner{err: fmt.Errorf("unexpected QueryRow: %s", query)}
	}
}

func (db *statefulRelayDB) save(args []interface{}) rowScanner {
	eventID := args[0].(string)
	userID := args[1].(string)
	payload := append([]byte(nil), args[5].([]byte)...)
	if stored := db.outbox[eventID]; stored != nil {
		accepted := stored.userID == userID && bytes.Equal(payloadWithoutIngestedAt(stored.payload), payloadWithoutIngestedAt(payload))
		return relayRow{accepted}
	}

	row := &statefulOutboxRow{
		eventID:    eventID,
		userID:     userID,
		runID:      args[2].(string),
		traceID:    args[3].(string),
		occurredAt: postgresMicrosecond(args[4].(time.Time)),
		payload:    payload,
		ingestedAt: postgresMicrosecond(args[6].(time.Time)),
		expiresAt:  postgresMicrosecond(args[7].(time.Time)),
	}
	db.outbox[eventID] = row
	db.updateSummary(row, args)
	return relayRow{true}
}

func (db *statefulRelayDB) updateSummary(outbox *statefulOutboxRow, args []interface{}) {
	if outbox.runID == "" {
		return
	}
	var correlation Correlation
	var versions RunVersions
	_ = json.Unmarshal(args[11].([]byte), &correlation)
	_ = json.Unmarshal(args[12].([]byte), &versions)
	incoming := RunSummary{
		RunID:             outbox.runID,
		Status:            args[8].(ExecutionStatus),
		DurationMs:        cloneInt64(args[9].(*int64)),
		ErrorFingerprints: append([]string(nil), args[10].([]string)...),
		Correlation:       correlation,
		Versions:          versions,
		UpdatedAt:         outbox.occurredAt,
	}
	key := outbox.userID + "\x00" + outbox.runID
	stored := db.summaries[key]
	if stored == nil {
		incoming.ErrorFingerprints = boundedFingerprints(incoming.ErrorFingerprints)
		db.summaries[key] = &statefulSummaryRow{summary: incoming, lastEventID: outbox.eventID}
		db.summaryWrites++
		return
	}
	stored.summary.ErrorFingerprints = boundedFingerprints(append(stored.summary.ErrorFingerprints, incoming.ErrorFingerprints...))
	if summaryStateAdvances(stored.summary.Status, stored.summary.UpdatedAt, stored.lastEventID, incoming.Status, incoming.UpdatedAt, outbox.eventID) {
		stored.summary.Status = incoming.Status
		if incoming.DurationMs != nil {
			stored.summary.DurationMs = cloneInt64(incoming.DurationMs)
		}
		stored.summary.Correlation = incoming.Correlation
		stored.summary.Versions = incoming.Versions
		stored.summary.UpdatedAt = incoming.UpdatedAt
		stored.lastEventID = outbox.eventID
	}
	db.summaryWrites++
}

func summaryStateAdvances(current ExecutionStatus, currentAt time.Time, currentID string, incoming ExecutionStatus, incomingAt time.Time, incomingID string) bool {
	after := incomingAt.After(currentAt) || (incomingAt.Equal(currentAt) && incomingID > currentID)
	return after && (!terminalSummaryStatus(current) || terminalSummaryStatus(incoming))
}

func terminalSummaryStatus(status ExecutionStatus) bool {
	switch status {
	case ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusSkipped:
		return true
	default:
		return false
	}
}

func boundedFingerprints(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if hashPattern.MatchString(value) {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > maxRunErrorFingerprints {
		result = result[:maxRunErrorFingerprints]
	}
	return result
}

func payloadWithoutIngestedAt(payload []byte) []byte {
	var decoded map[string]any
	if json.Unmarshal(payload, &decoded) != nil {
		return nil
	}
	delete(decoded, "ingestedAt")
	canonical, _ := json.Marshal(decoded)
	return canonical
}

func postgresMicrosecond(value time.Time) time.Time {
	return time.UnixMicro(value.UnixMicro()).UTC()
}

func (db *statefulRelayDB) Query(_ context.Context, query string, args ...interface{}) (eventRows, error) {
	if !strings.Contains(query, "FROM observability_event_outbox") {
		return nil, fmt.Errorf("unexpected Query: %s", query)
	}
	userID := args[0].(string)
	cursorAt := args[1].(time.Time)
	cursorID := args[2].(string)
	limit := args[3].(int)
	now := postgresMicrosecond(db.now())
	matching := make([]*statefulOutboxRow, 0)
	for _, row := range db.outbox {
		afterCursor := row.occurredAt.After(cursorAt) || (row.occurredAt.Equal(cursorAt) && row.eventID > cursorID)
		if row.userID == userID && afterCursor && !row.delivered && row.expiresAt.After(now) {
			matching = append(matching, row)
		}
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].occurredAt.Equal(matching[j].occurredAt) {
			return matching[i].eventID < matching[j].eventID
		}
		return matching[i].occurredAt.Before(matching[j].occurredAt)
	})
	if len(matching) > limit {
		matching = matching[:limit]
	}
	rows := &relayRows{}
	for _, row := range matching {
		rows.values = append(rows.values, relayRow{row.eventID, row.occurredAt, append([]byte(nil), row.payload...)})
	}
	return rows, nil
}

func (db *statefulRelayDB) Exec(_ context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	if !strings.Contains(query, "UPDATE observability_event_outbox") {
		return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec: %s", query)
	}
	userID := args[0].(string)
	ids := args[1].([]string)
	now := postgresMicrosecond(db.now())
	var count int64
	for _, eventID := range ids {
		row := db.outbox[eventID]
		if row != nil && row.userID == userID && !row.delivered && row.expiresAt.After(now) {
			row.delivered = true
			count++
		}
	}
	return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", count)), nil
}

func TestStatefulRelayDuplicateAndCollisionContract(t *testing.T) {
	now := time.Date(2026, 7, 27, 13, 0, 0, 123_456_789, time.UTC)
	clock := now
	db := newStatefulRelayDB(func() time.Time { return clock })
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return clock })
	event := validEvent()

	if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
		t.Fatal(err)
	}
	stored := db.outbox[event.EventID]
	originalPayload := append([]byte(nil), stored.payload...)
	originalExpiry := stored.expiresAt
	clock = clock.Add(time.Hour)
	if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
		t.Fatalf("identical duplicate: %v", err)
	}
	if !bytes.Equal(stored.payload, originalPayload) || !stored.expiresAt.Equal(originalExpiry) || db.summaryWrites != 1 {
		t.Fatalf("duplicate mutated state: expiry=%v payload=%s summaryWrites=%d", stored.expiresAt, stored.payload, db.summaryWrites)
	}

	conflicting := event
	conflicting.Execution.Attempt++
	if err := repo.SaveSummary(context.Background(), "user_a", conflicting); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("same-owner conflict error=%v", err)
	}
	if err := repo.SaveSummary(context.Background(), "user_b", event); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("cross-owner conflict error=%v", err)
	}
	if stored.userID != "user_a" || !bytes.Equal(stored.payload, originalPayload) || !stored.expiresAt.Equal(originalExpiry) {
		t.Fatalf("collision overwrote row: %#v", stored)
	}
}

func TestStatefulRelaySummaryTransitionMatrix(t *testing.T) {
	duration50, duration100, duration200 := int64(50), int64(100), int64(200)
	tests := []struct {
		name                                        string
		firstStatus, secondStatus                   ExecutionStatus
		firstID, secondID                           string
		secondOffset                                time.Duration
		firstDuration, secondDuration, wantDuration *int64
		wantStatus                                  ExecutionStatus
	}{
		{"terminal blocks newer progress", ExecutionStatusCompleted, ExecutionStatusInProgress, "evt_a", "evt_b", time.Second, &duration100, nil, &duration100, ExecutionStatusCompleted},
		{"newer terminal replaces terminal", ExecutionStatusFailed, ExecutionStatusCompleted, "evt_a", "evt_b", time.Second, &duration100, &duration200, &duration200, ExecutionStatusCompleted},
		{"newer progress preserves duration", ExecutionStatusStarted, ExecutionStatusInProgress, "evt_a", "evt_b", time.Second, &duration50, nil, &duration50, ExecutionStatusInProgress},
		{"equal time higher id wins", ExecutionStatusStarted, ExecutionStatusInProgress, "evt_a", "evt_b", 0, &duration50, &duration100, &duration100, ExecutionStatusInProgress},
		{"equal time lower id loses", ExecutionStatusStarted, ExecutionStatusCompleted, "evt_z", "evt_a", 0, &duration50, &duration200, &duration50, ExecutionStatusStarted},
		{"terminal without duration preserves prior", ExecutionStatusInProgress, ExecutionStatusFailed, "evt_a", "evt_b", time.Second, &duration50, nil, &duration50, ExecutionStatusFailed},
	}
	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := base.Add(time.Hour)
			db := newStatefulRelayDB(func() time.Time { return now })
			repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return now })
			first := validEvent()
			first.EventID, first.OccurredAt, first.Execution.Status, first.Execution.DurationMs = test.firstID, base, test.firstStatus, test.firstDuration
			second := validEvent()
			second.EventID, second.OccurredAt, second.Execution.Status, second.Execution.DurationMs = test.secondID, base.Add(test.secondOffset), test.secondStatus, test.secondDuration
			if err := repo.SaveSummary(context.Background(), "user_a", first); err != nil {
				t.Fatal(err)
			}
			if err := repo.SaveSummary(context.Background(), "user_a", second); err != nil {
				t.Fatal(err)
			}
			summary, err := repo.RunSummary(context.Background(), "user_a", first.Correlation.WorkflowRunID)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Status != test.wantStatus || !equalOptionalInt64(summary.DurationMs, test.wantDuration) {
				t.Fatalf("summary status=%s duration=%v, want %s/%v", summary.Status, summary.DurationMs, test.wantStatus, test.wantDuration)
			}
		})
	}
}

func equalOptionalInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func TestStatefulRelayTuplePaginationAckIsolationAndExpiry(t *testing.T) {
	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	clock := base.Add(time.Hour)
	db := newStatefulRelayDB(func() time.Time { return clock })
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return clock })
	for _, eventID := range []string{"evt_a", "evt_b", "evt_c"} {
		event := validEvent()
		event.EventID, event.OccurredAt = eventID, base
		if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
			t.Fatal(err)
		}
	}
	other := validEvent()
	other.EventID, other.OccurredAt = "evt_other", base
	if err := repo.SaveSummary(context.Background(), "user_b", other); err != nil {
		t.Fatal(err)
	}

	first, err := repo.Pull(context.Background(), "user_a", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{first.Events[0].EventID, first.Events[1].EventID}; fmt.Sprint(got) != "[evt_a evt_b]" || first.NextCursor == "" {
		t.Fatalf("first page ids=%v cursor=%q", got, first.NextCursor)
	}
	second, err := repo.Pull(context.Background(), "user_a", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) != 1 || second.Events[0].EventID != "evt_c" || second.NextCursor != "" {
		t.Fatalf("second page=%#v", second)
	}
	if count, err := repo.Acknowledge(context.Background(), "user_b", []string{"evt_b"}); err != nil || count != 0 {
		t.Fatalf("cross-user ack count=%d error=%v", count, err)
	}
	if count, err := repo.Acknowledge(context.Background(), "user_a", []string{"evt_b"}); err != nil || count != 1 {
		t.Fatalf("owner ack count=%d error=%v", count, err)
	}
	if count, err := repo.Acknowledge(context.Background(), "user_a", []string{"evt_b"}); err != nil || count != 0 {
		t.Fatalf("duplicate ack count=%d error=%v", count, err)
	}
	clock = base.Add(74 * time.Hour)
	expired, err := repo.Pull(context.Background(), "user_a", "", 10)
	if err != nil || len(expired.Events) != 0 {
		t.Fatalf("expired page=%#v error=%v", expired, err)
	}
}

func TestStatefulRelayBoundsAndSortsErrorFingerprints(t *testing.T) {
	base := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	now := base.Add(time.Hour)
	db := newStatefulRelayDB(func() time.Time { return now })
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return now })
	for index := 0; index < maxRunErrorFingerprints+1; index++ {
		event := validEvent()
		event.EventID = fmt.Sprintf("evt_%03d", index)
		event.OccurredAt = base.Add(time.Duration(index) * time.Millisecond)
		event.Error.Fingerprint = fmt.Sprintf("%064x", maxRunErrorFingerprints+1-index)
		if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := repo.RunSummary(context.Background(), "user_a", validEvent().Correlation.WorkflowRunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.ErrorFingerprints) != maxRunErrorFingerprints || !sort.StringsAreSorted(summary.ErrorFingerprints) {
		t.Fatalf("fingerprints count=%d sorted=%v", len(summary.ErrorFingerprints), sort.StringsAreSorted(summary.ErrorFingerprints))
	}
}

func TestStatefulRelayCursorHistoryCoversEventAgePlusRelayLifetime(t *testing.T) {
	acceptedAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	oldestAcceptedOccurrence := acceptedAt.Add(-maxEventAge)
	clock := acceptedAt
	db := newStatefulRelayDB(func() time.Time { return clock })
	repo := newRepositoryWithRelayDBAndClock(db, func() time.Time { return clock })
	event := validEvent()
	event.EventID = "evt_z"
	event.OccurredAt = oldestAcceptedOccurrence
	if err := repo.SaveSummary(context.Background(), "user_a", event); err != nil {
		t.Fatal(err)
	}
	cursor, err := encodeCursor(oldestAcceptedOccurrence, "evt_a")
	if err != nil {
		t.Fatal(err)
	}

	clock = acceptedAt.Add(relayLifetime - time.Microsecond)
	page, err := repo.Pull(context.Background(), "user_a", cursor, 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].EventID != "evt_z" {
		t.Fatalf("pre-expiry cursor page=%#v error=%v", page, err)
	}

	// Cursor history is inclusive at maxEventAge+relayLifetime. At the exact
	// boundary the row is expired by expires_at > NOW(), but the cursor remains
	// structurally valid and returns an empty page.
	clock = acceptedAt.Add(relayLifetime)
	page, err = repo.Pull(context.Background(), "user_a", cursor, 10)
	if err != nil || len(page.Events) != 0 {
		t.Fatalf("exact-expiry cursor page=%#v error=%v", page, err)
	}

	clock = acceptedAt.Add(relayLifetime + time.Microsecond)
	if _, err := repo.Pull(context.Background(), "user_a", cursor, 10); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("post-history cursor error=%v", err)
	}
}

func TestStatefulFingerprintNormalizationMatchesMigrationPolicy(t *testing.T) {
	if normalized := boundedFingerprints(nil); len(normalized) != 0 {
		t.Fatalf("nil normalization = %v", normalized)
	}
	values := []string{"invalid", strings.Repeat("a", 64), strings.Repeat("a", 64)}
	for index := maxRunErrorFingerprints + 1; index >= 1; index-- {
		values = append(values, fmt.Sprintf("%064x", index))
	}
	normalized := boundedFingerprints(values)
	if len(normalized) != maxRunErrorFingerprints || !sort.StringsAreSorted(normalized) {
		t.Fatalf("normalized count=%d sorted=%v", len(normalized), sort.StringsAreSorted(normalized))
	}
	for index := 1; index < len(normalized); index++ {
		if normalized[index] == normalized[index-1] || !hashPattern.MatchString(normalized[index]) {
			t.Fatalf("normalized fingerprints = %v", normalized)
		}
	}
}
