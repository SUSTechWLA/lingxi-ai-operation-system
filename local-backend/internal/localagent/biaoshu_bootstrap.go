package localagent

import (
	"net/http"
)

type BiaoshuBootstrapDiagnostics struct {
	ManagedProjectCount int      `json:"managedProjectCount"`
	LegacyProjectCount  int      `json:"legacyProjectCount"`
	MigratedLegacy      int      `json:"migratedLegacy"`
	ImportedManaged     int      `json:"importedManaged"`
	Warnings            []string `json:"warnings,omitempty"`
}

type BiaoshuBootstrapResponse struct {
	Status          string                      `json:"status"`
	DataDir         string                      `json:"dataDir"`
	Projects        []BiaoshuProjectManifest    `json:"projects"`
	History         []BiaoshuHistoryProject     `json:"history"`
	ResumeProjectID string                      `json:"resumeProjectId,omitempty"`
	Diagnostics     BiaoshuBootstrapDiagnostics `json:"diagnostics"`
}

func (s *Server) handleBiaoshuBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	response, err := s.buildBiaoshuBootstrap()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) buildBiaoshuBootstrap() (BiaoshuBootstrapResponse, error) {
	if err := s.EnsureDirs(); err != nil {
		return BiaoshuBootstrapResponse{}, err
	}

	diagnostics := BiaoshuBootstrapDiagnostics{}
	if migrated, err := s.migrateLegacyBiaoshuProjects(); err != nil {
		diagnostics.Warnings = append(diagnostics.Warnings, "legacy project migration failed: "+err.Error())
	} else {
		diagnostics.MigratedLegacy = migrated
	}
	if imported, err := s.importManagedBiaoshuProjectsFromRoots(s.legacyManagedBiaoshuProjectRoots()); err != nil {
		diagnostics.Warnings = append(diagnostics.Warnings, "legacy managed project import failed: "+err.Error())
	} else {
		diagnostics.ImportedManaged = imported
	}

	projects, err := s.listBiaoshuProjectManifests()
	if err != nil {
		return BiaoshuBootstrapResponse{}, err
	}
	history, err := s.buildBiaoshuHistory(s.legacyManagedBiaoshuProjectRoots())
	if err != nil {
		return BiaoshuBootstrapResponse{}, err
	}
	diagnostics.ManagedProjectCount = len(projects)
	diagnostics.LegacyProjectCount = history.Sources.LegacyRuns + history.Sources.LegacyTempManaged
	diagnostics.Warnings = append(diagnostics.Warnings, history.Warnings...)

	resumeProjectID := ""
	if len(projects) > 0 {
		resumeProjectID = projects[0].ProjectID
	}
	return BiaoshuBootstrapResponse{
		Status:          "READY",
		DataDir:         s.paths.DataDir,
		Projects:        projects,
		History:         history.Projects,
		ResumeProjectID: resumeProjectID,
		Diagnostics:     diagnostics,
	}, nil
}
