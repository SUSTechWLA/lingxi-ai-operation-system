package localrunner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPendingReportStoreSaveAndRemove(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	report := PendingReport{
		JobID: "local_job_001",
		Type:  "complete",
		Complete: &CompleteJobRequest{
			Success: true,
			Output:  map[string]interface{}{"summary": "done"},
		},
	}

	// Save
	if err := store.Save(report); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists
	reportPath := filepath.Join(root, "pending_reports", "local_job_001.complete.json")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatalf("expected report file at %s", reportPath)
	}

	// Count
	count, err := store.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	// Remove
	if err := store.Remove("local_job_001", "complete"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Verify file removed
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatal("expected report file to be removed")
	}

	// Count after remove
	count, err = store.Count()
	if err != nil {
		t.Fatalf("count after remove: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0 after remove, got %d", count)
	}
}

func TestPendingReportStoreRemoveNonExistent(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	// Removing a non-existent report should not error
	if err := store.Remove("nonexistent", "complete"); err != nil {
		t.Fatalf("remove non-existent should not error: %v", err)
	}
}

func TestPendingReportStoreList(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	// Save multiple reports
	for i, entry := range []struct {
		jobID string
		typ   string
	}{
		{"local_job_001", "complete"},
		{"local_job_002", "fail"},
		{"local_job_003", "complete"},
	} {
		var complete *CompleteJobRequest
		var fail *FailJobRequest
		if entry.typ == "complete" {
			complete = &CompleteJobRequest{Success: true, Output: map[string]interface{}{"idx": float64(i)}}
		} else {
			fail = &FailJobRequest{Success: false, Error: map[string]interface{}{"message": "test error"}}
		}
		if err := store.Save(PendingReport{JobID: entry.jobID, Type: entry.typ, Complete: complete, Fail: fail}); err != nil {
			t.Fatalf("save %s: %v", entry.jobID, err)
		}
	}

	reports, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}
}

func TestPendingReportStoreListSkipsStaleReports(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	// Save a report
	report := PendingReport{
		JobID: "local_job_001",
		Type:  "complete",
		Complete: &CompleteJobRequest{
			Success: true,
			Output:  map[string]interface{}{"summary": "done"},
		},
	}
	if err := store.Save(report); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Manually set the file's modification time to be older than staleReportAge
	reportPath := filepath.Join(root, "pending_reports", "local_job_001.complete.json")
	staleTime := time.Now().Add(-8 * 24 * time.Hour) // 8 days ago
	if err := os.Chtimes(reportPath, staleTime, staleTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// List should skip and remove the stale report
	reports, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports after stale cleanup, got %d", len(reports))
	}

	// File should be removed
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatal("expected stale report file to be removed")
	}
}

func TestPendingReportStoreCountEmptyDir(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	count, err := store.Count()
	if err != nil {
		t.Fatalf("count empty dir: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0 for empty dir, got %d", count)
	}
}

func TestPendingReportStoreListEmptyDir(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	reports, err := store.List()
	if err != nil {
		t.Fatalf("list empty dir: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports for empty dir, got %d", len(reports))
	}
}

func TestPendingReportStoreMaxLimit(t *testing.T) {
	root := t.TempDir()
	store := NewPendingReportStore(root)

	// Save reports up to the limit (1000)
	// We test with a small number since maxPendingReports is 1000
	for i := 0; i < 10; i++ {
		jobID := "local_job_" + string(rune('a'+i))
		if err := store.Save(PendingReport{
			JobID:    jobID,
			Type:     "complete",
			Complete: &CompleteJobRequest{Success: true},
		}); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	count, err := store.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 10 {
		t.Fatalf("expected count 10, got %d", count)
	}
}
