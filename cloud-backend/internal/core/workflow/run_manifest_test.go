package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tangying-ai/aios-core/internal/core/database"
	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type workflowManifestRow []interface{}

func (r workflowManifestRow) Scan(dest ...interface{}) error {
	if len(dest) != len(r) {
		return fmt.Errorf("destinations=%d values=%d", len(dest), len(r))
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
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

type workflowRepositoryCall struct {
	query string
	args  []interface{}
}

type fakeWorkflowRows struct {
	rows    []workflowManifestRow
	index   int
	scanErr error
	err     error
	closed  bool
}

func (r *fakeWorkflowRows) Next() bool { return r.index < len(r.rows) }
func (r *fakeWorkflowRows) Scan(dest ...interface{}) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.rows[r.index]
	r.index++
	return row.Scan(dest...)
}
func (r *fakeWorkflowRows) Close()     { r.closed = true }
func (r *fakeWorkflowRows) Err() error { return r.err }

type fakeWorkflowRunDB struct {
	execs     []workflowRepositoryCall
	row       workflowManifestRow
	queryRows *fakeWorkflowRows
	queryErr  error
}

func (db *fakeWorkflowRunDB) Exec(_ context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	db.execs = append(db.execs, workflowRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (db *fakeWorkflowRunDB) QueryRow(_ context.Context, query string, args ...interface{}) workflowRunScanner {
	db.execs = append(db.execs, workflowRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	return db.row
}
func (db *fakeWorkflowRunDB) Query(_ context.Context, query string, args ...interface{}) (workflowRunRows, error) {
	db.execs = append(db.execs, workflowRepositoryCall{query: query, args: append([]interface{}(nil), args...)})
	if db.queryErr != nil {
		return nil, db.queryErr
	}
	return db.queryRows, nil
}

func TestTraceIDForRunUsesExistingCorrelation(t *testing.T) {
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if got := traceIDForRun(ctx, "task-fallback"); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace ID = %q", got)
	}
	if got := traceIDForRun(context.Background(), "task-fallback"); got != "task-fallback" {
		t.Fatalf("fallback trace ID = %q", got)
	}
}

func TestBuildWorkflowRunManifestExcludesRequestPayloads(t *testing.T) {
	now := time.Now().UTC()
	run := &WorkflowRun{
		ID: "wfr-1", TraceID: "trace-1", ToolRegistrySnapshotID: "snapshot-1", CreatedAt: now,
		Input: map[string]interface{}{"prompt": "raw-prompt-secret", "apiKey": "credential-secret"},
	}
	manifest := buildWorkflowRunManifest(run)
	wire, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != run.ID || manifest.TraceID != run.TraceID || manifest.ToolRegistrySnapshotID != run.ToolRegistrySnapshotID {
		t.Fatalf("manifest identity mismatch: %#v", manifest)
	}
	for _, forbidden := range []string{"raw-prompt-secret", "credential-secret", "prompt", "apiKey"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("workflow run manifest leaked %q: %s", forbidden, wire)
		}
	}
}

func TestWorkflowRunManifestUsesSharedBounds(t *testing.T) {
	manifest := &RunManifest{
		SchemaVersion: strings.Repeat("v", database.RunManifestMaxVersionBytes),
		Runtime:       "cloud-workflow",
		RunID:         strings.Repeat("r", database.RunManifestMaxIdentifierBytes),
		TraceID:       strings.Repeat("t", database.RunManifestMaxIdentifierBytes),
		CreatedAt:     time.Now().UTC(),
	}
	if err := validateWorkflowRunManifest(manifest); err != nil {
		t.Fatalf("workflow manifest at bounds failed: %v", err)
	}
	manifest.RunID += "x"
	if err := validateWorkflowRunManifest(manifest); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
		t.Fatalf("workflow manifest overflow error = %v", err)
	}
}

func TestScanWorkflowRunPreservesManifestAndNullableLineage(t *testing.T) {
	now := time.Now().UTC()
	parent := "workflow-parent"
	stage := "render"
	run, err := scanWorkflowRun(workflowManifestRow{
		"wfr-1", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "trace-1", "snapshot-1",
		[]byte(`{"schemaVersion":"1","toolRegistrySnapshotId":"snapshot-1"}`), &parent, &stage,
		(*time.Time)(nil), (*time.Time)(nil), now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ToolRegistrySnapshotID != "snapshot-1" || run.RunManifest == nil || run.RunManifest.SchemaVersion != "1" {
		t.Fatalf("run manifest was not restored: %#v", run)
	}
	if run.ParentRunID == nil || *run.ParentRunID != parent || run.ReplayFromStageID == nil || *run.ReplayFromStageID != stage {
		t.Fatalf("lineage was not restored: %#v", run)
	}
}

func TestScanWorkflowRunPreservesNullManifestAndLineage(t *testing.T) {
	now := time.Now().UTC()
	run, err := scanWorkflowRun(workflowManifestRow{
		"wfr-legacy", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "", "", nil, nil, nil,
		(*time.Time)(nil), (*time.Time)(nil), now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.RunManifest != nil || run.ParentRunID != nil || run.ReplayFromStageID != nil {
		t.Fatalf("legacy SQL NULLs were not preserved: %#v", run)
	}
}

func TestWorkflowRepositoryCreateFindAndListUseManifestColumns(t *testing.T) {
	now := time.Now().UTC()
	parent, replay := "parent-1", "stage-1"
	row := workflowManifestRow{
		"wfr-1", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "trace-1", "snapshot-1",
		[]byte(`{"schemaVersion":"1","runtime":"cloud-workflow","runId":"wfr-1","traceId":"trace-1","toolRegistrySnapshotId":"snapshot-1","createdAt":"2026-07-27T12:00:00Z"}`),
		&parent, &replay, (*time.Time)(nil), (*time.Time)(nil), now,
	}
	db := &fakeWorkflowRunDB{row: row, queryRows: &fakeWorkflowRows{rows: []workflowManifestRow{row}}}
	repo := newRunRepositoryWithDB(db)
	run := &WorkflowRun{
		ID: "wfr-1", ProjectID: "project-1", UserID: "user-1", TemplateID: "template-1", TemplateVersion: "1",
		TaskID: "task-1", Status: RunPending, TraceID: "trace-1", ToolRegistrySnapshotID: "snapshot-1",
		ParentRunID: &parent, ReplayFromStageID: &replay, CreatedAt: now,
		RunManifest: &RunManifest{SchemaVersion: "1", Runtime: "cloud-workflow", RunID: "wfr-1", CreatedAt: now},
	}
	if err := repo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if db.execs[0].args[11] != "trace-1" || db.execs[0].args[12] != "snapshot-1" || db.execs[0].args[14] != run.ParentRunID || db.execs[0].args[15] != run.ReplayFromStageID {
		t.Fatalf("Create argument order = %#v", db.execs[0].args)
	}
	found, err := repo.FindByID(context.Background(), "wfr-1")
	if err != nil || found.ParentRunID == nil || found.RunManifest == nil {
		t.Fatalf("FindByID run=%#v error=%v", found, err)
	}
	listed, err := repo.FindByProject(context.Background(), "project-1")
	if err != nil || len(listed) != 1 || listed[0].ReplayFromStageID == nil {
		t.Fatalf("FindByProject runs=%#v error=%v", listed, err)
	}
	if !db.queryRows.closed {
		t.Fatal("FindByProject did not close rows")
	}
	for _, call := range db.execs {
		compact := strings.Join(strings.Fields(call.query), " ")
		if !strings.Contains(compact, "tool_registry_snapshot_id") || !strings.Contains(compact, "run_manifest, parent_run_id, replay_from_stage_id") {
			t.Fatalf("repository query missing manifest columns: %s", call.query)
		}
	}
	malformed := append(workflowManifestRow(nil), row...)
	malformed[13] = []byte(`{"schemaVersion":`)
	db.row = malformed
	if _, err := repo.FindByID(context.Background(), "wfr-malformed"); err == nil {
		t.Fatal("FindByID accepted malformed run manifest JSON")
	}
}

func TestWorkflowRepositoryRejectsOversizedManifestBeforeExec(t *testing.T) {
	db := &fakeWorkflowRunDB{}
	repo := newRunRepositoryWithDB(db)
	run := &WorkflowRun{RunManifest: &RunManifest{
		SchemaVersion: "1", Runtime: "cloud-workflow", RunID: strings.Repeat("r", database.RunManifestMaxIdentifierBytes+1),
	}}
	if err := repo.Create(context.Background(), run); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
		t.Fatalf("Create error = %v", err)
	}
	if len(db.execs) != 0 {
		t.Fatal("oversized workflow manifest reached database")
	}
}

func TestWorkflowRepositoryRunIdentityColumnBoundsBeforeCreate(t *testing.T) {
	fields := []struct {
		name  string
		limit int
		set   func(*WorkflowRun, string)
	}{
		{name: "trace ID", limit: database.RunTraceIDMaxBytes, set: func(run *WorkflowRun, value string) { run.TraceID = value }},
		{name: "tool snapshot ID", limit: database.RunToolRegistrySnapshotIDMaxBytes, set: func(run *WorkflowRun, value string) { run.ToolRegistrySnapshotID = value }},
		{name: "parent run ID", limit: database.RunParentRunIDMaxBytes, set: func(run *WorkflowRun, value string) { run.ParentRunID = &value }},
		{name: "replay stage ID", limit: database.RunReplayFromStageIDMaxBytes, set: func(run *WorkflowRun, value string) { run.ReplayFromStageID = &value }},
	}
	for _, field := range fields {
		t.Run(field.name+"/exact", func(t *testing.T) {
			db := &fakeWorkflowRunDB{}
			run := &WorkflowRun{ID: "wfr-1"}
			field.set(run, strings.Repeat("x", field.limit))
			if err := newRunRepositoryWithDB(db).Create(context.Background(), run); err != nil {
				t.Fatalf("exact boundary failed: %v", err)
			}
			if len(db.execs) != 1 {
				t.Fatalf("database calls = %d", len(db.execs))
			}
		})
		t.Run(field.name+"/overflow", func(t *testing.T) {
			db := &fakeWorkflowRunDB{}
			run := &WorkflowRun{ID: "wfr-1"}
			field.set(run, strings.Repeat("x", field.limit+1))
			err := newRunRepositoryWithDB(db).Create(context.Background(), run)
			if err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
				t.Fatalf("overflow error = %v", err)
			}
			if len(db.execs) != 0 {
				t.Fatalf("overflow reached database: %#v", db.execs)
			}
		})
	}
}

func TestWorkflowRepositoryRunIdentityColumnBoundsAllowNilLineage(t *testing.T) {
	db := &fakeWorkflowRunDB{}
	if err := newRunRepositoryWithDB(db).Create(context.Background(), &WorkflowRun{ID: "wfr-1"}); err != nil {
		t.Fatal(err)
	}
	if len(db.execs) != 1 {
		t.Fatalf("database calls = %d", len(db.execs))
	}
}

func TestWorkflowRepositoryFindByProjectPropagatesQueryError(t *testing.T) {
	queryErr := fmt.Errorf("query failed")
	_, err := newRunRepositoryWithDB(&fakeWorkflowRunDB{queryErr: queryErr}).FindByProject(context.Background(), "project-1")
	if err != queryErr {
		t.Fatalf("error = %v", err)
	}
}

func TestWorkflowRepositoryFindByProjectClosesRowsOnRowErrors(t *testing.T) {
	now := time.Now().UTC()
	validRow := workflowManifestRow{
		"wfr-1", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "trace-1", "snapshot-1",
		[]byte(`{"schemaVersion":"1"}`), nil, nil, (*time.Time)(nil), (*time.Time)(nil), now,
	}
	malformedRow := append(workflowManifestRow(nil), validRow...)
	malformedRow[13] = []byte(`{"schemaVersion":`)
	scanErr := fmt.Errorf("scan failed")
	rowsErr := fmt.Errorf("rows failed")
	cases := []struct {
		name string
		rows *fakeWorkflowRows
		want error
	}{
		{name: "scan", rows: &fakeWorkflowRows{rows: []workflowManifestRow{validRow}, scanErr: scanErr}, want: scanErr},
		{name: "malformed manifest", rows: &fakeWorkflowRows{rows: []workflowManifestRow{malformedRow}}},
		{name: "terminal rows error", rows: &fakeWorkflowRows{err: rowsErr}, want: rowsErr},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := newRunRepositoryWithDB(&fakeWorkflowRunDB{queryRows: test.rows}).FindByProject(context.Background(), "project-1")
			if test.want != nil && err != test.want {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.want == nil && err == nil {
				t.Fatal("malformed run manifest JSON was accepted")
			}
			if !test.rows.closed {
				t.Fatal("rows were not closed")
			}
		})
	}
}
