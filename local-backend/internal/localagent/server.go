package localagent

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	ProjectDir     string `json:"projectDir"`
	ArtifactDir    string `json:"artifactDir"`
	LogDir         string `json:"logDir"`
	DiagnosticsDir string `json:"diagnosticsDir"`
}

type DiagnosticResponse struct {
	Path      string `json:"path"`
	CreatedAt string `json:"createdAt"`
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

func NewServer(cfg Config) *Server {
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = defaultDataDir()
	}
	s := &Server{cfg: cfg}
	s.paths = Paths{
		DataDir:        cfg.DataDir,
		CacheDir:       filepath.Join(cfg.DataDir, "cache"),
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

func (s *Server) EnsureDirs() error {
	for _, dir := range []string{s.paths.DataDir, s.paths.CacheDir, s.paths.ProjectDir, s.paths.ArtifactDir, s.paths.LogDir, s.paths.DiagnosticsDir} {
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
	s.mux.HandleFunc("/api/local/diagnostics", s.handleDiagnostics)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
