package localagent

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const defaultAppVersion = "0.1.11"

type Config struct {
	DataDir       string
	CloudAPIBase  string
	CommandRunner CommandRunner
}

type Server struct {
	cfg   Config
	paths Paths
	mux   *http.ServeMux
}

type Paths struct {
	DataDir        string `json:"dataDir"`
	CacheDir       string `json:"cacheDir"`
	ConfigDir      string `json:"configDir"`
	ProjectDir     string `json:"projectDir"`
	ArtifactDir    string `json:"artifactDir"`
	LogDir         string `json:"logDir"`
	DiagnosticsDir string `json:"diagnosticsDir"`
}

type DiagnosticResponse struct {
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
}

type LocalArtifactResponse struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"projectId"`
	StorageRef    string                 `json:"storageRef,omitempty"`
	MimeType      string                 `json:"mimeType,omitempty"`
	ContentHash   string                 `json:"contentHash,omitempty"`
	SizeBytes     int64                  `json:"sizeBytes,omitempty"`
	Path          string                 `json:"path"`
	MetadataPath  string                 `json:"metadataPath"`
	Content       string                 `json:"content,omitempty"`
	ContentBase64 string                 `json:"contentBase64,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type ModelCapability string

const (
	CapabilityTextToText  ModelCapability = "text_to_text"
	CapabilityTextToImage ModelCapability = "text_to_image"
	CapabilityTextToVideo ModelCapability = "text_to_video"
)

type ModelProviderSettingsResponse struct {
	Providers map[ModelCapability]ModelProviderConfig `json:"providers"`
}

type ModelProviderConfig struct {
	BaseURL       string `json:"baseUrl"`
	Model         string `json:"model"`
	APIKey        string `json:"apiKey,omitempty"`
	HasAPIKey     bool   `json:"hasApiKey,omitempty"`
	APIKeyPreview string `json:"apiKeyPreview,omitempty"`
}

type logRequest struct {
	Source  string                 `json:"source"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields"`
}

type diagnosticRequest struct {
	Reason string `json:"reason"`
}

type localArtifactRequest struct {
	ID            string                 `json:"id"`
	ProjectID     string                 `json:"projectId"`
	StorageRef    string                 `json:"storageRef"`
	MimeType      string                 `json:"mimeType"`
	Content       string                 `json:"content"`
	ContentBase64 string                 `json:"contentBase64"`
	Metadata      map[string]interface{} `json:"metadata"`
}

func NewServer(cfg Config) *Server {
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = defaultDataDir()
	}
	s := &Server{cfg: cfg}
	s.paths = Paths{
		DataDir:        cfg.DataDir,
		CacheDir:       filepath.Join(cfg.DataDir, "cache"),
		ConfigDir:      filepath.Join(cfg.DataDir, "config"),
		ProjectDir:     filepath.Join(cfg.DataDir, "projects"),
		ArtifactDir:    filepath.Join(cfg.DataDir, "artifacts"),
		LogDir:         filepath.Join(cfg.DataDir, "logs"),
		DiagnosticsDir: filepath.Join(cfg.DataDir, "diagnostics"),
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withCORS(s.mux)
}

func (s *Server) Paths() Paths {
	return s.paths
}

func (s *Server) EnsureDirs() error {
	for _, dir := range []string{s.paths.DataDir, s.paths.CacheDir, s.paths.ConfigDir, s.paths.ProjectDir, s.paths.ArtifactDir, s.paths.LogDir, s.paths.DiagnosticsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/local/health", s.handleHealth)
	s.mux.HandleFunc("/api/local/paths", s.handlePaths)
	s.mux.HandleFunc("/api/local/logs", s.handleLogs)
	s.mux.HandleFunc("/api/local/model-providers", s.handleModelProviders)
	s.mux.HandleFunc("/api/local/mcp-providers", s.handleMCPProviders)
	s.mux.HandleFunc("/api/local/mcp-providers/status", s.handleMCPProviderStatus)
	s.mux.HandleFunc("/api/local/jimeng/setup/status", s.handleJiMengSetupStatus)
	s.mux.HandleFunc("/api/local/jimeng/setup/install-cli", s.handleJiMengInstallCLI)
	s.mux.HandleFunc("/api/local/jimeng/setup/register-mcp", s.handleJiMengRegisterMCP)
	s.mux.HandleFunc("/api/local/jimeng/setup/login-headless", s.handleJiMengLoginHeadless)
	s.mux.HandleFunc("/api/local/jimeng/setup/check-login", s.handleJiMengCheckLogin)
	s.mux.HandleFunc("/api/local/artifacts", s.handleArtifacts)
	s.mux.HandleFunc("/api/local/artifacts/", s.handleArtifactByID)
	s.mux.HandleFunc("/api/local/media", s.handleLocalProjectMedia)
	s.mux.HandleFunc("/api/local/projects/", s.handleProjectByID)
	s.mux.HandleFunc("/api/local/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("/api/local/openapi.json", handleOpenAPI)
	s.mux.HandleFunc("/api/local/docs", handleDocs)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"service":      "tangying-local-agent",
		"cloudApiBase": s.cfg.CloudAPIBase,
		"dataDir":      s.paths.DataDir,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	})
}

