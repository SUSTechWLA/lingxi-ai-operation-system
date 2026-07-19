package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestProjectRepositoryCompareAndSwapForUserUsesConfigRevision(t *testing.T) {
	db := &recordingProjectDB{tag: pgconn.NewCommandTag("UPDATE 1")}
	repo := &ProjectRepository{db: db}
	project := &model.VideoProject{ID: "vp-1", UserID: "u-1", ConfigRevision: 7}

	swapped, err := repo.CompareAndSwapForUser(context.Background(), "u-1", project, 7)
	if err != nil || !swapped {
		t.Fatalf("swapped=%v error=%v", swapped, err)
	}
	if !strings.Contains(db.sql, "config_revision=$14") || !strings.Contains(db.sql, "config_revision=$15") {
		t.Fatalf("CAS SQL does not compare and increment revision: %s", db.sql)
	}
	if project.ConfigRevision != 8 {
		t.Fatalf("project revision=%d, want 8", project.ConfigRevision)
	}
}

func TestProjectRepositoryCompareAndSwapForUserReportsLostRace(t *testing.T) {
	db := &recordingProjectDB{tag: pgconn.NewCommandTag("UPDATE 0")}
	repo := &ProjectRepository{db: db}
	project := &model.VideoProject{ID: "vp-1", UserID: "u-1", ConfigRevision: 7}
	swapped, err := repo.CompareAndSwapForUser(context.Background(), "u-1", project, 7)
	if err != nil || swapped || project.ConfigRevision != 7 {
		t.Fatalf("swapped=%v revision=%d error=%v", swapped, project.ConfigRevision, err)
	}
}

type recordingProjectDB struct {
	tag pgconn.CommandTag
	sql string
}

func (d *recordingProjectDB) Exec(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
	d.sql = sql
	return d.tag, nil
}

func (d *recordingProjectDB) QueryRow(context.Context, string, ...interface{}) pgx.Row { return nil }
func (d *recordingProjectDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
