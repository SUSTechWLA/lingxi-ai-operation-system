package localrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// NewPendingReportStore creates a store rooted at the given directory.
func NewPendingReportStore(dataDir string) *PendingReportStore {
	return &PendingReportStore{dir: filepath.Join(dataDir, "pending_reports")}
}

// Save persists a pending report to disk before attempting cloud delivery.
func (s *PendingReportStore) Save(report PendingReport) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create pending_reports dir: %w", err)
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

// List returns all pending reports on disk.
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
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // skip unreadable files
		}
		var report PendingReport
		if err := json.Unmarshal(data, &report); err != nil {
			continue // skip corrupted files
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