func (s *Server) handlePaths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.paths)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req logRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid log payload")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	if req.Source == "" {
		req.Source = "desktop"
	}
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"source":    req.Source,
		"level":     req.Level,
		"message":   req.Message,
		"fields":    redactFields(req.Fields),
	}
	if err := appendJSONLine(filepath.Join(s.paths.LogDir, "local-agent.jsonl"), entry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleModelProviders(w http.ResponseWriter, r *http.Request) {
	includeKey := r.URL.Query().Get("include_key") == "true" && isLocalhost(r) && strings.TrimSpace(r.Header.Get("Origin")) == ""
	switch r.Method {
	case http.MethodGet:
		settings, err := s.readModelProviderSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, maskModelProviderSettings(settings, includeKey))
	case http.MethodPut:
		var req ModelProviderSettingsResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid model provider payload")
			return
		}
		if err := validateProviderCapabilities(req.Providers); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		current, err := s.readModelProviderSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		merged := mergeModelProviderSettings(current, req.Providers)
		if err := s.writeModelProviderSettings(merged); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, maskModelProviderSettings(merged, includeKey))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		s.handleArtifactUpload(w, r)
		return
	}
	var req localArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact payload")
		return
	}
	if err := validateLocalArtifactScope(req.ProjectID, req.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	contentPath, metadataPath := s.localArtifactPaths(req.ProjectID, req.ID)
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload, err := localArtifactPayload(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	contentHash := localContentHash(payload)
	sizeBytes := int64(len(payload))
	if strings.TrimSpace(req.StorageRef) == "" {
		req.StorageRef = localUploadedArtifactRef(req.ProjectID, req.ID, contentHash, "content")
	}
	if err := os.WriteFile(contentPath, payload, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["id"] = req.ID
	metadata["projectId"] = req.ProjectID
	metadata["storageRef"] = req.StorageRef
	metadata["mimeType"] = req.MimeType
	metadata["contentHash"] = contentHash
	metadata["sizeBytes"] = sizeBytes
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	metadata["updatedAt"] = updatedAt
	ensureLocalArtifactProvenance(metadata, updatedAt)
	if err := writeIndentedJSON(metadataPath, metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LocalArtifactResponse{
		ID:           req.ID,
		ProjectID:    req.ProjectID,
		StorageRef:   req.StorageRef,
		MimeType:     req.MimeType,
		ContentHash:  contentHash,
		SizeBytes:    sizeBytes,
		Path:         contentPath,
		MetadataPath: metadataPath,
		Metadata:     metadata,
	})
}

func (s *Server) handleArtifactUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart artifact payload")
		return
	}
	projectID := strings.TrimSpace(r.FormValue("projectId"))
	id := strings.TrimSpace(r.FormValue("id"))
	if err := validateLocalArtifactScope(projectID, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	contentPath, metadataPath := s.localArtifactPaths(projectID, id)
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dst, err := os.Create(contentPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasher := sha256.New()
	sizeBytes, copyErr := io.Copy(io.MultiWriter(dst, hasher), file)
	closeErr := dst.Close()
	if copyErr != nil {
		writeError(w, http.StatusInternalServerError, copyErr.Error())
		return
	}
	if closeErr != nil {
		writeError(w, http.StatusInternalServerError, closeErr.Error())
		return
	}
	contentHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	mimeType := strings.TrimSpace(r.FormValue("mimeType"))
	if mimeType == "" && header != nil {
		mimeType = strings.TrimSpace(header.Header.Get("Content-Type"))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	storageRef := strings.TrimSpace(r.FormValue("storageRef"))
	if storageRef == "" {
		filename := "upload"
		if header != nil && strings.TrimSpace(header.Filename) != "" {
			filename = header.Filename
		}
		storageRef = localUploadedArtifactRef(projectID, id, contentHash, filename)
	}
	metadata, err := parseLocalArtifactMetadataField(r.FormValue("metadata"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metadata["id"] = id
	metadata["projectId"] = projectID
	metadata["storageRef"] = storageRef
	metadata["mimeType"] = mimeType
	metadata["contentHash"] = contentHash
	metadata["sizeBytes"] = sizeBytes
	if header != nil && strings.TrimSpace(header.Filename) != "" {
		metadata["originalFilename"] = header.Filename
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	metadata["updatedAt"] = updatedAt
	ensureLocalArtifactProvenance(metadata, updatedAt)
	if err := writeIndentedJSON(metadataPath, metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LocalArtifactResponse{
		ID:           id,
		ProjectID:    projectID,
		StorageRef:   storageRef,
		MimeType:     mimeType,
		ContentHash:  contentHash,
		SizeBytes:    sizeBytes,
		Path:         contentPath,
		MetadataPath: metadataPath,
		Metadata:     metadata,
	})
}

func (s *Server) handleArtifactByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/local/artifacts/")
	projectID := r.URL.Query().Get("projectId")
	if err := validateLocalArtifactScope(projectID, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.Method == http.MethodDelete {
		s.deleteLocalArtifact(w, projectID, id)
		return
	}
	contentPath, metadataPath := s.localArtifactPaths(projectID, id)
	if isRawLocalArtifactRequest(r) {
		s.serveLocalArtifactContent(w, r, projectID, id, contentPath, metadataPath)
		return
	}
	if r.Method == http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	content, err := os.ReadFile(contentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metadata := map[string]interface{}{}
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &metadata)
	}
	resp := LocalArtifactResponse{
		ID:           id,
		ProjectID:    projectID,
		Path:         contentPath,
		MetadataPath: metadataPath,
		Metadata:     metadata,
	}
	if isTextMime(resp.MimeType) {
		resp.Content = string(content)
	} else {
		resp.ContentBase64 = base64.StdEncoding.EncodeToString(content)
	}
	if value, ok := metadata["storageRef"].(string); ok {
		resp.StorageRef = value
	}
	if value, ok := metadata["mimeType"].(string); ok {
		resp.MimeType = value
		if isTextMime(resp.MimeType) {
			resp.Content = string(content)
			resp.ContentBase64 = ""
		} else {
			resp.Content = ""
			resp.ContentBase64 = base64.StdEncoding.EncodeToString(content)
		}
	}
	if value, ok := metadata["contentHash"].(string); ok {
		resp.ContentHash = value
	}
	if value, ok := metadata["sizeBytes"].(float64); ok {
		resp.SizeBytes = int64(value)
	}
	writeJSON(w, http.StatusOK, resp)
}

func isRawLocalArtifactRequest(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("raw"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func (s *Server) serveLocalArtifactContent(w http.ResponseWriter, r *http.Request, projectID, artifactID, contentPath, metadataPath string) {
	file, err := os.Open(contentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "artifact not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metadata := map[string]interface{}{}
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &metadata)
	}
	mimeType, _ := metadata["mimeType"].(string)
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Tangying-Artifact-ID", artifactID)
	w.Header().Set("X-Tangying-Project-ID", projectID)
	http.ServeContent(w, r, artifactID, info.ModTime(), file)
}

func (s *Server) deleteLocalArtifact(w http.ResponseWriter, projectID, artifactID string) {
	dir := filepath.Join(s.paths.ArtifactDir, projectID, artifactID)
	if err := removeLocalDir(dir); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "deleted",
		"projectId": projectID,
		"id":        artifactID,
		"path":      dir,
	})
}

func (s *Server) localArtifactPaths(projectID, artifactID string) (string, string) {
	dir := filepath.Join(s.paths.ArtifactDir, projectID, artifactID)
	return filepath.Join(dir, "content"), filepath.Join(dir, "metadata.json")
}

func (s *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	projectID := strings.TrimPrefix(r.URL.Path, "/api/local/projects/")
	if !isSafePathSegment(projectID) {
		writeError(w, http.StatusBadRequest, "projectId is required and must be a safe path segment")
		return
	}
	paths := []string{
		filepath.Join(s.paths.ProjectDir, projectID),
		filepath.Join(s.paths.ArtifactDir, projectID),
		filepath.Join(s.paths.CacheDir, projectID),
	}
	deleted := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := removeLocalDir(path); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		deleted = append(deleted, path)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "deleted",
		"projectId":    projectID,
		"deletedPaths": deleted,
	})
}

func (s *Server) handleLocalProjectMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if !isSafePathSegment(projectID) {
		writeError(w, http.StatusBadRequest, "projectId is required and must be a safe path segment")
		return
	}
	requestedPath := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
	if requestedPath == "." || !filepath.IsAbs(requestedPath) {
		writeError(w, http.StatusBadRequest, "path must be an absolute project media path")
		return
	}
	projectRoot := filepath.Clean(filepath.Join(s.paths.ProjectDir, projectID))
	relativePath, err := filepath.Rel(projectRoot, requestedPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "media path must stay inside the requested project")
		return
	}
	file, err := os.Open(requestedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "project media not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "project media not found")
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(requestedPath)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Tangying-Project-ID", projectID)
	http.ServeContent(w, r, filepath.Base(requestedPath), info.ModTime(), file)
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req diagnosticRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.EnsureDirs(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	createdAt := time.Now().UTC()
	path := filepath.Join(s.paths.DiagnosticsDir, "beta-diagnostics.zip")
	if err := s.createDiagnosticsZip(path, req.Reason, createdAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, DiagnosticResponse{Path: path, CreatedAt: createdAt.Format(time.RFC3339Nano)})
}

func (s *Server) createDiagnosticsZip(path, reason string, createdAt time.Time) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	manifest := map[string]interface{}{
		"schemaVersion": 1,
		"createdAt":     createdAt.Format(time.RFC3339Nano),
		"reason":        reason,
		"service":       "tangying-local-agent",
		"appVersion":    diagnosticAppVersion(),
		"gitCommit":     diagnosticGitCommit(),
		"recentTaskIds": s.diagnosticRecentTaskIDs(),
		"dataDir":       s.paths.DataDir,
		"cloudApiBase":  s.cfg.CloudAPIBase,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
	}
	if err := addJSONToZip(zw, "manifest.json", manifest); err != nil {
		return err
	}
	if err := addJSONToZip(zw, "environment.json", s.diagnosticEnvironment(createdAt)); err != nil {
		return err
	}
	if err := addJSONToZip(zw, "env-redacted.json", diagnosticEnvRedacted()); err != nil {
		return err
	}
	if err := addJSONToZip(zw, "mcp/provider-status.json", s.diagnosticMCPProviders()); err != nil {
		return err
	}
	if err := addJSONToZip(zw, "artifacts/manifest.json", s.diagnosticArtifactManifest()); err != nil {
		return err
	}
	if err := s.addDiagnosticQAReports(zw); err != nil {
		return err
	}
	if err := s.addDiagnosticPipelineReports(zw); err != nil {
		return err
	}
	if err := s.addDiagnosticFailureStacks(zw); err != nil {
		return err
	}
	return filepath.WalkDir(s.paths.LogDir, func(logPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.paths.DataDir, logPath)
		if err != nil {
			return err
		}
		return addRedactedFileToZip(zw, filepath.ToSlash(rel), logPath)
	})
}

