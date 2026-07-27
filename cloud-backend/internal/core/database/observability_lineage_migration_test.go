package database

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFreshSchemasDeclareLineageOnceOnCorrectTables(t *testing.T) {
	source, err := os.ReadFile("database.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	workflow := regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS workflow_runs \((.*?)\n\s*\);`).FindStringSubmatch(text)
	local := regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS local_jobs \((.*?)\n\s*\);`).FindStringSubmatch(text)
	if len(workflow) != 2 || len(local) != 2 {
		t.Fatal("fresh table definitions not found")
	}
	if count := strings.Count(workflow[1], "trace_id VARCHAR"); count != 1 {
		t.Fatalf("workflow trace declarations=%d\n%s", count, workflow[1])
	}
	for _, column := range []string{"trace_id VARCHAR(64)", "span_id VARCHAR(32)", "parent_span_id VARCHAR(32)"} {
		if count := strings.Count(local[1], "\n\t\t    "+column); count != 1 {
			t.Fatalf("local_jobs %s declarations=%d\n%s", column, count, local[1])
		}
	}
}
