package localagent

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Config struct {
	DataDir      string
	CloudAPIBase string
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

type biaoshuArtifactReadRequest struct {
	FilePath    string `json:"filePath"`
	FilePathAlt string `json:"file_path"`
}

type BiaoshuArtifactReadResponse struct {
	FilePath string `json:"filePath"`
	Format   string `json:"format"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
}

type BiaoshuProjectRecord struct {
	RunID              string `json:"runId"`
	ProjectName        string `json:"projectName"`
	BidFilePath        string `json:"bidFilePath"`
	Status             string `json:"status"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
	ArtifactCount      int    `json:"artifactCount,omitempty"`
	ValidArtifactCount int    `json:"validArtifactCount,omitempty"`
}

type BiaoshuProjectListResponse struct {
	Projects []BiaoshuProjectRecord `json:"projects"`
}

type BiaoshuProjectUpsertResponse struct {
	Project  BiaoshuProjectRecord   `json:"project"`
	Projects []BiaoshuProjectRecord `json:"projects"`
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
	s.mux.HandleFunc("/api/local/biaoshu-projects", s.handleBiaoshuProjects)
	s.mux.HandleFunc("/api/local/biaoshu-projects/", s.handleBiaoshuProjectByRunID)
	s.mux.HandleFunc("/api/local/biaoshu-artifacts/read", s.handleReadBiaoshuArtifact)
	s.mux.HandleFunc("/api/local/artifacts", s.handleArtifacts)
	s.mux.HandleFunc("/api/local/artifacts/", s.handleArtifactByID)
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
	includeKey := r.URL.Query().Get("include_key") == "true" && isLocalhost(r)
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

func (s *Server) handleBiaoshuProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	projects, err := s.readBiaoshuProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, BiaoshuProjectListResponse{Projects: projects})
}

func (s *Server) handleBiaoshuProjectByRunID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	runID := strings.TrimPrefix(r.URL.Path, "/api/local/biaoshu-projects/")
	if !isSafePathSegment(runID) {
		writeError(w, http.StatusBadRequest, "runId is required and must be a safe path segment")
		return
	}
	var req BiaoshuProjectRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid biaoshu project payload")
		return
	}
	req.RunID = runID
	project, projects, err := s.upsertBiaoshuProject(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, BiaoshuProjectUpsertResponse{Project: project, Projects: projects})
}

func (s *Server) handleReadBiaoshuArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req biaoshuArtifactReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid biaoshu artifact read payload")
		return
	}
	filePath := strings.TrimSpace(req.FilePath)
	if filePath == "" {
		filePath = strings.TrimSpace(req.FilePathAlt)
	}
	resp, err := s.readBiaoshuArtifactFile(filePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	metadata["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeIndentedJSON(metadataPath, metadata); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LocalArtifactResponse{
		ID:           req.ID,
		ProjectID:    req.ProjectID,
		StorageRef:   req.StorageRef,
		MimeType:     req.MimeType,
		Path:         contentPath,
		MetadataPath: metadataPath,
		Metadata:     metadata,
	})
}

func (s *Server) handleArtifactByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
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
	writeJSON(w, http.StatusOK, resp)
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
	path := filepath.Join(s.paths.DiagnosticsDir, fmt.Sprintf("diagnostics-%s.zip", createdAt.Format("20060102-150405")))
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
		"createdAt":    createdAt.Format(time.RFC3339Nano),
		"reason":       reason,
		"service":      "tangying-local-agent",
		"dataDir":      s.paths.DataDir,
		"cloudApiBase": s.cfg.CloudAPIBase,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	}
	if err := addJSONToZip(zw, "manifest.json", manifest); err != nil {
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
		return addFileToZip(zw, filepath.ToSlash(rel), logPath)
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

func redactFields(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return map[string]interface{}{}
	}
	redacted := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
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

func detectWorkspaceRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if hasWorkspaceMarkers(cwd) {
			return cwd, true
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", false
		}
		cwd = parent
	}
}