func (s *Server) diagnosticEnvironment(createdAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"schemaVersion": 1,
		"createdAt":     createdAt.Format(time.RFC3339Nano),
		"service":       "tangying-local-agent",
		"appVersion":    diagnosticAppVersion(),
		"gitCommit":     diagnosticGitCommit(),
		"goVersion":     runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"paths": map[string]string{
			"dataDir":        s.paths.DataDir,
			"cacheDir":       s.paths.CacheDir,
			"configDir":      s.paths.ConfigDir,
			"projectDir":     s.paths.ProjectDir,
			"artifactDir":    s.paths.ArtifactDir,
			"logDir":         s.paths.LogDir,
			"diagnosticsDir": s.paths.DiagnosticsDir,
		},
	}
}

func diagnosticAppVersion() string {
	if version := strings.TrimSpace(os.Getenv("TANGYING_APP_VERSION")); version != "" {
		return version
	}
	return defaultAppVersion
}

func diagnosticGitCommit() string {
	for _, key := range []string{"TANGYING_GIT_COMMIT", "GIT_COMMIT"} {
		if commit := strings.TrimSpace(os.Getenv(key)); commit != "" {
			return commit
		}
	}
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "unknown"
	}
	return commit
}

func (s *Server) diagnosticRecentTaskIDs() []string {
	ids := []string{}
	add := func(value interface{}) {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return
		}
		for _, existing := range ids {
			if existing == text {
				return
			}
		}
		ids = append(ids, text)
	}
	_ = filepath.WalkDir(s.paths.LogDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]interface{}
			if json.Unmarshal([]byte(line), &entry) != nil {
				continue
			}
			collectTaskID(entry, add)
			if fields, ok := entry["fields"].(map[string]interface{}); ok {
				collectTaskID(fields, add)
			}
		}
		return nil
	})
	_ = filepath.WalkDir(s.paths.ArtifactDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "metadata.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var metadata map[string]interface{}
		if json.Unmarshal(data, &metadata) == nil {
			collectTaskID(metadata, add)
		}
		return nil
	})
	if len(ids) > 10 {
		return ids[len(ids)-10:]
	}
	return ids
}

