package localagent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthAndPathsUseLocalDataDir(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root, CloudAPIBase: "https://cloud.example.com/api"})

	req := httptest.NewRequest(http.MethodGet, "/api/local/health", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var health map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("invalid health response: %v", err)
	}
	if health["service"] != "tangying-local-agent" {
		t.Fatalf("unexpected service: %#v", health)
	}
	if health["cloudApiBase"] != "https://cloud.example.com/api" {
		t.Fatalf("cloud API base not exposed: %#v", health)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/local/paths", nil)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paths status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var paths Paths
	if err := json.Unmarshal(rec.Body.Bytes(), &paths); err != nil {
		t.Fatalf("invalid paths response: %v", err)
	}
	if paths.DataDir != root {
		t.Fatalf("data dir = %q, want %q", paths.DataDir, root)
	}
	for _, dir := range []string{paths.CacheDir, paths.ProjectDir, paths.ArtifactDir, paths.LogDir, paths.DiagnosticsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %q to exist, stat=%v err=%v", dir, info, err)
		}
	}
}

func TestWriteLogAndCreateDiagnostics(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{DataDir: root})

	body := bytes.NewBufferString(`{"source":"desktop","level":"info","message":"render complete","fields":{"project":"demo"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/logs", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("log status = %d, body = %s", rec.Code, rec.Body.String())
	}
	logFile := filepath.Join(root, "logs", "local-agent.jsonl")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected log file: %v", err)
	}
	if !bytes.Contains(content, []byte("render complete")) {
		t.Fatalf("log file missing message: %s", string(content))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/local/diagnostics", bytes.NewBufferString(`{"reason":"support-request"}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp DiagnosticResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid diagnostics response: %v", err)
	}
	if resp.Path == "" {
		t.Fatalf("diagnostics path is empty")
	}
	zr, err := zip.OpenReader(resp.Path)
	if err != nil {
		t.Fatalf("diagnostics zip not readable: %v", err)
	}
	defer zr.Close()
	var hasManifest, hasLog bool
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			hasManifest = true
		}
		if f.Name == "logs/local-agent.jsonl" {
			hasLog = true
		}
	}
	if !hasManifest || !hasLog {
		t.Fatalf("diagnostics zip missing files: manifest=%v log=%v", hasManifest, hasLog)
	}
}

func TestHandlerAllowsLocalFrontendCORS(t *testing.T) {
	server := NewServer(Config{DataDir: t.TempDir()})

	req := httptest.NewRequest(http.MethodOptions, "/api/local/diagnostics", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("allow-methods header missing")
	}
}
