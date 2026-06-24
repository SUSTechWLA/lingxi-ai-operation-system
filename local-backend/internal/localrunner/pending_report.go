package localrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PendingReport represents a locally-persisted job completion/failure report
// that has not yet been successfully delivered to the cloud backend.
// Reports are persisted before cloud delivery and deleted after success,
// ensuring no result is lost on network failure.
type PendingReport struct {
	JobID    string      `json:"jobId"`
	Type     string      `json:"type"` // "complete" or "fail"
	Complete *CompleteJobRequest `json:"complete,omitempty"`
	Fail     *FailJobRequest     `json:"fail,omitempty"`
}

// PendingReportStore manages persistent storage of pending job reports.
type PendingReportStore struct {
	dir string
}

// maxPendingReports limits how many reports accumulate on disk before
// Save starts rejecting new reports to prevent disk exhaustion.
const maxPendingReports = 1000

// staleReportAge is the age after which an undelivered report is
// considered stale and can be cleaned up.
const staleReportAge = 7 * 24 * time.Hour

// NewPendingReportStore creates a store rooted at the given directory.
func NewPendingReportStore(dataDir string) *PendingReportStore {
	return &PendingReportStore{dir: filepath.Join(dataDir, "pending_reports")}
}

// Save persists a pending report to disk before attempting cloud delivery.
// Returns an error if the number of pending reports exceeds maxPendingReports.
func (s *PendingReportStore) Save(report PendingReport) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create pending_reports dir: %w", err)
	}

	// Limit check: refuse to save if we already have too many pending reports.
	count, _ := s.Count()
	if count >= maxPendingReports {
		return fmt.Errorf("pending report limit reached (%d), refusing to save more; check cloud connectivity", maxPendingReports)
	}

	filePath := s.reportPath(report.JobID, report.Type)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending report: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("write pending report: %w", err)
	}
	return nil
}

// Remove deletes a pending report after successful cloud delivery.
func (s *PendingReportStore) Remove(jobID, reportType string) error {
	filePath := s.reportPath(jobID, reportType)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pending report: %w", err)
	}
	return nil
}

// Count returns the number of pending reports currently on disk.
func (s *PendingReportStore) Count() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count, nil
}

// List returns all pending reports on disk. Stale reports (older than
// staleReportAge) are silently removed during listing.
func (s *PendingReportStore) List() ([]PendingReport, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pending_reports dir: %w", err)
	}

	var reports []PendingReport
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(s.dir, entry.Name())

		// Check staleness: skip and remove reports older than staleReportAge.
		info, err := entry.Info()
		if err == nil {
			if time.Since(info.ModTime()) > staleReportAge {
				_ = os.Remove(filePath)
				continue
			}
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // skip unreadable files
		}
		var report PendingReport
		if err := json.Unmarshal(data, &report); err != nil {
			_ = os.Remove(filePath) // remove corrupted files
			continue
		}
		if report.JobID != "" && (report.Type == "complete" || report.Type == "fail") {
			reports = append(reports, report)
		}
	}
	return reports, nil
}

func (s *PendingReportStore) reportPath(jobID, reportType string) string {
	name := fmt.Sprintf("%s.%s.json", sanitizeSegment(jobID), reportType)
	return filepath.Join(s.dir, name)
}

func sanitizeSegment(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' || r == ':' {
			return '_'
		}
		return r
	}, s)
	return s
}