func collectTaskID(input map[string]interface{}, add func(interface{})) {
	for _, key := range []string{"taskId", "taskID", "task_id", "jobId", "jobID", "job_id", "runId", "runID", "workflowRunId"} {
		if value, ok := input[key]; ok {
			add(value)
		}
	}
}

func diagnosticEnvRedacted() map[string]interface{} {
	keys := []string{
		"TANGYING_CLOUD_API_BASE",
		"TANGYING_LOCAL_AGENT_ADDR",
		"TANGYING_LOCAL_DATA_DIR",
		"TANGYING_DEVICE_ID",
		"TANGYING_USER_TOKEN",
		"TANGYING_HYPERFRAMES_SERVICE_URL",
		"OPENAI_API_KEY",
		"JIMENG_COOKIE",
		"DREAMINA_TOKEN",
	}
	out := map[string]interface{}{"schemaVersion": 1, "variables": map[string]interface{}{}}
	variables := out["variables"].(map[string]interface{})
	for _, key := range keys {
		value := os.Getenv(key)
		entry := map[string]interface{}{"configured": strings.TrimSpace(value) != ""}
		if entry["configured"].(bool) {
			if isSensitiveKey(key) {
				entry["value"] = "[REDACTED]"
			} else {
				entry["value"] = value
			}
		}
		variables[key] = entry
	}
	return out
}

