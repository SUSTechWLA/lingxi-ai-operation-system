package localagent

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ── Response types ──

// BiaoshuHistoryResponse is the unified history endpoint payload.
type BiaoshuHistoryResponse struct {
	Projects []BiaoshuHistoryProject `json:"projects"`
	Sources  BiaoshuHistorySources   `json:"sources"`
	Warnings []string                `json:"warnings,omitempty"`
}

// BiaoshuHistorySources counts projects discovered from each origin.
type BiaoshuHistorySources struct {
	CurrentManaged    int `json:"currentManaged"`
	LegacyTempManaged int `json:"legacyTempManaged"`
	LegacyRuns        int `json:"legacyRuns"`
}

// BiaoshuHistoryProject is a unified view of a single project row.
type BiaoshuHistoryProject struct {
	ProjectID          string `json:"projectId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	ProjectName        string `json:"projectName"`
	BidFilePath        string `json:"bidFilePath,omitempty"`
	OutputDir          string `json:"outputDir,omitempty"`
	Status             string `json:"status"`
	CurrentStage       string `json:"currentStage,omitempty"`
	GeneratedCount     int    `json:"generatedCount"`
	TotalCount         int    `json:"totalCount"`
	UpdatedAt          string `json:"updatedAt"`
	Source             string `json:"source"`
	HasManagedManifest bool   `json:"hasManagedManifest"`
	HasLegacyRun       bool   `json:"hasLegacyRun"`
}

// ── Handler ──

func (s *Server) handleBiaoshuHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	resp, err := s.buildBiaoshuHistory(s.legacyManagedBiaoshuProjectRoots())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── Main builder ──

func (s *Server) buildBiaoshuHistory(legacyManagedRoots []string) (BiaoshuHistoryResponse, error) {
	resp := BiaoshuHistoryResponse{
		Projects: []BiaoshuHistoryProject{},
	}

	// Current managed projects
	currentManifests, err := s.listBiaoshuProjectManifests()
	if err != nil {
		resp.Warnings = append(resp.Warnings, "current managed root read failed: "+err.Error())
	} else {
		for _, m := range currentManifests {
			resp.Projects = append(resp.Projects, biaoshuHistoryFromManagedProject(m, "current_managed"))
		}
		resp.Sources.CurrentManaged = len(currentManifests)
	}

	// Legacy temp managed projects (read-only direct scan)
	for _, root := range legacyManagedRoots {
		manifests, err := readBiaoshuManifestsFromRoot(root)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "legacy temp scan failed for "+root+": "+err.Error())
			continue
		}
		existingIDs := make(map[string]bool)
		for _, p := range resp.Projects {
			if p.ProjectID != "" {
				existingIDs[p.ProjectID] = true
			}
		}
		for _, m := range manifests {
			if existingIDs[m.ProjectID] {
				continue
			}
			resp.Projects = append(resp.Projects, biaoshuHistoryFromManagedProject(m, "legacy_temp_managed"))
			resp.Sources.LegacyTempManaged++
		}
	}

	// Legacy run-based projects
	legacyRecords, err := s.readBiaoshuProjects()
	if err != nil {
		resp.Warnings = append(resp.Warnings, "legacy run store read failed: "+err.Error())
	} else {
		for _, rec := range legacyRecords {
			resp.Projects = append(resp.Projects, biaoshuHistoryFromLegacyProject(rec))
		}
		resp.Sources.LegacyRuns = len(legacyRecords)
	}

	// Merge duplicates
	resp.Projects = mergeBiaoshuHistoryProjects(resp.Projects)

	// Sort by updatedAt descending
	sort.Slice(resp.Projects, func(i, j int) bool {
		return resp.Projects[i].UpdatedAt > resp.Projects[j].UpdatedAt
	})

	return resp, nil
}

// ── Conversion helpers ──

var (
	statusMap = map[string]string{
		"SUCCESS": "SUCCESS",
		"RUNNING": "RUNNING",
		"FAILED":  "FAILED",
		"PAUSED":  "PAUSED",
	}
)

func normalizeHistoryStatus(s string) string {
	if v, ok := statusMap[s]; ok {
		return v
	}
	return s
}

func biaoshuHistoryFromManagedProject(m BiaoshuProjectManifest, source string) BiaoshuHistoryProject {
	genCount := 0
	totalCount := len(m.Artifacts)
	for _, a := range m.Artifacts {
		if a.Status == "valid" && strings.TrimSpace(a.StorageRef) != "" {
			genCount++
		}
	}
	runID := ""
	if len(m.Runs) > 0 {
		runID = m.Runs[len(m.Runs)-1].RunID
	}
	bidFilePath := ""
	if len(m.SourceFiles) > 0 {
		bidFilePath = m.SourceFiles[0].Path
	}
	return BiaoshuHistoryProject{
		ProjectID:          m.ProjectID,
		RunID:              runID,
		ProjectName:        m.ProjectName,
		BidFilePath:        bidFilePath,
		OutputDir:          m.OutputDir,
		Status:             normalizeHistoryStatus(m.Status),
		CurrentStage:       string(m.CurrentStage),
		GeneratedCount:     genCount,
		TotalCount:         totalCount,
		UpdatedAt:          m.UpdatedAt,
		Source:             source,
		HasManagedManifest: true,
	}
}

func biaoshuHistoryFromLegacyProject(rec BiaoshuProjectRecord) BiaoshuHistoryProject {
	return BiaoshuHistoryProject{
		RunID:          rec.RunID,
		ProjectName:    rec.ProjectName,
		BidFilePath:    rec.BidFilePath,
		Status:         normalizeHistoryStatus(rec.Status),
		GeneratedCount: rec.ValidArtifactCount,
		TotalCount:     rec.ArtifactCount,
		UpdatedAt:      rec.UpdatedAt,
		Source:         "legacy_run",
		HasLegacyRun:   true,
	}
}

// ── Merge / dedup ──

func biaoshuHistoryProjectKey(item BiaoshuHistoryProject) string {
	if item.ProjectID != "" {
		return "pid:" + item.ProjectID
	}
	bid := strings.TrimSpace(item.BidFilePath)
	nm := strings.TrimSpace(item.ProjectName)
	if bid != "" && nm != "" {
		return "np:" + strings.ToLower(nm) + "|" + strings.ToLower(bid)
	}
	out := strings.TrimSpace(item.OutputDir)
	if out != "" {
		return "od:" + strings.ToLower(out)
	}
	return "rid:" + item.RunID
}

var biaoshuHistoryStageRank = map[string]int{
	string(StageWordExported):   100,
	string(StageDraftMerged):    90,
	string(StageWordcheckReady): 80,
	string(StageChaptersReady):  70,
	string(StageOutlineReady):   60,
	string(StageScoringReady):   50,
	string(StageContextReady):   40,
	string(StageAnalysisReady):  30,
	string(StageRawParsed):      20,
	string(StageCreated):        10,
	string(StageFailed):         0,
}

func biaoshuHistoryProjectScore(item BiaoshuHistoryProject) int {
	stageScore := biaoshuHistoryStageRank[item.CurrentStage]
	statusBonus := 0
	if item.Status == "SUCCESS" {
		statusBonus = 10
	}
	return stageScore + statusBonus + item.GeneratedCount
}

func mergeBiaoshuHistoryProjects(items []BiaoshuHistoryProject) []BiaoshuHistoryProject {
	byKey := make(map[string]*BiaoshuHistoryProject)
	for i := range items {
		item := &items[i]
		key := biaoshuHistoryProjectKey(*item)
		existing, exists := byKey[key]
		if !exists {
			byKey[key] = item
			continue
		}
		// Merge flags
		if item.HasManagedManifest {
			existing.HasManagedManifest = true
		}
		if item.HasLegacyRun {
			existing.HasLegacyRun = true
		}
		if item.RunID != "" && existing.RunID == "" {
			existing.RunID = item.RunID
		}
		// Fill missing fields
		if existing.ProjectID == "" {
			existing.ProjectID = item.ProjectID
		}
		if existing.OutputDir == "" {
			existing.OutputDir = item.OutputDir
		}
		if existing.BidFilePath == "" {
			existing.BidFilePath = item.BidFilePath
		}
		if existing.GeneratedCount == 0 {
			existing.GeneratedCount = item.GeneratedCount
		}
		if existing.TotalCount == 0 {
			existing.TotalCount = item.TotalCount
		}
		// Pick best record
		existingScore := biaoshuHistoryProjectScore(*existing)
		itemScore := biaoshuHistoryProjectScore(*item)
		if itemScore > existingScore {
			existing.ProjectName = item.ProjectName
			existing.Status = item.Status
			existing.CurrentStage = item.CurrentStage
			existing.UpdatedAt = item.UpdatedAt
			existing.GeneratedCount = item.GeneratedCount
			existing.TotalCount = item.TotalCount
			existing.Source = item.Source
			existing.ProjectID = item.ProjectID
			if item.RunID != "" {
				existing.RunID = item.RunID
			}
			existing.OutputDir = item.OutputDir
			existing.BidFilePath = item.BidFilePath
		}
	}
	result := make([]BiaoshuHistoryProject, 0, len(byKey))
	for _, p := range byKey {
		result = append(result, *p)
	}
	return result
}

// Prevent unused import compile error in case json is needed for future diagnostics.
var _ = json.Marshal

// readBiaoshuManifestsFromRoot reads all project.json manifests directly from
// a legacy biaoshu project root without any side effects.
func readBiaoshuManifestsFromRoot(root string) ([]BiaoshuProjectManifest, error) {
	projDir := filepath.Join(root, "projects", biaoshuProjectDirName)
	entries, err := os.ReadDir(projDir)
	if err != nil {
		return nil, err
	}
	var manifests []BiaoshuProjectManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(projDir, entry.Name(), biaoshuProjectManifestFilename)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m BiaoshuProjectManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.SchemaVersion == "" {
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
