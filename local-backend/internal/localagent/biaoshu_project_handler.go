package localagent

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleBiaoshuProjectCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_, _ = s.migrateLegacyBiaoshuProjects()
		projects, err := s.listBiaoshuProjectManifests()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})
	case http.MethodPost:
		var req BiaoshuProjectCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid biaoshu project payload")
			return
		}
		project, err := s.createBiaoshuProject(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"project": project})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleBiaoshuProjectResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/local/biaoshu/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || !isSafePathSegment(parts[0]) {
		writeError(w, http.StatusBadRequest, "projectId is required and must be a safe path segment")
		return
	}
	projectID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		project, err := s.readBiaoshuProjectManifest(projectID)
		if err != nil {
			writeError(w, http.StatusNotFound, "biaoshu project not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"project": project})
		return
	}
	if len(parts) == 2 && parts[1] == "artifacts" {
		s.handleBiaoshuProjectArtifacts(w, r, projectID)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleBiaoshuProjectArtifacts(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BiaoshuArtifactRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid biaoshu artifact payload")
		return
	}
	project, err := s.registerBiaoshuProjectArtifact(projectID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"project": project})
}