func (s *Server) diagnosticMCPProviders() map[string]interface{} {
	providers, err := s.readMCPProviders()
	if err != nil {
		return map[string]interface{}{"schemaVersion": 1, "error": err.Error(), "providers": []interface{}{}}
	}
	items := make([]interface{}, 0, len(providers))
	for _, provider := range providers {
		data, _ := json.Marshal(provider)
		var entry map[string]interface{}
		_ = json.Unmarshal(data, &entry)
		if env, ok := entry["env"].(map[string]interface{}); ok {
			entry["env"] = redactMap(env)
		}
		entry["reachable"] = false
		entry["diagnosticNote"] = "provider config snapshot only; live tools/list is checked by /api/local/mcp-providers/status"
		items = append(items, entry)
	}
	return map[string]interface{}{"schemaVersion": 1, "providers": items}
}

func (s *Server) diagnosticArtifactManifest() map[string]interface{} {
	items := []interface{}{}
	_ = filepath.WalkDir(s.paths.ArtifactDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "metadata.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil
		}
		if rel, err := filepath.Rel(s.paths.DataDir, path); err == nil {
			metadata["metadataPath"] = filepath.ToSlash(rel)
		}
		items = append(items, redactMap(metadata))
		return nil
	})
	return map[string]interface{}{"schemaVersion": 1, "artifacts": items}
}

func (s *Server) addDiagnosticQAReports(zw *zip.Writer) error {
	added := false
	return filepath.WalkDir(s.paths.ProjectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "shot_qa_reports.json" {
			return nil
		}
		name := "qa/shot_qa_reports.json"
		if added {
			if rel, relErr := filepath.Rel(s.paths.ProjectDir, path); relErr == nil {
				name = "qa/" + filepath.ToSlash(rel)
			}
		}
		added = true
		return addRedactedFileToZip(zw, name, path)
	})
}

