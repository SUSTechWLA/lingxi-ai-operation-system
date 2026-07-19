package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestVideoProjectConfigRevisionMigrationIsBackwardCompatible(t *testing.T) {
	if !strings.Contains(videoProjectConfigRevisionMigration, "ADD COLUMN IF NOT EXISTS config_revision") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "NOT NULL DEFAULT 0") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "video_project_config_revision_guard") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "app.video_project_expected_revision") {
		t.Fatalf("migration is not backward compatible: %s", videoProjectConfigRevisionMigration)
	}
}

func TestEnsureVideoProjectConfigRevisionReturnsStartupError(t *testing.T) {
	execer := revisionMigrationExecer{err: errors.New("permission denied")}
	err := ensureVideoProjectConfigRevision(context.Background(), execer)
	if err == nil || !strings.Contains(err.Error(), "required video project config revision schema") {
		t.Fatalf("error=%v", err)
	}
}

type revisionMigrationExecer struct{ err error }

func (e revisionMigrationExecer) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, e.err
}
