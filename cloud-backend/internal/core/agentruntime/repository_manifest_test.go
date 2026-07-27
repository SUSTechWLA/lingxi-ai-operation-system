package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tangying-ai/aios-core/internal/core/database"
	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type agentRepositoryCall struct {
	query string
	args  []interface{}
}

type fakeAgentRunDB struct {
	execs     []agentRepositoryCall
	rows      []pgx.Row
	queryRows agentRunRows
	queryErr  error
}

func (db *fakeAgentRunDB) Exec(_ context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	db.execs = append(db.execs, agentRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (db *fakeAgentRunDB) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	if len(db.rows) == 0 {
		return manifestRow(nil)
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	db.execs = append(db.execs, agentRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	return row
}

func (db *fakeAgentRunDB) Query(_ context.Context, query string, args ...interface{}) (agentRunRows, error) {
	db.execs = append(db.execs, agentRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	if db.queryErr != nil {
		return nil, db.queryErr
	}
	if db.queryRows == nil {
		return nil, fmt.Errorf("unexpected Query call")
	}
	return db.queryRows, nil
}

type manifestRow []interface{}

func (r manifestRow) Scan(dest ...interface{}) error {
	if len(dest) != len(r) {
		return fmt.Errorf("destinations=%d values=%d", len(dest), len(r))
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", i)
		}
		if r[i] == nil {
			target.Elem().Set(reflect.Zero(target.Elem().Type()))
			continue
		}
		value := reflect.ValueOf(r[i])
		if !value.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("value %d type %s cannot assign to %s", i, value.Type(), target.Elem().Type())
		}
		target.Elem().Set(value)
	}
	return nil
}

type fakeAgentRunRows struct {
	rows    []manifestRow
	index   int
	scanErr error
	err     error
	closed  bool
}

func (r *fakeAgentRunRows) Next() bool { return r.index < len(r.rows) }
func (r *fakeAgentRunRows) Scan(dest ...interface{}) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.index]
	r.index++
	return row.Scan(dest...)
}
func (r *fakeAgentRunRows) Close()     { r.closed = true }
func (r *fakeAgentRunRows) Err() error { return r.err }

func TestScanRunPreservesManifestAndNullableLineage(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	parent := "parent-1"
	stage := "stage-2"
	manifest := []byte(`{"schemaVersion":"1","runtime":"cloud-agent","runId":"run-1","toolRegistrySnapshotId":"tool_snapshot_hash","toolRegistrySha256":"hash","parentRunId":"parent-1","replayFromStageId":"stage-2","createdAt":"2026-07-27T12:00:00Z"}`)
	row := manifestRow{
		"run-1", (*string)(nil), (*string)(nil), (*string)(nil), "legacy prompt", []byte("null"),
		"CREATED", []byte("{}"), []byte("{}"), "trace-1", "tool_snapshot_hash", manifest,
		&parent, &stage, now, now,
	}
	run, err := scanRun(row)
	if err != nil {
		t.Fatal(err)
	}
	if run.TraceID != "trace-1" || run.ToolRegistrySnapshotID != "tool_snapshot_hash" || run.RunManifest == nil {
		t.Fatalf("manifest identity was not restored: %#v", run)
	}
	if run.ParentRunID == nil || *run.ParentRunID != parent || run.ReplayFromStageID == nil || *run.ReplayFromStageID != stage {
		t.Fatalf("lineage was not restored: %#v", run)
	}

	row[12], row[13] = nil, nil
	run, err = scanRun(row)
	if err != nil {
		t.Fatal(err)
	}
	if run.ParentRunID != nil || run.ReplayFromStageID != nil {
		t.Fatalf("SQL NULL lineage was not preserved: %#v", run)
	}
}

func TestScanRunRejectsMalformedRunManifestJSON(t *testing.T) {
	now := time.Now()
	_, err := scanRun(manifestRow{
		"run-1", (*string)(nil), (*string)(nil), (*string)(nil), "legacy prompt", []byte("null"),
		"CREATED", []byte("{}"), []byte("{}"), "", "", []byte(`{"schemaVersion":`),
		nil, nil, now, now,
	})
	if err == nil {
		t.Fatal("malformed run manifest JSON was accepted")
	}
}