func (s *Server) addDiagnosticPipelineReports(zw *zip.Writer) error {
	known := map[string]bool{
		"shot_list.json":                true,
		"shot_split_report.json":        true,
		"shot_duration_validation.json": true,
		"shot_candidates.json":          true,
		"shot_qa_reports.json":          true,
		"repair_plans.json":             true,
		"shot_repair_plan.json":         true,
		"accepted_shots.json":           true,
		"assembly_plan.json":            true,
		"subtitle_timeline.json":        true,
		"audio_mix_plan.json":           true,
		"final_qa_report.json":          true,
		"artifact_manifest.json":        true,
		"provenance_summary.json":       true,
	}
	return filepath.WalkDir(s.paths.ProjectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !known[d.Name()] {
			return nil
		}
		rel, relErr := filepath.Rel(s.paths.ProjectDir, path)
		if relErr != nil {
			return relErr
		}
		return addRedactedFileToZip(zw, "pipeline/"+filepath.ToSlash(rel), path)
	})
}

func (s *Server) addDiagnosticFailureStacks(zw *zip.Writer) error {
	return filepath.WalkDir(s.paths.LogDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lower := strings.ToLower(d.Name())
		if !strings.Contains(lower, "failure") && !strings.Contains(lower, "stack") && !strings.Contains(lower, "panic") {
			return nil
		}
		return addRedactedFileToZip(zw, "failures/"+d.Name(), path)
	})
}

func appendJSONLine(path string, value interface{}) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func writeIndentedJSON(path string, value interface{}) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func localArtifactPayload(req localArtifactRequest) ([]byte, error) {
	if strings.TrimSpace(req.ContentBase64) != "" {
		payload, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return nil, errors.New("contentBase64 is not valid base64")
		}
		return payload, nil
	}
	return []byte(req.Content), nil
}

func localContentHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseLocalArtifactMetadataField(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}, nil
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, errors.New("metadata must be a JSON object")
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return metadata, nil
}

func ensureLocalArtifactProvenance(metadata map[string]interface{}, generatedAt string) {
	if metadata == nil {
		return
	}
	provenance := map[string]interface{}{}
	if existing, ok := metadata["provenance"].(map[string]interface{}); ok {
		for key, value := range existing {
			provenance[key] = value
		}
	}
	sourceType := firstMetadataString(provenance, metadata, "sourceType")
	if sourceType == "" {
		sourceType = "uploaded"
	}
	providerName := firstMetadataString(provenance, metadata, "providerName")
	if providerName == "" {
		providerName = "local-upload"
	}
	providerJobID := firstMetadataString(provenance, metadata, "providerJobId")
	fallbackReason := firstMetadataString(provenance, metadata, "fallbackReason")
	inputPromptHash := firstMetadataString(provenance, metadata, "inputPromptHash")
	if generatedAt == "" {
		generatedAt = firstMetadataString(provenance, metadata, "generatedAt")
	}
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	isFallback := metadataBool(provenance, "isFallback", metadataBool(metadata, "isFallback", strings.HasPrefix(sourceType, "fallback_")))
	sourceArtifactIDs := metadataStringList(firstMetadataValue(provenance, metadata, "sourceArtifactIds"))
	if len(sourceArtifactIDs) == 0 {
		sourceArtifactIDs = metadataStringList(firstMetadataValue(provenance, metadata, "referenceAssetIds"))
	}

	provenance["schemaVersion"] = 1
	provenance["sourceType"] = sourceType
	provenance["providerName"] = providerName
	provenance["providerJobId"] = providerJobID
	provenance["fallbackReason"] = fallbackReason
	provenance["isFallback"] = isFallback
	provenance["generatedAt"] = generatedAt
	provenance["inputPromptHash"] = inputPromptHash
	provenance["sourceArtifactIds"] = sourceArtifactIDs

	metadata["schemaVersion"] = 1
	metadata["sourceType"] = sourceType
	metadata["providerName"] = providerName
	metadata["providerJobId"] = providerJobID
	metadata["fallbackReason"] = fallbackReason
	metadata["isFallback"] = isFallback
	metadata["generatedAt"] = generatedAt
	metadata["inputPromptHash"] = inputPromptHash
	metadata["sourceArtifactIds"] = sourceArtifactIDs
	metadata["provenance"] = provenance
}

