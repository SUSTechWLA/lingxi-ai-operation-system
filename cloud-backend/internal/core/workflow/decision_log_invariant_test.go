package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type decisionInvariantDB struct {
	execCalls int
	args      []interface{}
}

func (db *decisionInvariantDB) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	db.execCalls++
	db.args = append([]interface{}(nil), args...)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (*decisionInvariantDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

type countingDecisionResolver struct {
	runID string
	calls int
}

func (r *countingDecisionResolver) FindRunIDByTaskID(context.Context, string) (string, error) {
	r.calls++
	return r.runID, nil
}

func TestDecisionLogStoreSaveRequiresResolverAndTask(t *testing.T) {
	tests := []struct {
		name   string
		store  *pgxDecisionLogStore
		record *DecisionLogRecord
		want   string
	}{
		{"no resolver", &pgxDecisionLogStore{db: &decisionInvariantDB{}}, &DecisionLogRecord{TaskID: "task-1", WorkflowRunID: "wfr-real"}, "resolver"},
		{"empty task", &pgxDecisionLogStore{db: &decisionInvariantDB{}, resolver: &countingDecisionResolver{runID: "wfr-real"}}, &DecisionLogRecord{WorkflowRunID: "wfr-real"}, "taskId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.store.Save(context.Background(), tt.record); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Save error=%v, want %s", err, tt.want)
			}
			if tt.store.db.(*decisionInvariantDB).execCalls != 0 {
				t.Fatal("invalid decision reached persistence")
			}
		})
	}
}

func TestDecisionLogStoreSaveRejectsMismatchedWorkflowRun(t *testing.T) {
	db := &decisionInvariantDB{}
	store := &pgxDecisionLogStore{db: db, resolver: &countingDecisionResolver{runID: "wfr-real"}}
	err := store.Save(context.Background(), &DecisionLogRecord{TaskID: "task-1", WorkflowRunID: "agent_run_fake"})
	if err == nil || !strings.Contains(err.Error(), "does not belong") || db.execCalls != 0 {
		t.Fatalf("error=%v execCalls=%d", err, db.execCalls)
	}
}

func TestDecisionLogStoreSavePersistsOnlyResolvedExactIdentity(t *testing.T) {
	for _, supplied := range []string{"", "wfr-real"} {
		t.Run("supplied="+supplied, func(t *testing.T) {
			db := &decisionInvariantDB{}
			resolver := &countingDecisionResolver{runID: "wfr-real"}
			store := &pgxDecisionLogStore{db: db, resolver: resolver}
			record := &DecisionLogRecord{TaskID: "task-1", WorkflowRunID: supplied, DecisionType: DecisionStageApproval}
			if err := store.Save(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			if resolver.calls != 1 || db.execCalls != 1 || record.WorkflowRunID != "wfr-real" || db.args[1] != "wfr-real" {
				t.Fatalf("resolverCalls=%d execCalls=%d record=%+v args=%+v", resolver.calls, db.execCalls, record, db.args)
			}
		})
	}
}

func TestDecisionLogStoreSaveSimpleResolvesExactlyOnce(t *testing.T) {
	db := &decisionInvariantDB{}
	resolver := &countingDecisionResolver{runID: "wfr-real"}
	store := &pgxDecisionLogStore{db: db, resolver: resolver}
	if err := store.SaveSimple(context.Background(), "task-1", "render", DecisionStageApproval, "yes", "reviewer", "", true); err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || db.execCalls != 1 {
		t.Fatalf("resolverCalls=%d execCalls=%d", resolver.calls, db.execCalls)
	}
}