func TestMarshalRunFieldsIncludesPrivacySafeManifest(t *testing.T) {
	now := time.Now().UTC()
	run := &Run{RunManifest: &RunManifest{
		SchemaVersion: "1", Runtime: "cloud-agent", RunID: "run-1",
		ToolRegistrySnapshotID: "snapshot-1", ToolRegistrySHA256: "hash-1", CreatedAt: now,
	}}
	_, _, _, manifestJSON := marshalRunFields(run)
	var decoded map[string]interface{}
	if err := json.Unmarshal(manifestJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["toolRegistrySnapshotId"] != "snapshot-1" || decoded["toolRegistrySha256"] != "hash-1" {
		t.Fatalf("manifest JSON = %s", manifestJSON)
	}
}

func TestRepositoryCreateSaveAndTerminalUpsertPersistManifestColumns(t *testing.T) {
	db := &fakeAgentRunDB{}
	repo := newRepositoryWithDB(db)
	parent, replay := "parent-1", "stage-1"
	run := &Run{
		ID: "run-1", Status: RunStatusCreated, TraceID: "trace-1", ToolRegistrySnapshotID: "snapshot-1",
		ParentRunID: &parent, ReplayFromStageID: &replay, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		RunManifest: &RunManifest{
			SchemaVersion: "1", Runtime: "cloud-agent", RunID: "run-1", TraceID: "trace-1",
			ToolRegistrySnapshotID: "snapshot-1", ToolRegistrySHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC(),
		},
	}
	created, err := repo.CreateRun(context.Background(), run)
	if err != nil || !created {
		t.Fatalf("CreateRun created=%v error=%v", created, err)
	}
	if err := repo.SaveRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveRunTerminal(context.Background(), run, RunTerminalEvent{EventID: "terminal-1", RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	if len(db.execs) != 3 {
		t.Fatalf("exec calls = %d", len(db.execs))
	}
	for index, call := range db.execs {
		compact := strings.Join(strings.Fields(call.query), " ")
		ordered := "trace_id, tool_registry_snapshot_id, run_manifest, parent_run_id, replay_from_stage_id"
		if !strings.Contains(compact, ordered) {
			t.Fatalf("call %d has wrong manifest column order: %s", index, compact)
		}
		if call.args[9] != "trace-1" || call.args[10] != "snapshot-1" || call.args[12] != run.ParentRunID || call.args[13] != run.ReplayFromStageID {
			t.Fatalf("call %d manifest argument order = %#v", index, call.args)
		}
	}
	if db.execs[2].args[17] != "terminal-1" {
		t.Fatalf("terminal event argument order = %#v", db.execs[2].args)
	}
}

func TestRepositoryFindRunExecutesManifestSelectAndRejectsMalformedJSON(t *testing.T) {
	now := time.Now().UTC()
	valid := manifestRow{
		"run-1", (*string)(nil), (*string)(nil), (*string)(nil), "legacy", []byte("null"), "CREATED",
		[]byte("{}"), []byte("{}"), "trace-1", "snapshot-1",
		[]byte(`{"schemaVersion":"1","runtime":"cloud-agent","runId":"run-1","toolRegistrySnapshotId":"snapshot-1","toolRegistrySha256":"hash","createdAt":"2026-07-27T12:00:00Z"}`),
		nil, nil, now, now,
	}
	malformed := append(manifestRow(nil), valid...)
	malformed[11] = []byte(`{"schemaVersion":`)
	db := &fakeAgentRunDB{rows: []pgx.Row{valid, malformed}}
	repo := newRepositoryWithDB(db)
	run, err := repo.FindRun(context.Background(), "run-1")
	if err != nil || run.RunManifest == nil || run.RunManifest.SchemaVersion != "1" {
		t.Fatalf("FindRun run=%#v error=%v", run, err)
	}
	if !strings.Contains(strings.Join(strings.Fields(db.execs[0].query), " "), "trace_id, ''), COALESCE(tool_registry_snapshot_id, ''), run_manifest, parent_run_id, replay_from_stage_id") {
		t.Fatalf("FindRun select order: %s", db.execs[0].query)
	}
	if _, err := repo.FindRun(context.Background(), "run-2"); err == nil {
		t.Fatal("FindRun accepted malformed manifest JSON")
	}
}

func TestRepositoryRejectsOversizedManifestBeforeExec(t *testing.T) {
	db := &fakeAgentRunDB{}
	repo := newRepositoryWithDB(db)
	run := &Run{ID: "run-1", RunManifest: &RunManifest{
		SchemaVersion: "1", Runtime: "cloud-agent", RunID: strings.Repeat("r", database.RunManifestMaxIdentifierBytes+1),
	}}
	if _, err := repo.CreateRun(context.Background(), run); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
		t.Fatalf("CreateRun error = %v", err)
	}
	if len(db.execs) != 0 {
		t.Fatal("oversized manifest reached database")
	}
}

func TestRepositoryRunIdentityColumnBoundsApplyBeforeEveryWrite(t *testing.T) {
	fields := []struct {
		name  string
		limit int
		set   func(*Run, string)
	}{
		{name: "trace ID", limit: database.RunTraceIDMaxBytes, set: func(run *Run, value string) { run.TraceID = value }},
		{name: "tool snapshot ID", limit: database.RunToolRegistrySnapshotIDMaxBytes, set: func(run *Run, value string) { run.ToolRegistrySnapshotID = value }},
		{name: "parent run ID", limit: database.RunParentRunIDMaxBytes, set: func(run *Run, value string) { run.ParentRunID = &value }},
		{name: "replay stage ID", limit: database.RunReplayFromStageIDMaxBytes, set: func(run *Run, value string) { run.ReplayFromStageID = &value }},
	}
	writers := []struct {
		name  string
		write func(*Repository, *Run) error
	}{
		{name: "CreateRun", write: func(repo *Repository, run *Run) error {
			_, err := repo.CreateRun(context.Background(), run)
			return err
		}},
		{name: "SaveRun", write: func(repo *Repository, run *Run) error {
			return repo.SaveRun(context.Background(), run)
		}},
		{name: "SaveRunTerminal", write: func(repo *Repository, run *Run) error {
			return repo.SaveRunTerminal(context.Background(), run, RunTerminalEvent{EventID: "event-1", RunID: run.ID})
		}},
	}

	for _, writer := range writers {
		for _, field := range fields {
			t.Run(writer.name+"/"+field.name+"/exact", func(t *testing.T) {
				db := &fakeAgentRunDB{}
				run := &Run{ID: "run-1"}
				field.set(run, strings.Repeat("x", field.limit))
				if err := writer.write(newRepositoryWithDB(db), run); err != nil {
					t.Fatalf("exact boundary failed: %v", err)
				}
				if len(db.execs) != 1 {
					t.Fatalf("database calls = %d", len(db.execs))
				}
			})
			t.Run(writer.name+"/"+field.name+"/overflow", func(t *testing.T) {
				db := &fakeAgentRunDB{}
				run := &Run{ID: "run-1"}
				field.set(run, strings.Repeat("x", field.limit+1))
				err := writer.write(newRepositoryWithDB(db), run)
				if err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
					t.Fatalf("overflow error = %v", err)
				}
				if len(db.execs) != 0 {
					t.Fatalf("overflow reached database: %#v", db.execs)
				}
			})
		}
	}
}

func TestRepositoryRunIdentityColumnBoundsAllowNilLineage(t *testing.T) {
	db := &fakeAgentRunDB{}
	run := &Run{ID: "run-1"}
	if _, err := newRepositoryWithDB(db).CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if len(db.execs) != 1 {
		t.Fatalf("database calls = %d", len(db.execs))
	}
}

func TestRepositoryClaimTerminalEventsScansJSONAndClosesRows(t *testing.T) {
	event := RunTerminalEvent{
		EventID:                "evt_event_1",
		CallbackIdempotencyKey: "evt_event_1",
		RunID:                  "run-1",
		Status:                 RunStatusSuccess,
		Context:                map[string]interface{}{"result": "ok"},
		PreparedObservability:  &observability.SealedPreparedEvent{Payload: []byte("prepared")},
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	rows := &fakeAgentRunRows{rows: []manifestRow{{"run-1", "evt_event_1", "claim-1", true, false, eventJSON}}}
	db := &fakeAgentRunDB{queryRows: rows}

	deliveries, err := newRepositoryWithDB(db).ClaimTerminalEvents(
		context.Background(), 4, time.Now().Add(time.Minute), "claim-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].RunID != "run-1" || deliveries[0].EventID != "evt_event_1" ||
		deliveries[0].ClaimToken != "claim-1" || !deliveries[0].PayloadFrozen ||
		!deliveries[0].CallbackDelivered || deliveries[0].ObservabilityDelivered ||
		!reflect.DeepEqual(deliveries[0].Event, event) {
		t.Fatalf("deliveries = %#v", deliveries)
	}
	if !rows.closed {
		t.Fatal("terminal event rows were not closed")
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "RETURNING runs.id") {
		t.Fatalf("query calls = %#v", db.execs)
	}
}

func TestRepositoryClaimTerminalEventsRecoversLegacySQLIdentity(t *testing.T) {
	eventJSON := []byte(`{"runId":"run-legacy","userId":"owner-legacy","traceId":"trace-legacy","status":"SUCCESS","occurredAt":"2026-07-28T01:02:03Z"}`)
	rows := &fakeAgentRunRows{rows: []manifestRow{{
		"run-legacy", "evt_agent_terminal_legacy_run-legacy", "claim-legacy", false, false, eventJSON,
	}}}
	deliveries, err := newRepositoryWithDB(&fakeAgentRunDB{queryRows: rows}).ClaimTerminalEvents(
		context.Background(), 1, time.Now().Add(time.Minute), "claim-legacy",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries=%#v", deliveries)
	}
	delivery := deliveries[0]
	if delivery.PayloadFrozen || delivery.Event.EventID != delivery.EventID ||
		delivery.Event.CallbackIdempotencyKey != delivery.EventID {
		t.Fatalf("legacy delivery=%#v", delivery)
	}
}

func TestRepositoryClaimTerminalEventsRejectsInvalidOrConflictingIdentity(t *testing.T) {
	tests := []struct {
		name      string
		sqlID     string
		jsonEvent string
		want      string
	}{
		{name: "blank SQL identity", sqlID: " ", jsonEvent: `{"eventId":"evt_json_valid"}`, want: "identity invalid"},
		{name: "invalid SQL identity", sqlID: "bad/value", jsonEvent: `{"eventId":"evt_json_valid"}`, want: "identity invalid"},
		{name: "conflicting valid identities", sqlID: "evt_sql_valid", jsonEvent: `{"eventId":"evt_json_valid"}`, want: "identity mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := &fakeAgentRunRows{rows: []manifestRow{{
				"run-conflict", test.sqlID, "claim-conflict", false, false,
				[]byte(strings.TrimSuffix(test.jsonEvent, "}") + `,"runId":"run-conflict","status":"SUCCESS","occurredAt":"2026-07-28T01:02:03Z"}`),
			}}}
			_, err := newRepositoryWithDB(&fakeAgentRunDB{queryRows: rows}).ClaimTerminalEvents(
				context.Background(), 1, time.Now().Add(time.Minute), "claim-conflict",
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRepositoryClaimTerminalEventsPropagatesQueryError(t *testing.T) {
	queryErr := fmt.Errorf("query failed")
	rows := &fakeAgentRunRows{}
	db := &fakeAgentRunDB{queryRows: rows, queryErr: queryErr}
	_, err := newRepositoryWithDB(db).ClaimTerminalEvents(context.Background(), 1, time.Now(), "claim-1")
	if err != queryErr {
		t.Fatalf("error = %v", err)
	}
	if rows.closed {
		t.Fatal("rows were closed even though Query did not return them")
	}
}

func TestRepositoryClaimTerminalEventsClosesRowsOnRowErrors(t *testing.T) {
	scanErr := fmt.Errorf("scan failed")
	rowsErr := fmt.Errorf("rows failed")
	cases := []struct {
		name string
		rows *fakeAgentRunRows
		want error
	}{
		{name: "scan", rows: &fakeAgentRunRows{rows: []manifestRow{{"run-1"}}, scanErr: scanErr}, want: scanErr},
		{name: "malformed JSON", rows: &fakeAgentRunRows{rows: []manifestRow{{"run-1", "event-1", "claim-1", false, false, []byte(`{"eventId":`)}}}},
		{name: "terminal rows error", rows: &fakeAgentRunRows{err: rowsErr}, want: rowsErr},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := newRepositoryWithDB(&fakeAgentRunDB{queryRows: test.rows}).ClaimTerminalEvents(
				context.Background(), 1, time.Now(), "claim-1",
			)
			if test.want != nil && err != test.want {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.want == nil && err == nil {
				t.Fatal("malformed terminal event JSON was accepted")
			}
			if !test.rows.closed {
				t.Fatal("rows were not closed")
			}
		})
	}
}