func firstMetadataString(primary, secondary map[string]interface{}, key string) string {
	if value := strings.TrimSpace(fmt.Sprint(primary[key])); value != "" && value != "<nil>" {
		return value
	}
	if value := strings.TrimSpace(fmt.Sprint(secondary[key])); value != "" && value != "<nil>" {
		return value
	}
	return ""
}

func firstMetadataValue(primary, secondary map[string]interface{}, key string) interface{} {
	if value, ok := primary[key]; ok {
		return value
	}
	return secondary[key]
}

func metadataBool(input map[string]interface{}, key string, fallback bool) bool {
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return fallback
}

func metadataStringList(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) != "" {
			return []interface{}{strings.TrimSpace(typed)}
		}
	}
	return []interface{}{}
}

func localUploadedArtifactRef(projectID, artifactID, contentHash, filename string) string {
	hash := strings.TrimPrefix(strings.TrimSpace(contentHash), "sha256:")
	if hash == "" {
		hash = "pending"
	}
	return "local://projects/" +
		safeLocalRefSegment(projectID) +
		"/artifacts/" +
		safeLocalRefSegment(artifactID) +
		"/" +
		safeLocalRefSegment(hash) +
		"/" +
		safeLocalRefSegment(filename)
}

func safeLocalRefSegment(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	cleaned := strings.Trim(b.String(), ".-_")
	if cleaned == "" {
		return "artifact"
	}
	return cleaned
}

func isTextMime(mimeType string) bool {
	lower := strings.ToLower(strings.TrimSpace(mimeType))
	return lower == "" ||
		strings.HasPrefix(lower, "text/") ||
		strings.Contains(lower, "json") ||
		strings.Contains(lower, "xml") ||
		strings.Contains(lower, "markdown")
}

func defaultModelProviderSettings() map[ModelCapability]ModelProviderConfig {
	return map[ModelCapability]ModelProviderConfig{
		CapabilityTextToText: {
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4.1",
		},
		CapabilityTextToImage: {
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-image-1",
		},
		CapabilityTextToVideo: {
			BaseURL: "https://api.openai.com/v1",
			Model:   "sora",
		},
	}
}

func validateProviderCapabilities(providers map[ModelCapability]ModelProviderConfig) error {
	for capability := range providers {
		if !isSupportedModelCapability(capability) {
			return fmt.Errorf("unsupported model capability: %s", capability)
		}
	}
	return nil
}

func isSupportedModelCapability(capability ModelCapability) bool {
	switch capability {
	case CapabilityTextToText, CapabilityTextToImage, CapabilityTextToVideo:
		return true
	default:
		return false
	}
}

func mergeModelProviderSettings(current, updates map[ModelCapability]ModelProviderConfig) map[ModelCapability]ModelProviderConfig {
	merged := map[ModelCapability]ModelProviderConfig{}
	for capability, cfg := range current {
		merged[capability] = cfg
	}
	for capability, update := range updates {
		cfg := merged[capability]
		if strings.TrimSpace(update.BaseURL) != "" {
			cfg.BaseURL = strings.TrimSpace(update.BaseURL)
		}
		if strings.TrimSpace(update.Model) != "" {
			cfg.Model = strings.TrimSpace(update.Model)
		}
		if update.APIKey != "" {
			cfg.APIKey = update.APIKey
		}
		cfg.HasAPIKey = false
		cfg.APIKeyPreview = ""
		merged[capability] = cfg
	}
	return merged
}

func maskModelProviderSettings(settings map[ModelCapability]ModelProviderConfig, includeKey bool) ModelProviderSettingsResponse {
	masked := make(map[ModelCapability]ModelProviderConfig, len(settings))
	for capability, cfg := range settings {
		cfg.HasAPIKey = cfg.APIKey != ""
		cfg.APIKeyPreview = previewAPIKey(cfg.APIKey)
		if !includeKey {
			cfg.APIKey = ""
		}
		masked[capability] = cfg
	}
	return ModelProviderSettingsResponse{Providers: masked}
}

func isLocalhost(r *http.Request) bool {
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	// Remove brackets from IPv6 addresses
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func previewAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return "••••"
	}
	return apiKey[:4] + "••••" + apiKey[len(apiKey)-4:]
}

