package artifact

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRepositoryProjectArtifactQueryIncludesHistoryOnlyWhenRequested(t *testing.T) {
	queryErr := errors.New("stop after recording query")
	for _, test := range []struct {
		name           string
		includeHistory bool
		wantCurrent    bool
	}{
		{name: "creator current-only default", includeHistory: false, wantCurrent: true},
		{name: "diagnostics all versions", includeHistory: true, wantCurrent: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			queryer := &recordingArtifactProjectQueryer{err: queryErr}
			_, err := listProjectArtifactsFromQueryer(context.Background(), queryer, "project-1", test.includeHistory)
			if !errors.Is(err, queryErr) {
				t.Fatalf("error = %v, want recording sentinel", err)
			}
			if got := strings.Contains(queryer.query, "is_current=true"); got != test.wantCurrent {
				t.Fatalf("query current predicate = %v, want %v: %s", got, test.wantCurrent, queryer.query)
			}
			if len(queryer.args) != 1 || queryer.args[0] != "project-1" {
				t.Fatalf("query args = %#v, want project scope", queryer.args)
			}
		})
	}
}

type recordingArtifactProjectQueryer struct {
	query string
	args  []interface{}
	err   error
}

func (q *recordingArtifactProjectQueryer) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	q.query = sql
	q.args = args
	return nil, q.err
}

func TestServiceProjectArtifactListingPreservesCurrentDefaultAndSupportsDiagnosticsHistory(t *testing.T) {
	current := &Artifact{ID: "current", IsCurrent: true}
	historical := &Artifact{ID: "history", IsCurrent: false}
	store := &recordingArtifactProjectLister{
		current: []*Artifact{current},
		all:     []*Artifact{current, historical},
	}

	gotCurrent, err := listArtifactsByProject(context.Background(), store, "project-1", false)
	if err != nil || len(gotCurrent) != 1 || gotCurrent[0].ID != "current" || store.currentCalls != 1 || store.allCalls != 0 {
		t.Fatalf("current listing = %#v, calls current=%d all=%d, error=%v", gotCurrent, store.currentCalls, store.allCalls, err)
	}
	gotAll, err := listArtifactsByProject(context.Background(), store, "project-1", true)
	if err != nil || len(gotAll) != 2 || gotAll[1].ID != "history" || store.currentCalls != 1 || store.allCalls != 1 {
		t.Fatalf("all-version listing = %#v, calls current=%d all=%d, error=%v", gotAll, store.currentCalls, store.allCalls, err)
	}
}

type recordingArtifactProjectLister struct {
	current      []*Artifact
	all          []*Artifact
	currentCalls int
	allCalls     int
}

func (s *recordingArtifactProjectLister) ListByProject(context.Context, string) ([]*Artifact, error) {
	s.currentCalls++
	return s.current, nil
}

func (s *recordingArtifactProjectLister) ListAllVersionsByProject(context.Context, string) ([]*Artifact, error) {
	s.allCalls++
	return s.all, nil
}