func hasWorkspaceMarkers(dir string) bool {
	for _, name := range []string{"frontend", "local-backend", "cloud-backend"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func pathWithinAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func pathWithinRoot(path, root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func (s *Server) biaoshuProjectsPath() string {
	return filepath.Join(s.paths.ProjectDir, "biaoshu-projects.json")
}

func (s *Server) readBiaoshuProjects() ([]BiaoshuProjectRecord, error) {
	if err := s.EnsureDirs(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.biaoshuProjectsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []BiaoshuProjectRecord{}, nil
		}
		return nil, err
	}
	var stored BiaoshuProjectListResponse
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	projects := normalizeBiaoshuProjects(stored.Projects)
	sortBiaoshuProjects(projects)
	return projects, nil
}

func (s *Server) upsertBiaoshuProject(project BiaoshuProjectRecord) (BiaoshuProjectRecord, []BiaoshuProjectRecord, error) {
	if err := validateBiaoshuProject(project); err != nil {
		return BiaoshuProjectRecord{}, nil, err
	}
	projects, err := s.readBiaoshuProjects()
	if err != nil {
		return BiaoshuProjectRecord{}, nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	hasCreatedAt := strings.TrimSpace(project.CreatedAt) != ""
	if strings.TrimSpace(project.UpdatedAt) == "" {
		project.UpdatedAt = now
	}
	next := make([]BiaoshuProjectRecord, 0, len(projects)+1)
	for _, existing := range projects {
		if existing.RunID == project.RunID {
			if strings.TrimSpace(project.ProjectName) == "" {
				project.ProjectName = existing.ProjectName
			}
			if strings.TrimSpace(project.BidFilePath) == "" {
				project.BidFilePath = existing.BidFilePath
			}
			if !hasCreatedAt {
				project.CreatedAt = existing.CreatedAt
			}
			continue
		}
		next = append(next, existing)
	}
	if strings.TrimSpace(project.CreatedAt) == "" {
		project.CreatedAt = now
	}
	next = append(next, project)
	next = normalizeBiaoshuProjects(next)
	sortBiaoshuProjects(next)
	if len(next) > 50 {
		next = next[:50]
	}
	if err := s.writeBiaoshuProjects(next); err != nil {
		return BiaoshuProjectRecord{}, nil, err
	}
	for _, item := range next {
		if item.RunID == project.RunID {
			return item, next, nil
		}
	}
	return project, next, nil
}

func (s *Server) writeBiaoshuProjects(projects []BiaoshuProjectRecord) error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}
	return writeIndentedJSON(s.biaoshuProjectsPath(), BiaoshuProjectListResponse{Projects: projects})
}

func (s *Server) readBiaoshuArtifactFile(filePath string) (BiaoshuArtifactReadResponse, error) {
	if strings.TrimSpace(filePath) == "" {
		return BiaoshuArtifactReadResponse{}, errors.New("filePath is required")
	}
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return BiaoshuArtifactReadResponse{}, err
	}
	if !pathWithinAnyRoot(absPath, s.biaoshuArtifactReadRoots()) {
		return BiaoshuArtifactReadResponse{}, errors.New("filePath is outside trusted local artifact roots")
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BiaoshuArtifactReadResponse{}, errors.New("artifact file not found")
		}
		return BiaoshuArtifactReadResponse{}, err
	}
	if info.IsDir() {
		return BiaoshuArtifactReadResponse{}, errors.New("filePath must point to a file")
	}
	if info.Size() > 10*1024*1024 {
		return BiaoshuArtifactReadResponse{}, errors.New("artifact file is too large to preview")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return BiaoshuArtifactReadResponse{}, err
	}
	return BiaoshuArtifactReadResponse{
		FilePath: absPath,
		Format:   strings.TrimPrefix(strings.ToLower(filepath.Ext(absPath)), "."),
		Content:  string(content),
		Size:     info.Size(),
	}, nil
}

func (s *Server) biaoshuArtifactReadRoots() []string {
	roots := []string{s.paths.DataDir}
	if workspaceRoot, ok := detectWorkspaceRoot(); ok {
		roots = append(roots, workspaceRoot)
	}
	return roots
}

func validateBiaoshuProject(project BiaoshuProjectRecord) error {
	if !isSafePathSegment(project.RunID) {
		return errors.New("runId is required and must be a safe path segment")
	}
	if strings.TrimSpace(project.ProjectName) == "" {
		return errors.New("projectName is required")
	}
	if strings.TrimSpace(project.BidFilePath) == "" {
		return errors.New("bidFilePath is required")
	}
	if project.ArtifactCount < 0 || project.ValidArtifactCount < 0 {
		return errors.New("artifact counts must be non-negative")
	}
	if project.ValidArtifactCount > project.ArtifactCount {
		return errors.New("validArtifactCount cannot exceed artifactCount")
	}
	if !isBiaoshuProjectStatus(project.Status) {
		return fmt.Errorf("unsupported status: %s", project.Status)
	}
	return nil
}

func normalizeBiaoshuProjects(projects []BiaoshuProjectRecord) []BiaoshuProjectRecord {
	next := make([]BiaoshuProjectRecord, 0, len(projects))
	seen := map[string]bool{}
	for _, item := range projects {
		item.RunID = strings.TrimSpace(item.RunID)
		item.ProjectName = strings.TrimSpace(item.ProjectName)
		item.BidFilePath = strings.TrimSpace(item.BidFilePath)
		item.Status = strings.TrimSpace(item.Status)
		if item.Status == "" {
			item.Status = "UNKNOWN"
		}
		if item.RunID == "" || seen[item.RunID] || !isBiaoshuProjectStatus(item.Status) {
			continue
		}
		seen[item.RunID] = true
		next = append(next, item)
	}
	return next
}

func isBiaoshuProjectStatus(status string) bool {
	switch status {
	case "CREATED", "RUNNING", "SUCCESS", "FAILED", "UNKNOWN":
		return true
	default:
		return false
	}
}

func sortBiaoshuProjects(projects []BiaoshuProjectRecord) {
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].UpdatedAt > projects[j].UpdatedAt
	})
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