func addJSONToZip(zw *zip.Writer, name string, value interface{}) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func addFileToZip(zw *zip.Writer, name, path string) error {
	src, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer src.Close()
	dst, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

func addRedactedFileToZip(zw *zip.Writer, name, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(redactSensitiveText(string(data))))
	return err
}

var sensitiveTextPattern = regexp.MustCompile(`(?i)(\bsk-[A-Za-z0-9_-]+\b|secret-token|bearer\s+[A-Za-z0-9._~+/=-]+|"?authorization"?\s*[:=]\s*"?[^"\s,}]+(?:\s+[A-Za-z0-9._~+/=-]+)?|"?token"?\s*[:=]\s*"?[^"\s,}]+|"?password"?\s*[:=]\s*"?[^"\s,}]+|"?api[_-]?key"?\s*[:=]\s*"?[^"\s,}]+|"?cookie"?\s*[:=]\s*"?[^"\s,}]+|"?secret"?\s*[:=]\s*"?[^"\s,}]+)`)

func redactSensitiveText(text string) string {
	return sensitiveTextPattern.ReplaceAllString(text, "[REDACTED]")
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "token") ||
		strings.Contains(lower, "key") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "authorization") ||
		lower == "auth" ||
		strings.Contains(lower, "credential")
}

func redactMap(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		if isSensitiveKey(key) {
			if strings.TrimSpace(fmt.Sprint(value)) == "" {
				out[key] = ""
			} else {
				out[key] = "[REDACTED]"
			}
			continue
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			out[key] = redactMap(typed)
		case map[string]string:
			nested := make(map[string]interface{}, len(typed))
			for nestedKey, nestedValue := range typed {
				nested[nestedKey] = nestedValue
			}
			out[key] = redactMap(nested)
		case []interface{}:
			items := make([]interface{}, 0, len(typed))
			for _, item := range typed {
				if itemMap, ok := item.(map[string]interface{}); ok {
					items = append(items, redactMap(itemMap))
				} else if text, ok := item.(string); ok {
					items = append(items, redactSensitiveText(text))
				} else {
					items = append(items, item)
				}
			}
			out[key] = items
		case string:
			out[key] = redactSensitiveText(typed)
		default:
			out[key] = value
		}
	}
	return out
}

func redactFields(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return map[string]interface{}{}
	}
	return redactMap(fields)
}

func validateLocalArtifactScope(projectID, artifactID string) error {
	if !isSafePathSegment(projectID) {
		return errors.New("projectId is required and must be a safe path segment")
	}
	if !isSafePathSegment(artifactID) {
		return errors.New("id is required and must be a safe path segment")
	}
	return nil
}

func isSafePathSegment(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return false
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return false
	}
	return true
}

func removeLocalDir(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(path)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigin := allowedLocalOrigin(origin)
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if isMutatingMethod(r.Method) && strings.TrimSpace(origin) != "" && allowedOrigin == "" {
			writeError(w, http.StatusForbidden, "origin is not allowed for local agent write requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func allowedLocalOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return ""
	}
	if origin == "null" {
		return origin
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return origin
	}
	return ""
}

func defaultDataDir() string {
	if dir := os.Getenv("TANGYING_LOCAL_DATA_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "TangyingAIOS")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "TangyingAIOS")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "TangyingAIOS")
		}
		return filepath.Join(home, "AppData", "Roaming", "TangyingAIOS")
	default:
		return filepath.Join(home, ".tangying-aios")
	}
}

func (s *Server) modelProviderConfigPath() string {
	return filepath.Join(s.paths.ConfigDir, "model-providers.json")
}

func (s *Server) readModelProviderSettings() (map[ModelCapability]ModelProviderConfig, error) {
	settings := defaultModelProviderSettings()
	if err := s.EnsureDirs(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.modelProviderConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		return nil, err
	}
	var stored ModelProviderSettingsResponse
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	if err := validateProviderCapabilities(stored.Providers); err != nil {
		return nil, err
	}
	return mergeModelProviderSettings(settings, stored.Providers), nil
}

func (s *Server) writeModelProviderSettings(settings map[ModelCapability]ModelProviderConfig) error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ModelProviderSettingsResponse{Providers: settings}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.modelProviderConfigPath(), append(data, '\n'), 0o600)
}
