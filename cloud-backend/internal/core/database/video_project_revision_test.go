package database

import (
	"strings"
	"testing"
)

func TestVideoProjectConfigRevisionMigrationIsBackwardCompatible(t *testing.T) {
	if !strings.Contains(videoProjectConfigRevisionMigration, "ADD COLUMN IF NOT EXISTS config_revision") ||
		!strings.Contains(videoProjectConfigRevisionMigration, "NOT NULL DEFAULT 0") {
		t.Fatalf("migration is not backward compatible: %s", videoProjectConfigRevisionMigration)
	}
}
